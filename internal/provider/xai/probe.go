package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"credential-priority/internal/core"
	"credential-priority/internal/host"
)

// httpDoer 必须能返回非 2xx 响应体（xAI 免费/周/月耗尽常为 429）。
type httpDoer interface {
	HTTPDoRaw(ctx context.Context, req host.HTTPRequest) (host.HTTPResponse, error)
}

// Prober 通过宿主 HTTPDo 执行 xAI 轻量上游探测。
type Prober struct {
	host  httpDoer
	clock clock
}

// NewProber 创建使用宿主 HTTPDo 和注入时钟的 xAI fresh prober。
func NewProber(hostAPI httpDoer, clockSource clock) Prober {
	if clockSource == nil {
		clockSource = realClock{}
	}
	return Prober{host: hostAPI, clock: clockSource}
}

// Probe 绑定 AuthIndex，经 host.http.do 发送最小上游请求并解析额度信号。
// 依次尝试 /responses 与 /chat/completions，以及固定小模型列表，优先采用 QuotaKnown 结果。
func (p Prober) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	observedAt := p.clock.Now().UTC()
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	models := probeModels(request.Model)
	var last ProbeResult
	var lastStatus int
	for _, model := range models {
		for _, attempt := range probeAttempts(baseURL, model) {
			response, err := p.host.HTTPDoRaw(ctx, host.HTTPRequest{
				AuthIndex: request.AuthIndex,
				Method:    http.MethodPost,
				URL:       attempt.url,
				Headers:   probeHeaders(request),
				Body:      attempt.body,
			})
			if err != nil {
				last = failedProbe(request, observedAt, "host http do failed")
				continue
			}
			lastStatus = response.StatusCode
			result := ParseProbeResponse(response.StatusCode, response.Body, observedAt)
			result.Provider = core.ProviderXAI
			result.AuthIndex = request.AuthIndex
			if result.QuotaKnown && result.Status == StatusReady {
				return result
			}
			last = result
			if last.Error == "" {
				last.Error = safeError(fmt.Sprintf("xai probe status %d", response.StatusCode))
			}
		}
	}
	if last.AuthIndex == "" {
		last = failedProbe(request, observedAt, fmt.Sprintf("xai probe status %d", lastStatus))
	}
	last.Provider = core.ProviderXAI
	last.AuthIndex = request.AuthIndex
	return last
}

type probeAttempt struct {
	url  string
	body []byte
}

func probeModels(preferred string) []string {
	preferred = strings.TrimSpace(preferred)
	out := make([]string, 0, 2)
	seen := map[string]struct{}{}
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" {
			return
		}
		if _, ok := seen[m]; ok {
			return
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	// 收敛探测：默认非推理模型 + free 模型（用于 free-usage 信号）；避免多模型串行拖垮整轮自动排序。
	add(preferred)
	add(DefaultProbeModel)
	if DefaultProbeModel != "grok-4.5-build-free" {
		add("grok-4.5-build-free")
	}
	return out
}

func probeAttempts(baseURL, model string) []probeAttempt {
	// OAuth api.x.ai 上 chat/completions 更轻量；仅失败再试 /responses。
	// multi-agent 模型跳过 chat completions。
	attempts := make([]probeAttempt, 0, 2)
	if !strings.Contains(strings.ToLower(model), "multi-agent") {
		chatBody, _ := json.Marshal(map[string]any{
			"model":      model,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 1,
			"stream":     false,
		})
		attempts = append(attempts, probeAttempt{url: baseURL + "/chat/completions", body: chatBody})
	}
	responsesBody, _ := json.Marshal(map[string]any{
		"model":             model,
		"input":             "ping",
		"stream":            false,
		"max_output_tokens": 1,
	})
	attempts = append(attempts, probeAttempt{url: baseURL + "/responses", body: responsesBody})
	return attempts
}

func failedProbe(request ProbeRequest, observedAt time.Time, message string) ProbeResult {
	result := failedResult(observedAt, safeError(message))
	result.Provider = core.ProviderXAI
	result.AuthIndex = request.AuthIndex
	return result
}

func probeHeaders(request ProbeRequest) host.Header {
	// 与 Codex 一致：有 AccessToken 时用字面量；否则 $TOKEN$ 由宿主按 AuthIndex 注入。
	token := "$TOKEN$"
	if accessToken := strings.TrimSpace(request.AccessToken); accessToken != "" {
		token = accessToken
	}
	return host.Header{
		"Accept":        []string{"application/json"},
		"Authorization": []string{"Bearer " + token},
		"Content-Type":  []string{"application/json"},
		"User-Agent":    []string{"credential-priority/xai-probe"},
		// 与 CPA xAI executor 对齐的客户端标识（部分 chat-proxy 路径需要）。
		"X-XAI-Token-Auth":       []string{"xai-grok-cli"},
		"x-grok-client-version":  []string{"0.2.93"},
	}
}
