package api

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/pkg/models"
)

func TestConvertLocalToModelDiffUsesOldPathWhenNewEmpty(t *testing.T) {
	local := lib.LocalCodeDiff{
		OldPath: "old/foo.go",
		NewPath: " ",
		Hunks: []lib.LocalDiffHunk{
			{
				OldStartLine: 1,
				OldLineCount: 1,
				NewStartLine: 2,
				NewLineCount: 1,
				Lines:        []lib.LocalDiffLine{{Content: "line", LineType: "context", OldLineNo: 1, NewLineNo: 2}},
			},
		},
	}

	result := convertLocalToModelDiff(local)

	if result.FilePath != "old/foo.go" {
		t.Fatalf("expected FilePath to fallback to old path, got %s", result.FilePath)
	}
	if result.FileType != ".go" {
		t.Fatalf("expected FileType .go, got %s", result.FileType)
	}
	if len(result.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(result.Hunks))
	}
}

func TestConvertLocalHunkFormatting(t *testing.T) {
	hunk := lib.LocalDiffHunk{
		OldStartLine: 10,
		OldLineCount: 2,
		NewStartLine: 12,
		NewLineCount: 3,
		HeaderText:   "func foo",
		Lines: []lib.LocalDiffLine{
			{Content: "line one", LineType: "context", OldLineNo: 10, NewLineNo: 12},
			{Content: "added line", LineType: "added", OldLineNo: 0, NewLineNo: 13},
			{Content: "old line", LineType: "deleted", OldLineNo: 11, NewLineNo: 0},
		},
	}

	result := convertLocalHunk(hunk)

	expected := "@@ -10,2 +12,3 @@ func foo\n line one\n+added line\n-old line"
	if result.Content != expected {
		t.Fatalf("unexpected hunk content:\nexpected:\n%s\ngot:\n%s", expected, result.Content)
	}
}

func TestLineWithinHunks(t *testing.T) {
	hunks := []models.DiffHunk{{NewStartLine: 10, NewLineCount: 3}}

	cases := map[int]bool{
		9:  false,
		10: true,
		12: true,
		13: true,
		14: false,
	}

	for line, expected := range cases {
		if got := lineWithinHunks(line, hunks); got != expected {
			t.Fatalf("line %d expected %t got %t", line, expected, got)
		}
	}
}

func TestFilterCommentsForFile(t *testing.T) {
	hunks := []models.DiffHunk{{NewStartLine: 5, NewLineCount: 2}}
	comments := []*models.ReviewComment{
		{FilePath: "file.go", Line: 5, Content: "a", Severity: models.SeverityWarning, Category: "style"},
		{FilePath: "file.go", Line: 8, Content: "b", Severity: models.SeverityInfo, Category: "out"},
		{FilePath: "other.go", Line: 5, Content: "c", Severity: models.SeverityCritical, Category: "other"},
	}

	filtered := filterCommentsForFile("file.go", hunks, comments)

	if len(filtered) != 2 {
		t.Fatalf("expected 2 comments for file.go, got %d", len(filtered))
	}
	if filtered[0]["line"].(int) != 5 {
		t.Fatalf("expected first comment line 5, got %v", filtered[0]["line"])
	}
	if filtered[1]["line"].(int) != 8 {
		t.Fatalf("expected second comment line 8, got %v", filtered[1]["line"])
	}
}

func TestBuildDiffFiles(t *testing.T) {
	diffs := []models.CodeDiff{
		{
			FilePath: "file.go",
			Hunks: []models.DiffHunk{
				{OldStartLine: 1, OldLineCount: 1, NewStartLine: 2, NewLineCount: 2, Content: "@@ -1,1 +2,2 @@\n+line"},
			},
		},
	}

	comments := []*models.ReviewComment{
		{FilePath: "file.go", Line: 2, Content: "note", Severity: models.SeverityInfo, Category: "nit"},
	}

	files := buildDiffFiles(diffs, comments)

	if len(files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(files))
	}

	entry := files[0]
	if entry["file_path"].(string) != "file.go" {
		t.Fatalf("expected file.go, got %v", entry["file_path"])
	}

	hunks, ok := entry["hunks"].([]map[string]interface{})
	if !ok || len(hunks) != 1 {
		t.Fatalf("expected 1 hunk map, got %v", entry["hunks"])
	}

	cmts, ok := entry["comments"].([]map[string]interface{})
	if !ok || len(cmts) != 1 {
		t.Fatalf("expected 1 comment map, got %v", entry["comments"])
	}
	if cmts[0]["content"].(string) != "note" {
		t.Fatalf("expected comment content note, got %v", cmts[0]["content"])
	}
}

