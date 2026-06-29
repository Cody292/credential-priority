package antigravity

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/core"
)

type availableModelsResponse struct {
	Models  map[string]availableModel `json:"models"`
	Buckets []quotaBucket             `json:"buckets"`
	Groups  []quotaGroup              `json:"groups"`
}

type quotaGroup struct {
	DisplayName string        `json:"displayName"`
	Description string        `json:"description"`
	Buckets     []quotaBucket `json:"buckets"`
}

type availableModel struct {
	ModelProvider string    `json:"modelProvider"`
	QuotaInfo     quotaInfo `json:"quotaInfo"`
}

type quotaInfo struct {
	RemainingFraction any           `json:"remainingFraction"`
	ResetTime         any           `json:"resetTime"`
	Windows           []quotaWindow `json:"windows"`
}

type quotaWindow struct {
	Name              string `json:"name"`
	RemainingFraction any    `json:"remainingFraction"`
	ResetTime         any    `json:"resetTime"`
}

type quotaBucket struct {
	ModelID           string `json:"modelId"`
	Window            string `json:"window"`
	RemainingFraction any    `json:"remainingFraction"`
	ResetTime         any    `json:"resetTime"`
}

type candidateWindow struct {
	resetAt   *time.Time
	remaining int64
	window    WindowType
}

// ParseAvailableModels 将 Antigravity quota summary 或管理端 buckets 响应解析为目标模型组 evidence。
func ParseAvailableModels(raw []byte, observedAt time.Time, group ModelGroup) ProbeResult {
	var response availableModelsResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return failedResult(observedAt, group, "parse antigravity quota failed")
	}
	windows := collectGroupWindows(response, group)
	selected, ok := pickEffectiveWindow(windows)
	if !ok {
		return failedResult(observedAt, group, "target model group quota unavailable")
	}
	result := ProbeResult{
		Provider:    core.ProviderAntigravity,
		ModelGroup:  group,
		ObservedAt:  observedAt.UTC(),
		ResetAt:     selected.resetAt,
		Remaining:   int64Ptr(selected.remaining),
		Window:      selected.window,
		Freshness:   core.FreshnessFresh,
		ProbeStatus: core.ProbeStatusReady,
		Status:      StatusReady,
		PlanType:    inferPlanType(windows),
	}
	if weekly, ok := firstWindow(windows, WindowWeekly); ok {
		result.LongWindowResetAt = weekly.resetAt
	}
	return result
}

func collectGroupWindows(response availableModelsResponse, group ModelGroup) []candidateWindow {
	windows := make([]candidateWindow, 0)
	for modelID, model := range response.Models {
		if !modelBelongsToGroup(modelID, model.ModelProvider, group) {
			continue
		}
		if len(model.QuotaInfo.Windows) == 0 {
			if window, ok := quotaFieldsToWindow(model.QuotaInfo.RemainingFraction, model.QuotaInfo.ResetTime, WindowUnknown); ok {
				windows = append(windows, window)
			}
			continue
		}
		for _, item := range model.QuotaInfo.Windows {
			if window, ok := quotaFieldsToWindow(item.RemainingFraction, item.ResetTime, classifyWindow(item.Name)); ok {
				windows = append(windows, window)
			}
		}
	}
	for _, bucket := range response.Buckets {
		if !modelBelongsToGroup(bucket.ModelID, "", group) {
			continue
		}
		if window, ok := quotaFieldsToWindow(bucket.RemainingFraction, bucket.ResetTime, WindowUnknown); ok {
			windows = append(windows, window)
		}
	}
	for _, quotaGroup := range response.Groups {
		if !quotaGroupBelongsToModelGroup(quotaGroup, group) {
			continue
		}
		for _, bucket := range quotaGroup.Buckets {
			if window, ok := quotaFieldsToWindow(bucket.RemainingFraction, bucket.ResetTime, classifyWindow(bucket.Window)); ok {
				windows = append(windows, window)
			}
		}
	}
	return windows
}

