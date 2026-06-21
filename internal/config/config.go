package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
}

// ProviderOverride 是按 provider 覆盖的可选配置。
type ProviderOverride struct {
	Enabled        *bool
	AutoApply      *bool
	Interval       time.Duration
	MaxConcurrency int
}

type rawConfig struct {
	Enabled               *bool                          `json:"enabled"`
	AutoApply             *bool                          `json:"auto_apply"`
	Interval              *string                        `json:"interval"`
	MaxConcurrency        *int                           `json:"max_concurrency"`
	MinChange             *int                           `json:"min_change"`
	TopPriorityProbeCount *int                           `json:"top_priority_probe_count"`
	ActiveGroupSize       *int                           `json:"active_group_size"`
	ActiveGroupJitter     *string                        `json:"active_group_jitter"`
	DisabledGroupSize     *int                           `json:"disabled_group_size"`
	DisabledProbeInterval *string                        `json:"disabled_probe_interval"`
	CacheTTL              *string                        `json:"cache_ttl"`
	CachePath             *string                        `json:"cache_path"`
	ProviderOverrides     map[string]rawProviderOverride `json:"provider_overrides"`
}

type rawProviderOverride struct {
	Enabled        *bool   `json:"enabled"`
	AutoApply      *bool   `json:"auto_apply"`
	Interval       *string `json:"interval"`
	MaxConcurrency *int    `json:"max_concurrency"`
}

// Default 返回稳定的插件配置默认值。
func Default() Config {
	return Config{
		Enabled:               true,
		AutoApply:             true,
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
	yamlMap, err := parseYAMLMap(string(trimmed))
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
	if raw.CachePath != nil {
		cachePath := strings.TrimSpace(yamlText(*raw.CachePath))
		if cachePath == "" {
			return Config{}, invalid("cache_path", *raw.CachePath, "must not be empty")
		}
		cfg.CachePath = cachePath
	}
	for _, item := range []struct {
		field  string
		raw    *string
		target *time.Duration
	}{
		{"interval", raw.Interval, &cfg.Interval},
		{"active_group_jitter", raw.ActiveGroupJitter, &cfg.ActiveGroupJitter},
		{"disabled_probe_interval", raw.DisabledProbeInterval, &cfg.DisabledProbeInterval},
		{"cache_ttl", raw.CacheTTL, &cfg.CacheTTL},
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

func parseYAMLMap(data string) (map[string]any, error) {
	result := map[string]any{}
	providerOverrides := map[string]any{}
	section, providerName := "", ""
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, invalid("config", trimmed, "must use key: value syntax")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if indent == 0 {
			section, providerName = key, ""
			if key == "provider_overrides" {
				result[key] = providerOverrides
				continue
			}
			result[key] = yamlScalar(value)
			continue
		}
		if section != "provider_overrides" {
			continue
		}
		if indent == 2 {
			providerName = key
			providerOverrides[providerName] = map[string]any{}
			continue
		}
		if indent == 4 && providerName != "" {
			fields := providerOverrides[providerName].(map[string]any)
			fields[key] = yamlScalar(value)
		}
	}
	return result, nil
}

func parseDuration(field string, value string) (time.Duration, error) {
	text := yamlText(value)
	parsed, err := time.ParseDuration(text)
	if err != nil || parsed <= 0 {
		return 0, invalid(field, text, "must be a positive duration")
	}
	return parsed, nil
}

func yamlScalar(value string) any {
	text := yamlText(value)
	if parsed, err := strconv.Atoi(text); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseBool(text); err == nil {
		return parsed
	}
	return text
}

func yamlText(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > 1 && trimmed[0] == trimmed[len(trimmed)-1] && (trimmed[0] == '"' || trimmed[0] == '\'') {
		return trimmed[1 : len(trimmed)-1]
	}
	return trimmed
}

func invalid(field string, value string, reason string) error {
	return fmt.Errorf("%w: %s=%q %s", ErrInvalidConfig, field, value, reason)
}
