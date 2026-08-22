package mcpagent

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/rs/zerolog/log"
	"github.com/shrsv/dbctx"
)

// terminologyJSON maps LiveReview-specific vocabulary (LOC, LRC, org, repo,
// PR/MR/"MR/PR") to the exact schema objects they refer to, so dbctx's
// retrieval can resolve a question phrased in domain jargon even when
// neither lexical nor semantic matching would get there on its own - see
// Index.ImportTerminology and the package doc's "Terminology" section.
// Purely additive: never populated automatically, reviewed by hand before
// being embedded here. Some targets (org_billing_state.*,
// quota_batch_settlements.*, api_keys.*) point at tables now on
// livisql.deniedTables - harmless, just an inert entry for retrieval
// purposes, since those tables never render regardless of what matches.
//
//go:embed terminology.json
var terminologyJSON []byte

// Persisting the dbctx index to disk (Options.Path) was tried here, so a
// server restart could reuse cached embeddings instead of paying dbctx
// v0.1.1's semantic-index build cost every time (measured against this
// schema: ~31s cold vs ~1s for the old in-memory/no-semantic build).
// Reverted: the SECOND build against an existing .dtx file fails - "store
// schema: insert table ai_comments: constraint failed: UNIQUE constraint
// failed: tables.schema, tables.name" - which degrades gracefully (falls
// back to no schema for that turn) but would repeat on every restart after
// the first, permanently, since the .dtx file's state doesn't self-heal.
// That's worse than the in-memory cold-start cost this was meant to avoid.
// Worth reporting upstream to shrijith; revisit persistence once fixed.

// schemaIdx is the process-wide dbctx index used to describe the analytics
// schema to the LLM instead of the hand-written table listing that used to
// live in prompts/analytics_schema.md. See dbctx_schema_plan.md for why it
// points at the real "public" schema (dbctx cannot introspect views - this
// was verified empirically, not assumed) and why schema_render.go, not this
// file, is responsible for keeping tenant-unsafe content out of the prompt.
//
// A package-level singleton rather than a field threaded through Agent
// because it is built once per process from the server's own database, not
// per session - every Agent/session shares the same index.
var (
	schemaIdx     *dbctx.Index
	schemaIdxOnce sync.Once
)

