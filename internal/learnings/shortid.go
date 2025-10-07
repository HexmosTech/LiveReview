package learnings

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
)

// ShortIDFrom generates a compact base36-like ID prefix from input
func ShortIDFrom(parts ...string) string {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(p))))
		_, _ = h.Write([]byte{'|'})
	}
	v := h.Sum64()
	// take 5 bytes for brevity (~8 base36 chars)
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	n := uint64(b[3])<<32 | uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	out := make([]byte, 0, 8)
	for n > 0 {
		out = append(out, alphabet[n%36])
		n /= 36
	}
	// reverse
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}