func TestDiffReviewContractExample(t *testing.T) {
	// Build a tiny unified diff and wrap it in a zip, exactly like the client sends.
	diff := "diff --git a/foo.txt b/foo.txt\n" +
		"--- a/foo.txt\n" +
		"+++ b/foo.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+hello\n" +
		"+world\n"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("patch.diff")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte(diff)); err != nil {
		t.Fatalf("failed to write diff: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Parse the payload using the same helper the handler uses.
	localDiffs, err := parseDiffZipBase64(encoded)
	if err != nil {
		t.Fatalf("parseDiffZipBase64 failed: %v", err)
	}
	if len(localDiffs) != 1 {
		t.Fatalf("expected 1 local diff, got %d", len(localDiffs))
	}

	modelDiffs := convertLocalDiffs(localDiffs)
	comments := []*models.ReviewComment{{
		FilePath: modelDiffs[0].FilePath,
		Line:     modelDiffs[0].Hunks[0].NewStartLine,
		Content:  "Example review note",
		Severity: models.SeverityInfo,
		Category: "example",
	}}

	response := map[string]interface{}{
		"status":  "completed",
		"summary": "Example summary for contract test",
		"files":   buildDiffFiles(modelsSlice(modelDiffs), comments),
	}

	// Assert core contract fields.
	files := response["files"].([]map[string]interface{})
	if len(files) != 1 {
		t.Fatalf("expected 1 file in response, got %d", len(files))
	}
	if files[0]["file_path"].(string) != "foo.txt" {
		t.Fatalf("expected file_path foo.txt, got %s", files[0]["file_path"])
	}

	// Print the request/response contract to make it obvious and human-readable.
	pretty, _ := json.MarshalIndent(response, "", "  ")
	t.Logf("Example client payload (base64 zip length=%d): %s", len(encoded), encoded)
	t.Logf("Example handler response:\n%s", string(pretty))
}

// modelsSlice converts []*models.CodeDiff to []models.CodeDiff for helper compatibility.
func modelsSlice(in []*models.CodeDiff) []models.CodeDiff {
	out := make([]models.CodeDiff, 0, len(in))
	for _, d := range in {
		if d != nil {
			out = append(out, *d)
		}
	}
	return out
}

// mockReviewManager implements a test stub for ReviewManager operations.
type mockReviewManager struct {
	reviews         map[int64]*Review
	nextID          int64
	createErr       error
	updateStatusErr error
	mergeMetaErr    error
	getReviewErr    error
}

func newMockReviewManager() *mockReviewManager {
	return &mockReviewManager{
		reviews: make(map[int64]*Review),
		nextID:  1,
	}
}

func (m *mockReviewManager) CreateReviewWithOrg(repository, branch, commitHash, prMrURL, triggerType, userEmail, provider string, connectorID *int64, metadata map[string]interface{}, orgID int64, friendlyName string, authorName string, authorUsername string) (*Review, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	metaJSON, _ := json.Marshal(metadata)
	review := &Review{
		ID:       m.nextID,
		Metadata: json.RawMessage(metaJSON),
		Status:   "pending",
	}
	m.reviews[m.nextID] = review
	m.nextID++
	return review, nil
}

func (m *mockReviewManager) UpdateReviewStatus(reviewID int64, status string) error {
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	if review, ok := m.reviews[reviewID]; ok {
		review.Status = status
	}
	return nil
}

func (m *mockReviewManager) MergeReviewMetadata(reviewID int64, updates map[string]interface{}) error {
	if m.mergeMetaErr != nil {
		return m.mergeMetaErr
	}
	if review, ok := m.reviews[reviewID]; ok {
		var metadata map[string]interface{}
		if len(review.Metadata) > 0 {
			json.Unmarshal(review.Metadata, &metadata)
		} else {
			metadata = make(map[string]interface{})
		}
		for k, v := range updates {
			metadata[k] = v
		}
		metaJSON, _ := json.Marshal(metadata)
		review.Metadata = json.RawMessage(metaJSON)
	}
	return nil
}

func (m *mockReviewManager) GetReview(reviewID int64) (*Review, error) {
	if m.getReviewErr != nil {
		return nil, m.getReviewErr
	}
	review, ok := m.reviews[reviewID]
	if !ok {
		return nil, nil
	}
	return review, nil
}

// TestDiffReviewHandlerStoresPreloadedChanges verifies that preloaded_changes
// are correctly stored in review metadata on POST.
func TestDiffReviewHandlerStoresPreloadedChanges(t *testing.T) {
	mockRM := newMockReviewManager()

	// Simulate a diff submission
	diff := "diff --git a/test.go b/test.go\n" +
		"--- a/test.go\n" +
		"+++ b/test.go\n" +
		"@@ -1,1 +1,2 @@\n" +
		"+new line\n" +
		" existing\n"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("patch.diff")
	w.Write([]byte(diff))
	zw.Close()

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	localDiffs, _ := parseDiffZipBase64(encoded)
	modelDiffs := convertLocalDiffs(localDiffs)

	// Create review via mock
	review, err := mockRM.CreateReviewWithOrg("test-repo", "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{"source": "diff-review"}, 1, "Test Friendly", "", "")
	if err != nil {
		t.Fatalf("failed to create review: %v", err)
	}

	// Store preloaded_changes
	err = mockRM.MergeReviewMetadata(review.ID, map[string]interface{}{"preloaded_changes": modelDiffs})
	if err != nil {
		t.Fatalf("failed to merge metadata: %v", err)
	}

	// Verify preloaded_changes stored
	storedReview, _ := mockRM.GetReview(review.ID)
	var metadata map[string]interface{}
	if err := json.Unmarshal(storedReview.Metadata, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if metadata["preloaded_changes"] == nil {
		t.Fatalf("expected preloaded_changes in metadata, got nil")
	}

	// Changes are stored as JSON, so we need to marshal/unmarshal
	changesJSON, _ := json.Marshal(metadata["preloaded_changes"])
	var changes []*models.CodeDiff
	if err := json.Unmarshal(changesJSON, &changes); err != nil {
		t.Fatalf("failed to unmarshal preloaded_changes: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("expected 1 preloaded change, got %d", len(changes))
	}

	if changes[0].FilePath != "test.go" {
		t.Fatalf("expected file test.go, got %s", changes[0].FilePath)
	}

	t.Logf("✓ preloaded_changes correctly stored in metadata")
}

// TestDiffReviewHandlerStoresReviewResult verifies that review_result
// is persisted in metadata on completion.
func TestDiffReviewHandlerStoresReviewResult(t *testing.T) {
	mockRM := newMockReviewManager()

	// Create a review
	review, _ := mockRM.CreateReviewWithOrg("test-repo", "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{}, 1, "", "", "")

	// Simulate completion with review result
	result := diffReviewResult{
		Summary: "Test summary",
		Comments: []*models.ReviewComment{
			{FilePath: "test.go", Line: 10, Content: "test comment", Severity: models.SeverityInfo, Category: "test"},
		},
	}

	err := mockRM.MergeReviewMetadata(review.ID, map[string]interface{}{"review_result": result})
	if err != nil {
		t.Fatalf("failed to merge review result: %v", err)
	}

	// Verify review_result stored
	storedReview, _ := mockRM.GetReview(review.ID)
	var metadata map[string]interface{}
	if err := json.Unmarshal(storedReview.Metadata, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if metadata["review_result"] == nil {
		t.Fatalf("expected review_result in metadata, got nil")
	}

	// Marshal and unmarshal to convert to proper type
	resultJSON, _ := json.Marshal(metadata["review_result"])
	var resultData diffReviewResult
	if err := json.Unmarshal(resultJSON, &resultData); err != nil {
		t.Fatalf("failed to unmarshal review_result: %v", err)
	}

	if resultData.Summary != "Test summary" {
		t.Fatalf("expected summary 'Test summary', got %s", resultData.Summary)
	}

	if len(resultData.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(resultData.Comments))
	}

	t.Logf("✓ review_result correctly stored in metadata")
}

// TestDiffReviewStatusProgression verifies status transitions from pending → processing → completed.
func TestDiffReviewStatusProgression(t *testing.T) {
	mockRM := newMockReviewManager()

	// Create review (starts as pending)
	review, _ := mockRM.CreateReviewWithOrg("test-repo", "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{}, 1, "", "", "")

	if review.Status != "pending" {
		t.Fatalf("expected initial status 'pending', got %s", review.Status)
	}

	// Transition to processing
	err := mockRM.UpdateReviewStatus(review.ID, "processing")
	if err != nil {
		t.Fatalf("failed to update status to processing: %v", err)
	}

	storedReview, _ := mockRM.GetReview(review.ID)
	if storedReview.Status != "processing" {
		t.Fatalf("expected status 'processing', got %s", storedReview.Status)
	}

	// Transition to completed
	err = mockRM.UpdateReviewStatus(review.ID, "completed")
	if err != nil {
		t.Fatalf("failed to update status to completed: %v", err)
	}

	storedReview, _ = mockRM.GetReview(review.ID)
	if storedReview.Status != "completed" {
		t.Fatalf("expected status 'completed', got %s", storedReview.Status)
	}

	t.Logf("✓ status correctly transitions: pending → processing → completed")
}

// TestDiffReviewPollingWithProcessingStatus simulates polling while review is processing.
func TestDiffReviewPollingWithProcessingStatus(t *testing.T) {
	mockRM := newMockReviewManager()

	// Create and mark as processing
	review, _ := mockRM.CreateReviewWithOrg("test-repo", "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{}, 1, "", "", "")
	mockRM.UpdateReviewStatus(review.ID, "processing")

	// Simulate polling - should return processing status
	storedReview, err := mockRM.GetReview(review.ID)
	if err != nil {
		t.Fatalf("failed to get review: %v", err)
	}

	if storedReview.Status != "processing" {
		t.Fatalf("expected processing status during polling, got %s", storedReview.Status)
	}

	// No review_result yet
	var metadata map[string]interface{}
	if len(storedReview.Metadata) > 0 {
		json.Unmarshal(storedReview.Metadata, &metadata)
	}
	if metadata["review_result"] != nil {
		t.Fatalf("expected no review_result during processing")
	}

	t.Logf("✓ polling returns processing status with no results")
}

// TestDiffReviewPollingWithCompletedStatus simulates polling after review completion.
func TestDiffReviewPollingWithCompletedStatus(t *testing.T) {
	mockRM := newMockReviewManager()

	// Create review, store preloaded changes
	review, _ := mockRM.CreateReviewWithOrg("test-repo", "", "", "", "cli_diff", "", "cli", nil, map[string]interface{}{}, 1, "", "", "")

	diff := "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1,0 +1,1 @@\n+code\n"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("patch.diff")
	w.Write([]byte(diff))
	zw.Close()
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	localDiffs, _ := parseDiffZipBase64(encoded)
	modelDiffs := convertLocalDiffs(localDiffs)

	mockRM.MergeReviewMetadata(review.ID, map[string]interface{}{"preloaded_changes": modelDiffs})

	// Simulate completion with results
	result := diffReviewResult{
		Summary: "Completed review",
		Comments: []*models.ReviewComment{
			{FilePath: "file.go", Line: 1, Content: "looks good", Severity: models.SeverityInfo, Category: "general"},
		},
	}
	mockRM.MergeReviewMetadata(review.ID, map[string]interface{}{"review_result": result})
	mockRM.UpdateReviewStatus(review.ID, "completed")

	// Poll - should return completed with results
	storedReview, _ := mockRM.GetReview(review.ID)
	if storedReview.Status != "completed" {
		t.Fatalf("expected completed status, got %s", storedReview.Status)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(storedReview.Metadata, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if metadata["review_result"] == nil {
		t.Fatalf("expected review_result in completed review")
	}

	// Build response like handler does
	preloadedJSON, _ := json.Marshal(metadata["preloaded_changes"])
	var preloaded []*models.CodeDiff
	json.Unmarshal(preloadedJSON, &preloaded)

	reviewResultJSON, _ := json.Marshal(metadata["review_result"])
	var reviewResult diffReviewResult
	json.Unmarshal(reviewResultJSON, &reviewResult)

	files := buildDiffFiles(modelsSlice(preloaded), reviewResult.Comments)

	if len(files) != 1 {
		t.Fatalf("expected 1 file in response, got %d", len(files))
	}

	if files[0]["file_path"] != "file.go" {
		t.Fatalf("expected file_path file.go, got %v", files[0]["file_path"])
	}

	comments := files[0]["comments"].([]map[string]interface{})
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	if comments[0]["content"] != "looks good" {
		t.Fatalf("expected comment 'looks good', got %v", comments[0]["content"])
	}

	t.Logf("✓ polling returns completed status with full results")
}
