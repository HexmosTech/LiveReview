package mcpagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shrsv/dbctx"
)

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

		start := time.Now()
		fmt.Println("[dbctx] schema index: build starting...")
		log.Info().Msg("dbctx schema index: build starting")

		idx, ready, err := dbctx.BuildAsync(context.Background(), dsn, nil)
		if err != nil {
			fmt.Printf("[dbctx] schema index: build failed to start: %v\n", err)
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
				fmt.Printf("[dbctx] schema index: build failed after %s: %v (analytics prompts will use the static fallback schema)\n", elapsed.Round(time.Millisecond), err)
				log.Error().Err(err).Dur("elapsed", elapsed).
					Msg("dbctx schema index: build failed; analytics prompts will use the static fallback schema")
				return
			}
			stats, err := idx.Stats()
			if err != nil {
				fmt.Printf("[dbctx] schema index: ready after %s, but Stats() failed: %v\n", elapsed.Round(time.Millisecond), err)
				log.Error().Err(err).Dur("elapsed", elapsed).Msg("dbctx schema index: ready, but Stats() failed")
				return
			}
			fmt.Printf("[dbctx] schema index: ready in %s (%d tables, %d columns, %d foreign keys, %d state fields)\n",
				elapsed.Round(time.Millisecond), stats.Tables, stats.Columns, stats.ForeignKeys, stats.StateFields)
			log.Info().Dur("elapsed", elapsed).
				Int("tables", stats.Tables).Int("columns", stats.Columns).
				Int("foreign_keys", stats.ForeignKeys).Int("state_fields", stats.StateFields).
				Msg("dbctx schema index: ready")
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

// orgScopedColumns returns table name -> column names for every table dbctx
// knows about that has a direct org_id column, feeding
// livisql.CatalogFor's auto-generated shadows (see AutoOrgScopedShadow).
// Returns nil if the index isn't ready, in which case CatalogFor falls back
// to just its two hand-written specials (orgs, users) - degraded, but never
// zero tables.
func orgScopedColumns() map[string][]string {
	idx := schemaIndex()
	if idx == nil {
		return nil
	}
	tables, err := idx.Tables()
	if err != nil {
		return nil
	}
	out := make(map[string][]string, len(tables))
	for _, t := range tables {
		detail, err := idx.TableDetail(t.Name)
		if err != nil || detail == nil {
			continue
		}
		hasOrgID := false
		cols := make([]string, 0, len(detail.Columns))
		for _, c := range detail.Columns {
			cols = append(cols, c.Name)
			if c.Name == "org_id" {
				hasOrgID = true
			}
		}
		if hasOrgID {
			out[t.Name] = cols
		}
	}
	return out
}
