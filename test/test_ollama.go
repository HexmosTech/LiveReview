package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_ollama.go [generate|chat]")
		os.Exit(1)
	}

	testType := os.Args[1]

	// Your Ollama server configuration
	baseURL := "http://139.59.48.31/ollama"
	model := "mistral:7b"
	// Test API key for internal Ollama server - hardcoded for testing only
	apiKey := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6Ijc5NWQ5ZGFkLTg3OGYtNGYxNy05ZTM3LWNiYjkxYTE5YTY5OSJ9.fgzS3SxaLc17_BFS-Vq8OTtdNxbnu8FK_CJ0lutmB7s"

	fmt.Printf("=== Ollama Test Case: %s ===\n", testType)
	fmt.Printf("Server: %s\n", baseURL)
	fmt.Printf("Model: %s\n", model)
	fmt.Printf("API Key length: %d\n", len(apiKey))
	fmt.Println()

	// Create custom HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 60 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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

	prompt := "What is the capital of France? Please respond in one sentence."

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

	prompt := "Explain what HTTP status code 200 means in exactly 2 sentences."

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
