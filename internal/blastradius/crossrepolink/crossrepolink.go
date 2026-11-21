// Package crossrepolink is the shared "linked repositories" layer behind
// cross-repo blast radius. It owns:
//
//   - the org-scoped BFS over repository_relations (cycle-safe, bounded),
//   - resolving a repository row to a repocache checkout + git credentials,
//   - reusing an already-indexed related repo (or indexing it once, without
//     fetching when a checkout already exists),
//   - matching outbound HTTP calls to declared routes (exact + version alias),
//   - walking multi-hop chains with visited-set/step/chain caps.
//
// Both the server-side cross_repo_link worker and the CLI-facing
// /diff-review/:id/cross-repo/matches endpoint use this package, so the two
// paths cannot drift.
package crossrepolink

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
	"github.com/livereview/internal/githubapp"
	storagegithub "github.com/livereview/storage/providers/github"
)

const (
	// DefaultMaxDepth bounds how many repo hops (including the direct one) the
	// chain walk may traverse.
	DefaultMaxDepth = 3
	// DefaultMaxRepos bounds how many related repos one lookup will consider.
	DefaultMaxRepos = 10
	// MaxChainsPerSymbol caps the chains attached to one symbol.
	MaxChainsPerSymbol = 10
	// MaxChainSteps is a hard per-symbol walk budget.
	MaxChainSteps = 400
)

// RepoMeta is the org-scoped repository identity used for display/links.
type RepoMeta struct {
	ID       int64
	FullName string
	WebURL   string
}

// Reading a related-repo graph costs three codebase-memory-mcp invocations
// (each reloads the index), so keep a short-lived in-process cache for the
// read-only CLI path. A related repo changes far less often than that, and a
// PR review of that repo will reindex it; the TTL bounds staleness for
// everything else. refresh=true (the server worker) bypasses the cache.
const graphCacheTTL = 10 * time.Minute

type cachedRepoGraph struct {
	graph   *serverscore.RepoGraph
	expires time.Time
}

var repoGraphCache sync.Map // project -> cachedRepoGraph

func cachedGraph(project string) (*serverscore.RepoGraph, bool) {
	v, ok := repoGraphCache.Load(project)
	if !ok {
		return nil, false
	}
	entry := v.(cachedRepoGraph)
	if time.Now().After(entry.expires) {
		repoGraphCache.Delete(project)
		return nil, false
	}
	return entry.graph, true
}

func storeGraph(project string, g *serverscore.RepoGraph) {
	if project == "" || g == nil {
		return
	}
	repoGraphCache.Store(project, cachedRepoGraph{graph: g, expires: time.Now().Add(graphCacheTTL)})
}

// RelatedRepo is one related repository reached by the BFS, with its hop depth.
type RelatedRepo struct {
	ID    int64
	Depth int
}

// Call is one outbound HTTP call site reported by the CLI.
type Call struct {
	Method string `json:"method,omitempty"`
	Path   string `json:"path"`
}

