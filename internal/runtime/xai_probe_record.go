package runtime

import (
	"context"
	"errors"
	"time"

	"credential-priority/internal/core"
	"credential-priority/internal/priority"
	"credential-priority/internal/provider/xai"
	"credential-priority/internal/state"
)

// xAI plan re-fetch interval; free cooldown aligns NextEligibleAt.
const (
	xaiPositiveProbeInterval = 24 * time.Hour
	xaiFailureProbeBackoff   = time.Hour
)

// recordXAIPlanResult merges FetchPlan classification with usage free-policy store fields.
// No chat multi-model probe. Quota remaining comes from usage.handle path.
func recordXAIPlanResult(ctx context.Context, store *state.Store, plan xai.PlanResult, now time.Time) (priority.ProbeEvidence, error) {
	if planLooksUnauthorized(plan) {
		observedAt := plan.ObservedAt.UTC()
		if observedAt.IsZero() {
			observedAt = now.UTC()
		}
		if err := writeXAIAuthInvalid(ctx, store, plan.AuthIndex, observedAt); err != nil {
			return priority.ProbeEvidence{}, err
		}
		return priority.ProbeEvidence{
			Provider:      core.ProviderXAI,
			AuthIndex:     plan.AuthIndex,
			ObservedAt:    observedAt,
			Freshness:     core.FreshnessFresh,
			ProbeStatus:   core.ProbeStatusReady,
			Status:        priority.EvidenceStatusAuthInvalid,
			EvidenceFresh: true,
			QuotaKnown:    false,
		}, nil
	}
	prev := loadXAIPolicy(store, plan.AuthIndex)
	planClass := string(plan.PlanClass)
	if planClass == "" {
		planClass = string(xai.PlanClassFree)
	}
	// Preserve usage-driven fail_count / next_eligible / first_success.
	snap := prev
	snap.PlanClass = planClass
	snap.ObservedAt = plan.ObservedAt.UTC()
	if snap.ObservedAt.IsZero() {
		snap.ObservedAt = now.UTC()
	}

	inCooldown := xaiInFreeCooldown(snap, now)
	remaining := 1
	var depletedKind string
	var resetAt time.Time
	var nextEligible *time.Time
	if inCooldown {
		remaining = 0
		depletedKind = string(xai.DepletedFree)
		resetAt = snap.NextEligibleAt
		if resetAt.IsZero() {
			resetAt = now.Add(xaiFreeCooldown)
		}
		t := resetAt
		nextEligible = &t
	} else {
		if snap.Remaining > 0 {
			remaining = snap.Remaining
		}
		if !snap.NextEligibleAt.IsZero() {
			t := snap.NextEligibleAt
			nextEligible = &t
		}
		if !snap.ResetAt.IsZero() {
			resetAt = snap.ResetAt
		} else {
			resetAt = now.Add(xaiPositiveProbeInterval)
		}
		depletedKind = snap.XAIDepletedKind
		if depletedKind == string(xai.DepletedFree) && !inCooldown {
			depletedKind = ""
		}
	}
	snap.Remaining = remaining
	snap.XAIDepletedKind = depletedKind
	snap.ResetAt = resetAt
	nextProbeAt := now.Add(xaiPositiveProbeInterval)
	if inCooldown && !resetAt.IsZero() {
		nextProbeAt = resetAt
	}
	if err := writeXAIPolicy(ctx, store, plan.AuthIndex, snap, state.SourceFreshProbe, nextProbeAt); err != nil {
		return priority.ProbeEvidence{}, err
	}
	rem := int64(remaining)
	planType := plan.PlanType
	if planType == core.PlanTypeUnknown {
		planType = xaiPlanTypeFromClass(planClass)
	}
	return priority.ProbeEvidence{
		Provider:          core.ProviderXAI,
		AuthIndex:         plan.AuthIndex,
		ObservedAt:        snap.ObservedAt,
		ResetAt:           &resetAt,
		Remaining:         &rem,
		Freshness:         core.FreshnessFresh,
		ProbeStatus:       core.ProbeStatusReady,
		Status:            priority.EvidenceStatusReady,
		PlanType:          planType,
		EvidenceFresh:     true,
		XAIDepletedKind:   depletedKind,
		QuotaKnown:        true,
		XAIPlanClass:      planClass,
		XAINextEligibleAt: nextEligible,
		XAIQuotaFailCount: snap.QuotaFailCount,
	}, nil
}

