package codex

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"credential-priority/internal/host"
)

type httpDoer interface {
	HTTPDo(ctx context.Context, req host.HTTPRequest) (host.HTTPResponse, error)
}

// Prober 通过宿主 HTTPDo 执行 Codex/ChatGPT wham usage fresh probe。
type Prober struct {
	host  httpDoer
	clock clock
}

// NewProber 创建使用宿主 HTTPDo 和注入时钟的 Codex/ChatGPT fresh prober。
func NewProber(hostAPI httpDoer, clockSource clock) Prober {
	if clockSource == nil {
		clockSource = realClock{}
	}
	return Prober{host: hostAPI, clock: clockSource}
}

// Probe 请求 ChatGPT wham usage 并返回只包含安全字段的 probe 结果。
func (p Prober) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	observedAt := p.clock.Now().UTC()
	response, err := p.host.HTTPDo(ctx, host.HTTPRequest{
		Method:  http.MethodGet,
		URL:     WhamUsageURL,
		Headers: probeHeaders(request),
	})
	if err != nil {
		return failedProbe(request, observedAt, "host http do failed")
	}
	if response.StatusCode != http.StatusOK {
		return failedProbe(request, observedAt, fmt.Sprintf("wham usage status %d", response.StatusCode))
	}
	result := ParseWhamUsage(response.Body, observedAt)
	result.Provider = request.Provider
	result.AuthIndex = request.AuthIndex
	return result
}

func failedProbe(request ProbeRequest, observedAt time.Time, message string) ProbeResult {
	result := failedResult(observedAt, safeError(message))
	result.Provider = request.Provider
	result.AuthIndex = request.AuthIndex
	return result
}

func probeHeaders(request ProbeRequest) host.Header {
	headers := host.Header{
		"Accept":        []string{"application/json"},
		"Authorization": []string{"Bearer $TOKEN$"},
		"User-Agent":    []string{"codex_cli_rs/0.76.0"},
	}
	accountID := strings.TrimSpace(request.AccountID)
	if accountID != "" {
		headers["Chatgpt-Account-Id"] = []string{accountID}
	}
	return headers
}

func safeError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "probe failed"
	}
	return trimmed
}
