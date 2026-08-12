package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

// testTables stands in for the live dbctx-sourced table list
// mcpagent.allTableNames normally supplies to livisql.CatalogFor - just
// enough tables for the queries this file's tests exercise.
func testTables() []string {
	return []string{"reviews", "users"}
}

func rewrite(t *testing.T, orgID int64, sql string) string {
	t.Helper()
	out, err := livisql.New(livisql.CatalogFor(testTables()), orgID).Rewrite(sql)
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

// There is no structural org-scoping anymore (see internal/livisql's package
// doc): a query's org_id filter is whatever literal the model wrote, checked
// by the guard only for presence and for the OR-1=1 tautology shape. This
// test asserts what the guard DOES still catch, and separately proves an
// honestly-scoped query returns only that literal org's rows - ordinary SQL
// behavior, not something this package enforces.
func TestGuardRejectsUnscopedAndTautologousQueries(t *testing.T) {
	g := livisql.New(livisql.CatalogFor(testTables()), 1)

	for _, attack := range []string{
		`SELECT count(*) AS n FROM reviews`,
		`SELECT count(*) AS n FROM reviews WHERE org_id = 1 OR 1=1`,
		`SELECT count(*) AS n FROM reviews WHERE org_id = 1 OR TRUE`,
	} {
		if _, err := g.Rewrite(attack); err == nil {
			t.Fatalf("expected %q to be rejected, it was accepted", attack)
		}
	}
}

// An honestly org-scoped query only returns that org's rows. This is
// ordinary SQL, not a guarantee this package makes - see the test above and
// internal/livisql's package doc for what actually still limits the model.
func TestHonestlyScopedQueryReturnsOnlyThatOrg(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	ctx := context.Background()
	orgA, orgB := orgsWithReviews(t, db)

	truth := func(orgID int64) int64 {
		var n int64
		if err := db.QueryRow(`SELECT count(*) FROM reviews WHERE org_id = $1`, orgID).Scan(&n); err != nil {
			t.Fatalf("ground truth query failed: %v", err)
		}
		return n
	}

	for _, orgID := range []int64{orgA, orgB} {
		q := rewrite(t, orgID, fmt.Sprintf(`SELECT count(*) AS n FROM reviews WHERE org_id = %d`, orgID))
		got, err := store.Count(ctx, q)
		if err != nil {
			t.Fatalf("count failed for org %d: %v", orgID, err)
		}
		if want := truth(orgID); got != want {
			t.Fatalf("org %d: query returned %d, expected exactly that org's %d rows", orgID, got, want)
		}
	}
}

// Writes must fail at the database, not merely at the guard - the second
// line of defence. This deliberately bypasses the guard (store.Query is
// called directly with an INSERT), and is schema-qualified because that is
// now a query the guard itself would also tolerate (search_path is
// 'public', not empty - see adhoc_store.go) - qualifying it here just keeps
// this test's SQL valid regardless of the guard's own opinion of it, since
// the whole point is to bypass the guard entirely.
func TestReadOnlyTransactionRefusesWrites(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)

	_, err := store.Query(context.Background(),
		`INSERT INTO public.reviews (repository, status, org_id) VALUES ('guard-test', 'created', 999999999)`, 10)
	if err == nil {
		t.Fatal("an INSERT succeeded inside a read-only transaction")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected a read-only transaction error, got: %v", err)
	}
}

// The guard is the only thing keeping a credential column out of a result
// now (there is no shadow projection to fall back on if it has a bug - see
// internal/livisql's package doc). This asserts the guard itself refuses
// the reference; it does not and cannot assert the column is unreachable at
// the database if the guard were wrong, because nothing else stops it
// anymore.
func TestUserSecretColumnsRejectedByGuard(t *testing.T) {
	g := livisql.New(livisql.CatalogFor(testTables()), 1)
	for _, sql := range []string{
		`SELECT password_hash FROM users WHERE org_id = 1`,
		`SELECT * FROM users WHERE org_id = 1`,
	} {
		if _, err := g.Rewrite(sql); err == nil {
			t.Fatalf("expected %q to be rejected, it was accepted", sql)
		}
	}
}

// Every value that reaches a chart must survive json.Marshal — this is the test
// that would have caught NaN, []byte numerics and driver-local timestamps.
func TestCoercionProducesMarshalableRows(t *testing.T) {
	db := testDB(t)
	store := NewAdHocStore(db)
	orgA, _ := orgsWithReviews(t, db)

	q := rewrite(t, orgA, fmt.Sprintf(`
		SELECT date_trunc('month', created_at) AS bucket,
		       count(*) AS n,
		       avg(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completion_rate,
		       max(completed_at) AS last_completed,
		       bool_or(status = 'failed') AS any_failed
		FROM reviews WHERE org_id = %d GROUP BY 1 ORDER BY 1`, orgA))

	rs, err := store.Query(context.Background(), q, 500)
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

	q := rewrite(t, orgA, fmt.Sprintf(`SELECT id FROM reviews WHERE org_id = %d ORDER BY id`, orgA))

	rs, err := store.Query(context.Background(), q, 2)
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

	q := rewrite(t, orgA, fmt.Sprintf(`SELECT count(*) AS n, count(*) AS n FROM reviews WHERE org_id = %d`, orgA))
	if _, err := store.Query(context.Background(), q, 10); err == nil {
		t.Fatal("duplicate output columns were accepted")
	}
}
