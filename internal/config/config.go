package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// PluginID 是 CPA 宿主识别该插件的固定 ID。
	PluginID = "credential-priority"
	// DirectoryName 是源码目录和插件目录约定名。
	DirectoryName = "credential-priority"
	// DynamicLibraryBaseName 是构建动态库时不含平台扩展名的文件名。
	DynamicLibraryBaseName = "credential-priority"
	// CPAConfigKey 是 `plugins.configs` 下该插件的配置键。
	CPAConfigKey = "credential-priority"
)

// ErrInvalidConfig 标识配置解析或校验失败。
var ErrInvalidConfig = errors.New("config: invalid")

// Config 是插件自有配置的已校验形态。
type Config struct {
	Enabled               bool
	AutoApply             bool
	ProviderScope         ProviderScope
	SelectedProviders     []string
	AntigravityModelGroup AntigravityModelGroup
	Interval              time.Duration
	MaxConcurrency        int
	MinChange             int
	TopPriorityProbeCount int
	ActiveGroupSize       int
	ActiveGroupJitter     time.Duration
	DisabledGroupSize     int
	DisabledProbeInterval time.Duration
	CacheTTL              time.Duration
	CachePath             string
	ProviderOverrides     map[string]ProviderOverride
	PriorityRules         PriorityRules
}

// ProviderOverride 是按 provider 覆盖的可选配置。
type ProviderOverride struct {
	Enabled        *bool
	AutoApply      *bool
	Interval       time.Duration
	MaxConcurrency int
}

// PriorityRules 是管理页可编辑的 provider 独立排序规则草稿。
type PriorityRules struct {
	Enabled     bool
	Antigravity AntigravityPriorityRules
	Codex       CodexPriorityRules
}

// AntigravityPriorityRules 是 Antigravity 排序规则的可配置部分。
type AntigravityPriorityRules struct {
	StartPriority int
}

// CodexPriorityRules 是 Codex 排序规则的可配置部分。
type CodexPriorityRules struct {
	StartPriority            int
	FreeDepletedPriority     int
	FreeDepletedDisabled     bool
	PaidDepletedKeepsEnabled bool
}

type rawConfig struct {
	Enabled               *bool                          `json:"enabled"`
	AutoApply             *bool                          `json:"auto_apply"`
	ProviderScope         *string                        `json:"provider_scope"`
	SelectedProviders     selectedProviderList           `json:"selected_providers"`
	AntigravityModelGroup *string                        `json:"antigravity_model_group"`
	Interval              *string                        `json:"interval"`
	MaxConcurrency        *int                           `json:"max_concurrency"`
	MinChange             *int                           `json:"min_change"`
	TopPriorityProbeCount *int                           `json:"top_priority_probe_count"`
	ActiveGroupSize       *int                           `json:"active_group_size"`
	ActiveGroupJitter     *string                        `json:"active_group_jitter"`
	DisabledGroupSize     *int                           `json:"disabled_group_size"`
	DisabledProbeInterval *string                        `json:"disabled_probe_interval"`
	ProviderOverrides     map[string]rawProviderOverride `json:"provider_overrides"`
	PriorityRules         *rawPriorityRules              `json:"priority_rules"`
}

type rawProviderOverride struct {
	Enabled        *bool   `json:"enabled"`
	AutoApply      *bool   `json:"auto_apply"`
	Interval       *string `json:"interval"`
	MaxConcurrency *int    `json:"max_concurrency"`
}

type rawPriorityRules struct {
	Enabled     *bool                   `json:"enabled"`
	Antigravity *rawAntigravityPriority `json:"antigravity"`
	Codex       *rawCodexPriority       `json:"codex"`
	Unsupported map[string]json.RawMessage
}

type rawAntigravityPriority struct {
	StartPriority *int `json:"start_priority"`
}

type rawCodexPriority struct {
	StartPriority            *int  `json:"start_priority"`
	FreeDepletedPriority     *int  `json:"free_depleted_priority"`
	FreeDepletedDisabled     *bool `json:"free_depleted_disabled"`
	PaidDepletedKeepsEnabled *bool `json:"paid_depleted_keeps_enabled"`
}

