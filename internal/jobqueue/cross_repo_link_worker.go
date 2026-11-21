package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
	"github.com/livereview/internal/blobstore"
	"github.com/livereview/internal/githubapp"
	storageblastradius "github.com/livereview/storage/blastradius"
	storagegithub "github.com/livereview/storage/providers/github"
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

	related, err := w.loadRelatedRepos(ctx, orgID, repoID, crossRepoMaxDepth)
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
	_, mainMeta, err := w.loadRepoCredentials(ctx, orgID, repoID)
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

	settings, err := repocache.LoadSettings(ctx, w.db)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot read settings")
		return nil
	}

	// project -> LiveReview repository metadata, so every impact/chip can show
	// a friendly name and link instead of the engine's cache-derived project id.
	metaByProject := map[string]repoMeta{report.Project: mainMeta}
	graphs := map[string]*serverscore.RepoGraph{}

	var targetProjects []string
	var unlocks []func()
	defer func() {
		for _, u := range unlocks {
			u()
		}
	}()
	for _, rel := range related {
		relRepo, relMeta, err := w.loadRepoCredentials(ctx, orgID, rel.id)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] cannot load related repo credentials")
			continue
		}
		if size, ok := repocache.TooLarge(blastRadiusRoot, relRepo); ok && size > settings.MaxBytes() {
			continue
		}
		checkout, unlock, err := repocache.PrepareDefaultBranch(ctx, blastRadiusRoot, relRepo)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] related repo checkout failed")
			continue
		}
		unlocks = append(unlocks, unlock)
		relProject, err := serverscore.IndexOnly(ctx, checkout.Dir)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] related repo indexing failed")
			continue
		}
		_ = checkout.RecordIndex(relProject)
		targetProjects = append(targetProjects, relProject)
		metaByProject[relProject] = relMeta
		if g, gerr := serverscore.LoadRepoGraph(ctx, relProject); gerr != nil {
			logger.Warn().Err(gerr).Str("project", relProject).Msg("[cross_repo_link] cannot read related repo graph")
			report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("related_graph", relProject, gerr))
		} else {
			graphs[relProject] = g
		}
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
	// caller using /v1/orders never matches a route indexed as /orders. Add a
	// version-aliased fallback for exactly that mismatch, then dedupe against
	// the engine's exact matches.
	mainGraph, gerr := serverscore.LoadRepoGraph(ctx, report.Project)
	if gerr != nil {
		logger.Warn().Err(gerr).Msg("[cross_repo_link] cannot read main repo graph; using direct-only fallback")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("main_graph", report.Project, gerr))
	} else {
		graphs[report.Project] = mainGraph
		mergeImpacts(impactsBySymbol, versionAliasedImpacts(mainGraph, graphs, symbolQNs, exactCallNorms(impactsBySymbol)))
	}
	// Direct-only fallback: one symbol query + one declared-routes query per
	// target. It does not need HANDLES or a full graph read, so a partial graph
	// failure cannot silently suppress a version-aliased direct impact. Run it
	// even when exact matches exist: a review can touch symbols with exact
	// matches and symbols that only match across a version segment.
	if direct, derr := directVersionAliasedImpacts(ctx, report.Project, symbolQNs, targetProjects, exactCallNorms(impactsBySymbol)); derr != nil {
		logger.Warn().Err(derr).Msg("[cross_repo_link] direct version-alias fallback failed")
		report.CrossRepoDiagnostics = append(report.CrossRepoDiagnostics, diagnostic("direct_fallback", report.Project, derr))
	} else {
		mergeImpacts(impactsBySymbol, direct)
	}

	// Enrich every impact with the LiveReview repository row it points into.
	for qn, impacts := range impactsBySymbol {
		for i := range impacts {
			if impacts[i].MatchKind == "" {
				impacts[i].MatchKind = "exact"
			}
			enrichImpact(&impacts[i], metaByProject)
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
			chains := buildChains(mainGraph, graphs, metaByProject, impacts, crossRepoMaxDepth)
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

type relatedRepoRef struct {
	id    int64
	depth int
}

// loadRelatedRepos walks the org's repository-relations graph up to maxDepth
// so a review can show A -> B -> C, not just A -> B. Relations are read
// symmetrically, matching ListRelatedRepositories and the previous direct-only
// query.
func (w *CrossRepoLinkWorker) loadRelatedRepos(ctx context.Context, orgID, repoID int64, maxDepth int) ([]relatedRepoRef, error) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT repository_id, related_repository_id FROM repository_relations WHERE org_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	adj := map[int64][]int64{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	return relatedRepoGraph(adj, repoID, maxDepth, crossRepoMaxRepos), nil
}

// relatedRepoGraph is the pure BFS behind loadRelatedRepos. It is separated so
// cycles (A→B→C→A) and the repo cap can be tested without a database.
func relatedRepoGraph(adj map[int64][]int64, repoID int64, maxDepth, maxRepos int) []relatedRepoRef {
	seen := map[int64]bool{repoID: true}
	frontier := []int64{repoID}
	var out []relatedRepoRef
	for depth := 1; depth <= maxDepth && len(frontier) > 0 && len(out) < maxRepos; depth++ {
		var next []int64
		for _, id := range frontier {
			for _, nb := range adj[id] {
				if seen[nb] {
					continue
				}
				if len(out) >= maxRepos {
					break
				}
				seen[nb] = true
				out = append(out, relatedRepoRef{id: nb, depth: depth})
				next = append(next, nb)
			}
			if len(out) >= maxRepos {
				break
			}
		}
		frontier = next
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].depth != out[j].depth {
			return out[i].depth < out[j].depth
		}
		return out[i].id < out[j].id
	})
	return out
}

