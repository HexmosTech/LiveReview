package gemini_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livereview/internal/ai/gemini"
	"github.com/stretchr/testify/assert"
)

func TestCallGeminiAPI(t *testing.T) {
	// Create a mock Gemini API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "generateContent")
		assert.Contains(t, r.URL.RawQuery, "key=")

		// Return a mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"candidates": [
				{
					"content": {
						"parts": [
							{
								"text": "{\n  \"summary\": \"Test response from mock server\",\n  \"filesChanged\": [\"test/file.go\"],\n  \"comments\": []\n}"
							}
						]
					},
					"finishReason": "STOP"
				}
			]
		}`))
	}))
	defer mockServer.Close()

	// Create a custom HTTP client with our test server
	httpClient := &http.Client{}

	// Create a provider with our mock server and client
	provider := &gemini.GeminiProvider{
		TestableFields: gemini.TestableFields{
			APIKey:      "test-key",
			Model:       "gemini-pro",
			Temperature: 0.2,
			HTTPClient:  httpClient,
		},
	}

	// Override the API URL to use our mock server
	originalAPIURL := gemini.APIURLFormat
	gemini.APIURLFormat = mockServer.URL + "/%s:generateContent?key=%s"
	defer func() { gemini.APIURLFormat = originalAPIURL }()

	// Test the API call
	response, err := provider.TestCallGeminiAPI("Test prompt")

	// Verify response
	assert.NoError(t, err)
	assert.Contains(t, response, "Test response from mock server")
}
