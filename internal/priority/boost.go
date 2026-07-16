package priority

import "credential-priority/internal/core"

func plannedPriority(item PlanItem, basePriority int, options Options) int {
	if resetBoost(item, options) > 0 {
		return 999
	}
	return basePriority
}

func resetBoost(item PlanItem, options Options) int {
	resetAt := item.LongWindowResetAt
	if options.ResetBoostWithin <= 0 || options.ResetBoost <= 0 || resetAt == nil {
		return 0
	}
	// paid plan：沿用原 near long-window reset 提权。
	// Free/Unknown：仅 Antigravity/Gemini 周额度 near-reset 可提权，避免 Codex Free 误升 999。
	if paidRank(item.PlanType) == 0 && planItemProvider(item) != core.ProviderAntigravity {
		return 0
	}
	if resetAt.After(options.Now) && resetAt.Sub(options.Now) < options.ResetBoostWithin {
		return options.ResetBoost
	}
	return 0
}
