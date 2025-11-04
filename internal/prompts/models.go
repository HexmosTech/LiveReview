package prompts

// Core model types for prompt customization & rendering.

// TemplateDescriptor is metadata about a vendor template variant.
// The actual template body is not stored here; it lives as an encrypted blob
// and is accessed via the vendor pack.
type TemplateDescriptor struct {
	PromptKey      string   // logical key, e.g., "code_review"
	BuildID        string   // git commit or tag for the template pack
	CipherChecksum string   // checksum of the encrypted blob (non-sensitive)
	Variables      []string // optional declared variables; if empty, discover at runtime
	Provider       string   // optional provider name; empty means default
}

// Context selects the application scope for resolving chunks.
// Nil pointer fields indicate wildcard (unspecified) values.
type Context struct {
	OrgID              int64
	AIConnectorID      *int64 // nullable; determines provider for template variant
	IntegrationTokenID *int64 // nullable; git connector id
	Repository         *string
}

// TechnicalSummary captures the technical takeaways for a single file or
// architectural unit so downstream prompts can stay focused on facts instead
// of inline review commentary.
type TechnicalSummary struct {
	FilePath   string   // canonical file path; empty indicates repo-wide notes
	Summary    string   // concise technical description of what changed
	KeyChanges []string // optional bullet list of noteworthy impacts
}

// Chunk represents a user/system-provided text block that contributes to a variable.
type Chunk struct {
	ID                   int64
	OrgID                int64
	ApplicationContextID int64
	PromptKey            string
	VariableName         string
	Type                 string // "system" | "user"
	Title                string // optional
	Body                 string // plaintext by default
	SequenceIndex        int    // ordering within a variable for a context
	Enabled              bool
	AllowMarkdown        bool
	RedactOnLog          bool
	CreatedBy            *int64
	UpdatedBy            *int64
}
