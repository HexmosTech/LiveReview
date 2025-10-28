package database

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	_ "github.com/lib/pq"
)

// NewDB creates a new database connection
func NewDB() (*sql.DB, error) {
	dbURL, err := loadDatabaseURL()
	if err != nil {
		return nil, fmt.Errorf("failed to get database URL: %w", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	return db, nil
}

func loadDatabaseURL() (string, error) {
	if direct := strings.TrimSpace(os.Getenv("DATABASE_URL")); direct != "" {
		return direct, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	envPath, err := findEnvFile(wd)
	if err != nil {
		return "", err
	}

	file, err := os.Open(envPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", envPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eqIdx := strings.IndexRune(line, '=')
		if eqIdx <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		if key != "DATABASE_URL" {
			continue
		}

		value := strings.TrimSpace(line[eqIdx+1:])
		value = strings.Trim(value, "\"'")
		value = strings.TrimFunc(value, unicode.IsSpace)
		if value == "" {
			return "", errors.New("DATABASE_URL is empty in .env")
		}
		return value, nil
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", envPath, err)
	}

	return "", errors.New("DATABASE_URL not found in environment or .env")
}

func findEnvFile(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".env")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf(".env not found starting from %s", start)
}
