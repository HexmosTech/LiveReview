package users

import "database/sql"

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) QueryRow(query string, args ...interface{}) *sql.Row {
	return s.db.QueryRow(query, args...)
}

func (s *UserStore) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *UserStore) TxQueryRow(tx *sql.Tx, query string, args ...interface{}) *sql.Row {
	return tx.QueryRow(query, args...)
}

func (s *UserStore) TxExec(tx *sql.Tx, query string, args ...interface{}) (sql.Result, error) {
	return tx.Exec(query, args...)
}

func (s *UserStore) WithTx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}
