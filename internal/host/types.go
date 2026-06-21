package host

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// ManagementAuthFieldsPath 是 CPA 更新凭证优先级的管理接口路径。
	ManagementAuthFieldsPath = "/v0/management/auth-files/fields"
	// ManagementAuthStatusPath 是 CPA 更新凭证禁用状态的管理接口路径。
	ManagementAuthStatusPath = "/v0/management/auth-files/status"
	// RedactedValue 是记录中敏感 header 和 JSON 字段的替代值。
	RedactedValue = "[REDACTED]"
)

// ErrInvalidRequest 表示请求到达 CPA 前已被宿主适配层判定为非法。
var ErrInvalidRequest = errors.New("host: invalid request")

// Header 是宿主管理 HTTP 调用使用的轻量 header 映射。
type Header map[string][]string

// AuthFile 是 host.auth.list 返回的最小凭证记录。
type AuthFile struct {
	Name        string        `json:"name"`
	AuthIndex   string        `json:"auth_index"`
	Type        string        `json:"type,omitempty"`
	Provider    string        `json:"provider,omitempty"`
	Status      string        `json:"status,omitempty"`
	Disabled    bool          `json:"disabled"`
	Unavailable bool          `json:"unavailable"`
	Priority    int           `json:"priority"`
	Email       string        `json:"email,omitempty"`
	IDToken     IDTokenClaims `json:"id_token,omitempty"`
}

// IDTokenClaims 包含优先级规划需要的非敏感 ID token claims。
type IDTokenClaims struct {
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	PlanType         string `json:"plan_type,omitempty"`
}

// RuntimeAuth 是 host.auth.get_runtime 返回的最小运行时凭证视图。
type RuntimeAuth struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name"`
	Provider  string          `json:"provider,omitempty"`
	Disabled  bool            `json:"disabled"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// HTTPRequest 是替代生产路径 http.Client 的宿主管理请求类型。
type HTTPRequest struct {
	Method  string `json:"Method"`
	URL     string `json:"URL"`
	Headers Header `json:"Headers,omitempty"`
	Body    []byte `json:"Body,omitempty"`
}

// HTTPResponse 是 HTTPDo 返回的宿主管理响应类型。
type HTTPResponse struct {
	StatusCode int    `json:"StatusCode"`
	Headers    Header `json:"Headers,omitempty"`
	Body       []byte `json:"Body,omitempty"`
}

// RecordedHTTPRequest 是 HTTPDo 调用的脱敏审计视图。
type RecordedHTTPRequest struct {
	Method  string `json:"method"`
	URL     string `json:"url"`
	Headers Header `json:"headers,omitempty"`
	Body    string `json:"body,omitempty"`
}

// UnmarshalJSON 兼容官方 StatusCode/Headers/Body(base64) 和历史 status_code/body 形态。
func (r *HTTPResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		StatusCode      *int    `json:"StatusCode"`
		StatusCodeSnake *int    `json:"status_code"`
		Headers         Header  `json:"Headers"`
		HeadersLower    Header  `json:"headers"`
		Body            *string `json:"Body"`
		BodyLower       *string `json:"body"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.StatusCode != nil {
		r.StatusCode = *raw.StatusCode
	} else if raw.StatusCodeSnake != nil {
		r.StatusCode = *raw.StatusCodeSnake
	}
	if raw.Headers != nil {
		r.Headers = raw.Headers
	} else {
		r.Headers = raw.HeadersLower
	}
	body, err := decodeBodyString(raw.Body, raw.BodyLower)
	if err != nil {
		return err
	}
	r.Body = body
	return nil
}

func decodeBodyString(official *string, legacy *string) ([]byte, error) {
	if official != nil {
		if *official == "" {
			return nil, nil
		}
		decoded, err := base64.StdEncoding.DecodeString(*official)
		if err != nil {
			return nil, fmt.Errorf("decode Body base64: %w", err)
		}
		return decoded, nil
	}
	if legacy == nil {
		return nil, nil
	}
	if *legacy == "" {
		return nil, nil
	}
	if looksLikeBase64(*legacy) {
		if decoded, err := base64.StdEncoding.DecodeString(*legacy); err == nil {
			return decoded, nil
		}
	}
	return []byte(*legacy), nil
}

func looksLikeBase64(value string) bool {
	trimmed := strings.TrimSpace(value)
	return len(trimmed)%4 == 0 && !strings.ContainsAny(trimmed, "{}<>\n\r")
}

// HostCallbacks 是本包依赖的最小 CPA 宿主回调面。
type HostCallbacks interface {
	ListAuthFiles(ctx context.Context) ([]AuthFile, error)
	GetRuntime(ctx context.Context, authIndex string) (RuntimeAuth, error)
	SaveAuth(ctx context.Context, name string, doc json.RawMessage) error
	HTTPDo(ctx context.Context, req HTTPRequest) (HTTPResponse, error)
}

// API 是后续 credential-priority 包依赖的稳定宿主适配接口。
type API interface {
	ListAuthFiles(ctx context.Context) ([]AuthFile, error)
	GetRuntime(ctx context.Context, authIndex string) (RuntimeAuth, error)
	SaveAuth(ctx context.Context, name string, doc json.RawMessage) error
	PatchPriority(ctx context.Context, name string, priority int) error
	PatchDisabled(ctx context.Context, name string, disabled bool) error
	HTTPDo(ctx context.Context, req HTTPRequest) (HTTPResponse, error)
}
