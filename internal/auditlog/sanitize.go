package auditlog

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	maxStringLen    = 2048
	maxArrayItems   = 100
	maxMapKeys      = 100
	maxDetailsBytes = 16 * 1024
	maxDepth        = 6
)

// redactedValue replaces sensitive values. It is intentionally obvious in the
// UI so nobody mistakes it for real data.
const redactedValue = "[REDACTED]"

// sensitiveKeys are exact (lowercase) keys whose values must never be stored.
var sensitiveKeys = map[string]bool{
	"password":         true,
	"password_hash":    true,
	"new_password":     true,
	"old_password":     true,
	"current_password": true,
	"token":            true,
	"access_token":     true,
	"refresh_token":    true,
	"session_token":    true,
	"id_token":         true,
	"api_key":          true,
	"apikey":           true,
	"secret":           true,
	"client_secret":    true,
	"private_key":      true,
	"authorization":    true,
	"cookie":           true,
	"set-cookie":       true,
	"credential":       true,
	"credentials":      true,
	"body":             true,
	"request_body":     true,
	"response_body":    true,
	"raw_body":         true,
	"prompt":           true,
	"raw_prompt":       true,
	"response":         true,
	"raw_response":     true,
}

// sensitiveSuffixes catch variants like "github_token" or "webhook_secret".
// Note that "_tokens" (plural) is deliberately allowed: token *counts* are
// audit data, not secrets.
var sensitiveSuffixes = []string{
	"_password", "_secret", "_token", "_api_key", "_apikey",
	"_credential", "_credentials", "_private_key",
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if sensitiveKeys[k] {
		return true
	}
	for _, suffix := range sensitiveSuffixes {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// SanitizeFields returns a deep, redacted copy of f. It never mutates the input.
// This is a backstop: handlers should not pass secrets in the first place.
func SanitizeFields(f Fields) Fields {
	if f == nil {
		return Fields{}
	}
	sanitized := sanitizeValue(f, 0)
	if m, ok := sanitized.(Fields); ok {
		return clampDetailsSize(m)
	}
	if m, ok := sanitized.(map[string]any); ok {
		return clampDetailsSize(m)
	}
	return Fields{}
}

func sanitizeValue(v any, depth int) any {
	if depth > maxDepth {
		return "[TRUNCATED: max depth]"
	}
	switch val := v.(type) {
	case nil:
		return nil
	case string:
		return truncateString(val)
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return val
	case json.Number:
		return val
	case time.Time:
		return val.UTC().Format(time.RFC3339Nano)
	case []byte:
		return truncateString(string(val))
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(val, &decoded); err != nil {
			return truncateString(string(val))
		}
		return sanitizeValue(decoded, depth+1)
	case []any:
		if len(val) > maxArrayItems {
			val = val[:maxArrayItems]
		}
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = sanitizeValue(item, depth+1)
		}
		return out
	case []string:
		if len(val) > maxArrayItems {
			val = val[:maxArrayItems]
		}
		out := make([]string, len(val))
		for i, item := range val {
			out[i] = truncateString(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		count := 0
		for k, item := range val {
			if count >= maxMapKeys {
				break
			}
			count++
			if isSensitiveKey(k) {
				out[k] = redactedValue
				continue
			}
			out[k] = sanitizeValue(item, depth+1)
		}
		return out
	case Fields:
		out := make(Fields, len(val))
		count := 0
		for k, item := range val {
			if count >= maxMapKeys {
				break
			}
			count++
			if isSensitiveKey(k) {
				out[k] = redactedValue
				continue
			}
			out[k] = sanitizeValue(item, depth+1)
		}
		return out
	default:
		// Structs/maps of other types: normalize through JSON so we never store
		// an unredacted arbitrary Go value.
		b, err := json.Marshal(val)
		if err != nil {
			return truncateString(toString(val))
		}
		var decoded any
		if err := json.Unmarshal(b, &decoded); err != nil {
			return truncateString(string(b))
		}
		return sanitizeValue(decoded, depth+1)
	}
}

func truncateString(s string) string {
	if len(s) <= maxStringLen {
		return s
	}
	return s[:maxStringLen] + "...[truncated]"
}

func toString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[unserializable]"
	}
	return string(b)
}

func clampDetailsSize(f Fields) Fields {
	b, err := json.Marshal(f)
	if err != nil || len(b) <= maxDetailsBytes {
		return f
	}
	return Fields{
		"_truncated":     true,
		"original_bytes": len(b),
	}
}