type repoMeta struct {
	ID       int64
	FullName string
	WebURL   string
}

// loadRepoCredentials resolves a repocache.Repo for an arbitrary org-scoped
// repository row, not keyed by review like loadReviewRepo - this is the one
// other entry point into "resolve a repo's clone URL + git credentials" that
// cross-repo linking needs. It also returns the LiveReview row identity used
// for friendly display.
func (w *CrossRepoLinkWorker) loadRepoCredentials(ctx context.Context, orgID, repoID int64) (repocache.Repo, repoMeta, error) {
	var r repocache.Repo
	var meta repoMeta
	var connectorID int64
	var provider, cloneURL, fullName, webURL string
	err := w.db.QueryRowContext(ctx,
		`SELECT connector_id, provider, COALESCE(clone_url, ''), full_name, web_url FROM repositories WHERE id = $1 AND org_id = $2`,
		repoID, orgID,
	).Scan(&connectorID, &provider, &cloneURL, &fullName, &webURL)
	if err != nil {
		return r, meta, fmt.Errorf("load repository %d: %w", repoID, err)
	}
	if cloneURL == "" {
		return r, meta, fmt.Errorf("repository %d has no clone URL", repoID)
	}

	var tokenProvider, providerURL, accessToken, tokenType, patToken string
	err = w.db.QueryRowContext(ctx,
		`SELECT provider, provider_url, COALESCE(access_token, ''), COALESCE(token_type, ''), COALESCE(pat_token, '')
		 FROM integration_tokens WHERE id = $1 AND org_id = $2`,
		connectorID, orgID,
	).Scan(&tokenProvider, &providerURL, &accessToken, &tokenType, &patToken)
	if err != nil {
		return r, meta, fmt.Errorf("load connector %d: %w", connectorID, err)
	}

	meta = repoMeta{ID: repoID, FullName: fullName, WebURL: webURL}
	r = repocache.Repo{OrgID: orgID, ConnectorID: connectorID, Provider: provider, CloneURL: cloneURL}
	if tokenType == "GITHUB_APP" {
		store := storagegithub.NewTokenStore(w.db)
		rec, recErr := store.GetGitHubTokenByConnectorID(connectorID)
		if recErr != nil || rec == nil || rec.OrgID != orgID {
			return r, meta, fmt.Errorf("load GitHub App connector %d: %v", connectorID, recErr)
		}
		fresh, tokErr := githubapp.ConnectorToken(ctx, store, rec)
		if tokErr != nil {
			return r, meta, fmt.Errorf("GitHub App token for connector %d: %w", connectorID, tokErr)
		}
		patToken = fresh
	}
	r.Username, r.Token = gitCredentials(tokenProvider, accessToken, tokenType, patToken)
	return r, meta, nil
}

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
func versionAliasedImpacts(main *serverscore.RepoGraph, graphs map[string]*serverscore.RepoGraph, symbolQNs []string, exactBySymbol map[string]map[string]bool) map[string][]blastradius.CrossRepoImpact {
	out := map[string][]blastradius.CrossRepoImpact{}
	if main == nil {
		return out
	}
	for _, qn := range symbolQNs {
		calls := main.Outbound[qn]
		if len(calls) == 0 {
			continue
		}
		seen := map[string]bool{}
		for _, call := range calls {
			callNorm, callVersioned := serverscore.NormalizeRoutePath(call.Name)
			if callNorm == "" || callNorm == "/" {
				continue
			}
			// The engine already found an exact target for this call; do not
			// also claim a version-aliased target that merely shares the path.
			// Without this, a shared route like /e-kyc/read-aadhaar matches
			// every linked repo that declares it, not just the real callee.
			if exactBySymbol[qn][callNorm] {
				continue
			}
			for project, g := range graphs {
				if g == nil || project == main.Project {
					continue
				}
				for _, route := range g.DeclaredRoutes() {
					routeNorm, routeVersioned := serverscore.NormalizeRoutePath(route.Name)
					if routeNorm != callNorm || routeVersioned == callVersioned {
						continue
					}
					key := project + "|" + route.QualifiedName
					if seen[key] {
						continue
					}
					seen[key] = true
					out[qn] = append(out[qn], blastradius.CrossRepoImpact{
						RelatedRepo: project,
						CallSite:    route.QualifiedName,
						EdgeType:    "CROSS_HTTP_CALLS",
						Confidence:  0.6,
						MatchKind:   "version-alias",
					})
				}
			}
		}
	}
	return out
}

