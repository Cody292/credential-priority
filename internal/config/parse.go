package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func extractPluginConfigYAML(data string) string {
	if hasTopLevelPluginField(data) {
		return data
	}
	lines := strings.Split(data, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "credential-priority:" || !isCPAPluginConfigPath(lines, index) {
			continue
		}
		indent := leadingSpaces(line)
		collected := collectIndentedBlock(lines[index+1:], indent)
		if len(collected) == 0 {
			return data
		}
		return strings.Join(collected, "\n")
	}
	return data
}

func hasTopLevelPluginField(data string) bool {
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || leadingSpaces(line) != 0 {
			continue
		}
		key, _, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "enabled", "auto_apply", "provider_scope", "selected_providers", "antigravity_model_group", "priority_rules":
			return true
		}
	}
	return false
}

func isCPAPluginConfigPath(lines []string, credentialIndex int) bool {
	credentialIndent := leadingSpaces(lines[credentialIndex])
	configsIndent := -1
	for index := credentialIndex - 1; index >= 0; index-- {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(lines[index])
		if indent >= credentialIndent {
			continue
		}
		if configsIndent < 0 {
			if trimmed != "configs:" {
				return false
			}
			configsIndent = indent
			continue
		}
		return trimmed == "plugins:" && indent < configsIndent
	}
	return false
}

func collectIndentedBlock(lines []string, parentIndent int) []string {
	collected := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if indent <= parentIndent {
			break
		}
		collected = append(collected, line)
	}
	return normalizeIndentedBlock(collected)
}

func normalizeIndentedBlock(lines []string) []string {
	baseIndent := -1
	for _, line := range lines {
		indent := leadingSpaces(line)
		if baseIndent < 0 || indent < baseIndent {
			baseIndent = indent
		}
	}
	if baseIndent <= 0 {
		return lines
	}
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		if leadingSpaces(line) < baseIndent {
			normalized = append(normalized, strings.TrimLeft(line, " "))
			continue
		}
		normalized = append(normalized, line[baseIndent:])
	}
	return normalized
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func parseYAMLMap(data string) (map[string]any, error) {
	result := map[string]any{}
	providerOverrides := map[string]any{}
	priorityRules := map[string]any{}
	section, providerName, priorityProviderName := "", "", ""
	sectionIndent, priorityProviderIndent := -1, -1
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if strings.HasPrefix(trimmed, "-") {
			if section == "selected_providers" && indent > sectionIndent {
				items, _ := result[section].([]string)
				result[section] = append(items, yamlText(strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))))
				continue
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, invalid("config", trimmed, "must use key: value syntax")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if indent == 0 {
			section, providerName, priorityProviderName = key, "", ""
			sectionIndent, priorityProviderIndent = indent, -1
			if key == "selected_providers" && value == "" {
				result[key] = []string{}
				continue
			}
			if key == "provider_overrides" {
				result[key] = providerOverrides
				continue
			}
			if key == "priority_rules" {
				result[key] = priorityRules
				continue
			}
			result[key] = yamlScalar(value)
			continue
		}
		if section == "priority_rules" {
			if indent <= sectionIndent {
				continue
			}
			if priorityProviderName == "" || indent <= priorityProviderIndent {
				if value != "" {
					priorityRules[key] = yamlScalar(value)
					continue
				}
				priorityProviderName = key
				priorityProviderIndent = indent
				priorityRules[priorityProviderName] = map[string]any{}
				continue
			}
			if priorityProviderName != "" && indent > priorityProviderIndent {
				fields := priorityRules[priorityProviderName].(map[string]any)
				fields[key] = yamlScalar(value)
			}
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
