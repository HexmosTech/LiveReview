package shared

import (
	"context"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

// TokenStore only tracks the pending login request ID (in-memory).
// Auth tokens and org context are now carried by client headers on every request.
type TokenStore struct {
	mu               sync.RWMutex
	pendingRequestID string
}

func NewTokenStore() *TokenStore {
	return &TokenStore{}
}

func (s *TokenStore) SetPendingRequestID(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRequestID = requestID
}

func (s *TokenStore) GetPendingRequestID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingRequestID
}

// IProxy defines the interface for calling the local API.
type IProxy interface {
	CallAPI(ctx context.Context, method, path string, arguments interface{}) (*mcp.CallToolResult, error)
}

// GlobalProxy is the global instance for tool handlers to use.
var GlobalProxy IProxy
