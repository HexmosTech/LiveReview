package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
)

// diffReviewRequest models the POST payload to /api/v1/diff-review
type diffReviewRequest struct {
	DiffZipBase64 string `json:"diff_zip_base64"`
	RepoName      string `json:"repo_name"`
}

// diffReviewResponse models the response from GET /api/v1/diff-review/:id
type diffReviewResponse struct {
	Status  string                 `json:"status"`
	Summary string                 `json:"summary,omitempty"`
	Files   []diffReviewFileResult `json:"files,omitempty"`
}

type diffReviewFileResult struct {
	FilePath string              `json:"file_path"`
	Hunks    []diffReviewHunk    `json:"hunks"`
	Comments []diffReviewComment `json:"comments"`
}

type diffReviewHunk struct {
	OldStartLine int    `json:"old_start_line"`
	OldLineCount int    `json:"old_line_count"`
	NewStartLine int    `json:"new_start_line"`
	NewLineCount int    `json:"new_line_count"`
	Content      string `json:"content"`
}

type diffReviewComment struct {
	Line     int    `json:"line"`
	Content  string `json:"content"`
	Severity string `json:"severity"`
	Category string `json:"category"`
}

func main() {
	app := &cli.App{
		Name:  "lrc",
		Usage: "LiveReview CLI - submit local diffs for AI review",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo-name",
				Usage:   "repository name (defaults to current directory basename)",
				EnvVars: []string{"LRC_REPO_NAME"},
			},
			&cli.StringFlag{
				Name:    "diff-source",
				Value:   "working",
				Usage:   "diff source: working, staged, range, or file",
				EnvVars: []string{"LRC_DIFF_SOURCE"},
			},
			&cli.StringFlag{
				Name:    "range",
				Usage:   "git range for diff-source=range (e.g., HEAD~1..HEAD)",
				EnvVars: []string{"LRC_RANGE"},
			},
			&cli.StringFlag{
				Name:    "diff-file",
				Usage:   "path to pre-generated diff file (for diff-source=file)",
				EnvVars: []string{"LRC_DIFF_FILE"},
			},
			&cli.StringFlag{
				Name:    "api-url",
				Value:   "http://localhost:8888",
				Usage:   "LiveReview API base URL",
				EnvVars: []string{"LRC_API_URL"},
			},
			&cli.StringFlag{
				Name:    "api-key",
				Usage:   "API key for authentication (can be set in ~/.lrc.toml or env var)",
				EnvVars: []string{"LRC_API_KEY"},
			},
			&cli.DurationFlag{
				Name:    "poll-interval",
				Value:   2 * time.Second,
				Usage:   "interval between status polls",
				EnvVars: []string{"LRC_POLL_INTERVAL"},
			},
			&cli.DurationFlag{
				Name:    "timeout",
				Value:   5 * time.Minute,
				Usage:   "maximum time to wait for review completion",
				EnvVars: []string{"LRC_TIMEOUT"},
			},
			&cli.StringFlag{
				Name:    "output",
				Value:   "pretty",
				Usage:   "output format: pretty or json",
				EnvVars: []string{"LRC_OUTPUT"},
			},
			&cli.StringFlag{
				Name:    "save-bundle",
				Usage:   "save the base64-encoded bundle to this file for inspection before sending",
				EnvVars: []string{"LRC_SAVE_BUNDLE"},
			},
			&cli.StringFlag{
				Name:    "save-json",
				Usage:   "save the JSON response to this file after completion",
				EnvVars: []string{"LRC_SAVE_JSON"},
			},
			&cli.StringFlag{
				Name:    "save-text",
				Usage:   "save formatted text output with comment markers to this file",
				EnvVars: []string{"LRC_SAVE_TEXT"},
			},
			&cli.StringFlag{
				Name:    "save-html",
				Usage:   "save formatted HTML output (GitHub-style review) to this file",
				EnvVars: []string{"LRC_SAVE_HTML"},
			},
			&cli.BoolFlag{
				Name:    "serve",
				Usage:   "start HTTP server to serve the HTML output (requires --save-html)",
				EnvVars: []string{"LRC_SERVE"},
			},
			&cli.IntFlag{
				Name:    "port",
				Usage:   "port for HTTP server (used with --serve)",
				Value:   8000,
				EnvVars: []string{"LRC_PORT"},
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "enable verbose output",
				EnvVars: []string{"LRC_VERBOSE"},
			},
		},
		Action: runReview,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runReview(c *cli.Context) error {
	verbose := c.Bool("verbose")

	// Load configuration from config file or CLI/env
	config, err := loadConfig(c, verbose)
	if err != nil {
		return err
	}

	// Determine repo name
	repoName := c.String("repo-name")
	if repoName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		repoName = filepath.Base(cwd)
	}

	if verbose {
		log.Printf("Repository name: %s", repoName)
		log.Printf("API URL: %s", config.APIURL)
	}

	// Collect diff
	diffContent, err := collectDiff(c, verbose)
	if err != nil {
		return fmt.Errorf("failed to collect diff: %w", err)
	}

	if len(diffContent) == 0 {
		return fmt.Errorf("no diff content collected")
	}

	if verbose {
		log.Printf("Collected %d bytes of diff content", len(diffContent))
	}

	// Create ZIP archive
	zipData, err := createZipArchive(diffContent)
	if err != nil {
		return fmt.Errorf("failed to create zip archive: %w", err)
	}

	if verbose {
		log.Printf("Created ZIP archive: %d bytes", len(zipData))
	}

	// Base64 encode
	base64Diff := base64.StdEncoding.EncodeToString(zipData)

	// Save bundle if requested
	if bundlePath := c.String("save-bundle"); bundlePath != "" {
		if err := saveBundleForInspection(bundlePath, diffContent, zipData, base64Diff, verbose); err != nil {
			return fmt.Errorf("failed to save bundle: %w", err)
		}
	}

	// Submit review
	reviewID, err := submitReview(config.APIURL, config.APIKey, base64Diff, repoName, verbose)
	if err != nil {
		return fmt.Errorf("failed to submit review: %w", err)
	}

	if verbose {
		log.Printf("Review submitted, ID: %s", reviewID)
	}

	// Poll for completion
	pollInterval := c.Duration("poll-interval")
	timeout := c.Duration("timeout")
	result, err := pollReview(config.APIURL, config.APIKey, reviewID, pollInterval, timeout, verbose)
	if err != nil {
		return fmt.Errorf("failed to poll review: %w", err)
	}

	// Save JSON response if requested
	if jsonPath := c.String("save-json"); jsonPath != "" {
		if err := saveJSONResponse(jsonPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save JSON response: %w", err)
		}
	}

	// Save formatted text output if requested
	if textPath := c.String("save-text"); textPath != "" {
		if err := saveTextOutput(textPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save text output: %w", err)
		}
	}

	// Save HTML output if requested
	if htmlPath := c.String("save-html"); htmlPath != "" {
		if err := saveHTMLOutput(htmlPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save HTML output: %w", err)
		}

		// Start HTTP server if --serve flag is set
		if c.Bool("serve") {
			port := c.Int("port")
			if err := serveHTML(htmlPath, port); err != nil {
				return fmt.Errorf("failed to serve HTML: %w", err)
			}
		}
	}

	// Render result to stdout
	outputFormat := c.String("output")
	if err := renderResult(result, outputFormat); err != nil {
		return fmt.Errorf("failed to render result: %w", err)
	}

	return nil
}

