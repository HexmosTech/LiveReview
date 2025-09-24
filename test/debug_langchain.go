package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

// Auth transport that adds Authorization header
type authTransport struct {
	transport http.RoundTripper
	apiKey    string
}

func (a *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request and add auth header
	newReq := req.Clone(req.Context())
	newReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	return a.transport.RoundTrip(newReq)
}

// Debug transport that logs all HTTP requests
type debugTransport struct {
	transport http.RoundTripper
}

func (d *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	fmt.Println("=== LANGCHAIN HTTP REQUEST ===")
	fmt.Printf("Method: %s\n", req.Method)
	fmt.Printf("URL: %s\n", req.URL.String())
	fmt.Println("Headers:")
	for name, values := range req.Header {
		fmt.Printf("  %s: %v\n", name, values)
	}

	// Check specifically for Authorization header
	if auth := req.Header.Get("Authorization"); auth != "" {
		fmt.Printf("✓ Authorization header present: %s\n", auth[:20]+"...")
	} else {
		fmt.Printf("❌ Authorization header MISSING\n")
	}

	// Dump the full request
	if req.Body != nil {
		dump, err := httputil.DumpRequest(req, true)
		if err == nil {
			fmt.Printf("Full request dump:\n%s\n", string(dump))
		}
	}
	fmt.Println("================================") // Make the actual request
	resp, err := d.transport.RoundTrip(req)

	if err != nil {
		fmt.Printf("REQUEST ERROR: %v\n", err)
		return resp, err
	}

	fmt.Println("=== LANGCHAIN HTTP RESPONSE ===")
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Println("Response Headers:")
	for name, values := range resp.Header {
		fmt.Printf("  %s: %v\n", name, values)
	}
	fmt.Println("===============================")

	return resp, err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run debug_langchain.go [generate|chat]")
		os.Exit(1)
	}

	testType := os.Args[1]

	// Your Ollama server configuration
	baseURL := "http://139.59.48.31/ollama"
	model := "mistral:7b"
	apiKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6Ijc5NWQ5ZGFkLTg3OGYtNGYxNy05ZTM3LWNiYjkxYTE5YTY5OSJ9.fgzS3SxaLc17_BFS-Vq8OTtdNxbnu8FK_CJ0lutmB7s"

	fmt.Printf("=== LangChain Debug Test: %s ===\n", testType)
	fmt.Printf("Server: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("API Key length: %d\n", len(apiKey))
	fmt.Println()

	// Create debug HTTP client with auth header injection
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &debugTransport{
			transport: &authTransport{
				transport: &http.Transport{
					ResponseHeaderTimeout: 60 * time.Second,
					TLSHandshakeTimeout:   10 * time.Second,
				},
				apiKey: apiKey,
			},
		},
	}

	// Create Ollama client
	llm, err := ollama.New(
		ollama.WithServerURL(baseURL),
		ollama.WithModel(model),
		ollama.WithHTTPClient(httpClient),
	)
	if err != nil {
		log.Fatalf("Failed to create Ollama client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch testType {
	case "generate":
		testGenerate(ctx, llm)
	case "chat":
		testChat(ctx, llm)
	default:
		fmt.Println("Invalid test type. Use 'generate' or 'chat'")
		os.Exit(1)
	}
}

func testGenerate(ctx context.Context, llm llms.Model) {
	fmt.Println("=== Testing Generate (Non-streaming) ===")

	prompt := "Say hello"

	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println("Sending request...")

	start := time.Now()
	response, err := llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	})
	duration := time.Since(start)

	if err != nil {
		log.Fatalf("Generate failed: %v", err)
	}

	fmt.Printf("✓ Generate completed in %v\n", duration)
	fmt.Printf("Response: %s\n", response.Choices[0].Content)
}

func testChat(ctx context.Context, llm llms.Model) {
	fmt.Println("=== Testing Chat (Streaming) ===")

	prompt := "Say hello"

	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println("Starting streaming request...")

	start := time.Now()
	chunkCount := 0
	var fullResponse string

	_, err := llm.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, prompt),
	}, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		chunkCount++
		chunkStr := string(chunk)
		fullResponse += chunkStr

		elapsed := time.Since(start)
		fmt.Printf("[CHUNK %d] [%v] %q\n", chunkCount, elapsed, chunkStr)

		return nil
	}))

	duration := time.Since(start)

	if err != nil {
		log.Fatalf("Chat streaming failed: %v", err)
	}

	fmt.Printf("✓ Chat streaming completed in %v\n", duration)
	fmt.Printf("Total chunks received: %d\n", chunkCount)
	fmt.Printf("Full response: %s\n", fullResponse)
}
