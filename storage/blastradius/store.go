// Package blastradius is the Postgres-backed store for per-hunk blast
// radius scores derived from git-lrc's S3 artifact - see
// internal/blastradius for the pure scoring logic this package persists,
// and docs/blast-radius-backend-port-plan.md for the full design. Every
// method takes orgID explicitly (denormalized onto blast_radius_hunks, not
// just reachable via review_id -> reviews.org_id) per AGENTS.md's "Direct
// Context Filtering" rule.
package blastradius

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/livereview/internal/blastradius"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// StoredHunk is one persisted row, as read back for GetForReview.
type StoredHunk struct {
	Combined float64
	Tier     string
	MathMode blastradius.MathMode
}

// HunkKey identifies a hunk within a review the same way
// ui/src/lib/blastRadius.ts's hunkBlastKey() joins a diff-viewer hunk to its
// blast-radius report entry.
func HunkKey(filePath string, newStart, newLines int) string {
	return fmt.Sprintf("%s:%d:%d", filePath, newStart, newLines)
}

// ReplaceHunksForReview computes MathMode+Tier for every hunk (via
// internal/blastradius.ComputeMathMode/Tier) and atomically replaces
// reviewID's rows. A delete-then-insert, not an upsert: a re-uploaded
// artifact with fewer hunks than a previous upload (a rebased/amended diff)
// must not leave the vanished hunks behind as orphan rows - those would
// silently inflate any COUNT/MAX a caller (Livi included) runs over the
// table. The UNIQUE constraint still guards against duplicates within one
// call.
func (s *Store) ReplaceHunksForReview(ctx context.Context, orgID, reviewID int64, hunks []blastradius.HunkReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM blast_radius_hunks WHERE review_id = $1 AND org_id = $2
	`, reviewID, orgID); err != nil {
		return fmt.Errorf("delete existing blast_radius_hunks: %w", err)
	}

	for _, h := range hunks {
		mathMode := blastradius.ComputeMathMode(h)
		tier := blastradius.Tier(h.Combined)
		mathModeJSON, err := json.Marshal(mathMode)
		if err != nil {
			return fmt.Errorf("marshal math mode for %s:%d: %w", h.FilePath, h.NewStart, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO blast_radius_hunks (review_id, org_id, file_path, new_start, new_lines, combined, tier, math_mode)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, reviewID, orgID, h.FilePath, h.NewStart, h.NewLines, h.Combined, tier, mathModeJSON); err != nil {
			return fmt.Errorf("insert blast_radius_hunks for %s:%d: %w", h.FilePath, h.NewStart, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit blast_radius_hunks replace: %w", err)
	}
	return nil
}

// GetForReview returns every stored hunk for reviewID, keyed by HunkKey
// (the same join key ui/src/lib/blastRadius.ts's hunkBlastKey() uses) so a
// future caller can match rows back onto a freshly-fetched S3 report by
// (FilePath, NewStart, NewLines). No production caller today - the diff
// viewer's read path (GetDiffReviewArtifact) was deliberately left
// untouched, see docs/blast-radius-backend-port-plan.md's Status section.
func (s *Store) GetForReview(ctx context.Context, orgID, reviewID int64) (map[string]StoredHunk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT file_path, new_start, new_lines, combined, tier, math_mode
		FROM blast_radius_hunks
		WHERE review_id = $1 AND org_id = $2
	`, reviewID, orgID)
	if err != nil {
		return nil, fmt.Errorf("select blast_radius_hunks: %w", err)
	}
	defer rows.Close()

	out := make(map[string]StoredHunk)
	for rows.Next() {
		var filePath, tier string
		var newStart, newLines int
		var combined float64
		var mathModeJSON []byte
		if err := rows.Scan(&filePath, &newStart, &newLines, &combined, &tier, &mathModeJSON); err != nil {
			return nil, fmt.Errorf("scan blast_radius_hunks: %w", err)
		}
		var mathMode blastradius.MathMode
		if err := json.Unmarshal(mathModeJSON, &mathMode); err != nil {
			return nil, fmt.Errorf("unmarshal math_mode for %s:%d: %w", filePath, newStart, err)
		}
		out[HunkKey(filePath, newStart, newLines)] = StoredHunk{
			Combined: combined,
			Tier:     tier,
			MathMode: mathMode,
		}
	}
	return out, rows.Err()
}
