package codex

import (
	"strings"
	"time"

	"credential-priority/internal/core"
)

type effectiveWindow struct {
	resetAt           *time.Time
	remaining         int64
	windowType        WindowType
	longWindowResetAt *time.Time
}

type parsedWindow struct {
	resetAt   *time.Time
	remaining int64
}

func pickEffectiveWindow(usage whamUsage, observedAt time.Time) (effectiveWindow, bool) {
	planType := inferPlanType(usage.PlanType)
	if planType == core.PlanTypeFree {
		return pickFreeWindow(usage, observedAt)
	}
	if paidPlan(planType) {
		return pickPaidWindow(usage, observedAt)
	}
	return effectiveWindow{}, false
}

func pickPaidWindow(usage whamUsage, observedAt time.Time) (effectiveWindow, bool) {
	fiveHour, hasFiveHour := pickWindow(usage, observedAt, isFiveHourWindow)
	weekly, hasWeekly := pickWindow(usage, observedAt, isWeeklyWindow)
	if !hasFiveHour || !hasWeekly {
		return effectiveWindow{}, false
	}
	if weekly.remaining <= 0 {
		return effectiveWindow{resetAt: weekly.resetAt, remaining: 0, windowType: WindowWeekly}, true
	}
	if fiveHour.remaining <= 0 {
		return effectiveWindow{resetAt: fiveHour.resetAt, remaining: 0, windowType: WindowFiveHour, longWindowResetAt: weekly.resetAt}, true
	}
	return effectiveWindow{resetAt: fiveHour.resetAt, remaining: fiveHour.remaining, windowType: WindowFiveHour, longWindowResetAt: weekly.resetAt}, true
}

func pickFreeWindow(usage whamUsage, observedAt time.Time) (effectiveWindow, bool) {
	monthly, ok := pickWindow(usage, observedAt, isFreeLongWindow)
	if !ok {
		return effectiveWindow{}, false
	}
	return effectiveWindow{resetAt: monthly.resetAt, remaining: monthly.remaining, windowType: WindowMonthly}, true
}

func pickWindow(usage whamUsage, observedAt time.Time, match func(whamWindow) bool) (parsedWindow, bool) {
	windows := []whamWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow}
	for _, window := range windows {
		if !hasWindowData(window) || !match(window) {
			continue
		}
		resetAt := windowResetTime(observedAt, window)
		remaining, ok := windowRemaining(window)
		if resetAt == nil || !ok {
			return parsedWindow{}, false
		}
		return parsedWindow{resetAt: resetAt, remaining: remaining}, true
	}
	return parsedWindow{}, false
}

func isFiveHourWindow(window whamWindow) bool {
	if seconds, ok := toInt64(window.LimitWindowSeconds); ok && seconds == 5*60*60 {
		return true
	}
	for _, field := range windowMetadataStrings(window) {
		if strings.Contains(field, "5h") || strings.Contains(field, "5 h") || strings.Contains(field, "5 hour") || strings.Contains(field, "5 hr") {
			return true
		}
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

func isMonthlyWindow(window whamWindow) bool {
	if seconds, ok := toInt64(window.LimitWindowSeconds); ok && seconds >= 28*24*60*60 && seconds <= 31*24*60*60 {
		return true
	}
	for _, field := range windowMetadataStrings(window) {
		if strings.Contains(field, "monthly") || strings.Contains(field, "month") || strings.Contains(field, "30d") || strings.Contains(field, "30 days") || strings.Contains(field, "30 day") {
			return true
		}
	}
	return false
}

func isFreeLongWindow(window whamWindow) bool {
	if isFiveHourWindow(window) {
		return false
	}
	if seconds, ok := toInt64(window.LimitWindowSeconds); ok && seconds >= 24*60*60 {
		return true
	}
	return isMonthlyWindow(window) || isWeeklyWindow(window)
}
