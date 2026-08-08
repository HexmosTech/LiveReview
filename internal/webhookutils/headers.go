package webhookutils

import (
	"sort"
	"strings"
)

// GetHeaderCaseInsensitive retrieves a header value using case-insensitive key matching.
// This is needed because Go's HTTP library canonicalizes header keys (e.g., X-GitHub-Event -> X-Github-Event)
// which can cause exact string matches to fail.
func GetHeaderCaseInsensitive(headers map[string]string, key string) (string, bool) {
	keyLower := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == keyLower {
			return v, true
		}
	}
	return "", false
}

// SanitizeForLog bounds a webhook-supplied value (e.g. an event-type header)
// to maxLen and strips control characters, so a malicious or malformed
// header can't blow up log size or inject control sequences into log
// output. The values this wraps (event type names, User-Agent strings) are
// not credentials, but they are still attacker-influenced free text.
func SanitizeForLog(s string, maxLen int) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\r' || r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > maxLen {
		return out[:maxLen] + "...(truncated)"
	}
	return out
}

// HeaderNames returns the sorted header names present in headers, without
// their values. Provider webhook-detection code logs which headers arrived
// for debugging; since a webhook request's header set can include arbitrary
// admin-configured headers (auth tokens, secrets), logging names only - not
// the full map - gives the same debugging signal without a chance of a
// credential value ending up in the log.
func HeaderNames(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
