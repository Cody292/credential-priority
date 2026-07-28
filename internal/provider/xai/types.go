package xai

import (
	"time"

	"credential-priority/internal/core"
)

// DefaultAPIBaseURL 是 xAI 官方 API 根路径（与 CPA auth.xai 默认一致）。
const DefaultAPIBaseURL = "https://api.x.ai/v1"

// DefaultProbeModel 是轻量探测使用的固定小模型（免费额度文案与现网一致）。
const DefaultProbeModel = "grok-4.20-0309-non-reasoning"

// WindowType 标识 xAI 探测解析出的额度窗口类型。
type WindowType string

const (
	// WindowUnknown 表示未识别到可信额度窗口。
	WindowUnknown WindowType = "unknown"
	// WindowFree24h 表示免费滚动 24h 额度窗口。
	WindowFree24h WindowType = "free_24h"
	// WindowWeekly 表示付费周限额窗口。
	WindowWeekly WindowType = "weekly"
	// WindowMonthly 表示付费月度积分窗口。
	WindowMonthly WindowType = "monthly"
)

// Status 标识一次 xAI fresh probe 的可用性结论。
type Status string

const (
	// StatusReady 表示产出了可用于排序的额度信号（QuotaKnown）。
	StatusReady Status = "ready"
	// StatusProbeFailed 表示未产出可信额度信号，排序应保持现状。
	StatusProbeFailed Status = "probe_failed"
)

// DepletedKind 标识耗尽类型（供 planner 映射独立规则）。
type DepletedKind string

const (
	// DepletedNone 表示未耗尽或仍有可用额度。
	DepletedNone DepletedKind = ""
	// DepletedFree 表示免费额度耗尽。
	DepletedFree DepletedKind = "free"
	// DepletedWeekly 表示仅周限额耗尽（月度仍可用或未知月耗尽）。
	DepletedWeekly DepletedKind = "weekly"
	// DepletedMonthlyAndWeekly 表示周限额与月度积分均耗尽。
	DepletedMonthlyAndWeekly DepletedKind = "monthly_and_weekly"
)

// ProbeResult 是 xAI fresh probe 的安全输出（不含 token / 原始凭证 JSON）。
type ProbeResult struct {
	Provider          core.Provider
	AuthIndex         string
	ObservedAt        time.Time
	ResetAt           *time.Time
	Remaining         *int64
	Limit             *int64
	Window            WindowType
	LongWindowResetAt *time.Time
	Freshness         core.Freshness
	ProbeStatus       core.ProbeStatus
	Status            Status
	PlanType          core.PlanType
	DepletedKind      DepletedKind
	QuotaKnown        bool
	WeeklyDepleted    bool
	MonthlyDepleted   bool
	Error             string
}

// ProbeRequest 是执行 xAI 轻量上游探测所需的宿主凭证上下文。
type ProbeRequest struct {
	AuthIndex   string
	AccessToken string
	BaseURL     string
	Model       string
}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}
