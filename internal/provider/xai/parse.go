package xai

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/core"
)

var (
	// tokens (actual/limit): 501653/500000
	tokenActualLimitRe = regexp.MustCompile(`(?i)tokens\s*\(\s*actual\s*/\s*limit\s*\)\s*:\s*([0-9][0-9,]*)\s*/\s*([0-9][0-9,]*)`)
	// remaining/limit 宽松形态
	remainingLimitRe = regexp.MustCompile(`(?i)(?:remaining|left)\s*[:=]\s*([0-9][0-9,]*)\s*/\s*([0-9][0-9,]*)`)
)

// ParseProbeResponse 解析 xAI 上游探测 HTTP 状态与 body，产出安全额度信号。
// model 为探测所用模型名：free 模型（名含 build-free）上的 spending-limit/out-of-credits
// 在无 weekly 字样时按 free 24h 耗尽处理，避免把免费凭证永久锁成 monthly_and_weekly。
// 仅当能从错误码/文案/用量字段得到可信额度结论时 QuotaKnown=true。
func ParseProbeResponse(statusCode int, body []byte, observedAt time.Time, model string) ProbeResult {
	observedAt = observedAt.UTC()
	trimmed := strings.TrimSpace(string(body))
	code, message := extractErrorFields(body)

	if isFreeUsageExhausted(code, message, trimmed) {
		return freeDepletedResult(observedAt, message+" "+trimmed)
	}

	weekly := isWeeklyDepleted(code, message, trimmed)
	// free 模型 + spending-limit/credits 阻断且无 weekly 字样 → free 24h 耗尽（可刷新启用）。
	// 付费模型上的 spending-limit 仍走下方 monthly_and_weekly。
	if isFreeProbeModel(model) && !weekly && isSpendingOrCreditsBlocked(code, message, trimmed) {
		return freeDepletedResult(observedAt, message+" "+trimmed)
	}

	monthly := isMonthlyDepleted(code, message, trimmed)
	// spending-limit / 月度积分耗尽在现网通常同时阻断可用调用；
	// 若仅识别到 monthly 而未识别 weekly，仍按「周+月耗尽」处理（可禁用）。
	if monthly && !weekly {
		weekly = true
	}
	if weekly || monthly {
		remaining := int64(0)
		resetAt := observedAt.Add(7 * 24 * time.Hour)
		kind := DepletedWeekly
		window := WindowWeekly
		plan := core.PlanTypePlus
		longReset := observedAt.Add(30 * 24 * time.Hour)
		if weekly && monthly {
			kind = DepletedMonthlyAndWeekly
			window = WindowMonthly
		}
		result := ProbeResult{
			ObservedAt:      observedAt,
			ResetAt:         &resetAt,
			Remaining:       &remaining,
			Window:          window,
			Freshness:       core.FreshnessFresh,
			ProbeStatus:     core.ProbeStatusReady,
			Status:          StatusReady,
			PlanType:        plan,
			DepletedKind:    kind,
			QuotaKnown:      true,
			WeeklyDepleted:  weekly,
			MonthlyDepleted: monthly,
		}
		if monthly {
			result.LongWindowResetAt = &longReset
		}
		return result
	}

	// 成功响应：若 body 含 usage 剩余则视为有额度；否则 2xx 轻量成功视为有可用额度（QuotaKnown）。
	if statusCode >= 200 && statusCode < 300 {
		if remaining, limit, ok := parseUsageRemaining(body); ok {
			resetAt := observedAt.Add(24 * time.Hour)
			plan := core.PlanTypeFree
			window := WindowFree24h
			if limit > 500000 {
				plan = core.PlanTypePlus
				window = WindowWeekly
			}
			return ProbeResult{
				ObservedAt:  observedAt,
				ResetAt:     &resetAt,
				Remaining:   &remaining,
				Limit:       &limit,
				Window:      window,
				Freshness:   core.FreshnessFresh,
				ProbeStatus: core.ProbeStatusReady,
				Status:      StatusReady,
				PlanType:    plan,
				QuotaKnown:  true,
			}
		}
		// 2xx 且无显式用量：证明凭证仍可调用，视为正额度（剩余未知时用 1 参与正序）。
		remaining := int64(1)
		resetAt := observedAt.Add(24 * time.Hour)
		return ProbeResult{
			ObservedAt:  observedAt,
			ResetAt:     &resetAt,
			Remaining:   &remaining,
			Window:      WindowUnknown,
			Freshness:   core.FreshnessFresh,
			ProbeStatus: core.ProbeStatusReady,
			Status:      StatusReady,
			PlanType:    core.PlanTypeUnknown,
			QuotaKnown:  true,
		}
	}

	// 错误文案中带 actual/limit 且 actual < limit：仍有额度。
	if actual, limit, ok := parseActualLimit(message + " " + trimmed); ok && limit > 0 {
		remaining := limit - actual
		if remaining < 0 {
			remaining = 0
		}
		resetAt := observedAt.Add(24 * time.Hour)
		if remaining == 0 && (isFreeUsageExhausted(code, message, trimmed) || strings.Contains(strings.ToLower(message+trimmed), "free")) {
			return ProbeResult{
				ObservedAt:   observedAt,
				ResetAt:      &resetAt,
				Remaining:    &remaining,
				Limit:        &limit,
				Window:       WindowFree24h,
				Freshness:    core.FreshnessFresh,
				ProbeStatus:  core.ProbeStatusReady,
				Status:       StatusReady,
				PlanType:     core.PlanTypeFree,
				DepletedKind: DepletedFree,
				QuotaKnown:   true,
			}
		}
		if remaining > 0 {
			return ProbeResult{
				ObservedAt:  observedAt,
				ResetAt:     &resetAt,
				Remaining:   &remaining,
				Limit:       &limit,
				Window:      WindowFree24h,
				Freshness:   core.FreshnessFresh,
				ProbeStatus: core.ProbeStatusReady,
				Status:      StatusReady,
				PlanType:    core.PlanTypeFree,
				QuotaKnown:  true,
			}
		}
	}

	// 保留状态码与脱敏短摘要，便于排查（不含 token）。
	summary := strings.TrimSpace(code + " " + message)
	if summary == "" {
		summary = strings.TrimSpace(string(body))
	}
	if summary == "" {
		summary = fmt.Sprintf("status %d empty body", statusCode)
	} else {
		summary = fmt.Sprintf("status %d %s", statusCode, summary)
	}
	return failedResult(observedAt, safeError(summary))
}

