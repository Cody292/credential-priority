package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	sensitiveTextPattern = regexp.MustCompile(`(?i)(authorization|api[_-]?key|token|cookie|set-cookie)(\s*[:=]\s*)([^\s,;\"'}]+)`)
	bearerTextPattern    = regexp.MustCompile(`(?i)(bearer\s+)([^\s,;\"'}]+)`)
)

// Client 将 CPA 宿主回调适配为本包 API。
type Client struct {
	callbacks HostCallbacks
}

// NewClient 创建宿主回调适配器。
func NewClient(callbacks HostCallbacks) *Client {
	return &Client{callbacks: callbacks}
}

// HTTPStatusError 表示宿主管理 HTTP 返回非 2xx 响应且错误正文已脱敏。
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

// Error 实现 error。
func (e *HTTPStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("host http status %d", e.StatusCode)
	}
	return fmt.Sprintf("host http status %d: %s", e.StatusCode, e.Body)
}

// ListAuthFiles 通过 host.auth.list 列出宿主凭证。
func (c *Client) ListAuthFiles(ctx context.Context) ([]AuthFile, error) {
	files, err := c.callbacks.ListAuthFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("host.auth.list: %w", err)
	}
	return files, nil
}

// GetRuntime 通过 host.auth.get_runtime 读取运行时凭证。
func (c *Client) GetRuntime(ctx context.Context, authIndex string) (RuntimeAuth, error) {
	runtime, err := c.callbacks.GetRuntime(ctx, authIndex)
	if err != nil {
		return RuntimeAuth{}, fmt.Errorf("host.auth.get_runtime: %w", err)
	}
	return runtime, nil
}

// SaveAuth 通过 host.auth.save 保存凭证文档。
func (c *Client) SaveAuth(ctx context.Context, name string, doc json.RawMessage) error {
	trimmed, err := stableName(name)
	if err != nil {
		return err
	}
	if err := c.callbacks.SaveAuth(ctx, trimmed, doc); err != nil {
		return fmt.Errorf("host.auth.save: %w", err)
	}
	return nil
}

// PatchPriority 通过宿主管理 HTTP 更新凭证优先级。
func (c *Client) PatchPriority(ctx context.Context, name string, priority int) error {
	trimmed, err := stableName(name)
	if err != nil {
		return err
	}
	body, err := json.Marshal(patchPriorityRequest{Name: trimmed, Priority: priority})
	if err != nil {
		return fmt.Errorf("encode priority patch: %w", err)
	}
	_, err = c.HTTPDo(ctx, HTTPRequest{
		Method:  "PATCH",
		URL:     ManagementAuthFieldsPath,
		Headers: jsonHeaders(),
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("patch priority: %w", err)
	}
	return nil
}

// PatchDisabled 通过宿主管理 HTTP 更新凭证禁用状态。
func (c *Client) PatchDisabled(ctx context.Context, name string, disabled bool) error {
	trimmed, err := stableName(name)
	if err != nil {
		return err
	}
	body, err := json.Marshal(patchDisabledRequest{Name: trimmed, Disabled: disabled})
	if err != nil {
		return fmt.Errorf("encode disabled patch: %w", err)
	}
	_, err = c.HTTPDo(ctx, HTTPRequest{
		Method:  "PATCH",
		URL:     ManagementAuthStatusPath,
		Headers: jsonHeaders(),
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("patch disabled: %w", err)
	}
	return nil
}

// HTTPDo 通过 host.http.do 发送外部请求并拒绝非 2xx 响应。
func (c *Client) HTTPDo(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	method := strings.TrimSpace(req.Method)
	if method == "" || strings.TrimSpace(req.URL) == "" {
		return HTTPResponse{}, fmt.Errorf("%w: method and url are required", ErrInvalidRequest)
	}
	req.Method = method
	resp, err := c.callbacks.HTTPDo(ctx, req)
	if err != nil {
		return HTTPResponse{}, fmt.Errorf("host.http.do %s %s: %w", method, redactURL(req.URL), err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, fmt.Errorf("host.http.do %s %s: %w", method, redactURL(req.URL), &HTTPStatusError{StatusCode: resp.StatusCode, Body: RedactBytes(resp.Body)})
	}
	return resp, nil
}

// RedactHTTPRequest 返回可安全记录的宿主管理 HTTP 请求副本。
func RedactHTTPRequest(req HTTPRequest) RecordedHTTPRequest {
	return RecordedHTTPRequest{
		Method:  req.Method,
		URL:     redactURL(req.URL),
		Headers: redactHeaders(req.Headers),
		Body:    RedactBytes(req.Body),
	}
}

// String 为脱敏请求记录实现 fmt.Stringer。
func (r RecordedHTTPRequest) String() string {
	encoded, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("%s %s", r.Method, r.URL)
	}
	return string(encoded)
}

// RedactBytes 从 JSON 或文本字节中移除已知敏感字段。
func RedactBytes(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	redacted, ok := redactJSON(trimmed)
	if ok {
		return string(redacted)
	}
	return redactText(string(raw))
}

type patchPriorityRequest struct {
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

type patchDisabledRequest struct {
	Name     string `json:"name"`
	Disabled bool   `json:"disabled"`
}

func stableName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	return trimmed, nil
}

func jsonHeaders() Header {
	return Header{
		"Accept":       []string{"application/json"},
		"Content-Type": []string{"application/json"},
	}
}

func redactHeaders(headers Header) Header {
	if len(headers) == 0 {
		return nil
	}
	redacted := make(Header, len(headers))
	for key, values := range headers {
		copied := append([]string(nil), values...)
		if sensitiveKey(key) {
			for i := range copied {
				copied[i] = RedactedValue
			}
		}
		redacted[key] = copied
	}
	return redacted
}

func redactJSON(raw []byte) ([]byte, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		redacted := make(map[string]json.RawMessage, len(object))
		for key, value := range object {
			if sensitiveKey(key) {
				redacted[key] = json.RawMessage(`"[REDACTED]"`)
				continue
			}
			if child, ok := redactJSON(bytes.TrimSpace(value)); ok {
				redacted[key] = child
				continue
			}
			redacted[key] = append(json.RawMessage(nil), value...)
		}
		encoded, err := json.Marshal(redacted)
		return encoded, err == nil
	}

	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		redacted := make([]json.RawMessage, len(array))
		for i, value := range array {
			if child, ok := redactJSON(bytes.TrimSpace(value)); ok {
				redacted[i] = child
				continue
			}
			redacted[i] = append(json.RawMessage(nil), value...)
		}
		encoded, err := json.Marshal(redacted)
		return encoded, err == nil
	}
	return nil, false
}

func redactText(value string) string {
	redacted := bearerTextPattern.ReplaceAllString(value, "${1}"+RedactedValue)
	return sensitiveTextPattern.ReplaceAllString(redacted, "${1}${2}"+RedactedValue)
}

func redactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.RawQuery == "" {
		return rawURL
	}
	query := parsed.Query()
	for key := range query {
		if sensitiveKey(key) {
			query.Set(key, RedactedValue)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	return normalized == "authorization" ||
		normalized == "cookie" ||
		normalized == "set_cookie" ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "api_key") ||
		strings.Contains(normalized, "apikey")
}

var _ API = (*Client)(nil)
