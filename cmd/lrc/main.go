package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
				Value:   "staged",
				Usage:   "diff source: staged, working, range, or file",
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
				Name:     "api-key",
				Usage:    "API key for authentication",
				EnvVars:  []string{"LRC_API_KEY"},
				Required: true,
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

	// Submit review
	apiURL := c.String("api-url")
	apiKey := c.String("api-key")
	reviewID, err := submitReview(apiURL, apiKey, base64Diff, repoName, verbose)
	if err != nil {
		return fmt.Errorf("failed to submit review: %w", err)
	}

	if verbose {
		log.Printf("Review submitted, ID: %s", reviewID)
	}

	// Poll for completion
	pollInterval := c.Duration("poll-interval")
	timeout := c.Duration("timeout")
	result, err := pollReview(apiURL, apiKey, reviewID, pollInterval, timeout, verbose)
	if err != nil {
		return fmt.Errorf("failed to poll review: %w", err)
	}

	// Render result
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
