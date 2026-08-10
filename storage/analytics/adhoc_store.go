// Package analytics executes the org-scoped SQL that internal/livisql produces
// for Livi's analytics answers.
//
// Everything here runs inside a read-only transaction, so writes are refused by
// Postgres itself (SQLSTATE 25006) rather than by our own inspection of the
// statement. The guard in internal/livisql is the first line; this is the one
// that holds even if the guard has a bug.
package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// defaultStatementTimeoutMS bounds a single query. Analytics aggregates over
	// an indexed (org_id, created_at) should be far under this; anything that is
	// not is a query we would rather fail than let occupy a pool connection.
	defaultStatementTimeoutMS = 8000

	// idleInTxTimeoutMS stops a wedged transaction from holding one of the 25
	// pool connections open indefinitely.
	idleInTxTimeoutMS = 15000
)

// ErrTooManyColumns and friends are returned to the caller so the pipeline can
// turn them into a retry hint for the model rather than a user-facing error.
var (
	ErrDuplicateColumns = errors.New("query returns duplicate or unnamed columns")
	ErrCountShape       = errors.New("count query did not return a single number")
)

// ResultSet is a query result in the shape the chart and CSV builders need:
// column order preserved separately from the rows, because Go maps are
// unordered and CSV headers must match the SELECT list.
type ResultSet struct {
	Columns   []string
	Rows      []map[string]any
	Truncated bool
}

type AdHocStore struct {
	db                 *sql.DB
	statementTimeoutMS int
}

func NewAdHocStore(db *sql.DB) *AdHocStore {
	return &AdHocStore{db: db, statementTimeoutMS: defaultStatementTimeoutMS}
}

// WithStatementTimeout overrides the per-query timeout, in milliseconds.
func (s *AdHocStore) WithStatementTimeout(ms int) *AdHocStore {
	if ms <= 0 {
		return s
	}
	clone := *s
	clone.statementTimeoutMS = ms
	return &clone
}

// Count runs a rewritten count query and returns its single numeric result.
// rewritten must already have come from livisql.Guard.Rewrite, which is what
// binds orgID to $1.
func (s *AdHocStore) Count(ctx context.Context, orgID int64, rewritten string) (int64, error) {
	rs, err := s.Query(ctx, orgID, rewritten, 2)
	if err != nil {
		return 0, err
	}
	if len(rs.Rows) != 1 || len(rs.Columns) != 1 {
		return 0, fmt.Errorf("%w: got %d rows and %d columns", ErrCountShape, len(rs.Rows), len(rs.Columns))
	}
	switch v := rs.Rows[0][rs.Columns[0]].(type) {
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("%w: got %T", ErrCountShape, v)
	}
}

// Query runs a rewritten query and materializes up to maxRows rows. It reads one
// row beyond the limit purely to set Truncated, so the caller can tell "exactly
// at the limit" apart from "more than the limit" — the difference between a
// complete chart and a misleading one.
func (s *AdHocStore) Query(ctx context.Context, orgID int64, rewritten string, maxRows int) (*ResultSet, error) {
	if maxRows <= 0 {
		return nil, errors.New("maxRows must be positive")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin read-only tx: %w", err)
	}
	// Always roll back: nothing here should ever commit, and an explicit
	// rollback releases the connection immediately on every path.
	defer func() { _ = tx.Rollback() }()

	// Timeouts are formatted from ints, never from anything the model produced.
	for _, stmt := range []string{
		fmt.Sprintf("SET LOCAL statement_timeout = %d", s.statementTimeoutMS),
		fmt.Sprintf("SET LOCAL idle_in_transaction_session_timeout = %d", idleInTxTimeoutMS),
		// An empty search_path means any unqualified relation that is not one of
		// the guard's shadow CTEs fails to resolve at all, instead of quietly
		// binding to the real table. The shadow bodies are public-qualified, so
		// they are unaffected. pg_catalog stays implicitly searchable, so
		// operators and builtins still work.
		"SET LOCAL search_path = ''",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("apply session guard %q: %w", stmt, err)
		}
	}

	rows, err := tx.QueryContext(ctx, rewritten, orgID)
	if err != nil {
		return nil, fmt.Errorf("execute analytics query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("read column types: %w", err)
	}
	if err := checkColumns(cols); err != nil {
		return nil, err
	}

	rs := &ResultSet{Columns: cols, Rows: make([]map[string]any, 0, min(maxRows, 256))}

	scan := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range scan {
		ptrs[i] = &scan[i]
	}

	for rows.Next() {
		if len(rs.Rows) >= maxRows {
			rs.Truncated = true
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = coerce(scan[i], colTypes[i])
		}
		rs.Rows = append(rs.Rows, row)
	}
	// rows.Err surfaces failures that happen mid-iteration, including the
	// statement timeout firing partway through a large result.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return rs, nil
}

// checkColumns rejects results the row maps cannot represent faithfully:
// duplicates would silently overwrite each other, and an unnamed column has no
// key at all. Both are the model's fault and both are fixable by aliasing.
func checkColumns(cols []string) error {
	seen := make(map[string]bool, len(cols))
	for _, c := range cols {
		if c == "" {
			return fmt.Errorf("%w: an output column has no name", ErrDuplicateColumns)
		}
		if seen[c] {
			return fmt.Errorf("%w: column %q appears more than once", ErrDuplicateColumns, c)
		}
		seen[c] = true
	}
	return nil
}

// Ping verifies the read-only path works at all. Used at boot so a
// misconfigured database surfaces before a user asks a question.
func (s *AdHocStore) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var one int
	return tx.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}
