package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"credential-priority/internal/apply"
	"credential-priority/internal/config"
	"credential-priority/internal/management"
)

// ManagementRequest 是 CPA management.handle 调用传入的已解析 HTTP 请求信封。
type ManagementRequest struct {
	Method  string `json:"Method"`
	Path    string `json:"Path"`
	Headers http.Header
	Query   url.Values
	Body    []byte `json:"Body"`
}

// ManagementResponse 是 management.handle 返回给 CPA 宿主的 HTTP 结果信封。
type ManagementResponse struct {
	StatusCode  int                 `json:"StatusCode"`
	ContentType string              `json:"content_type"`
	Headers     map[string][]string `json:"Headers"`
	Body        string              `json:"Body"`
}

type managementRoute struct {
	Method string `json:"Method"`
	Path   string `json:"Path"`
}

type managementResource struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type managementRegistration struct {
	Routes    []managementRoute    `json:"routes"`
	Resources []managementResource `json:"resources"`
}

type managementRunner struct {
	runtime *Runtime
}

func (r *Runtime) registerManagement() []byte {
	result := managementRegistration{
		Routes: []managementRoute{
			{Method: http.MethodPost, Path: "/plugins/credential-priority/run"},
			{Method: http.MethodGet, Path: "/plugins/credential-priority/diagnostics"},
			{Method: http.MethodGet, Path: "/plugins/credential-priority/snapshot/latest"},
		},
		Resources: []managementResource{
			{Path: "/status", Menu: "Credential Priority", Description: "Shows credential priority status and audit summary."},
		},
	}
	return envelopeManagement(result, nil)
}

func (r *Runtime) handleManagement(ctx context.Context, raw []byte) []byte {
	request, err := decodeManagementRequest(raw)
	if err != nil {
		return failure(err)
	}
	httpRequest, err := request.toHTTPRequest(ctx)
	if err != nil {
		return failure(err)
	}
	recorder := httptest.NewRecorder()
	r.management.ServeHTTP(recorder, httpRequest)
	return envelopeManagement(newManagementResponse(recorder), nil)
}

func decodeManagementRequest(raw []byte) (ManagementRequest, error) {
	var request managementRequestWire
	if len(raw) == 0 {
		return ManagementRequest{}, fmt.Errorf("%w: management request is required", ErrInvalidRequest)
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return ManagementRequest{}, fmt.Errorf("%w: decode management request: %v", ErrInvalidRequest, err)
	}
	parsed, err := request.toManagementRequest()
	if err != nil {
		return ManagementRequest{}, err
	}
	if parsed.Method == "" || parsed.Path == "" {
		return ManagementRequest{}, fmt.Errorf("%w: management method and path are required", ErrInvalidRequest)
	}
	return parsed, nil
}

type managementRequestWire struct {
	Method      string      `json:"Method"`
	MethodLower string      `json:"method"`
	Path        string      `json:"Path"`
	PathLower   string      `json:"path"`
	Headers     http.Header `json:"Headers"`
	Query       url.Values  `json:"Query"`
	QueryLower  string      `json:"query"`
	Body        string      `json:"Body"`
	BodyLower   string      `json:"body"`
}

func (w managementRequestWire) toManagementRequest() (ManagementRequest, error) {
	method := firstNonEmpty(w.Method, w.MethodLower)
	path := firstNonEmpty(w.Path, w.PathLower)
	body, err := decodeManagementBody(w.Body, w.BodyLower)
	if err != nil {
		return ManagementRequest{}, err
	}
	query := w.Query
	if query == nil && strings.TrimSpace(w.QueryLower) != "" {
		values, err := url.ParseQuery(strings.TrimPrefix(w.QueryLower, "?"))
		if err != nil {
			return ManagementRequest{}, fmt.Errorf("%w: decode management query: %v", ErrInvalidRequest, err)
		}
		query = values
	}
	return ManagementRequest{Method: method, Path: path, Headers: w.Headers, Query: query, Body: body}, nil
}

