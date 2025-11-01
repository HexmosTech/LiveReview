
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

**Step 1.2:** Add helper method `buildGitLabArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error)`
- Location: Add as private method in `UnifiedProcessorV2Impl`, after other helper methods
- Copy pattern from cli.go:
  ```go
  func (p *UnifiedProcessorV2Impl) buildGitLabArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error) {
      if event.Repository.WebURL == "" || event.MergeRequest == nil {
          return nil, fmt.Errorf("missing required fields for MR URL construction")
      }
      
      // Get MR number - prioritize Number field (IID), fallback to ID
      var mrNumber string
      if event.MergeRequest.Number > 0 {
          mrNumber = fmt.Sprintf("%d", event.MergeRequest.Number)
      } else if event.MergeRequest.ID != "" {
          mrNumber = event.MergeRequest.ID
      } else {
          return nil, fmt.Errorf("missing MR number/ID")
      }
      
      // Construct MR URL from event
      mrURL := event.Repository.WebURL + "/-/merge_requests/" + mrNumber
      log.Printf("[DEBUG] Constructed MR URL: %s", mrURL)
      
      // Extract GitLab base URL from repository URL
      // event.Repository.WebURL is like "https://git.apps.hexmos.com/hexmos/liveapi"
      // We need just "https://git.apps.hexmos.com"
      var gitlabBaseURL string
      if idx := strings.Index(event.Repository.WebURL, "://"); idx != -1 {
          // Find the first slash after the protocol
          remaining := event.Repository.WebURL[idx+3:]
          if slashIdx := strings.Index(remaining, "/"); slashIdx != -1 {
              gitlabBaseURL = event.Repository.WebURL[:idx+3+slashIdx]
          } else {
              gitlabBaseURL = event.Repository.WebURL
          }
      } else {
          return nil, fmt.Errorf("invalid repository URL format: %s", event.Repository.WebURL)
      }
      
      gitlabBaseURL = strings.TrimRight(gitlabBaseURL, "/")
      log.Printf("[DEBUG] Extracted GitLab base URL: %s", gitlabBaseURL)
      
      // Look up GitLab PAT from integration_tokens table using base URL
      // IMPORTANT: Include 'gitlab-self-hosted' in provider list
      query := `SELECT pat_token FROM integration_tokens 
                WHERE provider IN ('gitlab', 'GitLab', 'gitlab-self-hosted') 
                AND RTRIM(provider_url, '/') = $1 
                LIMIT 1`
      
      var patToken string
      err := p.server.db.QueryRow(query, gitlabBaseURL).Scan(&patToken)
      if err != nil {
          return nil, fmt.Errorf("failed to find GitLab PAT for %s: %w", gitlabBaseURL, err)
      }
      
      log.Printf("[DEBUG] Found GitLab PAT for %s", gitlabBaseURL)
      
      // Create GitLab provider (following cli.go pattern)
      cfg := gl.GitLabConfig{URL: gitlabBaseURL, Token: patToken}
      provider, err := gl.New(cfg)
      if err != nil {
          return nil, fmt.Errorf("failed to init gitlab provider: %w", err)
      }
      
      // Create mrModel instance
      mrModel := &mrmodel.MrModelImpl{}
      mrModel.EnableArtifactWriting = false // Don't write to disk
      
      // Fetch GitLab data (following cli.go pattern)
      _, diffs, commits, discussions, standaloneNotes, err := mrModel.FetchGitLabData(provider, mrURL)
      if err != nil {
          return nil, fmt.Errorf("failed to fetch GitLab data: %w", err)
      }
      
      log.Printf("[DEBUG] Fetched GitLab data: commits=%d discussions=%d notes=%d diffs=%d",
          len(commits), len(discussions), len(standaloneNotes), len(diffs))
      
      // Build unified artifact (following cli.go pattern)
      artifact, err := mrModel.BuildGitLabUnifiedArtifact(commits, discussions, standaloneNotes, diffs, "")
      if err != nil {
          return nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
      }
      
      return artifact, nil
  }
  ```

