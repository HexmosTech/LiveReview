package api

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// A review whose setup fails (e.g. no AI provider) must not stay "created": the UI would show it stuck at the
// first stage, and the "reuse a running review" lookup would hand it back for the next 10 minutes.
func TestFailReviewSetup_MarksReviewFailedSoItIsNotReused(t *testing.T) {
	f := newFakeGitHub(t)
	s, db := newGitHubAppTestServer(t, f)

	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'setup failure test') RETURNING id`,
		fmt.Sprintf("gh-setupfail-%d@example.test", time.Now().UnixNano())).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	review, err := NewReviewManager(db).CreateReviewWithOrg("acme/api", "main", "", "https://github.com/acme/api/pull/1",
		"manual", "dev@example.test", "github", nil, map[string]interface{}{}, orgID, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, review.ID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	ongoing := func() int {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE id = $1 AND status IN ('created', 'in_progress')`, review.ID).Scan(&n)
		return n
	}
	if ongoing() != 1 {
		t.Fatal("precondition: a new review row counts as ongoing")
	}

	s.failReviewSetup(&reviewSetupContext{review: review}, errors.New("no AI provider"))

	var status string
	if err := db.QueryRow(`SELECT status FROM reviews WHERE id = $1`, review.ID).Scan(&status); err != nil || status != "failed" {
		t.Fatalf("status %q (err %v), want failed", status, err)
	}
	if ongoing() != 0 {
		t.Error("a failed review must not count as ongoing (it would be reused for 10 minutes)")
	}

	// No review row yet (failure before it was created): nothing to do, must not panic.
	s.failReviewSetup(&reviewSetupContext{}, errors.New("x"))
	s.failReviewSetup(nil, errors.New("x"))
}
