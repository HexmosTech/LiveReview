package mcpagent

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shrsv/dbctx"
)

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
// returns immediately - callers do not wait on it. Call exactly once at
// server startup (see internal/api/server.go's NewServer, next to
// logging.InitChatDebugLog). Safe to call with an empty dsn (analytics
// disabled) or to skip calling entirely: schemaIndex() returns nil until
// this has run, and callers already have to handle a nil/errored index as
// the fallback path.
func InitSchemaIndex(dsn string) {
	schemaIdxOnce.Do(func() {
		if dsn == "" {
			log.Warn().Msg("dbctx schema index not started: empty DSN")
			return
		}

		start := time.Now()
		log.Info().Msg("dbctx schema index: build starting")

		idx, ready, err := dbctx.BuildAsync(context.Background(), dsn, nil)
		if err != nil {
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
				log.Error().Err(err).Dur("elapsed", elapsed).
					Msg("dbctx schema index: build failed; analytics prompts will use the static fallback schema")
				return
			}
			stats, err := idx.Stats()
			if err != nil {
				log.Error().Err(err).Dur("elapsed", elapsed).Msg("dbctx schema index: ready, but Stats() failed")
				return
			}
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