func collectDiff(c *cli.Context, verbose bool) ([]byte, error) {
	diffSource := c.String("diff-source")

	switch diffSource {
	case "staged":
		if verbose {
			log.Println("Collecting staged changes...")
		}
		return runGitCommand("git", "diff", "--staged")

	case "working":
		if verbose {
			log.Println("Collecting working tree changes...")
		}
		return runGitCommand("git", "diff")

	case "range":
		rangeVal := c.String("range")
		if rangeVal == "" {
			return nil, fmt.Errorf("--range is required when diff-source=range")
		}
		if verbose {
			log.Printf("Collecting diff for range: %s", rangeVal)
		}
		return runGitCommand("git", "diff", rangeVal)

	case "file":
		filePath := c.String("diff-file")
		if filePath == "" {
			return nil, fmt.Errorf("--diff-file is required when diff-source=file")
		}
		if verbose {
			log.Printf("Reading diff from file: %s", filePath)
		}
		return os.ReadFile(filePath)

	default:
		return nil, fmt.Errorf("invalid diff-source: %s (must be staged, working, range, or file)", diffSource)
	}
}

func runGitCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git command failed: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	return output, nil
}

func createZipArchive(diffContent []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	fileWriter, err := zipWriter.Create("diff.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create zip entry: %w", err)
	}

	if _, err := fileWriter.Write(diffContent); err != nil {
		return nil, fmt.Errorf("failed to write to zip: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func submitReview(apiURL, apiKey, base64Diff, repoName string, verbose bool) (string, error) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review"

	payload := diffReviewRequest{
		DiffZipBase64: base64Diff,
		RepoName:      repoName,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)

	if verbose {
		log.Printf("POST %s", endpoint)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	reviewID, ok := result["review_id"].(string)
	if !ok {
		return "", fmt.Errorf("review_id not found in response")
	}

	return reviewID, nil
}

func pollReview(apiURL, apiKey, reviewID string, pollInterval, timeout time.Duration, verbose bool) (*diffReviewResponse, error) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/" + reviewID
	deadline := time.Now().Add(timeout)

	if verbose {
		log.Printf("Polling for review completion (timeout: %v)...", timeout)
	}

	for time.Now().Before(deadline) {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		var result diffReviewResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		if verbose {
			log.Printf("Status: %s", result.Status)
		}

		if result.Status == "completed" {
			return &result, nil
		}

		if result.Status == "failed" {
			return nil, fmt.Errorf("review failed")
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for review completion")
}

func renderResult(result *diffReviewResponse, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)

	case "pretty":
		return renderPretty(result)

	default:
		return fmt.Errorf("invalid output format: %s (must be json or pretty)", format)
	}
}

func renderPretty(result *diffReviewResponse) error {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("LIVEREVIEW RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	if result.Summary != "" {
		fmt.Println("\nSummary:")
		fmt.Println(result.Summary)
	}

	if len(result.Files) == 0 {
		fmt.Println("\nNo files reviewed or no comments generated.")
		return nil
	}

	fmt.Printf("\n%d file(s) with comments:\n", len(result.Files))

	for _, file := range result.Files {
		fmt.Println("\n" + strings.Repeat("-", 80))
		fmt.Printf("FILE: %s\n", file.FilePath)
		fmt.Println(strings.Repeat("-", 80))

		if len(file.Comments) == 0 {
			fmt.Println("  No comments for this file.")
			continue
		}

		for _, comment := range file.Comments {
			severity := strings.ToUpper(comment.Severity)
			if severity == "" {
				severity = "INFO"
			}

			fmt.Printf("\n  [%s] Line %d", severity, comment.Line)
			if comment.Category != "" {
				fmt.Printf(" (%s)", comment.Category)
			}
			fmt.Println()

			// Indent comment content
			lines := strings.Split(comment.Content, "\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Review complete: %d total comment(s)\n", countTotalComments(result.Files))
	fmt.Println(strings.Repeat("=", 80) + "\n")

	return nil
}

func countTotalComments(files []diffReviewFileResult) int {
	total := 0
	for _, file := range files {
		total += len(file.Comments)
	}
	return total
}

// Config holds the CLI configuration
type Config struct {
	APIKey string
	APIURL string
}

// loadConfig attempts to load configuration from ~/.lrc.toml, then falls back to CLI flags or env vars
func loadConfig(c *cli.Context, verbose bool) (*Config, error) {
	config := &Config{}

	// Try to load from config file first
	homeDir, err := os.UserHomeDir()
	var k *koanf.Koanf
	if err == nil {
		configPath := filepath.Join(homeDir, ".lrc.toml")
		if _, err := os.Stat(configPath); err == nil {
			// Config file exists, try to load it
			k = koanf.New(".")
			if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
				return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
			}
			if verbose {
				log.Printf("Loaded config from: %s", configPath)
			}
		}
	}

	// Load API key: CLI/env takes precedence over config file
	if apiKey := c.String("api-key"); apiKey != "" {
		config.APIKey = apiKey
		if verbose {
			log.Println("Using API key from CLI flag or environment variable")
		}
	} else if k != nil && k.String("api_key") != "" {
		config.APIKey = k.String("api_key")
		if verbose {
			log.Println("Using API key from config file")
		}
	} else {
		return nil, fmt.Errorf("API key not provided. Set via --api-key flag, LRC_API_KEY environment variable, or api_key in ~/.lrc.toml")
	}

	// Load API URL: CLI/env takes precedence over config file
	if apiURL := c.String("api-url"); apiURL != "http://localhost:8888" {
		// User explicitly set it via CLI or env
		config.APIURL = apiURL
		if verbose {
			log.Println("Using API URL from CLI flag or environment variable")
		}
	} else if k != nil && k.String("api_url") != "" {
		config.APIURL = k.String("api_url")
		if verbose {
			log.Println("Using API URL from config file")
		}
	} else {
		// Use default
		config.APIURL = c.String("api-url")
		if verbose {
			log.Printf("Using default API URL: %s", config.APIURL)
		}
	}

	return config, nil
}

// saveBundleForInspection saves the bundle in multiple formats for inspection
func saveBundleForInspection(path string, diffContent, zipData []byte, base64Diff string, verbose bool) error {
	// Create a comprehensive bundle file with sections
	var buf bytes.Buffer

	buf.WriteString("# LiveReview Bundle Inspection File\n")
	buf.WriteString("# Generated: " + time.Now().Format(time.RFC3339) + "\n\n")

	buf.WriteString("## SECTION 1: Original Diff Content\n")
	buf.WriteString("## This is the raw diff that was collected\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n\n")
	buf.Write(diffContent)
	buf.WriteString("\n\n")

	buf.WriteString("## SECTION 2: Zip Archive Info\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n")
	buf.WriteString(fmt.Sprintf("## Zip size: %d bytes\n", len(zipData)))
	buf.WriteString("## Contains: diff.txt\n\n")

	buf.WriteString("## SECTION 3: Base64 Encoded Bundle (sent to API)\n")
	buf.WriteString("## This is what gets transmitted in the API request\n")
	buf.WriteString("## " + strings.Repeat("-", 76) + "\n\n")
	buf.WriteString(base64Diff)
	buf.WriteString("\n")

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Bundle saved to: %s (%d bytes)", path, buf.Len())
	}

	return nil
}

// saveJSONResponse saves the raw JSON response to a file
func saveJSONResponse(path string, result *diffReviewResponse, verbose bool) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("JSON response saved to: %s (%d bytes)", path, len(data))
	}

	return nil
}

