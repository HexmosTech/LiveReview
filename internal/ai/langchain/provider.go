package langchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/prompts"
	"github.com/livereview/pkg/models"
)

// LangchainProvider implements the AI Provider interface using langchain abstractions
type LangchainProvider struct {
	llm          llms.Model
	apiKey       string
	modelName    string
	maxTokens    int
	providerType string // NEW: Provider type (gemini, ollama, openai, etc.)
	baseURL      string // NEW: Base URL for custom endpoints
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatHunkWithLineNumbers processes a diff hunk to add line number annotations
// This is CRITICAL for proper comment positioning - handles multiple hunks properly
func (p *LangchainProvider) formatHunkWithLineNumbers(hunk models.DiffHunk) string {
	logger := logging.GetCurrentLogger()

	// Extract the original hunk content
	content := hunk.Content
	lines := strings.Split(content, "\n")

	if logger != nil {
		logger.Log("Processing hunk with %d lines of content", len(lines))
	}

	// Find all hunk headers in the content to handle multiple hunks
	var hunkBoundaries []int
	headerPattern := `@@ -(\d+),(\d+) \+(\d+),(\d+) @@`
	re := regexp.MustCompile(headerPattern)

	for i, line := range lines {
		if re.MatchString(line) {
			hunkBoundaries = append(hunkBoundaries, i)
			if logger != nil {
				logger.Log("Found hunk header at line %d: %s", i, line)
			}
		}
	}

	// If we don't have any hunk headers, process as single hunk with metadata
	if len(hunkBoundaries) == 0 {
		if logger != nil {
			logger.Log("No @@ headers found, using hunk metadata: old=%d+%d, new=%d+%d",
				hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount)
		}
		return p.formatSingleHunk(lines, hunk.OldStartLine, hunk.NewStartLine, "")
	}

	// If we have only one hunk header, process normally
	if len(hunkBoundaries) == 1 {
		headerIdx := hunkBoundaries[0]
		headerLine := lines[headerIdx]

		// Extract line numbers from header
		matches := re.FindStringSubmatch(headerLine)
		if matches == nil {
			if logger != nil {
				logger.Log("Failed to parse header line: %s", headerLine)
			}
			return p.formatSingleHunk(lines, hunk.OldStartLine, hunk.NewStartLine, "")
		}

		oldStart, _ := strconv.Atoi(matches[1])
		newStart, _ := strconv.Atoi(matches[3])

		if logger != nil {
			logger.Log("Single hunk: oldStart=%d, newStart=%d", oldStart, newStart)
		}

		return p.formatSingleHunk(lines[headerIdx+1:], oldStart, newStart, headerLine)
	}

	// Handle multiple hunks - process each separately
	if logger != nil {
		logger.Log("Found multiple hunks (%d), processing separately", len(hunkBoundaries))
	}

	var result strings.Builder
	for i, startIdx := range hunkBoundaries {
		endIdx := len(lines)
		if i < len(hunkBoundaries)-1 {
			endIdx = hunkBoundaries[i+1]
		}

		headerLine := lines[startIdx]
		matches := re.FindStringSubmatch(headerLine)
		if matches == nil {
			if logger != nil {
				logger.Log("Failed to parse header line in multi-hunk: %s", headerLine)
			}
			continue
		}

		oldStart, _ := strconv.Atoi(matches[1])
		newStart, _ := strconv.Atoi(matches[3])

		if logger != nil {
			logger.Log("Processing sub-hunk %d (lines %d-%d): oldStart=%d, newStart=%d",
				i+1, startIdx, endIdx-1, oldStart, newStart)
		}

		// Process this individual hunk
		hunkContent := p.formatSingleHunk(lines[startIdx+1:endIdx], oldStart, newStart, headerLine)
		result.WriteString(hunkContent)

		// Add separator between hunks except for the last one
		if i < len(hunkBoundaries)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// formatSingleHunk formats a single hunk with line numbers
// Returns content formatted as "OLD | NEW | CONTENT" table
func (p *LangchainProvider) formatSingleHunk(lines []string, oldStart, newStart int, header string) string {
	logger := logging.GetCurrentLogger()

	var result strings.Builder

	// Add header if provided
	if header != "" {
		result.WriteString(header + "\n")
	}

	// Add table header
	result.WriteString("OLD | NEW | CONTENT\n")
	result.WriteString("----|-----|--------\n")

	currentOldLine := oldStart
	currentNewLine := newStart

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		prefix := line[0:1]
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}

		var oldNum, newNum string

		switch prefix {
		case "+":
			// Added line - only appears in new version
			oldNum = "   "
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentNewLine++

		case "-":
			// Removed line - only appears in old version
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = "   "
			currentOldLine++

		case " ":
			// Context line - appears in both versions
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentOldLine++
			currentNewLine++

		default:
			// Handle special cases first
			if strings.HasPrefix(line, "@@") {
				// This is likely a stray hunk header that shouldn't be here
				if logger != nil {
					logger.Log("Encountered unexpected hunk header in content: %s", line)
				}
				// Skip processing this line
				continue
			}

			// Unknown prefix - treat as context but log it
			if logger != nil && prefix != "" {
				logger.Log("Unknown line prefix '%s' in hunk, treating as context: %s", prefix, line)
			}
			oldNum = fmt.Sprintf("%3d", currentOldLine)
			newNum = fmt.Sprintf("%3d", currentNewLine)
			currentOldLine++
			currentNewLine++
		}

		// Format the line with proper table structure
		result.WriteString(fmt.Sprintf("%s | %s | %s%s\n", oldNum, newNum, prefix, content))
	}

	if logger != nil {
		logger.Log("Formatted hunk: old lines %d-%d, new lines %d-%d",
			oldStart, currentOldLine-1, newStart, currentNewLine-1)
	}

	return result.String()
}

// Config for the langchain provider
type Config struct {
	APIKey       string `json:"api_key"`
	ModelName    string `json:"model_name"`
	MaxTokens    int    `json:"max_tokens"`
	ProviderType string `json:"provider_type"` // NEW: "gemini", "ollama", "openai", etc.
	BaseURL      string `json:"base_url"`      // NEW: For custom endpoints like Ollama
}

// New creates a new langchain-based AI provider
func New(config Config) *LangchainProvider {
	return &LangchainProvider{
		apiKey:       config.APIKey,
		modelName:    config.ModelName,
		maxTokens:    config.MaxTokens,
		providerType: config.ProviderType, // NEW
		baseURL:      config.BaseURL,      // NEW
	}
}

func (p *LangchainProvider) Name() string {
	if p.providerType != "" {
		return p.providerType
	}
	return "langchain"
}

func (p *LangchainProvider) MaxTokensPerBatch() int {
	if p.maxTokens <= 0 {
		// Provider-specific defaults if not configured
		switch strings.ToLower(p.providerType) {
		case "ollama":
			return 8000 // Conservative limit for Ollama models
		case "gemini", "googleai":
			return 30000 // Gemini can handle larger batches
		case "openai":
			return 16000 // OpenAI models like GPT-3.5/4
		case "anthropic":
			return 20000 // Claude models
		default:
			return 8000 // Conservative default for unknown providers
		}
	}
	return p.maxTokens
}

func (p *LangchainProvider) Configure(config map[string]interface{}) error {
	if apiKey, ok := config["api_key"].(string); ok {
		p.apiKey = apiKey
	}
	if modelName, ok := config["model_name"].(string); ok {
		p.modelName = modelName
	}
	if maxTokens, ok := config["max_tokens"].(float64); ok { // JSON numbers are float64
		p.maxTokens = int(maxTokens)
	}

	// Initialize the LLM
	return p.initializeLLM()
}

func (p *LangchainProvider) initializeLLM() error {
	switch strings.ToLower(p.providerType) {
	case "ollama":
		return p.initializeOllamaLLM()
	case "google", "googleai", "gemini":
		return p.initializeGeminiLLM()
	case "openai":
		return p.initializeOpenAILLM()
	case "anthropic":
		return p.initializeAnthropicLLM()
	default:
		logger := logging.GetCurrentLogger()
		logger.Log("WARNING: Unknown provider type '%s', falling back to Gemini", p.providerType)
		return p.initializeGeminiLLM()
	}
}

func (p *LangchainProvider) initializeOllamaLLM() error {
	options := []ollama.Option{
		ollama.WithModel(p.getModelName()),
	}

	if p.baseURL != "" {
		// Clean up base URL - remove trailing /api/ if present for Ollama
		cleanURL := strings.TrimSuffix(p.baseURL, "/api/")
		cleanURL = strings.TrimSuffix(cleanURL, "/api")
		cleanURL = strings.TrimSuffix(cleanURL, "/")

		fmt.Printf("[OLLAMA INIT] Original base URL: %s\n", p.baseURL)
		fmt.Printf("[OLLAMA INIT] Cleaned base URL: %s\n", cleanURL)

		options = append(options, ollama.WithServerURL(cleanURL))
	}

	// If API key is provided, it might be a JWT token for authentication
	// We need to add it as an Authorization header
	if p.apiKey != "" {
		fmt.Printf("[OLLAMA INIT] API key provided (length: %d), adding as Authorization header\n", len(p.apiKey))

		// Create a custom HTTP client with Authorization header
		client := &http.Client{}

		// Create a custom transport that adds the Authorization header
		transport := &http.Transport{}
		client.Transport = &authTransport{
			Transport: transport,
			token:     p.apiKey,
		}

		options = append(options, ollama.WithHTTPClient(client))
	}

	fmt.Printf("[LANGCHAIN INIT] Initializing Ollama LLM with model: %s\n", p.getModelName())

	llm, err := ollama.New(options...)
	if err != nil {
		return fmt.Errorf("failed to create Ollama LLM: %w", err)
	}

	p.llm = llm
	return nil
}

// authTransport is a custom HTTP transport that adds Authorization header
type authTransport struct {
	Transport http.RoundTripper
	token     string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add Authorization header
	req.Header.Set("Authorization", "Bearer "+t.token)

	// Debug logging
	fmt.Printf("[HTTP DEBUG] Request URL: %s\n", req.URL.String())
	fmt.Printf("[HTTP DEBUG] Request Method: %s\n", req.Method)
	fmt.Printf("[HTTP DEBUG] Authorization header set with token length: %d\n", len(t.token))

	// Make the request
	resp, err := t.Transport.RoundTrip(req)

	// Debug response
	if err != nil {
		fmt.Printf("[HTTP DEBUG] Request failed: %v\n", err)
	} else {
		fmt.Printf("[HTTP DEBUG] Response status: %s\n", resp.Status)
		fmt.Printf("[HTTP DEBUG] Response content-type: %s\n", resp.Header.Get("Content-Type"))
	}

	return resp, err
}

func (p *LangchainProvider) initializeGeminiLLM() error {
	if p.apiKey == "" {
		return fmt.Errorf("API key is required for Gemini")
	}

	// Configure options for the LLM
	opts := []googleai.Option{
		googleai.WithAPIKey(p.apiKey),
		googleai.WithDefaultModel(p.getModelName()),
	}

	// Set max tokens if configured
	maxTokens := p.maxTokens
	if maxTokens <= 0 {
		maxTokens = 8192 // Default max output tokens for Gemini
	}
	opts = append(opts, googleai.WithDefaultMaxTokens(maxTokens))

	fmt.Printf("[LANGCHAIN INIT] Initializing Gemini LLM with model: %s, max tokens: %d\n", p.getModelName(), maxTokens)

	llm, err := googleai.New(context.Background(), opts...)
	if err != nil {
		return fmt.Errorf("failed to initialize Gemini LLM: %w", err)
	}

	p.llm = llm
	return nil
}

func (p *LangchainProvider) initializeOpenAILLM() error {
	if p.apiKey == "" {
		return fmt.Errorf("API key is required for OpenAI")
	}

	options := []openai.Option{
		openai.WithToken(p.apiKey),
		openai.WithModel(p.getModelName()),
	}

	if p.baseURL != "" {
		options = append(options, openai.WithBaseURL(p.baseURL))
	}

	fmt.Printf("[LANGCHAIN INIT] Initializing OpenAI LLM with model: %s, base URL: %s\n", p.getModelName(), p.baseURL)

	llm, err := openai.New(options...)
	if err != nil {
		return fmt.Errorf("failed to create OpenAI LLM: %w", err)
	}

	p.llm = llm
	return nil
}

func (p *LangchainProvider) initializeAnthropicLLM() error {
	if p.apiKey == "" {
		return fmt.Errorf("API key is required for Anthropic")
	}

	options := []anthropic.Option{
		anthropic.WithToken(p.apiKey),
		anthropic.WithModel(p.getModelName()),
	}

	fmt.Printf("[LANGCHAIN INIT] Initializing Anthropic LLM with model: %s\n", p.getModelName())

	llm, err := anthropic.New(options...)
	if err != nil {
		return fmt.Errorf("failed to create Anthropic LLM: %w", err)
	}

	p.llm = llm
	return nil
}

func (p *LangchainProvider) getModelName() string {
	if p.modelName != "" {
		return p.modelName
	}
	return "gemini-1.5-flash" // Default model
}

// ReviewCode is the legacy method for backwards compatibility
func (p *LangchainProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	// Convert to the newer batch-based approach
	batchProcessor := batch.DefaultBatchProcessor()
	return p.ReviewCodeWithBatching(ctx, diffs, batchProcessor)
}

// ReviewCodeBatch processes a single batch of code diffs
func (p *LangchainProvider) ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
	if p.llm == nil {
		return nil, fmt.Errorf("LLM not initialized")
	}

	// Generate unique timestamp for this batch
	timestamp := time.Now().Format("20060102_150405_000")
	batchId := fmt.Sprintf("batch_%s", timestamp)

	// Get global logger
	logger := logging.GetCurrentLogger()

	// CRITICAL: Add line numbers to hunks BEFORE creating prompt
	// This is essential for proper comment positioning
	if logger != nil {
		logger.LogSection(fmt.Sprintf("LINE NUMBERING - Batch %s", batchId))
		logger.Log("Adding line numbers to %d diffs before prompt creation", len(diffs))
	}

	// Process each diff to add line numbers (FIRST STEP - before batching/splitting)
	for i, diff := range diffs {
		if logger != nil {
			logger.Log("Processing diff %d: %s (%d hunks)", i+1, diff.FilePath, len(diff.Hunks))
		}

		var originalHunks []string
		var formattedHunks []string

		for j, hunk := range diff.Hunks {
			// Save original content for logging
			originalHunks = append(originalHunks, hunk.Content)

			// Format the hunk with line numbers (RESTORE CRITICAL FUNCTIONALITY)
			formattedContent := p.formatHunkWithLineNumbers(hunk)
			formattedHunks = append(formattedHunks, formattedContent)

			// Update the hunk content with the formatted version
			diff.Hunks[j].Content = formattedContent

			if logger != nil {
				logger.Log("  Hunk %d: Added line numbers (old: %d-%d, new: %d-%d)",
					j+1, hunk.OldStartLine, hunk.OldStartLine+hunk.OldLineCount-1,
					hunk.NewStartLine, hunk.NewStartLine+hunk.NewLineCount-1)
			}
		}

		// Log the transformation for debugging
		if logger != nil && len(originalHunks) > 0 {
			logger.Log("Line numbering transformation for %s:", diff.FilePath)
			logger.Log("--- ORIGINAL HUNK ---")
			logger.LogDiff(diff.FilePath, originalHunks[0][:minInt(200, len(originalHunks[0]))]+"...")
			logger.Log("--- FORMATTED HUNK ---")
			logger.LogDiff(diff.FilePath, formattedHunks[0][:minInt(200, len(formattedHunks[0]))]+"...")
		}
	}

	// Now process the batch with already-formatted diffs
	return p.reviewCodeBatchFormatted(ctx, diffs, batchId)
}

