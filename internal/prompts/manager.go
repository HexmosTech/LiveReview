package prompts

import "context"

// Manager coordinates vendor templates, context resolution, and chunk CRUD.
// All prompt rendering must go through this interface.
type Manager interface {
	// Vendor template runtime
	GetTemplateDescriptor(promptKey string, provider string) (TemplateDescriptor, error)
	Render(ctx context.Context, c Context, promptKey string, vars map[string]string) (string, error)

	// Application context resolution
	ResolveApplicationContext(ctx context.Context, c Context) (applicationContextID int64, err error)

	// Chunks CRUD
	ListChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string) ([]Chunk, error)
	CreateChunk(ctx context.Context, ch Chunk) (int64, error)
	UpdateChunk(ctx context.Context, ch Chunk) error
	DeleteChunk(ctx context.Context, orgID, chunkID int64) error
	ReorderChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string, orderedIDs []int64) error
}