// InitSchemaIndex starts building the dbctx index in the background and
// returns immediately - callers do not wait on it. Called exactly once, from
// internal/api/server.go's NewServer (next to logging.InitChatDebugLog),
// which runs on every server boot - including every `make run`/`make
// develop` restart, since those run the binary under Air's watch-and-rebuild
// loop and Air restarts the process (calling NewServer again) on every
// rebuild. There is nothing separate to wire up: the index is rebuilt fresh
// from the live schema each time the dev server restarts. Safe to call with
// an empty dsn (analytics disabled) or to skip calling entirely: schemaIndex()
// returns nil until this has run, and callers already have to handle a
// nil/errored index as the fallback path.
//
// Prints a plain stdout line at start and at finish (in addition to the
// zerolog lines below) because zerolog writes raw JSON with no console
// formatter configured in this codebase - easy to miss scrolling past in
// Air's rebuild output otherwise.
func InitSchemaIndex(dsn string) {
	schemaIdxOnce.Do(func() {
		if dsn == "" {
			log.Warn().Msg("dbctx schema index not started: empty DSN")
			return
		}

		// out mirrors every boot-status print to dbctx_debug.log alongside
		// stdout, so the whole build lifecycle - this function's own status
		// lines plus dbctx's internal "N/N Building ... index" progress fed
		// through Options.Logger below - lands in one correlated file
		// instead of only being visible in the terminal.
		out := io.MultiWriter(os.Stdout, logging.DBCtxDebugWriter())

		// Check if we should use a pre-built .dtx file instead of building
		if enabled := strings.TrimSpace(os.Getenv("DBCTX_SCHEMA_INDEX_ENABLED")); enabled != "" && strings.EqualFold(enabled, "false") {
			log.Warn().Msg("dbctx semantic index disabled via DBCTX_SCHEMA_INDEX_ENABLED=false")

			// Try to open existing .dtx file
			homeDir, _ := os.UserHomeDir()
			dtxPath := filepath.Join(homeDir, "livereview.dtx")

			start := time.Now()
			fmt.Fprintf(out, "[dbctx] schema index: opening pre-built .dtx file from %s...\n", dtxPath)
			log.Info().Str("path", dtxPath).Msg("dbctx schema index: opening pre-built .dtx file")

			idx, err := dbctx.Open(dtxPath)
			if err != nil {
				fmt.Fprintf(out, "[dbctx] schema index: failed to open .dtx file: %v\n", err)
				log.Error().Err(err).Str("path", dtxPath).Msg("dbctx schema index: failed to open .dtx file")
				return
			}
			schemaIdx = idx

			// Log stats and import terminology immediately (no async needed)
			elapsed := time.Since(start)
			stats, err := idx.Stats()
			if err != nil {
				fmt.Fprintf(out, "[dbctx] schema index: opened .dtx but Stats() failed: %v\n", err)
				log.Error().Err(err).Dur("elapsed", elapsed).Msg("dbctx schema index: opened .dtx but Stats() failed")
				return
			}
			fmt.Fprintf(out, "[dbctx] schema index: ready in %s (%d tables, %d columns, %d foreign keys, %d state fields)\n",
				elapsed.Round(time.Millisecond), stats.Tables, stats.Columns, stats.ForeignKeys, stats.StateFields)
			log.Info().Dur("elapsed", elapsed).
				Int("tables", stats.Tables).Int("columns", stats.Columns).
				Int("foreign_keys", stats.ForeignKeys).Int("state_fields", stats.StateFields).
				Msg("dbctx schema index: ready")

			// A pre-built .dtx already has its terminology imported ahead of
			// time (see `make prep-dbctx`'s `dbctx terminology import` step) -
			// this path never calls ImportTerminology itself, so without this
			// check the log gives no signal either way on whether that import
			// actually stuck across a rebuild of the .dtx file.
			if terms, err := idx.Terminology(); err != nil {
				fmt.Fprintf(out, "[dbctx] terminology: failed to read imported entries: %v\n", err)
				log.Warn().Err(err).Msg("dbctx terminology: failed to read imported entries")
			} else {
				distinct := make(map[string]struct{}, len(terms))
				for _, t := range terms {
					distinct[t.Term] = struct{}{}
				}
				fmt.Fprintf(out, "[dbctx] terminology: %d entries loaded from .dtx (%d distinct terms)\n", len(terms), len(distinct))
				log.Info().Int("entries", len(terms)).Int("distinct_terms", len(distinct)).
					Msg("dbctx terminology: loaded from pre-built .dtx")
			}

			// Warm-up query
			warmStart := time.Now()
			if _, err := idx.Query("Is LiveReview adoption increasing since my team started using it?"); err != nil {
				fmt.Fprintf(out, "[dbctx] warm-up query failed: %v\n", err)
				log.Warn().Err(err).Msg("dbctx warm-up query failed")
			} else {
				fmt.Fprintf(out, "[dbctx] warm-up query done in %s\n", time.Since(warmStart).Round(time.Millisecond))
				log.Info().Dur("elapsed", time.Since(warmStart)).Msg("dbctx warm-up query done")
			}
			return
		}

		// Default behavior: build in-memory index
		start := time.Now()
		fmt.Fprintln(out, "[dbctx] schema index: build starting...")
		log.Info().Msg("dbctx schema index: build starting")

		// Logger: without it, dbctx writes its own boot-time progress
		// ("4/4 Building search index...", "5/5 Building semantic
		// index...") to os.Stderr by default - invisible outside the
		// terminal and not correlated with anything else. Routing it into
		// dbctx_debug.log puts the whole build lifecycle in one place.
		idx, ready, err := dbctx.BuildAsync(context.Background(), dsn, &dbctx.Options{Logger: logging.DBCtxDebugWriter()})
		if err != nil {
			fmt.Fprintf(out, "[dbctx] schema index: build failed to start: %v\n", err)
			log.Error().Err(err).Msg("dbctx schema index: build failed to start")
			return
		}
		schemaIdx = idx

		// Queries against schemaIdx block until ready on their own (per
		// dbctx's own docs), so this goroutine exists only to log the
		// outcome - nothing downstream waits on it.
		go func() {
			<-ready
			elapsed := time.Since(start)
			if err := idx.Err(); err != nil {
				fmt.Fprintf(out, "[dbctx] schema index: build failed after %s: %v (analytics prompts will use the static fallback schema)\n", elapsed.Round(time.Millisecond), err)
				log.Error().Err(err).Dur("elapsed", elapsed).
					Msg("dbctx schema index: build failed; analytics prompts will use the static fallback schema")
				return
			}
			stats, err := idx.Stats()
			if err != nil {
				fmt.Fprintf(out, "[dbctx] schema index: ready after %s, but Stats() failed: %v\n", elapsed.Round(time.Millisecond), err)
				log.Error().Err(err).Dur("elapsed", elapsed).Msg("dbctx schema index: ready, but Stats() failed")
				return
			}
			fmt.Fprintf(out, "[dbctx] schema index: ready in %s (%d tables, %d columns, %d foreign keys, %d state fields)\n",
				elapsed.Round(time.Millisecond), stats.Tables, stats.Columns, stats.ForeignKeys, stats.StateFields)
			log.Info().Dur("elapsed", elapsed).
				Int("tables", stats.Tables).Int("columns", stats.Columns).
				Int("foreign_keys", stats.ForeignKeys).Int("state_fields", stats.StateFields).
				Msg("dbctx schema index: ready")

			termResult, err := idx.ImportTerminology(terminologyJSON)
			if err != nil {
				fmt.Fprintf(out, "[dbctx] terminology import failed: %v\n", err)
				log.Error().Err(err).Msg("dbctx terminology import failed")
				return
			}
			fmt.Fprintf(out, "[dbctx] terminology imported: %d accepted, %d rejected\n", termResult.Accepted, len(termResult.Rejected))
			for _, r := range termResult.Rejected {
				fmt.Fprintf(out, "[dbctx] terminology entry rejected: %+v\n", r)
			}
			log.Info().Int("accepted", termResult.Accepted).Int("rejected", len(termResult.Rejected)).
				Msg("dbctx terminology imported")

			// One throwaway idx.Query call here pays whatever cold-start cost
			// the first call incurs (e.g. spinning up the semantic scorer's
			// ONNX session - see openSemanticScorer's sync.Once) during
			// process startup instead of during the first real user's chat
			// turn. The query text is a representative real question, not
			// special in any way; the result is discarded.
			warmStart := time.Now()
			if _, err := idx.Query("Is LiveReview adoption increasing since my team started using it?"); err != nil {
				fmt.Fprintf(out, "[dbctx] warm-up query failed: %v\n", err)
				log.Warn().Err(err).Msg("dbctx warm-up query failed")
			} else {
				fmt.Fprintf(out, "[dbctx] warm-up query done in %s\n", time.Since(warmStart).Round(time.Millisecond))
				log.Info().Dur("elapsed", time.Since(warmStart)).Msg("dbctx warm-up query done")
			}
		}()
	})
}