// reviewCodeBatchFormatted processes diffs that already have line numbers formatted
// This is used for recursive batch processing to avoid double line numbering
func (p *LangchainProvider) reviewCodeBatchFormatted(ctx context.Context, diffs []models.CodeDiff, batchId string) (*batch.BatchResult, error) {
	logger := logging.GetCurrentLogger()

	// Generate the review prompt (diffs already have line numbers)
	// Use centralized prompt building
	promptBuilder := prompts.NewPromptBuilder()

	// Convert []models.CodeDiff to []*models.CodeDiff for the prompt builder
	diffPointers := make([]*models.CodeDiff, len(diffs))
	for i := range diffs {
		diffPointers[i] = &diffs[i]
	}

	prompt := promptBuilder.BuildCodeReviewPrompt(diffPointers)

	// Log request to global logger
	if logger != nil {
		logger.LogRequest(batchId, p.modelName, prompt)
		logger.Log("Processing batch %s with %d diffs", batchId, len(diffs))
	}

	// Call the LLM with streaming
	fmt.Printf("[LANGCHAIN REQUEST] Calling LLM for batch %s with streaming...\n", batchId)
	fmt.Printf("[LANGCHAIN DEBUG] Provider type: %s, Model: %s, Base URL: %s\n",
		p.providerType, p.modelName, p.baseURL)

	// Create a timeout context (5 minutes for Ollama)
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Variables to collect the streaming response
	var responseBuilder strings.Builder
	var lastActivity time.Time = time.Now()
	var totalChunks int = 0

	// Create streaming function that prints chunks as they arrive
	streamingFunc := func(ctx context.Context, chunk []byte) error {
		chunkStr := string(chunk)
		totalChunks++
		lastActivity = time.Now()

		// Print the chunk in real-time
		fmt.Printf("[STREAM] %s", chunkStr)

		// Also log to the review logger
		if logger != nil && len(chunkStr) > 0 {
			// Only log non-empty chunks to avoid spam
			if strings.TrimSpace(chunkStr) != "" {
				logger.Log("Streaming chunk %d: %q", totalChunks, chunkStr)
			}
		}

		// Add to response builder
		responseBuilder.WriteString(chunkStr)

		return nil
	}

	// Start activity monitor in a separate goroutine
	activityDone := make(chan bool)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-activityDone:
				return
			case <-ticker.C:
				timeSinceActivity := time.Since(lastActivity)
				if timeSinceActivity > 30*time.Second {
					fmt.Printf("\n[STREAM MONITOR] No activity for %v (chunks received: %d)\n", timeSinceActivity, totalChunks)
				}
			}
		}
	}()

	// Call the LLM with streaming
	startTime := time.Now()
	fmt.Printf("[STREAM START] Beginning streaming response...\n")

	_, err := llms.GenerateFromSinglePrompt(
		timeoutCtx,
		p.llm,
		prompt,
		llms.WithStreamingFunc(streamingFunc),
	)

	// Stop activity monitor
	close(activityDone)

	// Get the complete response
	response := responseBuilder.String()

	if err != nil {
		if logger != nil {
			logger.LogError(fmt.Sprintf("LLM call batch %s", batchId), err)
		}
		fmt.Printf("\n[LANGCHAIN ERROR] LLM call failed for batch %s: %v\n", batchId, err)
		fmt.Printf("[LANGCHAIN ERROR] Provider: %s, Model: %s, Base URL: %s\n",
			p.providerType, p.modelName, p.baseURL)
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	fmt.Printf("\n[STREAM COMPLETE] Full response received after %v (%d chunks, %d chars)\n",
		time.Since(startTime), totalChunks, len(response)) // Log response to global logger
	if logger != nil {
		logger.LogResponse(batchId, response)
	}

	// Parse the response
	fmt.Printf("[LANGCHAIN PARSE] Starting to parse response for batch %s...\n", batchId)
	result, err := p.parseResponse(response, diffs)
	if err != nil {
		if logger != nil {
			logger.LogError(fmt.Sprintf("JSON parsing batch %s", batchId), err)
		}
		fmt.Printf("[LANGCHAIN PARSE ERROR] Failed to parse response for batch %s: %v\n", batchId, err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Log parsed comments to global logger
	if logger != nil {
		logger.LogComments(batchId, result.Comments)
		logger.Log("Batch %s completed successfully with %d comments", batchId, len(result.Comments))
	}

	fmt.Printf("[LANGCHAIN SUCCESS] Batch %s completed with %d comments\n", batchId, len(result.Comments))

	// Convert to BatchResult
	return &batch.BatchResult{
		Summary:     result.FileSummary, // Use FileSummary for batch-level summary
		FileSummary: result.FileSummary,
		Comments:    result.Comments,
		Error:       nil,
		BatchID:     batchId,
	}, nil
}

// ReviewCodeWithBatching processes code diffs using the batch processor
func (p *LangchainProvider) ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error) {
	if p.llm == nil {
		if err := p.initializeLLM(); err != nil {
			return nil, fmt.Errorf("failed to initialize LLM: %w", err)
		}
	}

	// Convert []*models.CodeDiff to []models.CodeDiff
	input := batchProcessor.PrepareFullInput(diffs)

	// CRITICAL: Add line numbers to ALL diffs BEFORE batching
	// This ensures line numbering happens only once, not per batch
	logger := logging.GetCurrentLogger()
	if logger != nil {
		logger.LogSection("LINE NUMBERING - Pre-Batch Processing")
		logger.Log("Adding line numbers to %d diffs before batching", len(input))
	}

	// Process each diff to add line numbers (BEFORE batching/splitting)
	for i, diff := range input {
		if logger != nil {
			logger.Log("Processing diff %d: %s (%d hunks)", i+1, diff.FilePath, len(diff.Hunks))
		}

		for j, hunk := range diff.Hunks {
			// Format the hunk with line numbers
			formattedContent := p.formatHunkWithLineNumbers(hunk)
			// Update the hunk content with the formatted version
			diff.Hunks[j].Content = formattedContent

			if logger != nil {
				logger.Log("  Hunk %d: Added line numbers (old: %d-%d, new: %d-%d)",
					j+1, hunk.OldStartLine, hunk.OldStartLine+hunk.OldLineCount-1,
					hunk.NewStartLine, hunk.NewStartLine+hunk.NewLineCount-1)
			}
		}
	}

	// Create batch input (AFTER line numbering)
	batchInput := batchProcessor.BatchInputs(input)

	// Process batches using task queue (following Gemini provider pattern)
	taskQueue := batch.NewTaskQueue(4) // Use 4 workers by default

	// Use the batch processor's configuration for the task queue if available
	if batchProcessor.TaskQueueConfig.MaxWorkers > 0 {
		taskQueue = batch.ConfigureTaskQueue(batchProcessor.TaskQueueConfig)
	}

	// Create tasks for each batch
	for i, batchDiffs := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)

		// Create a processor function for this batch
		// Use the formatted method to avoid double line numbering
		processor := func(ctx context.Context, batchDiffs []models.CodeDiff) (*batch.BatchResult, error) {
			return p.reviewCodeBatchFormatted(ctx, batchDiffs, batchID)
		}

		// Create and add the task
		task := batch.NewBatchTask(batchID, batchDiffs, processor)
		task.SetBatchNumber(i + 1)
		task.SetLogger(batchProcessor.Logger)
		taskQueue.AddTask(task)
	}

	// Execute all tasks
	taskResults := taskQueue.ProcessAll(ctx)

	// Collect results
	batchResults := make([]*batch.BatchResult, len(batchInput.Batches))
	for i := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)
		taskResult, ok := taskResults[batchID]

		if !ok || taskResult.Error != nil {
			if !ok {
				return nil, fmt.Errorf("batch %s not found in results", batchID)
			}
			return nil, fmt.Errorf("error processing batch %s: %v", batchID, taskResult.Error)
		}

		batchResult, ok := taskResult.Result.(*batch.BatchResult)
		if !ok {
			return nil, fmt.Errorf("invalid result type for batch %s", batchID)
		}

		batchResults[i] = batchResult
	}

	// Aggregate results using the batch processor's aggregation logic
	return batchProcessor.AggregateAndCombineOutputs(ctx, p.llm, batchResults)
}

