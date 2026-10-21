package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/livereview/internal/database"
)

func TestConnectorIDFromMetadata(t *testing.T) {
	cases := []struct {
		name    string
		meta    map[string]interface{}
		want    int64
		wantErr bool
	}{
		{"int64", map[string]interface{}{"connector_id": int64(7)}, 7, false},
		{"int", map[string]interface{}{"connector_id": 7}, 7, false},
		{"float64 after json round trip", map[string]interface{}{"connector_id": float64(7)}, 7, false},
		{"json.Number", map[string]interface{}{"connector_id": json.Number("7")}, 7, false},
		{"missing", map[string]interface{}{}, 0, true},
		{"nil map", nil, 0, true},
		{"wrong type", map[string]interface{}{"connector_id": "7"}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := connectorIDFromMetadata(tc.meta)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestConnectorIDFromContext(t *testing.T) {
	if _, ok := connectorIDFromContext(context.Background()); ok {
		t.Fatal("empty context must not carry a connector")
	}
	got, ok := connectorIDFromContext(WithConnectorID(context.Background(), 42))
	if !ok || got != 42 {
		t.Fatalf("got (%d, %v), want (42, true)", got, ok)
	}
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertOrg(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'tenant isolation test') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, id) })
	return id
}

func insertGitHubConnector(t *testing.T, db *sql.DB, orgID int64, pat string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id, token_type, pat_token)
		VALUES ('github', 'tenant-test', 'na', 'tenant-test', 'https://github.com', $1, 'PAT', $2)
		RETURNING id
	`, orgID, pat).Scan(&id)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, id) })
	return id
}

// Two orgs each have a GitHub connector; an event must only ever resolve the connector that received it,
// never "the newest GitHub token" (the old GetLatestGitHubToken behaviour).
func TestFindIntegrationTokenForEvent_IsTenantScoped(t *testing.T) {
	db := testDB(t)
	suffix := time.Now().UnixNano()
	orgA := insertOrg(t, db, fmt.Sprintf("tenant-a-%d", suffix))
	orgB := insertOrg(t, db, fmt.Sprintf("tenant-b-%d", suffix))
	connA := insertGitHubConnector(t, db, orgA, "pat-of-org-a")
	connB := insertGitHubConnector(t, db, orgB, "pat-of-org-b") // created last: the old lookup would pick this one for everyone

	eventFor := func(id interface{}) *UnifiedWebhookEventV2 {
		return &UnifiedWebhookEventV2{Repository: UnifiedRepositoryV2{FullName: "acme/api", Metadata: map[string]interface{}{"connector_id": id}}}
	}

	tokA, err := FindIntegrationTokenForEvent(db, eventFor(float64(connA)))
	if err != nil {
		t.Fatalf("resolve A: %v", err)
	}
	if tokA.PatToken != "pat-of-org-a" || tokA.OrgID != orgA {
		t.Fatalf("event for connector A resolved token %q org %d, want pat-of-org-a org %d", tokA.PatToken, tokA.OrgID, orgA)
	}

	tokB, err := FindIntegrationTokenForEvent(db, eventFor(connB))
	if err != nil {
		t.Fatalf("resolve B: %v", err)
	}
	if tokB.PatToken != "pat-of-org-b" || tokB.OrgID != orgB {
		t.Fatalf("event for connector B resolved token %q org %d, want pat-of-org-b org %d", tokB.PatToken, tokB.OrgID, orgB)
	}

	if _, err := FindIntegrationTokenForEvent(db, &UnifiedWebhookEventV2{Repository: UnifiedRepositoryV2{FullName: "acme/api"}}); err == nil {
		t.Fatal("an event without a connector must fail, not fall back to another tenant's token")
	}
}

// A non-GitHub connector id must not resolve through the GitHub lookup.
func TestFindIntegrationTokenForGitHubConnector_RejectsOtherProviders(t *testing.T) {
	db := testDB(t)
	orgID := insertOrg(t, db, fmt.Sprintf("tenant-c-%d", time.Now().UnixNano()))
	var id int64
	if err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id, pat_token)
		VALUES ('gitlab', 'tenant-test', 'na', 'tenant-test', 'https://gitlab.com', $1, 'x') RETURNING id`, orgID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, id) })

	if _, err := FindIntegrationTokenForGitHubConnector(db, id); err == nil {
		t.Fatal("expected a gitlab connector to be rejected by the GitHub lookup")
	}
}
