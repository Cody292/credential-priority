package xai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/host"
)

const (
	// DefaultTokenEndpoint 是 xAI OAuth token 端点。
	DefaultTokenEndpoint = "https://auth.x.ai/oauth2/token"
	// DefaultClientID 是 xAI Grok CLI 默认 client_id。
	DefaultClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	// RefreshThrottle 是同一凭证自动 refresh 的最小间隔（与 expires_in≈21600 对齐）。
	RefreshThrottle = 6 * time.Hour
	// ExpirySkew 是 access_token 临期判定窗口。
	ExpirySkew = 10 * time.Minute
)

// RefreshHTTPDoer 是 OAuth refresh 所需的最小宿主 HTTP 面（保留非 2xx 响应体）。
type RefreshHTTPDoer interface {
	HTTPDoRaw(ctx context.Context, req host.HTTPRequest) (host.HTTPResponse, error)
}

// AuthRefreshFields 是从凭证 JSON 解析出的 refresh 相关字段。
type AuthRefreshFields struct {
	AccessToken    string
	RefreshToken   string
	ClientID       string
	TokenEndpoint  string
	ExpiresIn      int64
	ExpiredAt      time.Time
	LastRefreshAt  time.Time
	HasLastRefresh bool
	Disabled       bool
	Raw            map[string]json.RawMessage
}

// RefreshDecision 描述是否应执行 refresh。
type RefreshDecision struct {
	Need      bool
	Reason    string
	Throttled bool
}

// ParseAuthRefreshFields 从物理凭证 JSON 解析 refresh 元数据。
func ParseAuthRefreshFields(raw json.RawMessage) (AuthRefreshFields, error) {
	if len(raw) == 0 {
		return AuthRefreshFields{}, errors.New("empty auth json")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return AuthRefreshFields{}, fmt.Errorf("parse auth json: %w", err)
	}
	fields := AuthRefreshFields{Raw: object}
	fields.AccessToken = firstStringField(object, "access_token", "oauth_access_token")
	fields.RefreshToken = firstStringField(object, "refresh_token", "oauth_refresh_token")
	fields.ClientID = firstStringField(object, "client_id", "oauth_client_id")
	fields.TokenEndpoint = firstStringField(object, "token_endpoint", "oauth_token_endpoint")
	if fields.ClientID == "" {
		fields.ClientID = DefaultClientID
	}
	if fields.TokenEndpoint == "" {
		fields.TokenEndpoint = DefaultTokenEndpoint
	}
	if v, ok := object["disabled"]; ok {
		var disabled bool
		if err := json.Unmarshal(v, &disabled); err == nil {
			fields.Disabled = disabled
		}
	}
	if v, ok := object["expires_in"]; ok {
		fields.ExpiresIn = parseFlexibleInt64(v)
	}
	if t, ok := parseTimeField(object, "expired"); ok {
		fields.ExpiredAt = t
	} else if t, ok := parseTimeField(object, "expires_at"); ok {
		fields.ExpiredAt = t
	}
	if t, ok := parseTimeField(object, "last_refresh"); ok {
		fields.LastRefreshAt = t
		fields.HasLastRefresh = true
	}
	return fields, nil
}

// DecideRefresh 判断是否需要 refresh。
// 探测前：disabled 或 token 过期/临期(<10min)，且 last_refresh≥6h。
// force（401 重试）：有 refresh_token 即尝试，绕过节流与表面健康判定。
func DecideRefresh(fields AuthRefreshFields, now time.Time, force bool) RefreshDecision {
	now = now.UTC()
	if strings.TrimSpace(fields.RefreshToken) == "" {
		return RefreshDecision{Need: false, Reason: "missing_refresh_token"}
	}
	tokenStale := isTokenExpiredOrNear(fields, now)
	if force {
		return RefreshDecision{Need: true, Reason: "force_401"}
	}
	need := fields.Disabled || tokenStale
	if !need {
		return RefreshDecision{Need: false, Reason: "healthy"}
	}
	if fields.HasLastRefresh && !fields.LastRefreshAt.IsZero() {
		if now.Sub(fields.LastRefreshAt) < RefreshThrottle {
			return RefreshDecision{Need: false, Reason: "throttled", Throttled: true}
		}
	}
	reason := "disabled"
	if tokenStale && !fields.Disabled {
		reason = "token_expiring"
	} else if tokenStale && fields.Disabled {
		reason = "disabled_or_expiring"
	}
	return RefreshDecision{Need: true, Reason: reason}
}

