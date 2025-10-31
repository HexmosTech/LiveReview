
## Where is Prompt Created

1. Where is prompt created in unified_context_v2.go?
BuildPrompt() method (line 191) creates a prompt, BUT this is NOT used for comment replies. It's a generic method that builds prompts from CommentContextV2 and ResponseScenarioV2.

2. Actual prompt creation for replies:
The actual prompt used for GitLab comment replies is in unified_processor_v2.go:

buildCommentReplyPromptWithLearning() (line 254) - This creates the LLM prompt
Called by buildContextualResponseWithLearningV2() (line 336)
Which is called by ProcessCommentReply() (line 248)
3. Does it have MR URL available?
NO. The prompt only has:

event.Repository.Name
event.MergeRequest (if not nil) - but looking at the UnifiedWebhookEventV2 structure, this would have the MR metadata, not necessarily the URL
timeline data
event.Comment data
4. Where is MR URL and credentials available?
In webhook_orchestrator_v2.go:

The event object has event.Repository and event.MergeRequest
The provider object has access to credentials (it can fetch data)
BUT the MR URL is not directly available in the unified event structure
In unified_processor_v2.go:

Has p.server which has database access
Can look up tokens via p.server.githubProviderV2, p.server.bitbucketProviderV2, etc.
But again, MR URL is not readily available in the event object

## How MR URL can be constructed

How Reply Posting Works:
In PostCommentReply() (gitlab_provider_v2.go, line 886):

Gets event.Repository.WebURL → extracts GitLab instance URL
Gets event.Repository.ID → project ID
Gets event.MergeRequest.Number → MR IID
Gets event.Comment.DiscussionID → discussion ID (if replying to thread)
In APIClient.PostCommentReply() (api_client.go, line 32):

Uses event.Repository.ID (project ID)
Uses event.MergeRequest.Number (MR number/IID)
Uses event.Comment.DiscussionID (if replying to discussion)
Constructs API URLs like:
{gitlabInstanceURL}/api/v4/projects/{projectID}/merge_requests/{mrIID}/discussions/{discussionID}/notes
So the answer is:
YES, the MR URL context IS available - it's embedded in the UnifiedWebhookEventV2:

event.Repository.WebURL - full repository URL
event.Repository.ID - project ID
event.MergeRequest.Number - MR number
event.MergeRequest.ID - MR ID
These same fields can be used to construct the MR URL to pass to mrmodel's BuildGitLabUnifiedArtifact() method!

For GitLab specifically:

MR URL = {event.Repository.WebURL}/-/merge_requests/{event.MergeRequest.Number}
Credentials are fetched via getGitLabAccessTokenV2(gitlabInstanceURL) from the database

## Comment File and Position Information Available in the same event object like:

OK for a reply - comment - unless it is general comment - the file and line numbers, etc must also be available, right. I think UnifedPositionV2 or event.Position will contain it

## How to get the UnifedArtifact from mrmodel?

Refer to ./cmd/mrmodel/cli.go - we call each provider to get artifact

## How to get filewise comments?

There are functions in cmd/mrmodel/batch/lib/mrmodel_batch.go to get comment tree. But we may need more
work to get the diff hunks for a given file as well.  

Once all the above info is collected a nuanced prompt can be generated I think.

---

# DETAILED STEP-BY-STEP IMPLEMENTATION PLAN
## Goal: GitLab Reply with Contextual Prompts using mrmodel

**Two-Iteration Approach:**
- **Iteration 1:** Get artifact, convert to basic text context, submit to LLM (WORKING FIRST)
- **Iteration 2:** Add intelligent file/line-based filtering

---

# ITERATION 1: Basic Context (Get It Working First)

**Goal:** Build UnifiedArtifact and add basic MR context to prompt (all diffs + all comments)

## PHASE 1: Build Artifact and Add to Prompt

**File:** `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`

**Step 1.1:** Add imports (at top of file)
```go
mrmodel "github.com/livereview/cmd/mrmodel/lib"
gl "github.com/livereview/internal/providers/gitlab"
```

