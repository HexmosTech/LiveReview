package core

import "os"

type FileOpsStore struct{}

func NewFileOpsStore() *FileOpsStore {
	return &FileOpsStore{}
}

func (s *FileOpsStore) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
