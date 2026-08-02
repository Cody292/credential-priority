package xai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/core"
	"credential-priority/internal/host"
)

// PlanClass 是 xAI 套餐粗分类（free / paid），独立于业务额度冷却。
type PlanClass string

const (
	// PlanClassUnknown 表示尚未分类。
	PlanClassUnknown PlanClass = ""
	// PlanClassFree 表示 free / grokfree / none / 空 / 无法获取额度信息。
	PlanClassFree PlanClass = "free"
	// PlanClassPaid 表示 supergrok / pro / heavy / lite 等付费套餐。
	PlanClassPaid PlanClass = "paid"
)

// PlanResult 是一次轻量套餐分类结果（无 chat/completions）。
type PlanResult struct {
	Provider    core.Provider
	AuthIndex   string
	ObservedAt  time.Time
	PlanClass   PlanClass
	PlanType    core.PlanType
	Source      string
	Error       string
	HTTPStatus  int  // 最近一次 settings/billing 非 2xx 状态；0=无或网络失败
	AuthFailed  bool // 鉴权失效（401/凭证文案）；true 时不得标 free 正额度
}

// PlanRequest 是 FetchPlan 所需的宿主凭证上下文。
type PlanRequest struct {
	AuthIndex   string
	AccessToken string
	BaseURL     string
}

// FetchPlan 通过 settings / billing / JWT tier 分类套餐，禁止 chat 多模型 probe。
// 网络/404 等 unfetchable 仍标 Free；HTTP 401 / 凭证失效文案标 AuthFailed，禁止假 free 正额度。
func (p Prober) FetchPlan(ctx context.Context, request PlanRequest) PlanResult {
	observedAt := p.clock.Now().UTC()
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	headers := planHeaders(request)

	var settingsBody []byte
	var billingBody []byte
	var lastErr string
	var lastStatus int
	var authFailed bool
	var authErr string
	for _, url := range planCandidateURLs(baseURL, "settings") {
		resp, err := p.host.HTTPDoRaw(ctx, host.HTTPRequest{
			AuthIndex: request.AuthIndex,
			Method:    http.MethodGet,
			URL:       url,
			Headers:   headers,
		})
		if err != nil {
			lastErr = "settings request failed"
			continue
		}
		lastStatus = resp.StatusCode
		bodyText := string(resp.Body)
		if IsUnauthorizedProbe(resp.StatusCode, bodyText) {
			authFailed = true
			authErr = fmt.Sprintf("settings status %d", resp.StatusCode)
			if looksLikeAuthFailureText(bodyText) {
				authErr = safeError(bodyText)
			}
			// 401 继续试其它 URL 无意义，直接 break 候选
			break
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && len(resp.Body) > 0 {
			settingsBody = resp.Body
			break
		}
		lastErr = fmt.Sprintf("settings status %d", resp.StatusCode)
	}
	if !authFailed {
		for _, url := range planCandidateURLs(baseURL, "billing?format=credits") {
			resp, err := p.host.HTTPDoRaw(ctx, host.HTTPRequest{
				AuthIndex: request.AuthIndex,
				Method:    http.MethodGet,
				URL:       url,
				Headers:   headers,
			})
			if err != nil {
				if lastErr == "" {
					lastErr = "billing request failed"
				}
				continue
			}
			lastStatus = resp.StatusCode
			bodyText := string(resp.Body)
			if IsUnauthorizedProbe(resp.StatusCode, bodyText) {
				authFailed = true
				authErr = fmt.Sprintf("billing status %d", resp.StatusCode)
				if looksLikeAuthFailureText(bodyText) {
					authErr = safeError(bodyText)
				}
				break
			}
			if resp.StatusCode >= 200 && resp.StatusCode < 300 && len(resp.Body) > 0 {
				billingBody = resp.Body
				break
			}
			if lastErr == "" {
				lastErr = fmt.Sprintf("billing status %d", resp.StatusCode)
			}
		}
	}

	result := PlanResult{
		Provider:   core.ProviderXAI,
		AuthIndex:  request.AuthIndex,
		ObservedAt: observedAt,
		HTTPStatus: lastStatus,
	}
	if authFailed {
		// 鉴权失效：不得 ClassifyPlan → free 正额度
		result.AuthFailed = true
		result.PlanClass = PlanClassUnknown
		result.PlanType = core.PlanTypeUnknown
		result.Source = "auth_failed"
		if authErr != "" {
			result.Error = safeError(authErr)
		} else {
			result.Error = "unauthorized"
		}
		if result.HTTPStatus == 0 {
			result.HTTPStatus = http.StatusUnauthorized
		}
		return result
	}

	jwtTier := jwtTierClaim(request.AccessToken)
	class, source := ClassifyPlan(settingsBody, billingBody, jwtTier)
	result.PlanClass = class
	result.PlanType = planTypeFromClass(class)
	result.Source = source
	if lastErr != "" {
		result.Error = safeError(lastErr)
	}
	return result
}

// ClassifyPlan 从 settings/billing JSON 与 JWT tier 判定 free/paid。
// 规则：明确付费 product/tier → Paid；free/grokfree/none/空 → Free；无法获取 → Free。
func ClassifyPlan(settingsBody, billingBody []byte, jwtTier string) (PlanClass, string) {
	if class, ok := classifyTierToken(jwtTier); ok {
		if class == PlanClassPaid {
			return PlanClassPaid, "jwt_tier"
		}
		// free/none 仍继续看 body，避免 JWT 空壳覆盖明确付费 product
	}
	if class, source, ok := classifyJSONBodies(settingsBody, "settings"); ok {
		return class, source
	}
	if class, source, ok := classifyJSONBodies(billingBody, "billing"); ok {
		return class, source
	}
	if class, ok := classifyTierToken(jwtTier); ok {
		return class, "jwt_tier"
	}
	// 无法获取额度/套餐信息 → Free（用户拍板）
	return PlanClassFree, "default_unfetchable"
}

func classifyJSONBodies(body []byte, source string) (PlanClass, string, bool) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return "", "", false
	}
	blob := strings.ToLower(string(body))
	// 明确付费 product / tier 优先
	if hasPaidPlanSignal(blob) {
		return PlanClassPaid, source, true
	}
	if hasFreePlanSignal(blob) {
		return PlanClassFree, source, true
	}
	// 可解析但无套餐字段：不据此强制 free，留给后续源
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", false
	}
	if tier := extractTierLike(payload); tier != "" {
		if class, ok := classifyTierToken(tier); ok {
			return class, source, true
		}
	}
	return "", "", false
}

