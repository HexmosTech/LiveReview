package gemini_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/batch"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

// stubSummaryLLM is a minimal llms.Model: AggregateAndCombineOutputs always
// makes one extra LLM call (via llms.GenerateFromSinglePrompt) to synthesize
// the overall review summary from per-file technical summaries, independent
// of the per-batch REST calls the gemini.APIURLFormat mock server below
// handles. This stub just satisfies that call deterministically.
type stubSummaryLLM struct{}

func (stubSummaryLLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "stub summary"}}}, nil
}

func (stubSummaryLLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "stub summary", nil
}

const geminiBatchOKBody = `{
	"candidates": [
		{
			"content": {
				"parts": [
					{
						"text": "{\n  \"summary\": \"Looks fine\",\n  \"filesChanged\": [\"a.go\"],\n  \"comments\": [{\"filePath\": \"a.go\", \"lineNumber\": 1, \"content\": \"nit: rename this\", \"severity\": \"info\", \"confidence\": \"high\", \"type\": \"style\", \"category\": \"review\", \"subcategory\": \"\", \"isInternal\": false}]\n}"
					}
				]
			},
			"finishReason": "STOP"
		}
	]
}`

// twoBatchDiffs builds two CodeDiffs whose combined token count exceeds a
// small MaxBatchTokens but whose individual hunks don't, forcing
// BatchInputs to split them into exactly two single-diff batches.
func twoBatchDiffs() []*models.CodeDiff {
	return []*models.CodeDiff{
		{
			FilePath: "a.go",
			Hunks:    []models.DiffHunk{{Content: "line one two three"}},
		},
		{
			FilePath: "b.go",
			Hunks:    []models.DiffHunk{{Content: "line four five six"}},
		},
	}
}

func newTolerantBatchProcessor() *batch.BatchProcessor {
	bp := batch.DefaultBatchProcessor()
	bp.MaxBatchTokens = 8 // small enough that the two diffs above land in separate batches
	return bp
}

// readAndRestoreBody reads an HTTP request body for content inspection. The
// mock server below never reads it again afterward, so there's nothing to
// restore in practice - name kept descriptive of what it does either way.
func readAndRestoreBody(r *http.Request) string {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := r.Body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

// TestReviewCodeWithBatchingTolerateOneFailedBatch is a regression test for
// the "Findings disappear when one batch fails" bug: a single bad batch must
// not discard every other batch's already-good comments and fail the whole
// review - only abort if every batch fails. Mirrors
// internal/ai/langchain/provider.go's ReviewCodeWithBatching, which already
// tolerates partial failures; this is the equivalent for the Gemini path,
// which previously hard-failed on the first bad batch.
func TestReviewCodeWithBatchingTolerateOneFailedBatch(t *testing.T) {
	// Fail every request for the batch containing "a.go" (even across the
	// task queue's own retries - internal/batch/task_queue.go retries up to
	// 3 times by default), always succeed for "b.go"'s batch. Batches run
	// concurrently across worker goroutines, so request ORDER isn't
	// deterministic - keying off request content is.
	var aCalls, bCalls int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readAndRestoreBody(r)
		if strings.Contains(body, "a.go") {
			atomic.AddInt32(&aCalls, 1)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "simulated transient failure"}`))
			return
		}
		atomic.AddInt32(&bCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(geminiBatchOKBody))
	}))
	defer mockServer.Close()

	provider := &gemini.GeminiProvider{
		TestableFields: gemini.TestableFields{
			APIKey:     "test-key",
			Model:      "gemini-pro",
			HTTPClient: &http.Client{},
		},
	}
	provider.SetLLM(stubSummaryLLM{})

	originalAPIURL := gemini.APIURLFormat
	gemini.APIURLFormat = mockServer.URL + "/%s:generateContent?key=%s"
	defer func() { gemini.APIURLFormat = originalAPIURL }()

	result, err := provider.ReviewCodeWithBatching(context.Background(), twoBatchDiffs(), newTolerantBatchProcessor())

	require.NoError(t, err, "one failed batch out of two must not fail the whole review")
	require.NotNil(t, result)
	assert.Positive(t, atomic.LoadInt32(&aCalls), "the failing batch should have been attempted")
	assert.Positive(t, atomic.LoadInt32(&bCalls), "the succeeding batch should have been attempted")
	assert.NotEmpty(t, result.Comments, "the successful batch's comments must still be present")
	assert.True(t, strings.Contains(result.Summary, "Partial Review Results"),
		"summary should warn that this was a partial result, got: %q", result.Summary)
}

// TestReviewCodeWithBatchingFailsWhenAllBatchesFail confirms the tolerance
// above doesn't swallow a total failure: if every batch fails, the review
// must still correctly return an error, not a silently-empty success.
func TestReviewCodeWithBatchingFailsWhenAllBatchesFail(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "simulated failure"}`))
	}))
	defer mockServer.Close()

	provider := &gemini.GeminiProvider{
		TestableFields: gemini.TestableFields{
			APIKey:     "test-key",
			Model:      "gemini-pro",
			HTTPClient: &http.Client{},
		},
	}
	provider.SetLLM(stubSummaryLLM{})

	originalAPIURL := gemini.APIURLFormat
	gemini.APIURLFormat = mockServer.URL + "/%s:generateContent?key=%s"
	defer func() { gemini.APIURLFormat = originalAPIURL }()

	result, err := provider.ReviewCodeWithBatching(context.Background(), twoBatchDiffs(), newTolerantBatchProcessor())

	require.Error(t, err, "a review where every batch failed must still return an error")
	assert.Nil(t, result)
	assert.True(t, strings.Contains(err.Error(), "all") && strings.Contains(err.Error(), "batches failed"),
		"error should say every batch failed, got: %v", err)
}
