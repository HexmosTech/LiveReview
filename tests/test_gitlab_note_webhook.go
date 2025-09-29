package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Test payload structures
type TestGitLabNotePayload struct {
	ObjectKind       string                         `json:"object_kind"`
	EventType        string                         `json:"event_type"`
	User             TestGitLabUser                 `json:"user"`
	ProjectID        int                            `json:"project_id"`
	Project          TestGitLabProject              `json:"project"`
	Repository       TestGitLabRepository           `json:"repository"`
	ObjectAttributes TestGitLabNoteObjectAttributes `json:"object_attributes"`
	MergeRequest     TestGitLabMergeRequest         `json:"merge_request"`
}

type TestGitLabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

type TestGitLabProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

type TestGitLabRepository struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

type TestGitLabNoteObjectAttributes struct {
	ID           int    `json:"id"`
	Note         string `json:"note"`
	NoteableType string `json:"noteable_type"`
	AuthorID     int    `json:"author_id"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ProjectID    int    `json:"project_id"`
	System       bool   `json:"system"`
	Action       string `json:"action"`
	URL          string `json:"url"`
	DiscussionID string `json:"discussion_id"`
}

type TestGitLabMergeRequest struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func main() {
	// Test webhook endpoint URL (adjust as needed)
	webhookURL := "http://localhost:8888/api/v1/webhooks/gitlab/comments"

	// Test Case 1: Direct bot mention
	fmt.Println("=== Test Case 1: Direct Bot Mention ===")
	testDirectMention(webhookURL)

	// Test Case 2: General comment (should not trigger)
	fmt.Println("\n=== Test Case 2: General Comment (No Response) ===")
	testGeneralComment(webhookURL)

	// Test Case 3: Thread reply
	fmt.Println("\n=== Test Case 3: Thread Reply ===")
	testThreadReply(webhookURL)

	// Test Case 4: System comment (should not trigger)
	fmt.Println("\n=== Test Case 4: System Comment ===")
	testSystemComment(webhookURL)
}

func testDirectMention(webhookURL string) {
	payload := TestGitLabNotePayload{
		ObjectKind: "note",
		EventType:  "note",
		Project: TestGitLabProject{
			ID:                101,
			Name:              "Test Project",
			PathWithNamespace: "test/project",
			WebURL:            "https://gitlab.com/test/project",
		},
		Repository: TestGitLabRepository{
			Name:        "Test Project",
			URL:         "https://gitlab.com/test/project.git",
			Homepage:    "https://gitlab.com/test/project",
			Description: "A test project",
		},
		User: TestGitLabUser{
			ID:       1,
			Name:     "John Doe",
			Username: "johndoe",
			Email:    "john@example.com",
		},
		ObjectAttributes: TestGitLabNoteObjectAttributes{
			ID:           201,
			Note:         "@ai-bot Can you help me review this code?",
			NoteableType: "MergeRequest",
			AuthorID:     1,
			CreatedAt:    time.Now().Format(time.RFC3339),
			UpdatedAt:    time.Now().Format(time.RFC3339),
			ProjectID:    101,
			System:       false,
			Action:       "create",
			URL:          "https://gitlab.com/test/project/-/merge_requests/1#note_201",
			DiscussionID: "",
		},
		MergeRequest: TestGitLabMergeRequest{
			ID:    301,
			IID:   1,
			Title: "Feature: Add new API endpoint",
			URL:   "https://gitlab.com/test/project/-/merge_requests/1",
		},
	}

	sendTestWebhook(payload, "Direct bot mention test")
}

func testGeneralComment(webhookURL string) {
	payload := TestGitLabNotePayload{
		ObjectKind: "note",
		EventType:  "note",
		User: TestGitLabUser{
			ID:       124,
			Username: "developer2",
			Name:     "Developer Two",
			Email:    "dev2@example.com",
		},
		ProjectID: 456,
		Project: TestGitLabProject{
			ID:                456,
			Name:              "test-project",
			PathWithNamespace: "group/test-project",
			WebURL:            "https://gitlab.com/group/test-project",
		},
		Repository: TestGitLabRepository{
			Name:        "test-project",
			URL:         "https://gitlab.com/group/test-project.git",
			Description: "Test project for webhook",
			Homepage:    "https://gitlab.com/group/test-project",
		},
		ObjectAttributes: TestGitLabNoteObjectAttributes{
			ID:           790,
			Note:         "This looks good to me! Nice work on the implementation.",
			NoteableType: "MergeRequest",
			AuthorID:     124,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			ProjectID:    456,
			System:       false,
			Action:       "create",
			URL:          "https://gitlab.com/group/test-project/-/merge_requests/1#note_790",
			DiscussionID: "",
		},
		MergeRequest: TestGitLabMergeRequest{
			ID:    101,
			IID:   1,
			Title: "Add user authentication feature",
			URL:   "https://gitlab.com/group/test-project/-/merge_requests/1",
		},
	}

	sendTestWebhook(payload, "General comment test")
}

func testThreadReply(webhookURL string) {
	payload := TestGitLabNotePayload{
		ObjectKind: "note",
		EventType:  "note",
		User: TestGitLabUser{
			ID:       125,
			Username: "developer3",
			Name:     "Developer Three",
			Email:    "dev3@example.com",
		},
		ProjectID: 456,
		Project: TestGitLabProject{
			ID:                456,
			Name:              "test-project",
			PathWithNamespace: "group/test-project",
			WebURL:            "https://gitlab.com/group/test-project",
		},
		Repository: TestGitLabRepository{
			Name:        "test-project",
			URL:         "https://gitlab.com/group/test-project.git",
			Description: "Test project for webhook",
			Homepage:    "https://gitlab.com/group/test-project",
		},
		ObjectAttributes: TestGitLabNoteObjectAttributes{
			ID:           791,
			Note:         "Thanks for the explanation! That makes sense now.",
			NoteableType: "MergeRequest",
			AuthorID:     125,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			ProjectID:    456,
			System:       false,
			Action:       "create",
			URL:          "https://gitlab.com/group/test-project/-/merge_requests/1#note_791",
			DiscussionID: "abc123def456", // This indicates it's part of a discussion thread
		},
		MergeRequest: TestGitLabMergeRequest{
			ID:    101,
			IID:   1,
			Title: "Add user authentication feature",
			URL:   "https://gitlab.com/group/test-project/-/merge_requests/1",
		},
	}

	sendTestWebhook(payload, "Thread reply test")
}

func testSystemComment(webhookURL string) {
	payload := TestGitLabNotePayload{
		ObjectKind: "note",
		EventType:  "note",
		User: TestGitLabUser{
			ID:       1,
			Username: "root",
			Name:     "Administrator",
			Email:    "admin@example.com",
		},
		ProjectID: 456,
		Project: TestGitLabProject{
			ID:                456,
			Name:              "test-project",
			PathWithNamespace: "group/test-project",
			WebURL:            "https://gitlab.com/group/test-project",
		},
		Repository: TestGitLabRepository{
			Name:        "test-project",
			URL:         "https://gitlab.com/group/test-project.git",
			Description: "Test project for webhook",
			Homepage:    "https://gitlab.com/group/test-project",
		},
		ObjectAttributes: TestGitLabNoteObjectAttributes{
			ID:           792,
			Note:         "assigned to @developer1",
			NoteableType: "MergeRequest",
			AuthorID:     1,
			CreatedAt:    time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
			ProjectID:    456,
			System:       true, // This is a system comment
			Action:       "create",
			URL:          "https://gitlab.com/group/test-project/-/merge_requests/1#note_792",
			DiscussionID: "",
		},
		MergeRequest: TestGitLabMergeRequest{
			ID:    101,
			IID:   1,
			Title: "Add user authentication feature",
			URL:   "https://gitlab.com/group/test-project/-/merge_requests/1",
		},
	}

	sendTestWebhook(payload, "System comment test")
}

func sendTestWebhook(payload TestGitLabNotePayload, testName string) {
	webhookURL := "http://localhost:8888/api/v1/webhooks/gitlab/comments"

	// Convert payload to JSON
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Printf("❌ %s: Failed to marshal JSON: %v\n", testName, err)
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("❌ %s: Failed to create request: %v\n", testName, err)
		return
	}

	// Set headers (mimic GitLab webhook)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitlab-Event", "Note Hook")
	req.Header.Set("X-Gitlab-Instance", "https://gitlab.com")
	req.Header.Set("User-Agent", "GitLab/16.5.0")

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ %s: Failed to send request: %v\n", testName, err)
		return
	}
	defer resp.Body.Close()

	// Read response
	var responseBody bytes.Buffer
	responseBody.ReadFrom(resp.Body)

	// Print results
	if resp.StatusCode == http.StatusOK {
		fmt.Printf("✅ %s: Success (Status: %d)\n", testName, resp.StatusCode)
		fmt.Printf("   Response: %s\n", responseBody.String())
	} else {
		fmt.Printf("❌ %s: Failed (Status: %d)\n", testName, resp.StatusCode)
		fmt.Printf("   Response: %s\n", responseBody.String())
	}
}
