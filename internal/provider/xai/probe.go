package xai

import (
	"context"

	"credential-priority/internal/host"
)

// httpDoer 必须能返回非 2xx 响应体（xAI settings/billing 鉴权失败常为 401）。
type httpDoer interface {
	HTTPDoRaw(ctx context.Context, req host.HTTPRequest) (host.HTTPResponse, error)
}

// Prober 通过宿主 HTTPDo 执行 xAI 轻量套餐分类（FetchPlan）。
// 生产路径不再使用 chat/completions 多模型探额度。
type Prober struct {
	host  httpDoer
	clock clock
}

// NewProber 创建使用宿主 HTTPDo 和注入时钟的 xAI prober。
func NewProber(hostAPI httpDoer, clockSource clock) Prober {
	if clockSource == nil {
		clockSource = realClock{}
	}
	return Prober{host: hostAPI, clock: clockSource}
}
