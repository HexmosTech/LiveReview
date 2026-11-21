package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/crossrepolink"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
)

const (
	crossRepoMaxSymbols   = 500
	crossRepoMaxCalls     = 50
	crossRepoMaxPathBytes = 4096
	crossRepoMaxBodyBytes = 2 << 20 // 2 MiB
	crossRepoMaxImpacts   = 2000
	crossRepoMatchTimeout = 2 * time.Minute
	// Graph loading gets a single overall budget so several slow engine
	// invocations cannot push the request past the reverse proxy's timeout.
	// Repos loaded before it expires are cached for the next attempt.
	crossRepoGraphBudget   = 40 * time.Second
	crossRepoDefaultDepth  = 3
	crossRepoMaxDepthLimit = 3
)

// CrossRepoSymbolCalls is one touched symbol's outbound HTTP call sites, as
// observed by the CLI's own index. "key" is opaque to the server: it is echoed
// back in the response so the CLI can join the result onto the same symbol.
type CrossRepoSymbolCalls struct {
	Key   string               `json:"key"`
	Calls []crossrepolink.Call `json:"calls"`
}

type crossRepoMatchesRequest struct {
	Symbols []CrossRepoSymbolCalls `json:"symbols"`
	Depth   int                    `json:"depth,omitempty"`
}

type crossRepoRepoRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	WebURL   string `json:"web_url"`
}

type crossRepoRelatedRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	WebURL   string `json:"web_url"`
	Depth    int    `json:"depth"`
	Indexed  bool   `json:"indexed"`
}

