package aiconnectors

import (
	"context"
	"database/sql"
	"fmt"
)

type DisplayOrderUpdate struct {
	ConnectorID  int64
	DisplayOrder int
}

type ConnectorStore struct {
	db *sql.DB
}

func NewConnectorStore(db *sql.DB) *ConnectorStore {
	return &ConnectorStore{db: db}
}

func (s *ConnectorStore) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

func (s *ConnectorStore) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

func (s *ConnectorStore) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func (s *ConnectorStore) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, opts)
}

func (s *ConnectorStore) UpdateDisplayOrders(ctx context.Context, orgID int64, updates []DisplayOrderUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `UPDATE ai_connectors SET display_order = $1, updated_at = NOW() WHERE id = $2 AND org_id = $3`

	for _, update := range updates {
		result, err := tx.ExecContext(ctx, query, update.DisplayOrder, update.ConnectorID, orgID)
		if err != nil {
			return fmt.Errorf("failed to update display order for connector %d: %w", update.ConnectorID, err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get affected rows for connector %d: %w", update.ConnectorID, err)
		}

		if affected == 0 {
			return fmt.Errorf("connector %d not found for organization", update.ConnectorID)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
