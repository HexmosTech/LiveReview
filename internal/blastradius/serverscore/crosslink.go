package serverscore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/HexmosTech/git-lrc/blastradius/client"
	"github.com/livereview/internal/blastradius"
)

// crossEdgeTypes are codebase-memory-mcp's cross-repo edge labels (see
// docs/multi-repo-blast-radius-plan.md). query_graph's Cypher dialect has no
// type()/label-set matching in a WHERE clause (confirmed empirically against
// a live v0.12.1 binary), so each type is queried on its own rather than in
// one combined pattern.
var crossEdgeTypes = []string{
	"CROSS_HTTP_CALLS", "CROSS_ASYNC_CALLS", "CROSS_CHANNEL",
	"CROSS_GRPC_CALLS", "CROSS_GRAPHQL_CALLS", "CROSS_TRPC_CALLS",
}

// LinkRelatedProjects runs codebase-memory-mcp's cross-repo-intelligence pass
// over project and every entry in targetProjects (all must already be
// indexed - see Score), creating CROSS_* edges between Route/Channel nodes
// that match across them. repoDir must be the same directory project was
// indexed from (the engine re-derives repo_path on every index_repository
// call, it is not implied by the project name alone).
func LinkRelatedProjects(ctx context.Context, repoDir, project string, targetProjects []string) error {
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}

	args := struct {
		RepoPath       string   `json:"repo_path"`
		Name           string   `json:"name"`
		Mode           string   `json:"mode"`
		TargetProjects []string `json:"target_projects"`
	}{RepoPath: repoDir, Name: project, Mode: "cross-repo-intelligence", TargetProjects: targetProjects}
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "cbm-crosslink-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, binary, "cli", "index_repository", "--args-file", tmp.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("serverscore: cross-repo-intelligence: %w: %s", err, string(out))
	}
	return nil
}

// CrossRepoImpactsForSymbols queries every cross-repo edge leaving each of
// symbolQNs inside project, returning one blastradius.CrossRepoImpact per
// matched edge, keyed by the calling symbol's qualified name. symbolQNs with
// no matching edge are simply absent from the result, not an error.
func CrossRepoImpactsForSymbols(ctx context.Context, project string, symbolQNs []string) (map[string][]blastradius.CrossRepoImpact, error) {
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	c := &client.Client{Binary: binary, Project: project}

	wanted := make(map[string]bool, len(symbolQNs))
	for _, qn := range symbolQNs {
		wanted[qn] = true
	}

	result := make(map[string][]blastradius.CrossRepoImpact)
	for _, edgeType := range crossEdgeTypes {
		cypher := fmt.Sprintf(
			"MATCH (a)-[r:%s]->(b) RETURN a.qualified_name, b.name, b.qualified_name, r.target_project, r.confidence",
			edgeType)
		qr, err := c.QueryGraph(ctx, cypher, 0)
		if err != nil {
			return nil, fmt.Errorf("serverscore: query %s: %w", edgeType, err)
		}
		for _, row := range qr.Rows {
			if len(row) < 4 || !wanted[row[0]] {
				continue
			}
			impact := blastradius.CrossRepoImpact{
				RelatedRepo: row[3],
				CallSite:    row[2],
				EdgeType:    edgeType,
			}
			if len(row) >= 5 {
				if conf, err := strconv.ParseFloat(row[4], 64); err == nil {
					impact.Confidence = conf
				}
			}
			result[row[0]] = append(result[row[0]], impact)
		}
	}
	return result, nil
}
