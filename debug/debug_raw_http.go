//go:build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// maskSecretHeader keeps this debug tool's output safe to paste into a
// ticket or chat by masking any header that carries a credential.
func maskSecretHeader(name string, values []string) []string {
	lower := strings.ToLower(name)
	if lower != "authorization" && lower != "x-api-key" && !strings.Contains(lower, "token") && !strings.Contains(lower, "secret") {
		return values
	}
	masked := make([]string, len(values))
	for i, v := range values {
		if len(v) <= 12 {
			masked[i] = "[HIDDEN]"
		} else {
			masked[i] = v[:8] + "...[HIDDEN]"
		}
	}
	return masked
}

func main() {
	// Test the raw HTTP request to see what Ollama is actually returning
	baseURL := strings.TrimSpace(os.Getenv("LIVEREVIEW_DEBUG_OLLAMA_CHAT_URL"))
	if baseURL == "" {
		fmt.Println("LIVEREVIEW_DEBUG_OLLAMA_CHAT_URL is required")
		return
	}
	apiKey := strings.TrimSpace(os.Getenv("LIVEREVIEW_DEBUG_OLLAMA_API_KEY"))
	if apiKey == "" {
		fmt.Println("LIVEREVIEW_DEBUG_OLLAMA_API_KEY is required")
		return
	}

	// Create the request body (same as curl)
	requestBody := `{
		"model": "mistral:7b",
		"messages": [
			{
				"role": "user",
				"content": "What is the capital of France?"
			}
		],
		"stream": false
	}`

	fmt.Println("=== Raw HTTP Request Test ===")
	fmt.Printf("URL: %s\n", baseURL)
	fmt.Printf("API Key length: %d\n", len(apiKey))
	fmt.Println()

	// Create HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("POST", baseURL, strings.NewReader(requestBody))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	fmt.Println("Request headers:")
	for name, values := range req.Header {
		fmt.Printf("  %s: %v\n", name, maskSecretHeader(name, values))
	}
	fmt.Printf("Request body: %s\n", requestBody)
	fmt.Println()

	// Send request
	fmt.Println("Sending request...")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)
	fmt.Printf("Response received in %v\n", duration)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Println("Response headers:")
	for name, values := range resp.Header {
		fmt.Printf("  %s: %v\n", name, values)
	}
	fmt.Println()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Printf("Response body length: %d bytes\n", len(body))
	fmt.Printf("Response body (first 500 chars): %q\n", string(body[:min(len(body), 500)]))

	if len(body) > 500 {
		fmt.Printf("Response body (last 100 chars): %q\n", string(body[len(body)-100:]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

