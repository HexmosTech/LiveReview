package mcpagent

import (
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/livisql"
	"github.com/rs/zerolog/log"
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
// exact tenant-unsafe content the paragraph above keeps out. idx.Query IS
// used for the rendering itself now, though (see narrowTables/
// renderTableContext) - not just to pick which tables to render:
// dbctx.TableContext (what idx.Query already returns per matched table)
// carries the exact same Columns/PrimaryKey/ForeignKeys data as a separate
// idx.TableDetail(name) call would, per dbctx's own source (v0.1.2
// dbctx.go: TableContext and TableDetail are both backed by the same
// columns/primary_keys/foreign_keys tables in dbctx's internal SQLite
// store) - confirmed with shrijith. A second TableDetail call per matched
// table was therefore pure duplicate work: same data, a second round trip.
// Measured impact removing it: in an optimized build, the TableDetail loop
// added ~200-250ms on top of idx.Query's ~300-800ms for ~50 tables: real,
// but modest. In LiveReview's actual dev build (Air's `-gcflags='all=-N
// -l'`, needed for the Delve debugger on :2345 - see .air.toml), the same
// loop cost ~6.5-6.8s on top of idx.Query's own ~10-12s in that build,
// which is the dominant share of the 10s+ dbctxTableText calls seen in
// chat_debug_logs - eliminating it removes that cost entirely rather than
// just reducing it, since renderTableContext below needs no extra call at
// all for the common (query matched something) case.
//
// Falls back to a TableDetail call per table (the old path, kept only for
// this rare branch) when queryText is empty, idx.Query errors, or nothing
// matches - narrowTables reports this via its second return value.
//
// Returns "" and a non-nil error when the live index isn't available - no
// fallback text. The caller is expected to log that via
// ChatTurnLogger.SchemaSourceDegraded - this function has no session/turn
// context of its own to log through.
func dbctxTableText(queryText string) (string, error) {
	turnStart := time.Now()
	idx := schemaIndex()
	if idx == nil {
		return "", fmt.Errorf("dbctx index unavailable")
	}

	tables := livisql.CatalogFor(allTableNames()).Tables()
	visible := make(map[string]bool, len(tables))
	for _, t := range tables {
		visible[t] = true
	}

	narrowStart := time.Now()
	matched, ok := narrowTables(idx, visible, queryText)
	narrowElapsed := time.Since(narrowStart)

	renderStart := time.Now()
	var b strings.Builder
	rendered := 0
	renderedCount := 0
	if ok {
		for _, tc := range matched {
			if renderTableContext(&b, tc, visible) {
				rendered++
			}
		}
		renderedCount = len(matched)
	} else {
		// Degraded/rare path: no useful narrowing happened, so fall back to
		// every role-visible table by name via the old TableDetail-per-table
		// route - this is not the hot path the doc comment above is about.
		for _, name := range tables {
			detail, err := idx.TableDetail(name)
			if err != nil || detail == nil {
				continue
			}
			if renderTable(&b, name, detail, visible) {
				rendered++
			}
		}
		renderedCount = len(tables)
	}
	renderElapsed := time.Since(renderStart)
	totalElapsed := time.Since(turnStart)

	log.Info().
		Dur("total", totalElapsed).
		Dur("query_narrow", narrowElapsed).
		Dur("render", renderElapsed).
		Bool("used_query_narrowing", ok).
		Int("table_count", renderedCount).
		Msg("dbctxTableText timing breakdown")
	if totalElapsed > 2*time.Second {
		fmt.Printf("[dbctx] dbctxTableText SLOW: total=%s query_narrow=%s render=%s used_query_narrowing=%v tables=%d query=%q\n",
			totalElapsed.Round(time.Millisecond), narrowElapsed.Round(time.Millisecond),
			renderElapsed.Round(time.Millisecond), ok, renderedCount, queryText)
	}

	if rendered == 0 {
		return "", fmt.Errorf("dbctx returned no renderable tables")
	}
	return b.String(), nil
}

// narrowTables asks dbctx which of the role-visible tables are relevant to
// queryText - the user's question, or the specific report question being
// finalized - via idx.Query, and returns the matched dbctx.TableContext
// objects directly. Uses .Matched() (dbctx v0.1.2: matched + FK-expanded
// join context, the library's recommended default as of this version -
// table names may have swapped meaning between dbctx releases, since
// v0.1.1's .Matched() meant score > 0 only, what v0.1.2 calls
// .ScoredOnly(); check the installed dbctx version's doc comment on
// Query/ResultSet before assuming which one this is on a future bump).
// .ScoredOnly() (direct hits only, no FK expansion) is available but
// intentionally not used here - shrijith's guidance is that .Matched() is
// the one to keep using across the CLI/UI/library, so bugs in its
// relevance/FK logic get fixed in one place rather than every caller
// picking its own narrower selection.
//
// The returned TableContext values are Query's own result - not a second
// TableDetail lookup - see dbctxTableText's doc comment for why that's
// sufficient (same underlying data, no separate call needed).
//
// ok is false when queryText is empty, idx.Query errors, or nothing
// matches; the returned slice is nil in that case and the caller is
// expected to fall back to rendering every role-visible table some other
// way, so a turn never renders an empty schema.
func narrowTables(idx *dbctx.Index, visible map[string]bool, queryText string) ([]dbctx.TableContext, bool) {
	if strings.TrimSpace(queryText) == "" {
		return nil, false
	}
	rs, err := idx.Query(queryText)
	if err != nil {
		return nil, false
	}
	seen := make(map[string]bool)
	var narrowed []dbctx.TableContext
	for _, tc := range rs.Matched().Tables() {
		if visible[tc.TableName] && !seen[tc.TableName] {
			seen[tc.TableName] = true
			narrowed = append(narrowed, tc)
		}
	}
	if len(narrowed) == 0 {
		return nil, false
	}
	return narrowed, true
}

// renderTableContext is renderTable's twin for the common (query matched
// something) path: same output shape, but reading dbctx.ColumnInfo/FKInfo
// from a dbctx.TableContext (already in hand from idx.Query) instead of a
// dbctx.TableDetail fetched via a second call. Keep the two render
// functions' output format in sync by hand - ColumnInfo and ColumnDetail
// are separate (if identically-shaped) types in dbctx's API, so they can't
// share one loop body without an adapter that would cost more than it
// saves for two ~20-line functions.
func renderTableContext(b *strings.Builder, tc dbctx.TableContext, visible map[string]bool) bool {
	cols := make([]dbctx.ColumnInfo, 0, len(tc.Columns))
	for _, c := range tc.Columns {
		if !livisql.IsSecretColumn(c.Name) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		return false
	}

	fmt.Fprintf(b, "**`%s`**\n", tc.TableName)

	var pk []string
	for _, c := range cols {
		if c.IsPK {
			pk = append(pk, c.Name)
		}
	}
	if len(pk) > 0 {
		fmt.Fprintf(b, "PK: %s\n", strings.Join(pk, ", "))
	}

	for _, c := range tc.ForeignKeys {
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
			if jp.SampleValues != "" {
				fmt.Fprintf(b, "  - `%s` %s  {%s}\n", jp.Path, jp.InferredType, jp.SampleValues)
			} else {
				fmt.Fprintf(b, "  - `%s` %s\n", jp.Path, jp.InferredType)
			}
		}
	}

	b.WriteString("\n")
	return true
}

// renderTable writes one table's structural entry from a dbctx.TableDetail
// (the degraded/rare fallback path in dbctxTableText - see its doc
// comment). Returns false (writing nothing) if nothing is left to render,
// which should not happen for a real table but must never render an empty,
// confusing table header.
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
			if jp.SampleValues != "" {
				fmt.Fprintf(b, "  - `%s` %s  {%s}\n", jp.Path, jp.InferredType, jp.SampleValues)
			} else {
				fmt.Fprintf(b, "  - `%s` %s\n", jp.Path, jp.InferredType)
			}
		}
	}

	b.WriteString("\n")
	return true
}
