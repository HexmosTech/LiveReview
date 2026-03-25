package jobqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebhookStore centralizes jobqueue persistence operations.
type WebhookStore struct {
	pool         *pgxpool.Pool
	reverseProxy bool
	demoBaseURL  string
}

var ErrWebhookRegistryNotFound = errors.New("webhook registry not found")

func NewWebhookStore(pool *pgxpool.Pool, reverseProxy bool, demoBaseURL string) (*WebhookStore, error) {
	demoBaseURL = strings.TrimSpace(strings.TrimSuffix(demoBaseURL, "/"))
	if !reverseProxy && demoBaseURL == "" {
		return nil, fmt.Errorf("demo base URL is required when reverse proxy mode is disabled")
	}

	return &WebhookStore{pool: pool, reverseProxy: reverseProxy, demoBaseURL: demoBaseURL}, nil
}

func (s *WebhookStore) GetWebhookPublicEndpoint(ctx context.Context) (string, error) {
	if s.reverseProxy {
		var prodURL sql.NullString
		err := s.pool.QueryRow(ctx, `SELECT livereview_prod_url FROM instance_details ORDER BY id DESC LIMIT 1`).Scan(&prodURL)
		if err != nil {
			if err == pgx.ErrNoRows {
				return "", fmt.Errorf("production URL not set: please configure livereview_prod_url in settings before installing webhooks")
			}
			return "", fmt.Errorf("error querying production URL: %w", err)
		}
		if !prodURL.Valid || strings.TrimSpace(prodURL.String) == "" {
			return "", fmt.Errorf("production URL is empty: please configure livereview_prod_url in settings before installing webhooks")
		}
		return strings.TrimSuffix(strings.TrimSpace(prodURL.String), "/"), nil
	}

	return s.demoBaseURL, nil
}

func (s *WebhookStore) GetWebhookRegistryID(ctx context.Context, connectorID int, projectFullName string) (int, error) {
	var id int
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM webhook_registry
		WHERE integration_token_id = $1 AND project_full_name = $2
	`, connectorID, projectFullName).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrWebhookRegistryNotFound
		}
		return 0, err
	}
	return id, nil
}

type WebhookRegistryRecord struct {
	Provider           string
	ProviderProjectID  string
	ProjectName        string
	ProjectFullName    string
	WebhookID          string
	WebhookURL         string
	WebhookSecret      string
	WebhookName        string
	Events             string
	Status             string
	LastVerifiedAt     time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	IntegrationTokenID int
}

func (s *WebhookStore) InsertWebhookRegistry(ctx context.Context, rec WebhookRegistryRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_registry (
			provider,
			provider_project_id,
			project_name,
			project_full_name,
			webhook_id,
			webhook_url,
			webhook_secret,
			webhook_name,
			events,
			status,
			last_verified_at,
			created_at,
			updated_at,
			integration_token_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, rec.Provider, rec.ProviderProjectID, rec.ProjectName, rec.ProjectFullName, rec.WebhookID,
		rec.WebhookURL, rec.WebhookSecret, rec.WebhookName, rec.Events, rec.Status,
		rec.LastVerifiedAt, rec.CreatedAt, rec.UpdatedAt, rec.IntegrationTokenID)
	if err != nil {
		return fmt.Errorf("failed to insert webhook registry: %w", err)
	}
	return nil
}

type WebhookRegistryUpdate struct {
	WebhookID      string
	WebhookURL     string
	WebhookSecret  string
	WebhookName    string
	Events         string
	Status         string
	LastVerifiedAt time.Time
	UpdatedAt      time.Time
}

func (s *WebhookStore) UpdateWebhookRegistryByID(ctx context.Context, id int, upd WebhookRegistryUpdate) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE webhook_registry
		SET webhook_id = $1,
			webhook_url = $2,
			webhook_secret = $3,
			webhook_name = $4,
			events = $5,
			status = $6,
			last_verified_at = $7,
			updated_at = $8
		WHERE id = $9
	`, upd.WebhookID, upd.WebhookURL, upd.WebhookSecret, upd.WebhookName, upd.Events,
		upd.Status, upd.LastVerifiedAt, upd.UpdatedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update webhook registry: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrWebhookRegistryNotFound
	}
	return nil
}

func (s *WebhookStore) GetConnectorMetadata(ctx context.Context, connectorID int) ([]byte, error) {
	var metadata []byte
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(metadata, '{}') FROM integration_tokens WHERE id = $1`, connectorID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to get connector metadata: %w", err)
	}
	return metadata, nil
}
