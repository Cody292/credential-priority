package priority

import (
	"cmp"
	"slices"
	"time"

	"credential-priority/internal/core"
)

// Options 是 fresh-only 优先级规划器的已解析策略参数。
type Options struct {
	Now                           time.Time
	MaxPriority                   int
	StartPriorityByProvider       map[core.Provider]int
	CodexFreeDepletedPriority     *int
	CodexFreeDepletedDisabled     *bool
	CodexPaidDepletedKeepsEnabled *bool
	XAIFreeDepletedPriority              *int
	XAIFreeDepletedDisabled              *bool
	XAIWeeklyDepletedPriority            *int
	XAIMonthlyAndWeeklyDepletedPriority  *int
	XAIMonthlyAndWeeklyDepletedDisabled  *bool
	MinChange                     int
	PaidFirst                     bool
	ResetBoostWithin              time.Duration
	ResetBoost                    int
}

// ProbeEvidence 是本轮 probe 产出的排序证据；EvidenceFresh=false 时不得驱动变更。
type ProbeEvidence struct {
	Provider          core.Provider
	AuthIndex         string
	ObservedAt        time.Time
	ResetAt           *time.Time
	Remaining         *int64
	LongWindowResetAt *time.Time
	Freshness         core.Freshness
	ProbeStatus       core.ProbeStatus
	Status            EvidenceStatus
	PlanType          core.PlanType
	EvidenceFresh     bool
	// XAIDepletedKind: free | weekly | monthly_and_weekly；空表示非 xAI 耗尽语义。
	XAIDepletedKind string
	// QuotaKnown 仅 xAI：false 时禁止驱动 priority/disabled 变更。
	QuotaKnown bool
}

// EvidenceStatus 标识本轮 probe evidence 对规划器是否可用。
type EvidenceStatus string

const (
	// EvidenceStatusUnknown 表示没有可用于规划的 probe 结论。
	EvidenceStatusUnknown EvidenceStatus = "unknown"
	// EvidenceStatusReady 表示 evidence 可用于 fresh-only 规划。
	EvidenceStatusReady EvidenceStatus = "ready"
	// EvidenceStatusProbeFailed 表示本轮 probe 失败，必须保持现状。
	EvidenceStatusProbeFailed EvidenceStatus = "probe_failed"
	// EvidenceStatusUnsupported 表示 provider 不支持自动规划。
	EvidenceStatusUnsupported EvidenceStatus = "unsupported"
	// EvidenceStatusUnavailable 表示凭证当前不可用，必须保持现状。
	EvidenceStatusUnavailable EvidenceStatus = "unavailable"
)

// PlanItem 表示单个凭证在本轮规划后的目标状态。
type PlanItem struct {
	Credential        core.Credential
	Priority          int
	Disabled          bool
	PlanType          core.PlanType
	ResetAt           *time.Time
	Remaining         *int64
	LongWindowResetAt *time.Time
	EvidenceFresh     bool
	// ForceWrite 允许无本轮 fresh 证据的同伴因同 provider 优先级去重而写回宿主。
	ForceWrite bool
	Reason     string
}

// Change 表示需要由后续 apply writer 写回宿主的 fresh 证据变更。
type Change struct {
	Credential    core.Credential
	Priority      int
	Disabled      bool
	EvidenceFresh bool
	Reason        string
}

// Plan 是 fresh-only 优先级规划结果。
type Plan struct {
	Items   []PlanItem
	Changes []Change
}

// PlanFreshOnly 只使用本轮 fresh probe evidence 生成优先级和禁用变更。
func PlanFreshOnly(credentials []core.Credential, evidence []ProbeEvidence, options Options) Plan {
	evidenceByAuthIndex := freshEvidenceByAuthIndex(evidence)
	items := initialItems(credentials, evidenceByAuthIndex, options)
	planFreshPositive(items, options)
	// 局部探测只会给少数凭证 fresh 证据；若只写 start_priority，会与同伴历史优先级重叠。
	// 同 provider 启用态正优先级必须在本轮规划结果中唯一，必要时改写无 fresh 同伴并 ForceWrite。
	ensureUniqueProviderPriorities(items, options)
	sortPlanItems(items)
	return Plan{Items: items, Changes: changes(items, options)}
}

