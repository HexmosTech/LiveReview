package license

import (
	"bufio"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// getDatabaseURL attempts to read DATABASE_URL from env or .env file (best effort).
func getDatabaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	f, err := os.Open(".env")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "DATABASE_URL=") {
			return strings.Trim(strings.TrimPrefix(line, "DATABASE_URL="), "\"'")
		}
	}
	return ""
}

func TestStorage(t *testing.T) {
	dsn := getDatabaseURL()
	if dsn == "" {
		t.Skip("DATABASE_URL not set (skipping DB-backed storage test)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	// Ensure table exists (migration should have run)
	_, err = db.Exec("SELECT 1 FROM license_state LIMIT 1")
	if err != nil {
		t.Fatalf("license_state table missing or not migrated: %v", err)
	}

	// Clean any existing row
	_, _ = db.Exec("DELETE FROM license_state WHERE id=1")

	store := NewStorage(db)
	ctx := context.Background()

	// Initially none
	current, err := store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("GetLicenseState initial: %v", err)
	}
	if current != nil {
		t.Fatalf("expected nil state, got %+v", current)
	}

	// Insert new state
	st := &LicenseState{Status: StatusMissing}
	if err := store.UpsertLicenseState(ctx, st); err != nil {
		t.Fatalf("UpsertLicenseState insert: %v", err)
	}

	got, err := store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("GetLicenseState after insert: %v", err)
	}
	if got == nil || got.Status != StatusMissing {
		t.Fatalf("unexpected state after insert: %+v", got)
	}

	// Mark validation success -> active
	if err := store.UpdateValidationResult(ctx, true, nil, StatusActive, nil, 0, &sql.NullTime{Valid: false}); err != nil {
		t.Fatalf("UpdateValidationResult success: %v", err)
	}
	got2, err := store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("GetLicenseState after success: %v", err)
	}
	if got2.Status != StatusActive {
		t.Fatalf("expected active, got %s", got2.Status)
	}
	if got2.LastValidatedAt == nil {
		t.Fatalf("expected last_validated_at set")
	}

	// Simulate failure -> warning
	code := "NETWORK_ERROR"
	if err := store.UpdateValidationResult(ctx, false, &code, StatusWarning, nil, 1, &sql.NullTime{Valid: false}); err != nil {
		t.Fatalf("UpdateValidationResult failure: %v", err)
	}
	got3, err := store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("GetLicenseState after failure: %v", err)
	}
	if got3.Status != StatusWarning {
		t.Fatalf("expected warning, got %s", got3.Status)
	}
	if got3.ValidationFailures != 1 {
		t.Fatalf("expected failures=1 got %d", got3.ValidationFailures)
	}

	// Simulate entering grace (set grace_started_at)
	now := time.Now()
	grace := sql.NullTime{Time: now, Valid: true}
	if err := store.UpdateValidationResult(ctx, false, &code, StatusGrace, nil, 2, &grace); err != nil {
		t.Fatalf("UpdateValidationResult grace: %v", err)
	}
	got4, err := store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("GetLicenseState after grace: %v", err)
	}
	if got4.Status != StatusGrace {
		t.Fatalf("expected grace, got %s", got4.Status)
	}
	if got4.GraceStartedAt == nil {
		t.Fatalf("expected grace_started_at to be set")
	}
}
