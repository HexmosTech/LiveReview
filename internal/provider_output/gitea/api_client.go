package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
	giteautils "github.com/livereview/internal/providers/gitea"
	networkgitea "github.com/livereview/network/providers/gitea"
)

type (
	UnifiedWebhookEventV2  = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2  = coreprocessor.UnifiedMergeRequestV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
	UnifiedPositionV2      = coreprocessor.UnifiedPositionV2
)

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Gitea API request failed with status %d: %s", e.StatusCode, e.Body)
}

// APIClient posts outbound Gitea content on behalf of the provider.
type APIClient struct {
	httpClient *http.Client
}

// NewAPIClient constructs a Gitea output client with sensible defaults.
func NewAPIClient() *APIClient {
	return &APIClient{httpClient: networkgitea.NewHTTPClient(30 * time.Second)}
}

// NewAPIClientWithHTTPClient allows tests and callers to inject a custom HTTP client.
func NewAPIClientWithHTTPClient(httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = networkgitea.NewHTTPClient(30 * time.Second)
	}
	return &APIClient{httpClient: httpClient}
}

// PostCommentReply posts a reply to an existing Gitea comment thread.
func (c *APIClient) PostCommentReply(event *UnifiedWebhookEventV2, token, replyText string) error {
	log.Printf("[DEBUG] APIClient.PostCommentReply: comment_id=%s, has_position=%v, reply_len=%d",
		event.Comment.ID, event.Comment.Position != nil, len(replyText))

	if event == nil || event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	baseURL, err := extractBaseURL(event)
	if err != nil {
		return fmt.Errorf("failed to extract base URL: %w", err)
	}

	creds := giteautils.UnpackGiteaCredentials(token)
	if creds.Username == "" && event.MergeRequest != nil && event.MergeRequest.Metadata != nil {
		if u, ok := event.MergeRequest.Metadata["gitea_username"].(string); ok && u != "" {
			creds.Username = u
		}
		if p, ok := event.MergeRequest.Metadata["gitea_password"].(string); ok && p != "" {
			creds.Password = p
		}
	}
	token = creds.PAT
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	// Extract review_id from metadata if present
	var reviewID int64
	if event.Comment.Metadata != nil {
		if rid, ok := event.Comment.Metadata["review_id"].(int64); ok && rid > 0 {
			reviewID = rid
		} else if ridFloat, ok := event.Comment.Metadata["review_id"].(float64); ok && ridFloat > 0 {
			reviewID = int64(ridFloat)
		}
	}

	// Always enrich when reviewID==0: Gitea sends identical issue_comment webhooks for
	// true general PR comments AND inline thread replies. The only way to distinguish
	// them is via the API. enrichCommentMetadata is a no-op (leaves Position nil) when
	// the comment is not found in any review → falls through to general comment path.
	var replyCommentID int64
	if reviewID == 0 && event.Comment.ID != "" {
		log.Printf("[DEBUG] reviewID=0, attempting enrichment for comment_id=%s", event.Comment.ID)

		// Use username/password to create temporary PAT for API call if PAT is invalid
		enrichToken := token
		if creds.Username != "" && creds.Password != "" {
			log.Printf("[DEBUG] Creating temporary PAT for metadata enrichment (username=%s)", creds.Username)
			if tempPAT, err := c.createTemporaryPAT(baseURL, creds.Username, creds.Password); err != nil {
				log.Printf("[WARN] Failed to create temporary PAT: %v, using existing token", err)
			} else {
				enrichToken = tempPAT
				log.Printf("[DEBUG] Using temporary PAT for enrichment")
			}
		}
		if err := c.enrichCommentMetadata(baseURL, owner, repo, event, enrichToken); err != nil {
			log.Printf("[WARN] Metadata enrichment failed: %v (will proceed with general comment)", err)
		} else {
			log.Printf("[DEBUG] Metadata enrichment completed")
		}
		// Re-extract review_id and reply_comment_id after enrichment
		if event.Comment.Metadata != nil {
			if rid, ok := event.Comment.Metadata["review_id"].(int64); ok && rid > 0 {
				reviewID = rid
			} else if ridFloat, ok := event.Comment.Metadata["review_id"].(float64); ok && ridFloat > 0 {
				reviewID = int64(ridFloat)
			}
			if rcid, ok := event.Comment.Metadata["reply_comment_id"].(int64); ok && rcid > 0 {
				replyCommentID = rcid
			} else if rcidFloat, ok := event.Comment.Metadata["reply_comment_id"].(float64); ok && rcidFloat > 0 {
				replyCommentID = int64(rcidFloat)
			}
			log.Printf("[DEBUG] Extracted after enrichment: review_id=%d, reply_comment_id=%d", reviewID, replyCommentID)
		}
	} else if event.Comment.Metadata != nil {
		// Even if reviewID was present, try to get replyCommentID from metadata
		if rcid, ok := event.Comment.Metadata["reply_comment_id"].(int64); ok && rcid > 0 {
			replyCommentID = rcid
		} else if rcidFloat, ok := event.Comment.Metadata["reply_comment_id"].(float64); ok && rcidFloat > 0 {
			replyCommentID = int64(rcidFloat)
		}
	}

	log.Printf("[DEBUG] Routing decision: review_id=%d, has_position=%v → %s",
		reviewID, event.Comment.Position != nil,
		func() string {
			if reviewID > 0 && event.Comment.Position != nil {
				return "INLINE_MULTIPART"
			}
			return "GENERAL_COMMENT"
		}())

	// Route based on whether this is a review comment or general comment
	if reviewID > 0 && event.Comment.Position != nil {
		// Inline review comment - use multipart form
		return c.postInlineCommentReply(baseURL, owner, repo, event, replyText, reviewID, replyCommentID, creds.Username, creds.Password)
	}

	// General comment on issue/PR: quote the original comment and tag the author
	var quotedBody strings.Builder
	if event.Comment.Author.Username != "" {
		quotedBody.WriteString(fmt.Sprintf("@%s\n", event.Comment.Author.Username))
	}
	if event.Comment.Body != "" {
		for _, line := range strings.Split(strings.TrimSpace(event.Comment.Body), "\n") {
			quotedBody.WriteString("> " + line + "\n")
		}
	}
	if quotedBody.Len() > 0 {
		quotedBody.WriteString("\n")
	}

	formattedReply := quotedBody.String() + strings.TrimSpace(replyText)

	return c.postGeneralComment(baseURL, owner, repo, event.MergeRequest.Number, token, formattedReply)
}

