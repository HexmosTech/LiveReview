package livisql

import (
	"errors"
	"strings"
	"testing"
)

func codeOf(t *testing.T, err error) RejectionCode {
	t.Helper()
	var re *RejectionError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RejectionError, got %T: %v", err, err)
	}
	return re.Code
}

// testTables stands in for the live dbctx-sourced table list CatalogFor
// normally gets from mcpagent.allTableNames - just enough tables for the
// queries these tests exercise. api_keys and auth_tokens are deliberately
// absent, matching the "unknown table" rejections below.
func testTables() []string {
	return []string{"reviews", "loc_usage_ledger", "upgrade_requests", "users", "orgs"}
}

// testOrgID is the guard's configured org id in every test in this file -
// every accepted query below filters by this exact value.
const testOrgID = int64(7)

// The queries a guard must refuse. Each one is an escape route: if any of these
// starts passing, a category of risk this guard exists for is unenforced.
// Every SQL fragment here filters by testOrgID itself so the case exercises
// exactly one rejection reason, not an incidental CodeWrongOrg from an
// unrelated literal - see TestRewriteRejectsWrongOrgID for that check.
func TestRewriteRejects(t *testing.T) {
	g := New(CatalogFor(testTables()), testOrgID)

	cases := []struct {
		name string
		sql  string
		want RejectionCode
	}{
		{"cross-schema reaches pg_catalog",
			`SELECT relname FROM pg_catalog.pg_class WHERE org_id = 7`, CodeCrossSchema},
		{"stacked statement",
			`SELECT 1; DROP TABLE reviews`, CodeMultiStatement},
		{"update is not a select",
			`UPDATE reviews SET status = 'x'`, CodeNotSelect},
		{"insert is not a select",
			`INSERT INTO reviews (repository) VALUES ('x')`, CodeNotSelect},
		{"delete is not a select",
			`DELETE FROM reviews`, CodeNotSelect},
		{"unknown table",
			`SELECT id FROM api_keys WHERE org_id = 7`, CodeUnknownTable},
		{"auth token table",
			`SELECT id FROM auth_tokens WHERE org_id = 7`, CodeUnknownTable},
		{"file read function",
			`SELECT pg_read_file('/etc/passwd')`, CodeUnknownFunc},
		{"sleep function",
			`SELECT pg_sleep(60)`, CodeUnknownFunc},
		{"dblink exfiltration",
			`SELECT dblink('host=evil', 'SELECT 1')`, CodeUnknownFunc},
		{"current_setting leaks config",
			`SELECT current_setting('is_superuser')`, CodeUnknownFunc},
		{"qualified function bypasses the name check",
			`SELECT pg_catalog.pg_read_file('/etc/passwd')`, CodeUnknownFunc},
		{"disallowed function hidden inside a FILTER clause",
			`SELECT count(*) FILTER (WHERE pg_sleep(1) IS NULL) FROM reviews WHERE org_id = 7`, CodeUnknownFunc},
		{"disallowed function hidden inside a window OVER clause",
			`SELECT lag(pg_read_file('/etc/passwd')) OVER (ORDER BY id) FROM reviews WHERE org_id = 7`, CodeUnknownFunc},
		{"disallowed function hidden inside a subquery",
			`SELECT count(*) FROM (SELECT pg_sleep(1) FROM reviews WHERE org_id = 7) t`, CodeUnknownFunc},
		{"recursive cte",
			`WITH RECURSIVE t AS (SELECT 1 AS n UNION ALL SELECT n+1 FROM t WHERE n < 5) SELECT n FROM t`, CodeRecursiveCTE},
		{"locking clause",
			`SELECT id FROM reviews WHERE org_id = 7 FOR UPDATE`, CodeLocking},
		{"bind parameter",
			`SELECT count(*) FROM reviews WHERE org_id = $1`, CodePlaceholder},
		{"star select can smuggle a secret column",
			`SELECT * FROM users WHERE org_id = 7`, CodeStarSelect},
		{"qualified star select",
			`SELECT u.* FROM users u WHERE org_id = 7`, CodeStarSelect},
		{"secret column referenced by name",
			`SELECT password_hash FROM users WHERE org_id = 7`, CodeSecretColumn},
		{"secret column referenced in a predicate",
			`SELECT count(*) FROM users WHERE org_id = 7 AND api_key = 'x'`, CodeSecretColumn},
		{"missing org filter entirely",
			`SELECT count(*) FROM reviews`, CodeMissingOrgFilter},
		{"classic tautology bypass",
			`SELECT count(*) FROM reviews WHERE org_id = 7 OR 1=1`, CodeTautology},
		{"string tautology bypass",
			`SELECT count(*) FROM reviews WHERE org_id = 7 OR 'a'='a'`, CodeTautology},
		{"bare boolean literal bypass",
			`SELECT count(*) FROM reviews WHERE org_id = 7 OR TRUE`, CodeTautology},
		{"empty query",
			`   `, CodeUnparseable},
		{"garbage",
			`not sql at all`, CodeUnparseable},
		{"oversized query",
			`SELECT ` + strings.Repeat("1,", 5000) + `1`, CodeTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := g.Rewrite(tc.sql)
			if err == nil {
				t.Fatalf("expected rejection, query was accepted and rewritten to:\n%s", out)
			}
			if got := codeOf(t, err); got != tc.want {
				t.Fatalf("wrong rejection code: got %s want %s (%v)", got, tc.want, err)
			}
		})
	}
}