// recordXAIProbeResult keeps legacy chat-probe result path for tests / force tooling.
// Production auto path uses recordXAIPlanResult only.
func recordXAIProbeResult(ctx context.Context, store *state.Store, result xai.ProbeResult, now time.Time) (priority.ProbeEvidence, error) {
	if result.Status != xai.StatusReady || !result.QuotaKnown || result.ResetAt == nil || result.Remaining == nil {
		err := store.MarkProbeFailure(ctx, state.ProbeFailure{
			AuthIndex:   result.AuthIndex,
			Provider:    core.ProviderXAI,
			ObservedAt:  now,
			Err:         errors.New(result.Error),
			NextProbeAt: now.Add(xaiFailureProbeBackoff),
		})
		return priority.ProbeEvidence{Provider: core.ProviderXAI, AuthIndex: result.AuthIndex, Freshness: result.Freshness, ProbeStatus: result.ProbeStatus, Status: priority.EvidenceStatusProbeFailed, QuotaKnown: false}, err
	}
	prev := loadXAIPolicy(store, result.AuthIndex)
	nextProbeAt := xaiNextProbeAt(result)
	planClass := prev.PlanClass
	if planClass == "" {
		if result.PlanType == core.PlanTypeFree {
			planClass = string(xai.PlanClassFree)
		} else if result.PlanType == core.PlanTypePlus || result.PlanType == core.PlanTypePro {
			planClass = string(xai.PlanClassPaid)
		}
	}
	depleted := string(result.DepletedKind)
	var nextEligible *time.Time
	failCount := prev.QuotaFailCount
	firstSuccess := prev.FirstSuccessAt
	if isXAIFreeDepletedResult(result) {
		failCount = xaiQuotaFailThreshold
		depleted = string(xai.DepletedFree)
		eligible := nextEligibleFromFirstSuccess(firstSuccess, now)
		if result.ResetAt != nil {
			// Prefer explicit reset when closer to 24h policy.
			eligible = result.ResetAt.UTC()
		}
		nextEligible = &eligible
	} else if result.Remaining != nil && *result.Remaining > 0 {
		failCount = 0
		if firstSuccess.IsZero() {
			firstSuccess = result.ObservedAt.UTC()
		}
		depleted = ""
	}
	err := store.MarkProbeSuccess(ctx, state.ProbeSuccess{
		AuthIndex:       result.AuthIndex,
		Provider:        core.ProviderXAI,
		ObservedAt:      result.ObservedAt,
		ResetAt:         *result.ResetAt,
		Remaining:       int(*result.Remaining),
		Source:          state.SourceFreshProbe,
		NextProbeAt:     nextProbeAt,
		PlanClass:       planClass,
		QuotaFailCount:  failCount,
		FirstSuccessAt:  firstSuccess,
		NextEligibleAt:  timeOrZero(nextEligible),
		XAIDepletedKind: depleted,
	})
	return priority.ProbeEvidence{
		Provider:          core.ProviderXAI,
		AuthIndex:         result.AuthIndex,
		ObservedAt:        result.ObservedAt,
		ResetAt:           result.ResetAt,
		Remaining:         result.Remaining,
		LongWindowResetAt: result.LongWindowResetAt,
		Freshness:         result.Freshness,
		ProbeStatus:       result.ProbeStatus,
		Status:            priority.EvidenceStatusReady,
		PlanType:          result.PlanType,
		EvidenceFresh:     true,
		XAIDepletedKind:   depleted,
		QuotaKnown:        true,
		XAIPlanClass:      planClass,
		XAINextEligibleAt: nextEligible,
		XAIQuotaFailCount: failCount,
	}, err
}

func xaiNextProbeAt(result xai.ProbeResult) time.Time {
	observed := result.ObservedAt.UTC()
	if result.ResetAt == nil {
		return observed.Add(xaiPositiveProbeInterval)
	}
	resetAt := result.ResetAt.UTC()
	if isXAIFreeDepletedResult(result) {
		capAt := observed.Add(xaiPositiveProbeInterval)
		if resetAt.Before(capAt) {
			return resetAt
		}
		if resetAt.After(observed.Add(48 * time.Hour)) {
			return capAt
		}
		return resetAt
	}
	return observed.Add(xaiPositiveProbeInterval)
}

func isXAIFreeDepletedResult(result xai.ProbeResult) bool {
	if result.DepletedKind == xai.DepletedFree {
		return true
	}
	if result.Remaining != nil && *result.Remaining <= 0 {
		if result.PlanType == core.PlanTypeFree || result.Window == xai.WindowFree24h {
			return true
		}
	}
	return false
}

func xaiPlanTypeFromClass(class string) core.PlanType {
	switch class {
	case string(xai.PlanClassFree):
		return core.PlanTypeFree
	case string(xai.PlanClassPaid):
		return core.PlanTypePlus
	default:
		return core.PlanTypeUnknown
	}
}

func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}