func extractTierLike(payload map[string]any) string {
	keys := []string{"tier", "plan", "plan_type", "planType", "subscription", "product", "sku", "name"}
	for _, key := range keys {
		if v, ok := payload[key]; ok {
			if s := anyToTierString(v); s != "" {
				return s
			}
			if nested, ok := v.(map[string]any); ok {
				if s := extractTierLike(nested); s != "" {
					return s
				}
			}
		}
	}
	// nested user / subscription / product
	for _, key := range []string{"user", "subscription", "product", "account", "data"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if s := extractTierLike(nested); s != "" {
				return s
			}
		}
	}
	return ""
}

// anyToTierString 将 JWT/JSON 中的 tier 值规范为字符串。
// 现网 SuperGrok 发 `"tier": 1`（数字）；encoding/json 默认解成 float64。
func anyToTierString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	case float64:
		// JSON number → 整数字符串（1.0 → "1"），避免 "1.000000"
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		f := float64(t)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 32)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	default:
		return ""
	}
}

func classifyTierToken(tier string) (PlanClass, bool) {
	t := strings.ToLower(strings.TrimSpace(tier))
	if t == "" {
		// Empty is not an explicit tier; caller falls through to default_unfetchable=free.
		return "", false
	}
	// 数字 tier：≥1 → Paid（SuperGrok 等）；0 → Free
	if n, err := strconv.ParseInt(t, 10, 64); err == nil {
		if n >= 1 {
			return PlanClassPaid, true
		}
		if n == 0 {
			return PlanClassFree, true
		}
		// 负数不视为合法套餐信号
		return "", false
	}
	if hasPaidPlanSignal(t) {
		return PlanClassPaid, true
	}
	if hasFreePlanSignal(t) {
		return PlanClassFree, true
	}
	return "", false
}