**KEY LEARNINGS:**
1. **MR Number vs ID**: `event.MergeRequest.Number` is an `int` (IID), not a string. Format it with `fmt.Sprintf("%d", ...)` for URL construction.
2. **Base URL Extraction**: Repository WebURL contains full path (`https://git.apps.hexmos.com/hexmos/liveapi`). Must extract base URL (`https://git.apps.hexmos.com`) by finding first slash after protocol.
3. **Provider Name in DB**: Self-hosted GitLab stores as `'gitlab-self-hosted'`, not `'gitlab'`. Always include all variants in query: `('gitlab', 'GitLab', 'gitlab-self-hosted')`.
4. **Query Pattern**: Use `RTRIM(provider_url, '/') = $1` to handle trailing slash differences.
5. **Context Parameter**: Pass `ctx context.Context` as first parameter for consistency with other methods.

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
      
**Step 1.4:** Modify `ProcessCommentReply()` to build and use artifact
- Location: Line 240, at the beginning
- Add:
  ```go
  // Build GitLab artifact for context (Phase 1 - Iteration 1)
  var artifact *mrmodel.UnifiedArtifact
  if strings.ToLower(event.Provider) == "gitlab" {
      log.Printf("[DEBUG] Building GitLab artifact for contextual response")
      var err error
      artifact, err = p.buildGitLabArtifactFromEvent(ctx, event)
      if err != nil {
          return "", nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
      }
  }
  ```
- Pass to next function:
  ```go
  response, learning := p.buildContextualResponseWithLearningV2(ctx, event, timeline, orgID, artifact)
  ```