// CrossRepoMatches handles POST
// /api/v1/diff-review/:review_id/cross-repo/matches.
//
// It is deliberately a read: it never writes review state. It may warm the
// related-repo cache the first time a repo is seen (same one-time cost any PR
// review pays), but reuses an existing index without fetching or reindexing.
func (s *Server) CrossRepoMatches(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok || orgID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	reviewID, err := strconv.ParseInt(c.Param("review_id"), 10, 64)
	if err != nil || reviewID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid review_id")
	}

	// Bound the body before decoding. Echo's binder has no implicit limit.
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, crossRepoMaxBodyBytes)
	var req crossRepoMatchesRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}
	symbols, err := normalizeCrossRepoSymbols(req.Symbols)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}
	if req.Depth <= 0 {
		req.Depth = crossRepoDefaultDepth
	}
	if req.Depth > crossRepoMaxDepthLimit {
		req.Depth = crossRepoMaxDepthLimit
	}

	// Even if the CLI disconnects, let a first-time related-repo index finish
	// (bounded) so the next attempt is warm. WithoutCancel keeps the DB/engine
	// calls alive past the request context.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request().Context()), crossRepoMatchTimeout)
	defer cancel()

	resp := map[string]interface{}{
		"review_id":     reviewID,
		"self":          nil,
		"related_repos": []crossRepoRelatedRepo{},
		"impacts":       map[string][]blastradius.CrossRepoImpact{},
		"chains":        map[string][]blastradius.CrossRepoChain{},
		"generated_at":  time.Now().UTC(),
	}
	diagnostics := []string{}
	addDiagnostic := func(msg string) { diagnostics = append(diagnostics, msg) }

	repoID, self, _, reason, err := s.resolveReviewRepositoryForCrossRepo(ctx, orgID, reviewID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to resolve review repository")
	}
	if repoID == 0 {
		addDiagnostic(reason)
		resp["diagnostics"] = diagnostics
		return JSONWithEnvelope(c, http.StatusOK, resp)
	}
	resp["self"] = self

	related, err := crossrepolink.LoadRelatedRepos(ctx, s.db, orgID, repoID, req.Depth, crossrepolink.DefaultMaxRepos)
	if err != nil {
		addDiagnostic("relations_unavailable: " + truncateError(err))
		resp["diagnostics"] = diagnostics
		return JSONWithEnvelope(c, http.StatusOK, resp)
	}
	if len(related) == 0 {
		addDiagnostic("no_related_repos_configured")
		resp["diagnostics"] = diagnostics
		return JSONWithEnvelope(c, http.StatusOK, resp)
	}

	// Load related repos sequentially. Each graph read is one heavyweight
	// codebase-memory-mcp invocation; running several at once makes every
	// query contend for the engine and can push each past its timeout. The
	// in-process graph cache makes repeat reviews effectively free.
	loadCtx, cancelLoad := context.WithTimeout(ctx, crossRepoGraphBudget)
	defer cancelLoad()
	graphs := make(map[string]*serverscore.RepoGraph, len(related))
	meta := make(map[string]crossrepolink.RepoMeta, len(related))
	relatedRefs := make([]crossRepoRelatedRepo, 0, len(related))
	for _, rel := range related {
		project, g, relMeta, err := crossrepolink.EnsureRepoGraph(loadCtx, s.db, repocache.DefaultRoot, orgID, rel.ID, false)
		if err != nil {
			addDiagnostic(fmt.Sprintf("repo:%d:%s", rel.ID, truncateError(err)))
			continue
		}
		graphs[project] = g
		meta[project] = relMeta
		relatedRefs = append(relatedRefs, crossRepoRelatedRepo{
			ID: relMeta.ID, FullName: relMeta.FullName, WebURL: relMeta.WebURL,
			Depth: rel.Depth, Indexed: true,
		})
	}
	resp["related_repos"] = relatedRefs
	if len(graphs) == 0 {
		addDiagnostic("no_related_repo_graphs_available")
		resp["diagnostics"] = diagnostics
		return JSONWithEnvelope(c, http.StatusOK, resp)
	}

	callsByKey := make(map[string][]crossrepolink.Call, len(symbols))
	callCount := 0
	for _, sym := range symbols {
		callsByKey[sym.Key] = sym.Calls
		callCount += len(sym.Calls)
	}
	impacts := crossrepolink.MatchCalls(callsByKey, graphs, meta, true)
	capped, dropped := applyImpactCap(impacts)
	if dropped > 0 {
		addDiagnostic(fmt.Sprintf("impact_cap: dropped %d matches", dropped))
	}
	resp["impacts"] = capped

	chains := map[string][]blastradius.CrossRepoChain{}
	impactCount := 0
	for key, list := range capped {
		impactCount += len(list)
		got := crossrepolink.BuildChains("", graphs, meta, list, req.Depth)
		if len(got) > 0 {
			chains[key] = got
		}
	}
	resp["chains"] = chains
	if len(diagnostics) > 0 {
		resp["diagnostics"] = diagnostics
	}

	if s.auditRecorder != nil {
		_ = s.auditRecorder.RecordEcho(c, auditlog.Event{
			Action:   auditlog.ActionRepositoryCrossRepoQuery,
			Outcome:  auditlog.OutcomeSuccess,
			Severity: auditlog.SeverityInfo,
			Details: auditlog.Details(
				"review_id", reviewID,
				"symbols", len(symbols),
				"calls", callCount,
				"impacts", impactCount,
				"related_repos", len(graphs),
			),
		})
	}
	return JSONWithEnvelope(c, http.StatusOK, resp)
}