func hasPaidPlanSignal(blob string) bool {
	// supergrok / pro / heavy / lite 及常见付费 product
	paid := []string{
		"supergrok", "super-grok", "super_grok",
		"\"pro\"", "pro-", "-pro", "plan_pro", "plan-pro", "tier_pro", "tier-pro", "tier\":\"pro", "tier': 'pro",
		"heavy", "lite",
		"plus", "team", "premium", "enterprise",
		"paid", "subscription_active",
	}
	// 避免 "product" 误伤；用词边界近似
	lower := strings.ToLower(blob)
	for _, p := range paid {
		if strings.Contains(lower, p) {
			// "lite" 可能出现在无关词；要求邻近 plan/tier/product/sku/name
			if p == "lite" || p == "plus" || p == "pro-" || p == "-pro" || p == "\"pro\"" {
				if !paidContextOK(lower, p) {
					continue
				}
			}
			return true
		}
	}
	// 单独 token 精确匹配
	switch strings.Trim(lower, "\"' \t\n") {
	case "pro", "lite", "heavy", "plus", "team", "premium":
		return true
	}
	return false
}

func paidContextOK(blob, needle string) bool {
	idx := strings.Index(blob, needle)
	if idx < 0 {
		return false
	}
	start := idx - 24
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 24
	if end > len(blob) {
		end = len(blob)
	}
	window := blob[start:end]
	for _, ctx := range []string{"tier", "plan", "product", "sku", "subscription", "package"} {
		if strings.Contains(window, ctx) {
			return true
		}
	}
	// 整个 blob 很短（纯 tier 值）时放行
	return len(strings.TrimSpace(blob)) <= 32
}

func hasFreePlanSignal(blob string) bool {
	lower := strings.ToLower(blob)
	free := []string{"grokfree", "grok-free", "grok_free", "free_tier", "freetier", "plan_free", "plan-free", "tier_free", "tier-free", "tier\":\"free", "\"free\"", "none"}
	for _, f := range free {
		if strings.Contains(lower, f) {
			return true
		}
	}
	switch strings.Trim(lower, "\"' \t\n") {
	case "free", "none", "null", "":
		return true
	}
	return false
}

func planTypeFromClass(class PlanClass) core.PlanType {
	switch class {
	case PlanClassFree:
		return core.PlanTypeFree
	case PlanClassPaid:
		return core.PlanTypePlus
	default:
		return core.PlanTypeUnknown
	}
}

func planCandidateURLs(baseURL, suffix string) []string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	suffix = strings.TrimLeft(suffix, "/")
	out := make([]string, 0, 4)
	seen := map[string]struct{}{}
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	add(base + "/" + suffix)
	// api.x.ai/v1 → api.x.ai
	if strings.HasSuffix(base, "/v1") {
		add(strings.TrimSuffix(base, "/v1") + "/" + suffix)
	}
	// 常见 console / management 根
	if strings.Contains(base, "api.x.ai") {
		root := strings.Replace(base, "api.x.ai", "management-api.x.ai", 1)
		add(strings.TrimSuffix(root, "/v1") + "/" + suffix)
	}
	return out
}

func planHeaders(request PlanRequest) host.Header {
	token := "$TOKEN$"
	if accessToken := strings.TrimSpace(request.AccessToken); accessToken != "" {
		token = accessToken
	}
	return host.Header{
		"Accept":               []string{"application/json"},
		"Authorization":        []string{"Bearer " + token},
		"User-Agent":           []string{"credential-priority/xai-plan"},
		"X-XAI-Token-Auth":     []string{"xai-grok-cli"},
		"x-grok-client-version": []string{"0.2.93"},
	}
}

// jwtTierClaim 从 access token payload 提取 tier/plan（不校验签名，仅分类）。
// 支持 string / float64 / json.Number / int 等数字 tier（现网 SuperGrok `"tier":1`）。
func jwtTierClaim(accessToken string) string {
	parts := strings.Split(strings.TrimSpace(accessToken), ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// 兼容标准 base64
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	for _, key := range []string{"tier", "plan", "plan_type", "subscription_tier", "xai_tier"} {
		if s := anyToTierString(claims[key]); s != "" {
			return s
		}
	}
	return ""
}