func failedResult(observedAt time.Time, message string) ProbeResult {
	return ProbeResult{
		ObservedAt:  observedAt.UTC(),
		Window:      WindowUnknown,
		Freshness:   core.FreshnessUnknown,
		ProbeStatus: core.ProbeStatusUnknown,
		Status:      StatusProbeFailed,
		PlanType:    core.PlanTypeUnknown,
		QuotaKnown:  false,
		Error:       message,
	}
}

func extractErrorFields(body []byte) (code string, message string) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", strings.TrimSpace(string(body))
	}
	if c, ok := payload["code"].(string); ok {
		code = strings.TrimSpace(c)
	}
	if c, ok := payload["error_code"].(string); ok && code == "" {
		code = strings.TrimSpace(c)
	}
	switch v := payload["error"].(type) {
	case string:
		message = strings.TrimSpace(v)
	case map[string]any:
		if m, ok := v["message"].(string); ok {
			message = strings.TrimSpace(m)
		}
		if c, ok := v["code"].(string); ok && code == "" {
			code = strings.TrimSpace(c)
		}
		if m, ok := v["error"].(string); ok && message == "" {
			message = strings.TrimSpace(m)
		}
	}
	if message == "" {
		if m, ok := payload["message"].(string); ok {
			message = strings.TrimSpace(m)
		}
	}
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	return code, message
}

func isFreeUsageExhausted(code, message, raw string) bool {
	blob := strings.ToLower(code + " " + message + " " + raw)
	return strings.Contains(blob, "free-usage-exhausted") ||
		strings.Contains(blob, "subscription:free-usage-exhausted") ||
		strings.Contains(blob, "included free usage") ||
		(strings.Contains(blob, "free usage") && strings.Contains(blob, "used all")) ||
		(strings.Contains(blob, "rolling 24-hour") && strings.Contains(blob, "tokens"))
}

// isFreeProbeModel：探测模型为 free 额度信号源（build-free / grok-build* / 显式 freeProbeModel）。
func isFreeProbeModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if m == freeProbeModel || m == legacyFreeProbeModel {
		return true
	}
	return strings.Contains(m, "build-free") ||
		strings.HasPrefix(m, "grok-build") ||
		strings.Contains(m, "grok-build-")
}

