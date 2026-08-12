package livisql

import (
	"strings"
	"testing"

	pgquery "github.com/wasilibs/go-pgquery"
)

// Contract: confirm the parser handles the SQL shapes Livi will actually generate,
// exposes a walkable AST, and can deparse back to executable SQL.
func TestParserHandlesRealAnalyticsSQL(t *testing.T) {
	cases := []string{
		`SELECT date_trunc('month', created_at) AS month, count(*) AS review_count FROM reviews WHERE status = 'completed' GROUP BY 1 ORDER BY 1`,
		`SELECT author_username, count(*) FILTER (WHERE status = 'completed') AS done FROM reviews GROUP BY 1 ORDER BY 2 DESC`,
		`WITH monthly AS (SELECT date_trunc('month', created_at) AS m, count(*) AS n FROM reviews GROUP BY 1)
		 SELECT m, n, lag(n) OVER (ORDER BY m) AS prev,
		        round(100.0 * (n - lag(n) OVER (ORDER BY m)) / NULLIF(lag(n) OVER (ORDER BY m), 0), 2) AS pct_change
		 FROM monthly ORDER BY m`,
		`SELECT r.repository, sum(l.billable_loc) AS loc FROM reviews r JOIN loc_usage_ledger l ON l.review_id = r.id GROUP BY 1`,
		`SELECT u.email, count(*) AS n FROM reviews r JOIN users u ON u.email = r.user_email GROUP BY 1`,
		`SELECT extract(dow FROM created_at)::int AS dow, count(*) AS n FROM reviews GROUP BY 1`,
	}
	for i, sql := range cases {
		tree, err := pgquery.Parse(sql)
		if err != nil {
			t.Fatalf("case %d failed to parse: %v\nSQL: %s", i, err, sql)
		}
		if len(tree.Stmts) != 1 {
			t.Fatalf("case %d: expected 1 stmt, got %d", i, len(tree.Stmts))
		}
		sel := tree.Stmts[0].Stmt.GetSelectStmt()
		if sel == nil {
			t.Fatalf("case %d: not a SelectStmt", i)
		}
		out, err := pgquery.Deparse(tree)
		if err != nil {
			t.Fatalf("case %d deparse failed: %v", i, err)
		}
		if !strings.Contains(strings.ToLower(out), "select") {
			t.Fatalf("case %d: deparse produced junk: %s", i, out)
		}
		t.Logf("case %d OK -> %s", i, out)
	}
}

// Contract: confirm the things the guard must reject are detectable.
func TestParserExposesRejectableShapes(t *testing.T) {
	// multi statement
	tree, err := pgquery.Parse(`SELECT 1; DROP TABLE reviews`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(tree.Stmts) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(tree.Stmts))
	}
	t.Logf("multi-statement detected: %d stmts", len(tree.Stmts))

	// schema-qualified name must be visible on the RangeVar
	tree, err = pgquery.Parse(`SELECT * FROM public.reviews`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	from := tree.Stmts[0].Stmt.GetSelectStmt().GetFromClause()
	rv := from[0].GetRangeVar()
	if rv == nil {
		t.Fatal("expected RangeVar")
	}
	t.Logf("schemaname=%q relname=%q inh=%v", rv.Schemaname, rv.Relname, rv.Inh)
	if rv.Schemaname != "public" {
		t.Fatalf("schemaname not exposed, got %q", rv.Schemaname)
	}

	// FROM ONLY must be distinguishable
	tree, _ = pgquery.Parse(`SELECT * FROM ONLY reviews`)
	rvOnly := tree.Stmts[0].Stmt.GetSelectStmt().GetFromClause()[0].GetRangeVar()
	t.Logf("FROM ONLY -> inh=%v", rvOnly.Inh)

	// non-select
	tree, _ = pgquery.Parse(`UPDATE reviews SET status = 'x'`)
	if tree.Stmts[0].Stmt.GetSelectStmt() != nil {
		t.Fatal("UPDATE should not be a SelectStmt")
	}
	t.Log("UPDATE correctly not a SelectStmt")
}

// Contract: the whole security model rests on a prepended CTE shadowing the real
// table name. Confirm the parser round-trips that rewrite.
func TestParserRoundTripsShadowRewrite(t *testing.T) {
	llm := `SELECT date_trunc('month', created_at) AS month, count(*) AS n FROM reviews WHERE org_id = 1 OR 1=1 GROUP BY 1`
	shadowed := `WITH reviews AS NOT MATERIALIZED (SELECT id, created_at, status, author_username, org_id FROM public.reviews WHERE org_id = $1) ` + llm

	tree, err := pgquery.Parse(shadowed)
	if err != nil {
		t.Fatalf("shadowed query failed to parse: %v", err)
	}
	out, err := pgquery.Deparse(tree)
	if err != nil {
		t.Fatalf("deparse failed: %v", err)
	}
	t.Logf("rewritten: %s", out)
	if !strings.Contains(out, "$1") {
		t.Fatal("placeholder lost in deparse")
	}
}
