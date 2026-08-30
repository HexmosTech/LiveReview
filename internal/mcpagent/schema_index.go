package mcpagent

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

	// schemaIdxFailureReason holds a non-empty string only when the schema
	// index can never become ready this process - e.g.
	// DBCTX_SCHEMA_INDEX_ENABLED=false pointed at a pre-built .dtx file
	// that doesn't exist, opened empty, or has drifted from the live
	// schema (see the fingerprint check below). Distinct from "still
	// building": that resolves on its own within schemaIndexWaitTimeout;
	// this never will, no matter how long a caller waits. The specific
	// reason string lets agent.go show a message tailored to what's
	// actually wrong ("missing" vs "stale" vs "unverifiable") instead of
	// one generic "come back in 60s" that's actively misleading for a
	// failure that will never resolve on its own. Always holds a string
	// (possibly "") - never left as the zero Value.
	schemaIdxFailureReason atomic.Value
)

func init() {
	schemaIdxFailureReason.Store("")
}

// schemaIndexFailureReasons are the distinct hard-failure reasons
// schemaIdxFailureReason can hold, and the exact string agent.go matches
// on - kept together here so the two stay in sync.
const (
	schemaIndexFailureMissing      = "missing"      // pre-built .dtx file not found / empty / unreadable
	schemaIndexFailureStale        = "stale"        // fingerprint mismatch: live schema changed since the .dtx was built
	schemaIndexFailureUnverifiable = "unverifiable" // couldn't compute a fingerprint to compare (e.g. DB unreachable)
	schemaIndexFailureBuildFailed  = "build_failed" // async in-memory build never started
)

// setSchemaIndexFailure records why the schema index can never become
// ready this process. See schemaIdxFailureReason's doc comment.
func setSchemaIndexFailure(reason string) {
	schemaIdxFailureReason.Store(reason)
}