// routeNameFromCallSite turns the engine's route-node qualified name
// (__route__POST__/v1/orders) into the route name path (/v1/orders).
func routeNameFromCallSite(callSite string) string {
	const prefix = "__route__"
	if !strings.HasPrefix(callSite, prefix) {
		return callSite
	}
	rest := strings.TrimPrefix(callSite, prefix)
	if i := strings.Index(rest, "__"); i >= 0 {
		return rest[i+2:]
	}
	return callSite
}

// exactCallNorms records, per symbol, the normalized call paths the exact
// engine pass already resolved. The version-alias fallback skips those paths
// so it cannot add a second, path-colliding repo as a false positive.
func exactCallNorms(impactsBySymbol map[string][]blastradius.CrossRepoImpact) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for qn, impacts := range impactsBySymbol {
		for _, im := range impacts {
			norm, _ := serverscore.NormalizeRoutePath(routeNameFromCallSite(im.CallSite))
			if norm == "" || norm == "/" {
				continue
			}
			if out[qn] == nil {
				out[qn] = map[string]bool{}
			}
			out[qn][norm] = true
		}
	}
	return out
}

// directVersionAliasedImpacts builds the same version-alias matches without
// loading full RepoGraphs (HANDLES/route scans), using only the touched
// symbols' outbound routes and each target's declared routes.
func directVersionAliasedImpacts(ctx context.Context, mainProject string, symbolQNs, targetProjects []string, exactBySymbol map[string]map[string]bool) (map[string][]blastradius.CrossRepoImpact, error) {
	calls, err := serverscore.HTTPCallRouteNamesForSymbols(ctx, mainProject, symbolQNs)
	if err != nil {
		return nil, err
	}
	main := &serverscore.RepoGraph{Project: mainProject, Outbound: map[string][]serverscore.OutboundRoute{}}
	for qn, names := range calls {
		for _, name := range names {
			main.Outbound[qn] = append(main.Outbound[qn], serverscore.OutboundRoute{Name: name})
		}
	}
	graphs := map[string]*serverscore.RepoGraph{mainProject: main}
	for _, project := range targetProjects {
		routes, rerr := serverscore.DeclaredRoutes(ctx, project)
		if rerr != nil {
			return nil, rerr
		}
		graphs[project] = &serverscore.RepoGraph{Project: project, Routes: routes}
	}
	return versionAliasedImpacts(main, graphs, symbolQNs, exactBySymbol), nil
}

func diagnostic(kind, project string, err error) string {
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return kind + ":" + project + ":" + msg
}

func mergeImpacts(dst map[string][]blastradius.CrossRepoImpact, src map[string][]blastradius.CrossRepoImpact) {
	for qn, extra := range src {
		existing := dst[qn]
		seen := map[string]bool{}
		for _, im := range existing {
			seen[im.RelatedRepo+"|"+im.CallSite] = true
		}
		for _, im := range extra {
			if seen[im.RelatedRepo+"|"+im.CallSite] {
				continue
			}
			existing = append(existing, im)
			seen[im.RelatedRepo+"|"+im.CallSite] = true
		}
		dst[qn] = existing
	}
}

