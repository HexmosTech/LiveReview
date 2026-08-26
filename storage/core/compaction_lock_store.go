package core

import (
	"context"
	"fmt"
)

// eventCompactionLeaderLockKey is a unique PostgreSQL advisory lock key for the
// event compaction manager. Must differ from dashboardRefreshLeaderLockKey (821731).
const eventCompactionLeaderLockKey int64 = 849204821

// TryAcquireEventCompactionLeaderLock tries to acquire the pg advisory lock for
// the event compaction cycle. Returns true if this instance became the leader.
// Uses the same underlying connection-based advisory lock pattern as the dashboard.
func (s *SchedulerLockStore) TryAcquireEventCompactionLeaderLock(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("event compaction lock: get conn: %w", err)
	}
	defer conn.Close()

	var acquired bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, eventCompactionLeaderLockKey).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("event compaction lock: pg_try_advisory_lock: %w", err)
	}
	if acquired {
		// Immediately release — we only use the lock to elect a single winner
		// per cycle, not to hold it for the full cycle duration.
		_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, eventCompactionLeaderLockKey)
	}
	return acquired, nil
}