// LoadRelatedRepos walks the org's repository-relations graph up to maxDepth.
// Relations are read symmetrically, matching ListRelatedRepositories.
func LoadRelatedRepos(ctx context.Context, db *sql.DB, orgID, repoID int64, maxDepth, maxRepos int) ([]RelatedRepo, error) {
	rows, err := db.QueryContext(ctx,
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
	return RelatedRepoGraph(adj, repoID, maxDepth, maxRepos), nil
}

// RelatedRepoGraph is the pure BFS behind LoadRelatedRepos. Separated so
// cycles and the repo cap can be tested without a database.
func RelatedRepoGraph(adj map[int64][]int64, repoID int64, maxDepth, maxRepos int) []RelatedRepo {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if maxRepos < 1 {
		maxRepos = DefaultMaxRepos
	}
	seen := map[int64]bool{repoID: true}
	frontier := []int64{repoID}
	var out []RelatedRepo
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
				out = append(out, RelatedRepo{ID: nb, Depth: depth})
				next = append(next, nb)
			}
			if len(out) >= maxRepos {
				break
			}
		}
		frontier = next
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// LoadRepoCredentials resolves a repocache.Repo for an arbitrary org-scoped
// repository row, plus the identity used for friendly display/links.
func LoadRepoCredentials(ctx context.Context, db *sql.DB, orgID, repoID int64) (repocache.Repo, RepoMeta, error) {
	var r repocache.Repo
	var meta RepoMeta
	var connectorID int64
	var provider, cloneURL, fullName, webURL string
	err := db.QueryRowContext(ctx,
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
	err = db.QueryRowContext(ctx,
		`SELECT provider, provider_url, COALESCE(access_token, ''), COALESCE(token_type, ''), COALESCE(pat_token, '')
		 FROM integration_tokens WHERE id = $1 AND org_id = $2`,
		connectorID, orgID,
	).Scan(&tokenProvider, &providerURL, &accessToken, &tokenType, &patToken)
	if err != nil {
		return r, meta, fmt.Errorf("load connector %d: %w", connectorID, err)
	}

	meta = RepoMeta{ID: repoID, FullName: fullName, WebURL: webURL}
	r = repocache.Repo{OrgID: orgID, ConnectorID: connectorID, Provider: provider, CloneURL: cloneURL}
	if tokenType == "GITHUB_APP" {
		store := storagegithub.NewTokenStore(db)
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

// gitCredentials centralizes the provider-specific username/token convention
// shared by the review worker and cross-repo indexing. Copied verbatim from
// the worker so a refactor cannot silently change how a related repo clones.
func gitCredentials(provider, accessToken, tokenType, patToken string) (username, token string) {
	token = accessToken
	// GitHub App rows keep their (hourly) installation token in pat_token and leave access_token empty.
	if (tokenType == "PAT" || tokenType == "GITHUB_APP") && patToken != "" {
		token = patToken
	}
	switch {
	case strings.HasPrefix(provider, "gitea"):
		var p struct{ Pat, Username string }
		if json.Unmarshal([]byte(strings.TrimSpace(patToken)), &p) == nil && p.Pat != "" {
			return p.Username, p.Pat
		}
	case strings.HasPrefix(provider, "bitbucket") && tokenType == "PAT":
		// Atlassian API tokens use this fixed git username; OAuth tokens use x-token-auth (repocache default).
		return "x-bitbucket-api-token-auth", token
	}
	return "", token // repocache fills in the provider's fixed username
}

// EnsureRepoGraph returns an indexed graph for repoID. With refresh=false
// (the CLI path) it reuses an existing index whenever possible:
//
//  1. recorded project + a successful graph read -> zero engine mutation;
//  2. existing checkout, no usable index -> index in place (no fetch);
//  3. no checkout -> clone the default branch once, then index.
//
// With refresh=true (the server PR worker, which historically fetched the
// related repo's default branch on every review) it always goes through
// PrepareDefaultBranch + IndexOnly so a related repo's new routes are picked
// up. The repo cache settings are respected either way (disabled or too-large
// entries are skipped with an error the caller records as a diagnostic).
func EnsureRepoGraph(ctx context.Context, db *sql.DB, root string, orgID, repoID int64, refresh bool) (string, *serverscore.RepoGraph, RepoMeta, error) {
	repo, meta, err := LoadRepoCredentials(ctx, db, orgID, repoID)
	if err != nil {
		return "", nil, meta, err
	}

	settings, err := repocache.LoadSettings(ctx, db)
	if err != nil {
		return "", nil, meta, fmt.Errorf("read repo cache settings: %w", err)
	}
	if !settings.Enabled {
		return "", nil, meta, fmt.Errorf("repo cache is disabled; cannot read related-repo graph")
	}
	if size, ok := repocache.TooLarge(root, repo); ok && size > settings.MaxBytes() {
		return "", nil, meta, fmt.Errorf("related repo is larger than the repo-cache limit")
	}

	if !refresh {
		// 1. Already indexed: read it without touching git.
		if project, ok := repocache.ProjectIndex(root, repo); ok {
			if g, ok := cachedGraph(project); ok {
				return project, g, meta, nil
			}
			g, gerr := serverscore.LoadRepoGraph(ctx, project)
			if gerr == nil {
				storeGraph(project, g)
				return project, g, meta, nil
			}
			// The recorded project may have been evicted from the engine; only
			// in that case is a rebuild worth the cost. If the project is still
			// indexed, surface the read error instead of re-indexing a large
			// repo on every request.
			if indexed, ierr := serverscore.ProjectIndexed(ctx, project); ierr != nil || indexed {
				return project, nil, meta, fmt.Errorf("read related repo graph: %w", gerr)
			}
		}

		// 2. Existing checkout: index it in place. No fetch, no re-clone.
		entry, entryErr := repocache.EntryDir(root, repo)
		checkoutDir := ""
		if entryErr == nil {
			candidate := filepath.Join(entry, "repo")
			if st, statErr := os.Stat(candidate); statErr == nil && st.IsDir() {
				checkoutDir = candidate
			}
		}

		if checkoutDir != "" {
			project, ierr := serverscore.IndexOnly(ctx, checkoutDir)
			if ierr != nil {
				return "", nil, meta, fmt.Errorf("index related repo in place: %w", ierr)
			}
			if err := recordIndex(entry, project); err != nil {
				// Non-fatal: the graph below is still usable.
				_ = err
			}
			g, gerr := serverscore.LoadRepoGraph(ctx, project)
			if gerr != nil {
				return project, nil, meta, fmt.Errorf("read related repo graph: %w", gerr)
			}
			storeGraph(project, g)
			return project, g, meta, nil
		}
	}

	// 3. First use: clone the default branch once, then index.
	checkout, unlock, err := repocache.PrepareDefaultBranch(ctx, root, repo)
	if err != nil {
		return "", nil, meta, fmt.Errorf("checkout related repo: %w", err)
	}
	defer unlock()
	project, err := serverscore.IndexOnly(ctx, checkout.Dir)
	if err != nil {
		return "", nil, meta, fmt.Errorf("index related repo: %w", err)
	}
	if err := checkout.RecordIndex(project); err != nil {
		_ = err
	}
	g, err := serverscore.LoadRepoGraph(ctx, project)
	if err != nil {
		return project, nil, meta, fmt.Errorf("read related repo graph: %w", err)
	}
	storeGraph(project, g)
	return project, g, meta, nil
}

func recordIndex(entry, project string) error {
	if entry == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(entry, ".index"), []byte(project), 0o644)
}

// MatchCalls matches client-supplied outbound calls to declared routes in the
// supplied related-repo graphs. Exact path matches (method-aware when both
// sides know it) win over version-alias matches (one side has a leading /vN).
// includeExact=false is used by the server worker, whose exact matches already
// come from the engine's CROSS_* edges.
func MatchCalls(callsByKey map[string][]Call, graphs map[string]*serverscore.RepoGraph, meta map[string]RepoMeta, includeExact bool) map[string][]blastradius.CrossRepoImpact {
	out := map[string][]blastradius.CrossRepoImpact{}
	for key, calls := range callsByKey {
		seen := map[string]bool{}
		for _, call := range calls {
			callNorm, callVersioned := serverscore.NormalizeRoutePath(call.Path)
			if callNorm == "" || callNorm == "/" {
				continue
			}
			// Stable output order regardless of map iteration.
			projects := make([]string, 0, len(graphs))
			for project := range graphs {
				projects = append(projects, project)
			}
			sort.Strings(projects)
			for _, project := range projects {
				g := graphs[project]
				if g == nil {
					continue
				}
				for _, route := range g.DeclaredRoutes() {
					routeNorm, routeVersioned := serverscore.NormalizeRoutePath(route.Name)
					if routeNorm == "" || routeNorm != callNorm {
						continue
					}
					if call.Method != "" && route.Method != "" && !strings.EqualFold(call.Method, route.Method) {
						continue
					}
					kind, confidence := "exact", 0.95
					if callVersioned != routeVersioned {
						kind, confidence = "version-alias", 0.6
					} else if !includeExact {
						continue
					}
					dedupe := project + "|" + route.QualifiedName
					if seen[dedupe] {
						continue
					}
					seen[dedupe] = true
					im := blastradius.CrossRepoImpact{
						RelatedRepo: project,
						CallSite:    route.QualifiedName,
						EdgeType:    "CROSS_HTTP_CALLS",
						Confidence:  confidence,
						MatchKind:   kind,
					}
					EnrichImpact(&im, meta)
					out[key] = append(out[key], im)
				}
			}
		}
	}
	return out
}

// EnrichImpact fills the friendly repository identity from meta, keyed by the
// engine project id in RelatedRepo.
func EnrichImpact(im *blastradius.CrossRepoImpact, meta map[string]RepoMeta) {
	if m, ok := meta[im.RelatedRepo]; ok {
		im.RelatedRepoID = m.ID
		im.RelatedRepoName = m.FullName
		im.RelatedRepoWebURL = m.WebURL
	}
}

// MergeImpacts appends src into dst, deduping by repository + route.
func MergeImpacts(dst, src map[string][]blastradius.CrossRepoImpact) {
	for key, extra := range src {
		existing := dst[key]
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
		dst[key] = existing
	}
}

// BuildChains walks from each direct impact into the first repo's route
// handler and follows that handler's outbound HTTP calls into any other
// indexed related repo, up to maxDepth total hops. The direct impact already
// scores; chains are informational and add a small capped depth signal.
func BuildChains(selfProject string, graphs map[string]*serverscore.RepoGraph, meta map[string]RepoMeta, impacts []blastradius.CrossRepoImpact, maxDepth int) []blastradius.CrossRepoChain {
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
		visited := map[string]bool{impact.RelatedRepo: true}
		if selfProject != "" {
			visited[selfProject] = true
		}
		steps := 0
		for _, h := range handlers {
			walkChains(g, h, visited, []blastradius.CrossRepoHop{direct}, graphs, meta, maxDepth, &chains, seen, &steps)
		}
		if len(chains) >= MaxChainsPerSymbol {
			break
		}
	}
	return chains
}

func walkChains(g *serverscore.RepoGraph, handler string, visited map[string]bool, hops []blastradius.CrossRepoHop, graphs map[string]*serverscore.RepoGraph, meta map[string]RepoMeta, maxDepth int, out *[]blastradius.CrossRepoChain, seen map[string]bool, steps *int) {
	if len(*out) >= MaxChainsPerSymbol || g == nil || steps == nil {
		return
	}
	if *steps >= MaxChainSteps {
		return
	}
	if len(hops) >= maxDepth {
		return
	}
	for _, call := range g.Outbound[handler] {
		*steps++
		if *steps >= MaxChainSteps {
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
				if len(*out) >= MaxChainsPerSymbol {
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

// NormalizeRemoteRepoURL reduces the URL forms git remotes use to a
// comparable host/path identity:
//
//	https://host/group/repo.git       -> host/group/repo
//	ssh://git@host/group/repo.git     -> host/group/repo
//	git@host:group/repo.git           -> host/group/repo
//	https://host/group/repo/-/tree/x  -> host/group/repo
//
// The host is lowercased; the path keeps its case (providers are
// case-sensitive there).
func NormalizeRemoteRepoURL(raw string) (host, path string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	// scp-like: [user@]host:path (but not a scheme:// URL)
	if !strings.Contains(raw, "://") {
		if at := strings.Index(raw, "@"); at >= 0 {
			if colon := strings.Index(raw[at:], ":"); colon > 0 {
				host = raw[at+1 : at+colon]
				path = raw[at+colon+1:]
				return cleanRepoHostPath(host, path)
			}
		}
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return cleanRepoHostPath(u.Host, u.Path)
	}
	return cleanRepoHostPath("", raw)
}

func cleanRepoHostPath(host, path string) (string, string) {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i] // strip port
	}
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if i := strings.Index(path, "/-/"); i >= 0 {
		path = path[:i]
	}
	path = strings.Trim(path, "/")
	return host, path
}

// RepositoryMatchesRemote reports whether a repositories row's identity
// (full_name + web/clone/ssh URLs) refers to the same repository as remote.
func RepositoryMatchesRemote(fullName, webURL, cloneURL, sshURL, remote string) bool {
	remoteHost, remotePath := NormalizeRemoteRepoURL(remote)
	if remotePath == "" {
		return false
	}
	if remoteHost == "" {
		// Caller passed only full_name/path.
		return strings.EqualFold(strings.Trim(fullName, "/"), remotePath)
	}
	candidates := []string{webURL, cloneURL, sshURL}
	for _, c := range candidates {
		h, p := NormalizeRemoteRepoURL(c)
		if p != "" && p == remotePath && (h == "" || h == remoteHost) {
			return true
		}
	}
	return strings.EqualFold(strings.Trim(fullName, "/"), remotePath)
}