// openPrebuiltDtx opens dtxPath and verifies it's actually usable: present,
// non-empty, and fingerprint-matched against the live schema at dsn. Returns
// (idx, "") on success, or (nil, reason) using the schemaIndexFailure*
// constants on any failure - never sets schemaIdxFailureReason itself, so
// the caller can retry once (after an automatic `make prep-dbctx`) before
// deciding a reason is final.
func openPrebuiltDtx(dsn, dtxPath string, out, hardFailOut io.Writer) (*dbctx.Index, string) {
	// Check existence explicitly rather than relying on dbctx.Open's own
	// error behavior: Open() silently creates/opens an empty SQLite file at
	// a missing path instead of erroring, which previously made a deleted
	// .dtx report "ready" with 0 tables - schemaIndexReady() then returned
	// true and the turn proceeded past its precondition gate straight into
	// a degraded response deep in the pipeline instead of refusing up
	// front.
	if _, statErr := os.Stat(dtxPath); statErr != nil {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: pre-built .dtx file not found at %s: %v\n", dtxPath, statErr)
		log.Error().Err(statErr).Str("path", dtxPath).Msg("dbctx schema index: pre-built .dtx file not found")
		return nil, schemaIndexFailureMissing
	}

	start := time.Now()
	fmt.Fprintf(out, "[dbctx] schema index: opening pre-built .dtx file from %s...\n", dtxPath)
	log.Info().Str("path", dtxPath).Msg("dbctx schema index: opening pre-built .dtx file")

	idx, err := dbctx.Open(dtxPath)
	if err != nil {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: failed to open .dtx file: %v\n", err)
		log.Error().Err(err).Str("path", dtxPath).Msg("dbctx schema index: failed to open .dtx file")
		return nil, schemaIndexFailureMissing
	}

	elapsed := time.Since(start)
	stats, err := idx.Stats()
	if err != nil {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: opened .dtx but Stats() failed: %v\n", err)
		log.Error().Err(err).Dur("elapsed", elapsed).Msg("dbctx schema index: opened .dtx but Stats() failed")
		return nil, schemaIndexFailureMissing
	}
	if stats.Tables == 0 {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: opened .dtx at %s but it has 0 tables - treating as missing/corrupt\n", dtxPath)
		log.Error().Str("path", dtxPath).Msg("dbctx schema index: opened .dtx but it has 0 tables")
		return nil, schemaIndexFailureMissing
	}

	// The .dtx is a snapshot - nothing re-checks it against the live schema
	// on its own, so if the schema changed since this file was built (a
	// column added/dropped/retyped), the model gets shown a schema that no
	// longer exists and produces subtly wrong SQL. Compare the fingerprint
	// dbctx stored at build time against one computed fresh right now -
	// cheap (one schema-extraction query, not a rebuild) but forced on
	// every load, not an opt-in check someone has to remember to run. An
	// unverifiable comparison (DB unreachable, no stored fingerprint on an
	// older .dtx) is treated the same as a confirmed mismatch - "forced,
	// not quietly ignored" applies to the check failing too, not only to a
	// confirmed drift.
	storedFP, fpErr := idx.SchemaFingerprint()
	if fpErr != nil {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: could not read stored fingerprint: %v\n", fpErr)
		log.Error().Err(fpErr).Msg("dbctx schema index: could not read stored fingerprint")
		return nil, schemaIndexFailureUnverifiable
	}
	fpCtx, fpCancel := context.WithTimeout(context.Background(), liveFingerprintTimeout)
	liveFP, liveErr := dbctx.LiveFingerprint(fpCtx, dsn, nil)
	fpCancel()
	if liveErr != nil {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: could not compute live schema fingerprint: %v\n", liveErr)
		log.Error().Err(liveErr).Msg("dbctx schema index: could not compute live schema fingerprint")
		return nil, schemaIndexFailureUnverifiable
	}
	if storedFP == "" || storedFP != liveFP {
		fmt.Fprintf(hardFailOut, "[dbctx] schema index: stale - .dtx fingerprint %q does not match live schema fingerprint %q\n", storedFP, liveFP)
		log.Error().Str("stored_fingerprint", storedFP).Str("live_fingerprint", liveFP).
			Msg("dbctx schema index: stale - .dtx does not match live schema")
		return nil, schemaIndexFailureStale
	}

	fmt.Fprintf(out, "[dbctx] schema index: ready in %s (%d tables, %d columns, %d foreign keys, %d state fields)\n",
		elapsed.Round(time.Millisecond), stats.Tables, stats.Columns, stats.ForeignKeys, stats.StateFields)
	log.Info().Dur("elapsed", elapsed).
		Int("tables", stats.Tables).Int("columns", stats.Columns).
		Int("foreign_keys", stats.ForeignKeys).Int("state_fields", stats.StateFields).
		Msg("dbctx schema index: ready")
	return idx, ""
}

// liveFingerprintTimeout bounds the live-DB schema-extraction query the
// staleness check runs on every load. It's a single lightweight query (the
// same one Build's own phase 1 runs, not a full rebuild), so this stays
// short - a hung/slow DB should surface as "unverifiable" quickly rather
// than blocking server startup indefinitely.
const liveFingerprintTimeout = 15 * time.Second

// runPrepDbctxTimeout bounds the automatic `make prep-dbctx` rebuild - the
// semantic-embedding phase alone has been observed to take over a minute
// against this schema, so this needs real headroom, not a short guard.
const runPrepDbctxTimeout = 5 * time.Minute

