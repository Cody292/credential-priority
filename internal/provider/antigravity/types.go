package antigravity

import (
	"time"

	"credential-priority/internal/config"
	"credential-priority/internal/core"
)

// ModelGroup 表示 Antigravity 上游独立计量的模型组。
type ModelGroup = config.AntigravityModelGroup

const (
	// ModelGroupGemini 表示 Gemini 模型组。
	ModelGroupGemini = config.AntigravityModelGroupGemini
	// ModelGroupClaudeGPT 表示 Claude 和 GPT 模型组。
	ModelGroupClaudeGPT = config.AntigravityModelGroupClaudeGPT
)

// RetrieveUserQuotaSummaryURL 是 CPA 配额页使用的 Antigravity 额度查询端点。
const RetrieveUserQuotaSummaryURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"

var retrieveUserQuotaSummaryURLs = []string{
	RetrieveUserQuotaSummaryURL,
	"https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary",
	"https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary",
}

// WindowType 标识 Antigravity quota 响应中选中的额度窗口类型。
type WindowType string

const (
	// WindowUnknown 表示响应未标注明确窗口类型。
	WindowUnknown WindowType = "unknown"
	// WindowFiveHour 表示 Pro 计划 5 小时额度窗口。
	WindowFiveHour WindowType = "5h"
	// WindowWeekly 表示周额度窗口。
	WindowWeekly WindowType = "weekly"
)

// Status 标识一次 Antigravity fresh probe 的可用性结论。
type Status string

const (
	// StatusReady 表示 fresh probe 产出了可用于排序的模型组额度信号。
	StatusReady Status = "ready"
	// StatusProbeFailed 表示 probe 未产出可信模型组额度信号。
	StatusProbeFailed Status = "probe_failed"
)

// ProbeResult 是 Antigravity fresh probe 的安全输出。
type ProbeResult struct {
	Provider          core.Provider
	AuthIndex         string
	ModelGroup        ModelGroup
	ObservedAt        time.Time
	ResetAt           *time.Time
	Remaining         *int64
	Window            WindowType
	LongWindowResetAt *time.Time
	Freshness         core.Freshness
	ProbeStatus       core.ProbeStatus
	Status            Status
	PlanType          core.PlanType
	Error             string
}

// ProbeRequest 是执行 Antigravity quota summary fresh probe 所需的认证上下文。
type ProbeRequest struct {
	AuthIndex   string
	AccessToken string
	ProjectID   string
	ModelGroup  ModelGroup
}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}
