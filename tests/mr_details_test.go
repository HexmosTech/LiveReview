package livereview_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/livereview/internal/config"
	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestFetchMR335Details(t *testing.T) {
	// Load configuration from livereview.toml
	// Get the path to livereview.toml from project root
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filename))
	configPath := filepath.Join(projectRoot, "livereview.toml")

	cfg, err := config.LoadConfig(configPath)
	assert.NoError(t, err, "Failed to load config")

	// Get GitLab configuration
	gitlabCfg, ok := cfg.Providers["gitlab"]
	assert.True(t, ok, "GitLab provider not found in config")

	url, ok := gitlabCfg["url"].(string)
	assert.True(t, ok, "GitLab URL not found in config")

	token, ok := gitlabCfg["token"].(string)
	assert.True(t, ok, "GitLab token not found in config")

	// Create a GitLab provider with the configuration from the file
	provider, err := gitlab.New(gitlab.GitLabConfig{
		URL:   url,
		Token: token,
	})
	assert.NoError(t, err)

	// The MR URL for MR 335
	mrURL := "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/335"

	// Create test context
	ctx := context.Background()

	// Step 1: Get MR Details
	fmt.Println("Fetching details for MR 335...")
	mrDetails, err := provider.GetMergeRequestDetails(ctx, mrURL)
	assert.NoError(t, err)

	// Print the MR details
	fmt.Println("MR Details:")
	fmt.Printf("  ID: %s\n", mrDetails.ID)
	fmt.Printf("  Project ID: %s\n", mrDetails.ProjectID)
	fmt.Printf("  Title: %s\n", mrDetails.Title)
	fmt.Printf("  Description: %s\n", mrDetails.Description)
	fmt.Printf("  Source Branch: %s\n", mrDetails.SourceBranch)
	fmt.Printf("  Target Branch: %s\n", mrDetails.TargetBranch)
	fmt.Printf("  Author: %s\n", mrDetails.Author)
	fmt.Printf("  State: %s\n", mrDetails.State)
	fmt.Printf("  URL: %s\n", mrDetails.URL)

	// Step 2: Get MR Changes
	fmt.Println("\nFetching changes for MR 335...")
	changes, err := provider.GetMergeRequestChanges(ctx, mrDetails.ID)
	assert.NoError(t, err)

	// Print the changes summary
	fmt.Printf("Found %d changed files:\n", len(changes))
	for i, change := range changes {
		fmt.Printf("  %d. %s\n", i+1, change.FilePath)
		if change.IsNew {
			fmt.Println("     (New file)")
		}
		if change.IsDeleted {
			fmt.Println("     (Deleted file)")
		}
		if change.IsRenamed {
			fmt.Printf("     (Renamed from: %s)\n", change.OldFilePath)
		}

		// Print a sample of the diff (first hunk only, truncated)
		if len(change.Hunks) > 0 {
			diffContent := change.Hunks[0].Content
			if len(diffContent) > 200 {
				diffContent = diffContent[:200] + "...[truncated]"
			}
			fmt.Printf("     Diff sample: %s\n", diffContent)
		}
	}

	// Step 3: Create a result structure to hold the merge request details and changes
	// This could be passed to an AI provider for review
	mrContext := struct {
		ProjectID    string
		MergeReqID   string
		Title        string
		Description  string
		SourceBranch string
		TargetBranch string
		Diffs        []*models.CodeDiff
	}{
		ProjectID:    mrDetails.ProjectID,
		MergeReqID:   mrDetails.ID,
		Title:        mrDetails.Title,
		Description:  mrDetails.Description,
		SourceBranch: mrDetails.SourceBranch,
		TargetBranch: mrDetails.TargetBranch,
		Diffs:        changes,
	}

	// Print MR context summary
	fmt.Println("\nMerge Request Context (ready for AI review):")
	fmt.Printf("  Title: %s\n", mrContext.Title)
	fmt.Printf("  Description: %s\n", mrContext.Description)
	fmt.Printf("  Source Branch: %s\n", mrContext.SourceBranch)
	fmt.Printf("  Target Branch: %s\n", mrContext.TargetBranch)
	fmt.Printf("  Number of files: %d\n", len(mrContext.Diffs))

	// Step 4: Verify that we could extract all necessary information
	assert.NotEmpty(t, mrDetails.ID)
	assert.NotEmpty(t, mrDetails.Title)
	assert.NotEmpty(t, mrDetails.ProjectID)
	assert.NotEmpty(t, changes)
}
