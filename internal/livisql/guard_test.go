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

// testOrgScopedColumns stands in for the live dbctx-sourced map CatalogFor
// normally gets from mcpagent.orgScopedColumns - just enough columns for the
// queries these tests exercise. api_keys and auth_tokens are deliberately
// absent: neither has an org_id column in the real schema, so they are
// never auto-scoped, matching the "unknown table" rejections below.
func testOrgScopedColumns() map[string][]string {
	return map[string][]string{
		"reviews": {"id", "org_id", "repository", "status", "created_at",
			"author_username", "user_email"},
		"loc_usage_ledger": {"id", "org_id", "billable_loc"},
		"upgrade_requests": {"id", "org_id", "status"},
	}
}

// The queries a guard must refuse. Each one is an escape route: if any of these
// starts passing, tenant isolation is broken.
func TestRewriteRejects(t *testing.T) {
	g := New(CatalogFor(testOrgScopedColumns()))

	cases := []struct {
		name string
		sql  string
		want RejectionCode
	}{
		{"schema qualified reaches the real table",
			`SELECT count(*) FROM public.reviews`, CodeQualifiedName},
		{"pg_catalog probing",
			`SELECT relname FROM pg_catalog.pg_class`, CodeQualifiedName},
		{"FROM ONLY skips the CTE",
			`SELECT count(*) FROM ONLY reviews`, CodeOnlyModifier},
		{"CTE shadows our shadow",
			`WITH reviews AS (SELECT * FROM orgs) SELECT count(*) FROM reviews`, CodeCTEShadow},
		{"stacked statement",
			`SELECT 1; DROP TABLE reviews`, CodeMultiStatement},
		{"update is not a select",
			`UPDATE reviews SET status = 'x'`, CodeNotSelect},
		{"insert is not a select",
			`INSERT INTO reviews (repository) VALUES ('x')`, CodeNotSelect},
		{"delete is not a select",
			`DELETE FROM reviews`, CodeNotSelect},
		{"unknown table",
			`SELECT * FROM api_keys`, CodeUnknownTable},
		{"auth token table",
			`SELECT * FROM auth_tokens`, CodeUnknownTable},
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
			`SELECT count(*) FILTER (WHERE pg_sleep(1) IS NULL) FROM reviews`, CodeUnknownFunc},
		{"disallowed function hidden inside a window OVER clause",
			`SELECT lag(pg_read_file('/etc/passwd')) OVER (ORDER BY id) FROM reviews`, CodeUnknownFunc},
		{"disallowed function hidden inside a subquery",
			`SELECT count(*) FROM (SELECT pg_sleep(1) FROM reviews) t`, CodeUnknownFunc},
		{"recursive cte",
			`WITH RECURSIVE t AS (SELECT 1 AS n UNION ALL SELECT n+1 FROM t WHERE n < 5) SELECT * FROM t`, CodeRecursiveCTE},
		{"locking clause",
			`SELECT * FROM reviews FOR UPDATE`, CodeLocking},
		{"bind parameter would displace the org id",
			`SELECT count(*) FROM reviews WHERE org_id = $1`, CodePlaceholder},
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

// Queries that must survive the guard: the ones Livi actually needs to answer
// CTO questions. A guard that rejects these is useless even if it is safe.
func TestRewriteAccepts(t *testing.T) {
	g := New(CatalogFor(testOrgScopedColumns()))

	cases := []struct {
		name string
		sql  string
	}{
		{"reviews per month",
			`SELECT date_trunc('month', created_at) AS month, count(*) AS review_count
			 FROM reviews WHERE status = 'completed' GROUP BY 1 ORDER BY 1`},
		{"top reviewers",
			`SELECT author_username, count(*) AS n FROM reviews GROUP BY 1 ORDER BY 2 DESC`},
		{"aggregate filter",
			`SELECT count(*) FILTER (WHERE status = 'completed') AS done FROM reviews`},
		{"window function percentage change",
			`WITH monthly AS (SELECT date_trunc('month', created_at) AS m, count(*) AS n FROM reviews GROUP BY 1)
			 SELECT m, n, round(100.0 * (n - lag(n) OVER (ORDER BY m)) / NULLIF(lag(n) OVER (ORDER BY m), 0), 2) AS pct
			 FROM monthly ORDER BY m`},
		{"join across two scoped tables",
			`SELECT u.email, count(*) AS n FROM reviews r JOIN users u ON u.email = r.user_email GROUP BY 1`},
		{"cast and extract",
			`SELECT extract(dow FROM created_at)::int AS dow, count(*) AS n FROM reviews GROUP BY 1`},
		{"count wrapped in a subquery, as the plan prompt teaches",
			`SELECT count(*) AS n FROM (SELECT date_trunc('month', created_at) FROM reviews GROUP BY 1) t`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := g.Rewrite(tc.sql)
			if err != nil {
				t.Fatalf("unexpected rejection: %v", err)
			}
			if !strings.Contains(out, "$1") {
				t.Fatal("rewrite lost the org placeholder")
			}
			if !strings.Contains(out, "NOT MATERIALIZED") {
				t.Fatal("rewrite lost the shadow CTEs")
			}
			if !strings.Contains(out, resultAlias) {
				t.Fatal("rewrite lost the wrapper subquery")
			}
		})
	}
}

