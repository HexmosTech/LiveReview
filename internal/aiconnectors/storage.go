package aiconnectors

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ConnectorRecord represents a connector record in the database
type ConnectorRecord struct {
	ID            int64     `json:"id"`
	ProviderName  string    `json:"provider_name"` // Maps to provider_name in DB
	Provider      Provider  `json:"provider"`      // For internal use, derived from ProviderName
	ApiKey        string    `json:"api_key"`
	ConnectorName string    `json:"connector_name"` // Maps to connector_name in DB
	DisplayOrder  int       `json:"display_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Additional fields stored in metadata JSON (not in the table directly)
	BaseURL       string `json:"-"`
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
		provider_name, api_key, connector_name, display_order,
		created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, 
		NOW(), NOW()
	) RETURNING id, created_at, updated_at
	`

	err := s.db.QueryRowContext(
		ctx, query,
		connector.ProviderName, connector.ApiKey, connector.ConnectorName, connector.DisplayOrder,
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
	SELECT id, provider_name, api_key, connector_name, display_order, 
	       created_at, updated_at
	FROM ai_connectors
	WHERE id = $1
	`

	var connector ConnectorRecord
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
		&connector.DisplayOrder, &connector.CreatedAt, &connector.UpdatedAt,
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

// GetConnectorsByProvider retrieves all connectors for a specific provider
func (s *Storage) GetConnectorsByProvider(ctx context.Context, provider Provider) ([]*ConnectorRecord, error) {
	query := `
	SELECT id, provider_name, api_key, connector_name, display_order, 
	       created_at, updated_at
	FROM ai_connectors
	WHERE provider_name = $1
	ORDER BY display_order ASC
	`

	rows, err := s.db.QueryContext(ctx, query, string(provider))
	if err != nil {
		return nil, fmt.Errorf("failed to get connectors by provider: %w", err)
	}
	defer rows.Close()

	var connectors []*ConnectorRecord
	for rows.Next() {
		var connector ConnectorRecord
		err := rows.Scan(
			&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
			&connector.DisplayOrder, &connector.CreatedAt, &connector.UpdatedAt,
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

// GetAllConnectors retrieves all connectors
func (s *Storage) GetAllConnectors(ctx context.Context) ([]*ConnectorRecord, error) {
	query := `
	SELECT id, provider_name, api_key, connector_name, display_order, 
	       created_at, updated_at
	FROM ai_connectors
	ORDER BY provider_name, display_order ASC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all connectors: %w", err)
	}
	defer rows.Close()

	var connectors []*ConnectorRecord
	for rows.Next() {
		var connector ConnectorRecord
		err := rows.Scan(
			&connector.ID, &connector.ProviderName, &connector.ApiKey, &connector.ConnectorName,
			&connector.DisplayOrder, &connector.CreatedAt, &connector.UpdatedAt,
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
	SET provider_name = $1, api_key = $2, connector_name = $3, display_order = $4,
	    updated_at = NOW()
	WHERE id = $5
	RETURNING updated_at
	`

	err := s.db.QueryRowContext(
		ctx, query,
		connector.ProviderName, connector.ApiKey, connector.ConnectorName, connector.DisplayOrder,
		connector.ID,
	).Scan(&connector.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update connector: %w", err)
	}

	// Update the Provider based on ProviderName
	connector.Provider = Provider(connector.ProviderName)

	return nil
}

// DeleteConnector deletes a connector from the database
func (s *Storage) DeleteConnector(ctx context.Context, id int64) error {
	query := `DELETE FROM ai_connectors WHERE id = $1`

	result, err := s.db.ExecContext(ctx, query, id)
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
		BaseURL:  r.BaseURL,
		ModelConfig: ModelConfig{
			Model: r.Model,
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
	if options.ModelConfig.MaxTokens == 0 {
		options.ModelConfig.MaxTokens = 2048
	}

	return options
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
	}
}

// maskAPIKey returns a masked version of the API key for display purposes
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}
