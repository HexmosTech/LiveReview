package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	_ "github.com/lib/pq"
)

// dumpIntegrationTokens prints every row in public.integration_tokens and returns the data.
func dumpIntegrationTokens() ([]map[string]interface{}, error) {
	rowSet, err := fetchIntegrationTokens()
	if err != nil {
		return nil, err
	}

	if len(rowSet) == 0 {
		fmt.Println("[]")
		return rowSet, nil
	}

	encoded, err := json.MarshalIndent(rowSet, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode result set: %w", err)
	}

	fmt.Println(string(encoded))
	return rowSet, nil
}

// fetchIntegrationTokens reads DATABASE_URL, queries integration_tokens, and returns the rows.
func fetchIntegrationTokens() ([]map[string]interface{}, error) {
	dbURL, err := loadDatabaseURL()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	query := `SELECT id, provider, provider_app_id, access_token, refresh_token, token_type, scope,
		expires_at, metadata, created_at, updated_at, code, connection_name, provider_url,
		client_secret, pat_token, projects_cache, org_id
	FROM public.integration_tokens
	ORDER BY id`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query integration_tokens: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("fetch column metadata: %w", err)
	}

	rowSet := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanTargets := make([]interface{}, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}

		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		rowData := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			switch v := values[i].(type) {
			case nil:
				rowData[col] = nil
			case []byte:
				rowData[col] = string(v)
			case time.Time:
				rowData[col] = v.UTC().Format(time.RFC3339Nano)
			default:
				rowData[col] = v
			}
		}

		rowSet = append(rowSet, rowData)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return rowSet, nil
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
