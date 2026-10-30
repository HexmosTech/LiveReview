package reviewprocessor

import "context"

// ReviewNotifier is the cross-cutting hook used to fire browser push
// notifications for review progress. Implemented by internal/pushnotify,
// which this package deliberately does not import — pushnotify needs no
// types from here, and keeping the dependency one-directional (set from
// outside, called from inside) avoids an import cycle and matches how
// internal/jobqueue already wires DatabaseEventSink into a ReviewLogger.
//
// A package-level hook (not a constructor parameter on ReviewManager/
// PollingEventService) is used because NewReviewManager(db) and
// NewPollingEventService(db) are each constructed fresh at many unrelated
// call sites throughout the codebase; threading a notifier through every one
// of them would be the same "touch everything" problem this hook exists to
// avoid for a cross-cutting concern.
type ReviewNotifier interface {
	// NotifyReviewEvent should be non-blocking (fire-and-forget) — it is
	// called from hot paths (batch-completion event persistence, review
	// status updates) that must never wait on a push send. kind is one of
	// "first_finding", "completed", "failed".
	NotifyReviewEvent(ctx context.Context, reviewID, orgID int64, kind string)
}

var notifier ReviewNotifier

// SetNotifier wires the push-notification dispatcher in. Call once at server
// startup (internal/api/server.go's NewServer); until called, notifyReviewEvent
// is a no-op, which is also the correct behavior for tests and for any
// command that constructs a ReviewManager without a full server (e.g. one-off
// scripts), rather than requiring every caller to nil-check.
func SetNotifier(n ReviewNotifier) {
	notifier = n
}

// NotifyReviewEvent is the entry point every event-sink implementation calls
// (this package's own, and internal/api's separate
// DatabaseEventSink/PollingEventService used by the synchronous CLI-diff
// review path — see internal/api/review_service.go) so both write paths
// fire push notifications identically, through this one no-op-if-unset hook.
func NotifyReviewEvent(ctx context.Context, reviewID, orgID int64, kind string) {
	if notifier == nil {
		return
	}
	notifier.NotifyReviewEvent(ctx, reviewID, orgID, kind)
}
