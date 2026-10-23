package config

import "strings"

// AntigravityModelGroup 表示 Antigravity 配额排序使用的独立模型组。
type AntigravityModelGroup string

const (
	// AntigravityModelGroupGemini 表示 Gemini 模型组。
	AntigravityModelGroupGemini AntigravityModelGroup = "gemini"
	// AntigravityModelGroupClaudeGPT 表示 Claude 和 GPT 模型组。
	AntigravityModelGroupClaudeGPT AntigravityModelGroup = "claude_gpt"
	// AntigravityModelGroupGemini4Argon 表示 Gemini 4 Argon 模型组。
	AntigravityModelGroupGemini4Argon AntigravityModelGroup = "gemini_4_argon"
)

// ParseAntigravityModelGroup 将 UI、JSON 或 YAML 中的模型组文本解析为受控枚举。
func ParseAntigravityModelGroup(value string) (AntigravityModelGroup, error) {
	switch normalizeAntigravityModelGroup(value) {
	case "", string(AntigravityModelGroupGemini):
		return AntigravityModelGroupGemini, nil
	case string(AntigravityModelGroupClaudeGPT), "claudegpt", "claude-gpt", "claude gpt", "claude_and_gpt", "claude-and-gpt", "claude and gpt":
		return AntigravityModelGroupClaudeGPT, nil
	case string(AntigravityModelGroupGemini4Argon), "gemini 4 argon", "gemini4argon":
		return AntigravityModelGroupGemini4Argon, nil
	default:
		return "", invalid("antigravity_model_group", value, "must be gemini, claude_gpt or gemini_4_argon")
	}
}

func normalizeAntigravityModelGroup(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(yamlText(value)))
	trimmed = strings.ReplaceAll(trimmed, "-", "_")
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	return trimmed
}
