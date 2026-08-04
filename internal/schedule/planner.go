package schedule

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"credential-priority/internal/core"
)

// ErrInvalidOptions 标识探测计划参数不满足调度器不变量。
var ErrInvalidOptions = errors.New("schedule: invalid options")

// Clock 提供可注入时间源，避免调度计划依赖真实 wall clock。
type Clock interface {
	Now() time.Time
}

// RNG 提供可注入随机源，用于生成可复现的 active group jitter。
type RNG interface {
	Int63n(int64) int64
}

// Options 是探测调度策略的已解析参数。
//
// Interval 用于 disabled 分批递进（now + k*Interval，k 从 1 起）；
// 组大小与 active 共用 ActiveGroupSize。旧 DisabledGroupSize / DisabledProbeInterval
// 字段保留为零值兼容，不再参与校验与调度。
type Options struct {
	Clock                 Clock
	RNG                   RNG
	ImmediateProbeLimit   int
	TopPriorityProbeCount int
	ActiveGroupSize       int
	ActiveGroupJitter     time.Duration
	// Interval 是 disabled 分批探测的时间步长（通常来自 cfg.Interval）。
	Interval time.Duration
	// DisabledGroupSize 已废弃，不再驱动调度（保留字段避免调用方编译失败）。
	DisabledGroupSize int
	// DisabledProbeInterval 已废弃，不再驱动 1h 冷冻（保留字段避免调用方编译失败）。
	DisabledProbeInterval time.Duration
}

// Probe 表示单个凭证下一次探测时间。
type Probe struct {
	Credential  core.Credential
	NextProbeAt time.Time
}

// ProbeGroup 表示一批共享同一调度时间的探测任务。
type ProbeGroup struct {
	Probes []Probe
}

// Plan 是本轮调度产出的立即探测与延迟分组。
type Plan struct {
	Immediate      []Probe
	ActiveGroups   []ProbeGroup
	DisabledGroups []ProbeGroup
}

// PlanProbeSchedule 根据用户拍板策略生成可复现的探测分组。
//
// 当总数 ≤ ImmediateProbeLimit 时，active 与 disabled 一并进入 Immediate（now 探测），
// 完全去掉固定 1h 冷冻。否则 disabled 按 ActiveGroupSize 分批，第 k 批为 now+k*Interval（k≥1）。
func PlanProbeSchedule(credentials []core.Credential, options Options) (Plan, error) {
	if err := validateOptions(options); err != nil {
		return Plan{}, err
	}
	now := options.Clock.Now()
	ordered := sortedCredentials(credentials)
	active, disabled := partitionCredentials(ordered)
	if len(credentials) <= options.ImmediateProbeLimit {
		// 小规模：disabled 与 active 一起立即探测，不再进入 1h 阶梯 DisabledGroups。
		immediate := append(probesAt(active, now), probesAt(disabled, now)...)
		return Plan{Immediate: immediate}, nil
	}
	immediateCount := min(options.TopPriorityProbeCount, len(active))
	return Plan{
		Immediate:      probesAt(active[:immediateCount], now),
		ActiveGroups:   activeProbeGroups(active[immediateCount:], now, options),
		DisabledGroups: disabledProbeGroups(disabled, now, options),
	}, nil
}

func validateOptions(options Options) error {
	switch {
	case options.Clock == nil:
		return fmt.Errorf("clock: %w", ErrInvalidOptions)
	case options.RNG == nil:
		return fmt.Errorf("rng: %w", ErrInvalidOptions)
	case options.ImmediateProbeLimit < 1:
		return fmt.Errorf("immediate probe limit %d: %w", options.ImmediateProbeLimit, ErrInvalidOptions)
	case options.TopPriorityProbeCount < 1:
		return fmt.Errorf("top priority probe count %d: %w", options.TopPriorityProbeCount, ErrInvalidOptions)
	case options.ActiveGroupSize < 1:
		return fmt.Errorf("active group size %d: %w", options.ActiveGroupSize, ErrInvalidOptions)
	case options.ActiveGroupJitter < 0:
		return fmt.Errorf("active group jitter %s: %w", options.ActiveGroupJitter, ErrInvalidOptions)
	case options.Interval <= 0:
		return fmt.Errorf("interval %s: %w", options.Interval, ErrInvalidOptions)
	default:
		return nil
	}
}

func sortedCredentials(credentials []core.Credential) []core.Credential {
	ordered := slices.Clone(credentials)
	slices.SortStableFunc(ordered, func(left core.Credential, right core.Credential) int {
		if left.Priority != right.Priority {
			return left.Priority - right.Priority
		}
		if left.Name != right.Name {
			return compareText(left.Name, right.Name)
		}
		return compareText(left.AuthIndex, right.AuthIndex)
	})
	return ordered
}

func compareText(left string, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func partitionCredentials(credentials []core.Credential) ([]core.Credential, []core.Credential) {
	active := make([]core.Credential, 0, len(credentials))
	disabled := make([]core.Credential, 0)
	for _, credential := range credentials {
		// 物理 disabled，或 soft-disabled（priority<0 且仍 enabled）走 DisabledGroups 低频探测，
		// 避免 free depleted 软禁用账号按 CacheTTL/15min 狂探。
		if credential.Disabled || credential.Priority < 0 {
			disabled = append(disabled, credential)
			continue
		}
		active = append(active, credential)
	}
	return active, disabled
}

func activeProbeGroups(credentials []core.Credential, now time.Time, options Options) []ProbeGroup {
	groups := make([]ProbeGroup, 0, groupCount(len(credentials), options.ActiveGroupSize))
	for start := 0; start < len(credentials); start += options.ActiveGroupSize {
		end := min(start+options.ActiveGroupSize, len(credentials))
		groups = append(groups, ProbeGroup{Probes: probesAt(credentials[start:end], jitteredAt(now, options))})
	}
	return groups
}

func disabledProbeGroups(credentials []core.Credential, now time.Time, options Options) []ProbeGroup {
	// 与 active 共用 ActiveGroupSize；时间步长用 Interval（禁止固定 1h 冷冻）。
	size := options.ActiveGroupSize
	groups := make([]ProbeGroup, 0, groupCount(len(credentials), size))
	for start := 0; start < len(credentials); start += size {
		end := min(start+size, len(credentials))
		// k 从 1 起：第 1 批 now+Interval，第 2 批 now+2*Interval …
		groupNumber := start/size + 1
		nextProbeAt := now.Add(time.Duration(groupNumber) * options.Interval)
		groups = append(groups, ProbeGroup{Probes: probesAt(credentials[start:end], nextProbeAt)})
	}
	return groups
}

func groupCount(length int, size int) int {
	if length == 0 {
		return 0
	}
	return (length + size - 1) / size
}

func jitteredAt(now time.Time, options Options) time.Time {
	if options.ActiveGroupJitter == 0 {
		return now
	}
	offset := time.Duration(options.RNG.Int63n(options.ActiveGroupJitter.Nanoseconds()) + 1)
	return now.Add(offset)
}

func probesAt(credentials []core.Credential, nextProbeAt time.Time) []Probe {
	probes := make([]Probe, len(credentials))
	for index, credential := range credentials {
		probes[index] = Probe{Credential: credential, NextProbeAt: nextProbeAt}
	}
	return probes
}
