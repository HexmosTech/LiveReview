package dashboard

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// CacheStore is the read/write layer for dashboard_cache; shaping/merging json_raw and final_json is the caller's (DashboardManager's) responsibility.
type CacheStore struct {
	db *sql.DB
}

func NewCacheStore(db *sql.DB) *CacheStore {
	return &CacheStore{db: db}
}

// CacheRow is one org's row in dashboard_cache.
type CacheRow struct {
	OrgID     int64
	JSONRaw   []byte
	FinalJSON []byte
	UpdatedAt time.Time
}

// ListOrgIDs returns every org id, used by the background refresh to know which orgs to compute data for.
func (s *CacheStore) ListOrgIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM orgs`)
	if err != nil {
		return nil, fmt.Errorf("list org ids: %w", err)
	}
	defer rows.Close()

	var orgIDs []int64
	for rows.Next() {
		var orgID int64
		if err := rows.Scan(&orgID); err != nil {
			return nil, fmt.Errorf("list org ids: scan: %w", err)
		}
		orgIDs = append(orgIDs, orgID)
	}
	return orgIDs, rows.Err()
}

// GetFinalJSON returns the precomputed final_json for one org, or (nil, nil) if no row exists yet.
func (s *CacheStore) GetFinalJSON(ctx context.Context, orgID int64) (json.RawMessage, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT final_json FROM dashboard_cache WHERE org_id = $1`, orgID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get final_json for org %d: %w", orgID, err)
	}
	return raw, nil
}

// GetBatch returns existing json_raw/final_json for every org in orgIDs in one round trip; missing orgs are simply absent from the map.
func (s *CacheStore) GetBatch(ctx context.Context, orgIDs []int64) (map[int64]CacheRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT org_id, json_raw, final_json, updated_at
		FROM dashboard_cache
		WHERE org_id = ANY($1)
	`, pq.Array(orgIDs))
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]CacheRow, len(orgIDs))
	for rows.Next() {
		var row CacheRow
		if err := rows.Scan(&row.OrgID, &row.JSONRaw, &row.FinalJSON, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("get batch: scan: %w", err)
		}
		result[row.OrgID] = row
	}
	return result, rows.Err()
}

// UpsertBatch writes every row in one round trip via INSERT ... FROM UNNEST ... ON CONFLICT, never one write per org.
func (s *CacheStore) UpsertBatch(ctx context.Context, rows []CacheRow) error {
	if len(rows) == 0 {
		return nil
	}

	orgIDs := make([]int64, len(rows))
	jsonRaws := make([]string, len(rows))
	finalJSONs := make([]string, len(rows))
	updatedAts := make([]time.Time, len(rows))
	for i, r := range rows {
		orgIDs[i] = r.OrgID
		jsonRaws[i] = string(r.JSONRaw)
		finalJSONs[i] = string(r.FinalJSON)
		updatedAts[i] = r.UpdatedAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO dashboard_cache (org_id, json_raw, final_json, updated_at)
		SELECT org_id, json_raw::jsonb, final_json::jsonb, updated_at
		FROM UNNEST($1::bigint[], $2::text[], $3::text[], $4::timestamptz[]) AS t(org_id, json_raw, final_json, updated_at)
		ON CONFLICT (org_id) DO UPDATE
		SET json_raw = EXCLUDED.json_raw,
		    final_json = EXCLUDED.final_json,
		    updated_at = EXCLUDED.updated_at
	`, pq.Array(orgIDs), pq.Array(jsonRaws), pq.Array(finalJSONs), pq.Array(updatedAts))
	if err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}
	return nil
}