// saveTextOutput saves formatted text output with special markers for easy comment navigation
func saveTextOutput(path string, result *diffReviewResponse, verbose bool) error {
	var buf bytes.Buffer

	// Use a distinctive marker that's easy to search for
	const commentMarker = ">>>COMMENT<<<"

	buf.WriteString("=" + strings.Repeat("=", 79) + "\n")
	buf.WriteString("LIVEREVIEW RESULTS - TEXT FORMAT\n")
	buf.WriteString("=" + strings.Repeat("=", 79) + "\n")
	buf.WriteString(fmt.Sprintf("Generated: %s\n", time.Now().Format(time.RFC3339)))
	buf.WriteString("\nSearch for '" + commentMarker + "' to jump between review comments\n")
	buf.WriteString("=" + strings.Repeat("=", 79) + "\n\n")

	if result.Summary != "" {
		buf.WriteString("SUMMARY:\n")
		buf.WriteString(result.Summary)
		buf.WriteString("\n\n")
	}

	totalComments := countTotalComments(result.Files)
	buf.WriteString(fmt.Sprintf("TOTAL FILES: %d\n", len(result.Files)))
	buf.WriteString(fmt.Sprintf("TOTAL COMMENTS: %d\n\n", totalComments))

	if len(result.Files) == 0 {
		buf.WriteString("No files reviewed or no comments generated.\n")
	} else {
		for fileIdx, file := range result.Files {
			buf.WriteString("\n" + strings.Repeat("=", 80) + "\n")
			buf.WriteString(fmt.Sprintf("FILE %d/%d: %s\n", fileIdx+1, len(result.Files), file.FilePath))
			buf.WriteString(strings.Repeat("=", 80) + "\n")

			if len(file.Comments) == 0 {
				buf.WriteString("\n  No comments for this file.\n")
				continue
			}

			buf.WriteString(fmt.Sprintf("\n  %d comment(s) on this file\n\n", len(file.Comments)))

			// Create a map of line numbers to comments for easy lookup
			commentsByLine := make(map[int][]diffReviewComment)
			for _, comment := range file.Comments {
				commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
			}

			// Process each hunk and insert comments inline
			for hunkIdx, hunk := range file.Hunks {
				if hunkIdx > 0 {
					buf.WriteString("\n")
				}

				// Parse and render the hunk with line numbers
				renderHunkWithComments(&buf, hunk, commentsByLine, commentMarker)
			}
		}
	}

	buf.WriteString("\n" + strings.Repeat("=", 80) + "\n")
	buf.WriteString(fmt.Sprintf("END OF REVIEW - %d total comment(s)\n", totalComments))
	buf.WriteString(strings.Repeat("=", 80) + "\n")

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("Text output saved to: %s (%d bytes)", path, buf.Len())
		log.Printf("Search for '%s' in the file to navigate between comments", commentMarker)
	}

	return nil
}

