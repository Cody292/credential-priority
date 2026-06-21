package codex

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/core"
)

type whamUsage struct {
	PlanType  string        `json:"plan_type"`
	RateLimit whamRateLimit `json:"rate_limit"`
}

type whamRateLimit struct {
	PrimaryWindow   whamWindow `json:"primary_window"`
	SecondaryWindow whamWindow `json:"secondary_window"`
}

type whamWindow struct {
	ResetAt            any `json:"reset_at"`
	ResetAfterSeconds  any `json:"reset_after_seconds"`
	LimitWindowSeconds any `json:"limit_window_seconds"`
	Remaining          any `json:"remaining"`
	Limit              any `json:"limit"`
	Used               any `json:"used"`
	UsedPercent        any `json:"used_percent"`
	LimitReached       any `json:"limit_reached"`
	Name               any `json:"name"`
	Type               any `json:"type"`
	Category           any `json:"category"`
	Label              any `json:"label"`
	Bucket             any `json:"bucket"`
	Scope              any `json:"scope"`
}

// ParseWhamUsage 将 wham/usage JSON 解析为只信任 weekly 窗口的 fresh probe 结果。
func ParseWhamUsage(raw []byte, observedAt time.Time) ProbeResult {
	var usage whamUsage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&usage); err != nil {
		return failedResult(observedAt, "parse wham usage failed")
	}
	window, ok := pickWeeklyWindow(usage)
	if !ok {
		return failedResult(observedAt, "weekly window unavailable")
	}
	resetAt := windowResetTime(observedAt, window)
	remaining, ok := windowRemaining(window)
	if resetAt == nil || !ok {
		return failedResult(observedAt, "weekly window incomplete")
	}
	return ProbeResult{
		ObservedAt:  observedAt.UTC(),
		ResetAt:     resetAt,
		Remaining:   int64Ptr(remaining),
		Window:      WindowWeekly,
		Freshness:   core.FreshnessFresh,
		ProbeStatus: core.ProbeStatusReady,
		Status:      StatusReady,
		PlanType:    inferPlanType(usage.PlanType),
	}
}

func failedResult(observedAt time.Time, message string) ProbeResult {
	return ProbeResult{
		ObservedAt:  observedAt.UTC(),
		Window:      WindowUnknown,
		Freshness:   core.FreshnessUnknown,
		ProbeStatus: core.ProbeStatusUnknown,
		Status:      StatusProbeFailed,
		PlanType:    core.PlanTypeUnknown,
		Error:       message,
	}
}

func pickWeeklyWindow(usage whamUsage) (whamWindow, bool) {
	windows := []whamWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow}
	for _, window := range windows {
		if hasWindowData(window) && isWeeklyWindow(window) {
			return window, true
		}
	}
	return whamWindow{}, false
}

func hasWindowData(window whamWindow) bool {
	if _, ok := parseAnyTime(window.ResetAt); ok {
		return true
	}
	if seconds, ok := toInt64(window.ResetAfterSeconds); ok && seconds > 0 {
		return true
	}
	if _, ok := toFloat64(window.Remaining); ok {
		return true
	}
	if _, ok := toFloat64(window.Limit); ok {
		return true
	}
	if _, ok := toFloat64(window.Used); ok {
		return true
	}
	if _, ok := toFloat64(window.UsedPercent); ok {
		return true
	}
	if _, ok := toBool(window.LimitReached); ok {
		return true
	}
	return false
}

func isWeeklyWindow(window whamWindow) bool {
	if seconds, ok := toInt64(window.LimitWindowSeconds); ok && seconds == 7*24*60*60 {
		return true
	}
	for _, field := range windowMetadataStrings(window) {
		if strings.Contains(field, "weekly") || strings.Contains(field, "week") || strings.Contains(field, "7d") || strings.Contains(field, "7 days") || strings.Contains(field, "7 day") {
			return true
		}
	}
	return false
}

func windowResetTime(observedAt time.Time, window whamWindow) *time.Time {
	if resetAt, ok := parseAnyTime(window.ResetAt); ok {
		return resetAt
	}
	seconds, ok := toInt64(window.ResetAfterSeconds)
	if !ok || seconds <= 0 {
		return nil
	}
	resetAt := observedAt.UTC().Add(time.Duration(seconds) * time.Second)
	return &resetAt
}

func windowRemaining(window whamWindow) (int64, bool) {
	if remaining, ok := toFloat64(window.Remaining); ok {
		return nonNegativeCeil(remaining), true
	}
	limit, okLimit := toFloat64(window.Limit)
	used, okUsed := toFloat64(window.Used)
	if okLimit && okUsed {
		return nonNegativeCeil(limit - used), true
	}
	if reached, ok := toBool(window.LimitReached); ok && reached {
		return 0, true
	}
	if usedPercent, ok := toFloat64(window.UsedPercent); ok {
		remainingPercent := 100 - usedPercent
		return nonNegativeCeil(remainingPercent), true
	}
	return 0, false
}

func parseAnyTime(raw any) (*time.Time, bool) {
	switch value := raw.(type) {
	case nil:
		return nil, false
	case string:
		return parseTimeString(value)
	case float64:
		return parseUnix(int64(value))
	case json.Number:
		integer, err := value.Int64()
		if err != nil {
			return nil, false
		}
		return parseUnix(integer)
	default:
		return nil, false
	}
}

func parseTimeString(value string) (*time.Time, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false
	}
	if integer, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return parseUnix(integer)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			utc := parsed.UTC()
			return &utc, true
		}
	}
	return nil, false
}

func parseUnix(value int64) (*time.Time, bool) {
	if value <= 0 {
		return nil, false
	}
	if value > 1_000_000_000_000 {
		parsed := time.UnixMilli(value).UTC()
		return &parsed, true
	}
	parsed := time.Unix(value, 0).UTC()
	return &parsed, true
}

func windowMetadataStrings(window whamWindow) []string {
	fields := []any{window.Name, window.Type, window.Category, window.Label, window.Bucket, window.Scope}
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		text, ok := toString(field)
		if !ok {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(text))
		normalized = strings.ReplaceAll(normalized, "_", " ")
		normalized = strings.ReplaceAll(normalized, "-", " ")
		if normalized != "" {
			values = append(values, normalized)
		}
	}
	return values
}

func inferPlanType(value string) core.PlanType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "free":
		return core.PlanTypeFree
	case "plus":
		return core.PlanTypePlus
	case "pro":
		return core.PlanTypePro
	case "team":
		return core.PlanTypeTeam
	default:
		return core.PlanTypeUnknown
	}
}

func nonNegativeCeil(value float64) int64 {
	if value <= 0 {
		return 0
	}
	return int64(math.Ceil(value))
}

func int64Ptr(value int64) *int64 {
	return &value
}
