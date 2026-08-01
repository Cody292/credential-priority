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
// 仅正额度可立即返回。free/付费耗尽均缓存后换下一模型继续探测。
// 最终优先级：positive > freeDepleted > paidDepleted > last。
func (p Prober) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	observedAt := p.clock.Now().UTC()
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	models := probeModels(request.Model)
	var last ProbeResult
	var lastStatus int
	var freeDepleted *ProbeResult
	var paidDepleted *ProbeResult
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
			result := ParseProbeResponse(response.StatusCode, response.Body, observedAt, model)
			result.Provider = core.ProviderXAI
			result.AuthIndex = request.AuthIndex
			if !result.QuotaKnown || result.Status != StatusReady {
				last = result
				if last.Error == "" {
					last.Error = safeError(fmt.Sprintf("xai probe status %d", response.StatusCode))
				}
				continue
			}
			if isPositiveQuota(result) {
				return result
			}
			// free 耗尽：缓存后 break 当前模型 endpoint 循环，继续下一模型（如 grok 4.5 paid）。
			if result.DepletedKind == DepletedFree {
				if freeDepleted == nil {
					copy := result
					freeDepleted = &copy
				}
				last = result
				break
			}
			if isPaidWindowDepleted(result) {
				if paidDepleted == nil {
					copy := result
					paidDepleted = &copy
				}
				last = result
				// 付费周/月耗尽：该模型不再试其它 endpoint，换下一模型。
				break
			}
			last = result
		}
	}
	// 优先级：positive 已 return；free 耗尽优先于付费窗口耗尽。
	if freeDepleted != nil {
		return *freeDepleted
	}
	if paidDepleted != nil {
		return *paidDepleted
	}
	if last.AuthIndex == "" {
		last = failedProbe(request, observedAt, fmt.Sprintf("xai probe status %d", lastStatus))
	}
	last.Provider = core.ProviderXAI
	last.AuthIndex = request.AuthIndex
	return last
}

func isPositiveQuota(result ProbeResult) bool {
	if result.DepletedKind != DepletedNone {
		return false
	}
	return result.Remaining != nil && *result.Remaining > 0
}

// isPaidWindowDepleted：付费周/月耗尽不得短路，须继续试 free 模型。
func isPaidWindowDepleted(result ProbeResult) bool {
	switch result.DepletedKind {
	case DepletedWeekly, DepletedMonthlyAndWeekly:
		return true
	default:
		return result.WeeklyDepleted || result.MonthlyDepleted
	}
}

type probeAttempt struct {
	url  string
	body []byte
}

// freeProbeModel 是 xAI 免费额度探测优先模型（现网有效：grok-build-0.1；
// grok-4.5-build-free 已 404 not-found，仅作兼容别名探测）。
const freeProbeModel = "grok-build-0.1"

// legacyFreeProbeModel 历史 free 模型名（部分旧文案仍引用；探测顺序靠后）。
const legacyFreeProbeModel = "grok-4.5-build-free"

func probeModels(preferred string) []string {
	preferred = strings.TrimSpace(preferred)
	out := make([]string, 0, 4)
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
	// free 模型优先：现网 grok-build-0.1 可 2xx；再 legacy free 名；再 preferred / 付费默认。
	add(freeProbeModel)
	add(legacyFreeProbeModel)
	add(preferred)
	add(DefaultProbeModel)
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