// renderHunkWithComments renders a diff hunk with line numbers and inline comments
func renderHunkWithComments(buf *bytes.Buffer, hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment, marker string) {
	// Write hunk header
	buf.WriteString(strings.Repeat("-", 80) + "\n")
	buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
		hunk.OldStartLine, hunk.OldLineCount,
		hunk.NewStartLine, hunk.NewLineCount))
	buf.WriteString(strings.Repeat("-", 80) + "\n")

	// Parse the hunk content line by line
	lines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// Skip the hunk header line if it's in the content
		if strings.HasPrefix(line, "@@") {
			continue
		}

		var oldNum, newNum string
		var diffLine string

		if strings.HasPrefix(line, "-") {
			// Deleted line - only old line number
			oldNum = fmt.Sprintf("%4d", oldLine)
			newNum = "    "
			diffLine = line
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			// Added line - only new line number
			oldNum = "    "
			newNum = fmt.Sprintf("%4d", newLine)
			diffLine = line

			// Check for comments on this new line
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				// First write the diff line
				buf.WriteString(fmt.Sprintf("%s | %s | %s\n", oldNum, newNum, diffLine))

				// Then write all comments for this line
				for _, comment := range comments {
					buf.WriteString(fmt.Sprintf("\n%s ", marker))
					severity := strings.ToUpper(comment.Severity)
					if severity == "" {
						severity = "INFO"
					}
					buf.WriteString(fmt.Sprintf("[%s] Line %d", severity, comment.Line))
					if comment.Category != "" {
						buf.WriteString(fmt.Sprintf(" (%s)", comment.Category))
					}
					buf.WriteString("\n" + strings.Repeat("-", 80) + "\n")

					// Write comment content with indentation
					commentLines := strings.Split(comment.Content, "\n")
					for _, cl := range commentLines {
						buf.WriteString("  " + cl + "\n")
					}
					buf.WriteString(strings.Repeat("-", 80) + "\n\n")
				}
				newLine++
				continue
			}

			newLine++
		} else {
			// Context line - both line numbers
			oldNum = fmt.Sprintf("%4d", oldLine)
			newNum = fmt.Sprintf("%4d", newLine)
			diffLine = " " + line
			oldLine++
			newLine++
		}

		buf.WriteString(fmt.Sprintf("%s | %s | %s\n", oldNum, newNum, diffLine))
	}

	buf.WriteString("\n")
}

