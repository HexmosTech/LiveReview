
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