// parseResponse parses the LLM response into a structured result
func (p *LangchainProvider) parseResponse(response string, diffs []models.CodeDiff) (*ParsedResult, error) {
	// Define JSON structures for response
	type Comment struct {
		FilePath    string   `json:"filePath"`
		LineNumber  int      `json:"lineNumber"`
		Content     string   `json:"content"`
		Severity    string   `json:"severity"`
		Suggestions []string `json:"suggestions"`
		IsInternal  bool     `json:"isInternal"`
	}

	type Response struct {
		FileSummary string    `json:"fileSummary"`
		Comments    []Comment `json:"comments"`
	}

	// Try to extract JSON from the response
	jsonStr := p.extractJSONFromResponse(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	// Check if the JSON appears to be truncated
	if !strings.HasSuffix(strings.TrimSpace(jsonStr), "}") {
		// Try to repair truncated JSON by attempting to close incomplete structures
		repairedJSON := p.attemptJSONRepair(jsonStr)
		if repairedJSON != jsonStr {
			fmt.Printf("[LANGCHAIN REPAIR] Detected truncated JSON, attempting repair\n")
			fmt.Printf("[LANGCHAIN REPAIR] Original length: %d, Repaired length: %d\n", len(jsonStr), len(repairedJSON))
			jsonStr = repairedJSON
		} else {
			return nil, fmt.Errorf("JSON response appears to be truncated (doesn't end with '}') - possible token limit exceeded. Response length: %d chars", len(response))
		}
	}

	var resp Response
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		// Try to provide more helpful error messages
		if strings.Contains(err.Error(), "unexpected end of JSON input") {
			return nil, fmt.Errorf("JSON response is incomplete/truncated - likely hit token limit. Original response length: %d chars, JSON length: %d chars. Error: %w", len(response), len(jsonStr), err)
		}
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Convert to our models
	var comments []*models.ReviewComment
	for _, comment := range resp.Comments {
		// TODO: Fix line validation logic - currently too restrictive
		// For now, trust the LLM's line numbers since we provide formatted line numbers
		validLine := p.isLineInDiff(comment.FilePath, comment.LineNumber, diffs)
		if !validLine {
			fmt.Printf("[LANGCHAIN WARNING] Line validation failed for line %d in %s - but proceeding anyway\n",
				comment.LineNumber, comment.FilePath)
			// Continue instead of skipping - trust the line numbering we provided to LLM
		}

		// Convert severity
		severity := p.convertSeverity(comment.Severity)

		// Determine if this is a deleted line based on the diff content
		isDeletedLine := p.isDeletedLine(comment.FilePath, comment.LineNumber, diffs)

		reviewComment := &models.ReviewComment{
			FilePath:      comment.FilePath,
			Line:          comment.LineNumber,
			Content:       comment.Content,
			Severity:      severity,
			Suggestions:   comment.Suggestions,
			Category:      "review",
			IsInternal:    comment.IsInternal,
			IsDeletedLine: isDeletedLine,
		}

		comments = append(comments, reviewComment)
	}

	return &ParsedResult{
		FileSummary: resp.FileSummary,
		Comments:    comments,
	}, nil
}

type ParsedResult struct {
	FileSummary string
	Comments    []*models.ReviewComment
}

// extractJSONFromResponse extracts JSON from the LLM response
func (p *LangchainProvider) extractJSONFromResponse(response string) string {
	// Try to extract JSON from markdown code blocks
	start := strings.Index(response, "```json")
	if start == -1 {
		start = strings.Index(response, "```")
	}
	if start == -1 {
		// Check if the whole response is JSON
		trimmed := strings.TrimSpace(response)
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			return trimmed
		}
		return ""
	}

	start = strings.Index(response[start:], "{")
	if start == -1 {
		return ""
	}
	start += strings.Index(response, "```")

	end := strings.LastIndex(response, "}")
	if end == -1 || end <= start {
		return ""
	}

	return response[start : end+1]
}

