package pushnotify

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	_ "github.com/lib/pq"
)

// testDB mirrors internal/jobqueue/pr_policy_claim_sql_test.go's testDB:
// runs the real claim-once UPDATE against a real Postgres DB rather than
// mocking it, since the whole point of ClaimFirstFinding is that the SQL
// itself (not application code) resolves the race.
func testDB(t *testing.T) *sql.DB {
	if testing.Short() {
		t.Skip("skipping database integration test")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func seedOrgAndReview(t *testing.T, db *sql.DB, userEmail string) (orgID, reviewID int64) {
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('pushnotify-test-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID) })

	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, user_email, metadata)
		VALUES ('test/repo', 'in_progress', $1, $2, '{}'::jsonb) RETURNING id`, orgID, userEmail).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })
	return orgID, reviewID
}

func TestClaimFirstFindingOnlyOneWinnerUnderConcurrency(t *testing.T) {
	db := testDB(t)
	_, reviewID := seedOrgAndReview(t, db, "")

	const n = 20
	var wg sync.WaitGroup
	wins := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			won, err := ClaimFirstFinding(context.Background(), db, reviewID)
			if err != nil {
				t.Errorf("ClaimFirstFinding: %v", err)
				return
			}
			wins[i] = won
		}(i)
	}
	wg.Wait()

	winCount := 0
	for _, w := range wins {
		if w {
			winCount++
		}
	}
	if winCount != 1 {
		t.Fatalf("expected exactly 1 winner out of %d concurrent claims, got %d", n, winCount)
	}
}

func TestClaimFirstFindingSecondCallLoses(t *testing.T) {
	db := testDB(t)
	_, reviewID := seedOrgAndReview(t, db, "")

	first, err := ClaimFirstFinding(context.Background(), db, reviewID)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !first {
		t.Fatalf("expected first claim to win")
	}

	second, err := ClaimFirstFinding(context.Background(), db, reviewID)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if second {
		t.Fatalf("expected second claim to lose")
	}
}

func TestDispatchSkipsReviewsWithNoIdentifiableUser(t *testing.T) {
	db := testDB(t)
	// Scheduled/webhook-triggered reviews have an empty user_email — the
	// documented "no one to notify" case, not an error.
	_, reviewID := seedOrgAndReview(t, db, "")

	n := New(db)
	if err := n.dispatch(context.Background(), reviewID, KindCompleted); err != nil {
		t.Fatalf("dispatch should silently no-op for an empty UserEmail, got error: %v", err)
	}
}

func TestDispatchSkipsWhenUserHasNoSubscriptions(t *testing.T) {
	db := testDB(t)

	var userID int64
	email := "pushnotify-test-user@example.com"
	if err := db.QueryRow(`
		INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id
	`, email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	_, reviewID := seedOrgAndReview(t, db, email)

	n := New(db)
	// No push_subscriptions row for this user — dispatch should return
	// cleanly without attempting to send anything.
	if err := n.dispatch(context.Background(), reviewID, KindCompleted); err != nil {
		t.Fatalf("dispatch should no-op with zero subscriptions, got error: %v", err)
	}
}

func TestPreferenceAllowsDefaultsTrueWithNoPreferenceRow(t *testing.T) {
	db := testDB(t)

	var userID int64
	email := "pushnotify-pref-default-test@example.com"
	if err := db.QueryRow(`
		INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id
	`, email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	n := New(db)
	for _, kind := range []Kind{KindFirstFinding, KindCompleted} {
		allowed, err := n.preferenceAllows(context.Background(), userID, kind)
		if err != nil {
			t.Fatalf("preferenceAllows(%s): %v", kind, err)
		}
		if !allowed {
			t.Fatalf("expected %s to default to allowed with no preference row", kind)
		}
	}
}

func TestGetVAPIDPublicKeyGeneratesOnceAndPersists(t *testing.T) {
	db := testDB(t)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM system_settings WHERE name = 'push_vapid'`) })
	_, _ = db.Exec(`DELETE FROM system_settings WHERE name = 'push_vapid'`)

	first, err := GetVAPIDPublicKey(context.Background(), db)
	if err != nil {
		t.Fatalf("first GetVAPIDPublicKey: %v", err)
	}
	if first == "" {
		t.Fatalf("expected a non-empty public key")
	}

	second, err := GetVAPIDPublicKey(context.Background(), db)
	if err != nil {
		t.Fatalf("second GetVAPIDPublicKey: %v", err)
	}
	if second != first {
		t.Fatalf("expected the same key to be returned on repeat calls, got %q then %q", first, second)
	}
}

func TestPreferenceAllowsRespectsStoredFalse(t *testing.T) {
	db := testDB(t)

	var userID int64
	email := "pushnotify-pref-false-test@example.com"
	if err := db.QueryRow(`
		INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id
	`, email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	if _, err := db.Exec(`
		INSERT INTO user_notification_preferences (user_id, notify_first_finding, notify_completion)
		VALUES ($1, false, true)
	`, userID); err != nil {
		t.Fatalf("insert preferences: %v", err)
	}

	n := New(db)
	allowedFirst, err := n.preferenceAllows(context.Background(), userID, KindFirstFinding)
	if err != nil {
		t.Fatalf("preferenceAllows(first_finding): %v", err)
	}
	if allowedFirst {
		t.Fatalf("expected notify_first_finding=false to be respected")
	}

	allowedCompletion, err := n.preferenceAllows(context.Background(), userID, KindCompleted)
	if err != nil {
		t.Fatalf("preferenceAllows(completed): %v", err)
	}
	if !allowedCompletion {
		t.Fatalf("expected notify_completion=true to be respected")
	}
}
