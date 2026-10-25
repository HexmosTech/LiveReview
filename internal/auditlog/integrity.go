package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ChainReport is the result of verifying one organization's hash chain.
type ChainReport struct {
	OrgID             int64      `json:"org_id"`
	Rows              int64      `json:"rows"`
	Verified          int64      `json:"verified"`
	FirstBrokenSeq    *int64     `json:"first_broken_seq,omitempty"`
	FirstBrokenAt     *time.Time `json:"first_broken_at,omitempty"`
	FirstBrokenReason string     `json:"first_broken_reason,omitempty"`
	ChainStart        *time.Time `json:"chain_start,omitempty"`
	ChainEnd          *time.Time `json:"chain_end,omitempty"`
	Intact            bool       `json:"intact"`
}

// VerifyChain recomputes the per-org hash chain and reports the first break.
// It is bounded by limit so a huge org cannot pin a request indefinitely.
func VerifyChain(ctx context.Context, db *sql.DB, orgID int64, fromSeq int64, limit int) (*ChainReport, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("auditlog: org_id is required")
	}
	if limit <= 0 || limit > 200000 {
		limit = 50000
	}
	if fromSeq < 1 {
		fromSeq = 1
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+eventColumns+`
		FROM audit_events
		WHERE org_id = $1 AND seq IS NOT NULL AND seq >= $2
		ORDER BY seq ASC
		LIMIT $3`, orgID, fromSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("auditlog: verify query: %w", err)
	}
	defer rows.Close()

	report := &ChainReport{OrgID: orgID, Intact: true}
	prev := ""
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		report.Rows++
		details, err := json.Marshal(ev.Details)
		if err != nil {
			return nil, err
		}
		expected, err := HashEvent(*ev.Seq, prev, ev, details)
		if err != nil {
			return nil, err
		}
		if ev.PrevHash != prev {
			report.Intact = false
			report.FirstBrokenSeq = ev.Seq
			report.FirstBrokenAt = &ev.OccurredAt
			report.FirstBrokenReason = "prev_hash does not match the previous row"
			break
		}
		if ev.Hash != expected {
			report.Intact = false
			report.FirstBrokenSeq = ev.Seq
			report.FirstBrokenAt = &ev.OccurredAt
			report.FirstBrokenReason = "hash does not match the row contents"
			break
		}
		if report.ChainStart == nil {
			report.ChainStart = &ev.OccurredAt
		}
		report.ChainEnd = &ev.OccurredAt
		report.Verified++
		prev = ev.Hash
		if report.FirstBrokenSeq != nil {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auditlog: verify rows: %w", err)
	}
	return report, nil
}

// LatestChainTip returns the highest seq/hash for an org (used by the daily
// anchor writer).
func LatestChainTip(ctx context.Context, db *sql.DB, orgID int64) (int64, string, time.Time, error) {
	var seq sql.NullInt64
	var hash sql.NullString
	var at sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT seq, hash, occurred_at FROM audit_events
		WHERE org_id = $1 AND seq IS NOT NULL
		ORDER BY seq DESC LIMIT 1`, orgID).Scan(&seq, &hash, &at)
	if err == sql.ErrNoRows {
		return 0, "", time.Time{}, nil
	}
	if err != nil {
		return 0, "", time.Time{}, err
	}
	return seq.Int64, hash.String, at.Time, nil
}