func (raw *rawPriorityRules) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return err
		}
		inner := strings.TrimSpace(encoded)
		if inner == "" {
			*raw = rawPriorityRules{}
			return nil
		}
		if strings.HasPrefix(inner, "{") {
			return json.Unmarshal([]byte(inner), raw)
		}
		fields, err := parseYAMLMap(inner)
		if err != nil {
			return err
		}
		encodedFields, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		return json.Unmarshal(encodedFields, raw)
	}
	type alias rawPriorityRules
	var decoded alias
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return err
	}
	for _, allowed := range []string{"enabled", "antigravity", "codex"} {
		delete(fields, allowed)
	}
	*raw = rawPriorityRules(decoded)
	raw.Unsupported = fields
	return nil
}

type selectedProviderList struct {
	values []string
	set    bool
}

func (list *selectedProviderList) UnmarshalJSON(data []byte) error {
	list.set = true
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		list.values = nil
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		list.values = values
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		list.values = nil
		return nil
	}
	list.values = []string{value}
	return nil
}

// Default 返回稳定的插件配置默认值。
func Default() Config {
	return Config{
		Enabled:               true,
		AutoApply:             false,
		ProviderScope:         ProviderScopeAll,
		AntigravityModelGroup: AntigravityModelGroupGemini,
		Interval:              5 * time.Minute,
		MaxConcurrency:        2,
		MinChange:             1,
		TopPriorityProbeCount: 10,
		ActiveGroupSize:       10,
		ActiveGroupJitter:     10 * time.Minute,
		DisabledGroupSize:     5,
		DisabledProbeInterval: 30 * time.Minute,
		CacheTTL:              15 * time.Minute,
		CachePath:             DirectoryName + "/refresh-cache.json",
		PriorityRules:         defaultPriorityRules(),
	}
}

func defaultPriorityRules() PriorityRules {
	return PriorityRules{
		Enabled: false,
		Antigravity: AntigravityPriorityRules{
			StartPriority: 100,
		},
		Codex: CodexPriorityRules{
			StartPriority:            100,
			FreeDepletedPriority:     -1,
			FreeDepletedDisabled:     true,
			PaidDepletedKeepsEnabled: true,
		},
	}
}

// LoadBytes 将 CPA 传入的插件配置字节解析为 Config。
func LoadBytes(data []byte) (Config, error) {
	raw, err := decodeRaw(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return raw.apply(Default())
}

func decodeRaw(data []byte) (rawConfig, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return rawConfig{}, nil
	}
	var raw rawConfig
	if trimmed[0] == '{' {
		if err := json.NewDecoder(bytes.NewReader(trimmed)).Decode(&raw); err != nil {
			return rawConfig{}, invalid("config", "json", "must be valid JSON")
		}
		return raw, nil
	}
	yamlMap, err := parseYAMLMap(extractPluginConfigYAML(string(trimmed)))
	if err != nil {
		return rawConfig{}, err
	}
	encoded, err := json.Marshal(yamlMap)
	if err != nil {
		return rawConfig{}, invalid("config", "yaml", "must be encodable")
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return rawConfig{}, invalid("config", err.Error(), "must match config schema")
	}
	return raw, nil
}