func freshEvidenceByAuthIndex(evidence []ProbeEvidence) map[string]ProbeEvidence {
	byAuthIndex := make(map[string]ProbeEvidence, len(evidence))
	for _, item := range evidence {
		if isFreshReadyEvidence(item) {
			byAuthIndex[item.AuthIndex] = item
		}
	}
	return byAuthIndex
}

func isFreshReadyEvidence(evidence ProbeEvidence) bool {
	return evidence.EvidenceFresh &&
		evidence.Freshness == core.FreshnessFresh &&
		evidence.ProbeStatus == core.ProbeStatusReady &&
		evidence.Status == EvidenceStatusReady
}

func initialItems(credentials []core.Credential, evidenceByAuthIndex map[string]ProbeEvidence, options Options) []PlanItem {
	items := make([]PlanItem, len(credentials))
	for index, credential := range credentials {
		item := PlanItem{
			Credential: credential,
			Priority:   credential.Priority,
			Disabled:   credential.Disabled,
			PlanType:   credential.PlanType,
			Reason:     "keep current state",
		}
		evidence, ok := evidenceByAuthIndex[credential.AuthIndex]
		if ok {
			if isCodexFreeDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = codexFreeDepletedPriority(options)
				item.Disabled = credential.Disabled || codexFreeDepletedDisabled(options)
				item.Reason = "fresh remaining depleted"
			} else if isCodexPaidDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = codexFreeDepletedPriority(options)
				item.Disabled = credential.Disabled || !codexPaidDepletedKeepsEnabled(options)
				item.Reason = "fresh paid remaining depleted"
			} else if isXAIFreeDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = xaiFreeDepletedPriority(options)
				item.Disabled = credential.Disabled || xaiFreeDepletedDisabled(options)
				item.Reason = "fresh remaining depleted"
			} else if isXAIMonthlyAndWeeklyDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = xaiMonthlyAndWeeklyDepletedPriority(options)
				item.Disabled = credential.Disabled || xaiMonthlyAndWeeklyDepletedDisabled(options)
				item.Reason = "fresh monthly and weekly depleted"
			} else if isXAIWeeklyDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = xaiWeeklyDepletedPriority(options)
				// 仅周限额耗尽：降优先级，不禁用。
				item.Disabled = credential.Disabled
				item.Reason = "fresh weekly depleted"
			} else if isAntigravityWeeklyDepleted(credential, evidence) {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
				item.Priority = -1
				item.Disabled = true
				item.Reason = "fresh remaining depleted"
			} else if evidence.Remaining != nil && evidence.ResetAt != nil {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.LongWindowResetAt = evidence.LongWindowResetAt
				item.EvidenceFresh = true
			}
		}
		items[index] = item
	}
	return items
}

func isCodexFreeDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return planItemProvider(PlanItem{Credential: credential}) == core.ProviderCodex &&
		evidence.PlanType == core.PlanTypeFree &&
		evidence.Remaining != nil &&
		*evidence.Remaining <= 0
}

func isCodexPaidDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return planItemProvider(PlanItem{Credential: credential}) == core.ProviderCodex &&
		paidRank(evidence.PlanType) > 0 &&
		evidence.Remaining != nil &&
		*evidence.Remaining <= 0
}

func isAntigravityWeeklyDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return planItemProvider(PlanItem{Credential: credential}) == core.ProviderAntigravity &&
		evidence.Remaining != nil &&
		*evidence.Remaining <= 0
}

func isAntigravityWeeklyDepletedItem(item PlanItem) bool {
	return planItemProvider(item) == core.ProviderAntigravity &&
		isFreeOrUnknownPlan(item.PlanType) &&
		item.Remaining != nil &&
		*item.Remaining <= 0 &&
		item.LongWindowResetAt != nil
}

func isFreeOrUnknownPlan(planType core.PlanType) bool {
	return planType == core.PlanTypeFree || planType == core.PlanTypeUnknown
}

func codexFreeDepletedPriority(options Options) int {
	if options.CodexFreeDepletedPriority == nil {
		return -1
	}
	return *options.CodexFreeDepletedPriority
}

func codexFreeDepletedDisabled(options Options) bool {
	if options.CodexFreeDepletedDisabled == nil {
		return true
	}
	return *options.CodexFreeDepletedDisabled
}