**Step 1.2:** Add helper method `buildGitLabArtifactFromEvent(event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error)`
- Location: Add as private method in `UnifiedProcessorV2Impl`, after other helper methods
- Copy pattern from cli.go:
  ```go
  func (p *UnifiedProcessorV2Impl) buildGitLabArtifactFromEvent(event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error) {
      // Extract GitLab instance URL from repository web URL
      gitlabInstanceURL := event.Repository.WebURL
      // Remove project path to get base URL (everything before the project)
      if idx := strings.LastIndex(gitlabInstanceURL, "/"); idx != -1 {
          parts := strings.Split(gitlabInstanceURL, "/")
          if len(parts) >= 3 {
              gitlabInstanceURL = strings.Join(parts[:3], "/") // https://domain.com
          }
      }
      
      // Get token from database
      var token string
      query := `SELECT pat_token FROM integration_tokens WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') AND RTRIM(provider_url, '/') = RTRIM($1, '/') LIMIT 1`
      err := p.server.db.QueryRow(query, gitlabInstanceURL).Scan(&token)
      if err != nil {
          return nil, fmt.Errorf("no access token found for GitLab instance %s: %w", gitlabInstanceURL, err)
      }
      
      // Construct MR URL
      mrURL := fmt.Sprintf("%s/-/merge_requests/%d", event.Repository.WebURL, event.MergeRequest.Number)
      
      // Create GitLab provider (exactly like cli.go)
      cfg := gl.GitLabConfig{URL: gitlabInstanceURL, Token: token}
      provider, err := gl.New(cfg)
      if err != nil {
          return nil, fmt.Errorf("failed to init gitlab provider: %w", err)
      }
      
      // Create mrmodel instance (exactly like cli.go)
      mrModel := &mrmodel.MrModelImpl{EnableArtifactWriting: false}
      
      // Fetch data (exactly like cli.go)
      _, diffs, commits, discussions, standaloneNotes, err := mrModel.FetchGitLabData(provider, mrURL)
      if err != nil {
          return nil, fmt.Errorf("failed to fetch GitLab data: %w", err)
      }
      
      // Build artifact (exactly like cli.go) - empty string for outDir
      unifiedArtifact, err := mrModel.BuildGitLabUnifiedArtifact(commits, discussions, standaloneNotes, diffs, "")
      if err != nil {
          return nil, fmt.Errorf("failed to build unified artifact: %w", err)
      }
      
      log.Printf("[DEBUG] Built artifact: %d diffs, %d timeline items, %d comment roots", 
          len(unifiedArtifact.Diffs), len(unifiedArtifact.Timeline), len(unifiedArtifact.CommentTree.Roots))
      
      return unifiedArtifact, nil
  }
  ```

**Step 1.3:** Add basic artifact formatter `formatArtifactForPrompt(artifact *mrmodel.UnifiedArtifact) string`
- Location: After `buildGitLabArtifactFromEvent()`
- Simple text conversion with basic heuristics:
  ```go
  func (p *UnifiedProcessorV2Impl) formatArtifactForPrompt(artifact *mrmodel.UnifiedArtifact) string {
      if artifact == nil {
          return ""
      }
      
      var b strings.Builder
      
      b.WriteString("\n=== MERGE REQUEST CONTEXT ===\n\n")
      
      // Summary stats
      b.WriteString(fmt.Sprintf("Files changed: %d\n", len(artifact.Diffs)))
      b.WriteString(fmt.Sprintf("Discussion threads: %d\n", len(artifact.CommentTree.Roots)))
      b.WriteString(fmt.Sprintf("Timeline items: %d\n\n", len(artifact.Timeline)))
      
      // List changed files
      if len(artifact.Diffs) > 0 {
          b.WriteString("Changed files:\n")
          for i, diff := range artifact.Diffs {
              if i >= 20 { // Limit to 20 files max
                  b.WriteString(fmt.Sprintf("... and %d more files\n", len(artifact.Diffs)-20))
                  break
              }
              b.WriteString(fmt.Sprintf("  - %s\n", diff.NewPath))
          }
          b.WriteString("\n")
      }
      
      // Show diffs (limit size)
      b.WriteString("Code changes:\n```diff\n")
      totalLines := 0
      for _, diff := range artifact.Diffs {
          for _, hunk := range diff.Hunks {
              // Add file header
              b.WriteString(fmt.Sprintf("--- %s\n+++ %s\n", diff.OldPath, diff.NewPath))
              b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
                  hunk.OldStartLine, hunk.OldLineCount,
                  hunk.NewStartLine, hunk.NewLineCount))
              
              for _, line := range hunk.Lines {
                  b.WriteString(line.Content + "\n")
                  totalLines++
                  if totalLines > 200 { // Limit to 200 lines of diff
                      b.WriteString("... (diff truncated for brevity)\n")
                      goto endDiff
                  }
              }
          }
      }
      endDiff:
      b.WriteString("```\n\n")
      
      // Show comment threads (basic format)
      if len(artifact.CommentTree.Roots) > 0 {
          b.WriteString("Discussion threads:\n")
          threadCount := 0
          for _, root := range artifact.CommentTree.Roots {
              threadCount++
              if threadCount > 30 { // Limit to 30 threads
                  b.WriteString(fmt.Sprintf("... and %d more threads\n", len(artifact.CommentTree.Roots)-30))
                  break
              }
              b.WriteString(p.formatCommentThreadBasic(root, 0))
          }
      }
      
      return b.String()
  }
  
  func (p *UnifiedProcessorV2Impl) formatCommentThreadBasic(node *mrmodel.CommentNode, depth int) string {
      var b strings.Builder
      indent := strings.Repeat("  ", depth)
      
      author := node.Author.Username
      if author == "" {
          author = "unknown"
      }
      
      marker := "-"
      if depth > 0 {
          marker = "↳"
      }
      
      location := ""
      if node.FilePath != "" {
          location = fmt.Sprintf(" [%s:%d]", node.FilePath, node.LineNew)
      }
      
      // Truncate long comments
      body := node.Body
      if len(body) > 200 {
          body = body[:197] + "..."
      }
      
      b.WriteString(fmt.Sprintf("%s%s [%s]%s: %s\n", indent, marker, author, location, body))
      
      // Recursively format children
      for _, child := range node.Children {
          b.WriteString(p.formatCommentThreadBasic(child, depth+1))
      }
      
      return b.String()
  }
  ```

**Step 1.4:** Modify `ProcessCommentReply()` to build and use artifact
- Location: Line 240, at the beginning
- Add:
  ```go
  // Build UnifiedArtifact for contextual information (GitLab only for now)
  var artifact *mrmodel.UnifiedArtifact
  if event.Provider == "gitlab" && event.MergeRequest != nil {
      var err error
      artifact, err = p.buildGitLabArtifactFromEvent(event)
      if err != nil {
          log.Printf("[WARN] Failed to build GitLab artifact for context: %v", err)
          // Continue without artifact - better to reply without context than not reply
      }
  }
  ```
- Pass to next function:
  ```go
  response, learning := p.buildContextualResponseWithLearningV2(ctx, event, timeline, orgID, artifact)
  ```

**Step 1.5:** Update `buildContextualResponseWithLearningV2()` signature (line 336)
- Add parameter: `, artifact *mrmodel.UnifiedArtifact`
- Pass to prompt builder:
  ```go
  prompt := p.buildCommentReplyPromptWithLearning(event, timeline, artifact)
  ```

**Step 1.6:** Update `buildCommentReplyPromptWithLearning()` signature (line 254)
- Add parameter: `, artifact *mrmodel.UnifiedArtifact`

**Step 1.7:** Add artifact context to prompt in `buildCommentReplyPromptWithLearning()`
- Location: After the repository/MR context (around line 268), before timeline
- Add:
  ```go
  // Add artifact context if available
  if artifact != nil {
      artifactContext := p.formatArtifactForPrompt(artifact)
      if artifactContext != "" {
          prompt.WriteString(artifactContext)
      }
  }
  ```

---

## PHASE 2: Testing Iteration 1

**Step 2.1:** Build
```bash
bash -lc 'go build livereview.go'
```

**Step 2.2:** Test with GitLab
- Post a comment on GitLab MR
- Check `debug_prompt.txt` should show:
  - "=== MERGE REQUEST CONTEXT ==="
  - List of changed files
  - Diff hunks (truncated)
  - Discussion threads

**Step 2.3:** Verify bot reply uses context
- Bot should have awareness of other files/comments in MR
- Reply should be more contextually relevant

---

# ITERATION 2: Intelligent File-Based Context

**Goal:** Filter artifact by file/line to show only relevant context

## PHASE 3: Add Smart Context Extraction

**File:** `/home/shrsv/bin/LiveReview/cmd/mrmodel/lib/mrmodel_batch.go`

**Step 3.1:** Add FileContext struct (after FileCommentTree, around line 19)
```go
// FileContext holds file-specific context for prompts
type FileContext struct {
    FilePath     string
    Diff         *LocalCodeDiff
    CommentRoots []*reviewmodel.CommentNode
}
```

**Step 3.2:** Add `ExtractFileContext()` (after `BuildFileCommentTree()`)
```go
// ExtractFileContext extracts diff and comments for a specific file
func ExtractFileContext(artifact *UnifiedArtifact, filePath string) *FileContext {
    if artifact == nil || filePath == "" {
        return nil
    }
    
    // Find the diff for this file
    var targetDiff *LocalCodeDiff
    for _, diff := range artifact.Diffs {
        if diff.NewPath == filePath {
            targetDiff = diff
            break
        }
    }
    
    // Get comment tree for this file
    commentTree := BuildFileCommentTree(artifact)
    commentRoots := commentTree[filePath]
    
    return &FileContext{
        FilePath:     filePath,
        Diff:         targetDiff,
        CommentRoots: commentRoots,
    }
}
```

**Step 3.3:** Add `FormatFileContextForPrompt()`
```go
// FormatFileContextForPrompt formats file context for LLM prompt
func FormatFileContextForPrompt(ctx *FileContext) string {
    if ctx == nil {
        return ""
    }
    
    var b strings.Builder
    b.WriteString(fmt.Sprintf("\n=== FILE CONTEXT: %s ===\n\n", ctx.FilePath))
    
    // Format diff
    if ctx.Diff != nil && len(ctx.Diff.Hunks) > 0 {
        b.WriteString("Code changes in this file:\n```diff\n")
        for _, hunk := range ctx.Diff.Hunks {
            b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
                hunk.OldStartLine, hunk.OldLineCount,
                hunk.NewStartLine, hunk.NewLineCount))
            for _, line := range hunk.Lines {
                b.WriteString(line.Content + "\n")
            }
        }
        b.WriteString("```\n\n")
    }
    
    // Format comment threads for this file
    if len(ctx.CommentRoots) > 0 {
        b.WriteString(fmt.Sprintf("Discussion threads on this file (%d):\n", len(ctx.CommentRoots)))
        for _, root := range ctx.CommentRoots {
            b.WriteString(formatCommentForPrompt(root, 0))
        }
    }
    
    return b.String()
}

