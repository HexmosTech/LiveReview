package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/blobstore"
	"github.com/livereview/pkg/models"
)

func TestDiffArchivalManager_ExecutesBulkArchival(t *testing.T) {
	dbURL := "postgres://livereview:livereview_password_123@localhost:5432/after?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}
	defer db.Close()

	// Create test diff payload
	diffs := []models.CodeDiff{
		{
			FilePath:   "test_file.go",
			OldContent: "package old",
			NewContent: "package main",
		},
	}
	diffsJSON, err := json.Marshal(diffs)
	if err != nil {
		t.Fatalf("failed to marshal diffs: %v", err)
	}

	// Insert review created 45 days ago with preloaded_changes in DB metadata
	pastTime := time.Now().AddDate(0, 0, -45)
	var reviewID int64
	var orgID int64 = 1
	metaJSON := fmt.Sprintf(`{"preloaded_changes": %s}`, string(diffsJSON))

	_, _ = db.Exec(`SELECT setval(pg_get_serial_sequence('reviews', 'id'), COALESCE(max(id), 1)) FROM reviews;`)

	err = db.QueryRow(`
		INSERT INTO reviews (org_id, repository, trigger_type, status, metadata, created_at)
		VALUES ($1, 'test/repo', 'cli_diff', 'completed', $2::jsonb, $3)
		RETURNING id;
	`, orgID, metaJSON, pastTime).Scan(&reviewID)
	if err != nil {
		t.Fatalf("failed to insert test review: %v", err)
	}

	// Verify preloaded_changes exists in DB metadata before archival
	var hasPreloaded bool
	err = db.QueryRow("SELECT metadata ? 'preloaded_changes' FROM reviews WHERE id = $1", reviewID).Scan(&hasPreloaded)
	if err != nil || !hasPreloaded {
		t.Fatalf("expected preloaded_changes in DB metadata before archival")
	}

	// Initialize manager and execute archival for reviews > 30 days old
	mgr := NewDiffArchivalManager(db, nil)
	archived, errs := mgr.ExecuteBulkArchivalTest(context.Background(), 30, 50, 0)
	if errs != 0 {
		t.Fatalf("expected 0 errors during archival, got %d", errs)
	}
	if archived < 1 {
		t.Fatalf("expected at least 1 archived review, got %d", archived)
	}

	// Verify preloaded_changes has been pruned from DB metadata for our review
	err = db.QueryRow("SELECT metadata ? 'preloaded_changes' FROM reviews WHERE id = $1", reviewID).Scan(&hasPreloaded)
	if err != nil {
		t.Fatalf("failed to check metadata: %v", err)
	}
	if hasPreloaded {
		t.Fatalf("expected preloaded_changes to be pruned from DB metadata after archival")
	}

	// Verify blob artifact exists in Blob Storage
	blobData, err := blobstore.ReadArtifact(context.Background(), db, orgID, reviewID, blobstore.ArtifactPreloadedChanges)
	if err != nil || len(blobData) == 0 {
		t.Fatalf("expected blob artifact in storage after archival, got error: %v", err)
	}

	var fetchedDiffs []models.CodeDiff
	if err := json.Unmarshal(blobData, &fetchedDiffs); err != nil || len(fetchedDiffs) != 1 {
		t.Fatalf("failed to unmarshal archived blob diffs: %v", err)
	}
	if fetchedDiffs[0].FilePath != "test_file.go" {
		t.Fatalf("unexpected diff content in blob: %+v", fetchedDiffs[0])
	}

	// Verify Server.fetchPreloadedChanges reads from Blob Storage when metadata is pruned
	server := &Server{db: db}
	meta := map[string]interface{}{} // empty metadata (pruned)
	serverDiffs, err := server.fetchPreloadedChanges(context.Background(), orgID, reviewID, meta)
	if err != nil {
		t.Fatalf("fetchPreloadedChanges failed for archived review: %v", err)
	}
	if len(serverDiffs) != 1 || serverDiffs[0].FilePath != "test_file.go" {
		t.Fatalf("fetchPreloadedChanges returned invalid data: %+v", serverDiffs)
	}

	t.Logf("✓ DiffArchivalManager successfully archived review %d (> 30 days old) to Blob Storage, pruned DB metadata, and verified API dual-read fallback!", reviewID)
}