**KEY LEARNINGS:**
1. **Use Case-Insensitive Check**: Use `strings.ToLower(event.Provider) == "gitlab"` instead of exact match.
2. **Fail on Error**: Don't continue without context - return error immediately. Missing credentials or network issues should stop the reply flow rather than silently failing.
3. **Pass Context**: Always pass `ctx` to `buildGitLabArtifactFromEvent(ctx, event)`. if node.FilePath != "" {
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

## PHASE 2: Testing Iteration 1 (GitLab)

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

## PHASE A: GitHub Support

**Goal:** Add same contextual artifact support for GitHub PRs as we have for GitLab MRs

**File:** `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`

**Step A.1:** Add GitHub provider import (at top of file, after GitLab import)
```go
gh "github.com/livereview/internal/providers/github"
```

**Step A.2:** Add helper method `buildGitHubArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error)`
- Location: Add after `buildGitLabArtifactFromEvent()`
- Copy pattern from cli.go GitHub handling:
  ```go
  func (p *UnifiedProcessorV2Impl) buildGitHubArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error) {
      if event.Repository.WebURL == "" || event.MergeRequest == nil {
          return nil, fmt.Errorf("missing required fields for PR URL construction")
      }
      
      // Parse GitHub PR URL from event
      // event.Repository.WebURL is like "https://github.com/owner/repo"
      // We need owner, repo, and PR number
      
      // Extract owner and repo from WebURL
      // Format: https://github.com/owner/repo
      var owner, repo string
      if idx := strings.Index(event.Repository.WebURL, "github.com/"); idx != -1 {
          remainder := event.Repository.WebURL[idx+len("github.com/"):]
          parts := strings.Split(remainder, "/")
          if len(parts) >= 2 {
              owner = parts[0]
              repo = parts[1]
          } else {
              return nil, fmt.Errorf("invalid GitHub repository URL format: %s", event.Repository.WebURL)
          }
      } else {
          return nil, fmt.Errorf("not a GitHub URL: %s", event.Repository.WebURL)
      }
      
      // Get PR number
      var prNumber string
      if event.MergeRequest.Number > 0 {
          prNumber = fmt.Sprintf("%d", event.MergeRequest.Number)
      } else if event.MergeRequest.ID != "" {
          prNumber = event.MergeRequest.ID
      } else {
          return nil, fmt.Errorf("missing PR number/ID")
      }
      
      log.Printf("[DEBUG] Extracted GitHub PR info: owner=%s, repo=%s, pr=%s", owner, repo, prNumber)
      
      // Look up GitHub PAT from integration_tokens table
      // GitHub uses 'github.com' as provider_url or 'github' as provider
      query := `SELECT pat_token FROM integration_tokens 
                WHERE provider IN ('github', 'GitHub') 
                AND (provider_url = 'https://github.com' OR provider_url = 'github.com' OR provider_url = 'github')
                LIMIT 1`
      
      var patToken string
      err := p.server.DB().QueryRow(query).Scan(&patToken)
      if err != nil {
          return nil, fmt.Errorf("failed to find GitHub PAT: %w", err)
      }
      
      log.Printf("[DEBUG] Found GitHub PAT")
      
      // Create mrModel instance
      mrModel := &mrmodel.MrModelImpl{}
      mrModel.EnableArtifactWriting = false // Don't write to disk
      
      // Build GitHub artifact (following cli.go pattern)
      artifact, err := mrModel.BuildGitHubArtifact(owner, repo, prNumber, patToken, "")
      if err != nil {
          return nil, fmt.Errorf("failed to build GitHub artifact: %w", err)
      }
      
      log.Printf("[DEBUG] Built GitHub artifact: timeline=%d participants=%d diffs=%d",
          len(artifact.Timeline), len(artifact.Participants), len(artifact.Diffs))
      
      return artifact, nil
  }
  ```

**KEY LEARNINGS - GitHub:**
1. **URL Structure**: GitHub URLs are simpler - `https://github.com/owner/repo` (no project path like GitLab)
2. **Token Lookup**: GitHub PAT is stored with provider='github' and provider_url can be 'github.com' or 'github'
3. **Artifact Building**: `BuildGitHubArtifact(owner, repo, prNumber, pat, outDir)` takes owner/repo separately
4. **No Provider Instance**: Unlike GitLab which needs a provider instance, GitHub artifact builder uses direct API calls
5. **Same Artifact Structure**: Returns same `UnifiedArtifact` type - can reuse `formatArtifactForPrompt()`

**Step A.3:** Modify `ProcessCommentReply()` to handle GitHub (update existing code from Step 1.4)
- Location: Line 240, modify the artifact building section
- Change from:
  ```go
  // Build GitLab artifact for context (Phase 1 - Iteration 1)
  var artifact *mrmodel.UnifiedArtifact
  if strings.ToLower(event.Provider) == "gitlab" && p.server.DB() != nil {
      log.Printf("[DEBUG] Building GitLab artifact for contextual response")
      var err error
      artifact, err = p.buildGitLabArtifactFromEvent(ctx, event)
      if err != nil {
          return "", nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
      }
  }
  ```
- To:
  ```go
  // Build artifact for context (Phase 1 + Phase A)
  var artifact *mrmodel.UnifiedArtifact
  if p.server.DB() != nil {
      provider := strings.ToLower(event.Provider)
      switch provider {
      case "gitlab":
          log.Printf("[DEBUG] Building GitLab artifact for contextual response")
          var err error
          artifact, err = p.buildGitLabArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
          }
      case "github":
          log.Printf("[DEBUG] Building GitHub artifact for contextual response")
          var err error
          artifact, err = p.buildGitHubArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitHub artifact: %w", err)
          }
      }
  }
  ```

**Step A.4:** No other changes needed!
- The `formatArtifactForPrompt()` method works for both GitLab and GitHub (same UnifiedArtifact structure)
- The `formatCommentThreadBasic()` method works for both providers
- All prompt building logic is provider-agnostic

---

## PHASE A Testing: GitHub Support

**Step A.T1:** Build
```bash
bash -lc 'go build livereview.go'
```

**Step A.T2:** Test with GitHub
- Post a comment on GitHub PR (e.g., `@livereview what does this code do?`)
- Check `debug_prompt.txt` should show:
  - "=== MERGE REQUEST CONTEXT ===" (same header for both providers)
  - List of changed files from the PR
  - Diff hunks (truncated)
  - Discussion threads (PR comments, review comments, reviews)

**Step A.T3:** Verify bot reply uses context
- Bot should have awareness of other files/comments in PR
- Should reference existing review comments
- Should understand full PR context when answering questions

**Step A.T4:** Test both providers work together
- Verify GitLab still works (no regression)
- Verify GitHub works independently
- Both use the same context formatting

---

## PHASE B: Bitbucket Support

**Goal:** Add same contextual artifact support for Bitbucket PRs as we have for GitLab MRs and GitHub PRs

**File:** `/home/shrsv/bin/LiveReview/internal/api/unified_processor_v2.go`

**Step B.1:** Add Bitbucket provider import (at top of file, after GitHub import)
```go
bb "github.com/livereview/internal/providers/bitbucket"
```

**Step B.2:** Add helper method `buildBitbucketArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error)`
- Location: Add after `buildGitHubArtifactFromEvent()`
- Copy pattern from cli.go Bitbucket handling:
  ```go
  func (p *UnifiedProcessorV2Impl) buildBitbucketArtifactFromEvent(ctx context.Context, event UnifiedWebhookEventV2) (*mrmodel.UnifiedArtifact, error) {
      if event.Repository.WebURL == "" || event.MergeRequest == nil {
          return nil, fmt.Errorf("missing required fields for PR URL construction")
      }
      
      // For Bitbucket, we need the full PR URL and PR ID
      // event.Repository.WebURL is like "https://bitbucket.org/workspace/repo"
      // PR URL format: https://bitbucket.org/workspace/repo/pull-requests/123
      
      // Get PR ID/number
      var prID string
      if event.MergeRequest.Number > 0 {
          prID = fmt.Sprintf("%d", event.MergeRequest.Number)
      } else if event.MergeRequest.ID != "" {
          prID = event.MergeRequest.ID
      } else {
          return nil, fmt.Errorf("missing PR number/ID")
      }
      
      // Construct full PR URL
      // event.Repository.WebURL should already contain workspace/repo
      prURL := strings.TrimRight(event.Repository.WebURL, "/") + "/pull-requests/" + prID
      
      log.Printf("[DEBUG] Constructed Bitbucket PR URL: %s", prURL)
      
      // Look up Bitbucket credentials from integration_tokens table
      // Bitbucket stores provider='bitbucket' or 'Bitbucket'
      query := `SELECT pat_token FROM integration_tokens 
                WHERE provider IN ('bitbucket', 'Bitbucket') 
                LIMIT 1`
      
      var patToken string
      err := p.server.DB().QueryRow(query).Scan(&patToken)
      if err != nil {
          return nil, fmt.Errorf("failed to find Bitbucket PAT: %w", err)
      }
      
      log.Printf("[DEBUG] Found Bitbucket PAT")
      
      // Bitbucket provider needs email - use default for bot
      // In production, this could come from metadata or config
      botEmail := "livereviewbot@gmail.com"
      
      // Create Bitbucket provider (following cli.go pattern)
      provider, err := bb.NewBitbucketProvider(patToken, botEmail, prURL)
      if err != nil {
          return nil, fmt.Errorf("failed to init bitbucket provider: %w", err)
      }
      
      // Create mrModel instance
      mrModel := &mrmodel.MrModelImpl{}
      mrModel.EnableArtifactWriting = false // Don't write to disk
      
      // Build Bitbucket artifact (following cli.go pattern)
      artifact, err := mrModel.BuildBitbucketArtifact(provider, prID, prURL, "")
      if err != nil {
          return nil, fmt.Errorf("failed to build Bitbucket artifact: %w", err)
      }
      
      log.Printf("[DEBUG] Built Bitbucket artifact: timeline=%d participants=%d diffs=%d",
          len(artifact.Timeline), len(artifact.Participants), len(artifact.Diffs))
      
      return artifact, nil
  }
  ```

**KEY LEARNINGS - Bitbucket:**
1. **URL Structure**: Bitbucket URLs use workspace/repo format - `https://bitbucket.org/workspace/repo/pull-requests/123`
2. **PR URL Construction**: Must append `/pull-requests/{prID}` to repository WebURL
3. **Token Lookup**: Bitbucket PAT is stored with provider='bitbucket' or 'Bitbucket'
4. **Email Requirement**: Bitbucket provider constructor requires email parameter (unlike GitLab/GitHub)
5. **Provider Instance**: Like GitLab, Bitbucket needs a provider instance created with `NewBitbucketProvider(token, email, prURL)`
6. **Same Artifact Structure**: Returns same `UnifiedArtifact` type - can reuse `formatArtifactForPrompt()`
7. **Artifact Building**: `BuildBitbucketArtifact(provider, prID, prURL, outDir)` takes provider instance and both prID and prURL

**Step B.3:** Modify `ProcessCommentReply()` to handle Bitbucket (update existing switch from Phase A)
- Location: Line 250, add new case to existing switch statement
- Change from:
  ```go
  // Build artifact for context (Phase 1 + Phase A)
  var artifact *mrmodel.UnifiedArtifact
  if p.server.DB() != nil {
      provider := strings.ToLower(event.Provider)
      switch provider {
      case "gitlab":
          log.Printf("[DEBUG] Building GitLab artifact for contextual response")
          var err error
          artifact, err = p.buildGitLabArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
          }
      case "github":
          log.Printf("[DEBUG] Building GitHub artifact for contextual response")
          var err error
          artifact, err = p.buildGitHubArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitHub artifact: %w", err)
          }
      }
  }
  ```
- To:
  ```go
  // Build artifact for context (Phase 1 + Phase A + Phase B)
  var artifact *mrmodel.UnifiedArtifact
  if p.server.DB() != nil {
      provider := strings.ToLower(event.Provider)
      switch provider {
      case "gitlab":
          log.Printf("[DEBUG] Building GitLab artifact for contextual response")
          var err error
          artifact, err = p.buildGitLabArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitLab artifact: %w", err)
          }
      case "github":
          log.Printf("[DEBUG] Building GitHub artifact for contextual response")
          var err error
          artifact, err = p.buildGitHubArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build GitHub artifact: %w", err)
          }
      case "bitbucket":
          log.Printf("[DEBUG] Building Bitbucket artifact for contextual response")
          var err error
          artifact, err = p.buildBitbucketArtifactFromEvent(ctx, event)
          if err != nil {
              return "", nil, fmt.Errorf("failed to build Bitbucket artifact: %w", err)
          }
      }
  }
  ```

**Step B.4:** No other changes needed!
- The `formatArtifactForPrompt()` method works for all three providers (same UnifiedArtifact structure)
- The `formatCommentThreadBasic()` method works for all providers
- All prompt building logic is provider-agnostic

---

## PHASE B Testing: Bitbucket Support

**Step B.T1:** Build
```bash
bash -lc 'go build livereview.go'
```

**Step B.T2:** Test with Bitbucket
- Post a comment on Bitbucket PR (e.g., `@livereview can you explain this change?`)
- Check `debug_prompt.txt` should show:
  - "=== MERGE REQUEST CONTEXT ===" (same header for all providers)
  - List of changed files from the PR
  - Diff hunks (truncated)
  - Discussion threads (PR comments, inline comments, activities)

**Step B.T3:** Verify bot reply uses context
- Bot should have awareness of other files/comments in PR
- Should reference existing PR comments
- Should understand full PR context when answering questions

**Step B.T4:** Test all three providers work together
- Verify GitLab still works (no regression)
- Verify GitHub still works (no regression)
- Verify Bitbucket works independently
- All three use the same context formatting

**Step B.T5:** Test database integration
- Verify Bitbucket PAT lookup works from integration_tokens table
- Test with `provider='bitbucket'` and `provider='Bitbucket'` (case variations)
- Confirm email parameter doesn't cause issues

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