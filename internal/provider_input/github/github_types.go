package github

// GitHubV2WebhookPayload represents a GitHub webhook payload
type GitHubV2WebhookPayload struct {
	Action      string              `json:"action"`
	Number      int                 `json:"number"`
	PullRequest GitHubV2PullRequest `json:"pull_request"`
	Repository  GitHubV2Repository  `json:"repository"`
	Sender      GitHubV2User        `json:"sender"`
	// For review_requested/review_request_removed actions
	RequestedReviewer GitHubV2User `json:"requested_reviewer,omitempty"`
	RequestedTeam     GitHubV2Team `json:"requested_team,omitempty"`
}

// GitHubV2PullRequest represents a GitHub pull request
type GitHubV2PullRequest struct {
	ID                 int            `json:"id"`
	Number             int            `json:"number"`
	Title              string         `json:"title"`
	Body               string         `json:"body"`
	State              string         `json:"state"`
	HTMLURL            string         `json:"html_url"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
	Head               GitHubV2Branch `json:"head"`
	Base               GitHubV2Branch `json:"base"`
	User               GitHubV2User   `json:"user"`
	RequestedReviewers []GitHubV2User `json:"requested_reviewers"`
	RequestedTeams     []GitHubV2Team `json:"requested_teams"`
	Assignees          []GitHubV2User `json:"assignees"`
}

// GitHubV2Repository represents a GitHub repository
type GitHubV2Repository struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	FullName string       `json:"full_name"`
	HTMLURL  string       `json:"html_url"`
	Owner    GitHubV2User `json:"owner"`
	Private  bool         `json:"private"`
}

// GitHubV2User represents a GitHub user
type GitHubV2User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// GitHubV2Team represents a GitHub team
type GitHubV2Team struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

// GitHubV2Branch represents a GitHub branch
type GitHubV2Branch struct {
	Ref  string             `json:"ref"`
	SHA  string             `json:"sha"`
	Repo GitHubV2Repository `json:"repo"`
}

// GitHubV2ReviewerChangeInfo represents reviewer change information
type GitHubV2ReviewerChangeInfo struct {
	Previous []int `json:"previous"`
	Current  []int `json:"current"`
}

// GitHubV2ReviewerBotUserInfo represents reviewer bot user information
type GitHubV2ReviewerBotUserInfo struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

// GitHubV2IssueCommentWebhookPayload represents a GitHub issue comment webhook payload
type GitHubV2IssueCommentWebhookPayload struct {
	Action     string             `json:"action"`
	Issue      GitHubV2Issue      `json:"issue"`
	Comment    GitHubV2Comment    `json:"comment"`
	Repository GitHubV2Repository `json:"repository"`
	Sender     GitHubV2User       `json:"sender"`
}

// GitHubV2PullRequestReviewCommentWebhookPayload represents a GitHub PR review comment webhook payload
type GitHubV2PullRequestReviewCommentWebhookPayload struct {
	Action      string                `json:"action"`
	Comment     GitHubV2ReviewComment `json:"comment"`
	PullRequest GitHubV2PullRequest   `json:"pull_request"`
	Repository  GitHubV2Repository    `json:"repository"`
	Sender      GitHubV2User          `json:"sender"`
}

// GitHubV2Comment represents a GitHub comment
type GitHubV2Comment struct {
	ID                    int          `json:"id"`
	HTMLURL               string       `json:"html_url"`
	IssueURL              string       `json:"issue_url"`
	User                  GitHubV2User `json:"user"`
	CreatedAt             string       `json:"created_at"`
	UpdatedAt             string       `json:"updated_at"`
	AuthorAssociation     string       `json:"author_association"`
	Body                  string       `json:"body"`
	Reactions             interface{}  `json:"reactions"`
	PerformedViaGitHubApp interface{}  `json:"performed_via_github_app"`
}

// GitHubV2ReviewComment represents a GitHub review comment
type GitHubV2ReviewComment struct {
	ID                    int          `json:"id"`
	DiffHunk              string       `json:"diff_hunk"`
	Path                  string       `json:"path"`
	Position              int          `json:"position"`
	OriginalPosition      int          `json:"original_position"`
	CommitID              string       `json:"commit_id"`
	OriginalCommitID      string       `json:"original_commit_id"`
	InReplyToID           *int         `json:"in_reply_to_id"`
	User                  GitHubV2User `json:"user"`
	Body                  string       `json:"body"`
	CreatedAt             string       `json:"created_at"`
	UpdatedAt             string       `json:"updated_at"`
	HTMLURL               string       `json:"html_url"`
	PullRequestURL        string       `json:"pull_request_url"`
	AuthorAssociation     string       `json:"author_association"`
	StartLine             *int         `json:"start_line"`
	OriginalStartLine     *int         `json:"original_start_line"`
	StartSide             string       `json:"start_side"`
	Line                  int          `json:"line"`
	OriginalLine          int          `json:"original_line"`
	Side                  string       `json:"side"`
	Reactions             interface{}  `json:"reactions"`
	PerformedViaGitHubApp interface{}  `json:"performed_via_github_app"`
}

// GitHubV2Issue represents a GitHub issue
type GitHubV2Issue struct {
	ID                int                 `json:"id"`
	Number            int                 `json:"number"`
	Title             string              `json:"title"`
	User              GitHubV2User        `json:"user"`
	Labels            []GitHubV2Label     `json:"labels"`
	State             string              `json:"state"`
	Locked            bool                `json:"locked"`
	Assignee          *GitHubV2User       `json:"assignee"`
	Assignees         []GitHubV2User      `json:"assignees"`
	Milestone         *GitHubV2Milestone  `json:"milestone"`
	Comments          int                 `json:"comments"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	ClosedAt          *string             `json:"closed_at"`
	AuthorAssociation string              `json:"author_association"`
	ActiveLockReason  *string             `json:"active_lock_reason"`
	Body              string              `json:"body"`
	ClosedBy          *GitHubV2User       `json:"closed_by"`
	HTMLURL           string              `json:"html_url"`
	NodeID            string              `json:"node_id"`
	RepositoryURL     string              `json:"repository_url"`
	PullRequest       *GitHubV2IssuePRRef `json:"pull_request"`
}

