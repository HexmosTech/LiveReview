package license

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestGraceExpiry ensures a grace state older than grace days transitions to expired.
func TestGraceExpiry(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	cfg := LoadConfig()
	svc := NewService(cfg, db)
	ctx := context.Background()

	db.Exec("DELETE FROM license_state WHERE id=1")
	old := time.Now().Add(-time.Duration(cfg.GraceDays+1) * 24 * time.Hour)
	if _, err := db.Exec(`INSERT INTO license_state (id,status,grace_started_at,created_at,updated_at,validation_failures) VALUES (1,$1,$2,now(),now(),3)`, StatusGrace, old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.expireIfGraceExceeded(ctx); err != nil {
		t.Fatalf("expire: %v", err)
	}
	st, err := svc.store.GetLicenseState(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if st.Status != StatusExpired {
		t.Fatalf("expected expired got %s", st.Status)
	}
}
