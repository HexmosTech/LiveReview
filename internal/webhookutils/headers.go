package webhookutils

import "strings"

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
