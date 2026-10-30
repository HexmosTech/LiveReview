// Package pushnotify sends browser push notifications (Web Push API) for
// review progress: the first finding becoming available, and the review
// completing or failing. See /home/shrsv/.claude/plans/wiggly-floating-thunder.md
// for the design this implements.
//
// Scope is intentionally limited to reviews triggered by a logged-in session
// — Review.UserEmail (internal/review_processor's Review struct) is only
// reliably populated in that case; scheduled/webhook-triggered reviews have
// no identifiable human to notify, and silently not notifying anyone is the
// correct behavior for those, not a gap.
package pushnotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Kind identifies which of the two notification moments fired.
type Kind string

const (
	KindFirstFinding Kind = "first_finding"
	KindCompleted    Kind = "completed"
	KindFailed       Kind = "failed"
)

// Notifier sends review-progress push notifications. It satisfies
// internal/review_processor's ReviewNotifier interface structurally, so that
// package never needs to import this one (avoiding an import cycle, since
// this package itself has no need to depend on review_processor's types —
// a review id and org id, looked up fresh here, are enough).
type Notifier struct {
	db *sql.DB
}

// New creates a Notifier backed by db. Call reviewprocessor.SetNotifier(New(db))
// once at server startup (internal/api/server.go's NewServer) to wire it in.
func New(db *sql.DB) *Notifier {
	return &Notifier{db: db}
}

// NotifyReviewEvent is fire-and-forget: it spawns its own goroutine with a
// context detached from the caller's, so a slow or unreachable push
// endpoint never blocks the hot paths that call it (batch-completion event
// persistence, review status updates). Errors are logged, never returned —
// a failed push must never fail the review itself.
func (n *Notifier) NotifyReviewEvent(ctx context.Context, reviewID, orgID int64, kind string) {
	go func() {
		dispatchCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := n.dispatch(dispatchCtx, reviewID, Kind(kind)); err != nil {
			log.Printf("[WARN] push notification dispatch failed for review %d (kind=%s): %v", reviewID, kind, err)
		}
	}()
}

type reviewInfo struct {
	userEmail    string
	friendlyName string
	repository   string
	prMrURL      string
	mrTitle      string
}

func (n *Notifier) loadReviewInfo(ctx context.Context, reviewID int64) (*reviewInfo, error) {
	var info reviewInfo
	var friendlyName, prMrURL, mrTitle sql.NullString
	err := n.db.QueryRowContext(ctx, `
		SELECT user_email, friendly_name, repository, pr_mr_url, mr_title
		FROM reviews WHERE id = $1
	`, reviewID).Scan(&info.userEmail, &friendlyName, &info.repository, &prMrURL, &mrTitle)
	if err != nil {
		return nil, fmt.Errorf("load review %d: %w", reviewID, err)
	}
	info.friendlyName = friendlyName.String
	info.prMrURL = prMrURL.String
	info.mrTitle = mrTitle.String
	return &info, nil
}

func (n *Notifier) resolveUserID(ctx context.Context, email string) (int64, bool, error) {
	if strings.TrimSpace(email) == "" {
		// No identifiable session user triggered this review (scheduled/webhook) — nothing to notify, not an error.
		return 0, false, nil
	}
	var userID int64
	err := n.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return userID, true, nil
}

// preferenceAllows reports whether userID has notifications enabled for
// kind. A missing user_notification_preferences row means both types are
// enabled (the agreed default) — see the migration's comment.
func (n *Notifier) preferenceAllows(ctx context.Context, userID int64, kind Kind) (bool, error) {
	var notifyFirstFinding, notifyCompletion bool
	err := n.db.QueryRowContext(ctx, `
		SELECT notify_first_finding, notify_completion
		FROM user_notification_preferences WHERE user_id = $1
	`, userID).Scan(&notifyFirstFinding, &notifyCompletion)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if kind == KindFirstFinding {
		return notifyFirstFinding, nil
	}
	return notifyCompletion, nil
}

type subscriptionRow struct {
	id       int64
	endpoint string
	p256dh   string
	authKey  string
}

