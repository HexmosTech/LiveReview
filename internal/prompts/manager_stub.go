package prompts

import (
	"context"
	"errors"
)

// NewStubManager returns a no-op Manager used during early development.
func NewStubManager() Manager {
	return &stubManager{}
}

type stubManager struct{}

func (s *stubManager) GetTemplateDescriptor(promptKey string, provider string) (TemplateDescriptor, error) {
	return TemplateDescriptor{}, errors.New("prompts: GetTemplateDescriptor not implemented")
}

func (s *stubManager) Render(ctx context.Context, c Context, promptKey string, vars map[string]string) (string, error) {
	return "", errors.New("prompts: Render not implemented")
}

func (s *stubManager) ResolveApplicationContext(ctx context.Context, c Context) (int64, error) {
	return 0, errors.New("prompts: ResolveApplicationContext not implemented")
}

func (s *stubManager) ListChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string) ([]Chunk, error) {
	return []Chunk{}, nil
}

func (s *stubManager) CreateChunk(ctx context.Context, ch Chunk) (int64, error) {
	return 0, errors.New("prompts: CreateChunk not implemented")
}

func (s *stubManager) UpdateChunk(ctx context.Context, ch Chunk) error {
	return errors.New("prompts: UpdateChunk not implemented")
}

func (s *stubManager) DeleteChunk(ctx context.Context, orgID, chunkID int64) error {
	return errors.New("prompts: DeleteChunk not implemented")
}

func (s *stubManager) ReorderChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string, orderedIDs []int64) error {
	return errors.New("prompts: ReorderChunks not implemented")
}
