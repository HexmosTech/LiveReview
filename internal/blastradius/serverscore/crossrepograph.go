package serverscore

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/HexmosTech/git-lrc/blastradius/client"
)

// RouteNode is one declared route in a project graph. FilePath is non-empty
// for routes extracted from source (controller/decorator, handler registration)
// and empty for the synthetic route nodes the engine creates from outbound
// call sites. Only declared routes are eligible as cross-repo targets -
// otherwise every caller's own "I call /orders" node would look like a
// service that exposes /orders.
type RouteNode struct {
	QualifiedName string
	Name          string
	Method        string
	FilePath      string
}

// OutboundRoute is one HTTP call edge leaving a symbol.
type OutboundRoute struct {
	Name          string
	QualifiedName string
	Method        string
}

// RepoGraph is the slice of one project's knowledge graph the cross-repo
// walker needs: declared routes, which handler serves each route, and which
// routes each symbol calls. Loaded once per project with three queries.
type RepoGraph struct {
	Project  string
	Routes   []RouteNode
	Handlers map[string][]string // route qualified name -> handler qualified names
	Outbound map[string][]OutboundRoute
}

// LoadRepoGraph reads the route/handler/call slice for an already-indexed
// project. The queries intentionally use only labels and properties observed
// in the engine's public graph (Route.label/file_path, HANDLES, HTTP_CALLS).
func LoadRepoGraph(ctx context.Context, project string) (*RepoGraph, error) {
	if strings.TrimSpace(project) == "" {
		return nil, fmt.Errorf("serverscore: project is required")
	}
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	// The engine pays a multi-second index-open cost on every invocation; 30s
	// is too tight when the host is also running reviews and the dashboard
	// cache. One minute still bounds a stuck query.
	c := &client.Client{Binary: binary, Project: project, Timeout: 60 * time.Second}
	g := &RepoGraph{Project: project, Handlers: map[string][]string{}, Outbound: map[string][]OutboundRoute{}}

	// One query instead of three. Each codebase-memory-mcp invocation pays a
	// multi-second index-open cost, so three separate route/handler/call
	// queries made the CLI match endpoint exceed the reverse proxy's timeout.
	// HANDLES + HTTP_CALLS together contain everything this struct needs:
	// a declared route is one with a source handler, and the handler row
	// carries the route's own name/method/file path.
	qr, err := c.QueryGraph(ctx,
		"MATCH (f)-[e:HANDLES|HTTP_CALLS]->(r) RETURN type(e) AS kind, f.qualified_name AS a, r.qualified_name AS b, r.name AS c, r.method AS d, r.file_path AS e2",
		0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load route/call graph for %s: %w", project, err)
	}
	seenRoute := map[string]bool{}
	for _, row := range qr.Rows {
		if len(row) < 6 {
			continue
		}
		kind, from, routeQN, routeName, method, filePath := row[0], row[1], row[2], row[3], row[4], row[5]
		switch kind {
		case "HANDLES":
			if from == "" || routeQN == "" {
				continue
			}
			g.Handlers[routeQN] = append(g.Handlers[routeQN], from)
			if !seenRoute[routeQN] {
				seenRoute[routeQN] = true
				g.Routes = append(g.Routes, RouteNode{
					QualifiedName: routeQN,
					Name:          routeName,
					Method:        method,
					FilePath:      filePath,
				})
			}
		case "HTTP_CALLS":
			if from == "" || routeName == "" {
				continue
			}
			g.Outbound[from] = append(g.Outbound[from], OutboundRoute{
				Name:          routeName,
				QualifiedName: routeQN,
				Method:        method,
			})
		}
	}
	return g, nil
}

// DeclaredRoutes returns only the routes extracted from source, which are the
// only valid cross-repo targets.
func (g *RepoGraph) DeclaredRoutes() []RouteNode {
	if g == nil {
		return nil
	}
	out := make([]RouteNode, 0, len(g.Routes))
	for _, r := range g.Routes {
		if r.FilePath != "" {
			out = append(out, r)
		}
	}
	return out
}

var routeVersionPrefix = regexp.MustCompile(`^/v[0-9]+`)

// NormalizeRoutePath reduces a call-site or route name to a version-agnostic
// comparable path. It strips scheme+host, query/fragment, cleans the path, and
// removes one leading URI version segment (/v1, /v2, ...). The bool reports
// whether a version segment was present, so callers can restrict fallback
// matching to the version-mismatch case instead of second-guessing the
// engine's exact matcher on ordinary routes.
func NormalizeRoutePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Path != "" {
			raw = u.Path
		}
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if raw == "" {
		return "/", false
	}
	raw = path.Clean(raw)
	versioned := false
	if m := routeVersionPrefix.FindString(raw); m != "" {
		rest := strings.TrimPrefix(raw, m)
		if rest == "" || strings.HasPrefix(rest, "/") {
			versioned = true
			raw = rest
			if raw == "" {
				raw = "/"
			}
		}
	}
	return raw, versioned
}