func (n *Notifier) loadSubscriptions(ctx context.Context, userID int64) ([]subscriptionRow, error) {
	rows, err := n.db.QueryContext(ctx, `
		SELECT id, endpoint, p256dh, auth_key FROM push_subscriptions WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []subscriptionRow
	for rows.Next() {
		var s subscriptionRow
		if err := rows.Scan(&s.id, &s.endpoint, &s.p256dh, &s.authKey); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (n *Notifier) deleteSubscription(ctx context.Context, id int64) {
	if _, err := n.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id); err != nil {
		log.Printf("[WARN] failed to prune dead push subscription %d: %v", id, err)
	}
}

type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// shortRepoLabel turns a stored repository/PR value — which can be a bare
// "owner/repo", a git remote URL, or (for webui-triggered reviews) the PR/MR
// URL itself stored verbatim in the repository column — into a compact
// "owner/repo" label, by dropping a leading host segment (the one
// containing a dot) and any trailing path segments (e.g. "pull/150") past
// the first two.
func shortRepoLabel(raw string) string {
	s := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.ReplaceAll(s, ":", "/")
	parts := strings.Split(s, "/")
	if len(parts) > 0 && strings.Contains(parts[0], ".") {
		parts = parts[1:]
	}
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0]
	}
	return raw
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}

// reviewDetail builds the "quickly recognize which review this is" portion
// shown in the notification body — the MR/PR title when known (most
// specific), falling back to a friendly name, then a short "owner/repo".
func reviewDetail(info *reviewInfo) string {
	if title := strings.TrimSpace(info.mrTitle); title != "" {
		return truncate(title, 80)
	}
	if info.friendlyName != "" {
		return info.friendlyName
	}
	if info.repository != "" {
		return shortRepoLabel(info.repository)
	}
	return "your review"
}

func buildPayload(kind Kind, reviewID int64, info *reviewInfo, prodURL string) pushPayload {
	ref := fmt.Sprintf("#%d", reviewID)
	repo := shortRepoLabel(info.repository)
	detail := reviewDetail(info)
	identity := ref
	if repo != "" {
		identity = fmt.Sprintf("%s · %s", ref, repo)
	}

	var p pushPayload
	switch kind {
	case KindFirstFinding:
		p = pushPayload{Title: "First finding ready", Body: fmt.Sprintf("%s — %s: first results are in.", identity, detail)}
	case KindFailed:
		// Not "Review failed" — most failures here are partial/recoverable
		// (a batch errored, not necessarily every finding lost), so this
		// avoids implying total loss before the user has even looked.
		p = pushPayload{Title: "Review completed with errors", Body: fmt.Sprintf("%s — %s: didn't finish cleanly, take a look.", identity, detail)}
	default:
		p = pushPayload{Title: "Review completed", Body: fmt.Sprintf("%s — %s: ready to view.", identity, detail)}
	}
	p.Tag = fmt.Sprintf("review-%d", reviewID)
	if prodURL != "" {
		p.URL = strings.TrimRight(prodURL, "/") + fmt.Sprintf("/#/reviews/%d", reviewID)
	}
	return p
}

func (n *Notifier) getProductionURL(ctx context.Context) string {
	var url sql.NullString
	if err := n.db.QueryRowContext(ctx, `SELECT livereview_prod_url FROM instance_details LIMIT 1`).Scan(&url); err != nil {
		return ""
	}
	return strings.TrimSpace(url.String)
}

func (n *Notifier) dispatch(ctx context.Context, reviewID int64, kind Kind) error {
	// Callers fire "first_finding" on every batch that completes with
	// comments (internal/review_processor/events.go's CreateBatchEvent) —
	// batches run concurrently, so the atomic claim below, not the caller,
	// is what guarantees only the first one actually sends a notification.
	if kind == KindFirstFinding {
		won, err := ClaimFirstFinding(ctx, n.db, reviewID)
		if err != nil {
			return fmt.Errorf("claim first finding: %w", err)
		}
		if !won {
			return nil
		}
	}

	info, err := n.loadReviewInfo(ctx, reviewID)
	if err != nil {
		return err
	}

	userID, found, err := n.resolveUserID(ctx, info.userEmail)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	allowed, err := n.preferenceAllows(ctx, userID, kind)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}

	subs, err := n.loadSubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}

	vapidPublic, vapidPrivate, err := loadOrCreateVAPIDKeys(ctx, n.db)
	if err != nil {
		return fmt.Errorf("load VAPID keys: %w", err)
	}

	payload, err := json.Marshal(buildPayload(kind, reviewID, info, n.getProductionURL(ctx)))
	if err != nil {
		return err
	}

	options := &webpush.Options{
		Subscriber:      "mailto:support@hexmos.com",
		VAPIDPublicKey:  vapidPublic,
		VAPIDPrivateKey: vapidPrivate,
		TTL:             60 * 60,
	}

	for _, sub := range subs {
		resp, sendErr := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: sub.endpoint,
			Keys:     webpush.Keys{Auth: sub.authKey, P256dh: sub.p256dh},
		}, options)
		if sendErr != nil {
			log.Printf("[WARN] push send failed for subscription %d: %v", sub.id, sendErr)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			// The browser revoked this subscription; stop sending to it.
			n.deleteSubscription(ctx, sub.id)
			continue
		}
		if resp.StatusCode >= 300 {
			log.Printf("[WARN] push send for subscription %d got status %d", sub.id, resp.StatusCode)
		}
	}

	return nil
}

// ClaimFirstFinding atomically marks reviewID as having notified its first
// finding, returning true only for the caller that won the race — batches
// run concurrently (internal/batch/task_queue.go's worker pool), so this
// must be a single atomic UPDATE, not an in-memory flag.
func ClaimFirstFinding(ctx context.Context, db *sql.DB, reviewID int64) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE reviews SET first_finding_notified_at = NOW()
		WHERE id = $1 AND first_finding_notified_at IS NULL
	`, reviewID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}
