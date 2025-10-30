package lib

import (
	"fmt"
)

// MrModelImpl is a struct to hold the mrmodel library implementation.
type MrModelImpl struct {
	EnableArtifactWriting bool
}

// Helpers
func atoi(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}