func isTokenExpiredOrNear(fields AuthRefreshFields, now time.Time) bool {
	if fields.ExpiredAt.IsZero() {
		return false
	}
	return !fields.ExpiredAt.After(now.Add(ExpirySkew))
}

// TokenRefreshResult 是 OAuth token 端点成功响应的可写回字段。
type TokenRefreshResult struct {
	AccessToken  string
	RefreshToken string
	IDToken      json.RawMessage
	TokenType    json.RawMessage
	ExpiresIn    json.RawMessage
	ExpiresInSec int64
}

// ExecuteRefresh 对 token_endpoint 发起 refresh_token grant（form-urlencoded）。
func ExecuteRefresh(ctx context.Context, doer RefreshHTTPDoer, fields AuthRefreshFields) (TokenRefreshResult, error) {
	if doer == nil {
		return TokenRefreshResult{}, errors.New("refresh http doer is nil")
	}
	refreshToken := strings.TrimSpace(fields.RefreshToken)
	if refreshToken == "" {
		return TokenRefreshResult{}, errors.New("missing_refresh_token")
	}
	endpoint := strings.TrimSpace(fields.TokenEndpoint)
	if endpoint == "" {
		endpoint = DefaultTokenEndpoint
	}
	clientID := strings.TrimSpace(fields.ClientID)
	if clientID == "" {
		clientID = DefaultClientID
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)

	resp, err := doer.HTTPDoRaw(ctx, host.HTTPRequest{
		Method: http.MethodPost,
		URL:    endpoint,
		Headers: host.Header{
			"Accept":       []string{"application/json"},
			"Content-Type": []string{"application/x-www-form-urlencoded"},
		},
		Body: []byte(form.Encode()),
	})
	if err != nil {
		return TokenRefreshResult{}, fmt.Errorf("refresh network: %s", sanitizeRefreshErr(err.Error()))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return TokenRefreshResult{}, fmt.Errorf("refresh HTTP %d: %s", resp.StatusCode, sanitizeRefreshBody(resp.Body))
	}
	return parseTokenRefreshBody(resp.Body)
}

func parseTokenRefreshBody(body []byte) (TokenRefreshResult, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return TokenRefreshResult{}, errors.New("refresh invalid token json")
	}
	access := stringField(payload, "access_token")
	if access == "" {
		return TokenRefreshResult{}, errors.New("refresh response missing access_token")
	}
	result := TokenRefreshResult{
		AccessToken:  access,
		RefreshToken: stringField(payload, "refresh_token"),
	}
	if v, ok := payload["id_token"]; ok {
		result.IDToken = append(json.RawMessage(nil), v...)
	}
	if v, ok := payload["token_type"]; ok {
		result.TokenType = append(json.RawMessage(nil), v...)
	}
	if v, ok := payload["expires_in"]; ok {
		result.ExpiresIn = append(json.RawMessage(nil), v...)
		result.ExpiresInSec = parseFlexibleInt64(v)
	}
	return result, nil
}

// MergeRefreshIntoAuth 将 refresh 结果合并进原始 auth 对象 JSON，保留未知字段。
func MergeRefreshIntoAuth(raw json.RawMessage, result TokenRefreshResult, now time.Time) (json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("merge auth json: %w", err)
	}
	if object == nil {
		object = make(map[string]json.RawMessage)
	}
	now = now.UTC()
	accessEnc, err := json.Marshal(result.AccessToken)
	if err != nil {
		return nil, err
	}
	object["access_token"] = accessEnc
	if _, ok := object["oauth_access_token"]; ok {
		object["oauth_access_token"] = accessEnc
	}
	if rt := strings.TrimSpace(result.RefreshToken); rt != "" {
		rtEnc, err := json.Marshal(rt)
		if err != nil {
			return nil, err
		}
		object["refresh_token"] = rtEnc
		if _, ok := object["oauth_refresh_token"]; ok {
			object["oauth_refresh_token"] = rtEnc
		}
	}
	if len(result.IDToken) > 0 {
		object["id_token"] = append(json.RawMessage(nil), result.IDToken...)
	}
	if len(result.TokenType) > 0 {
		object["token_type"] = append(json.RawMessage(nil), result.TokenType...)
	}
	var expiresAt time.Time
	if len(result.ExpiresIn) > 0 {
		object["expires_in"] = append(json.RawMessage(nil), result.ExpiresIn...)
		if secs := result.ExpiresInSec; secs > 0 {
			expiresAt = now.Add(time.Duration(secs) * time.Second)
		}
	}
	if !expiresAt.IsZero() {
		expiredEnc, err := json.Marshal(expiresAt.Format(time.RFC3339))
		if err != nil {
			return nil, err
		}
		object["expired"] = expiredEnc
		expiresAtEnc, err := json.Marshal(expiresAt.Unix())
		if err != nil {
			return nil, err
		}
		object["expires_at"] = expiresAtEnc
	}
	lastEnc, err := json.Marshal(now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	object["last_refresh"] = lastEnc
	merged, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("encode merged auth: %w", err)
	}
	return merged, nil
}