// convertSeverity converts string severity to models.CommentSeverity
func (p *LangchainProvider) convertSeverity(severity string) models.CommentSeverity {
	switch strings.ToLower(severity) {
	case "critical":
		return models.SeverityCritical
	case "warning":
		return models.SeverityWarning
	case "info":
		return models.SeverityInfo
	default:
		return models.SeverityInfo
	}
}

// isDeletedLine determines if a line comment is on a deleted line
func (p *LangchainProvider) isDeletedLine(filePath string, lineNumber int, diffs []models.CodeDiff) bool {
	for _, diff := range diffs {
		if diff.FilePath == filePath {
			for _, hunk := range diff.Hunks {
				if p.lineInHunk(lineNumber, hunk) {
					return p.lineIsDeleted(lineNumber, hunk)
				}
			}
		}
	}
	return false // Default to false if we can't determine
}

// isLineInDiff checks if a line number is actually part of any diff hunk
func (p *LangchainProvider) isLineInDiff(filePath string, lineNumber int, diffs []models.CodeDiff) bool {
	for _, diff := range diffs {
		if diff.FilePath == filePath {
			for _, hunk := range diff.Hunks {
				if p.lineInHunk(lineNumber, hunk) {
					return true
				}
			}
		}
	}
	return false
}

// lineInHunk checks if a line number is within the range of the given hunk
func (p *LangchainProvider) lineInHunk(lineNumber int, hunk models.DiffHunk) bool {
	return lineNumber >= hunk.OldStartLine && lineNumber <= hunk.OldStartLine+hunk.OldLineCount ||
		lineNumber >= hunk.NewStartLine && lineNumber <= hunk.NewStartLine+hunk.NewLineCount
}

