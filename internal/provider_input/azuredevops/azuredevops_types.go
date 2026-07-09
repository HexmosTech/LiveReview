package azuredevops

import (
	"encoding/json"

	coreprocessor "github.com/livereview/internal/core_processor"
)

// Type aliases for unified types
type (
	UnifiedWebhookEventV2 = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2 = coreprocessor.UnifiedMergeRequestV2
	UnifiedCommentV2      = coreprocessor.UnifiedCommentV2
	UnifiedUserV2         = coreprocessor.UnifiedUserV2
	UnifiedRepositoryV2   = coreprocessor.UnifiedRepositoryV2
	UnifiedPositionV2     = coreprocessor.UnifiedPositionV2
	UnifiedBotUserInfoV2  = coreprocessor.UnifiedBotUserInfoV2
)

// AzureWebhookPayload represents the envelope Azure DevOps Service Hooks sends
// for every subscribed event. The shape of Resource depends on EventType.
// Reference: https://learn.microsoft.com/azure/devops/service-hooks/events
type AzureWebhookPayload struct {
	ID                 string                    `json:"id"`
	EventType          string                    `json:"eventType"`
	PublisherID        string                    `json:"publisherId"`
	Resource           json.RawMessage           `json:"resource"`
	ResourceContainers *AzureResourceContainers  `json:"resourceContainers,omitempty"`
	CreatedDate        string                    `json:"createdDate"`
}

// AzureResourceContainers identifies the collection/account/project scope of the event.
type AzureResourceContainers struct {
	Collection AzureResourceContainer `json:"collection"`
	Account    AzureResourceContainer `json:"account"`
	Project    AzureResourceContainer `json:"project"`
}

// AzureResourceContainer is a single {id, baseUrl} container reference.
type AzureResourceContainer struct {
	ID      string `json:"id"`
	BaseURL string `json:"baseUrl,omitempty"`
}

// AzureIdentity mirrors an Azure DevOps identity/user reference.
type AzureIdentity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	UniqueName  string `json:"uniqueName"`
	ImageURL    string `json:"imageUrl"`
}

// AzureProject mirrors a project reference embedded in a repository.
type AzureProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AzureRepository mirrors the repository object embedded in PR resources.
type AzureRepository struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	URL       string       `json:"url"`
	RemoteURL string       `json:"remoteUrl"`
	Project   AzureProject `json:"project"`
}

// AzureCommitRef mirrors a commit reference ({commitId}).
type AzureCommitRef struct {
	CommitID string `json:"commitId"`
}

// AzurePullRequestResource is the `resource` payload for
// git.pullrequest.created / git.pullrequest.updated events.
type AzurePullRequestResource struct {
	Repository            AzureRepository `json:"repository"`
	PullRequestID         int64           `json:"pullRequestId"`
	Status                string          `json:"status"`
	CreatedBy             AzureIdentity   `json:"createdBy"`
	CreationDate          string          `json:"creationDate"`
	Title                 string          `json:"title"`
	Description           string          `json:"description"`
	SourceRefName         string          `json:"sourceRefName"`
	TargetRefName         string          `json:"targetRefName"`
	MergeStatus           string          `json:"mergeStatus"`
	LastMergeSourceCommit AzureCommitRef  `json:"lastMergeSourceCommit"`
	LastMergeTargetCommit AzureCommitRef  `json:"lastMergeTargetCommit"`
	URL                   string          `json:"url"`
}

// AzureHref is a single {href} hypermedia link.
type AzureHref struct {
	Href string `json:"href"`
}

// AzureCommentEventLinks carries the hypermedia links delivered on a
// PR-comment webhook resource, used to recover the thread id, repository
// GUID, and PR number - none of which are present as plain fields on the
// resource itself.
type AzureCommentEventLinks struct {
	Self         *AzureHref `json:"self,omitempty"`
	Repository   *AzureHref `json:"repository,omitempty"`
	Threads      *AzureHref `json:"threads,omitempty"`
	PullRequests *AzureHref `json:"pullRequests,omitempty"`
}

// AzureCommentEventResource is the `resource` payload for
// ms.vss-code.git-pullrequest-comment-event events.
//
// Confirmed against a live subscription's captured notification payload -
// Microsoft's published docs example is misleading: it shows resource as
// {comment: {...}, pullRequest: {...}}, but the real payload has the comment
// fields directly on resource with no "comment"/"pullRequest" wrapper, and
// carries no repository/project/PR *names* at all - only a repository GUID
// and a PR-number link, both recoverable via _links hrefs.
type AzureCommentEventResource struct {
	ID              int64                   `json:"id"`
	ParentCommentID int64                   `json:"parentCommentId"`
	Content         string                  `json:"content"`
	CommentType     string                  `json:"commentType"`
	Author          AzureIdentity           `json:"author"`
	PublishedDate   string                  `json:"publishedDate"`
	LastUpdatedDate string                  `json:"lastUpdatedDate"`
	Links           *AzureCommentEventLinks `json:"_links,omitempty"`
}

// IntegrationToken represents an Azure DevOps connector row from integration_tokens.
type IntegrationToken struct {
	ID          int64
	Provider    string
	ProviderURL string // organization URL, e.g. https://dev.azure.com/myorg
	PatToken    string
	OrgID       int64
	Metadata    map[string]any
}
