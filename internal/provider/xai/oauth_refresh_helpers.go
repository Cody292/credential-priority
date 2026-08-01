package xai

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"credential-priority/internal/host"
)

func firstStringField(object map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if v, ok := object[key]; ok {
			if s := stringFromRaw(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func stringField(object map[string]json.RawMessage, key string) string {
	v, ok := object[key]
	if !ok {
		return ""
	}
	return stringFromRaw(v)
}

func stringFromRaw(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseFlexibleInt64(raw json.RawMessage) int64 {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int64(f)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0
		}
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func parseTimeField(object map[string]json.RawMessage, key string) (time.Time, bool) {
	raw, ok := object[key]
	if !ok {
		return time.Time{}, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC(), true
			}
		}
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil && unix > 0 {
			return time.Unix(unix, 0).UTC(), true
		}
		return time.Time{}, false
	}
	if n := parseFlexibleInt64(raw); n > 1_000_000_000_000 {
		return time.UnixMilli(n).UTC(), true
	} else if n > 1_000_000_000 {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}

func sanitizeRefreshErr(msg string) string {
	msg = host.RedactBytes([]byte(msg))
	if len(msg) > 160 {
		return msg[:160]
	}
	return msg
}

func sanitizeRefreshBody(body []byte) string {
	redacted := host.RedactBytes(body)
	if redacted == "" {
		return "(empty)"
	}
	runes := []rune(redacted)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return redacted
}