// lineIsDeleted analyzes hunk content to determine if a line is deleted
func (p *LangchainProvider) lineIsDeleted(lineNumber int, hunk models.DiffHunk) bool {
	lines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			continue // Skip hunk header
		}

		if strings.HasPrefix(line, "-") {
			if oldLine == lineNumber {
				return true
			}
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			newLine++
		} else {
			// Context line
			if oldLine == lineNumber || newLine == lineNumber {
				return false // Context lines are not deleted
			}
			oldLine++
			newLine++
		}
	}

	return false
}

// Helper functions for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func getLastChars(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// writeLogFile writes content to a log file, creating directories if needed
func (p *LangchainProvider) writeLogFile(filename, content string) error {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write log file: %w", err)
	}

	return nil
}

// attemptJSONRepair tries to repair truncated JSON by closing incomplete structures
func (p *LangchainProvider) attemptJSONRepair(jsonStr string) string {
	// Remove any trailing incomplete content after the last complete comment
	lines := strings.Split(jsonStr, "\n")
	var repairedLines []string

	bracketCount := 0
	inString := false
	escapeNext := false
	lastCompleteIndex := -1

	for i, line := range lines {
		// Track brackets and strings to find the last complete structure
		for _, char := range line {
			if escapeNext {
				escapeNext = false
				continue
			}

			if char == '\\' {
				escapeNext = true
				continue
			}

			if char == '"' && !escapeNext {
				inString = !inString
				continue
			}

			if !inString {
				if char == '{' || char == '[' {
					bracketCount++
				} else if char == '}' || char == ']' {
					bracketCount--
				}
			}
		}

		repairedLines = append(repairedLines, line)

		// If we have balanced brackets and this line ends with } or ], this might be a good stopping point
		if bracketCount == 0 && (strings.HasSuffix(strings.TrimSpace(line), "}") || strings.HasSuffix(strings.TrimSpace(line), "]")) {
			lastCompleteIndex = i
		}
	}

	// If we found a good stopping point and there's more content after it, truncate there
	if lastCompleteIndex >= 0 && lastCompleteIndex < len(repairedLines)-1 {
		// Check if the content after the stopping point looks incomplete
		remainingContent := strings.Join(repairedLines[lastCompleteIndex+1:], "\n")
		if strings.TrimSpace(remainingContent) != "" {
			fmt.Printf("[LANGCHAIN REPAIR] Truncating at line %d, removing: %s\n", lastCompleteIndex+1, strings.TrimSpace(remainingContent))
			repairedLines = repairedLines[:lastCompleteIndex+1]
		}
	}

	return strings.Join(repairedLines, "\n")
}
