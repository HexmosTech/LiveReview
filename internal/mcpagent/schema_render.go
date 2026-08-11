package mcpagent

import (
	"fmt"
	"strings"

	"github.com/livereview/internal/livisql"
	"github.com/shrsv/dbctx"
)

// dbctxTableText renders the "### Tables" section of the count_query
// branch's system prompt from the live dbctx index, filtered through
// catalog.go's column allowlist (livisql.ColumnsFor) and role's visible
// table set (livisql.CatalogFor). See dbctx_schema_plan.md for the two rules
// this function exists to enforce:
//
//  1. Never render a column that isn't in the shadow's own SELECT list -
//     dbctx introspects the real tables (it cannot see views), so this Go-side
//     filter is what keeps reviews.metadata, users.password_hash and
//     users.onboarding_api_key out of the prompt, the same way a view's
//     projection would have.
//  2. Never render ColumnInfo.Values or a JSONBPathInfo's SampleValues -
//     both come from dbctx's pg_stats/TABLESAMPLE sampling, which runs over
//     the whole table with no org_id filter and is therefore not safe to
//     reuse verbatim in a multi-tenant prompt. Path + InferredType (the
//     JSONB key structure) IS rendered - see dbctx_schema_plan.md's "Why the
//     model doesn't need to see a row..." section for why that's sufficient
//     to avoid hallucinated JSON paths without reopening this.
//
// Returns the fallback text and a non-nil error when the live index isn't
// available. The caller is expected to log that via
// ChatTurnLogger.SchemaSourceDegraded - this function has no session/turn
// context of its own to log through.
func dbctxTableText(role livisql.Role) (string, error) {
	idx := schemaIndex()
	if idx == nil {
		return analyticsSchemaFallback, fmt.Errorf("dbctx index unavailable")
	}

	tables := livisql.CatalogFor(role).Tables()
	visible := make(map[string]bool, len(tables))
	for _, t := range tables {
		visible[t] = true
	}

	var b strings.Builder
	rendered := 0
	for _, name := range tables {
		detail, err := idx.TableDetail(name)
		if err != nil || detail == nil {
			// A shadow with no live counterpart is a drift bug caught by
			// shadow_columns_test.go, not something to fail this render
			// over - skip it and still render every other table.
			continue
		}
		if renderTable(&b, name, detail, visible) {
			rendered++
		}
	}

	if rendered == 0 {
		return analyticsSchemaFallback, fmt.Errorf("dbctx returned no renderable tables for role %q", role)
	}
	return b.String(), nil
}

// renderTable writes one table's structural entry. Returns false (writing
// nothing) if the table has no allowlisted columns present in the live
// schema, which should not happen outside a drift bug but must never render
// an empty, confusing table header.
func renderTable(b *strings.Builder, name string, detail *dbctx.TableDetail, visible map[string]bool) bool {
	allowed := make(map[string]bool)
	for _, c := range livisql.ColumnsFor(name) {
		allowed[c] = true
	}

	cols := make([]dbctx.ColumnDetail, 0, len(detail.Columns))
	for _, c := range detail.Columns {
		if allowed[c.Name] {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return false
	}

	fmt.Fprintf(b, "**`%s`**\n", name)

	var pk []string
	for _, c := range cols {
		if c.IsPK {
			pk = append(pk, c.Name)
		}
	}
	if len(pk) > 0 {
		fmt.Fprintf(b, "PK: %s\n", strings.Join(pk, ", "))
	}

	for _, c := range detail.ForeignKeys {
		if visible[c.RefTable] {
			fmt.Fprintf(b, "%s -> %s\n", c.SrcColumns, c.RefTable)
		}
	}

	for _, c := range cols {
		flag := ""
		switch {
		case c.IsState:
			flag = " [state]"
		case c.IsCategoric:
			flag = " [cat]"
		}
		nullable := ""
		if c.Nullable {
			nullable = " NULL"
		}
		fmt.Fprintf(b, "- `%s` %s%s%s\n", c.Name, c.Type, nullable, flag)

		// Structure only, never SampleValues - see the function doc comment.
		for _, jp := range c.JSONBPaths {
			fmt.Fprintf(b, "  - `%s` %s\n", jp.Path, jp.InferredType)
		}
	}

	b.WriteString("\n")
	return true
}