// A query filtering by a different org's id must be rejected even though it
// is otherwise a perfectly well-formed, org_id-filtered query - this is the
// check that closes most of "presence of a filter isn't the same as the
// right filter" (see the package doc for what it still doesn't close).
func TestRewriteRejectsWrongOrgID(t *testing.T) {
	g := New(CatalogFor(testTables()), testOrgID)

	cases := []struct {
		name string
		sql  string
	}{
		{"wrong literal org id",
			`SELECT count(*) AS n FROM reviews WHERE org_id = 999`},
		{"wrong org id as a quoted string",
			`SELECT count(*) AS n FROM reviews WHERE org_id = '999'`},
		{"wrong org id hidden behind a true predicate that also passes",
			`SELECT count(*) AS n FROM reviews WHERE org_id = 7 AND id = 1 OR org_id = 999`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := g.Rewrite(tc.sql)
			if err == nil {
				t.Fatalf("expected rejection, query was accepted and rewritten to:\n%s", out)
			}
			if got := codeOf(t, err); got != CodeWrongOrg {
				t.Fatalf("wrong rejection code: got %s want %s (%v)", got, CodeWrongOrg, err)
			}
		})
	}
}

// A join condition tying two tables' own org_id columns together (rather
// than comparing org_id to a literal) has no fixed value to check and must
// not be rejected as a "wrong org" - see orgIDComparisonOtherSide.
func TestRewriteAcceptsOrgIDJoinCondition(t *testing.T) {
	g := New(CatalogFor(testTables()), testOrgID)
	_, err := g.Rewrite(
		`SELECT r.id FROM reviews r JOIN loc_usage_ledger l ON l.org_id = r.org_id WHERE r.org_id = 7`)
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
}

// Queries that must survive the guard: the ones Livi actually needs to answer
// CTO questions, each carrying its own explicit org_id filter (there is no
// automatic injection anymore - see the package doc).
func TestRewriteAccepts(t *testing.T) {
	g := New(CatalogFor(testTables()), testOrgID)

	cases := []struct {
		name string
		sql  string
	}{
		{"reviews per month",
			`SELECT date_trunc('month', created_at) AS month, count(*) AS review_count
			 FROM reviews WHERE status = 'completed' AND org_id = 7 GROUP BY 1 ORDER BY 1`},
		{"top reviewers",
			`SELECT author_username, count(*) AS n FROM reviews WHERE org_id = 7 GROUP BY 1 ORDER BY 2 DESC`},
		{"aggregate filter",
			`SELECT count(*) FILTER (WHERE status = 'completed') AS done FROM reviews WHERE org_id = 7`},
		{"window function percentage change",
			`WITH monthly AS (SELECT date_trunc('month', created_at) AS m, count(*) AS n FROM reviews WHERE org_id = 7 GROUP BY 1)
			 SELECT m, n, round(100.0 * (n - lag(n) OVER (ORDER BY m)) / NULLIF(lag(n) OVER (ORDER BY m), 0), 2) AS pct
			 FROM monthly ORDER BY m`},
		{"join across two tables, org filter on one side",
			`SELECT r.author_username, count(*) AS n FROM reviews r JOIN loc_usage_ledger l ON l.id = r.id
			 WHERE r.org_id = 7 GROUP BY 1`},
		{"cast and extract",
			`SELECT extract(dow FROM created_at)::int AS dow, count(*) AS n FROM reviews WHERE org_id = 7 GROUP BY 1`},
		{"count wrapped in a subquery, as the plan prompt teaches",
			`SELECT count(*) AS n FROM (SELECT date_trunc('month', created_at) FROM reviews WHERE org_id = 7 GROUP BY 1) t`},
		{"public-qualified table name is tolerated",
			`SELECT count(*) AS n FROM public.reviews WHERE org_id = 7`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := g.Rewrite(tc.sql)
			if err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
			if !strings.Contains(out, "org_id") {
				t.Fatal("rewrite lost the org_id filter")
			}
		})
	}
}

// CatalogFor's table set comes entirely from allTables minus deniedTables -
// there is no role input at all (see the Role doc comment in catalog.go).
func TestCatalogFor(t *testing.T) {
	cat := CatalogFor(testTables())

	for _, table := range []string{"reviews", "loc_usage_ledger", "orgs", "users"} {
		if !cat.Allows(table) {
			t.Fatalf("table %q must be visible", table)
		}
	}

	if _, err := New(cat, testOrgID).Rewrite(`SELECT sum(billable_loc) AS loc FROM loc_usage_ledger WHERE org_id = 7`); err != nil {
		t.Fatalf("billing query should be accepted: %v", err)
	}

	// upgrade_requests is on deniedTables even though testTables supplies it -
	// deniedTables must win.
	if cat.Allows("upgrade_requests") {
		t.Fatal("a table on deniedTables must stay invisible even when supplied")
	}

	// A table absent from allTables stays invisible.
	if cat.Allows("api_keys") {
		t.Fatal("a table absent from allTables must not become visible")
	}
}

// A CTE the model defines itself is legitimate and must resolve even though the
// guard collects CTE names in a separate pass from the reference check.
func TestOwnCTEResolvesRegardlessOfWalkOrder(t *testing.T) {
	g := New(CatalogFor(testTables()), testOrgID)
	for i := 0; i < 50; i++ {
		if _, err := g.Rewrite(
			`WITH per_month AS (SELECT date_trunc('month', created_at) AS m FROM reviews WHERE org_id = 7)
			 SELECT m, count(*) FROM per_month GROUP BY 1`); err != nil {
			t.Fatalf("iteration %d: own CTE was rejected: %v", i, err)
		}
	}
}
