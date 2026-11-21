package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/crossrepolink"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
	"github.com/livereview/internal/blobstore"
	storageblastradius "github.com/livereview/storage/blastradius"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
)

// CrossRepoLinkJobArgs runs the cross-repo intelligence pass between a
// review's repo and its configured related repos (see
// docs/multi-repo-blast-radius-plan.md), folds any matched cross-repo
// impacts into the blast-radius report the blast_radius job already saved,
// and adds extended (multi-hop) chains when the related-repo graph allows
// them. It is queued by BlastRadiusWorker after the report is saved, not
// alongside it - see QueueCrossRepoLinkJob.
type CrossRepoLinkJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (CrossRepoLinkJobArgs) Kind() string { return "cross_repo_link" }

// crossRepoMaxDepth bounds how many repo hops (including the direct one) the
// extended chain walk may traverse. Three is enough to show A -> B -> C ->
// D without turning a large related-repo mesh into a combinatorial crawl.
const crossRepoMaxDepth = 3

// crossRepoMaxChainsPerSymbol caps the number of chains attached to one
// symbol, so one very connected service cannot make a diff panel enormous.
const crossRepoMaxChainsPerSymbol = 10

// crossRepoMaxRepos bounds how many related repos one review will clone/index.
// BFS visits depth 1 first; the cap keeps a large mesh from turning every
// review into a fleet-wide indexing job.
const crossRepoMaxRepos = 10

// crossRepoMaxChainSteps is a hard per-symbol walk budget. Cycle prevention is
// the primary guard (visited repos), but a dense mesh can still fan out; this
// stops a pathological route graph before it burns CPU or produces a giant
// chain list.
const crossRepoMaxChainSteps = 400

type CrossRepoLinkWorker struct {
	river.WorkerDefaults[CrossRepoLinkJobArgs]
	db *sql.DB
}

// Timeout allows for indexing every related repo plus the cross-repo-intelligence pass itself.
func (w *CrossRepoLinkWorker) Timeout(job *river.Job[CrossRepoLinkJobArgs]) time.Duration {
	return 15 * time.Minute
}

