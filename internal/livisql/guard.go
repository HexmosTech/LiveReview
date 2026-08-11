// Package livisql validates and rewrites the SQL that Livi's LLM generates for
// analytics questions.
//
// The security model is enforcement, not proof. Checking that a generated query
// "filters by org_id" is not soundly decidable from a parse tree — org_id = 3 OR
// 1=1, a predicate parked in a LEFT JOIN ... ON clause (which does not filter),
// a correlated subquery, or CASE WHEN all defeat a predicate walker, and that
// walker would be the entire tenant boundary.
//
// Instead the query is rewritten. Every allowlisted table is redefined as a CTE
// whose body already applies WHERE org_id = $1, and the generated SELECT is
// nested inside that WITH:
//
//	WITH reviews AS NOT MATERIALIZED (SELECT ... FROM public.reviews WHERE org_id = $1),
//	     users   AS NOT MATERIALIZED (...)
//	SELECT * FROM ( <generated SELECT> ) AS __livi_result
//
// Postgres resolves the unqualified name `reviews` to the CTE, so the org filter
// is applied by the planner no matter what the model wrote. The model's own
// predicates can only further restrict rows inside an already-scoped relation.
// The guard's remaining job is to reject anything that could escape that shadow:
// schema-qualified names, FROM ONLY, CTEs shadowing our shadows, and relations
// or functions outside the allowlist.
package livisql

import (
	"encoding/json"
	"fmt"
	"strings"

	pgquery "github.com/wasilibs/go-pgquery"
)

// maxSQLLen bounds work before the parser is invoked at all.
const maxSQLLen = 8000

// resultAlias names the wrapper subquery. Double-underscored to avoid colliding
// with anything a model would plausibly write.
const resultAlias = "__livi_result"

type Guard struct {
	catalog Catalog
}

func New(catalog Catalog) *Guard {
	return &Guard{catalog: catalog}
}

// Rewrite validates rawSQL and returns an org-scoped query that takes the org
// id as $1. The returned SQL is built from the parser's own deparse of the
// generated statement rather than its original text, so comment, whitespace and
// unicode tricks cannot make the executed statement differ from the validated
// one.
func (g *Guard) Rewrite(rawSQL string) (string, error) {
	stmt, err := g.validate(rawSQL)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("WITH ")
	for i, name := range g.catalog.Tables() {
		if i > 0 {
			b.WriteString(",\n")
		}
		// NOT MATERIALIZED lets the planner inline the CTE, so the real
		// (org_id, created_at) indexes are still used. Without it Postgres 12+
		// materializes each CTE and every query becomes a full table scan.
		fmt.Fprintf(&b, "%s AS NOT MATERIALIZED (%s)", name, g.catalog.shadows[name])
	}
	fmt.Fprintf(&b, "\nSELECT * FROM (%s) AS %s", stmt, resultAlias)

	out := b.String()
	// Re-parse the assembled statement: cheap insurance that the rewrite is
	// syntactically sound before it reaches the database.
	if _, err := pgquery.Parse(out); err != nil {
		return "", reject(CodeDeparse, "I could not build that query. Try a simpler shape.",
			"assembled query failed to parse: %v", err)
	}
	return out, nil
}

// validate runs every check and returns the canonical deparse of the statement.
func (g *Guard) validate(rawSQL string) (string, error) {
	trimmed := strings.TrimSpace(rawSQL)
	if trimmed == "" {
		return "", reject(CodeUnparseable, "Write a SELECT query.", "empty query")
	}
	if len(trimmed) > maxSQLLen {
		return "", reject(CodeTooLong, "That query is too long. Ask for a narrower slice of data.",
			"query is %d bytes, limit %d", len(trimmed), maxSQLLen)
	}

	tree, err := pgquery.Parse(trimmed)
	if err != nil {
		return "", reject(CodeUnparseable, "That is not valid PostgreSQL. Check the syntax and try again.",
			"parse failed: %v", err)
	}
	if len(tree.Stmts) != 1 {
		return "", reject(CodeMultiStatement, "Send exactly one SELECT statement.",
			"expected 1 statement, got %d", len(tree.Stmts))
	}
	if tree.Stmts[0].Stmt.GetSelectStmt() == nil {
		return "", reject(CodeNotSelect, "Only SELECT queries are allowed. You cannot modify data.",
			"top-level statement is not a SELECT")
	}

	// The typed AST answers statement-shape questions; the JSON form is used for
	// the deep walk because every node is a uniform single-key object, which
	// makes the traversal total — no node type can be silently skipped because
	// we forgot to handle its Go struct.
	raw, err := pgquery.ParseToJSON(trimmed)
	if err != nil {
		return "", reject(CodeUnparseable, "That is not valid PostgreSQL. Check the syntax and try again.",
			"parse to json failed: %v", err)
	}
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", reject(CodeUnparseable, "That is not valid PostgreSQL. Check the syntax and try again.",
			"json decode failed: %v", err)
	}

	// Two passes: CTE names must all be known before any relation reference is
	// judged, because JSON object key order is not traversal order and a CTE can
	// otherwise be visited after the reference that resolves against it.
	w := &walker{catalog: g.catalog, cteNames: map[string]bool{}}
	w.collectCTEs(doc)
	if w.err != nil {
		return "", w.err
	}
	w.walk(doc)
	if w.err != nil {
		return "", w.err
	}

	deparsed, err := pgquery.Deparse(tree)
	if err != nil {
		return "", reject(CodeDeparse, "I could not build that query. Try a simpler shape.",
			"deparse failed: %v", err)
	}
	return deparsed, nil
}

