package host

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

// GetAuth 通过 host.auth.get 读取物理凭证 JSON。
func (c *Client) GetAuth(ctx context.Context, authIndex string) (AuthDocument, error) {
	document, err := c.callbacks.GetAuth(ctx, authIndex)
	if err != nil {
		return AuthDocument{}, fmt.Errorf("host.auth.get: %w", err)
	}
	return document, nil
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

// PatchPriority 通过 host.auth.get 暴露的物理凭证路径仅更新 priority 字段。
func (c *Client) PatchPriority(ctx context.Context, authIndex string, priority int) error {
	trimmed, err := stableName(authIndex)
	if err != nil {
		return err
	}
	document, err := c.GetAuth(ctx, trimmed)
	if err != nil {
		return err
	}
	err = patchPriorityDocument(ctx, document, priority)
	if err != nil {
		return fmt.Errorf("patch priority: %w", err)
	}
	return nil
}

// PatchDisabled 通过 host.auth.get 暴露的物理凭证路径仅更新 disabled 字段。
func (c *Client) PatchDisabled(ctx context.Context, name string, disabled bool) error {
	trimmed, err := stableName(name)
	if err != nil {
		return err
	}
	document, err := c.authDocumentByName(ctx, trimmed)
	if err != nil {
		return err
	}
	if err := patchDisabledDocument(ctx, document, disabled); err != nil {
		return fmt.Errorf("patch disabled: %w", err)
	}
	return nil
}

func (c *Client) authDocumentByName(ctx context.Context, name string) (AuthDocument, error) {
	files, err := c.ListAuthFiles(ctx)
	if err != nil {
		return AuthDocument{}, err
	}
	for _, file := range files {
		if file.Name == name || file.AuthIndex == name {
			return c.GetAuth(ctx, file.AuthIndex)
		}
	}
	return AuthDocument{}, fmt.Errorf("%w: auth document not found", ErrInvalidRequest)
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
		AuthIndex: req.AuthIndex,
		Method:    req.Method,
		URL:       redactURL(req.URL),
		Headers:   redactHeaders(req.Headers),
		Body:      RedactBytes(req.Body),
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

func patchPriorityDocument(ctx context.Context, document AuthDocument, priority int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := strings.TrimSpace(document.Path)
	if path == "" {
		return fmt.Errorf("%w: auth document path is required", ErrInvalidRequest)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read auth document: %w", err)
	}
	patched, err := patchPriorityBytes(raw, priority)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(ctx, path, patched); err != nil {
		return err
	}
	return ctx.Err()
}

func patchPriorityBytes(raw []byte, priority int) ([]byte, error) {
	encodedPriority, err := json.Marshal(priority)
	if err != nil {
		return nil, fmt.Errorf("encode priority: %w", err)
	}
	return patchTopLevelFieldBytes(raw, "priority", encodedPriority)
}

func patchDisabledDocument(ctx context.Context, document AuthDocument, disabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := strings.TrimSpace(document.Path)
	if path == "" {
		return fmt.Errorf("%w: auth document path is required", ErrInvalidRequest)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read auth document: %w", err)
	}
	patched, err := patchDisabledBytes(raw, disabled)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(ctx, path, patched); err != nil {
		return err
	}
	return ctx.Err()
}

func patchDisabledBytes(raw []byte, disabled bool) ([]byte, error) {
	encodedDisabled, err := json.Marshal(disabled)
	if err != nil {
		return nil, fmt.Errorf("encode disabled: %w", err)
	}
	return patchTopLevelFieldBytes(raw, "disabled", encodedDisabled)
}

func patchTopLevelFieldBytes(raw []byte, field string, encodedValue []byte) ([]byte, error) {
	rangeStart, rangeEnd, found, err := topLevelFieldValueRange(raw, field)
	if err != nil {
		return nil, err
	}
	if found {
		patched := make([]byte, 0, len(raw)-rangeEnd+rangeStart+len(encodedValue))
		patched = append(patched, raw[:rangeStart]...)
		patched = append(patched, encodedValue...)
		patched = append(patched, raw[rangeEnd:]...)
		return patched, nil
	}
	return appendTopLevelField(raw, field, encodedValue)
}

func topLevelFieldValueRange(raw []byte, field string) (int, int, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return 0, 0, false, fmt.Errorf("decode auth document: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return 0, 0, false, fmt.Errorf("%w: auth document must be a JSON object", ErrInvalidRequest)
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return 0, 0, false, fmt.Errorf("decode auth document key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return 0, 0, false, fmt.Errorf("%w: auth document key must be a string", ErrInvalidRequest)
		}
		valueScanStart := int(decoder.InputOffset())
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return 0, 0, false, fmt.Errorf("decode auth document value: %w", err)
		}
		valueEnd := int(decoder.InputOffset())
		if key != field {
			continue
		}
		valueStart, err := jsonValueStart(raw, valueScanStart, valueEnd)
		if err != nil {
			return 0, 0, false, err
		}
		return valueStart, valueEnd, true, nil
	}
	if _, err := decoder.Token(); err != nil {
		return 0, 0, false, fmt.Errorf("decode auth document close: %w", err)
	}
	return 0, 0, false, nil
}

func jsonValueStart(raw []byte, start int, end int) (int, error) {
	for index := start; index < end; index++ {
		switch raw[index] {
		case ' ', '\n', '\r', '\t':
			continue
		case ':':
			for valueStart := index + 1; valueStart < end; valueStart++ {
				switch raw[valueStart] {
				case ' ', '\n', '\r', '\t':
					continue
				default:
					return valueStart, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("%w: priority value offset not found", ErrInvalidRequest)
}

func appendTopLevelField(raw []byte, field string, value []byte) ([]byte, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%w: auth document contains invalid JSON", ErrInvalidRequest)
	}
	insertAt := len(raw)
	for insertAt > 0 {
		insertAt--
		switch raw[insertAt] {
		case ' ', '\n', '\r', '\t':
			continue
		case '}':
			fieldBytes, err := json.Marshal(field)
			if err != nil {
				return nil, fmt.Errorf("encode priority field: %w", err)
			}
			prefix := []byte{','}
			if objectIsEmpty(raw[:insertAt]) {
				prefix = nil
			}
			patched := make([]byte, 0, len(raw)+len(prefix)+len(fieldBytes)+1+len(value))
			patched = append(patched, raw[:insertAt]...)
			patched = append(patched, prefix...)
			patched = append(patched, fieldBytes...)
			patched = append(patched, ':')
			patched = append(patched, value...)
			patched = append(patched, raw[insertAt:]...)
			return patched, nil
		default:
			return nil, fmt.Errorf("%w: auth document must end with object close", ErrInvalidRequest)
		}
	}
	return nil, fmt.Errorf("%w: auth document object close not found", ErrInvalidRequest)
}

func objectIsEmpty(rawBeforeClose []byte) bool {
	for index := len(rawBeforeClose) - 1; index >= 0; index-- {
		switch rawBeforeClose[index] {
		case ' ', '\n', '\r', '\t':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

func writeFileAtomic(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write auth document context: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat auth document: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write auth document context: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create auth document temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := ctx.Err(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write auth document context: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write auth document temp: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write auth document context: %w", err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod auth document temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close auth document temp: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write auth document context: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace auth document: %w", err)
	}
	cleanup = false
	return nil
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
