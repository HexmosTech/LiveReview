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

func (s *UserStore) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *UserStore) TxQueryRow(tx *sql.Tx, query string, args ...interface{}) *sql.Row {
	return tx.QueryRow(query, args...)
}

func (s *UserStore) TxExec(tx *sql.Tx, query string, args ...interface{}) (sql.Result, error) {
	return tx.Exec(query, args...)
}

// LockOrgPlanCode locks the org row for the rest of tx (so concurrent invites into the same org
// serialize) and returns its current plan code ("" when the org has no billing state yet).
func (s *UserStore) LockOrgPlanCode(tx *sql.Tx, orgID int64) (string, error) {
	var code sql.NullString
	err := tx.QueryRow(`
		SELECT obs.current_plan_code
		FROM orgs o
		LEFT JOIN org_billing_state obs ON obs.org_id = o.id
		WHERE o.id = $1
		FOR UPDATE OF o
	`, orgID).Scan(&code)
	return code.String, err
}

// CountActiveOrgMembers counts the org's members whose user account is active.
func (s *UserStore) CountActiveOrgMembers(tx *sql.Tx, orgID int64) (int, error) {
	var n int
	err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		WHERE ur.org_id = $1 AND u.is_active
	`, orgID).Scan(&n)
	return n, err
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