// saveHTMLOutput saves formatted HTML output with GitHub-style review UI
func saveHTMLOutput(path string, result *diffReviewResponse, verbose bool) error {
	var buf bytes.Buffer

	// HTML header with embedded CSS and basic structure
	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>LiveReview Results</title>
    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <script>
        // Define functions early so onclick handlers can reference them
        function toggleFile(fileId) {
            const file = document.getElementById(fileId);
            if (file.classList.contains('expanded')) {
                file.classList.remove('expanded');
                file.classList.add('collapsed');
            } else {
                file.classList.remove('collapsed');
                file.classList.add('expanded');
            }
        }

        let allExpanded = false;
        function toggleAll() {
            const files = document.querySelectorAll('.file');
            const button = document.querySelector('.expand-all');
            if (allExpanded) {
                files.forEach(f => {
                    f.classList.remove('expanded');
                    f.classList.add('collapsed');
                });
                button.textContent = 'Expand All Files';
                allExpanded = false;
            } else {
                files.forEach(f => {
                    f.classList.remove('collapsed');
                    f.classList.add('expanded');
                });
                button.textContent = 'Collapse All Files';
                allExpanded = true;
            }
        }
    </script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
            font-size: 14px;
            line-height: 1.5;
            color: #24292f;
            background-color: #f6f8fa;
            display: flex;
            height: 100vh;
            overflow: hidden;
        }
        .sidebar {
            width: 300px;
            background: white;
            border-right: 1px solid #d0d7de;
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }
        .sidebar-header {
            padding: 16px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
        }
        .sidebar-header h2 {
            font-size: 14px;
            font-weight: 600;
            color: #24292f;
            margin-bottom: 4px;
        }
        .sidebar-stats {
            font-size: 12px;
            color: #57606a;
        }
        .sidebar-content {
            flex: 1;
            overflow-y: auto;
            padding: 8px 0;
        }
        .sidebar-file {
            padding: 8px 16px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
            border-left: 3px solid transparent;
        }
        .sidebar-file:hover {
            background: #f6f8fa;
        }
        .sidebar-file.active {
            background: #ddf4ff;
            border-left-color: #0969da;
        }
        .sidebar-file-name {
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
            flex: 1;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        .sidebar-file-badge {
            background: #0969da;
            color: white;
            padding: 2px 6px;
            border-radius: 10px;
            font-size: 10px;
            font-weight: 600;
        }
        .main-content {
            flex: 1;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            width: 100%;
        }
        .header {
            padding: 16px 20px;
            background: #ffffff;
            border-bottom: 1px solid #d0d7de;
            position: sticky;
            top: 0;
            z-index: 100;
            height: 60px;
            box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
        }
        .header h1 { font-size: 24px; font-weight: 600; margin-bottom: 8px; }
        .header .meta { color: #57606a; font-size: 12px; }
        .summary {
            padding: 16px 20px;
            background: #ddf4ff;
            border-bottom: 1px solid #54aeff;
        }
        .summary h1 { font-size: 18px; font-weight: 600; margin-bottom: 12px; margin-top: 16px; }
        .summary h1:first-child { margin-top: 0; }
        .summary h2 { font-size: 16px; font-weight: 600; margin-bottom: 10px; margin-top: 14px; }
        .summary h3 { font-size: 14px; font-weight: 600; margin-bottom: 8px; margin-top: 12px; }
        .summary p { margin-bottom: 8px; }
        .summary ul, .summary ol { margin-left: 20px; margin-bottom: 8px; }
        .summary code {
            background: rgba(175, 184, 193, 0.2);
            padding: 2px 6px;
            border-radius: 3px;
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
        }
        .summary pre {
            background: rgba(175, 184, 193, 0.2);
            padding: 12px;
            border-radius: 6px;
            overflow-x: auto;
            margin-bottom: 8px;
        }
        .summary pre code {
            background: none;
            padding: 0;
        }
        .summary strong { font-weight: 600; }
        .stats {
            padding: 12px 20px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
            display: flex;
            gap: 20px;
            font-size: 13px;
        }
        .stats .stat { font-weight: 600; }
        .stats .stat .count { color: #0969da; }
        .file {
            border-bottom: 1px solid #d0d7de;
        }
        .file:last-child { border-bottom: none; }
        .file-header {
            padding: 12px 20px;
            background: #f6f8fa;
            border-bottom: 1px solid #d0d7de;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .file-header:hover { background: #eaeef2; }
        .file-header .filename {
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-weight: 600;
            flex: 1;
        }
        .file-header .comment-count {
            background: #0969da;
            color: white;
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }
        .file-header .toggle { font-size: 12px; color: #57606a; }
        .file-content { display: none; }
        .file.expanded .file-content { display: block; }
        .file.expanded .file-header .toggle::before { content: "▼ "; }
        .file.collapsed .file-header .toggle::before { content: "▶ "; }
        .diff-table {
            width: 100%;
            border-collapse: collapse;
            font-family: ui-monospace, SFMono-Regular, monospace;
            font-size: 12px;
        }
        .diff-table td {
            padding: 0 8px;
            border: none;
            vertical-align: top;
        }
        .diff-line { background: #ffffff; }
        .diff-line:hover { background: #f6f8fa; }
        .line-num {
            width: 50px;
            color: #57606a;
            text-align: right;
            user-select: none;
            padding: 0 8px;
            background: #f6f8fa;
        }
        .line-content {
            white-space: pre;
            padding-left: 12px;
            width: 100%;
        }
        .diff-add { background: #dafbe1; }
        .diff-add .line-content { background: #dafbe1; }
        .diff-del { background: #ffebe9; }
        .diff-del .line-content { background: #ffebe9; }
        .diff-context .line-content { background: #ffffff; }
        .comment-row {
            background: #fff8c5;
            border-top: 1px solid #d4a72c;
            border-bottom: 1px solid #d4a72c;
        }
        .comment-container {
            padding: 12px 16px;
            margin: 8px 50px 8px 110px;
            background: white;
            border: 1px solid #d4a72c;
            border-radius: 6px;
        }
        .comment-header {
            display: flex;
            align-items: center;
            gap: 8px;
            margin-bottom: 8px;
            font-weight: 600;
        }
        .comment-badge {
            padding: 2px 8px;
            border-radius: 12px;
            font-size: 11px;
            font-weight: 600;
            text-transform: uppercase;
        }
        .badge-info { background: #ddf4ff; color: #0969da; }
        .badge-warning { background: #fff8c5; color: #9a6700; }
        .badge-error { background: #ffebe9; color: #cf222e; }
        .comment-category {
            color: #57606a;
            font-size: 12px;
            font-weight: normal;
        }
        .comment-body {
            color: #24292f;
            line-height: 1.5;
            white-space: pre-wrap;
        }
        .hunk-header {
            background: #f6f8fa;
            color: #57606a;
            padding: 4px 8px;
            font-weight: 600;
            border-top: 1px solid #d0d7de;
            border-bottom: 1px solid #d0d7de;
        }
        .footer {
            padding: 16px 20px;
            text-align: center;
            color: #57606a;
            font-size: 12px;
            background: #f6f8fa;
        }
        .expand-all {
            padding: 8px 16px;
            margin: 10px 20px;
            background: #0969da;
            color: white;
            border: none;
            border-radius: 6px;
            cursor: pointer;
            font-size: 13px;
            font-weight: 600;
        }
        .expand-all:hover { background: #0860ca; }

        /* Scrollbar styling */
        .sidebar-content::-webkit-scrollbar,
        .main-content::-webkit-scrollbar {
            width: 8px;
        }
        .sidebar-content::-webkit-scrollbar-track,
        .main-content::-webkit-scrollbar-track {
            background: #f6f8fa;
        }
        .sidebar-content::-webkit-scrollbar-thumb,
        .main-content::-webkit-scrollbar-thumb {
            background: #d0d7de;
            border-radius: 4px;
        }
        .sidebar-content::-webkit-scrollbar-thumb:hover,
        .main-content::-webkit-scrollbar-thumb:hover {
            background: #afb8c1;
        }
    </style>
</head>
<body>
    <div class="sidebar">
        <div class="sidebar-header">
            <h2>📂 Files</h2>
            <div class="sidebar-stats" id="sidebar-stats"></div>
        </div>
        <div class="sidebar-content" id="sidebar-files">
        </div>
    </div>
    <div class="main-content">
        <div class="container">
            <div class="header">
                <h1>🔍 LiveReview Results</h1>
                <div class="meta">Generated: ` + time.Now().Format("2006-01-02 15:04:05 MST") + `</div>
            </div>
`)

	// Summary section - store raw markdown in a script tag for rendering
	if result.Summary != "" {
		buf.WriteString(`        <script type="text/markdown" id="summary-markdown">`)
		buf.WriteString(html.EscapeString(result.Summary))
		buf.WriteString(`</script>
        <div class="summary" id="summary-content"></div>
`)
	}

	// Stats
	totalComments := countTotalComments(result.Files)
	buf.WriteString(fmt.Sprintf(`        <div class="stats">
            <div class="stat">Files: <span class="count">%d</span></div>
            <div class="stat">Comments: <span class="count">%d</span></div>
        </div>
`, len(result.Files), totalComments))

	// Expand all button
	buf.WriteString(`        <button class="expand-all" onclick="toggleAll()">Expand All Files</button>
`)

	// Files
	if len(result.Files) > 0 {
		for _, file := range result.Files {
			renderHTMLFile(&buf, file)
		}
	} else {
		buf.WriteString(`        <div style="padding: 40px 20px; text-align: center; color: #57606a;">
            No files reviewed or no comments generated.
        </div>
`)
	}

	// Footer
	buf.WriteString(fmt.Sprintf(`        <div class="footer">
            Review complete: %d total comment(s)
        </div>
        </div>
    </div>
    </div>

    <script>
        // Initialize on page load
        document.addEventListener('DOMContentLoaded', function() {
            // Render markdown in summary
            const summaryMarkdown = document.getElementById('summary-markdown');
            const summaryEl = document.getElementById('summary-content');
            if (summaryMarkdown && summaryEl && typeof marked !== 'undefined') {
                const markdownText = summaryMarkdown.textContent;
                summaryEl.innerHTML = marked.parse(markdownText);
            }

            // Build sidebar file list
            const sidebarFiles = document.getElementById('sidebar-files');
            const files = document.querySelectorAll('.file');
            
            files.forEach((file, index) => {
                const fileName = file.querySelector('.filename').textContent;
                const commentCount = file.querySelector('.comment-count');
                const hasComments = commentCount !== null;
                const count = hasComments ? commentCount.textContent : '0';
                
                const sidebarItem = document.createElement('div');
                sidebarItem.className = 'sidebar-file';
                sidebarItem.dataset.fileId = file.id;
                
                const nameSpan = document.createElement('span');
                nameSpan.className = 'sidebar-file-name';
                nameSpan.textContent = fileName;
                nameSpan.title = fileName;
                sidebarItem.appendChild(nameSpan);
                
                if (hasComments) {
                    const badge = document.createElement('span');
                    badge.className = 'sidebar-file-badge';
                    badge.textContent = count;
                    sidebarItem.appendChild(badge);
                }
                
                sidebarItem.addEventListener('click', function() {
                    // Remove active from all
                    document.querySelectorAll('.sidebar-file').forEach(f => f.classList.remove('active'));
                    // Add active to clicked
                    sidebarItem.classList.add('active');
                    
                    // Expand the file if collapsed BEFORE scrolling
                    if (file.classList.contains('collapsed')) {
                        toggleFile(file.id);
                    }
                    
                    // Scroll to file accounting for fixed header
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const fileRect = file.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + fileRect.top - mainContentRect.top - headerHeight - 10;
                    
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                });
                
                sidebarFiles.appendChild(sidebarItem);
            });

            // Update sidebar stats
            const stats = document.getElementById('sidebar-stats');
            const totalFiles = files.length;
            const totalComments = %d;
            stats.textContent = totalFiles + ' files • ' + totalComments + ' comments';

            // Auto-expand files with comments on load
            const filesWithComments = document.querySelectorAll('.file[data-has-comments="true"]');
            filesWithComments.forEach(f => f.classList.add('expanded'));

            // Highlight active file in sidebar on scroll
            const mainContent = document.querySelector('.main-content');
            mainContent.addEventListener('scroll', function() {
                let currentFile = null;
                files.forEach(file => {
                    const rect = file.getBoundingClientRect();
                    if (rect.top >= 0 && rect.top < window.innerHeight / 2) {
                        currentFile = file;
                    }
                });
                
                if (currentFile) {
                    document.querySelectorAll('.sidebar-file').forEach(f => f.classList.remove('active'));
                    const sidebarItem = document.querySelector('.sidebar-file[data-file-id="' + currentFile.id + '"]');
                    if (sidebarItem) {
                        sidebarItem.classList.add('active');
                    }
                }
            });
        });
    </script>
</body>
</html>
`, totalComments, totalComments))

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("HTML output saved to: %s (%d bytes)", path, buf.Len())
		log.Printf("Open in browser: file://%s", path)
	}

	return nil
}

// renderHTMLFile renders a single file's diff and comments as HTML
func renderHTMLFile(buf *bytes.Buffer, file diffReviewFileResult) {
	fileID := strings.ReplaceAll(file.FilePath, "/", "_")
	hasComments := len(file.Comments) > 0

	buf.WriteString(fmt.Sprintf(`        <div class="file collapsed" id="file_%s" data-has-comments="%t">
            <div class="file-header" onclick="toggleFile('file_%s')">
                <span class="toggle"></span>
                <span class="filename">%s</span>
`, fileID, hasComments, fileID, html.EscapeString(file.FilePath)))

	if hasComments {
		buf.WriteString(fmt.Sprintf(`                <span class="comment-count">%d</span>
`, len(file.Comments)))
	}

	buf.WriteString(`            </div>
            <div class="file-content">
`)

	if !hasComments {
		buf.WriteString(`                <div style="padding: 20px; text-align: center; color: #57606a;">
                    No comments for this file.
                </div>
`)
	} else {
		// Create comment lookup map
		commentsByLine := make(map[int][]diffReviewComment)
		for _, comment := range file.Comments {
			commentsByLine[comment.Line] = append(commentsByLine[comment.Line], comment)
		}

		// Render hunks
		buf.WriteString(`                <table class="diff-table">
`)
		for _, hunk := range file.Hunks {
			renderHTMLHunk(buf, hunk, commentsByLine)
		}
		buf.WriteString(`                </table>
`)
	}

	buf.WriteString(`            </div>
        </div>
`)
}

// renderHTMLHunk renders a single hunk with inline comments
func renderHTMLHunk(buf *bytes.Buffer, hunk diffReviewHunk, commentsByLine map[int][]diffReviewComment) {
	// Hunk header
	buf.WriteString(fmt.Sprintf(`                    <tr>
                        <td colspan="3" class="hunk-header">@@ -%d,%d +%d,%d @@</td>
                    </tr>
`, hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount))

	// Parse hunk content
	lines := strings.Split(hunk.Content, "\n")
	oldLine := hunk.OldStartLine
	newLine := hunk.NewStartLine

	for _, line := range lines {
		if len(line) == 0 || strings.HasPrefix(line, "@@") {
			continue
		}

		var oldNum, newNum, content, class string

		if strings.HasPrefix(line, "-") {
			oldNum = fmt.Sprintf("%d", oldLine)
			newNum = ""
			content = html.EscapeString(line)
			class = "diff-del"
			oldLine++
		} else if strings.HasPrefix(line, "+") {
			oldNum = ""
			newNum = fmt.Sprintf("%d", newLine)
			content = html.EscapeString(line)
			class = "diff-add"

			// Render the diff line
			buf.WriteString(fmt.Sprintf(`                    <tr class="diff-line %s">
                        <td class="line-num">%s</td>
                        <td class="line-num">%s</td>
                        <td class="line-content">%s</td>
                    </tr>
`, class, oldNum, newNum, content))

			// Check for comments and render them
			if comments, hasComment := commentsByLine[newLine]; hasComment {
				for _, comment := range comments {
					renderHTMLComment(buf, comment)
				}
			}

			newLine++
			continue
		} else {
			oldNum = fmt.Sprintf("%d", oldLine)
			newNum = fmt.Sprintf("%d", newLine)
			content = html.EscapeString(" " + line)
			class = "diff-context"
			oldLine++
			newLine++
		}

		buf.WriteString(fmt.Sprintf(`                    <tr class="diff-line %s">
                        <td class="line-num">%s</td>
                        <td class="line-num">%s</td>
                        <td class="line-content">%s</td>
                    </tr>
`, class, oldNum, newNum, content))
	}
}

// renderHTMLComment renders a comment inline in the diff
func renderHTMLComment(buf *bytes.Buffer, comment diffReviewComment) {
	severity := strings.ToLower(comment.Severity)
	if severity == "" {
		severity = "info"
	}

	badgeClass := "badge-" + severity
	if severity != "info" && severity != "warning" && severity != "error" {
		badgeClass = "badge-info"
	}

	buf.WriteString(`                    <tr class="comment-row">
                        <td colspan="3">
                            <div class="comment-container">
                                <div class="comment-header">
                                    <span class="comment-badge ` + badgeClass + `">` + strings.ToUpper(severity) + `</span>
`)

	if comment.Category != "" {
		buf.WriteString(`                                    <span class="comment-category">` + html.EscapeString(comment.Category) + `</span>
`)
	}

	buf.WriteString(`                                </div>
                                <div class="comment-body">` + html.EscapeString(comment.Content) + `</div>
                            </div>
                        </td>
                    </tr>
`)
}

// serveHTML starts an HTTP server to serve the HTML file
func serveHTML(htmlPath string, port int) error {
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("HTML file not found: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Starting HTTP server on %s", url)
	log.Printf("Serving: %s", absPath)
	log.Printf("Press Ctrl+C to stop the server")

	// Try to open browser
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	// Setup HTTP handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, absPath)
	})

	// Start server
	addr := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// openBrowser tries to open the URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch {
	case fileExists("/usr/bin/xdg-open"):
		cmd = exec.Command("xdg-open", url)
	case fileExists("/usr/bin/open"):
		cmd = exec.Command("open", url)
	case fileExists("/mnt/c/Windows/System32/cmd.exe"):
		// WSL
		cmd = exec.Command("/mnt/c/Windows/System32/cmd.exe", "/c", "start", url)
	default:
		log.Printf("Could not detect browser opener. Please open manually: %s", url)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
		log.Printf("Please open manually: %s", url)
	}
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