// Work is best-effort and never retried: a failure here must never block or
// degrade the review's own blast-radius report, which blast_radius already
// saved on its own.
func (w *CrossRepoLinkWorker) Work(ctx context.Context, job *river.Job[CrossRepoLinkJobArgs]) error {
	orgID, reviewID := job.Args.OrgID, job.Args.ReviewID
	logger := log.With().Int64("org_id", orgID).Int64("review_id", reviewID).Logger()

	repoID, skip, err := w.loadReviewRepositoryID(ctx, orgID, reviewID)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot resolve repository row for review")
		return nil
	}
	if skip != "" {
		return nil // not a PR review on a known repository row - nothing to link
	}

	related, err := crossrepolink.LoadRelatedRepos(ctx, w.db, orgID, repoID, crossRepoMaxDepth, crossRepoMaxRepos)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot load related repos")
		return nil
	}
	if len(related) == 0 {
		return nil // the common case: no related repos configured for this repo
	}

	payload, err := blobstore.ReadArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius)
	if err != nil || len(payload) == 0 {
		logger.Warn().Err(err).Msg("[cross_repo_link] no blast-radius report to enrich")
		return nil
	}
	var report blastradius.Report
	if err := json.Unmarshal(payload, &report); err != nil || report.Project == "" {
		logger.Warn().Err(err).Msg("[cross_repo_link] saved report is not a scored report; skipping")
		return nil
	}

	mainRepo, skip, err := loadReviewRepo(ctx, w.db, orgID, reviewID)
	if err != nil || skip != "" {
		logger.Warn().Err(err).Str("skip", skip).Msg("[cross_repo_link] cannot re-resolve main repo checkout")
		return nil
	}
	_, mainMeta, err := crossrepolink.LoadRepoCredentials(ctx, w.db, orgID, repoID)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot resolve main repo metadata")
		return nil
	}
	mainCheckout, mainUnlock, err := repocache.Prepare(ctx, blastRadiusRoot, mainRepo)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] main repo checkout failed")
		return nil
	}
	defer mainUnlock()

	// project -> LiveReview repository metadata, so every impact/chip can show
	// a friendly name and link instead of the engine's cache-derived project id.
	metaByProject := map[string]crossrepolink.RepoMeta{report.Project: mainMeta}
	graphs := map[string]*serverscore.RepoGraph{}

	var targetProjects []string
	for _, rel := range related {
		relProject, g, relMeta, err := crossrepolink.EnsureRepoGraph(ctx, w.db, blastRadiusRoot, orgID, rel.ID, true)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.ID).Msg("[cross_repo_link] related repo graph unavailable")
			report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("related_graph", fmt.Sprintf("repo:%d", rel.ID), err))
			continue
		}
		targetProjects = append(targetProjects, relProject)
		metaByProject[relProject] = relMeta
		graphs[relProject] = g
	}
	if len(targetProjects) == 0 {
		logger.Info().Msg("[cross_repo_link] no related repos could be indexed")
		return nil
	}

	if err := serverscore.LinkRelatedProjects(ctx, mainCheckout.Dir, report.Project, targetProjects); err != nil {
		// Best-effort: the version-aliased fallback below does not depend on
		// the engine's CROSS_* edges, so continue instead of losing all
		// enrichment when the pass fails.
		logger.Warn().Err(err).Msg("[cross_repo_link] cross-repo-intelligence pass failed; continuing with fallback")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("link_pass", report.Project, err))
	}

	symbolQNs, locate := indexSymbols(&report)
	if len(symbolQNs) == 0 {
		return nil
	}
	impactsBySymbol, err := serverscore.CrossRepoImpactsForSymbols(ctx, report.Project, symbolQNs)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] querying cross-repo edges failed; continuing with fallback")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("exact_query", report.Project, err))
		impactsBySymbol = map[string][]blastradius.CrossRepoImpact{}
	}

	// The engine's own cross-repo pass only matches identical route strings.
	// It cannot see a NestJS controller's URI version (`version: '1'`), so a
	// caller using /v1/orders never matches a route indexed as /orders. The
	// shared matcher covers that mismatch (and, when includeExact is on, plain
	// path matches too); here it is asked for alias-only results and merged
	// with the engine's exact edges.
	mainGraph, gerr := serverscore.LoadRepoGraph(ctx, report.Project)
	if gerr != nil {
		logger.Warn().Err(gerr).Msg("[cross_repo_link] cannot read main repo graph; using direct-only fallback")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("main_graph", report.Project, gerr))
	} else {
		graphs[report.Project] = mainGraph
	}
	callsBySymbol := callsFromGraph(mainGraph, symbolQNs)
	if len(callsBySymbol) == 0 {
		if direct, derr := serverscore.HTTPCallRouteNamesForSymbols(ctx, report.Project, symbolQNs); derr != nil {
			logger.Warn().Err(derr).Msg("[cross_repo_link] direct call-site query failed")
			report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("direct_fallback", report.Project, derr))
		} else {
			callsBySymbol = make(map[string][]crossrepolink.Call, len(direct))
			for qn, names := range direct {
				for _, name := range names {
					callsBySymbol[qn] = append(callsBySymbol[qn], crossrepolink.Call{Path: name})
				}
			}
		}
	}
	// Remove the main project from the target set so the matcher cannot match
	// the reviewed repo's own routes as "related".
	delete(graphs, report.Project)
	crossrepolink.MergeImpacts(impactsBySymbol, crossrepolink.MatchCalls(callsBySymbol, graphs, metaByProject, false))

	// Engine edges carry no MatchKind; they are exact by definition.
	for qn, impacts := range impactsBySymbol {
		for i := range impacts {
			if impacts[i].MatchKind == "" {
				impacts[i].MatchKind = "exact"
			}
			crossrepolink.EnrichImpact(&impacts[i], metaByProject)
		}
		impactsBySymbol[qn] = impacts
	}
	if len(impactsBySymbol) == 0 {
		return nil
	}

	changed := false
	chainsBySymbol := map[string][]blastradius.CrossRepoChain{}
	for qn, impacts := range impactsBySymbol {
		loc, ok := locate[qn]
		if !ok {
			continue
		}
		if blastradius.RecordCrossRepoImpact(&report, loc.filePath, loc.newStart, loc.newLines, qn, impacts) {
			changed = true
		}
		if mainGraph != nil {
			chains := crossrepolink.BuildChains(report.Project, graphs, metaByProject, impacts, crossRepoMaxDepth)
			if len(chains) > 0 {
				chainsBySymbol[qn] = chains
			}
		}
	}
	for qn, chains := range chainsBySymbol {
		loc, ok := locate[qn]
		if !ok {
			continue
		}
		if blastradius.RecordCrossRepoChains(&report, loc.filePath, loc.newStart, loc.newLines, qn, chains) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	blastradius.Renormalize(&report)

	enriched, err := json.Marshal(&report)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] encoding enriched report failed")
		return nil
	}
	if err := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius, enriched); err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] saving enriched report failed")
		return nil
	}
	if err := storageblastradius.NewStore(w.db).ReplaceFromReport(ctx, orgID, reviewID, enriched); err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] Postgres replication failed")
	}
	logger.Info().
		Int("related_repos", len(targetProjects)).
		Int("impacted_symbols", len(impactsBySymbol)).
		Int("chained_symbols", len(chainsBySymbol)).
		Msg("[cross_repo_link] report enriched")
	return nil
}

