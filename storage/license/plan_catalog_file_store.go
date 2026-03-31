package license

import (
	"fmt"

	"github.com/livereview/storage/core"
)

// PlanCatalogFileStore centralizes file operations for the plan catalog.
type PlanCatalogFileStore struct {
	fileOps *core.FileOpsStore
}

func NewPlanCatalogFileStore() *PlanCatalogFileStore {
	return &PlanCatalogFileStore{fileOps: core.NewFileOpsStore()}
}

func (s *PlanCatalogFileStore) ReadPlanCatalogFile(path string) ([]byte, error) {
	content, err := s.fileOps.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan catalog file %s: %w", path, err)
	}
	return content, nil
}
