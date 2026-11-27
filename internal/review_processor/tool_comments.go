package reviewprocessor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/providers"
	reviewpkg "github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

// ToolPostConfig configures PR comment posting for classified SAST tool findings.
type ToolPostConfig struct {
	ProviderConfig *reviewpkg.ProviderConfig
	PostingID      string
	PacingDelay    time.Duration
	MaxRetries     int
}

// MergeAndDedupeReviewComments merges AI comments and static tool comments in memory,
// preserving all AI comments first and deduplicating overlapping tool findings.
func MergeAndDedupeReviewComments(aiComments, toolComments []*models.ReviewComment) []*models.ReviewComment {
	_, merged := DedupeToolCommentsAgainstAI(aiComments, toolComments)
	return merged
}

// DedupeToolCommentsAgainstAI filters toolComments against aiComments (same normalized
// file_path + line as an AI comment, or identical tool comment), returning both the
// filtered tool comments for provider posting and the combined merged list for storage.
// Running this once before PostToolCommentsToProvider guarantees PR and UI match.
func DedupeToolCommentsAgainstAI(aiComments, toolComments []*models.ReviewComment) (uniqueTools []*models.ReviewComment, merged []*models.ReviewComment) {
	merged = make([]*models.ReviewComment, 0, len(aiComments)+len(toolComments))
	uniqueTools = make([]*models.ReviewComment, 0, len(toolComments))
	aiLineSeen := make(map[string]bool)
	seenToolKeys := make(map[string]bool)

	for _, c := range aiComments {
		if c == nil {
			continue
		}
		merged = append(merged, c)
		if c.Source == "tool" || c.ToolName != "" {
			seenToolKeys[toolCommentDedupKey(c)] = true
			continue
		}
		if lineKey := aiCommentLineKey(c); lineKey != "" {
			aiLineSeen[lineKey] = true
		}
	}

	for _, tc := range toolComments {
		if tc == nil {
			continue
		}
		if lineKey := aiCommentLineKey(tc); lineKey != "" && aiLineSeen[lineKey] {
			continue
		}
		key := toolCommentDedupKey(tc)
		if seenToolKeys[key] {
			continue
		}
		seenToolKeys[key] = true
		uniqueTools = append(uniqueTools, tc)
		merged = append(merged, tc)
	}

	return uniqueTools, merged
}

func normalizeCommentPath(p string) string {
	norm := filepath.ToSlash(strings.TrimSpace(p))
	norm = strings.TrimPrefix(norm, "./")
	norm = strings.TrimPrefix(norm, "/")
	return norm
}

func aiCommentLineKey(c *models.ReviewComment) string {
	normPath := normalizeCommentPath(c.FilePath)
	if normPath == "" || c.Line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", normPath, c.Line)
}

func toolCommentDedupKey(c *models.ReviewComment) string {
	return fmt.Sprintf("%s:%s:%d:%s", strings.ToLower(strings.TrimSpace(c.ToolName)), normalizeCommentPath(c.FilePath), c.Line, strings.TrimSpace(c.Content))
}

func commentPostMarkerKey(c *models.ReviewComment) string {
	normTool := strings.ToLower(strings.TrimSpace(c.ToolName))
	if normTool == "" {
		normTool = "unknown"
	}
	normRule := strings.ToLower(strings.TrimSpace(c.RuleID))
	normPath := normalizeCommentPath(c.FilePath)

	// Hash only stable identity fields so Tier-3 wording variations across retries
	// do not change the marker key. Fall back to content only for general un-anchored notes.
	identity := fmt.Sprintf("%s|%s|%s|%d", normTool, normRule, normPath, c.Line)
	if normRule == "" && c.Line <= 0 {
		identity = identity + "|" + strings.TrimSpace(c.Content)
	}
	sum := sha256.Sum256([]byte(identity))
	return normTool + ":" + hex.EncodeToString(sum[:8])
}

type commentErrorKind int

const (
	commentErrPermanentSkip commentErrorKind = iota
	commentErrTransient
	commentErrAuthAbort
)

func classifyCommentPostError(parentCtx context.Context, err error, providerName string) commentErrorKind {
	if err == nil {
		return commentErrPermanentSkip
	}
	if isTransientCommentError(parentCtx, err) {
		return commentErrTransient
	}
	if providers.IsAuthError(err, providerName) {
		return commentErrAuthAbort
	}
	return commentErrPermanentSkip
}

