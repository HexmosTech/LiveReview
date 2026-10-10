//go:build unix

package repocache

import (
	"os"
	"syscall"
)

// FreeBytes is the free space on the volume holding root.
func FreeBytes(root string) (uint64, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, err
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
