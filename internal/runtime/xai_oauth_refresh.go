package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"credential-priority/internal/host"
	"credential-priority/internal/provider/xai"
)

// maybeRefreshXAIAuth 探测前/401 强制路径：GetAuth→Decide→HTTP refresh→SaveAuth；失败不阻断探测。
func maybeRefreshXAIAuth(
	ctx context.Context,
	client *host.Client,
	authIndex string,
	credentialDisabled bool,
	force bool,
	now time.Time,
) (accessToken string, refreshed bool) {
	if client == nil || strings.TrimSpace(authIndex) == "" {
		return "", false
	}
	raw, err := readCredentialAuthJSON(ctx, client, authIndex)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	fields, err := xai.ParseAuthRefreshFields(raw)
	if err != nil {
		return "", false
	}
	if credentialDisabled {
		fields.Disabled = true
	}
	decision := xai.DecideRefresh(fields, now, force)
	if !decision.Need {
		return "", false
	}
	merged, access, err := xai.RefreshAndMerge(ctx, client, raw, now)
	if err != nil || strings.TrimSpace(access) == "" {
		return "", false
	}
	saveName := strings.TrimSpace(authIndex)
	if err := client.SaveAuth(ctx, saveName, merged); err != nil {
		return "", false
	}
	return access, true
}

func refreshMetaFromJSON(raw json.RawMessage) (disabled bool, lastRefresh time.Time, expiredAt time.Time, hasRefreshToken bool) {
	fields, err := xai.ParseAuthRefreshFields(raw)
	if err != nil {
		return false, time.Time{}, time.Time{}, false
	}
	return fields.Disabled, fields.LastRefreshAt, fields.ExpiredAt, strings.TrimSpace(fields.RefreshToken) != ""
}