// postGeneralComment posts a general comment to an issue or PR
func (c *APIClient) postGeneralComment(baseURL, owner, repo string, prNumber int, token, replyText string) error {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments",
		baseURL, owner, repo, prNumber)

	requestBody := map[string]interface{}{
		"body": replyText,
	}

	log.Printf("[DEBUG] Gitea API call (general comment): %s", apiURL)
	log.Printf("[DEBUG] Gitea API payload: %+v", requestBody)

	return c.postToGiteaAPI(apiURL, token, requestBody)
}

// postInlineCommentReply posts a reply to an inline code review comment using multipart form.
func (c *APIClient) postInlineCommentReply(baseURL, owner, repo string, event *UnifiedWebhookEventV2, replyText string, reviewID, replyCommentID int64, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("inline comment reply requires username/password credentials")
	}
	if reviewID == 0 {
		return fmt.Errorf("inline comment reply requires review_id")
	}
	if event.Comment.Position == nil {
		return fmt.Errorf("inline comment reply requires position information")
	}
	if event.Comment.Position.FilePath == "" {
		return fmt.Errorf("inline comment reply requires file path")
	}
	if event.Comment.Position.LineNumber <= 0 {
		return fmt.Errorf("inline comment reply requires valid line number (got: %d)", event.Comment.Position.LineNumber)
	}

	return c.postInlineCommentReplyMultipart(baseURL, owner, repo, event.MergeRequest.Number, event, replyText, reviewID, replyCommentID, username, password)
}