func codexPaidDepletedKeepsEnabled(options Options) bool {
	if options.CodexPaidDepletedKeepsEnabled == nil {
		return true
	}
	return *options.CodexPaidDepletedKeepsEnabled
}

func isXAICredential(credential core.Credential) bool {
	return planItemProvider(PlanItem{Credential: credential}) == core.ProviderXAI
}

func isXAIFreeDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return isXAICredential(credential) &&
		(evidence.XAIDepletedKind == "free" || (evidence.PlanType == core.PlanTypeFree && evidence.Remaining != nil && *evidence.Remaining <= 0 && evidence.XAIDepletedKind == ""))
}

func isXAIWeeklyDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return isXAICredential(credential) && evidence.XAIDepletedKind == "weekly"
}

func isXAIMonthlyAndWeeklyDepleted(credential core.Credential, evidence ProbeEvidence) bool {
	return isXAICredential(credential) && evidence.XAIDepletedKind == "monthly_and_weekly"
}

func xaiFreeDepletedPriority(options Options) int {
	if options.XAIFreeDepletedPriority == nil {
		return -1
	}
	return *options.XAIFreeDepletedPriority
}

func xaiFreeDepletedDisabled(options Options) bool {
	if options.XAIFreeDepletedDisabled == nil {
		return true
	}
	return *options.XAIFreeDepletedDisabled
}

func xaiWeeklyDepletedPriority(options Options) int {
	if options.XAIWeeklyDepletedPriority == nil {
		return -1
	}
	return *options.XAIWeeklyDepletedPriority
}

func xaiMonthlyAndWeeklyDepletedPriority(options Options) int {
	if options.XAIMonthlyAndWeeklyDepletedPriority == nil {
		return -1
	}
	return *options.XAIMonthlyAndWeeklyDepletedPriority
}

func xaiMonthlyAndWeeklyDepletedDisabled(options Options) bool {
	if options.XAIMonthlyAndWeeklyDepletedDisabled == nil {
		return true
	}
	return *options.XAIMonthlyAndWeeklyDepletedDisabled
}

func planFreshPositive(items []PlanItem, options Options) {
	candidates := positiveCandidates(items, options)
	for _, group := range providerCandidateGroups(items, candidates) {
		slices.SortStableFunc(group, func(left int, right int) int {
			return compareCandidates(items[left], items[right], options)
		})
		priority := startPriorityForProvider(planItemProvider(items[group[0]]), options)
		for _, itemIndex := range group {
			items[itemIndex].Priority = plannedPriority(items[itemIndex], priority, options)
			// 禁用因额度耗尽的凭证，在探测到正向剩余额度后自动恢复启用并参与常规排序。
			items[itemIndex].Disabled = false
			items[itemIndex].Reason = "fresh remaining positive"
			priority--
			if priority < 1 {
				priority = 1
			}
		}
	}
}

// ensureUniqueProviderPriorities 保证同 provider 启用态 priority>=1 的槽位唯一。
// 参与者包括：本轮 fresh 正额度、以及仍占用正优先级的无 fresh 同伴（历史局部写回残留）。
// 不改写 disabled 或 priority<=0（含 depleted -1）的凭证。
func ensureUniqueProviderPriorities(items []PlanItem, options Options) {
	order := make([]core.Provider, 0)
	seen := make(map[core.Provider]struct{})
	groups := make(map[core.Provider][]int)
	for index, item := range items {
		if item.Disabled || item.Priority < 1 {
			continue
		}
		// 仅当本轮至少有一条同 provider 的 fresh 正额度证据时，才触发去重写回，
		// 避免无探测的空跑改写全站优先级。
		provider := planItemProvider(item)
		if _, ok := seen[provider]; !ok {
			seen[provider] = struct{}{}
			order = append(order, provider)
		}
		groups[provider] = append(groups[provider], index)
	}
	for _, provider := range order {
		group := groups[provider]
		if !providerGroupHasFreshPositive(items, group) {
			continue
		}
		if !providerGroupHasPriorityCollision(items, group) && !providerGroupNeedsStartRealign(items, group, options) {
			// 无碰撞且已从 start 对齐时仍可能因局部写回导致「全员 start」；
			// providerGroupHasPriorityCollision 已覆盖重复值；此处保留 full re-pack 仅在有碰撞时。
			continue
		}
		slices.SortStableFunc(group, func(left int, right int) int {
			return compareUniquenessCandidates(items[left], items[right], options)
		})
		priority := startPriorityForProvider(provider, options)
		for _, itemIndex := range group {
			// resetBoost 999 仅保留给有 fresh 的 boost 项；无 fresh 同伴不得继承 999。
			nextPriority := priority
			if items[itemIndex].EvidenceFresh {
				nextPriority = plannedPriority(items[itemIndex], priority, options)
			}
			if items[itemIndex].Priority != nextPriority {
				if !items[itemIndex].EvidenceFresh {
					items[itemIndex].ForceWrite = true
					items[itemIndex].Reason = "provider priority uniqueness"
				} else if items[itemIndex].Reason == "keep current state" || items[itemIndex].Reason == "" {
					items[itemIndex].Reason = "provider priority uniqueness"
				}
				items[itemIndex].Priority = nextPriority
			}
			priority--
			if priority < 1 {
				priority = 1
			}
		}
	}
}

