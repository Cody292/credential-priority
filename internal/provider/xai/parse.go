package xai

import "strings"

// safeError 截断并脱敏错误摘要，避免把 token / 凭证 JSON 写进日志或 store。
// FetchPlan / AuthInvalid 路径复用。
func safeError(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "probe failed"
	}
	if strings.Contains(trimmed, "access_token") || strings.Contains(trimmed, "refresh_token") {
		return "probe failed"
	}
	if len(trimmed) > 240 {
		return trimmed[:240]
	}
	return trimmed
}
