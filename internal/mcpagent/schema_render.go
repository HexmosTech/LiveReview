package mcpagent

import (
	"fmt"
	"strings"

	"github.com/livereview/internal/livisql"
	"github.com/shrsv/dbctx"
)

// dbctxTableText renders the "### Tables" section of the count_query/
// finalize branches' system prompt from the live dbctx index, narrowed to
// what idx.Query(queryText) returns (see narrowTables). Renders every
// column dbctx reports for a table EXCEPT livisql.IsSecretColumn matches -
// the same exact-name denylist the SQL guard uses (password_hash, api_key,
// access_token, ...), reused here rather than duplicated. This is narrower
// than the old catalog.go/livisql.ColumnsFor allowlist that used to filter
// this text (removed entirely, then partially restored to just this
// denylist): secrets stay out of the prompt, but reviews.metadata and every
// other non-secret column are still rendered in full. Note this denylist
// only excludes ~13 exact column names schema-wide - it is not a meaningful
// prompt-size reduction on its own. The token-count jump from removing
// ColumnsFor came from restoring EVERY non-secret column of every table,
// which this change does not undo.
//
// ColumnInfo.Values and JSONBPathInfo.SampleValues are still never
// rendered - both come from dbctx's pg_stats/TABLESAMPLE sampling, which
// runs over the whole table with no org_id filter, so showing them verbatim
// would let one org's chat surface another org's actual row data. That is
// a separate, still-live tenant-isolation rule, not part of what was waived
// here. Path + InferredType (the JSONB key structure) IS rendered - see
// dbctx_schema_plan.md's "Why the model doesn't need to see a row..."
// section for why that's sufficient to avoid hallucinated JSON paths.
//
// This is also why idx.Query's own Selection.Text()/TextRaw() is never used
// here even though it would be the one-line way to do this: that rendering
// includes ColumnInfo.Values and JSONBPathInfo.SampleValues verbatim, the
// exact tenant-unsafe content the paragraph above keeps out. idx.Query is
// used only to pick which tables to render; the actual rendering always
// goes through TableDetail + renderTable below.
//
// Returns "" and a non-nil error when the live index isn't available - no
// fallback text. The caller is expected to log that via
// ChatTurnLogger.SchemaSourceDegraded - this function has no session/turn
// context of its own to log through.
func dbctxTableText(queryText string) (string, error) {
	idx := schemaIndex()
	if idx == nil {
		return "", fmt.Errorf("dbctx index unavailable")
	}

	tables := livisql.CatalogFor(orgScopedColumns()).Tables()
	visible := make(map[string]bool, len(tables))
	for _, t := range tables {
		visible[t] = true
	}

	renderNames := narrowTables(idx, tables, visible, queryText)

	var b strings.Builder
	rendered := 0
	for _, name := range renderNames {
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
		return "", fmt.Errorf("dbctx returned no renderable tables")
	}
	return b.String(), nil
}

// narrowTables asks dbctx which of the role-visible tables are relevant to
// queryText - the user's question, or the specific report question being
// finalized - via idx.Query, and renders only those. Uses .All() (matched
// plus FK-expanded neighbors) rather than .Matched() alone so a join target
// reachable only through a foreign key isn't dropped just because it didn't
// score on its own.
//
// Falls back to every role-visible table when queryText is empty, idx.Query
// errors, or nothing matches, so a turn never renders an empty schema.
func narrowTables(idx *dbctx.Index, allVisible []string, visible map[string]bool, queryText string) []string {
	if strings.TrimSpace(queryText) == "" {
		return allVisible
	}
	rs, err := idx.Query(queryText)
	if err != nil {
		return allVisible
	}
	seen := make(map[string]bool, len(allVisible))
	var narrowed []string
	for _, tc := range rs.All().Tables() {
		if visible[tc.TableName] && !seen[tc.TableName] {
			seen[tc.TableName] = true
			narrowed = append(narrowed, tc.TableName)
		}
	}
	if len(narrowed) == 0 {
		return allVisible
	}
	return narrowed
}

// renderTable writes one table's structural entry: every column dbctx
// reports for it except livisql.IsSecretColumn matches - see
// dbctxTableText's doc comment. Returns false (writing nothing) if nothing
// is left to render, which should not happen for a real table but must
// never render an empty, confusing table header.
func renderTable(b *strings.Builder, name string, detail *dbctx.TableDetail, visible map[string]bool) bool {
	cols := make([]dbctx.ColumnDetail, 0, len(detail.Columns))
	for _, c := range detail.Columns {
		if !livisql.IsSecretColumn(c.Name) {
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
