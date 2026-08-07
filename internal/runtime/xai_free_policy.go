package runtime

import (
	"context"
	"time"

	"credential-priority/internal/core"
	"credential-priority/internal/provider/xai"
	"credential-priority/internal/state"
)

// xAI free 策略常量（用户拍板）。
const (
	xaiQuotaFailThreshold       = 3
	xaiQuotaFailWindowThreshold = 2
	xaiQuotaFailWindow          = 30 * time.Minute
	xaiFreeCooldown             = 24 * time.Hour
	xaiAuthInvalidReason        = "xai auth invalid"
)

// xaiPolicySnapshot 是 store 中 xAI free 策略的可读快照。
type xaiPolicySnapshot struct {
	PlanClass         string
	QuotaFailCount    int
	FirstSuccessAt    time.Time
	NextEligibleAt    time.Time
	XAIDepletedKind   string
	AuthInvalid       bool
	Remaining         int
	ResetAt           time.Time
	ObservedAt        time.Time
	QuotaFailTimes    []time.Time
	LongWindowResetAt time.Time
}

func loadXAIPolicy(store *state.Store, authIndex string) xaiPolicySnapshot {
	entry, ok := store.GetEntry(authIndex, "")
	if !ok {
		return xaiPolicySnapshot{}
	}
	return xaiPolicySnapshot{
		PlanClass:         entry.PlanClass,
		QuotaFailCount:    entry.QuotaFailCount,
		FirstSuccessAt:    entry.FirstSuccessAt,
		NextEligibleAt:    entry.NextEligibleAt,
		XAIDepletedKind:   entry.XAIDepletedKind,
		AuthInvalid:       entry.AuthInvalid,
		Remaining:         entry.Remaining,
		ResetAt:           entry.ResetAt,
		ObservedAt:        entry.ObservedAt,
		QuotaFailTimes:    entry.QuotaFailTimes,
		LongWindowResetAt: entry.LongWindowResetAt,
	}
}

// applyXAIQuotaFailure 处理额度类失败（429 free-exhausted 等）。
// 30分钟窗口内累计 2 次 429 即判 free 额度耗尽（触发 depleted，priority=-1）。
func applyXAIQuotaFailure(prev xaiPolicySnapshot, now time.Time) xaiPolicySnapshot {
	next := prev
	next.ObservedAt = now.UTC()
	if next.PlanClass == "" {
		next.PlanClass = string(xai.PlanClassFree)
	}

	// 1. 清理超过 30 分钟的旧时间戳
	cutoff := now.Add(-xaiQuotaFailWindow)
	var activeTimes []time.Time
	for _, t := range prev.QuotaFailTimes {
		if t.After(cutoff) {
			activeTimes = append(activeTimes, t)
		}
	}

	// 2. 加入当前失败时间戳
	activeTimes = append(activeTimes, now.UTC())
	next.QuotaFailTimes = activeTimes
	next.QuotaFailCount = len(activeTimes)

	// 3. 累计达到阈值则触发 depleted
	if len(activeTimes) >= xaiQuotaFailWindowThreshold {
		next.XAIDepletedKind = string(xai.DepletedFree)
		next.Remaining = 0
		anchor := next.FirstSuccessAt
		if anchor.IsZero() {
			anchor = now.UTC()
		}
		next.NextEligibleAt = anchor.Add(xaiFreeCooldown)
		next.ResetAt = next.NextEligibleAt
	}
	return next
}

// applyXAISuccess 处理成功 usage：清零连续失败；若已过 next_eligible 且 free → 恢复高优信号。
func applyXAISuccess(prev xaiPolicySnapshot, now time.Time) xaiPolicySnapshot {
	next := prev
	next.QuotaFailCount = 0
	next.QuotaFailTimes = nil // 成功调用立即清零
	next.XAIDepletedKind = ""
	next.Remaining = 1
	next.ObservedAt = now.UTC()
	if next.FirstSuccessAt.IsZero() {
		next.FirstSuccessAt = now.UTC()
	}
	// 成功且冷却已过（或无冷却）→ 清除 next_eligible，恢复可高优
	if next.NextEligibleAt.IsZero() || !now.Before(next.NextEligibleAt) {
		next.NextEligibleAt = time.Time{}
		next.ResetAt = now.UTC().Add(xaiFreeCooldown)
	} else {
		// 仍在冷却窗内但业务成功：清失败计数，保留 next_eligible 供 planner 判断
		next.ResetAt = next.NextEligibleAt
	}
	return next
}

