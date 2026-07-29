package management

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"credential-priority/internal/apply"
	"credential-priority/internal/config"
	"credential-priority/internal/host"
)

// StatusInfo 表示用于渲染 HTML 页面和 JSON API 的状态摘要。
type StatusInfo struct {
	TotalCredentials int       `json:"total_credentials"`
	FreshCount       int       `json:"fresh_count"`
	UnknownCount     int       `json:"unknown_count"`
	FailedCount      int       `json:"failed_count"`
	NextProbeAt      time.Time `json:"next_probe_at"`
	LatestAudit      string    `json:"latest_audit"`
}

// Runner 定义了管理 API 处理程序所需的 runtime 服务契约。
// 用于与 runtime 层解耦，防止循环导入。
type Runner interface {
	Run(ctx context.Context, request RunRequest) (apply.Result, error)
	Status(ctx context.Context) (StatusInfo, error)
	LatestSnapshot(ctx context.Context) (apply.PlanSnapshot, error)
	Diagnostics(ctx context.Context) (map[string]any, error)
}

// RunRequest 是资源页触发手动排序时的已解析请求。
type RunRequest struct {
	Mode                  string
	Scope                 config.ProviderScope
	Providers             []string
	AntigravityModelGroup config.AntigravityModelGroup
	AuthIndexes           []string
}

// Handler 实现了 http.Handler 用于提供管理接口与 HTML 状态页面。
type Handler struct {
	runner Runner
	mu     sync.Mutex
	active bool
}

// NewHandler 创建一个新的 Handler 实例。
func NewHandler(runner Runner) *Handler {
	return &Handler{
		runner: runner,
	}
}

// routeSourceHeader 与 runtime 层约定一致：resource 仅静态壳，management 才跑动态业务。
const routeSourceHeader = "X-Credential-Priority-Route-Source"

// ServeHTTP 实现 http.Handler 接口。
// resource 前缀：仅 GET /status 静态 HTML。
// management 前缀：POST /run、GET /diagnostics、GET /snapshot/latest。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	method := r.Method
	source := r.Header.Get(routeSourceHeader)

	// 资源路由边界：禁止在 resource 上承载 run/apply/config 等动态业务。
	if source == "resource" {
		if path == "/status" && method == http.MethodGet {
			h.handleStatus(w, r)
			return
		}
		h.writeJSONError(w, http.StatusNotFound, "resource route only serves static /status")
		return
	}

	switch {
	case path == "/run" && method == http.MethodPost:
		h.handleRun(w, r)
	case path == "/diagnostics" && method == http.MethodGet:
		h.handleDiagnostics(w, r)
	case path == "/snapshot/latest" && method == http.MethodGet:
		h.handleSnapshot(w, r)
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "route not found"})
	}
}

// handleStatus 返回静态 HTML 壳（Key 验证 + 只读概览/配置展示 + 手动排序入口）。
// 不在服务端注入动态业务数据；只读数据与 run 均由浏览器调用 management 路径完成。
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(StatusHTML)); err != nil {
		// 响应已开始写入时无法再改状态码，仅尽量结束。
		return
	}
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode != "dry-run" && mode != "apply" {
		h.writeJSONError(w, http.StatusBadRequest, "invalid mode: must be 'dry-run' or 'apply'")
		return
	}

	// 使用互斥体锁定并发 /run 执行以避免并发写入 (single-flight / 409 Conflict)
	if !h.tryAcquire() {
		h.writeJSONError(w, http.StatusConflict, "concurrency conflict: runner is already active")
		return
	}
	defer h.release()

	providerScope, err := parseRunProviderScope(r)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	providers, err := config.NormalizeSelectedProviders(r.URL.Query()["provider"])
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	modelGroup, err := parseRunAntigravityModelGroup(r)
	if err != nil {
		h.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.runner.Run(r.Context(), RunRequest{Mode: mode, Scope: providerScope, Providers: providers, AntigravityModelGroup: modelGroup, AuthIndexes: r.URL.Query()["auth_index"]})
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func parseRunAntigravityModelGroup(r *http.Request) (config.AntigravityModelGroup, error) {
	value := r.URL.Query().Get("antigravity_model_group")
	return config.ParseAntigravityModelGroup(value)
}

func parseRunProviderScope(r *http.Request) (config.ProviderScope, error) {
	if r.URL.Query().Get("provider_scope") == "all" {
		return config.ProviderScopeAll, nil
	}
	providers := r.URL.Query()["provider"]
	if len(providers) > 0 {
		return config.ProviderScopeSelected, nil
	}
	return "", nil
}

func (h *Handler) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	diag, err := h.runner.Diagnostics(r.Context())
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 诊断结果全量脱敏
	redactedDiag := redactMap(diag)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(redactedDiag)
}

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap, err := h.runner.LatestSnapshot(r.Context())
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	snapBytes, err := json.Marshal(snap)
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, "marshal snapshot failed: "+err.Error())
		return
	}
	var snapMap map[string]any
	if err := json.Unmarshal(snapBytes, &snapMap); err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, "unmarshal snapshot failed: "+err.Error())
		return
	}
	redactedSnap := redactMap(snapMap)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(redactedSnap)
}

func (h *Handler) tryAcquire() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.active {
		return false
	}
	h.active = true
	return true
}

func (h *Handler) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active = false
}

func (h *Handler) writeJSONError(w http.ResponseWriter, statusCode int, errMsg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": redactText(errMsg)})
}

// redactMap 深度复制并脱敏 diagnostic 映射。
func redactMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		if isSensitiveKey(k) {
			dst[k] = "[REDACTED]"
			continue
		}
		switch val := v.(type) {
		case string:
			dst[k] = redactText(val)
		case map[string]any:
			dst[k] = redactMap(val)
		case []any:
			dst[k] = redactSlice(val)
		default:
			dst[k] = v
		}
	}
	return dst
}

func redactSlice(src []any) []any {
	dst := make([]any, len(src))
	for i, v := range src {
		switch val := v.(type) {
		case string:
			dst[i] = redactText(val)
		case map[string]any:
			dst[i] = redactMap(val)
		case []any:
			dst[i] = redactSlice(val)
		default:
			dst[i] = v
		}
	}
	return dst
}

func redactText(val string) string {
	return host.RedactBytes([]byte(val))
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	return k == "authorization" ||
		k == "cookie" ||
		k == "set_cookie" ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "api_key") ||
		strings.Contains(k, "apikey")
}