// runPrepDbctx shells out to `make prep-dbctx` (repo root - the process's
// own working directory, same assumption every other relative path in this
// file already makes) and streams its output into out live, so the rebuild
// is visible while it's happening rather than only after the fact. Local
// dev only (see the DBCTX_SCHEMA_INDEX_ENABLED=false caller): this assumes
// `make`, the `dbctx` CLI, and this repo's Makefile are all present, which
// is true for a local checkout but not necessarily a deployed environment -
// exactly why this path only runs when that env var opts into it.
func runPrepDbctx(out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), runPrepDbctxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "make", "prep-dbctx")
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

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

		// hardFailOut additionally mirrors into chat_debug.log (the "normal"
		// log everything else in a chat turn lands in) - a hard failure here
		// means every subsequent turn gets refused, so it needs to be
		// visible wherever anyone is actually looking for why turns are
		// failing, not only in dbctx_debug.log's dbctx-specific detail.
		hardFailOut := io.MultiWriter(out, logging.ChatDebugWriter())

		// Check if we should use a pre-built .dtx file instead of building
		if enabled := strings.TrimSpace(os.Getenv("DBCTX_SCHEMA_INDEX_ENABLED")); enabled != "" && strings.EqualFold(enabled, "false") {
			log.Warn().Msg("dbctx semantic index disabled via DBCTX_SCHEMA_INDEX_ENABLED=false")

			homeDir, _ := os.UserHomeDir()
			dtxPath := filepath.Join(homeDir, "livereview.dtx")

			idx, reason := openPrebuiltDtx(dsn, dtxPath, out, hardFailOut)

			// Missing or stale is exactly the "someone forgot to run `make
			// prep-dbctx`" failure mode this whole check exists to catch -
			// and per the RCA it replaces ("don't rely on people
			// remembering to do things manually"), the fix for it is
			// mechanical too: run the command ourselves instead of just
			// telling a human to. One retry only - if the rebuild itself
			// fails, or the rebuilt file still doesn't check out, this
			// stops and hard-fails rather than looping. Deliberately NOT
			// done for "unverifiable" (e.g. DB unreachable): rebuilding
			// needs that same DB connection, so retrying would just fail
			// the same way for the same reason.
			if reason == schemaIndexFailureMissing || reason == schemaIndexFailureStale {
				fmt.Fprintf(hardFailOut, "[dbctx] schema index: %s - running `make prep-dbctx` to rebuild automatically...\n", reason)
				log.Warn().Str("reason", reason).Msg("dbctx schema index: attempting automatic rebuild via make prep-dbctx")

				if rebuildErr := runPrepDbctx(hardFailOut); rebuildErr != nil {
					fmt.Fprintf(hardFailOut, "[dbctx] schema index: make prep-dbctx failed: %v\n", rebuildErr)
					log.Error().Err(rebuildErr).Msg("dbctx schema index: automatic rebuild failed")
					setSchemaIndexFailure(reason)
					return
				}
				fmt.Fprintf(hardFailOut, "[dbctx] schema index: make prep-dbctx finished, retrying...\n")
				log.Info().Msg("dbctx schema index: automatic rebuild finished, retrying")
				idx, reason = openPrebuiltDtx(dsn, dtxPath, out, hardFailOut)
			}

			if reason != "" {
				setSchemaIndexFailure(reason)
				return
			}
			schemaIdx = idx

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
			fmt.Fprintf(hardFailOut, "[dbctx] schema index: build failed to start: %v\n", err)
			log.Error().Err(err).Msg("dbctx schema index: build failed to start")
			setSchemaIndexFailure(schemaIndexFailureBuildFailed)
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
				fmt.Fprintf(out, "[dbctx] schema index: build failed after %s: %v [FATAL: halting server boot]\n", elapsed.Round(time.Millisecond), err)
				log.Fatal().Err(err).Dur("elapsed", elapsed).
					Msg("dbctx schema index: build failed; halting server boot")
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

// schemaIndexFailureReason returns why the schema index can never become
// ready this process (schemaIndexFailureMissing/Stale/Unverifiable/
// BuildFailed), or "" if it hasn't permanently failed - either because it's
// still building or because it's ready. Callers refusing a turn on
// !schemaIndexReady() check this to choose between a "still building, try
// again shortly" message and one specific to what's actually wrong.
func schemaIndexFailureReason() string {
	v, _ := schemaIdxFailureReason.Load().(string)
	return v
}

// schemaIndexHardFailed reports whether the schema index can never become
// ready this process. Equivalent to schemaIndexFailureReason() != "".
func schemaIndexHardFailed() bool {
	return schemaIndexFailureReason() != ""
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
