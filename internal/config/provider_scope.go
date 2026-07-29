package config

import "strings"

// ProviderScope 表示自动排序适用的 provider 范围。
type ProviderScope string

const (
	// ProviderScopeAll 表示自动排序适用于全部 provider。
	ProviderScopeAll ProviderScope = "all"
	// ProviderScopeSelected 表示自动排序仅适用于用户选择的 provider。
	ProviderScopeSelected ProviderScope = "selected"
)

// parseProviderScope 解析旧式 all/selected 枚举；pipe 列表由 apply 层单独处理。
func parseProviderScope(value string) (ProviderScope, error) {
	switch strings.ToLower(strings.TrimSpace(yamlText(value))) {
	case "", string(ProviderScopeAll):
		return ProviderScopeAll, nil
	case string(ProviderScopeSelected):
		return ProviderScopeSelected, nil
	default:
		return "", invalid("provider_scope", value, "must be all, selected, or a provider list")
	}
}

// ParseProviderScopeValue 解析合并后的 provider_scope 字段。
// 支持：
//   - all / 空 → 全部提供商
//   - selected → 旧枚举，需配合 selected_providers
//   - antigravity / antigravity|codex|xai / 逗号分隔 → 选定提供商
func ParseProviderScopeValue(value string) (ProviderScope, []string, error) {
	text := strings.ToLower(strings.TrimSpace(yamlText(value)))
	if text == "" || text == string(ProviderScopeAll) {
		return ProviderScopeAll, nil, nil
	}
	if text == string(ProviderScopeSelected) {
		return ProviderScopeSelected, nil, nil
	}
	parts := splitProviderScopeList(text)
	if len(parts) == 0 {
		return ProviderScopeAll, nil, nil
	}
	providers, err := NormalizeSelectedProviders(parts)
	if err != nil {
		return "", nil, err
	}
	if len(providers) == 0 {
		return ProviderScopeAll, nil, nil
	}
	return ProviderScopeSelected, providers, nil
}

func splitProviderScopeList(value string) []string {
	normalized := strings.NewReplacer(",", "|", " ", "|", ";", "|").Replace(value)
	raw := strings.Split(normalized, "|")
	parts := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts = append(parts, item)
	}
	return parts
}

// NormalizeSelectedProviders 规范化 UI 或配置传入的 provider 列表。
func NormalizeSelectedProviders(values []string) ([]string, error) {
	providers := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		provider := strings.ToLower(strings.TrimSpace(value))
		if provider == "" {
			continue
		}
		if provider != "antigravity" && provider != "codex" && provider != "xai" {
			return nil, invalid("selected_providers", value, "only antigravity, codex and xai are supported")
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}
	if len(values) > 0 && len(providers) == 0 {
		return nil, invalid("selected_providers", strings.Join(values, ","), "must include non-empty provider names")
	}
	return providers, nil
}

// FormatProviderScope 将内部状态格式化为合并后的 provider_scope 展示值。
func FormatProviderScope(scope ProviderScope, selected []string) string {
	if scope != ProviderScopeSelected || len(selected) == 0 {
		return string(ProviderScopeAll)
	}
	return strings.Join(selected, "|")
}