func providerGroupHasFreshPositive(items []PlanItem, group []int) bool {
	for _, index := range group {
		item := items[index]
		if item.EvidenceFresh && item.Remaining != nil && *item.Remaining > 0 {
			return true
		}
		if item.EvidenceFresh && item.Reason == "fresh remaining positive" {
			return true
		}
	}
	return false
}

func providerGroupHasPriorityCollision(items []PlanItem, group []int) bool {
	seen := make(map[int]struct{}, len(group))
	for _, index := range group {
		priority := items[index].Priority
		if _, ok := seen[priority]; ok {
			return true
		}
		seen[priority] = struct{}{}
	}
	return false
}

func providerGroupNeedsStartRealign(items []PlanItem, group []int, options Options) bool {
	// 预留：当前仅在碰撞时 re-pack；保留钩子便于后续策略扩展。
	_ = options
	_ = items
	_ = group
	return false
}

func compareUniquenessCandidates(left PlanItem, right PlanItem, options Options) int {
	leftFreshPositive := left.EvidenceFresh && left.Remaining != nil && *left.Remaining > 0
	rightFreshPositive := right.EvidenceFresh && right.Remaining != nil && *right.Remaining > 0
	switch {
	case leftFreshPositive && rightFreshPositive:
		return compareCandidates(left, right, options)
	case leftFreshPositive:
		return -1
	case rightFreshPositive:
		return 1
	}
	// 无 fresh 同伴：较高现有优先级在前，其次 AuthIndex，保证稳定可复现。
	if left.Priority != right.Priority {
		return right.Priority - left.Priority
	}
	return cmp.Compare(left.Credential.AuthIndex, right.Credential.AuthIndex)
}

func providerCandidateGroups(items []PlanItem, candidates []int) [][]int {
	order := make([]core.Provider, 0)
	seen := make(map[core.Provider]struct{})
	groups := make(map[core.Provider][]int)
	for _, itemIndex := range candidates {
		provider := planItemProvider(items[itemIndex])
		if _, ok := seen[provider]; !ok {
			seen[provider] = struct{}{}
			order = append(order, provider)
		}
		groups[provider] = append(groups[provider], itemIndex)
	}
	result := make([][]int, 0, len(order))
	for _, provider := range order {
		result = append(result, groups[provider])
	}
	return result
}

func planItemProvider(item PlanItem) core.Provider {
	if item.Credential.Provider != "" {
		return item.Credential.Provider
	}
	switch item.Credential.Type {
	case core.CredentialTypeCodex:
		return core.ProviderCodex
	case core.CredentialTypeAntigravity:
		return core.ProviderAntigravity
	case core.CredentialTypeXAI:
		return core.ProviderXAI
	default:
		return core.ProviderUnknown
	}
}

func positiveCandidates(items []PlanItem, options Options) []int {
	candidates := make([]int, 0, len(items))
	for index, item := range items {
		if !item.EvidenceFresh || item.Remaining == nil {
			continue
		}
		if *item.Remaining > 0 {
			candidates = append(candidates, index)
			continue
		}
		if isAntigravityWeeklyDepletedItem(item) {
			continue
		}
		// Remaining<=0：仅 long-window near-reset 可再入排序（Gemini 周额度）；
		// Codex depleted 已在 initialItems 处理，禁止再进 planFreshPositive。
		if planItemProvider(item) != core.ProviderCodex && resetBoost(item, options) > 0 {
			candidates = append(candidates, index)
		}
	}
	return candidates
}

