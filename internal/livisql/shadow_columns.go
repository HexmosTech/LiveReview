package livisql

import "strings"

// shadowColumnCache holds every shadow's projected column names, computed
// once at package init.
var shadowColumnCache = buildShadowColumnCache()

// ColumnsFor returns the projected column names for table's shadow, in the
// order they appear in its SELECT list. Nil if table has no shadow.
//
// This is the column allowlist the dbctx-backed schema renderer
// (internal/mcpagent/schema_render.go) filters against before any table
// structure reaches an LLM prompt: dbctx introspects the real Postgres
// tables (it cannot see views - confirmed empirically, see
// internal/mcpagent/dbctx_schema_plan.md), so ColumnsFor is what keeps
// withheld columns like reviews.metadata and users.password_hash out of the
// rendered output, doing in Go at render time what a view's projection
// would have done in the database.
func ColumnsFor(table string) []string {
	return shadowColumnCache[table]
}

func buildShadowColumnCache() map[string][]string {
	out := make(map[string][]string, len(memberShadows)+len(ownerShadows))
	for _, s := range memberShadows {
		out[s.name] = parseSelectColumns(s.body)
	}
	for _, s := range ownerShadows {
		out[s.name] = parseSelectColumns(s.body)
	}
	return out
}

// parseSelectColumns extracts column names from a shadow body's SELECT list.
// Deliberately not a general SQL parser - shadow bodies are hand-written,
// simple SELECT lists (a straight column list, optionally DISTINCT,
// optionally qualified with a single table alias, optionally `AS alias`).
// It assumes every shadow keeps that shape; shadow_columns_test.go is the
// drift guard that catches one that doesn't.
func parseSelectColumns(body string) []string {
	// Collapse the body's newlines/tabs to single spaces first, so keyword
	// search doesn't have to account for the hand-written formatting.
	normalized := strings.Join(strings.Fields(body), " ")
	upper := strings.ToUpper(normalized)

	const selectKW = "SELECT "
	const fromKW = " FROM "
	selectIdx := strings.Index(upper, selectKW)
	fromIdx := strings.Index(upper, fromKW)
	if selectIdx < 0 || fromIdx < 0 || fromIdx <= selectIdx {
		return nil
	}

	list := strings.TrimSpace(normalized[selectIdx+len(selectKW) : fromIdx])
	if strings.HasPrefix(strings.ToUpper(list), "DISTINCT ") {
		list = strings.TrimSpace(list[len("DISTINCT "):])
	}
	if list == "" {
		return nil
	}

	parts := strings.Split(list, ",")
	cols := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// "expr AS alias" -> alias
		if idx := strings.LastIndex(strings.ToUpper(part), " AS "); idx >= 0 {
			part = strings.TrimSpace(part[idx+len(" AS "):])
		}
		// "u.email" -> "email" (single alias-qualified column, no expressions
		// today; a future computed/aliased column must use AS, handled above)
		if idx := strings.LastIndex(part, "."); idx >= 0 {
			part = part[idx+1:]
		}
		if part != "" {
			cols = append(cols, part)
		}
	}
	return cols
}