// formatCommentForPrompt formats a comment node recursively
func formatCommentForPrompt(node *reviewmodel.CommentNode, depth int) string {
    var b strings.Builder
    
    indent := strings.Repeat("  ", depth)
    author := node.Author.Username
    if author == "" {
        author = "unknown"
    }
    
    marker := "-"
    if depth > 0 {
        marker = "↳"
    }
    
    b.WriteString(fmt.Sprintf("%s%s [%s] at line %d: %s\n",
        indent, marker, author, node.LineNew, node.Body))
    
    // Recursively format children
    for _, child := range node.Children {
        b.WriteString(formatCommentForPrompt(child, depth+1))
    }
    
    return b.String()
}
```

---

## PHASE 4: Use Smart Context in Prompt

**File:** `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`

**Step 4.1:** Replace basic context with smart context in `buildCommentReplyPromptWithLearning()`
- Replace the artifact formatting code from Step 1.7 with:
  ```go
  // Add smart context based on comment location
  if artifact != nil {
      // If comment is on a specific file, show file-specific context
      if event.Comment != nil && event.Comment.Position != nil && event.Comment.Position.FilePath != "" {
          fileCtx := mrmodel.ExtractFileContext(artifact, event.Comment.Position.FilePath)
          if fileCtx != nil {
              contextStr := mrmodel.FormatFileContextForPrompt(fileCtx)
              if contextStr != "" {
                  prompt.WriteString(contextStr)
              }
          }
      } else {
          // For general comments, use basic full MR context
          artifactContext := p.formatArtifactForPrompt(artifact)
          if artifactContext != "" {
              prompt.WriteString(artifactContext)
          }
      }
  }
  ```

---

## PHASE 5: Testing Iteration 2

**Step 5.1:** Test file-specific comment
- Post comment on specific line in a file
- Verify `debug_prompt.txt` shows:
  - "=== FILE CONTEXT: {filename} ==="
  - Only that file's diff
  - Only that file's comment threads

**Step 5.2:** Test general comment
- Post general MR comment (not on specific line)
- Verify shows full MR context (like Iteration 1)

---

## Summary

**Iteration 1 (Basic - Get Working First):**
- File: `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`
  - Import mrmodel lib
  - Add `buildGitLabArtifactFromEvent()` (copy from cli.go pattern)
  - Add `formatArtifactForPrompt()` (basic text conversion)
  - Add `formatCommentThreadBasic()` (simple formatter)
  - Modify `ProcessCommentReply()` to build artifact
  - Update signatures to pass artifact through
  - Inject basic context into prompt

**Iteration 2 (Smart - Add Intelligence):**
- File: `/home/shrsv/bin/LiveReview/cmd/mrmodel/lib/mrmodel_batch.go`
  - Add `FileContext` struct
  - Add `ExtractFileContext()` 
  - Add `FormatFileContextForPrompt()`
  - Add `formatCommentForPrompt()`
- File: `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`
  - Replace basic context with smart file-based filtering