func decodeManagementBody(official string, legacy string) ([]byte, error) {
	if official != "" {
		decoded, err := base64.StdEncoding.DecodeString(official)
		if err != nil {
			return nil, fmt.Errorf("%w: decode management body: %v", ErrInvalidRequest, err)
		}
		return decoded, nil
	}
	return []byte(legacy), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// routeSourceHeader 标记请求来自 resource 还是 management，供 Handler 做边界校验。
// 仅插件内部使用，不依赖宿主透传。
const routeSourceHeader = "X-Credential-Priority-Route-Source"

func (r ManagementRequest) toHTTPRequest(ctx context.Context) (*http.Request, error) {
	normalized, source := normalizeManagementPath(r.Path)
	if !strings.HasPrefix(normalized, "/") {
		return nil, fmt.Errorf("%w: management path must start with /", ErrInvalidRequest)
	}
	path := normalized
	if r.Query != nil {
		encoded := r.Query.Encode()
		if encoded != "" {
			path += "?" + encoded
		}
	}
	request, err := http.NewRequestWithContext(ctx, r.Method, path, bytes.NewBuffer(r.Body))
	if err != nil {
		return nil, fmt.Errorf("%w: build management request: %v", ErrInvalidRequest, err)
	}
	// 复制宿主头后写入路由来源，避免 resource 前缀被误当成 management 动态 API。
	if r.Headers != nil {
		request.Header = r.Headers.Clone()
	} else {
		request.Header = make(http.Header)
	}
	request.Header.Set(routeSourceHeader, source)
	return request, nil
}

// normalizeManagementPath 将宿主完整路径规约为插件内相对路径，并标明来源。
// resource → 仅允许静态 /status；management/legacy → 动态业务路由。
func normalizeManagementPath(path string) (normalized string, source string) {
	resourcePrefix := "/v0/resource/plugins/" + config.PluginID
	managementPrefix := "/v0/management/plugins/" + config.PluginID
	legacyRoutePrefix := "/plugins/" + config.PluginID

	switch {
	case path == resourcePrefix:
		return "/", "resource"
	case strings.HasPrefix(path, resourcePrefix+"/"):
		return strings.TrimPrefix(path, resourcePrefix), "resource"
	case path == managementPrefix:
		return "/", "management"
	case strings.HasPrefix(path, managementPrefix+"/"):
		return strings.TrimPrefix(path, managementPrefix), "management"
	case path == legacyRoutePrefix:
		return "/", "management"
	case strings.HasPrefix(path, legacyRoutePrefix+"/"):
		return strings.TrimPrefix(path, legacyRoutePrefix), "management"
	default:
		// 未识别前缀按 management 处理，仍由 Handler 白名单拒绝未知路径。
		return path, "management"
	}
}

func newManagementResponse(recorder *httptest.ResponseRecorder) ManagementResponse {
	result := recorder.Result()
	return ManagementResponse{
		StatusCode:  result.StatusCode,
		ContentType: result.Header.Get("Content-Type"),
		Headers:     result.Header,
		Body:        base64.StdEncoding.EncodeToString(recorder.Body.Bytes()),
	}
}

func envelopeManagement(result any, err error) []byte {
	if err != nil {
		return failure(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return failure(fmt.Errorf("encode management result: %w", err))
	}
	return mustMarshal(Envelope{OK: true, Result: encoded})
}

func (r managementRunner) Run(ctx context.Context, request management.RunRequest) (apply.Result, error) {
	if request.Mode == "apply" {
		if err := r.runtime.ManualApplyWithProviderScopeModelGroupAndAuthIndexes(ctx, request.Scope, request.Providers, request.AntigravityModelGroup, request.AuthIndexes); err != nil {
			return apply.Result{}, err
		}
		result, _ := r.runtime.currentRunSnapshot()
		return result, nil
	}
	if err := r.runtime.RunWithProviderScopeAndModelGroup(ctx, request.Scope, request.Providers, request.AntigravityModelGroup); err != nil {
		return apply.Result{}, err
	}
	result, _ := r.runtime.currentRunSnapshot()
	return result, nil
}

func (r managementRunner) Status(ctx context.Context) (management.StatusInfo, error) {
	cfg, err := r.runtime.Config()
	if err != nil {
		return management.StatusInfo{}, err
	}
	latestAudit := "runtime management API ready"
	if !cfg.Enabled {
		latestAudit = "runtime management API disabled by config"
	}
	_, audit := r.runtime.currentRunSnapshot()
	if audit != "" {
		latestAudit = audit
	}
	return management.StatusInfo{LatestAudit: latestAudit}, nil
}

func (r managementRunner) LatestSnapshot(ctx context.Context) (apply.PlanSnapshot, error) {
	if _, err := r.runtime.Config(); err != nil {
		return apply.PlanSnapshot{}, err
	}
	result, _ := r.runtime.currentRunSnapshot()
	return result.Snapshot, nil
}

func (r managementRunner) Diagnostics(ctx context.Context) (map[string]any, error) {
	cfg, err := r.runtime.Config()
	if err != nil {
		return nil, err
	}
	result, audit := r.runtime.currentRunSnapshot()
	r.runtime.mu.Lock()
	lastAuto := r.runtime.lastAutoApplyAt
	workerActive := r.runtime.worker != nil
	r.runtime.mu.Unlock()
	nextWait := r.runtime.nextAutoApplyWait(cfg.Interval)
	return map[string]any{
		"management_api": map[string]any{
			"status":     "ready",
			"auto_apply": cfg.AutoApply,
			"enabled":    cfg.Enabled,
		},
		"usage_plugin": map[string]any{
			"status":  "ready",
			"method":  "usage.handle",
			"scope":   "xai",
			"purpose": "business_bypass_primary_signal",
		},
		"scheduler": map[string]any{
			"interval":           cfg.Interval.String(),
			"last_auto_apply_at": lastAuto,
			"next_wait":          nextWait.String(),
			"worker_active":      workerActive,
		},
		"latest_audit": audit,
		"last_result":  result,
		"run_history":  r.runtime.currentRunHistory(),
	}, nil
}

var _ management.Runner = managementRunner{}
