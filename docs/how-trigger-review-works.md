Incoming POST /api/trigger-review hits internal/api/review_service.go::TriggerReviewV2, which parses the body, creates a DB review row, spins up a logging.ReviewLogger, resolves the integration token, and builds a review.Service plus review.ReviewRequest (createReviewService/buildReviewRequest in reviews_api.go).

launchBackgroundProcessing then calls review.Service.ProcessReview (service.go) in a goroutine; this method instantiates the SCM provider, constructs an AI provider (LangChain wrapper today), pulls merge-request details/diffs, and invokes aiProvider.ReviewCodeWithBatching.

The LangChain provider (provider.go) line-numbers every diff hunk, renders the “code_review” prompt via prompts.Manager.Render + BuildCodeChangesSection, streams the LLM output, and parses per-file summaries/comments. Each batch yields a batch.BatchResult carrying FileSummary and structured comments.

After all batches finish, batch.BatchProcessor.AggregateAndCombineOutputs (batch.go) gathers every FileSummary plus both internal/external comments, renders the “summary” prompt (prompts.Manager.Render("summary") pulling SummaryWriterRole/SummaryRequirements/SummaryStructure from templates.go via registry_stub.go), appends BuildSummarySection output, and calls llms.GenerateFromSinglePrompt to synthesize the final markdown summary; external comments become ReviewResult.Comments.

Back in review.Service, postReviewResults posts that markdown summary as a top-level MR comment (category summary, enriched by appendLearningAcknowledgment in learning_acknowledgment.go) and then posts each external line comment via the provider SDK; the goroutine updates the review status once posting finishes.

If you want to inspect the exact prompt/response for a run, grab the review ID from the API response and open the matching log under review_logs—the LangChain provider persists the rendered prompts and streamed chunks there.