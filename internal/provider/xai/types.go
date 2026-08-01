package xai

import "time"

// DefaultAPIBaseURL 是 xAI 官方 API 根路径（与 CPA auth.xai 默认一致）。
const DefaultAPIBaseURL = "https://api.x.ai/v1"

// DepletedKind 标识耗尽类型（供 planner / store 映射独立规则）。
// 生产路径：free 由 usage 连续失败 + 冷却写入；weekly / monthly_and_weekly 保留常量供策略兼容。
type DepletedKind string

const (
	// DepletedNone 表示未耗尽或仍有可用额度。
	DepletedNone DepletedKind = ""
	// DepletedFree 表示免费额度耗尽（usage 连续失败达阈值后的 soft cooldown）。
	DepletedFree DepletedKind = "free"
	// DepletedWeekly 表示仅周限额耗尽。
	DepletedWeekly DepletedKind = "weekly"
	// DepletedMonthlyAndWeekly 表示周限额与月度积分均耗尽。
	DepletedMonthlyAndWeekly DepletedKind = "monthly_and_weekly"
)

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}
