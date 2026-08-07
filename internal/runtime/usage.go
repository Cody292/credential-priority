package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"credential-priority/internal/config"
	"credential-priority/internal/core"
	"credential-priority/internal/provider/xai"
	"credential-priority/internal/state"
)

// usageRecord is the boundary parse of host usage.handle UsageRecord.
// Field names accept snake_case / camelCase / host variants; unknown fields ignored.
type usageRecord struct {
	AuthIndex  string
	Provider   string
	Model      string
	StatusCode int
	Success    *bool
	Error      string
	ErrorCode  string
	RawBody    string
}

// HandleUsage handles usage.handle: business-side signal updates xAI store evidence.
// Non-xAI or unparseable payloads succeed silently to avoid host retry storms.
func (r *Runtime) HandleUsage(ctx context.Context, raw []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("usage handle context: %w", err)
	}
	rec, ok := parseUsageRecord(raw)
	if !ok {
		return nil
	}
	if !isXAIUsageProvider(rec.Provider) {
		return nil
	}
	authIndex := strings.TrimSpace(rec.AuthIndex)
	if authIndex == "" {
		return nil
	}

	r.mu.Lock()
	if r.shutdown {
		r.mu.Unlock()
		return ErrShutdown
	}
	now := r.clock.Now().UTC()
	r.mu.Unlock()

	store, err := state.Load(ctx, config.DefaultStateCachePath)
	if err != nil {
		return fmt.Errorf("usage load store: %w", err)
	}
	applied, err := applyXAIUsageToStore(ctx, store, rec, now)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}
	if err := store.SaveAtomic(ctx); err != nil {
		return fmt.Errorf("usage save store: %w", err)
	}
	return nil
}

func applyXAIUsageToStore(ctx context.Context, store *state.Store, rec usageRecord, now time.Time) (bool, error) {
	decision := classifyXAIUsage(rec, now)
	authIndex := strings.TrimSpace(rec.AuthIndex)
	prev := loadXAIPolicy(store, authIndex)

	switch decision.kind {
	case usageDecisionIgnore:
		return false, nil
	case usageDecisionAuthInvalid:
		if err := writeXAIAuthInvalid(ctx, store, authIndex, now); err != nil {
			return false, err
		}
		return true, nil
	case usageDecisionSuccess:
		next := applyXAISuccess(prev, now)
		if next.PlanClass == "" {
			next.PlanClass = string(xai.PlanClassFree)
		}
		nextProbe := now.Add(xaiPositiveProbeInterval)
		// usage 不触碰周账单：保留既有 LongWindowResetAt。
		if err := writeXAIPolicyWithLongWindow(ctx, store, authIndex, next, state.SourceFreshProbe, nextProbe, true); err != nil {
			return false, err
		}
		return true, nil
	case usageDecisionFreeDepleted:
		next := applyXAIQuotaFailure(prev, now)
		// Below threshold: only accumulate fail_count, keep remaining if positive.
		if next.QuotaFailCount < xaiQuotaFailWindowThreshold {
			if prev.Remaining > 0 {
				next.Remaining = prev.Remaining
			}
			if next.ResetAt.IsZero() {
				next.ResetAt = now.Add(xaiFreeCooldown)
			}
			nextProbe := now.Add(xaiFailureProbeBackoff)
			if err := writeXAIPolicyWithLongWindow(ctx, store, authIndex, next, state.SourceFreshProbe, nextProbe, true); err != nil {
				return false, err
			}
			return true, nil
		}
		// 3 consecutive quota failures: soft-disable evidence (planner maps priority=-1).
		nextProbe := next.NextEligibleAt
		if nextProbe.IsZero() {
			nextProbe = now.Add(xaiFreeCooldown)
		}
		if err := writeXAIPolicyWithLongWindow(ctx, store, authIndex, next, state.SourceFreshProbe, nextProbe, true); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

type usageDecisionKind int

const (
	usageDecisionIgnore usageDecisionKind = iota
	usageDecisionAuthInvalid
	usageDecisionSuccess
	usageDecisionFreeDepleted
)

type usageDecision struct {
	kind      usageDecisionKind
	remaining *int64
	resetAt   *time.Time
	planType  core.PlanType
}

func classifyXAIUsage(rec usageRecord, now time.Time) usageDecision {
	status := rec.StatusCode
	code := rec.ErrorCode
	message := rec.Error
	raw := rec.RawBody
	if raw == "" {
		raw = message
	}
	blob := strings.ToLower(strings.TrimSpace(code + " " + message + " " + raw))

	// 401 / 凭证失效文案：AuthInvalid 硬路径；永不计入 free 额度失败。
	if status == 401 || xai.IsUnauthorizedProbe(status, blob) {
		return usageDecision{kind: usageDecisionAuthInvalid}
	}

	// 5xx / pure network: never count as consecutive quota failure.
	if status >= 500 {
		return usageDecision{kind: usageDecisionIgnore}
	}

	// 任何 429 均计入（不再像之前那样过滤额度关键字）
	if status == 429 {
		resetAt := now.Add(24 * time.Hour)
		return usageDecision{kind: usageDecisionFreeDepleted, resetAt: &resetAt}
	}

	// Success: positive remaining + clear fail_count.
	if isUsageSuccess(rec) {
		remaining := int64(1)
		resetAt := now.Add(24 * time.Hour)
		return usageDecision{
			kind:      usageDecisionSuccess,
			remaining: &remaining,
			resetAt:   &resetAt,
			planType:  core.PlanTypeUnknown,
		}
	}

	return usageDecision{kind: usageDecisionIgnore}
}

func isUsageSuccess(rec usageRecord) bool {
	if rec.Success != nil {
		return *rec.Success
	}
	if rec.StatusCode == 0 {
		return strings.TrimSpace(rec.Error) == "" && strings.TrimSpace(rec.ErrorCode) == ""
	}
	return rec.StatusCode >= 200 && rec.StatusCode < 300
}

func isUsageQuotaExhausted(code, message, raw string, status int) bool {
	if isUsageFreeExhausted(code, message, raw) {
		return true
	}
	blob := strings.ToLower(code + " " + message + " " + raw)
	if status == 429 && strings.Contains(blob, "tokens") && strings.Contains(blob, "actual") && strings.Contains(blob, "limit") {
		return true
	}
	return false
}

func isUsageFreeExhausted(code, message, raw string) bool {
	blob := strings.ToLower(code + " " + message + " " + raw)
	return strings.Contains(blob, "free-usage-exhausted") ||
		strings.Contains(blob, "subscription:free-usage-exhausted") ||
		strings.Contains(blob, "included free usage") ||
		(strings.Contains(blob, "free usage") && strings.Contains(blob, "used all")) ||
		(strings.Contains(blob, "rolling 24-hour") && strings.Contains(blob, "tokens"))
}

func isXAIUsageProvider(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	return p == string(core.ProviderXAI) || p == "grok" || p == "x-ai" || p == "x_ai"
}
