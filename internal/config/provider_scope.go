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

func parseProviderScope(value string) (ProviderScope, error) {
	switch strings.ToLower(strings.TrimSpace(yamlText(value))) {
	case "", string(ProviderScopeAll):
		return ProviderScopeAll, nil
	case string(ProviderScopeSelected):
		return ProviderScopeSelected, nil
	default:
		return "", invalid("provider_scope", value, "must be all or selected")
	}
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
		if provider != "antigravity" && provider != "codex" {
			return nil, invalid("selected_providers", value, "only antigravity and codex are supported")
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
