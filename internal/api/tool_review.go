package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	reviewpkg "github.com/livereview/internal/review"
	storagetools "github.com/livereview/storage/tools"
)

// CreateToolReview handles POST /api/v1/reviews/tool-reviews
func (s *Server) CreateToolReview(c echo.Context) error {
	// Gated to cloud-mode only
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Tool reviews endpoint is only available in cloud mode",
		})
	}

	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	userEmail := ""
	if pc.User != nil {
		userEmail = pc.User.Email
	}

	type RequestBody struct {
		PRURL string `json:"pr_url"`
	}

	var req RequestBody
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.PRURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "pr_url is required"})
	}

	// Auto-detect connector from PR URL (same as AI review TriggerReviewV2)
	_, baseURL, err := validateAndParseURL(req.PRURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid PR URL: %v", err)})
	}

	token, err := s.findIntegrationToken(baseURL, orgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	provider := token.Provider
	connectorID := token.ID

	// Resolve the actual access token
	var actualToken string
	if token.TokenType == "PAT" && token.PatToken != "" {
		actualToken = token.PatToken
	} else {
		actualToken = token.AccessToken
	}

	// Create review row with trigger_type = 'tool_review'
	reviewManager := NewReviewManager(s.db)
	review, err := reviewManager.CreateReviewWithOrg(
		req.PRURL,     // repository
		"",            // branch
		"",            // commit_hash
		req.PRURL,     // pr_mr_url
		"tool_review", // trigger_type
		userEmail,
		provider,
		&connectorID,
		map[string]interface{}{"triggered_from": "tool_review"},
		orgID,
		"", "", "", // friendlyName, authorName, authorUsername
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create review: %v", err)})
	}

	// Mark status as in progress immediately
	_ = reviewManager.UpdateReviewStatus(review.ID, "in_progress")

	// Trigger diff extraction and tools queuing in the background to prevent HTTP timeout
	go func() {
		// Create standard provider factory to fetch changes
		providerFactory := reviewpkg.NewStandardProviderFactory()

		providerConfigMap := map[string]interface{}{}
		if token.TokenType == "PAT" && token.PatToken != "" {
			providerConfigMap["pat_token"] = token.PatToken
			if strings.HasPrefix(provider, "bitbucket") {
				providerConfigMap["repo_url"] = req.PRURL
			}
		}

		provConfig := reviewpkg.ProviderConfig{
			Type:   provider,
			URL:    token.ProviderURL,
			Token:  actualToken,
			Config: providerConfigMap,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		providerInstance, err := providerFactory.CreateProvider(ctx, provConfig)
		if err != nil {
			log.Printf("[ERROR] ToolReview Background: Failed to create provider: %v", err)
			_ = reviewManager.UpdateReviewStatus(review.ID, "failed")
			return
		}

		// Parse PR URL to get PR ID for GitLab/GitHub
		prID := fmt.Sprintf("%d", review.ID) // default fallback
		parsedURL, err := url.Parse(req.PRURL)
		if err == nil {
			parts := strings.Split(parsedURL.Path, "/")
			if strings.HasPrefix(provider, "github") {
				if len(parts) >= 5 && parts[3] == "pull" {
					prID = parts[1] + "/" + parts[2] + "/" + parts[4]
				}
			} else if strings.HasPrefix(provider, "bitbucket") {
				if len(parts) >= 5 && parts[3] == "pull-requests" {
					prID = parts[1] + "/" + parts[2] + "/" + parts[4]
				}
			}
		}

		// Let's call GetMergeRequestDetails first to get metadata and correct MR ID if possible
		mrDetails, err := providerInstance.GetMergeRequestDetails(ctx, req.PRURL)
		if err == nil && mrDetails != nil {
			prID = mrDetails.ID
			if provider == "github" {
				u, parseErr := url.Parse(mrDetails.URL)
				if parseErr == nil {
					parts := strings.Split(u.Path, "/")
					if len(parts) >= 5 && parts[3] == "pull" {
						prID = parts[1] + "/" + parts[2] + "/" + parts[4]
					}
				}
			} else if provider == "bitbucket" {
				u, parseErr := url.Parse(mrDetails.URL)
				if parseErr == nil {
					parts := strings.Split(u.Path, "/")
					if len(parts) >= 5 && parts[3] == "pull-requests" {
						prID = parts[1] + "/" + parts[2] + "/" + parts[4]
					}
				}
			}
			// Update metadata with actual MR info
			metaUpdate := ReviewMetadataUpdate{}
			if mrDetails.RepositoryURL != "" {
				metaUpdate.Repository = &mrDetails.RepositoryURL
			}
			if mrDetails.SourceBranch != "" {
				metaUpdate.Branch = &mrDetails.SourceBranch
			}
			if mrDetails.Title != "" {
				metaUpdate.MRTitle = &mrDetails.Title
			}
			authorName := mrDetails.AuthorName
			if authorName == "" {
				authorName = mrDetails.Author
			}
			if authorName != "" {
				metaUpdate.AuthorName = &authorName
			}
			authorUsername := mrDetails.AuthorUsername
			if authorUsername == "" {
				authorUsername = mrDetails.Author
			}
			if authorUsername != "" {
				metaUpdate.AuthorUsername = &authorUsername
			}
			_ = reviewManager.UpdateReviewMetadata(review.ID, metaUpdate)
		}

		changes, err := providerInstance.GetMergeRequestChanges(ctx, prID)
		if err != nil {
			log.Printf("[ERROR] ToolReview Background: Failed to get changes: %v", err)
			_ = reviewManager.UpdateReviewStatus(review.ID, "failed")
			return
		}

		// Format diff text
		rawDiff := reviewpkg.FormatDiffs(changes)
		if rawDiff != "" {
			_, err = s.db.Exec(`UPDATE public.reviews SET diff = $1 WHERE id = $2`, rawDiff, review.ID)
			if err != nil {
				log.Printf("[ERROR] ToolReview Background: Failed to save diff: %v", err)
				_ = reviewManager.UpdateReviewStatus(review.ID, "failed")
				return
			}
		} else {
			log.Printf("[WARN] ToolReview Background: Empty diff for review %d", review.ID)
			_ = reviewManager.UpdateReviewStatus(review.ID, "completed")
			return
		}

		// Fetch enabled tools and queue River jobs
		toolsStore := storagetools.NewToolsStore(s.db)
		enabledTools, err := toolsStore.GetEnabledToolsForOrg(ctx, orgID)
		if err != nil {
			log.Printf("[ERROR] ToolReview Background: Failed to fetch enabled tools: %v", err)
			_ = reviewManager.UpdateReviewStatus(review.ID, "failed")
			return
		}

		if len(enabledTools) == 0 {
			log.Printf("[INFO] ToolReview Background: No enabled tools for org %d", orgID)
			_ = reviewManager.UpdateReviewStatus(review.ID, "completed")
			return
		}

		var totalMultiplier float64
		for _, t := range enabledTools {
			totalMultiplier += t.Multiplier
			err := s.jobQueue.QueueToolInvocationJob(
				ctx,
				review.ID,
				orgID,
				t.ID,
				t.Name,
				t.LambdaARN,
				review.PrMrURL,
				connectorID,
				provider,
			)
			if err != nil {
				log.Printf("[ERROR] ToolReview Background: Failed to queue job for tool %s: %v", t.Name, err)
			}
		}

		if totalMultiplier > 0 {
			err = reviewManager.MergeReviewMetadata(review.ID, map[string]interface{}{
				"multiplier_used": totalMultiplier,
			})
			if err != nil {
				log.Printf("[ERROR] ToolReview Background: Failed to save multiplier for review %d: %v", review.ID, err)
			}
		}

		// Update review status to completed
		_ = reviewManager.UpdateReviewStatus(review.ID, "completed")
	}()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"reviewId": review.ID,
		"message":  "Tool review triggered successfully",
	})
}

