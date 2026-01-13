package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile loads environment variables from a file, overwriting existing ones.
func LoadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		// Overwrite environment variable
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set env var %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