func enrichImpact(im *blastradius.CrossRepoImpact, meta map[string]repoMeta) {
	if m, ok := meta[im.RelatedRepo]; ok {
		im.RelatedRepoID = m.ID
		im.RelatedRepoName = m.FullName
		im.RelatedRepoWebURL = m.WebURL
	}
}

// buildChains walks from each direct impact into the first repo's route
// handler and follows that handler's outbound HTTP calls into any other
// indexed related repo, up to maxDepth total hops. The direct impact already
// scores; chains are informational and add a small capped depth signal.
func buildChains(main *serverscore.RepoGraph, graphs map[string]*serverscore.RepoGraph, meta map[string]repoMeta, impacts []blastradius.CrossRepoImpact, maxDepth int) []blastradius.CrossRepoChain {
	if maxDepth < 2 {
		return nil
	}
	var chains []blastradius.CrossRepoChain
	seen := map[string]bool{}
	for _, impact := range impacts {
		g := graphs[impact.RelatedRepo]
		if g == nil {
			continue
		}
		direct := blastradius.CrossRepoHop{
			RelatedRepo:       impact.RelatedRepo,
			RelatedRepoID:     impact.RelatedRepoID,
			RelatedRepoName:   impact.RelatedRepoName,
			RelatedRepoWebURL: impact.RelatedRepoWebURL,
			CallSite:          impact.CallSite,
			EdgeType:          impact.EdgeType,
		}
		handlers := g.Handlers[impact.CallSite]
		if len(handlers) == 0 {
			continue
		}
		direct.Handler = handlers[0]
		visited := map[string]bool{main.Project: true, impact.RelatedRepo: true}
		steps := 0
		for _, h := range handlers {
			walkChains(g, h, visited, []blastradius.CrossRepoHop{direct}, graphs, meta, maxDepth, &chains, seen, &steps)
		}
		if len(chains) >= crossRepoMaxChainsPerSymbol {
			break
		}
	}
	return chains
}

func walkChains(g *serverscore.RepoGraph, handler string, visited map[string]bool, hops []blastradius.CrossRepoHop, graphs map[string]*serverscore.RepoGraph, meta map[string]repoMeta, maxDepth int, out *[]blastradius.CrossRepoChain, seen map[string]bool, steps *int) {
	if len(*out) >= crossRepoMaxChainsPerSymbol || g == nil || steps == nil {
		return
	}
	if *steps >= crossRepoMaxChainSteps {
		return
	}
	if len(hops) >= maxDepth {
		return
	}
	for _, call := range g.Outbound[handler] {
		*steps++
		if *steps >= crossRepoMaxChainSteps {
			return
		}
		callNorm, _ := serverscore.NormalizeRoutePath(call.Name)
		if callNorm == "" || callNorm == "/" {
			continue
		}
		for project, target := range graphs {
			if target == nil || visited[project] {
				continue
			}
			for _, route := range target.DeclaredRoutes() {
				routeNorm, _ := serverscore.NormalizeRoutePath(route.Name)
				if routeNorm != callNorm {
					continue
				}
				m := meta[project]
				hop := blastradius.CrossRepoHop{
					RelatedRepo:       project,
					RelatedRepoID:     m.ID,
					RelatedRepoName:   m.FullName,
					RelatedRepoWebURL: m.WebURL,
					CallSite:          route.QualifiedName,
					EdgeType:          "CROSS_HTTP_CALLS",
				}
				handlers := target.Handlers[route.QualifiedName]
				if len(handlers) > 0 {
					hop.Handler = handlers[0]
				}
				next := append(append([]blastradius.CrossRepoHop{}, hops...), hop)
				sig := blastradius.ChainSignature(blastradius.CrossRepoChain{Hops: next})
				if !seen[sig] {
					seen[sig] = true
					*out = append(*out, blastradius.CrossRepoChain{Hops: next})
				}
				if len(*out) >= crossRepoMaxChainsPerSymbol {
					return
				}
				for _, nextHandler := range handlers {
					nextVisited := map[string]bool{}
					for k, v := range visited {
						nextVisited[k] = v
					}
					nextVisited[project] = true
					walkChains(target, nextHandler, nextVisited, next, graphs, meta, maxDepth, out, seen, steps)
				}
			}
		}
	}
}