func quotaGroupBelongsToModelGroup(quotaGroup quotaGroup, group ModelGroup) bool {
	text := strings.ToLower(strings.TrimSpace(quotaGroup.DisplayName + " " + quotaGroup.Description))
	if group == ModelGroupClaudeGPT {
		return strings.Contains(text, "claude") || strings.Contains(text, "gpt")
	}
	return strings.Contains(text, "gemini") && !strings.Contains(text, "claude") && !strings.Contains(text, "gpt")
}

func pickEffectiveWindow(windows []candidateWindow) (candidateWindow, bool) {
	fiveHour, hasFiveHour := firstWindow(windows, WindowFiveHour)
	weekly, hasWeekly := firstWindow(windows, WindowWeekly)
	if hasFiveHour && hasWeekly {
		if weekly.remaining <= 0 {
			return weekly, true
		}
		if fiveHour.remaining <= 0 {
			return fiveHour, true
		}
		return fiveHour, true
	}
	if hasWeekly {
		return weekly, true
	}
	if hasFiveHour {
		return fiveHour, true
	}
	for _, window := range windows {
		return window, true
	}
	return candidateWindow{}, false
}

func firstWindow(windows []candidateWindow, windowType WindowType) (candidateWindow, bool) {
	for _, window := range windows {
		if window.window == windowType {
			return window, true
		}
	}
	return candidateWindow{}, false
}

func quotaFieldsToWindow(rawRemaining any, rawReset any, windowType WindowType) (candidateWindow, bool) {
	resetAt, ok := parseAnyTime(rawReset)
	if !ok {
		return candidateWindow{}, false
	}
	remaining, ok := remainingPercent(rawRemaining)
	if !ok {
		return candidateWindow{}, false
	}
	return candidateWindow{resetAt: resetAt, remaining: remaining, window: windowType}, true
}

func modelBelongsToGroup(modelID string, provider string, group ModelGroup) bool {
	text := strings.ToLower(strings.TrimSpace(modelID + " " + provider))
	if group == ModelGroupClaudeGPT {
		return strings.Contains(text, "claude") || strings.Contains(text, "gpt") || strings.Contains(text, "openai")
	}
	return strings.Contains(text, "gemini") && !strings.Contains(text, "claude") && !strings.Contains(text, "gpt")
}

func classifyWindow(name string) WindowType {
	text := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(text, "5") && (strings.Contains(text, "hour") || strings.Contains(text, "hr") || strings.Contains(text, "h")) {
		return WindowFiveHour
	}
	if strings.Contains(text, "week") || strings.Contains(text, "7d") {
		return WindowWeekly
	}
	return WindowUnknown
}

func inferPlanType(windows []candidateWindow) core.PlanType {
	if _, ok := firstWindow(windows, WindowFiveHour); ok {
		return core.PlanTypePro
	}
	if _, ok := firstWindow(windows, WindowWeekly); ok {
		return core.PlanTypeFree
	}
	return core.PlanTypeUnknown
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

func remainingPercent(raw any) (int64, bool) {
	value, ok := toFloat64(raw)
	if !ok {
		return 0, false
	}
	if value <= 1 {
		value *= 100
	}
	if value <= 0 {
		return 0, true
	}
	return int64(math.Ceil(value)), true
}

func toFloat64(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func failedResult(observedAt time.Time, group ModelGroup, message string) ProbeResult {
	return ProbeResult{Provider: core.ProviderAntigravity, ModelGroup: group, ObservedAt: observedAt.UTC(), Window: WindowUnknown, Freshness: core.FreshnessUnknown, ProbeStatus: core.ProbeStatusUnknown, Status: StatusProbeFailed, PlanType: core.PlanTypeUnknown, Error: message}
}

func int64Ptr(value int64) *int64 {
	return &value
}
