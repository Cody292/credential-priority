package xai

import (
	"encoding/json"
	"strings"
	"time"
)

// CLIChatProxyBillingURL 是官方 xAI OAuth 周账单查询端点（Management Center 合同）。
const CLIChatProxyBillingURL = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"

// oauthBillingDocument 仅建模 Management Center 合同中与周长窗相关的字段。
// 支持官方 camelCase 与 snake_case 别名；禁止发明其它端点/字段。
type oauthBillingDocument struct {
	Config *oauthBillingConfig `json:"config"`
}

type oauthBillingConfig struct {
	CurrentPeriod         *oauthBillingPeriod `json:"currentPeriod"`
	CurrentPeriodSnake    *oauthBillingPeriod `json:"current_period"`
	BillingPeriodEnd      string              `json:"billingPeriodEnd"`
	BillingPeriodEndSnake string              `json:"billing_period_end"`
}

type oauthBillingPeriod struct {
	Type string `json:"type"`
	End  string `json:"end"`
}

// ParseWeeklyLongWindowReset 从 OAuth billing JSON 提取周长窗重置时刻。
// 仅当 config.currentPeriod(type=weekly).end 为合法 RFC3339 时返回 UTC 时间；
// monthly 的 billingPeriodEnd / currentPeriod.type=monthly 一律忽略（非 rate-limit 容量重置）；
// 畸形/缺失/不支持 period 返回 (nil, nil) 或 (nil, err)，不得 panic。
func ParseWeeklyLongWindowReset(body []byte) (*time.Time, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var doc oauthBillingDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc.Config == nil {
		return nil, nil
	}
	period := doc.Config.CurrentPeriod
	if period == nil {
		period = doc.Config.CurrentPeriodSnake
	}
	if period == nil {
		// billingPeriodEnd 仅为账单周期元数据，不得映射为 LongWindowResetAt。
		return nil, nil
	}
	periodType := strings.ToLower(strings.TrimSpace(period.Type))
	if periodType != "weekly" {
		// monthly / 空 / 未知类型：明确不作为长窗
		return nil, nil
	}
	endRaw := strings.TrimSpace(period.End)
	if endRaw == "" {
		return nil, nil
	}
	end, err := parseRFC3339UTC(endRaw)
	if err != nil {
		return nil, nil
	}
	return &end, nil
}

func parseRFC3339UTC(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// isOAuthAuthKind 判断 auth JSON 的 auth_kind 是否为 OAuth（官方小写 oauth）。
func isOAuthAuthKind(authKind string) bool {
	return strings.EqualFold(strings.TrimSpace(authKind), "oauth")
}
