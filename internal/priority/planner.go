package priority

import (
	"cmp"
	"slices"
	"time"

	"credential-priority/internal/core"
)

// Options 是 fresh-only 优先级规划器的已解析策略参数。
type Options struct {
	Now              time.Time
	MaxPriority      int
	MinChange        int
	PaidFirst        bool
	ResetBoostWithin time.Duration
	ResetBoost       int
}

// ProbeEvidence 是本轮 probe 产出的排序证据；EvidenceFresh=false 时不得驱动变更。
type ProbeEvidence struct {
	Provider      core.Provider
	AuthIndex     string
	ObservedAt    time.Time
	ResetAt       *time.Time
	Remaining     *int64
	Freshness     core.Freshness
	ProbeStatus   core.ProbeStatus
	Status        EvidenceStatus
	PlanType      core.PlanType
	EvidenceFresh bool
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
	Credential    core.Credential
	Priority      int
	Disabled      bool
	PlanType      core.PlanType
	ResetAt       *time.Time
	Remaining     *int64
	EvidenceFresh bool
	Reason        string
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
	items := initialItems(credentials, evidenceByAuthIndex)
	planFreshPositive(items, options)
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

func initialItems(credentials []core.Credential, evidenceByAuthIndex map[string]ProbeEvidence) []PlanItem {
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
		if ok && !credential.Unavailable {
			if evidence.Remaining != nil && *evidence.Remaining <= 0 {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.EvidenceFresh = true
				item.Priority = -1
				item.Disabled = true
				item.Reason = "fresh remaining depleted"
			} else if evidence.Remaining != nil && evidence.ResetAt != nil {
				item.PlanType = evidence.PlanType
				item.ResetAt = evidence.ResetAt
				item.Remaining = evidence.Remaining
				item.EvidenceFresh = true
			}
		}
		items[index] = item
	}
	return items
}

func planFreshPositive(items []PlanItem, options Options) {
	candidates := positiveCandidates(items)
	slices.SortStableFunc(candidates, func(left int, right int) int {
		return compareCandidates(items[left], items[right], options)
	})
	priority := normalizedMaxPriority(options.MaxPriority)
	for _, itemIndex := range candidates {
		items[itemIndex].Priority = priority + resetBoost(items[itemIndex], options)
		items[itemIndex].Disabled = false
		items[itemIndex].Reason = "fresh remaining positive"
		priority--
		if priority < 1 {
			priority = 1
		}
	}
}

func positiveCandidates(items []PlanItem) []int {
	candidates := make([]int, 0, len(items))
	for index, item := range items {
		if item.EvidenceFresh && item.Remaining != nil && *item.Remaining > 0 {
			candidates = append(candidates, index)
		}
	}
	return candidates
}

func compareCandidates(left PlanItem, right PlanItem, options Options) int {
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

func resetBoost(item PlanItem, options Options) int {
	if options.ResetBoostWithin <= 0 || options.ResetBoost <= 0 || item.ResetAt == nil {
		return 0
	}
	if item.ResetAt.After(options.Now) && item.ResetAt.Sub(options.Now) < options.ResetBoostWithin {
		return options.ResetBoost
	}
	return 0
}

func normalizedMaxPriority(maxPriority int) int {
	if maxPriority < 1 {
		return 1
	}
	return maxPriority
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
				Credential:    item.Credential,
				Priority:      item.Priority,
				Disabled:      item.Disabled,
				EvidenceFresh: item.EvidenceFresh,
				Reason:        item.Reason,
			})
		}
	}
	return result
}

func shouldChange(item PlanItem, options Options) bool {
	if !item.EvidenceFresh {
		return false
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
