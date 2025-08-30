package api

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filePath string) (map[string]string, error) {
	envMap := make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .env file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Skip comments or empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Remove quotes if present
		value = strings.Trim(value, `"'`)
		envMap[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %v", err)
	}

	return envMap, nil
}

// validateDatabaseConnection checks if the database URL is valid and the database exists
func validateDatabaseConnection(dbURL string) error {
	if dbURL == "" {
		return errors.New("DATABASE_URL is not defined in .env file.\nPlease add DATABASE_URL=postgres://username:password@localhost:5432/dbname?sslmode=disable to your .env file")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("invalid database URL: %v\n\nPlease check your DATABASE_URL format in .env file. It should be in the format:\npostgres://username:password@hostname:5432/dbname?sslmode=disable", err)
	}
	defer db.Close()

	// Check connection
	err = db.Ping()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v\n\nPlease ensure:\n1. PostgreSQL is running\n2. The database exists\n3. Username and password are correct\n4. Database is accepting connections from this host", err)
	}

	// Check if the database has tables (simple check for migrations)
	rows, err := db.Query(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public'
			LIMIT 1
		)
	`)
	if err != nil {
		return fmt.Errorf("error checking database tables: %v", err)
	}
	defer rows.Close()

	var hasTable bool
	if rows.Next() {
		err = rows.Scan(&hasTable)
		if err != nil {
			return fmt.Errorf("error reading database information: %v", err)
		}
	}

	if !hasTable {
		return errors.New("database exists but has no tables.\n\nPlease run migrations first using the migration command or scripts")
	}

	return nil
}
