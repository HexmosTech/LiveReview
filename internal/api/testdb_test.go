package api

import "os"

// testDatabaseURL is the DB the integration tests use: DATABASE_URL, else the local dev default.
func testDatabaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
}
