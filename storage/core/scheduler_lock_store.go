package core

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

const dashboardRefreshLeaderLockKey int64 = 821731

type SchedulerLockStore struct {
	db *sql.DB

	mu       sync.Mutex
	lockConn *sql.Conn
}

func NewSchedulerLockStore(db *sql.DB) *SchedulerLockStore {
	return &SchedulerLockStore{db: db}
}

func (s *SchedulerLockStore) TryAcquireDashboardRefreshLeaderLock(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockConn != nil {
		if pingErr := s.lockConn.PingContext(ctx); pingErr == nil {
			return true, nil
		}
		if closeErr := s.lockConn.Close(); closeErr != nil {
			return false, fmt.Errorf("existing dashboard lock connection is unhealthy and close failed: %w", closeErr)
		}
		s.lockConn = nil
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return false, err
	}

	var acquired bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, dashboardRefreshLeaderLockKey).Scan(&acquired)
	if err != nil {
		_ = conn.Close()
		return false, err
	}

	if !acquired {
		_ = conn.Close()
		return false, nil
	}

	s.lockConn = conn
	return true, nil
}

func (s *SchedulerLockStore) ReleaseDashboardRefreshLeaderLock(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lockConn == nil {
		return nil
	}

	conn := s.lockConn
	s.lockConn = nil

	var unlocked bool
	unlockErr := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, dashboardRefreshLeaderLockKey).Scan(&unlocked)
	closeErr := conn.Close()

	if unlockErr != nil {
		return unlockErr
	}
	if !unlocked {
		if closeErr != nil {
			return fmt.Errorf("dashboard refresh leader lock was not released and connection close failed: %w", closeErr)
		}
		return fmt.Errorf("dashboard refresh leader lock was not released")
	}
	if closeErr != nil {
		return closeErr
	}

	return nil
}
