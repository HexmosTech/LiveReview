package shared

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TokenStore manages authentication tokens for the MCP server.
type TokenStore struct {
	path   string
	tokens map[string]interface{}
	mu     sync.RWMutex
}

func NewTokenStore() *TokenStore {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting home directory: %v", err)
		home = "."
	}
	path := filepath.Join(home, ".livereview", "mcp_tokens.json")
	store := &TokenStore{
		path:   path,
		tokens: make(map[string]interface{}),
	}
	store.load()
	return store
}

func (s *TokenStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		log.Printf("Error reading token file: %v", err)
		return
	}

	if err := json.Unmarshal(data, &s.tokens); err != nil {
		log.Printf("Error unmarshaling tokens: %v", err)
	}
}

func (s *TokenStore) save() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Error creating token directory: %v", err)
		return
	}

	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		log.Printf("Error marshaling tokens: %v", err)
		return
	}

	if err := os.WriteFile(s.path, data, 0600); err != nil {
		log.Printf("Error writing token file: %v", err)
	}
}

func (s *TokenStore) SetToken(accessToken, refreshToken string) {
	s.mu.Lock()
	s.tokens["access_token"] = accessToken
	s.tokens["refresh_token"] = refreshToken
	s.tokens["updated_at"] = time.Now().Format(time.RFC3339)
	s.mu.Unlock()
	s.save()
}

func (s *TokenStore) SetOrgID(orgID int) {
	s.mu.Lock()
	s.tokens["org_id"] = orgID
	s.mu.Unlock()
	s.save()
}

func (s *TokenStore) SetPendingRequestID(requestID string) {
	s.mu.Lock()
	s.tokens["pending_request_id"] = requestID
	s.mu.Unlock()
	s.save()
}

func (s *TokenStore) GetAccessToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.tokens["access_token"].(string); ok {
		return v
	}
	return ""
}

func (s *TokenStore) GetOrgID() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.tokens["org_id"].(float64); ok {
		return int(v)
	}
	return 0
}

func (s *TokenStore) GetPendingRequestID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.tokens["pending_request_id"].(string); ok {
		return v
	}
	return ""
}

// IProxy defines the interface for calling the local API.
type IProxy interface {
	CallAPI(ctx context.Context, method, path string, arguments interface{}) (*mcp.CallToolResult, error)
}

// GlobalProxy is the global instance for tool handlers to use.
var GlobalProxy IProxy