func normalizeCrossRepoSymbols(in []CrossRepoSymbolCalls) ([]CrossRepoSymbolCalls, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("symbols is required")
	}
	if len(in) > crossRepoMaxSymbols {
		return nil, fmt.Errorf("too many symbols (max %d)", crossRepoMaxSymbols)
	}
	merged := make(map[string][]crossrepolink.Call, len(in))
	order := make([]string, 0, len(in))
	for _, sym := range in {
		key := strings.TrimSpace(sym.Key)
		if key == "" {
			return nil, fmt.Errorf("symbol key is required")
		}
		if len(key) > 512 {
			return nil, fmt.Errorf("symbol key is too long")
		}
		if len(sym.Calls) > crossRepoMaxCalls {
			return nil, fmt.Errorf("too many calls for a symbol (max %d)", crossRepoMaxCalls)
		}
		if _, seen := merged[key]; !seen {
			order = append(order, key)
		}
		for _, call := range sym.Calls {
			if strings.TrimSpace(call.Path) == "" {
				continue
			}
			if len(call.Path) > crossRepoMaxPathBytes {
				return nil, fmt.Errorf("call path is too long")
			}
			merged[key] = append(merged[key], crossrepolink.Call{
				Method: strings.ToUpper(strings.TrimSpace(call.Method)),
				Path:   strings.TrimSpace(call.Path),
			})
		}
	}
	out := make([]CrossRepoSymbolCalls, 0, len(order))
	for _, key := range order {
		out = append(out, CrossRepoSymbolCalls{Key: key, Calls: merged[key]})
	}
	return out, nil
}

// resolveReviewRepositoryForCrossRepo resolves a review to an org-scoped
// repositories row. CLI reviews store only the checkout directory basename in
// reviews.repository, so the remote URL captured at submit time is the primary
// identity; the old value is a best-effort fallback. No client-supplied id or
// URL is ever trusted past this org-scoped comparison.
func (s *Server) resolveReviewRepositoryForCrossRepo(ctx context.Context, orgID, reviewID int64) (repoID int64, ref *crossRepoRepoRef, remote string, reason string, err error) {
	var repository string
	var metadataRepoURL sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT repository, metadata->>'repo_remote_url' FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&repository, &metadataRepoURL)
	if err == sql.ErrNoRows {
		return 0, nil, "", "review_not_found", nil
	}
	if err != nil {
		return 0, nil, "", "", fmt.Errorf("load review: %w", err)
	}

	candidates := make([]string, 0, 2)
	if metadataRepoURL.Valid && strings.TrimSpace(metadataRepoURL.String) != "" {
		candidates = append(candidates, metadataRepoURL.String)
	}
	if strings.TrimSpace(repository) != "" {
		candidates = append(candidates, repository)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, full_name, web_url, COALESCE(clone_url, ''), COALESCE(ssh_url, '')
		 FROM repositories WHERE org_id = $1 ORDER BY id`, orgID)
	if err != nil {
		return 0, nil, "", "", fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()
	type candidateRow struct {
		id                           int64
		fullName, webURL, clone, ssh string
	}
	var repos []candidateRow
	for rows.Next() {
		var r candidateRow
		if err := rows.Scan(&r.id, &r.fullName, &r.webURL, &r.clone, &r.ssh); err != nil {
			return 0, nil, "", "", fmt.Errorf("scan repository: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, "", "", fmt.Errorf("list repositories: %w", err)
	}

	for _, candidate := range candidates {
		for _, r := range repos {
			if crossrepolink.RepositoryMatchesRemote(r.fullName, r.webURL, r.clone, r.ssh, candidate) {
				return r.id, &crossRepoRepoRef{ID: r.id, FullName: r.fullName, WebURL: r.webURL}, candidate, "", nil
			}
		}
	}
	return 0, nil, "", "repository_not_connected", nil
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return msg
}

// applyImpactCap keeps the response bounded and deterministic: keys are
// sorted before truncation so two identical requests never disagree.
func applyImpactCap(impacts map[string][]blastradius.CrossRepoImpact) (map[string][]blastradius.CrossRepoImpact, int) {
	keys := make([]string, 0, len(impacts))
	for k := range impacts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string][]blastradius.CrossRepoImpact, len(impacts))
	total := 0
	dropped := 0
	for _, k := range keys {
		list := impacts[k]
		if total+len(list) > crossRepoMaxImpacts {
			keep := crossRepoMaxImpacts - total
			if keep < 0 {
				keep = 0
			}
			dropped += len(list) - keep
			list = list[:keep]
		}
		if len(list) > 0 {
			out[k] = list
			total += len(list)
		}
	}
	return out, dropped
}