// GitHubV2Label represents a GitHub label
type GitHubV2Label struct {
	ID          int     `json:"id"`
	NodeID      string  `json:"node_id"`
	URL         string  `json:"url"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Default     bool    `json:"default"`
	Description *string `json:"description"`
}

// GitHubV2Milestone represents a GitHub milestone
type GitHubV2Milestone struct {
	URL          string       `json:"url"`
	HTMLURL      string       `json:"html_url"`
	LabelsURL    string       `json:"labels_url"`
	ID           int          `json:"id"`
	NodeID       string       `json:"node_id"`
	Number       int          `json:"number"`
	Title        string       `json:"title"`
	Description  *string      `json:"description"`
	Creator      GitHubV2User `json:"creator"`
	OpenIssues   int          `json:"open_issues"`
	ClosedIssues int          `json:"closed_issues"`
	State        string       `json:"state"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
	DueOn        *string      `json:"due_on"`
	ClosedAt     *string      `json:"closed_at"`
}

// GitHubV2IssuePRRef represents a GitHub issue PR reference
type GitHubV2IssuePRRef struct {
	URL      string `json:"url"`
	HTMLURL  string `json:"html_url"`
	DiffURL  string `json:"diff_url"`
	PatchURL string `json:"patch_url"`
}

// GitHubV2BotUserInfo represents GitHub bot user information
type GitHubV2BotUserInfo struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// GitHubV2CommitInfo represents GitHub commit information for timeline
type GitHubV2CommitInfo struct {
	SHA       string               `json:"sha"`
	Message   string               `json:"message"`
	Author    GitHubV2CommitAuthor `json:"author"`
	Committer GitHubV2CommitAuthor `json:"committer"`
	URL       string               `json:"url"`
}

// GitHubV2CommitAuthor represents GitHub commit author
type GitHubV2CommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// GitHubV2CommentInfo represents GitHub comment information for timeline
type GitHubV2CommentInfo struct {
	ID        int          `json:"id"`
	Body      string       `json:"body"`
	User      GitHubV2User `json:"user"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
}
