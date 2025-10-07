package learnings

import "time"

type ScopeKind string

const (
	ScopeOrg  ScopeKind = "org"
	ScopeRepo ScopeKind = "repo"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// SourceContext captures where a learning originated for traceability
type SourceContext struct {
	Provider   string `json:"provider"`
	Repository string `json:"repository"`
	PRNumber   int    `json:"pr_number"`
	MRNumber   int    `json:"mr_number"`
	CommitSHA  string `json:"commit_sha"`
	FilePath   string `json:"file_path"`
	LineStart  int    `json:"line_start"`
	LineEnd    int    `json:"line_end"`
	ThreadID   string `json:"thread_id"`
	CommentID  string `json:"comment_id"`
}

type Learning struct {
	ID            string         `json:"id"`
	ShortID       string         `json:"short_id"`
	OrgID         int64          `json:"org_id"`
	Scope         ScopeKind      `json:"scope"`
	RepoID        string         `json:"repo_id,omitempty"`
	Title         string         `json:"title"`
	Body          string         `json:"body"`
	Tags          []string       `json:"tags"`
	Status        Status         `json:"status"`
	Confidence    int            `json:"confidence"`
	Simhash       uint64         `json:"simhash"`
	Embedding     []byte         `json:"embedding,omitempty"`
	SourceURLs    []string       `json:"source_urls"`
	SourceContext *SourceContext `json:"source_context,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type LearningEventAction string

const (
	EventAdd    LearningEventAction = "add"
	EventUpdate LearningEventAction = "update"
	EventDelete LearningEventAction = "delete"
)

type LearningEvent struct {
	ID         string                 `json:"id"`
	LearningID string                 `json:"learning_id"`
	OrgID      int64                  `json:"org_id"`
	Action     LearningEventAction    `json:"action"`
	Provider   string                 `json:"provider"`
	ThreadID   string                 `json:"thread_id"`
	CommentID  string                 `json:"comment_id"`
	Repository string                 `json:"repository"`
	CommitSHA  string                 `json:"commit_sha"`
	FilePath   string                 `json:"file_path"`
	LineStart  int                    `json:"line_start"`
	LineEnd    int                    `json:"line_end"`
	Reason     string                 `json:"reason"`
	Classifier string                 `json:"classifier"`
	Context    map[string]interface{} `json:"context"`
	CreatedAt  time.Time              `json:"created_at"`
}

// Draft represents LLM-provided fields for creating a learning
type Draft struct {
	Title     string
	Body      string
	Tags      []string
	Scope     ScopeKind
	RepoID    string
	SourceURL string
}

// Deltas represents edit requests parsed from NL
type Deltas struct {
	Title  *string
	Body   *string
	Tags   *[]string
	Scope  *ScopeKind
	RepoID *string
}

// MRContext is an internal representation of contextual fields captured at action time
type MRContext struct {
	Provider   string
	Repository string
	PRNumber   int
	MRNumber   int
	CommitSHA  string
	FilePath   string
	LineStart  int
	LineEnd    int
	ThreadID   string
	CommentID  string
}
