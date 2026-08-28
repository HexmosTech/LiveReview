package core

import (
	"context"
	"fmt"
)

// eventCompactionLeaderLockKey is a unique PostgreSQL advisory lock key for the
// event compaction manager. Must differ from dashboardRefreshLeaderLockKey (821731).
const eventCompactionLeaderLockKey int64 = 849204821

// TryAcquireEventCompactionLeaderLock is retained for future use if compaction is
// ever moved to a worker process (multiple instances). Currently unused because
// compaction runs in the backend process (single instance).
func (s *SchedulerLockStore) TryAcquireEventCompactionLeaderLock(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("event compaction lock: get conn: %w", err)
	}

	var acquired bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, eventCompactionLeaderLockKey).Scan(&acquired)
	if err != nil {
		_ = conn.Close()
		return false, fmt.Errorf("event compaction lock: pg_try_advisory_lock: %w", err)
	}

	if !acquired {
		_ = conn.Close()
		return false, nil
	}

	// Hold and immediately release — retained only as a future hook for worker deployments.
	_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, eventCompactionLeaderLockKey)
	_ = conn.Close()
	return true, nil
}

