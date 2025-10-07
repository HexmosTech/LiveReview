package learnings

import (
	"hash/fnv"
	"math/bits"
	"strings"
)

// very small simhash: tokenizes on whitespace, FNV-1a tokens; weighted by length
func Simhash64(s string) uint64 {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	var vec [64]int64
	fields := strings.Fields(s)
	for _, tok := range fields {
		h := fnv.New64a()
		_, _ = h.Write([]byte(tok))
		v := h.Sum64()
		w := int64(1 + len(tok)/4)
		for i := 0; i < 64; i++ {
			if (v>>uint(i))&1 == 1 {
				vec[i] += w
			} else {
				vec[i] -= w
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if vec[i] >= 0 {
			out |= (1 << uint(i))
		}
	}
	return out
}

func Hamming(a, b uint64) int { return bits.OnesCount64(a ^ b) }
