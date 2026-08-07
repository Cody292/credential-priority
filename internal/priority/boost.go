package priority

import (
	"credential-priority/internal/core"
	"time"
)

func plannedPriority(item PlanItem, basePriority int, options Options) int {
	if resetBoost(item, options) > 0 {
		return 999
	}
	return basePriority
}

func resetBoost(item PlanItem, options Options) int {
	if options.ResetBoostWithin <= 0 || options.ResetBoost <= 0 {
		return 0
	}

	provider := planItemProvider(item)
	var resetAt *time.Time

	// Antigravity / Codex / xAI：999 提权仅看 LongWindowResetAt；
	// ResetAt 为短窗/free 冷却，不得单独制造 999。
	if provider == core.ProviderAntigravity || provider == core.ProviderCodex || provider == core.ProviderXAI {
		resetAt = item.LongWindowResetAt
	} else {
		resetAt = item.ResetAt
	}

	if resetAt == nil {
		return 0
	}
	// paid：三提供商 effective ResetAt near-reset 均可提权。
	// Free/Unknown：仅 Antigravity、Codex；禁止 xAI Free（及 xAI free 计划）。
	if paidRank(item.PlanType) == 0 {
		provider := planItemProvider(item)
		if provider == core.ProviderXAI || isXAIFreePlanItem(item) {
			return 0
		}
		if provider != core.ProviderAntigravity && provider != core.ProviderCodex {
			return 0
		}
	}
	if resetAt.After(options.Now) && resetAt.Sub(options.Now) < options.ResetBoostWithin {
		return options.ResetBoost
	}
	return 0
}
