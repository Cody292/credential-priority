package runtime

import (
	"context"
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