func (raw rawConfig) apply(cfg Config) (Config, error) {
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	if raw.AutoApply != nil {
		cfg.AutoApply = *raw.AutoApply
	}
	if raw.ProviderScope != nil {
		providerScope, err := parseProviderScope(*raw.ProviderScope)
		if err != nil {
			return Config{}, err
		}
		cfg.ProviderScope = providerScope
	}
	if raw.SelectedProviders.set {
		providers, err := NormalizeSelectedProviders(raw.SelectedProviders.values)
		if err != nil {
			return Config{}, err
		}
		cfg.SelectedProviders = providers
	}
	if raw.AntigravityModelGroup != nil {
		modelGroup, err := ParseAntigravityModelGroup(*raw.AntigravityModelGroup)
		if err != nil {
			return Config{}, err
		}
		cfg.AntigravityModelGroup = modelGroup
	}
	if cfg.ProviderScope == ProviderScopeSelected && len(cfg.SelectedProviders) == 0 {
		cfg.ProviderScope = ProviderScopeAll
		cfg.SelectedProviders = nil
	}
	if raw.PriorityRules != nil {
		priorityRules, err := raw.PriorityRules.apply(cfg.PriorityRules)
		if err != nil {
			return Config{}, err
		}
		cfg.PriorityRules = priorityRules
	}
	for _, item := range []struct {
		field  string
		raw    *string
		target *time.Duration
	}{
		{"interval", raw.Interval, &cfg.Interval},
		{"active_group_jitter", raw.ActiveGroupJitter, &cfg.ActiveGroupJitter},
		{"disabled_probe_interval", raw.DisabledProbeInterval, &cfg.DisabledProbeInterval},
	} {
		if item.raw != nil {
			parsed, err := parseDuration(item.field, *item.raw)
			if err != nil {
				return Config{}, err
			}
			*item.target = parsed
		}
	}
	for _, item := range []struct {
		field  string
		raw    *int
		target *int
		min    int
	}{
		{"max_concurrency", raw.MaxConcurrency, &cfg.MaxConcurrency, 1},
		{"min_change", raw.MinChange, &cfg.MinChange, 0},
		{"top_priority_probe_count", raw.TopPriorityProbeCount, &cfg.TopPriorityProbeCount, 1},
		{"active_group_size", raw.ActiveGroupSize, &cfg.ActiveGroupSize, 1},
		{"disabled_group_size", raw.DisabledGroupSize, &cfg.DisabledGroupSize, 1},
	} {
		if item.raw != nil {
			*item.target = *item.raw
		}
		if *item.target < item.min {
			return Config{}, invalid(item.field, fmt.Sprint(*item.target), fmt.Sprintf("must be at least %d", item.min))
		}
	}
	if raw.ProviderOverrides == nil {
		return cfg, nil
	}
	cfg.ProviderOverrides = make(map[string]ProviderOverride, len(raw.ProviderOverrides))
	for providerName, rawOverride := range raw.ProviderOverrides {
		override, err := rawOverride.apply(providerName)
		if err != nil {
			return Config{}, err
		}
		cfg.ProviderOverrides[providerName] = override
	}
	return cfg, nil
}

func (raw rawPriorityRules) apply(rules PriorityRules) (PriorityRules, error) {
	for provider := range raw.Unsupported {
		return PriorityRules{}, invalid("priority_rules."+provider, provider, "only antigravity and codex are supported")
	}
	if raw.Enabled != nil {
		rules.Enabled = *raw.Enabled
	}
	if raw.Antigravity != nil {
		updated, err := raw.Antigravity.apply(rules.Antigravity)
		if err != nil {
			return PriorityRules{}, err
		}
		rules.Antigravity = updated
	}
	if raw.Codex != nil {
		updated, err := raw.Codex.apply(rules.Codex)
		if err != nil {
			return PriorityRules{}, err
		}
		rules.Codex = updated
	}
	return rules, nil
}

func (raw rawAntigravityPriority) apply(rule AntigravityPriorityRules) (AntigravityPriorityRules, error) {
	if raw.StartPriority != nil {
		rule.StartPriority = *raw.StartPriority
	}
	if rule.StartPriority < 1 {
		return AntigravityPriorityRules{}, invalid("priority_rules.antigravity.start_priority", fmt.Sprint(rule.StartPriority), "must be at least 1")
	}
	return rule, nil
}

func (raw rawCodexPriority) apply(rule CodexPriorityRules) (CodexPriorityRules, error) {
	if raw.StartPriority != nil {
		rule.StartPriority = *raw.StartPriority
	}
	if raw.FreeDepletedPriority != nil {
		rule.FreeDepletedPriority = *raw.FreeDepletedPriority
	}
	if raw.FreeDepletedDisabled != nil {
		rule.FreeDepletedDisabled = *raw.FreeDepletedDisabled
	}
	if raw.PaidDepletedKeepsEnabled != nil {
		rule.PaidDepletedKeepsEnabled = *raw.PaidDepletedKeepsEnabled
	}
	if rule.StartPriority < 1 {
		return CodexPriorityRules{}, invalid("priority_rules.codex.start_priority", fmt.Sprint(rule.StartPriority), "must be at least 1")
	}
	return rule, nil
}

func (raw rawProviderOverride) apply(providerName string) (ProviderOverride, error) {
	override := ProviderOverride{Enabled: raw.Enabled, AutoApply: raw.AutoApply}
	if raw.Interval != nil {
		parsed, err := parseDuration("provider_overrides."+providerName+".interval", *raw.Interval)
		if err != nil {
			return ProviderOverride{}, err
		}
		override.Interval = parsed
	}
	if raw.MaxConcurrency == nil {
		return override, nil
	}
	if *raw.MaxConcurrency < 1 {
		return ProviderOverride{}, invalid("provider_overrides."+providerName+".max_concurrency", fmt.Sprint(*raw.MaxConcurrency), "must be at least 1")
	}
	override.MaxConcurrency = *raw.MaxConcurrency
	return override, nil
}