type hunkLocation struct {
	filePath           string
	newStart, newLines int
}

// indexSymbols collects every distinct symbol qualified name touched by
// report and a lookup from qualified name back to the hunk it belongs to
// (the shape RecordCrossRepoImpact needs). A symbol touched by more than one
// hunk keeps only the first hunk seen - good enough for attaching a bonus
// signal, which only needs one place to live.
func indexSymbols(report *blastradius.Report) (qns []string, locate map[string]hunkLocation) {
	locate = make(map[string]hunkLocation)
	seen := make(map[string]bool)
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			for _, sym := range hunk.Symbols {
				if sym.QualifiedName == "" || seen[sym.QualifiedName] {
					continue
				}
				seen[sym.QualifiedName] = true
				qns = append(qns, sym.QualifiedName)
				locate[sym.QualifiedName] = hunkLocation{filePath: file.Path, newStart: hunk.NewStart, newLines: hunk.NewLines}
			}
		}
	}
	return qns, locate
}

// loadReviewRepositoryID resolves the repositories row backing a review.
// Manual PR reviews store `reviews.repository` as the provider's web URL
// (internal/api/review_service.go enrichMetadata), while webhook reviews store
// the full_name. Match all three identities so cross-repo enrichment works for
// both paths, not just webhooks.
func (w *CrossRepoLinkWorker) loadReviewRepositoryID(ctx context.Context, orgID, reviewID int64) (repoID int64, skip string, err error) {
	var repository, triggerType string
	var connectorID sql.NullInt64
	err = w.db.QueryRowContext(ctx,
		`SELECT repository, connector_id, trigger_type FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&repository, &connectorID, &triggerType)
	if err != nil {
		return 0, "", fmt.Errorf("load review: %w", err)
	}
	if triggerType == "cli_diff" || !connectorID.Valid || strings.TrimSpace(repository) == "" {
		return 0, "not_tracked", nil
	}
	candidates := repositoryIdentities(repository)
	for _, candidate := range candidates {
		err = w.db.QueryRowContext(ctx,
			`SELECT id FROM repositories WHERE org_id = $1 AND connector_id = $2 AND full_name = $3`,
			orgID, connectorID.Int64, candidate,
		).Scan(&repoID)
		if err == nil {
			return repoID, "", nil
		}
		if err != sql.ErrNoRows {
			return 0, "", fmt.Errorf("load repository row: %w", err)
		}
	}
	// Last resort: the exact web URL (covers unusual provider URL shapes).
	err = w.db.QueryRowContext(ctx,
		`SELECT id FROM repositories WHERE org_id = $1 AND connector_id = $2 AND web_url = $3`,
		orgID, connectorID.Int64, repository,
	).Scan(&repoID)
	if err == sql.ErrNoRows {
		return 0, "not_tracked", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("load repository row by web_url: %w", err)
	}
	return repoID, "", nil
}

// repositoryIdentities returns the plausible full_name forms for a review's
// repository value: itself, the path of a provider URL, and that path without
// a trailing .git or a /-/merge_requests suffix.
func repositoryIdentities(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := []string{raw}
	trim := func(p string) string {
		p = strings.Trim(strings.TrimSpace(p), "/")
		p = strings.TrimSuffix(p, ".git")
		if i := strings.Index(p, "/-/"); i >= 0 {
			p = p[:i]
		}
		return strings.Trim(p, "/")
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		if p := trim(u.Path); p != "" && p != raw {
			out = append(out, p)
		}
	} else if p := trim(raw); p != "" && p != raw {
		out = append(out, p)
	}
	// Dedupe while preserving order.
	seen := map[string]bool{}
	uniq := out[:0]
	for _, v := range out {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		uniq = append(uniq, v)
	}
	return uniq
}

// loadRelatedRepos walks the org's repository-relations graph up to maxDepth
// so a review can show A -> B -> C, not just A -> B. Relations are read
// symmetrically, matching ListRelatedRepositories and the previous direct-only
// query.
// relatedRepoGraph is the pure BFS behind loadRelatedRepos. It is separated so
// cycles (A→B→C→A) and the repo cap can be tested without a database.
// loadRepoCredentials resolves a repocache.Repo for an arbitrary org-scoped
// repository row, not keyed by review like loadReviewRepo - this is the one
// other entry point into "resolve a repo's clone URL + git credentials" that
// cross-repo linking needs. It also returns the LiveReview row identity used
// for friendly display.
// versionAliasedImpacts is the fallback matcher for the version-segment gap:
// a call site /v1/orders and a declared route /orders are the same endpoint,
// but the engine's exact matcher cannot see it. It only runs when exactly one
// side is versioned (both unversioned is the exact matcher's job).
//
// It intentionally does not suppress a match just because the reviewed
// project declares the same path locally: a consumer can expose its own
// /sms/send-otp while calling a different service's versioned
// /v1/sms/send-otp. Path-only matching cannot distinguish those without the
// call's host, so the impact is marked MatchKind=version-alias and carries
// lower confidence instead of being silently dropped.
// routeNameFromCallSite turns the engine's route-node qualified name
// (__route__POST__/v1/orders) into the route name path (/v1/orders).
// exactCallNorms records, per symbol, the normalized call paths the exact
// engine pass already resolved. The version-alias fallback skips those paths
// so it cannot add a second, path-colliding repo as a false positive.
// directVersionAliasedImpacts builds the same version-alias matches without
// loading full RepoGraphs (HANDLES/route scans), using only the touched
// symbols' outbound routes and each target's declared routes.
// callsFromGraph turns a project graph's outbound HTTP edges into the
// key → calls shape the shared matcher expects. Returns nil when the graph is
// unavailable, so the caller can fall back to the direct route-name query.
func callsFromGraph(g *serverscore.RepoGraph, symbolQNs []string) map[string][]crossrepolink.Call {
	if g == nil {
		return nil
	}
	out := make(map[string][]crossrepolink.Call, len(symbolQNs))
	for _, qn := range symbolQNs {
		for _, call := range g.Outbound[qn] {
			out[qn] = append(out[qn], crossrepolink.Call{Method: call.Method, Path: call.Name})
		}
	}
	return out
}

func diagnostic(kind, project string, err error) string {
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return kind + ":" + project + ":" + msg
}

// buildChains walks from each direct impact into the first repo's route
// handler and follows that handler's outbound HTTP calls into any other
// indexed related repo, up to maxDepth total hops. The direct impact already
// scores; chains are informational and add a small capped depth signal.