func compareCandidates(left PlanItem, right PlanItem, options Options) int {
	// xAI：禁止套餐名序；优先剩余额度，其次重置时间，再 AuthIndex 破同分。
	if planItemProvider(left) == core.ProviderXAI || planItemProvider(right) == core.ProviderXAI {
		if left.Remaining != nil && right.Remaining != nil && *left.Remaining != *right.Remaining {
			if *left.Remaining > *right.Remaining {
				return -1
			}
			return 1
		}
		if result := compareResetAt(left.ResetAt, right.ResetAt); result != 0 {
			return result
		}
		return cmp.Compare(left.Credential.AuthIndex, right.Credential.AuthIndex)
	}
	if options.PaidFirst && paidRank(left.PlanType) != paidRank(right.PlanType) {
		return paidRank(right.PlanType) - paidRank(left.PlanType)
	}
	if result := compareResetAt(left.ResetAt, right.ResetAt); result != 0 {
		return result
	}
	return cmp.Compare(left.Credential.AuthIndex, right.Credential.AuthIndex)
}

func paidRank(planType core.PlanType) int {
	switch planType {
	case core.PlanTypeTeam, core.PlanTypePlus, core.PlanTypePro:
		return 1
	case core.PlanTypeFree, core.PlanTypeUnknown:
		return 0
	default:
		return 0
	}
}

func compareResetAt(left *time.Time, right *time.Time) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return 1
	case right == nil:
		return -1
	case left.Equal(*right):
		return 0
	case left.Before(*right):
		return -1
	default:
		return 1
	}
}

func normalizedMaxPriority(maxPriority int) int {
	if maxPriority < 1 {
		return 1
	}
	return maxPriority
}

func startPriorityForProvider(provider core.Provider, options Options) int {
	if options.StartPriorityByProvider != nil {
		if priority, ok := options.StartPriorityByProvider[provider]; ok {
			return normalizedMaxPriority(priority)
		}
	}
	return normalizedMaxPriority(options.MaxPriority)
}

func sortPlanItems(items []PlanItem) {
	slices.SortStableFunc(items, func(left PlanItem, right PlanItem) int {
		if left.EvidenceFresh && right.EvidenceFresh {
			if left.Priority != right.Priority {
				return right.Priority - left.Priority
			}
			return cmp.Compare(left.Credential.AuthIndex, right.Credential.AuthIndex)
		}
		if left.EvidenceFresh {
			return -1
		}
		if right.EvidenceFresh {
			return 1
		}
		return 0
	})
}

func changes(items []PlanItem, options Options) []Change {
	result := make([]Change, 0)
	for _, item := range items {
		if shouldChange(item, options) {
			result = append(result, Change{
				Credential: item.Credential,
				Priority:   item.Priority,
				Disabled:   item.Disabled,
				// ForceWrite 同伴无本轮 probe，但必须通过 apply 的 EvidenceFresh 写入门闸。
				EvidenceFresh: item.EvidenceFresh || item.ForceWrite,
				Reason:        item.Reason,
			})
		}
	}
	return result
}

func shouldChange(item PlanItem, options Options) bool {
	// ForceWrite：同 provider 优先级去重改写无 fresh 同伴时必须写回宿主。
	if !item.EvidenceFresh && !item.ForceWrite {
		return false
	}
	if item.ForceWrite && !item.EvidenceFresh {
		if item.Priority == item.Credential.Priority && item.Disabled == item.Credential.Disabled {
			return false
		}
		return abs(item.Priority-item.Credential.Priority) >= normalizedMinChange(options.MinChange) ||
			item.Disabled != item.Credential.Disabled ||
			item.Credential.PriorityMissing
	}
	if item.Credential.PriorityMissing {
		return true
	}
	if item.Priority == item.Credential.Priority && item.Disabled == item.Credential.Disabled {
		return false
	}
	if item.Priority == -1 && item.Disabled {
		return item.Credential.Priority != -1 || !item.Credential.Disabled
	}
	if item.Credential.Disabled != item.Disabled {
		return true
	}
	return abs(item.Priority-item.Credential.Priority) >= normalizedMinChange(options.MinChange)
}

func normalizedMinChange(minChange int) int {
	if minChange < 0 {
		return 0
	}
	return minChange
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
