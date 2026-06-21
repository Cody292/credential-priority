package runtime

import (
	"context"
	"errors"
	"time"

	"credential-priority/internal/config"
	"credential-priority/internal/host"
)

// ErrRunInProgress 表示本轮自动排序任务仍在执行，新的触发被 single-flight 拒绝。
var ErrRunInProgress = errors.New("runtime: run already in progress")

// ErrShutdown 表示 runtime 已经进入关闭状态。
var ErrShutdown = errors.New("runtime: shutdown")

// ErrInvalidRequest 表示 CPA 传入的 JSON 信封请求无效。
var ErrInvalidRequest = errors.New("runtime: invalid request")

// Ticker 是 runtime 自动触发循环依赖的最小 ticker 接口。
type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

// TickerFactory 创建可停止的 ticker，测试可注入手动 ticker 避免 sleep。
type TickerFactory interface {
	NewTicker(interval time.Duration) Ticker
}

// Clock 提供生产 runner 的当前时间，测试可注入固定时间避免 flaky。
type Clock interface {
	Now() time.Time
}

// Trigger 标识任务由管理端手动触发还是 ticker 自动触发。
type Trigger string

const (
	// TriggerManual 表示管理端手动调用 Run。
	TriggerManual Trigger = "manual"
	// TriggerAutoApply 表示 ticker 或宿主自动应用触发。
	TriggerAutoApply Trigger = "auto_apply"
)

// TaskRequest 是 runtime 交给后续任务实现的已解析输入。
type TaskRequest struct {
	Config  config.Config
	Trigger Trigger
}

// TaskRunner 执行一轮 credential-priority 任务。
type TaskRunner func(ctx context.Context, request TaskRequest) error

// Options 提供 runtime 的可注入依赖。
type Options struct {
	TickerFactory TickerFactory
	Runner        TaskRunner
	Host          host.HostCallbacks
	Clock         Clock
}

// RegisterRequest 是 plugin.register 的 JSON 请求形态。
type RegisterRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

// ReconfigureRequest 是 plugin.reconfigure 的 JSON 请求形态。
type ReconfigureRequest struct {
	ConfigYAML string `json:"config_yaml"`
}

// RegisterResult 是 plugin.register 返回给 CPA 宿主的元数据和能力声明。
type RegisterResult struct {
	SchemaVersion int             `json:"schema_version"`
	Metadata      Metadata        `json:"metadata"`
	Capabilities  map[string]bool `json:"capabilities"`
}

// Metadata 描述插件在 CPA 管理端展示的非敏感信息。
type Metadata struct {
	Name             string        `json:"Name"`
	Version          string        `json:"Version"`
	Author           string        `json:"Author"`
	GitHubRepository string        `json:"GitHubRepository"`
	Description      string        `json:"Description"`
	ConfigFields     []ConfigField `json:"ConfigFields"`
}

// ConfigField 描述 CPA 管理端可渲染的插件自有配置字段。
type ConfigField struct {
	Name        string   `json:"Name"`
	Type        string   `json:"Type"`
	Description string   `json:"Description"`
	EnumValues  []string `json:"EnumValues,omitempty"`
}