// RefreshAndMerge 执行 refresh 并返回合并后的 auth JSON 与新 access_token。
func RefreshAndMerge(ctx context.Context, doer RefreshHTTPDoer, raw json.RawMessage, now time.Time) (merged json.RawMessage, accessToken string, err error) {
	fields, err := ParseAuthRefreshFields(raw)
	if err != nil {
		return nil, "", err
	}
	token, err := ExecuteRefresh(ctx, doer, fields)
	if err != nil {
		return nil, "", err
	}
	merged, err = MergeRefreshIntoAuth(raw, token, now)
	if err != nil {
		return nil, "", err
	}
	return merged, token.AccessToken, nil
}

// IsUnauthorizedProbe 判断 xAI 探测/plan/usage 是否为鉴权失效类失败。
// 覆盖 HTTP 401、CPA 文案（invalid or expired credentials / no auth context / PermissionDenied）等。
// 纯网络失败、404、普通 5xx 不得误判为鉴权失效。
func IsUnauthorizedProbe(httpStatus int, errMsg string) bool {
	if httpStatus == http.StatusUnauthorized {
		return true
	}
	return looksLikeAuthFailureText(errMsg)
}

// looksLikeAuthFailureText 从错误串/响应体识别鉴权失效（不依赖 HTTP status 字段）。
func looksLikeAuthFailureText(msg string) bool {
	lower := strings.ToLower(strings.TrimSpace(msg))
	if lower == "" {
		return false
	}
	markers := []string{
		"status 401",
		"http 401",
		"unauthorized",
		"unauthenticated",
		"invalid or expired credentials",
		"expired credentials",
		"no auth context",
		"permissiondenied",
		"permission denied",
		"auth context",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			// "auth context" 仅在 no/missing 语境下视为失效，避免误伤普通文案
			if m == "auth context" && !strings.Contains(lower, "no auth") && !strings.Contains(lower, "missing auth") {
				continue
			}
			return true
		}
	}
	return false
}

// AccessTokenExpired 解析 access token JWT exp；无法解析则返回 false（不臆测失效）。
func AccessTokenExpired(accessToken string, now time.Time) bool {
	exp, ok := jwtExpClaim(accessToken)
	if !ok {
		return false
	}
	return !exp.After(now.UTC())
}

// jwtExpClaim 从 JWT payload 读取 exp（不校验签名）。
func jwtExpClaim(accessToken string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(accessToken), ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := decodeJWTPayload(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	raw, ok := claims["exp"]
	if !ok {
		return time.Time{}, false
	}
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(v), 0).UTC(), true
	case json.Number:
		n, err := v.Int64()
		if err != nil || n <= 0 {
			return time.Time{}, false
		}
		return time.Unix(n, 0).UTC(), true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || n <= 0 {
			return time.Time{}, false
		}
		return time.Unix(n, 0).UTC(), true
	default:
		return time.Time{}, false
	}
}

func decodeJWTPayload(segment string) ([]byte, error) {
	payload, err := base64.RawURLEncoding.DecodeString(segment)
	if err == nil {
		return payload, nil
	}
	return base64.StdEncoding.DecodeString(segment)
}

// AuthMaterialExpired 综合 JWT exp 与 auth JSON expired 字段判断本地凭证是否已过期。
func AuthMaterialExpired(accessToken string, expiredAt time.Time, now time.Time) bool {
	now = now.UTC()
	if !expiredAt.IsZero() && !expiredAt.After(now) {
		return true
	}
	return AccessTokenExpired(accessToken, now)
}
