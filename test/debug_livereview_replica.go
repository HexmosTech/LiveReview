package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

// Replicate LiveReview's authTransport exactly
type authTransport struct {
	Transport http.RoundTripper
	token     string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add Authorization header
	req.Header.Set("Authorization", "Bearer "+t.token)

	// Debug logging - show full request details including body
	fmt.Printf("[HTTP DEBUG] Request URL: %s\n", req.URL.String())
	fmt.Printf("[HTTP DEBUG] Request Method: %s\n", req.Method)
	fmt.Printf("[HTTP DEBUG] Authorization header set with token length: %d\n", len(t.token))
	fmt.Printf("[HTTP DEBUG] All Headers:\n")
	for k, v := range req.Header {
		fmt.Printf("  %s: %v\n", k, v)
	}

	// Make the request
	resp, err := t.Transport.RoundTrip(req)
	return resp, err
}

func main() {
	// Replicate LiveReview's exact setup
	baseURL := "http://139.59.48.31/ollama/api" // What LiveReview logs show
	model := "mistral:7b"
	apiKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6Ijc5NWQ5ZGFkLTg3OGYtNGYxNy05ZTM3LWNiYjkxYTE5YTY5OSJ9.fgzS3SxaLc17_BFS-Vq8OTtdNxbnu8FK_CJ0lutmB7s"

	// Clean up base URL - remove trailing /api/ if present for Ollama (same as LiveReview)
	cleanURL := strings.TrimSuffix(baseURL, "/api/")
	cleanURL = strings.TrimSuffix(cleanURL, "/api")
	cleanURL = strings.TrimSuffix(cleanURL, "/")

	fmt.Printf("[OLLAMA INIT] Original base URL: %s\n", baseURL)
	fmt.Printf("[OLLAMA INIT] Cleaned base URL: %s\n", cleanURL)

	// Create a custom HTTP client with Authorization header and proper timeouts (same as LiveReview)
	client := &http.Client{
		Timeout: 5 * time.Minute, // Overall request timeout
	}

	// Create a custom transport that adds the Authorization header with connection timeouts
	transport := &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second, // TLS timeout
		ResponseHeaderTimeout: 60 * time.Second, // Time to wait for response headers
		ExpectContinueTimeout: 1 * time.Second,  // Expect 100-continue timeout
	}
	client.Transport = &authTransport{
		Transport: transport,
		token:     apiKey,
	}

	fmt.Printf("[OLLAMA INIT] Custom HTTP client configured with timeouts\n")

	// Create Ollama LLM exactly like LiveReview
	llm, err := ollama.New(
		ollama.WithModel(model),
		ollama.WithServerURL(cleanURL),
		ollama.WithHTTPClient(client),
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create Ollama LLM: %v", err))
	}

	fmt.Printf("[LANGCHAIN INIT] Initializing Ollama LLM with model: %s\n", model)

	// Create a simple prompt like LiveReview uses
	prompt := "Review this code change: func hello() { fmt.Println(\"Hello world\") }. Respond with JSON."

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Replicate LiveReview's streaming function exactly
	var totalChunks int = 0
	var lastActivity time.Time = time.Now()
	var responseBuilder strings.Builder

	streamingFunc := func(ctx context.Context, chunk []byte) error {
		chunkStr := string(chunk)
		totalChunks++
		lastActivity = time.Now()

		// Debug: Log raw chunk info to diagnose streaming issues (same as LiveReview)
		fmt.Printf("[STREAM DEBUG] Received chunk %d: length=%d, content=%q\n", totalChunks, len(chunk), chunkStr)

		// Add to response builder
		responseBuilder.WriteString(chunkStr)
		return nil
	}

	// Start activity monitor (same as LiveReview)
	activityDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-activityDone:
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				timeSinceActivity := time.Since(lastActivity)
				fmt.Printf("\n[STREAM PROGRESS] Elapsed: %v | Chunks: %d | Last activity: %v ago\n", elapsed.Truncate(time.Second), totalChunks, timeSinceActivity.Truncate(time.Second))
				if timeSinceActivity > 30*time.Second {
					fmt.Printf("[STREAM MONITOR] No activity for %v (chunks received: %d)\n", timeSinceActivity, totalChunks)
				}
			}
		}
	}()

	// Call the LLM with streaming exactly like LiveReview
	fmt.Printf("[STREAM START] Beginning streaming response...\n")
	fmt.Printf("[STREAM DEBUG] Calling GenerateFromSinglePrompt with streaming function\n")

	_, err = llms.GenerateFromSinglePrompt(
		ctx,
		llm,
		prompt,
		llms.WithStreamingFunc(streamingFunc),
	)

	// Stop activity monitor
	close(activityDone)

	fmt.Printf("[STREAM DEBUG] GenerateFromSinglePrompt returned, err=%v\n", err)
	fmt.Printf("Total chunks received: %d\n", totalChunks)
	fmt.Printf("Response: %s\n", responseBuilder.String())

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}
}