// The org filter must be structural: an injected OR cannot widen it, because it
// only ever applies inside an already-scoped relation.
func TestRewriteNeutralizesOrInjection(t *testing.T) {
	g := New(CatalogFor(testOrgScopedColumns()))
	out, err := g.Rewrite(`SELECT count(*) FROM reviews WHERE org_id = 3 OR 1=1`)
	if err != nil {
		t.Fatalf("query should be accepted and neutralized, not rejected: %v", err)
	}
	if !strings.Contains(out, "WHERE org_id = $1") {
		t.Fatalf("shadow filter missing from rewrite:\n%s", out)
	}
	// The model's predicate survives, but only inside the scoped CTE.
	if !strings.Contains(out, resultAlias) {
		t.Fatalf("wrapper missing:\n%s", out)
	}
}

// CatalogFor's table set comes entirely from orgScopedColumns
// (auto-generated from the live schema) plus the two hand-written specials -
// there is no role input at all (see the Role doc comment in catalog.go).
func TestCatalogFor(t *testing.T) {
	cols := testOrgScopedColumns()
	cat := CatalogFor(cols)

	for _, table := range []string{"reviews", "loc_usage_ledger", "orgs", "users"} {
		if !cat.Allows(table) {
			t.Fatalf("table %q must be visible", table)
		}
	}

	if _, err := New(cat).Rewrite(`SELECT sum(billable_loc) AS loc FROM loc_usage_ledger`); err != nil {
		t.Fatalf("billing query should be accepted: %v", err)
	}

	// upgrade_requests is on deniedTables even though testOrgScopedColumns
	// supplies it - deniedTables must win over orgScopedColumns.
	if cat.Allows("upgrade_requests") {
		t.Fatal("a table on deniedTables must stay invisible even when orgScopedColumns supplies it")
	}

	// A table absent from orgScopedColumns (no org_id in the real schema, or
	// simply not supplied) stays invisible.
	if cat.Allows("api_keys") {
		t.Fatal("a table absent from orgScopedColumns must not become visible")
	}
}

// AutoOrgScopedShadow must never project a denylisted column, and must
// refuse to produce a shadow with nothing left to select.
func TestAutoOrgScopedShadowWithholdsSecrets(t *testing.T) {
	s, ok := AutoOrgScopedShadow("api_keys", []string{"id", "org_id", "key_hash", "key_prefix", "label"})
	if !ok {
		t.Fatal("expected a shadow with non-secret columns remaining")
	}
	if strings.Contains(s.body, "key_hash") {
		t.Fatalf("shadow projects a denylisted column:\n%s", s.body)
	}
	for _, want := range []string{"id", "org_id", "key_prefix", "label"} {
		if !strings.Contains(s.body, want) {
			t.Fatalf("shadow dropped a legitimate column %q:\n%s", want, s.body)
		}
	}

	if _, ok := AutoOrgScopedShadow("secrets_only", []string{"api_key"}); ok {
		t.Fatal("a table with nothing left after denylisting must not produce a shadow")
	}
}

// The users shadow must not project credentials. This asserts the column is
// absent from the rewrite; the companion DB test proves Postgres then rejects
// the reference.
func TestUsersShadowWithholdsSecrets(t *testing.T) {
	g := New(CatalogFor(testOrgScopedColumns()))
	out, err := g.Rewrite(`SELECT email FROM users`)
	if err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	for _, secret := range []string{"password_hash", "onboarding_api_key", "password_reset_required"} {
		if strings.Contains(out, secret) {
			t.Fatalf("users shadow projects %q:\n%s", secret, out)
		}
	}
	// And it must be scoped through membership, since users has no org_id.
	if !strings.Contains(out, "ur.org_id = $1") {
		t.Fatalf("users shadow is not scoped through user_roles:\n%s", out)
	}
}

// A CTE the model defines itself is legitimate and must resolve even though the
// guard collects CTE names in a separate pass from the reference check.
func TestOwnCTEResolvesRegardlessOfWalkOrder(t *testing.T) {
	g := New(CatalogFor(testOrgScopedColumns()))
	for i := 0; i < 50; i++ {
		if _, err := g.Rewrite(
			`WITH per_month AS (SELECT date_trunc('month', created_at) AS m FROM reviews)
			 SELECT m, count(*) FROM per_month GROUP BY 1`); err != nil {
			t.Fatalf("iteration %d: own CTE was rejected: %v", i, err)
		}
	}
}
