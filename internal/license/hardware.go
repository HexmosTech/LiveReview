package license

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
)

// NOTE: We keep this minimal and deterministic. We intentionally avoid
// adding gopsutil for now to keep Phase 2 lean; can be extended later.

func GenerateHardwareFingerprint(cfg *Config) (string, error) {
	if !cfg.IncludeHardwareID {
		return "", nil
	}
	// Basic factors: GOOS, GOARCH, runtime version (acts as salt), hostname redacted to prefix.
	host := safeHostname()
	raw := strings.Join([]string{host, runtime.GOOS, runtime.GOARCH, runtime.Version()}, "|")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

func safeHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	parts := strings.Split(h, ".")
	return parts[0]
}
