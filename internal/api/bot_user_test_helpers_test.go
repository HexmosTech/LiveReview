package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupBotUserTestServer(t *testing.T) *Server {
	t.Helper()

	versionInfo := &VersionInfo{Version: "test"}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to change directory to repo root %s: %v", repoRoot, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	server, err := NewServer(8888, versionInfo)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() {
		if server.db != nil {
			server.db.Close()
		}
	})

	return server
}

func stubHTTPTransport(t *testing.T, responder func(*http.Request) (*http.Response, error)) {
	t.Helper()

	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(responder)
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func getAnyOrgID(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var id int64
	err := db.QueryRow("SELECT id FROM orgs ORDER BY id LIMIT 1").Scan(&id)
	if err == sql.ErrNoRows {
		err = db.QueryRow(
			"INSERT INTO orgs (name, description, created_at, updated_at) VALUES ($1, $2, now(), now()) RETURNING id",
			fmt.Sprintf("Test Org %d", time.Now().UnixNano()),
			"created by bot user tests",
		).Scan(&id)
	}
	if err != nil {
		t.Fatalf("failed to ensure org row for test: %v", err)
	}
	return id
}

func insertIntegrationToken(t *testing.T, db *sql.DB, provider, providerURL, pat string, metadata map[string]interface{}) int64 {
	t.Helper()

	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("failed to marshal metadata: %v", err)
	}

	orgID := getAnyOrgID(t, db)

	query := `
		INSERT INTO integration_tokens (
			provider,
			provider_app_id,
			token_type,
			pat_token,
			access_token,
			connection_name,
			provider_url,
			metadata,
			expires_at,
			org_id,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
		RETURNING id
	`

	var id int64
	err = db.QueryRow(query,
		provider,
		"",
		"PAT",
		pat,
		"NA",
		"Test Token",
		providerURL,
		string(metadataJSON),
		nil,
		orgID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert integration token: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM webhook_registry WHERE integration_token_id = $1", id)
		_, _ = db.Exec("DELETE FROM integration_tokens WHERE id = $1", id)
	})

	return id
}

func insertBitbucketWebhook(t *testing.T, db *sql.DB, repoFullName string, tokenID int64) int64 {
	t.Helper()

	parts := strings.Split(repoFullName, "/")
	projectName := parts[len(parts)-1]

	query := `
		INSERT INTO webhook_registry (
			provider,
			provider_project_id,
			project_name,
			project_full_name,
			webhook_id,
			webhook_url,
			webhook_secret,
			webhook_name,
			events,
			status,
			last_verified_at,
			created_at,
			updated_at,
			integration_token_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`

	now := time.Now()

	var id int64
	err := db.QueryRow(query,
		"bitbucket",
		repoFullName,
		projectName,
		repoFullName,
		"test-webhook",
		"https://example.com/bitbucket-hook",
		"secret",
		"Test Bitbucket Hook",
		"pullrequest:comment_created",
		"automatic",
		now,
		now,
		now,
		tokenID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert webhook registry row: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM webhook_registry WHERE id = $1", id)
	})

	return id
}