// xaiFreeEligible 判断 free 凭证当前是否可参与高优排序。
func xaiFreeEligible(snap xaiPolicySnapshot, now time.Time) bool {
	if snap.XAIDepletedKind == string(xai.DepletedFree) || snap.QuotaFailCount >= xaiQuotaFailThreshold {
		if !snap.NextEligibleAt.IsZero() && now.Before(snap.NextEligibleAt) {
			return false
		}
	}
	if !snap.NextEligibleAt.IsZero() && now.Before(snap.NextEligibleAt) && snap.Remaining <= 0 {
		return false
	}
	return true
}

// xaiInFreeCooldown 判断是否处于 free 冷却（priority 应 -1）。
func xaiInFreeCooldown(snap xaiPolicySnapshot, now time.Time) bool {
	if snap.QuotaFailCount >= xaiQuotaFailThreshold {
		if snap.NextEligibleAt.IsZero() {
			return true
		}
		return now.Before(snap.NextEligibleAt)
	}
	if snap.XAIDepletedKind == string(xai.DepletedFree) {
		if snap.NextEligibleAt.IsZero() {
			return snap.Remaining <= 0
		}
		return now.Before(snap.NextEligibleAt)
	}
	return false
}

func writeXAIPolicy(ctx context.Context, store *state.Store, authIndex string, snap xaiPolicySnapshot, source state.Source, nextProbeAt time.Time) error {
	return writeXAIPolicyWithLongWindow(ctx, store, authIndex, snap, source, nextProbeAt, false)
}

// writeXAIPolicyWithLongWindow 写入策略；preserveLongWindow=true 时 usage 路径保留旧周长窗。
func writeXAIPolicyWithLongWindow(ctx context.Context, store *state.Store, authIndex string, snap xaiPolicySnapshot, source state.Source, nextProbeAt time.Time, preserveLongWindow bool) error {
	return store.MarkProbeSuccess(ctx, state.ProbeSuccess{
		AuthIndex:          authIndex,
		Provider:           core.ProviderXAI,
		ObservedAt:         snap.ObservedAt,
		ResetAt:            snap.ResetAt,
		Remaining:          snap.Remaining,
		Source:             source,
		NextProbeAt:        nextProbeAt,
		AuthInvalid:        false,
		PlanClass:          snap.PlanClass,
		QuotaFailCount:     snap.QuotaFailCount,
		FirstSuccessAt:     snap.FirstSuccessAt,
		NextEligibleAt:     snap.NextEligibleAt,
		XAIDepletedKind:    snap.XAIDepletedKind,
		QuotaFailTimes:     snap.QuotaFailTimes,
		LongWindowResetAt:  snap.LongWindowResetAt,
		PreserveLongWindow: preserveLongWindow,
	})
}

func writeXAIAuthInvalid(ctx context.Context, store *state.Store, authIndex string, observedAt time.Time) error {
	return store.UpsertXAIPolicy(ctx, authIndex, func(entry *state.Entry) {
		entry.ObservedAt = observedAt.UTC()
		entry.ResetAt = time.Time{}
		entry.Remaining = 0
		entry.Source = state.SourceFreshProbe
		entry.LastError = xaiAuthInvalidReason
		entry.NextProbeAt = time.Time{}
		entry.AuthInvalid = true
		entry.PlanClass = ""
		entry.NextEligibleAt = time.Time{}
		entry.XAIDepletedKind = ""
		entry.LongWindowResetAt = time.Time{}
	})
}
