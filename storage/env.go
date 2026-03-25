package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CmdEnvOpen(filename string) (*os.File, error) {
	trimmed := strings.TrimSpace(filename)
	if trimmed != filename {
		return nil, fmt.Errorf("filename must not contain leading or trailing whitespace")
	}

	cleaned := filepath.Clean(trimmed)
	return os.Open(cleaned)
}
