package codex

import (
	"encoding/json"
	"strconv"
	"strings"
)

func toString(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != ""
	case json.Number:
		trimmed := strings.TrimSpace(value.String())
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}

func toInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case float64:
		return int64(value), true
	case json.Number:
		integer, err := value.Int64()
		return integer, err == nil
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		integer, err := strconv.ParseInt(trimmed, 10, 64)
		return integer, err == nil
	default:
		return 0, false
	}
}

func toFloat64(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case json.Number:
		floatValue, err := value.Float64()
		return floatValue, err == nil
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		floatValue, err := strconv.ParseFloat(trimmed, 64)
		return floatValue, err == nil
	default:
		return 0, false
	}
}

func toBool(raw any) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		return parsed, err == nil
	default:
		return false, false
	}
}
