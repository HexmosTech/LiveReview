// Package serverscore indexes a repocache checkout with codebase-memory-mcp and
// scores a PR diff with git-lrc's blast-radius engine (same engine the CLI uses).
package serverscore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/HexmosTech/git-lrc/blastradius"
	"github.com/HexmosTech/git-lrc/blastradius/client"
	"github.com/livereview/internal/blastradius/repocache"
)

const engineBinary = "codebase-memory-mcp"

// indexDir keeps graph indexes on the lrdata volume next to the repo cache, not in ~/.cache.
var indexDir = filepath.Join(repocache.DefaultRoot, "index")

// The engine client has no per-call env, so CBM_CACHE_DIR is set once here, at program
// start before any job runs, and never changed afterwards.
func init() {
	if abs, err := filepath.Abs(indexDir); err == nil {
		indexDir = abs
	}
	_ = os.Setenv("CBM_CACHE_DIR", indexDir)
}

// Score refreshes the graph index for repoDir (incremental after the first run) and scores
// diff against it; project is the index's name.
func Score(ctx context.Context, repoDir string, diff []byte) (report *blastradius.Report, project string, err error) {
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, "", fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, "", err
	}

	project, err = (&client.Client{Binary: binary}).IndexRepository(ctx, repoDir, "fast")
	if err != nil {
		return nil, "", err
	}
	opts := blastradius.DefaultOptions()
	opts.Binary = binary
	report, err = blastradius.ScoreDiff(ctx, diff, project, opts)
	return report, project, err
}