func isTransientCommentError(parentCtx context.Context, err error) bool {
	if err == nil {
		return false
	}
	var rlErr *providers.RateLimitedError
	if errors.As(err, &rlErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) && (parentCtx == nil || parentCtx.Err() == nil) {
		return true
	}

	msg := strings.ToLower(err.Error())
	// Secondary / abuse rate limits often arrive on HTTP 403; treat as transient
	// before IsAuthError classifies generic 403s as auth failures.
	if strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "secondary rate") ||
		strings.Contains(msg, "abuse detection") ||
		strings.Contains(msg, "too many requests") ||
		containsHTTPStatus(msg, "429") {
		return true
	}
	for _, code := range []string{"500", "502", "503", "504"} {
		if containsHTTPStatus(msg, code) {
			return true
		}
	}
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "i/o timeout")
}

func containsHTTPStatus(msg, code string) bool {
	idx := 0
	for {
		pos := strings.Index(msg[idx:], code)
		if pos < 0 {
			return false
		}
		start := idx + pos
		end := start + len(code)
		leftOK := start == 0 || !isASCIIDigit(msg[start-1])
		rightOK := end == len(msg) || !isASCIIDigit(msg[end])
		if leftOK && rightOK {
			return true
		}
		idx = end
	}
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// PostToolCommentsToProvider posts classified SAST findings to the configured git provider
// once per review, pacing requests, respecting rate-limit Retry-After backoff, continuing
// across single-comment failures (e.g. HTTP 422), and recording per-tool markers in
// reviews.metadata.tools_posted_to_provider so job retries never double-post.
func PostToolCommentsToProvider(
	ctx context.Context,
	db *sql.DB,
	orgID, reviewID int64,
	comments []*models.ReviewComment,
	cfg ToolPostConfig,
	eventSink logging.EventSink,
) {
	if cfg.ProviderConfig == nil || cfg.ProviderConfig.Type == "" || cfg.ProviderConfig.Type == "cli" || cfg.PostingID == "" || len(comments) == 0 {
		return
	}

	providerCfg := *cfg.ProviderConfig
	clonedConfig := make(map[string]interface{}, len(providerCfg.Config)+1)
	for k, v := range providerCfg.Config {
		clonedConfig[k] = v
	}
	providerCfg.Config = clonedConfig
	if repoURL, _ := providerCfg.Config["repo_url"].(string); repoURL == "" && providerCfg.URL != "" {
		providerCfg.Config["repo_url"] = providerCfg.URL
	}

	providerFactory := reviewpkg.NewStandardProviderFactory()
	provider, err := providerFactory.CreateProvider(ctx, providerCfg)
	if err != nil {
		log.Printf("[WARN] PostToolCommentsToProvider: failed to create provider %s for review %d: %v",
			providerCfg.Type, reviewID, err)
		if eventSink != nil {
			_ = eventSink.EmitLogEvent(ctx, reviewID, orgID, "warn",
				fmt.Sprintf("Failed to create %s provider for static tool comments: %v", providerCfg.Type, err), "")
		}
		return
	}

	postToolCommentsWithProvider(ctx, db, orgID, reviewID, comments, cfg, providerCfg.Type, provider, eventSink)
}

func postToolCommentsWithProvider(
	ctx context.Context,
	db *sql.DB,
	orgID, reviewID int64,
	comments []*models.ReviewComment,
	cfg ToolPostConfig,
	providerType string,
	provider providers.Provider,
	eventSink logging.EventSink,
) {
	if provider == nil || cfg.PostingID == "" || len(comments) == 0 {
		return
	}

	postedMarkers := loadPostedToolMarkers(ctx, db, orgID, reviewID)

	toolOrder := make([]string, 0)
	byTool := make(map[string][]*models.ReviewComment)
	for _, c := range comments {
		if c == nil {
			continue
		}
		toolName := strings.TrimSpace(c.ToolName)
		if toolName == "" {
			toolName = "unknown"
		}
		normKey := strings.ToLower(toolName)
		commentCopy := new(models.ReviewComment)
		*commentCopy = *c
		if commentCopy.ToolName == "" {
			commentCopy.ToolName = toolName
		}
		if postedMarkers[normKey] || postedMarkers[commentPostMarkerKey(commentCopy)] {
			continue
		}
		if _, exists := byTool[normKey]; !exists {
			toolOrder = append(toolOrder, normKey)
		}
		byTool[normKey] = append(byTool[normKey], commentCopy)
	}

	if len(toolOrder) == 0 {
		return
	}

	for _, normKey := range toolOrder {
		if ctx.Err() != nil {
			return
		}
		group := byTool[normKey]
		if len(group) == 0 {
			continue
		}
		displayToolName := group[0].ToolName

		postedCount, permanentSkipped := postToolCommentGroup(ctx, db, orgID, reviewID, provider, cfg.PostingID, displayToolName, group, cfg.PacingDelay, cfg.MaxRetries)

		if postedCount > 0 {
			log.Printf("[INFO] PostToolCommentsToProvider: posted %d/%d %s comments to %s for review %d",
				postedCount, len(group), displayToolName, providerType, reviewID)
			if eventSink != nil {
				_ = eventSink.EmitLogEvent(ctx, reviewID, orgID, "success",
					fmt.Sprintf("Posted %d %s findings to pull request", postedCount, strings.ToUpper(displayToolName)), "")
			}
		} else if permanentSkipped < len(group) {
			if eventSink != nil {
				_ = eventSink.EmitLogEvent(ctx, reviewID, orgID, "warn",
					fmt.Sprintf("Failed to post %s comments to pull request", strings.ToUpper(displayToolName)), "")
			}
		}
	}
}

func postToolCommentGroup(
	ctx context.Context,
	db *sql.DB,
	orgID, reviewID int64,
	provider providers.Provider,
	postingID string,
	toolName string,
	group []*models.ReviewComment,
	pacingDelay time.Duration,
	maxRetries int,
) (postedCount int, permanentSkipped int) {
	if maxRetries < 0 {
		maxRetries = 0
	}

	for idx, comment := range group {
		if ctx.Err() != nil {
			return postedCount, permanentSkipped
		}
		if idx > 0 && pacingDelay > 0 {
			if !sleepWithContext(ctx, pacingDelay) {
				return postedCount, permanentSkipped
			}
		}

		postErr, abortAuth := postSingleCommentWithRetry(ctx, provider, postingID, toolName, comment, maxRetries)
		if postErr == nil {
			postedCount++
			if err := persistCommentMarkerWithRetry(ctx, db, orgID, reviewID, "tools_posted_to_provider", commentPostMarkerKey(comment)); err != nil {
				log.Printf("[WARN] PostToolCommentsToProvider: failed to persist comment marker for tool=%s review=%d: %v", toolName, reviewID, err)
			}
			continue
		}
		if abortAuth {
			log.Printf("[WARN] PostToolCommentsToProvider: auth failure posting %s comment at %s:%d (aborting remaining posts): %v",
				toolName, comment.FilePath, comment.Line, postErr)
			return postedCount, permanentSkipped
		}
		if !isTransientCommentError(ctx, postErr) {
			permanentSkipped++
			if err := persistCommentMarkerWithRetry(ctx, db, orgID, reviewID, "tools_skipped_on_provider", commentPostMarkerKey(comment)); err != nil {
				log.Printf("[WARN] PostToolCommentsToProvider: failed to persist skipped marker for tool=%s review=%d: %v", toolName, reviewID, err)
			}
			log.Printf("[WARN] PostToolCommentsToProvider: non-transient error posting %s comment at %s:%d (skipped): %v",
				toolName, comment.FilePath, comment.Line, postErr)
			continue
		}
		log.Printf("[WARN] PostToolCommentsToProvider: transient error posting %s comment at %s:%d (leaving unmarked for retry): %v",
			toolName, comment.FilePath, comment.Line, postErr)
	}
	return postedCount, permanentSkipped
}

func postSingleCommentWithRetry(
	ctx context.Context,
	provider providers.Provider,
	postingID string,
	toolName string,
	comment *models.ReviewComment,
	maxRetries int,
) (error, bool) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err(), false
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := provider.PostComment(attemptCtx, postingID, comment)
		cancel()
		if err == nil {
			return nil, false
		}
		lastErr = err

		kind := classifyCommentPostError(ctx, err, provider.Name())
		switch kind {
		case commentErrAuthAbort:
			return err, true
		case commentErrPermanentSkip:
			return err, false
		}

		if attempt >= maxRetries {
			break
		}

		waitDuration := time.Duration(attempt+1) * time.Second
		var rlErr *providers.RateLimitedError
		if errors.As(err, &rlErr) && rlErr.RetryAfter > 0 {
			waitDuration = rlErr.RetryAfter
		}
		if waitDuration > 90*time.Second {
			waitDuration = 90 * time.Second
		}
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
			remaining := time.Until(deadline) - 2*time.Second
			if remaining <= 0 || waitDuration > remaining {
				return lastErr, false
			}
		}

		log.Printf("[WARN] PostToolCommentsToProvider: retrying %s comment at %s:%d in %v (attempt %d/%d): %v",
			toolName, comment.FilePath, comment.Line, waitDuration, attempt+1, maxRetries+1, err)
		if !sleepWithContext(ctx, waitDuration) {
			return ctx.Err(), false
		}
	}
	return lastErr, false
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func loadPostedToolMarkers(ctx context.Context, db *sql.DB, orgID, reviewID int64) map[string]bool {
	posted := make(map[string]bool)
	if db == nil || reviewID <= 0 || orgID <= 0 {
		return posted
	}

	var rawPosted, rawSkipped []byte
	err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(metadata->'tools_posted_to_provider', '{}'::jsonb),
			COALESCE(metadata->'tools_skipped_on_provider', '{}'::jsonb)
		FROM public.reviews
		WHERE id = $1 AND org_id = $2
	`, reviewID, orgID).Scan(&rawPosted, &rawSkipped)
	if err != nil {
		log.Printf("[WARN] PostToolCommentsToProvider: failed to load posted markers for review=%d org=%d: %v", reviewID, orgID, err)
		return posted
	}

	for _, raw := range [][]byte{rawPosted, rawSkipped} {
		if len(raw) == 0 {
			continue
		}
		var decoded map[string]bool
		if unmarshalErr := json.Unmarshal(raw, &decoded); unmarshalErr != nil {
			log.Printf("[WARN] PostToolCommentsToProvider: malformed marker JSONB for review=%d org=%d: %v", reviewID, orgID, unmarshalErr)
			continue
		}
		for k, v := range decoded {
			if v {
				posted[strings.ToLower(strings.TrimSpace(k))] = true
			}
		}
	}
	return posted
}

func persistCommentMarkerWithRetry(ctx context.Context, db *sql.DB, orgID, reviewID int64, sectionKey, toolKey string) error {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		markerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err := markToolMetadataMarker(markerCtx, db, orgID, reviewID, sectionKey, toolKey)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func markToolPostedToProvider(ctx context.Context, db *sql.DB, orgID, reviewID int64, toolKey string) error {
	return markToolMetadataMarker(ctx, db, orgID, reviewID, "tools_posted_to_provider", toolKey)
}

func markToolMetadataMarker(ctx context.Context, db *sql.DB, orgID, reviewID int64, sectionKey, toolKey string) error {
	if db == nil || reviewID <= 0 || orgID <= 0 || toolKey == "" {
		return nil
	}
	if sectionKey != "tools_posted_to_provider" && sectionKey != "tools_skipped_on_provider" {
		return fmt.Errorf("invalid marker section %q", sectionKey)
	}
	normKey := strings.ToLower(strings.TrimSpace(toolKey))

	// Set only the specific marker key path inside the ensured parent object so
	// concurrent marker or review_result updates never overwrite sibling keys.
	_, err := db.ExecContext(ctx, `
		UPDATE public.reviews
		SET metadata = jsonb_set(
			jsonb_set(
				COALESCE(metadata, '{}'::jsonb),
				ARRAY[$1::text],
				COALESCE(metadata->$1::text, '{}'::jsonb),
				true
			),
			ARRAY[$1::text, $2::text],
			'true'::jsonb,
			true
		)
		WHERE id = $3 AND org_id = $4
	`, sectionKey, normKey, reviewID, orgID)
	return err
}
