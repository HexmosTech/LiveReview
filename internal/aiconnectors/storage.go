package aiconnectors

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// ConnectorRecord represents a connector record in the database
type ConnectorRecord struct {
	ID            int64          `json:"id"`
	ProviderName  string         `json:"provider_name"` // Maps to provider_name in DB
	Provider      Provider       `json:"provider"`      // For internal use, derived from ProviderName
	ApiKey        string         `json:"api_key"`
	ConnectorName string         `json:"connector_name"` // Maps to connector_name in DB
	BaseURL       sql.NullString `json:"base_url"`       // Base URL for providers like Ollama
	SelectedModel sql.NullString `json:"selected_model"` // Selected model for the connector
	DisplayOrder  int            `json:"display_order"`
	OrgID         int64          `json:"org_id"` // Organization ID for multi-tenancy
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`

	// Additional fields for internal use (not stored in the table directly)
	Model         string `json:"-"`
	Configuration string `json:"-"`
	IsActive      bool   `json:"-"`
}

// Storage provides methods to store and retrieve connectors
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new storage instance
func NewStorage(db *sql.DB) *Storage {
	return &Storage{
		db: db,
	}
}

// CreateConnector creates a new connector in the database
func (s *Storage) CreateConnector(ctx context.Context, connector *ConnectorRecord) error {
	query := `
	INSERT INTO ai_connectors (
		provider_name, api_key, connector_name, base_url, selected_model, display_order, org_id,
		created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7,
		NOW(), NOW()
	) RETURNING id, created_at, updated_at
	`

	// Debug logging to see what display_order is being inserted
	log.Debug().
		Str("provider_name", connector.ProviderName).
		Str("connector_name", connector.ConnectorName).
		Int("display_order", connector.DisplayOrder).
		Msg("Inserting connector with display_order")

	// Convert string values to sql.NullString for database insertion
	var baseURL, selectedModel interface{}
	if connector.BaseURL.Valid && connector.BaseURL.String != "" {
		baseURL = connector.BaseURL.String
	} else {
		baseURL = nil
	}

	if connector.SelectedModel.Valid && connector.SelectedModel.String != "" {
		selectedModel = connector.SelectedModel.String
	} else {
		selectedModel = nil
	}

	err := s.db.QueryRowContext(
		ctx, query,
		connector.ProviderName, connector.ApiKey, connector.ConnectorName,
		baseURL, selectedModel, connector.DisplayOrder, connector.OrgID,
	).Scan(&connector.ID, &connector.CreatedAt, &connector.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create connector: %w", err)
	}

	// Set the Provider based on ProviderName
	connector.Provider = Provider(connector.ProviderName)

	return nil
}

// GetConnectorByID retrieves a connector by ID
func (s *Storage) GetConnectorByID(ctx context.Context, id int64) (*ConnectorRecord, error) {
	query := `
	SELECT id, provider_name, api_key, connector_name, base_url, selected_model, display_order, org_id,
	       created_at, updated_at
	FROM ai_connectors
	WHERE id = $1
	`

	var connector ConnectorRecord
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
		&connector.BaseURL, &connector.SelectedModel, &connector.DisplayOrder, &connector.OrgID,
		&connector.CreatedAt, &connector.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connector not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get connector: %w", err)
	}

	// Set the Provider based on ProviderName
	connector.Provider = Provider(connector.ProviderName)

	return &connector, nil
}

// GetConnectorsByProvider retrieves all connectors for a specific provider and organization
func (s *Storage) GetConnectorsByProvider(ctx context.Context, provider Provider, orgID int64) ([]*ConnectorRecord, error) {
	query := `
	SELECT id, provider_name, api_key, connector_name, base_url, selected_model, display_order, org_id,
	       created_at, updated_at
	FROM ai_connectors
	WHERE provider_name = $1 AND org_id = $2
	ORDER BY display_order ASC
	`

	rows, err := s.db.QueryContext(ctx, query, string(provider), orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get connectors by provider: %w", err)
	}
	defer rows.Close()

	var connectors []*ConnectorRecord
	for rows.Next() {
		var connector ConnectorRecord
		err := rows.Scan(
			&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
			&connector.BaseURL, &connector.SelectedModel, &connector.DisplayOrder, &connector.OrgID,
			&connector.CreatedAt, &connector.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan connector: %w", err)
		}

		// Set the Provider based on ProviderName
		connector.Provider = Provider(connector.ProviderName)

		connectors = append(connectors, &connector)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating connectors: %w", err)
	}

	return connectors, nil
}

// GetAllConnectors retrieves all connectors for an organization
func (s *Storage) GetAllConnectors(ctx context.Context, orgID int64) ([]*ConnectorRecord, error) {
	query := `
	SELECT id, provider_name, api_key, connector_name, base_url, selected_model, display_order, org_id,
	       created_at, updated_at
	FROM ai_connectors
	WHERE org_id = $1
	ORDER BY display_order ASC
	`

	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all connectors: %w", err)
	}
	defer rows.Close()

	var connectors []*ConnectorRecord
	for rows.Next() {
		var connector ConnectorRecord
		err := rows.Scan(
			&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
			&connector.BaseURL, &connector.SelectedModel, &connector.DisplayOrder, &connector.OrgID,
			&connector.CreatedAt, &connector.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan connector: %w", err)
		}

		// Set the Provider based on ProviderName
		connector.Provider = Provider(connector.ProviderName)

		connectors = append(connectors, &connector)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating connectors: %w", err)
	}

	return connectors, nil
}

// UpdateConnector updates a connector in the database
func (s *Storage) UpdateConnector(ctx context.Context, connector *ConnectorRecord) error {
	query := `
	UPDATE ai_connectors
	SET provider_name = $1, api_key = $2, connector_name = $3, base_url = $4, selected_model = $5, display_order = $6,
	    updated_at = NOW()
	WHERE id = $7 AND org_id = $8
	RETURNING updated_at
	`

	// Convert string values to interface{} for database update
	var baseURL, selectedModel interface{}
	if connector.BaseURL.Valid && connector.BaseURL.String != "" {
		baseURL = connector.BaseURL.String
	} else {
		baseURL = nil
	}

	if connector.SelectedModel.Valid && connector.SelectedModel.String != "" {
		selectedModel = connector.SelectedModel.String
	} else {
		selectedModel = nil
	}

	err := s.db.QueryRowContext(
		ctx, query,
		connector.ProviderName, connector.ApiKey, connector.ConnectorName,
		baseURL, selectedModel, connector.DisplayOrder,
		connector.ID, connector.OrgID,
	).Scan(&connector.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update connector: %w", err)
	}

	// Update the Provider based on ProviderName
	connector.Provider = Provider(connector.ProviderName)

	return nil
}

// DeleteConnector deletes a connector from the database
func (s *Storage) DeleteConnector(ctx context.Context, id int64, orgID int64) error {
	query := `DELETE FROM ai_connectors WHERE id = $1 AND org_id = $2`

	result, err := s.db.ExecContext(ctx, query, id, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete connector: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("connector not found: %d", id)
	}

	return nil
}

// GetConnectorOptions creates ConnectorOptions from a ConnectorRecord
func (r *ConnectorRecord) GetConnectorOptions() ConnectorOptions {
	options := ConnectorOptions{
		Provider: r.Provider,
		APIKey:   r.ApiKey,
		BaseURL:  r.BaseURL.String, // Extract string from sql.NullString
		ModelConfig: ModelConfig{
			Model: r.GetSelectedModel(), // Use helper method
		},
	}

	// Use default model if not specified
	if options.ModelConfig.Model == "" {
		options.ModelConfig.Model = GetDefaultModel(r.Provider)
	}

	// Set default values if not specified
	if options.ModelConfig.Temperature == 0 {
		options.ModelConfig.Temperature = 0.7
	}

	return options
}

// Helper methods to get string values from sql.NullString fields
func (r *ConnectorRecord) GetBaseURL() string {
	if r.BaseURL.Valid {
		return r.BaseURL.String
	}
	return ""
}

func (r *ConnectorRecord) GetSelectedModel() string {
	if r.SelectedModel.Valid {
		return r.SelectedModel.String
	}
	return ""
}

// We don't need the CreateConnectorTable and CreateConnectorIndexes methods since the table
// already exists through migrations.

// ToAPIResponse converts a ConnectorRecord to a format suitable for API responses
func (r *ConnectorRecord) ToAPIResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":              r.ID,
		"provider":        r.ProviderName,
		"name":            r.ConnectorName,
		"display_order":   r.DisplayOrder,
		"created_at":      r.CreatedAt,
		"updated_at":      r.UpdatedAt,
		"api_key_preview": maskAPIKey(r.ApiKey),
		"model":           r.Model,
		"is_active":       r.IsActive,
		"base_url":        r.GetBaseURL(),
		"selected_model":  r.GetSelectedModel(),
	}
}

// maskAPIKey returns a masked version of the API key for display purposes
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}

// GetMaxDisplayOrder returns the maximum display_order value in the database
func (s *Storage) GetMaxDisplayOrder(ctx context.Context) (int, error) {
	query := `SELECT COALESCE(MAX(display_order), 0) FROM ai_connectors`

	var maxOrder int
	err := s.db.QueryRowContext(ctx, query).Scan(&maxOrder)
	if err != nil {
		return 0, fmt.Errorf("failed to get max display order: %w", err)
	}

	if maxOrder == 0 {
		log.Debug().Msg("No existing connectors found, max display order is 0 (first connector will get order 1)")
	} else {
		log.Debug().
			Int("max_order", maxOrder).
			Msg("Retrieved max display order from database")
	}

	return maxOrder, nil
}

// UpdateDisplayOrders updates the display order for multiple connectors
func (s *Storage) UpdateDisplayOrders(ctx context.Context, updates []DisplayOrderUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `UPDATE ai_connectors SET display_order = $1, updated_at = NOW() WHERE id = $2`

	for _, update := range updates {
		// Convert string ID to int64
		connectorID, err := strconv.ParseInt(update.ID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid connector ID %s: %w", update.ID, err)
		}

		_, err = tx.ExecContext(ctx, query, update.DisplayOrder, connectorID)
		if err != nil {
			return fmt.Errorf("failed to update display order for connector %s: %w", update.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DisplayOrderUpdate represents a display order update for a connector
type DisplayOrderUpdate struct {
	ID           string `json:"id"`
	DisplayOrder int    `json:"display_order"`
}
