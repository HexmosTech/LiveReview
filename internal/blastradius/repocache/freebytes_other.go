//go:build !unix

package repocache

import "math"

// FreeBytes can't be measured portably here, so report "plenty" and skip the low-disk check.
func FreeBytes(root string) (uint64, error) {
	return math.MaxUint64, nil
}
