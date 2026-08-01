package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseUsageRecord 兼容多字段名的 UsageRecord JSON。
func parseUsageRecord(raw []byte) (usageRecord, bool) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return usageRecord{}, false
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return usageRecord{}, false
	}
	if nested, ok := firstMap(root, "record", "usage", "data", "UsageRecord", "usage_record"); ok {
		root = nested
	}
	rec := usageRecord{
		AuthIndex:  firstString(root, "auth_index", "AuthIndex", "authIndex", "auth"),
		Provider:   firstString(root, "provider", "Provider"),
		Model:      firstString(root, "model", "Model", "model_name", "ModelName"),
		StatusCode: firstInt(root, "status_code", "StatusCode", "status", "Status", "http_status", "HTTPStatus"),
		Error:      firstString(root, "error", "Error", "message", "Message", "error_message", "ErrorMessage", "err"),
		ErrorCode:  firstString(root, "error_code", "ErrorCode", "code", "Code", "errorCode"),
		RawBody:    firstString(root, "body", "Body", "response_body", "ResponseBody", "raw", "Raw"),
	}
	if v, ok := firstBool(root, "success", "Success", "ok", "OK", "succeeded", "Succeeded"); ok {
		rec.Success = &v
	}
	if errObj, ok := firstMap(root, "error", "Error"); ok {
		if rec.Error == "" {
			rec.Error = firstString(errObj, "message", "Message", "error", "Error")
		}
		if rec.ErrorCode == "" {
			rec.ErrorCode = firstString(errObj, "code", "Code", "error_code", "ErrorCode", "type", "Type")
		}
	}
	if strings.TrimSpace(rec.AuthIndex) == "" {
		return usageRecord{}, false
	}
	return rec, true
}

func firstMap(root map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if v, ok := root[key]; ok {
			if m, ok := v.(map[string]any); ok {
				return m, true
			}
		}
	}
	return nil, false
}

func firstString(root map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := root[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case fmt.Stringer:
			if s := strings.TrimSpace(t.String()); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstInt(root map[string]any, keys ...string) int {
	for _, key := range keys {
		v, ok := root[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return int(t)
		case json.Number:
			i, err := t.Int64()
			if err == nil {
				return int(i)
			}
		case int:
			return t
		case int64:
			return int(t)
		case string:
			var n int
			if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil {
				return n
			}
		}
	}
	return 0
}

func firstBool(root map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		v, ok := root[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case bool:
			return t, true
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "true", "1", "yes", "ok":
				return true, true
			case "false", "0", "no":
				return false, true
			}
		case float64:
			return t != 0, true
		}
	}
	return false, false
}
