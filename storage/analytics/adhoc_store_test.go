package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/livereview/internal/database"
	"github.com/livereview/internal/livisql"
)

// testDB opens a connection to the real dev Postgres (via DATABASE_URL / .env),
// matching this codebase's no-testcontainers, real-DB testing convention.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testOrgScopedColumns stands in for the live dbctx-sourced map
// mcpagent.orgScopedColumns normally supplies to livisql.CatalogFor - just
// enough columns for the queries this file's tests exercise. users is not
// listed: it is one of CatalogFor's two hand-written specials (no org_id
// column at all), always present regardless of this map.
func testOrgScopedColumns() map[string][]string {
	return map[string][]string{
		"reviews": {"id", "org_id", "created_at", "completed_at", "status"},
	}
}

func rewrite(t *testing.T, sql string) string {
	t.Helper()
	out, err := livisql.New(livisql.CatalogFor(testOrgScopedColumns())).Rewrite(sql)
	if err != nil {
		t.Fatalf("guard rejected a query the test expects to run: %v", err)
	}
	return out
}

// orgsWithReviews finds two org ids that both have reviews, so the isolation
// assertions have real data on both sides rather than trivially passing.
func orgsWithReviews(t *testing.T, db *sql.DB) (int64, int64) {
	t.Helper()
	rows, err := db.Query(`SELECT org_id FROM reviews GROUP BY org_id HAVING count(*) > 0 ORDER BY count(*) DESC LIMIT 2`)
	if err != nil {
		t.Skipf("skipping: cannot read reviews: %v", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan org id: %v", err)
		}
		ids = append(ids, id)
	}
	if len(ids) < 2 {
		t.Skip("skipping: need two orgs with reviews to test isolation")
	}
	return ids[0], ids[1]
}

// The core guarantee: the same generated SQL, bound to two different orgs, must
// never return the other org's rows — even when the query tries to widen its own
// WHERE clause.
func TestQueryIsolatesTenants(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	ctx := context.Background()
	orgA, orgB := orgsWithReviews(t, db)

	// Ground truth straight from the table, bypassing the guard entirely.
	truth := func(orgID int64) int64 {
		var n int64
		if err := db.QueryRow(`SELECT count(*) FROM reviews WHERE org_id = $1`, orgID).Scan(&n); err != nil {
			t.Fatalf("ground truth query failed: %v", err)
		}
		return n
	}

	for _, attack := range []string{
		`SELECT count(*) AS n FROM reviews`,
		`SELECT count(*) AS n FROM reviews WHERE org_id = 1 OR 1=1`,
		`SELECT count(*) AS n FROM reviews WHERE true`,
	} {
		q := rewrite(t, attack)
		for _, orgID := range []int64{orgA, orgB} {
			got, err := store.Count(ctx, orgID, q)
			if err != nil {
				t.Fatalf("count failed for org %d: %v", orgID, err)
			}
			if want := truth(orgID); got != want {
				t.Fatalf("org %d: query %q returned %d, expected exactly that org's %d rows",
					orgID, attack, got, want)
			}
		}
	}
}

// Writes must fail at the database, not merely at the guard. The table is
// schema-qualified on purpose: an unqualified name is stopped earlier by
// search_path='', which would make this pass without exercising read-only at
// all. Qualifying it isolates the layer under test.
func TestReadOnlyTransactionRefusesWrites(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)

	// Deliberately bypasses the guard: this asserts the second line of defence.
	_, err := store.Query(context.Background(), 1,
		`INSERT INTO public.reviews (repository, status, org_id) VALUES ('guard-test', 'created', $1)`, 10)
	if err == nil {
		t.Fatal("an INSERT succeeded inside a read-only transaction")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected a read-only transaction error, got: %v", err)
	}
}

// search_path='' means an unqualified relation that is not a shadow CTE cannot
// resolve, so a query that slipped past the guard still cannot reach a table.
func TestUnshadowedTableIsUnreachable(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)

	_, err := store.Query(context.Background(), 1, `SELECT count(*) AS n FROM api_keys`, 10)
	if err == nil {
		t.Fatal("reached a table with no shadow CTE")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected an unresolved relation error, got: %v", err)
	}
}

// The users shadow must make credential columns unreachable at the database, not
// just absent from the rewrite text.
func TestUserSecretsUnreachable(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	orgA, _ := orgsWithReviews(t, db)

	q := rewrite(t, `SELECT password_hash FROM users`)
	_, err := store.Query(context.Background(), orgA, q, 10)
	if err == nil {
		t.Fatal("password_hash was readable through the users shadow")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a missing-column error, got: %v", err)
	}
}

// Every value that reaches a chart must survive json.Marshal — this is the test
// that would have caught NaN, []byte numerics and driver-local timestamps.
func TestCoercionProducesMarshalableRows(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	orgA, _ := orgsWithReviews(t, db)

	q := rewrite(t, `
		SELECT date_trunc('month', created_at) AS bucket,
		       count(*) AS n,
		       avg(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completion_rate,
		       max(completed_at) AS last_completed,
		       bool_or(status = 'failed') AS any_failed
		FROM reviews GROUP BY 1 ORDER BY 1`)

	rs, err := store.Query(context.Background(), orgA, q, 500)
	if err != nil {
		// bool_or is not in the function allowlist by design; if the guard
		// refused it the rewrite above would have failed, so a failure here is
		// an execution problem worth surfacing.
		t.Fatalf("query failed: %v", err)
	}
	if len(rs.Rows) == 0 {
		t.Skip("no rows to coerce")
	}

	if _, err := json.Marshal(rs.Rows); err != nil {
		t.Fatalf("rows are not JSON-marshalable: %v", err)
	}
	for _, row := range rs.Rows {
		if b, ok := row["bucket"].(string); !ok || !strings.Contains(b, "T") {
			t.Fatalf("timestamp was not normalized to RFC3339: %#v", row["bucket"])
		}
		if _, ok := row["completion_rate"].(float64); !ok && row["completion_rate"] != nil {
			t.Fatalf("numeric avg() was not coerced to a number: %#v", row["completion_rate"])
		}
	}
}

// Truncated must distinguish "exactly at the cap" from "more than the cap",
// because showing a silently-clipped result as if it were complete is the
// failure mode this whole feature exists to avoid.
func TestTruncationIsDetected(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	orgA, _ := orgsWithReviews(t, db)

	q := rewrite(t, `SELECT id FROM reviews ORDER BY id`)

	rs, err := store.Query(context.Background(), orgA, q, 2)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	var total int64
	if err := db.QueryRow(`SELECT count(*) FROM reviews WHERE org_id = $1`, orgA).Scan(&total); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if total > 2 && !rs.Truncated {
		t.Fatalf("expected Truncated with %d rows available and a cap of 2", total)
	}
	if len(rs.Rows) > 2 {
		t.Fatalf("cap exceeded: got %d rows", len(rs.Rows))
	}
}

// Duplicate output columns would silently overwrite each other in the row map.
func TestDuplicateColumnsRejected(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	orgA, _ := orgsWithReviews(t, db)

	q := rewrite(t, `SELECT count(*) AS n, count(*) AS n FROM reviews`)
	if _, err := store.Query(context.Background(), orgA, q, 10); err == nil {
		t.Fatal("duplicate output columns were accepted")
	}
}
