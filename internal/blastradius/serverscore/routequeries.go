package serverscore

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/HexmosTech/git-lrc/blastradius/client"
)

// HTTPCallRouteNamesForSymbols is the lighter, direct-only sibling of
// LoadRepoGraph: one query for the touched symbols' outbound routes. It is
// used as a fallback when the full graph read fails, so a HANDLES/route scan
// problem cannot suppress version-aliased direct impacts.
func HTTPCallRouteNamesForSymbols(ctx context.Context, project string, symbolQNs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(symbolQNs) == 0 {
		return out, nil
	}
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	c := &client.Client{Binary: binary, Project: project}
	cypher := fmt.Sprintf("MATCH (a)-[:HTTP_CALLS]->(b) WHERE a.qualified_name IN %s RETURN a.qualified_name, b.name", client.CypherStringList(symbolQNs))
	qr, err := c.QueryGraph(ctx, cypher, 0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load HTTP calls for %s: %w", project, err)
	}
	for _, row := range qr.Rows {
		if len(row) < 2 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		out[row[0]] = append(out[row[0]], row[1])
	}
	return out, nil
}

// DeclaredRoutes returns only source-declared routes for a project. Used by
// the direct-only fallback when a full LoadRepoGraph read is unavailable.
func DeclaredRoutes(ctx context.Context, project string) ([]RouteNode, error) {
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	c := &client.Client{Binary: binary, Project: project}
	qr, err := c.QueryGraph(ctx, "MATCH (r) WHERE r.label='Route' AND r.file_path <> '' RETURN r.qualified_name, r.name, r.file_path", 0)
	if err != nil {
		return nil, fmt.Errorf("serverscore: load declared routes for %s: %w", project, err)
	}
	var out []RouteNode
	for _, row := range qr.Rows {
		if len(row) < 2 {
			continue
		}
		filePath := ""
		if len(row) >= 3 {
			filePath = row[2]
		}
		if filePath == "" {
			continue
		}
		out = append(out, RouteNode{QualifiedName: row[0], Name: row[1], FilePath: filePath})
	}
	return out, nil
}