// schemaIndexWaitTimeout bounds how long a single turn will wait for the
// index to finish building. dbctx.Index blocks TableDetail/Query calls until
// ready with no timeout of its own, so without this a stalled build (DB
// connectivity trouble, not necessarily a hard failure - Err() stays nil
// until the build actually finishes one way or the other) would hang the
// chat turn that hit it indefinitely instead of degrading gracefully.
const schemaIndexWaitTimeout = 3 * time.Second

// schemaIndex returns the process-wide dbctx index, or nil if InitSchemaIndex
// was never called, failed to start, hasn't finished building within
// schemaIndexWaitTimeout, or failed to build. schema_render.go treats all of
// these the same way: fall back to the static minimal schema text and log
// SchemaSourceDegraded for the turn.
func schemaIndex() *dbctx.Index {
	if schemaIdx == nil {
		return nil
	}
	select {
	case <-schemaIdx.Ready():
	case <-time.After(schemaIndexWaitTimeout):
		log.Warn().Dur("waited", schemaIndexWaitTimeout).
			Msg("dbctx schema index: not ready in time, using fallback schema for this turn")
		return nil
	}
	if err := schemaIdx.Err(); err != nil {
		return nil
	}
	return schemaIdx
}

// schemaIndexReady reports whether the live schema index is available for
// this turn. A turn that needs the schema cannot produce a correct answer
// without it - the model would write SQL against tables and columns it was
// never shown - so callers use this to refuse the turn up front rather than
// spend tokens on calls that can only fail or, worse, succeed against
// guessed column names.
func schemaIndexReady() bool {
	return schemaIndex() != nil
}

// allTableNames returns every table name dbctx knows about, feeding
// livisql.CatalogFor (which then subtracts deniedTables). Returns nil if the
// index isn't ready, in which case CatalogFor returns an empty catalog.
func allTableNames() []string {
	idx := schemaIndex()
	if idx == nil {
		return nil
	}
	tables, err := idx.Tables()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(tables))
	for _, t := range tables {
		out = append(out, t.Name)
	}
	return out
}
