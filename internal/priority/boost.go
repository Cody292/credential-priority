package priority

func plannedPriority(item PlanItem, basePriority int, options Options) int {
	if resetBoost(item, options) > 0 {
		return 999
	}
	return basePriority
}

func resetBoost(item PlanItem, options Options) int {
	resetAt := item.LongWindowResetAt
	if options.ResetBoostWithin <= 0 || options.ResetBoost <= 0 || resetAt == nil || paidRank(item.PlanType) == 0 {
		return 0
	}
	if resetAt.After(options.Now) && resetAt.Sub(options.Now) < options.ResetBoostWithin {
		return options.ResetBoost
	}
	return 0
}
