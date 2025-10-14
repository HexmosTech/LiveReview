# Bitbucket Reply Regression – Single-Path Recovery Plan

We follow the exact three checkpoints you asked for. Each step must be demonstrated independently, with prints/logs that you can inspect yourself. No extra detours.

## Step 1 – Fetch Fresh Bot User Details (Reuse Production DB Code)

1. Use the real development database that `make run` / `make develop` already connect to—no in-memory instances, no env-variable overrides.
2. Create a Go test `internal/api/bitbucket_bot_user_test.go` that:
   - Constructs a `Server` via `NewServer` so the same DB connection and configuration used in dev mode is leveraged.
   - Invokes `server.getFreshBitbucketBotUserInfo("<workspace>/<repo>")` (use the repo you trigger reviews against).
   - Prints (`t.Logf`) the returned `Username`, `DisplayName`, `AccountID`, and `UUID` so you can see the exact identifiers.
   - Asserts each field is non-empty and that `AccountID` matches the value you have on record for the Bitbucket bot.
3. Run `go test ./internal/api -run TestFetchBitbucketBotUser` while the dev DB is accessible; the test should pass and show the live identifiers.

## Step 2 – Verify Comment Hierarchy Capture

1. Add a unit test `internal/provider_input/bitbucket/bitbucket_comment_hierarchy_test.go`.
2. Feed the captured payload from `captures/bitbucket/20251014-162832` into `ConvertCommentEvent` (use the JSON from that directory as the fixture).
3. Assert the resulting `UnifiedWebhookEventV2` has:
   - `Comment.InReplyToID` equal to `"699606563"`.
   - `Comment.Metadata["workspace"]`, `Comment.Metadata["repository"]`, and `Comment.Metadata["pr_number"]` populated (required for posting a reply).
   - `Comment.Author.ID` different from the bot’s AccountID (proving we know who sent the reply).
4. Print the decoded hierarchy with `t.Logf` so you can visually inspect parent/child relationships.

## Step 3 – Generate and Post the Reply in Context

1. Implement `isReplyToBotComment` so it actually checks whether the parent comment (ID from `InReplyToID`) belongs to the bot:
   - Use Step 1 data (bot AccountID/UUID) and Step 2 metadata (workspace, repo, PR number) to fetch the parent comment via Bitbucket API.
   - Compare the parent comment’s author identifiers against the bot identifiers.
2. Add a focused test `internal/api/bitbucket_reply_flow_test.go` that:
   - Constructs a `UnifiedWebhookEventV2` representing “user replies to bot”.
   - Uses a fake Bitbucket output client to capture `PostCommentReply` calls.
   - Asserts that `handleCommentReplyFlow` is invoked and the reply targets the correct parent ID.
   - Prints the synthesized reply text and the target comment ID for inspection.
3. After the code passes tests, run a real PR interaction (bot comment -> your reply). Confirm Bitbucket shows the bot’s response in place, and capture the logs as proof.

Once these three checkpoints are proven, we delete the temporary mention fallback and rely solely on the reply hierarchy.
