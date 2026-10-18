package policyengine

import (
	"context"
	"database/sql"
	"testing"
)

// If the approval count can't be read, the cap is skipped and approvals go on.
// This pins that current fail-open behavior; flip the assertion if it changes.
func TestDailyCapQueryErrorFailsOpen(t *testing.T) {
	db, err := sql.Open("postgres", "")
	if err != nil {
		t.Fatal(err)
	}
	db.Close() // every query now errors with "sql: database is closed"

	limit := 1
	e := &Engine{db: db}
	capped, reason := e.dailyCapExceeded(context.Background(), 1, &Policy{DailyApproveCap: &limit})
	if capped || reason != "" {
		t.Fatalf("query error: got capped=%v reason=%q, want capped=false reason=\"\"", capped, reason)
	}
}

// No cap set means unlimited, so the database is never asked.
func TestDailyCapNilIsUnlimited(t *testing.T) {
	e := &Engine{db: nil}
	if capped, _ := e.dailyCapExceeded(context.Background(), 1, &Policy{}); capped {
		t.Fatal("nil cap should never block")
	}
}
