package reviews

import "database/sql"

type ReviewStore struct {
	db *sql.DB
}

func NewReviewStore(db *sql.DB) *ReviewStore {
	return &ReviewStore{db: db}
}

func (s *ReviewStore) QueryRow(query string, args ...interface{}) *sql.Row {
	return s.db.QueryRow(query, args...)
}

func (s *ReviewStore) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *ReviewStore) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}