type walker struct {
	catalog  Catalog
	cteNames map[string]bool
	err      *RejectionError
}

func (w *walker) fail(e *RejectionError) {
	if w.err == nil {
		w.err = e
	}
}

func (w *walker) walk(node any) {
	if w.err != nil {
		return
	}
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			switch key {
			case "RangeVar":
				w.checkRangeVar(child)
			case "FuncCall":
				w.checkFuncCall(child)
			case "ParamRef":
				// $1 belongs to the rewrite. A generated query carrying its own
				// placeholder would shift the org id's position.
				w.fail(reject(CodePlaceholder, "Do not use bind parameters such as $1. Write literal values.",
					"query contains a parameter placeholder"))
			case "LockingClause":
				w.fail(reject(CodeLocking, "Do not use FOR UPDATE or FOR SHARE.",
					"query contains a locking clause"))
			case "intoClause":
				if child != nil {
					w.fail(reject(CodeSelectInto, "Do not use SELECT INTO.", "query contains an INTO clause"))
				}
			// Node types are capitalized in pg_query's JSON (RangeVar, FuncCall)
			// but struct fields are camelCase, so the CTE list arrives as
			// "withClause", not "WithClause".
			case "withClause":
				if m, ok := child.(map[string]any); ok {
					if rec, _ := m["recursive"].(bool); rec {
						w.fail(reject(CodeRecursiveCTE, "Do not use WITH RECURSIVE.",
							"query contains a recursive CTE"))
					}
				}
			}
			w.walk(child)
		}
	case []any:
		for _, child := range v {
			w.walk(child)
		}
	}
}

func (w *walker) checkRangeVar(node any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	relname, _ := m["relname"].(string)
	schema, _ := m["schemaname"].(string)

	// A schema-qualified name binds to the real table, bypassing the CTE that
	// shares its unqualified name. This is the primary escape route and must be
	// refused outright rather than rewritten.
	if schema != "" {
		w.fail(reject(CodeQualifiedName, "Write table names unqualified, for example `reviews` rather than `public.reviews`.",
			"schema-qualified relation %q.%q", schema, relname))
		return
	}

	// pg_query omits `inh` when false, which is exactly what FROM ONLY produces.
	// ONLY also skips the CTE and reaches the base table.
	if inh, ok := m["inh"].(bool); !ok || !inh {
		w.fail(reject(CodeOnlyModifier, "Do not use FROM ONLY.", "FROM ONLY on relation %q", relname))
		return
	}

	// A name defined as a CTE by the generated query itself is fine; it cannot
	// be one of ours because checkCTE rejects those collisions.
	if w.cteNames[relname] {
		return
	}
	if !w.catalog.Allows(relname) {
		w.fail(reject(CodeUnknownTable,
			fmt.Sprintf("The table %q is not available. You can query: %s.", relname, strings.Join(w.catalog.Tables(), ", ")),
			"relation %q is not in the catalog", relname))
	}
}

// collectCTEs records every CTE name the generated query defines, rejecting any
// that would shadow a catalog table. Run to completion before walk().
func (w *walker) collectCTEs(node any) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if key == "CommonTableExpr" {
				w.checkCTE(child)
			}
			w.collectCTEs(child)
		}
	case []any:
		for _, child := range v {
			w.collectCTEs(child)
		}
	}
}

func (w *walker) checkCTE(node any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	name, _ := m["ctename"].(string)
	if name == "" {
		return
	}
	// A CTE named `reviews` would shadow our shadow and reach the unfiltered
	// table through it. Refuse the collision instead of trying to resolve scope.
	if w.catalog.Allows(name) {
		w.fail(reject(CodeCTEShadow,
			fmt.Sprintf("Do not name a CTE %q — that is a table name. Pick a different alias.", name),
			"CTE %q shadows a catalog table", name))
		return
	}
	w.cteNames[name] = true
}

func (w *walker) checkFuncCall(node any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	parts, ok := m["funcname"].([]any)
	if !ok || len(parts) == 0 {
		return
	}
	// A qualified function name (pg_catalog.pg_read_file) arrives as multiple
	// parts; judge the final element and reject any explicit qualification.
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		s, ok := pm["String"].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := s["sval"].(string); ok {
			names = append(names, strings.ToLower(v))
		}
	}
	if len(names) == 0 {
		return
	}
	// SQL-standard constructs are normalized by the parser into explicitly
	// qualified calls — extract(dow FROM ts) becomes pg_catalog.extract — so
	// pg_catalog qualification has to be tolerated. It grants nothing: every
	// builtin already lives there and is always in scope, and the allowlist
	// check below still applies to the bare name, so pg_catalog.pg_read_file is
	// refused exactly like pg_read_file is.
	if len(names) == 2 && names[0] == "pg_catalog" {
		names = names[1:]
	}
	if len(names) > 1 {
		w.fail(reject(CodeUnknownFunc, "Use unqualified function names.",
			"schema-qualified function %q", strings.Join(names, ".")))
		return
	}
	if !functionAllowed(names[0]) {
		w.fail(reject(CodeUnknownFunc,
			fmt.Sprintf("The function %q is not available. Use standard aggregate, date and string functions.", names[0]),
			"function %q is not in the allowlist", names[0]))
	}
}