// isSpendingOrCreditsBlocked：spending-limit / personal-team-blocked / out of credits 类阻断。
func isSpendingOrCreditsBlocked(code, message, raw string) bool {
	blob := strings.ToLower(code + " " + message + " " + raw)
	return strings.Contains(blob, "spending-limit") ||
		strings.Contains(blob, "personal-team-blocked") ||
		strings.Contains(blob, "run out of credits") ||
		strings.Contains(blob, "out of credits") ||
		strings.Contains(blob, "credits exhausted")
}

// freeDepletedResult：free 24h 窗耗尽（ResetAt=+24h，DepletedFree）。
func freeDepletedResult(observedAt time.Time, text string) ProbeResult {
	actual, limit, ok := parseActualLimit(text)
	remaining := int64(0)
	if ok && actual < limit {
		remaining = 0
	}
	resetAt := observedAt.Add(24 * time.Hour)
	return ProbeResult{
		ObservedAt:   observedAt,
		ResetAt:      &resetAt,
		Remaining:    &remaining,
		Limit:        limitPtr(limit, ok),
		Window:       WindowFree24h,
		Freshness:    core.FreshnessFresh,
		ProbeStatus:  core.ProbeStatusReady,
		Status:       StatusReady,
		PlanType:     core.PlanTypeFree,
		DepletedKind: DepletedFree,
		QuotaKnown:   true,
	}
}

func isWeeklyDepleted(code, message, raw string) bool {
	blob := strings.ToLower(code + " " + message + " " + raw)
	if strings.Contains(blob, "free-usage") {
		return false
	}
	return strings.Contains(blob, "weekly") && (strings.Contains(blob, "exhaust") || strings.Contains(blob, "limit") || strings.Contains(blob, "quota")) ||
		strings.Contains(blob, "weekly-usage-exhausted") ||
		strings.Contains(blob, "subscription:weekly")
}

func isMonthlyDepleted(code, message, raw string) bool {
	blob := strings.ToLower(code + " " + message + " " + raw)
	if strings.Contains(blob, "free-usage") {
		return false
	}
	// 付费模型路径：spending-limit / credits 阻断 → monthly（与 free 模型路径分流）。
	if isSpendingOrCreditsBlocked(code, message, raw) {
		return true
	}
	return strings.Contains(blob, "monthly") && (strings.Contains(blob, "exhaust") || strings.Contains(blob, "limit") || strings.Contains(blob, "credit") || strings.Contains(blob, "quota")) ||
		strings.Contains(blob, "monthly-usage-exhausted") ||
		strings.Contains(blob, "subscription:monthly")
}

func parseActualLimit(text string) (actual int64, limit int64, ok bool) {
	if m := tokenActualLimitRe.FindStringSubmatch(text); len(m) == 3 {
		a, errA := parseCommaInt(m[1])
		l, errL := parseCommaInt(m[2])
		if errA == nil && errL == nil {
			return a, l, true
		}
	}
	if m := remainingLimitRe.FindStringSubmatch(text); len(m) == 3 {
		r, errR := parseCommaInt(m[1])
		l, errL := parseCommaInt(m[2])
		if errR == nil && errL == nil {
			return l - r, l, true
		}
	}
	return 0, 0, false
}

func parseUsageRemaining(body []byte) (remaining int64, limit int64, ok bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, 0, false
	}
	// 常见 usage 形态
	if usage, okUsage := payload["usage"].(map[string]any); okUsage {
		if rem, okRem := toInt64(usage["remaining"]); okRem {
			lim, _ := toInt64(usage["limit"])
			return rem, lim, true
		}
		if total, okTotal := toInt64(usage["total_tokens"]); okTotal {
			// 成功调用仅有 total_tokens 时不算耗尽信号
			_ = total
		}
	}
	if rem, okRem := toInt64(payload["remaining"]); okRem {
		lim, _ := toInt64(payload["limit"])
		return rem, lim, true
	}
	return 0, 0, false
}

func parseCommaInt(value string) (int64, error) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	return strconv.ParseInt(cleaned, 10, 64)
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case json.Number:
		i, err := v.Int64()
		return i, err == nil
	case int:
		return int64(v), true
	case int64:
		return v, true
	case string:
		i, err := parseCommaInt(v)
		return i, err == nil
	default:
		return 0, false
	}
}

func limitPtr(limit int64, ok bool) *int64 {
	if !ok || limit <= 0 {
		return nil
	}
	return &limit
}

func safeError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "probe failed"
	}
	// 防止误把整段凭证 JSON 写进错误。
	if strings.Contains(trimmed, "access_token") || strings.Contains(trimmed, "refresh_token") {
		return "probe failed"
	}
	if len(trimmed) > 240 {
		return trimmed[:240]
	}
	return trimmed
}
