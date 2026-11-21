package serverscore

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strings"

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
	FilePath      string
}

// OutboundRoute is one HTTP call edge leaving a symbol.
type OutboundRoute struct {
	Name          string
	QualifiedName string
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
	c := &client.Client{Binary: binary, Project: project}
	g := &RepoGraph{Project: project, Handlers: map[string][]string{}, Outbound: map[string][]OutboundRoute{}}

	qr, err := c.QueryGraph(ctx, "MATCH (r) WHERE r.label='Route' RETURN r.qualified_name, r.name, r.file_path", 0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load routes for %s: %w", project, err)
	}
	for _, row := range qr.Rows {
		if len(row) < 2 {
			continue
		}
		filePath := ""
		if len(row) >= 3 {
			filePath = row[2]
		}
		g.Routes = append(g.Routes, RouteNode{QualifiedName: row[0], Name: row[1], FilePath: filePath})
	}

	qr, err = c.QueryGraph(ctx, "MATCH (f)-[:HANDLES]->(r) RETURN r.qualified_name, f.qualified_name", 0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load route handlers for %s: %w", project, err)
	}
	for _, row := range qr.Rows {
		if len(row) < 2 || row[0] == "" || row[1] == "" {
			continue
		}
		g.Handlers[row[0]] = append(g.Handlers[row[0]], row[1])
	}

	qr, err = c.QueryGraph(ctx, "MATCH (f)-[:HTTP_CALLS]->(r) RETURN f.qualified_name, r.name, r.qualified_name", 0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load HTTP calls for %s: %w", project, err)
	}
	for _, row := range qr.Rows {
		if len(row) < 2 || row[0] == "" {
			continue
		}
		qn := ""
		if len(row) >= 3 {
			qn = row[2]
		}
		g.Outbound[row[0]] = append(g.Outbound[row[0]], OutboundRoute{Name: row[1], QualifiedName: qn})
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