// enrichCommentMetadata fetches review context by scanning all reviews in the PR.
// Gitea webhooks for user replies to inline review comments lack position/review_id.
// We scan all reviews to find the inline comment being replied to and extract its context.
func (c *APIClient) enrichCommentMetadata(baseURL, owner, repo string, event *UnifiedWebhookEventV2, token string) error {
	prNumber := event.MergeRequest.Number
	targetID, parseErr := strconv.ParseInt(event.Comment.ID, 10, 64)
	if parseErr != nil {
		return fmt.Errorf("invalid comment ID %q: %w", event.Comment.ID, parseErr)
	}
	log.Printf("[DIAG] enrichCommentMetadata ENTRY: pr=%d, target_comment_id=%d", prNumber, targetID)

	// Fetch all reviews for this PR
	reviewsURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", baseURL, owner, repo, prNumber)
	log.Printf("[DIAG] Fetching reviews from: %s", reviewsURL)

	req, err := networkgitea.NewRequestWithContext(context.Background(), "GET", reviewsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	client := c.httpClient
	if client == nil {
		client = networkgitea.NewHTTPClient(30 * time.Second)
	}

	resp, err := networkgitea.Do(client, req)
	if err != nil {
		return fmt.Errorf("failed to fetch reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] Failed to fetch reviews: status=%d, body=%s", resp.StatusCode, string(body)[:min(len(body), 300)])
		return fmt.Errorf("reviews API returned %d", resp.StatusCode)
	}

	var reviews []struct {
		ID            int64 `json:"id"`
		CommentsCount int64 `json:"comments_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return fmt.Errorf("failed to decode reviews: %w", err)
	}

	log.Printf("[DIAG] Found %d reviews, scanning for target comment %d", len(reviews), targetID)

	// Build full index: commentID → {reviewID, path, position, originalPosition, line}
	// Gitea sets position=0 when a comment becomes "outdated" (new commits pushed),
	// but original_position retains the original diff position and is always non-zero.
	type entry struct {
		ReviewID         int64
		Path             string
		Position         float64 // current diff position (0 when outdated)
		OriginalPosition float64 // original diff position (always set for inline comments)
		Line             float64 // actual file line (null in this Gitea version)
		InReplyTo        int64   // ID of the comment being replied to (null in this Gitea version)
	}
	index := make(map[int64]entry)
	// reviewRootComment tracks the LOWEST comment ID per review.
	// Gitea's multipart form `reply` field expects this root comment ID, not the review ID.
	// Using the review ID creates a new thread; using the root comment ID appends to the thread.
	reviewRootComment := make(map[int64]int64) // reviewID → root comment ID

	for _, review := range reviews {
		commentsURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews/%d/comments",
			baseURL, owner, repo, prNumber, review.ID)
		log.Printf("[DIAG] Fetching comments from review %d: %s", review.ID, commentsURL)

		creq, creqErr := networkgitea.NewRequestWithContext(context.Background(), "GET", commentsURL, nil)
		if creqErr != nil {
			log.Printf("[WARN] Failed to create review comments request for review %d: %v", review.ID, creqErr)
			continue
		}
		creq.Header.Set("Authorization", "token "+token)
		creq.Header.Set("Accept", "application/json")

		cresp, err := networkgitea.Do(client, creq)
		if err != nil {
			log.Printf("[WARN] Failed to fetch review %d comments: %v", review.ID, err)
			continue
		}
		if cresp.StatusCode != 200 {
			log.Printf("[WARN] Review %d comments returned %d", review.ID, cresp.StatusCode)
			cresp.Body.Close()
			continue
		}

		var comments []map[string]interface{}
		if err := json.NewDecoder(cresp.Body).Decode(&comments); err != nil {
			log.Printf("[WARN] Failed to decode review %d comments: %v", review.ID, err)
			cresp.Body.Close()
			continue
		}
		cresp.Body.Close()

		log.Printf("[DIAG] Review %d has %d comments", review.ID, len(comments))
		for _, cmt := range comments {
			id, _ := cmt["id"].(float64)
			path, _ := cmt["path"].(string)
			position, _ := cmt["position"].(float64)
			originalPosition, _ := cmt["original_position"].(float64)
			line, _ := cmt["line"].(float64)
			originalLine, _ := cmt["original_line"].(float64)
			if line == 0 {
				line = originalLine
			}
			inReplyTo, _ := cmt["in_reply_to"].(float64)
			prReviewID, _ := cmt["pull_request_review_id"].(float64)
			rid := int64(prReviewID)
			if rid == 0 {
				rid = review.ID
			}

			// Extract side information for LineType determination
			side, _ := cmt["side"].(string)
			if side == "" {
				side = "RIGHT" // default to new side
			}

			index[int64(id)] = entry{
				ReviewID:         rid,
				Path:             path,
				Position:         position,
				OriginalPosition: originalPosition,
				Line:             line,
				InReplyTo:        int64(inReplyTo),
			}

			// Store side information for LineType determination
			if event.Comment.ID == fmt.Sprintf("%.0f", id) {
				log.Printf("[DEBUG] Found target comment in API response: side=%s, path=%s, position=%.0f", side, path, position)
				event.Comment.Metadata["original_side"] = side
			}
			// Track the lowest comment ID per review (= thread root for the reply field)
			cmtID := int64(id)
			if existing, ok := reviewRootComment[rid]; !ok || cmtID < existing {
				reviewRootComment[rid] = cmtID
			}
			log.Printf("[DIAG] Indexed comment %.0f: review_id=%d, path=%s, pos=%.0f, orig_pos=%.0f, line=%.0f, in_reply_to=%.0f",
				id, rid, path, position, originalPosition, line, inReplyTo)
		}
	}

	// NOTE: The flat /pulls/{index}/comments endpoint returns 404 on this Gitea instance
	// (confirmed — not a PAT scope issue, the endpoint is not accessible).
	// We rely entirely on per-review /reviews/{id}/comments which gives us original_position.

	// Step 1: Is our target comment in any review at all?
	target, found := index[targetID]
	if !found {
		log.Printf("[DIAG] Comment %d not found in any review → true general PR comment, no enrichment", targetID)
		return nil
	}
	log.Printf("[DIAG] Target comment %d found in reviews: review_id=%d, path=%s, position=%.0f, line=%.0f",
		targetID, target.ReviewID, target.Path, target.Position, target.Line)

	// effectiveLine returns the best position to use for the multipart reply form.
	// Priority: position (current diff pos) → original_position (pre-outdated pos) → line (file line).
	// Gitea sets position=0 when commits are pushed after the comment, but original_position
	// retains the value needed to correctly anchor the multipart reply to the right thread.
	effectiveLine := func(e entry) float64 {
		if e.Position > 0 {
			return e.Position
		}
		if e.OriginalPosition > 0 {
			return e.OriginalPosition
		}
		return e.Line
	}

	// Step 2: If target itself has an inline anchor, use it directly.
	if ef := effectiveLine(target); ef > 0 {
		event.Comment.Metadata["review_id"] = target.ReviewID
		if rootCmtID, ok := reviewRootComment[target.ReviewID]; ok {
			event.Comment.Metadata["reply_comment_id"] = rootCmtID
		}
		// Determine LineType: Use position=0 to identify deleted lines (more reliable than Gitea's side field)
		lineType := "new" // default for new lines
		if target.Position == 0 && target.OriginalPosition > 0 {
			// position=0 means the line was deleted/modified after the comment was made
			// This indicates a deleted line
			lineType = "old"
			log.Printf("[DEBUG] Detected deleted line: position=0, original_position=%.0f -> LineType=old", target.OriginalPosition)
		} else {
			log.Printf("[DEBUG] Detected new/modified line: position=%.0f -> LineType=new", target.Position)
		}

		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   target.Path,
			LineNumber: int(ef),
			LineType:   lineType,
		}

		log.Printf("[DEBUG] Set comment position: path=%s, line=%d, lineType=%s", target.Path, int(ef), lineType)
		log.Printf("[DIAG] EXIT: target has anchor, review_id=%d, reply_comment_id=%v, path=%s, line=%d",
			target.ReviewID, event.Comment.Metadata["reply_comment_id"], target.Path, int(ef))
		return nil
	}

	// Step 3: Target has no anchor (position=0, line=0). Trace the InReplyTo chain.
	log.Printf("[DIAG] Step 3: Tracing InReplyTo chain for comment %d", targetID)
	currID := targetID
	visited := make(map[int64]bool)
	effectiveReviewID := target.ReviewID

	for currID != 0 && !visited[currID] {
		visited[currID] = true
		curr, exists := index[currID]
		if !exists {
			log.Printf("[DIAG] Trace broke at comment %d (not in index)", currID)
			break
		}

		// Update effectiveReviewID as we climb the chain (prefer non-zero)
		if curr.ReviewID > 0 {
			effectiveReviewID = curr.ReviewID
		}

		if ef := effectiveLine(curr); ef > 0 {
			event.Comment.Metadata["review_id"] = curr.ReviewID
			if rootCmtID, ok := reviewRootComment[curr.ReviewID]; ok {
				event.Comment.Metadata["reply_comment_id"] = rootCmtID
			}
			event.Comment.Position = &UnifiedPositionV2{
				FilePath:   curr.Path,
				LineNumber: int(ef),
				LineType:   "new",
			}
			log.Printf("[DIAG] EXIT: traced to anchor at comment %d, review_id=%d, reply_comment_id=%v, path=%s, line=%d",
				currID, curr.ReviewID, event.Comment.Metadata["reply_comment_id"], curr.Path, int(ef))
			return nil
		}
		currID = curr.InReplyTo
	}

	// Step 4: Fallback - if no chain found, use the closest review heuristic.
	// We use effectiveReviewID to constrain the search to the correct discussion thread
	// when multiple discussions exist on the same file path.
	log.Printf("[DIAG] Step 4: Fallback to closest-review heuristic for comment %d (effectiveReviewID=%d)", targetID, effectiveReviewID)
	if target.Path != "" {
		type candidate struct {
			cmtID    int64
			reviewID int64
			line     float64
		}
		var candidates []candidate
		for cmtID, e := range index {
			ef := effectiveLine(e)
			// Constraint: same path AND (we don't know the review OR it matches exactly)
			if e.Path == target.Path && ef > 0 {
				if effectiveReviewID == 0 || e.ReviewID == effectiveReviewID {
					candidates = append(candidates, candidate{cmtID, e.ReviewID, ef})
				}
			}
		}
		log.Printf("[DIAG] Candidates with anchor on path=%s and reviewID=%d: %d", target.Path, effectiveReviewID, len(candidates))

		if len(candidates) > 0 {
			// Pick closest review_id ≤ effectiveReviewID (or closest overall if all are later).
			var best *candidate
			for i := range candidates {
				c := &candidates[i]
				if effectiveReviewID == 0 || c.reviewID <= effectiveReviewID {
					if best == nil || c.reviewID > best.reviewID {
						best = c
					}
				}
			}
			if best == nil {
				// Fallback: pick the one with lowest review_id overall
				for i := range candidates {
					c := &candidates[i]
					if best == nil || c.reviewID < best.reviewID {
						best = c
					}
				}
			}
			if best != nil {
				e := index[best.cmtID]
				ef := effectiveLine(e)
				event.Comment.Metadata["review_id"] = e.ReviewID
				if rootCmtID, ok := reviewRootComment[e.ReviewID]; ok {
					event.Comment.Metadata["reply_comment_id"] = rootCmtID
				}
				event.Comment.Position = &UnifiedPositionV2{
					FilePath:   e.Path,
					LineNumber: int(ef),
					LineType:   "new",
				}
				log.Printf("[DIAG] EXIT: fallback to candidate %d, review_id=%d, reply_comment_id=%v, path=%s, line=%d",
					best.cmtID, e.ReviewID, event.Comment.Metadata["reply_comment_id"], e.Path, int(ef))
				return nil
			}
		}
	}

	log.Printf("[DIAG] Comment %d in review but no inline position found → routing as general comment", targetID)
	return nil
}

// fetchCommentFromEndpoint fetches comment details from a given API endpoint.
func (c *APIClient) fetchCommentFromEndpoint(apiURL, token string) (map[string]interface{}, error) {
	req, err := networkgitea.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := c.httpClient
	if client == nil {
		client = networkgitea.NewHTTPClient(30 * time.Second)
	}

	log.Printf("[DIAG] Executing fetch request...")
	resp, err := networkgitea.Do(client, req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] Fetch response: status=%d, content-type=%s, body_len=%d",
		resp.StatusCode, resp.Header.Get("Content-Type"), len(bodyBytes))

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("comment not found (404)")
	}

	if resp.StatusCode != 200 {
		log.Printf("[DEBUG] Fetch failed: status=%d, body=%s", resp.StatusCode, string(bodyBytes)[:min(len(bodyBytes), 300)])
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var comment map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &comment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return comment, nil
}

// populateMetadataFromReviewComment populates event metadata from a pull request review comment.
func (c *APIClient) populateMetadataFromReviewComment(event *UnifiedWebhookEventV2, comment map[string]interface{}) error {
	reviewID, _ := comment["pull_request_review_id"].(float64)
	path, _ := comment["path"].(string)
	line, _ := comment["line"].(float64)
	commitID, _ := comment["commit_id"].(string)

	log.Printf("[DIAG] Parsed review comment: review_id=%.0f, path=%s, line=%.0f, commit=%s",
		reviewID, path, line, commitID[:min(len(commitID), 8)])

	if event.Comment.Metadata == nil {
		event.Comment.Metadata = make(map[string]interface{})
	}
	event.Comment.Metadata["review_id"] = int64(reviewID)
	log.Printf("[DIAG] Populated review_id in metadata: %.0f", reviewID)

	if path != "" && line > 0 {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   path,
			LineNumber: int(line),
			LineType:   "new",
		}
		log.Printf("[DIAG] Populated position: path=%s, line=%.0f", path, line)
	} else {
		log.Printf("[DEBUG] No position info in API response (path=%s, line=%.0f)", path, line)
	}

	log.Printf("[DIAG] enrichCommentMetadata EXIT: SUCCESS (review_id=%.0f, has_position=%v)",
		reviewID, event.Comment.Position != nil)
	return nil
}

// createTemporaryPAT creates a short-lived PAT for API operations using username/password.
func (c *APIClient) createTemporaryPAT(baseURL, username, password string) (string, error) {
	tokenName := fmt.Sprintf("livereview-temp-%d", time.Now().Unix())
	apiURL := fmt.Sprintf("%s/api/v1/users/%s/tokens", baseURL, username)

	log.Printf("[DIAG] Creating temporary PAT: url=%s, token_name=%s", apiURL, tokenName)

	payload := map[string]interface{}{
		"name":   tokenName,
		"scopes": []string{"write:repository"},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := networkgitea.NewRequestWithContext(context.Background(), "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.httpClient
	if client == nil {
		client = networkgitea.NewHTTPClient(30 * time.Second)
		c.httpClient = client
	}

	log.Printf("[DIAG] Executing PAT creation request...")
	resp, err := networkgitea.Do(client, req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] PAT creation response: status=%d, body_len=%d", resp.StatusCode, len(bodyBytes))

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		log.Printf("[ERROR] PAT creation failed: %s", string(bodyBytes)[:min(len(bodyBytes), 500)])
		return "", fmt.Errorf("PAT creation returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		SHA1 string `json:"sha1"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.SHA1 == "" {
		return "", fmt.Errorf("PAT not found in response")
	}

	log.Printf("[DIAG] Temporary PAT created: %s...", result.SHA1[:min(len(result.SHA1), 12)])
	return result.SHA1, nil
}

// PostEmojiReaction posts an emoji reaction to a Gitea comment.
func (c *APIClient) PostEmojiReaction(event *UnifiedWebhookEventV2, token, reaction string) error {
	if event == nil || event.Comment == nil {
		return fmt.Errorf("invalid event for emoji reaction")
	}

	baseURL, err := extractBaseURL(event)
	if err != nil {
		return fmt.Errorf("failed to extract base URL: %w", err)
	}

	token = giteautils.UnpackGiteaPAT(token)
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	commentID := event.Comment.ID

	// Gitea reactions endpoint: POST /api/v1/repos/{owner}/{repo}/issues/comments/{id}/reactions
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/comments/%s/reactions",
		baseURL, owner, repo, commentID)

	// Map common reaction names to Gitea format
	giteaReaction := mapReactionToGitea(reaction)

	requestBody := map[string]string{
		"content": giteaReaction,
	}

	log.Printf("[DEBUG] Gitea API call (emoji reaction): %s", apiURL)
	log.Printf("[DEBUG] Gitea API payload: %+v", requestBody)

	return c.postToGiteaAPI(apiURL, token, requestBody)
}

// PostReviewComments posts the structured review comments collected during processing.
func (c *APIClient) PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	token = giteautils.UnpackGiteaPAT(token)

	// Extract base URL from metadata
	baseURL, ok := mr.Metadata["base_url"].(string)
	if !ok || baseURL == "" {
		return fmt.Errorf("base_url not found in merge request metadata")
	}
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	repoFullName, ok := mr.Metadata["repository_full_name"].(string)
	if !ok || repoFullName == "" {
		return fmt.Errorf("repository_full_name not found in metadata")
	}

	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	headSHA, _ := mr.Metadata["head_sha"].(string)

	for _, comment := range comments {
		apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/comments",
			baseURL, owner, repo, mr.Number)

		// Convert line type to side from Position
		side := "RIGHT"
		if comment.Position != nil && comment.Position.LineType == "old" {
			side = "LEFT"
		}

		requestBody := map[string]interface{}{
			"body": fmt.Sprintf("**%s** (%s)\n\n%s", comment.Severity, comment.Category, comment.Content),
			"path": comment.FilePath,
			"line": comment.LineNumber,
			"side": side,
		}

		if headSHA != "" {
			requestBody["commit_id"] = headSHA
		}

		if err := c.postToGiteaAPI(apiURL, token, requestBody); err != nil {
			if apiErr, ok := err.(*apiError); ok && (apiErr.StatusCode == http.StatusUnprocessableEntity || apiErr.StatusCode == http.StatusBadRequest) {
				log.Printf("[WARN] Skipping Gitea review comment due to %d response (path=%s line=%d): %s",
					apiErr.StatusCode, comment.FilePath, comment.LineNumber, apiErr.Body)
				continue
			}
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}

func (c *APIClient) postToGiteaAPI(apiURL, token string, requestBody interface{}) error {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := networkgitea.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	log.Printf("[DEBUG] Making Gitea API request to: %s", apiURL)

	client := c.httpClient
	if client == nil {
		client = networkgitea.NewHTTPClient(30 * time.Second)
	}

	resp, err := networkgitea.Do(client, req)
	if err != nil {
		log.Printf("[ERROR] Gitea API request failed: %v", err)
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] Gitea API error response (status %d): %s", resp.StatusCode, string(body))
		return &apiError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	log.Printf("[INFO] Successfully posted to Gitea API: %s", apiURL)
	return nil
}

// postInlineCommentReplyMultipart emulates the browser form submission used by Gitea when replying inline.
// Requires username/password (from packed connector) to obtain session + CSRF.
func (c *APIClient) postInlineCommentReplyMultipart(baseURL, owner, repo string, prNumber int, event *UnifiedWebhookEventV2, replyText string, reviewID, replyCommentID int64, username, password string) error {
	log.Printf("[DIAG] postInlineCommentReplyMultipart ENTRY: baseURL=%s, owner=%s, repo=%s, prNumber=%d, reviewID=%d, replyCommentID=%d, replyTextLen=%d",
		baseURL, owner, repo, prNumber, reviewID, replyCommentID, len(replyText))
	log.Printf("[DIAG] Event comment ID=%s, author=%s", event.Comment.ID, event.Comment.Author.Username)
	if username == "" || password == "" {
		return fmt.Errorf("multipart fallback requires username/password in connector token")
	}

	jar, _ := cookiejar.New(nil)
	client := networkgitea.NewHTTPClientWithJar(30*time.Second, jar)

	loginURL := fmt.Sprintf("%s/user/login", baseURL)
	log.Printf("[DIAG] Fetching login page: %s", loginURL)
	resp, err := client.Get(loginURL)
	if err != nil {
		return fmt.Errorf("failed to load login page: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	csrf := extractCSRFToken(string(bodyBytes))
	log.Printf("[DIAG] CSRF from login page HTML: %s (len=%d)", csrf[:min(len(csrf), 16)], len(csrf))
	if csrf == "" {
		// try cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == "_csrf" {
				csrf = ck.Value
				log.Printf("[DIAG] CSRF from cookie: %s (len=%d)", csrf[:min(len(csrf), 16)], len(csrf))
				break
			}
		}
	}
	if csrf == "" {
		return fmt.Errorf("could not obtain CSRF token from login page")
	}

	form := url.Values{}
	form.Set("_csrf", csrf)
	form.Set("user_name", username)
	form.Set("password", password)
	form.Set("remember", "on")

	log.Printf("[DIAG] POSTing login with user=%s, csrf_len=%d", username, len(csrf))
	resp, err = client.PostForm(loginURL, form)
	if err != nil {
		return fmt.Errorf("login post failed: %w", err)
	}
	log.Printf("[DIAG] Login POST response: status=%d, location=%s", resp.StatusCode, resp.Header.Get("Location"))
	resp.Body.Close()

	// Refresh CSRF from cookies after login (Gitea rotates it)
	oldCSRF := csrf
	if newCSRF := extractCSRFFromCookies(client, baseURL); newCSRF != "" {
		csrf = newCSRF
		log.Printf("[DIAG] CSRF refreshed from cookie: old_len=%d, new_len=%d, changed=%v", len(oldCSRF), len(csrf), oldCSRF != csrf)
	} else {
		log.Printf("[DIAG] No CSRF cookie found after login, keeping old: len=%d", len(csrf))
	}
	if csrf == "" {
		return fmt.Errorf("missing CSRF token after login")
	}

	// Position validated in postInlineCommentReply
	line := strconv.Itoa(event.Comment.Position.LineNumber)
	path := event.Comment.Position.FilePath

	// Determine side based on line type: 'previous' for deleted lines, 'proposed' for new lines
	side := "proposed" // default for new lines

	// Debug logging to understand the data structure
	log.Printf("[DEBUG] Comment position data: LineType=%s, LineNumber=%d, FilePath=%s",
		event.Comment.Position.LineType, event.Comment.Position.LineNumber, event.Comment.Position.FilePath)

	if event.Comment.Position.Metadata != nil {
		if originalSide, exists := event.Comment.Position.Metadata["original_side"]; exists {
			log.Printf("[DEBUG] Original Gitea side from metadata: %v", originalSide)
		}
	}

	if event.Comment.Position.LineType == "old" {
		side = "previous" // for deleted lines
		log.Printf("[DEBUG] Detected deleted line, setting side=previous")
	} else {
		log.Printf("[DEBUG] Using default side=proposed for lineType=%s", event.Comment.Position.LineType)
	}

	log.Printf("[DEBUG] Final inline reply: line=%s, path=%s, reviewID=%d, replyCommentID=%d, side=%s, lineType=%s",
		line, path, reviewID, replyCommentID, side, event.Comment.Position.LineType)

	// Build multipart form mirroring the working Python implementation exactly
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fields := map[string]string{
		"_csrf":            csrf,
		"origin":           "timeline",
		"latest_commit_id": "", // Empty string matches working Python implementation
		"side":             side,
		"line":             line,
		"path":             path,
		"diff_start_cid":   "",
		"diff_end_cid":     "",
		"diff_base_cid":    "",
		"content":          replyText,
		"reply":            strconv.FormatInt(reviewID, 10), // Use reviewID, not replyCommentID
		"single_review":    "true",
	}
	for k, v := range fields {
		fw, ferr := mw.CreateFormField(k)
		if ferr != nil {
			return fmt.Errorf("failed to create form field %s: %w", k, ferr)
		}
		if _, ferr = fw.Write([]byte(v)); ferr != nil {
			return fmt.Errorf("failed to write form field %s: %w", k, ferr)
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	commentURL := fmt.Sprintf("%s/%s/%s/pulls/%d/files/reviews/comments", baseURL, owner, repo, prNumber)
	log.Printf("[DIAG] Multipart POST URL: %s", commentURL)
	log.Printf("[DIAG] Multipart body size: %d bytes", buf.Len())
	req, err := networkgitea.NewRequestWithContext(context.Background(), "POST", commentURL, &buf)
	if err != nil {
		return fmt.Errorf("failed to create multipart request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "LiveReview-Bot")
	req.Header.Set("Referer", fmt.Sprintf("%s/%s/%s/pulls/%d/files", baseURL, owner, repo, prNumber))

	log.Printf("[DIAG] Multipart POST headers: Content-Type=%s, CSRF_len=%d, Referer=%s",
		mw.FormDataContentType(), len(csrf), req.Header.Get("Referer"))
	log.Printf("[DIAG] Executing multipart POST request...")
	resp, err = networkgitea.Do(client, req)
	if err != nil {
		return fmt.Errorf("multipart reply request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] Multipart POST response: status=%d, location=%s, content-type=%s, body_len=%d",
		resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Content-Type"), len(body))
	log.Printf("[DIAG] Response body preview: %s", string(body)[:min(len(body), 500)])

	// Check for redirects
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		log.Printf("[DIAG] Redirect detected: %d -> %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[ERROR] Multipart POST failed: status=%d, body=%s", resp.StatusCode, string(body)[:min(len(body), 1000)])
		return &apiError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	log.Printf("[DIAG] postInlineCommentReplyMultipart EXIT: SUCCESS")
	return nil
}

var csrfPattern = regexp.MustCompile(`name="_csrf"\s+value="([^"]+)"`)

func extractCSRFToken(html string) string {
	m := csrfPattern.FindStringSubmatch(html)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// extractCSRFFromCookies returns the _csrf cookie value if present after auth.
func extractCSRFFromCookies(client *http.Client, baseURL string) string {
	if client == nil || client.Jar == nil {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	for _, ck := range client.Jar.Cookies(u) {
		if ck.Name == "_csrf" && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

// extractBaseURL extracts the Gitea base URL from event metadata or repository URL
func extractBaseURL(event *UnifiedWebhookEventV2) (string, error) {
	// Try to get from merge request metadata first
	if event.MergeRequest != nil && event.MergeRequest.Metadata != nil {
		if baseURL, ok := event.MergeRequest.Metadata["base_url"].(string); ok && baseURL != "" {
			return baseURL, nil
		}
	}

	// Try to extract from repository web URL
	if event.Repository.WebURL != "" {
		// Extract base URL from web URL (e.g., https://gitea.hexmos.site/owner/repo -> https://gitea.hexmos.site)
		parts := strings.Split(event.Repository.WebURL, "/")
		if len(parts) >= 3 {
			// parts: [https:] [] [host] [owner] [repo]
			baseURL := strings.Join(parts[:3], "/")
			return baseURL, nil
		}
	}

	return "", fmt.Errorf("could not extract base URL from event")
}

// mapReactionToGitea maps common emoji names to Gitea reaction format
func mapReactionToGitea(reaction string) string {
	// Gitea supports: +1, -1, laugh, hooray, confused, heart, rocket, eyes
	reactionMap := map[string]string{
		"thumbsup":   "+1",
		"+1":         "+1",
		"thumbsdown": "-1",
		"-1":         "-1",
		"laugh":      "laugh",
		"hooray":     "hooray",
		"confused":   "confused",
		"heart":      "heart",
		"rocket":     "rocket",
		"eyes":       "eyes",
	}

	if mapped, ok := reactionMap[strings.ToLower(reaction)]; ok {
		return mapped
	}

	// Return as-is if not in map
	return reaction
}
