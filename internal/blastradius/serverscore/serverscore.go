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
)

const engineBinary = "codebase-memory-mcp"

// Score refreshes the graph index for repoDir (incremental after the first run) and scores
// diff against it. Indexes live under <root>/index; project is the index's name.
func Score(ctx context.Context, root, repoDir string, diff []byte) (report *blastradius.Report, project string, err error) {
	binary, err := exec.LookPath(engineBinary)
	if err != nil {
		return nil, "", fmt.Errorf("serverscore: %s not found on PATH: %w", engineBinary, err)
	}
	if err := useIndexDir(root); err != nil {
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

// useIndexDir points the engine's index store at the volume, not ~/.cache.
// ponytail: process-wide env (the engine client has no per-call env); fine while only this package runs the engine.
func useIndexDir(root string) error {
	dir, err := filepath.Abs(filepath.Join(root, "index"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Setenv("CBM_CACHE_DIR", dir)
}
