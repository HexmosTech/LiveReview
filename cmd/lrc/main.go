package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// Version information (set via ldflags during build)
const appVersion = "v0.1.0" // Semantic version - bump this for releases

var (
	version   = appVersion // Can be overridden via ldflags
	buildTime = "unknown"
	gitCommit = "unknown"
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

const (
	defaultAPIURL       = "http://localhost:8888"
	defaultPollInterval = 2 * time.Second
	defaultTimeout      = 5 * time.Minute
	defaultOutputFormat = "pretty"
	commitMessageFile   = "livereview_commit_message"
	editorWrapperScript = "lrc_editor.sh"
	editorBackupFile    = ".lrc_editor_backup"
)

// highlightURL adds ANSI color to make served links stand out in terminals.
func highlightURL(url string) string {
	return "\033[36m" + url + "\033[0m"
}

var baseFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "repo-name",
		Usage:   "repository name (defaults to current directory basename)",
		EnvVars: []string{"LRC_REPO_NAME"},
	},
	&cli.BoolFlag{
		Name:    "staged",
		Usage:   "use staged changes instead of working tree",
		EnvVars: []string{"LRC_STAGED"},
	},
	&cli.StringFlag{
		Name:    "range",
		Usage:   "git range for staged/working diff override (e.g., HEAD~1..HEAD)",
		EnvVars: []string{"LRC_RANGE"},
	},
	&cli.StringFlag{
		Name:    "diff-file",
		Usage:   "path to pre-generated diff file",
		EnvVars: []string{"LRC_DIFF_FILE"},
	},
	&cli.StringFlag{
		Name:    "api-url",
		Value:   defaultAPIURL,
		Usage:   "LiveReview API base URL",
		EnvVars: []string{"LRC_API_URL"},
	},
	&cli.StringFlag{
		Name:    "api-key",
		Usage:   "API key for authentication (can be set in ~/.lrc.toml or env var)",
		EnvVars: []string{"LRC_API_KEY"},
	},
	&cli.StringFlag{
		Name:    "output",
		Value:   defaultOutputFormat,
		Usage:   "output format: pretty or json",
		EnvVars: []string{"LRC_OUTPUT"},
	},
	&cli.StringFlag{
		Name:    "save-html",
		Usage:   "save formatted HTML output (GitHub-style review) to this file",
		EnvVars: []string{"LRC_SAVE_HTML"},
	},
	&cli.BoolFlag{
		Name:    "serve",
		Usage:   "start HTTP server to serve the HTML output (auto-creates HTML when omitted)",
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
		Usage:   "enable verbose output",
		EnvVars: []string{"LRC_VERBOSE"},
	},
	&cli.BoolFlag{
		Name:    "precommit",
		Usage:   "pre-commit mode: interactive prompts for commit decision (Ctrl-C=abort, Ctrl-S=skip+commit, Enter=commit)",
		EnvVars: []string{"LRC_PRECOMMIT"},
	},
}

var debugFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "diff-source",
		Usage:   "diff source: working, staged, range, or file (debug override)",
		EnvVars: []string{"LRC_DIFF_SOURCE"},
		Hidden:  true,
	},
	&cli.DurationFlag{
		Name:    "poll-interval",
		Value:   defaultPollInterval,
		Usage:   "interval between status polls",
		EnvVars: []string{"LRC_POLL_INTERVAL"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Value:   defaultTimeout,
		Usage:   "maximum time to wait for review completion",
		EnvVars: []string{"LRC_TIMEOUT"},
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
}

func main() {
	app := &cli.App{
		Name:    "lrc",
		Usage:   "LiveReview CLI - submit local diffs for AI review",
		Version: version,
		Flags:   baseFlags,
		Commands: []*cli.Command{
			{
				Name:    "review",
				Aliases: []string{"r"},
				Usage:   "Run a review with sensible defaults",
				Flags:   baseFlags,
				Action:  runReviewSimple,
			},
			{
				Name:   "review-debug",
				Usage:  "Run a review with advanced debug options",
				Flags:  append(baseFlags, debugFlags...),
				Action: runReviewDebug,
			},
			{
				Name:  "install-hooks",
				Usage: "Install pre-commit and commit-msg Git hooks for automatic code review",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Usage: "overwrite existing lrc hook sections",
					},
				},
				Action: runInstallHooks,
			},
			{
				Name:   "uninstall-hooks",
				Usage:  "Remove lrc Git hooks, preserving other hook content",
				Action: runUninstallHooks,
			},
			{
				Name:  "version",
				Usage: "Show version information",
				Action: func(c *cli.Context) error {
					fmt.Printf("lrc version %s\n", version)
					fmt.Printf("  Build time: %s\n", buildTime)
					fmt.Printf("  Git commit: %s\n", gitCommit)
					return nil
				},
			},
		},
		Action: runReviewSimple,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type reviewOptions struct {
	repoName     string
	diffSource   string
	rangeVal     string
	diffFile     string
	apiURL       string
	apiKey       string
	pollInterval time.Duration
	timeout      time.Duration
	output       string
	saveBundle   string
	saveJSON     string
	saveText     string
	saveHTML     string
	serve        bool
	port         int
	verbose      bool
	precommit    bool
}

func runReviewSimple(c *cli.Context) error {
	opts := buildOptionsFromContext(c, false)
	return runReviewWithOptions(opts)
}

func runReviewDebug(c *cli.Context) error {
	opts := buildOptionsFromContext(c, true)
	return runReviewWithOptions(opts)
}

func buildOptionsFromContext(c *cli.Context, includeDebug bool) reviewOptions {
	opts := reviewOptions{
		repoName:  c.String("repo-name"),
		rangeVal:  c.String("range"),
		diffFile:  c.String("diff-file"),
		apiURL:    c.String("api-url"),
		apiKey:    c.String("api-key"),
		output:    c.String("output"),
		saveHTML:  c.String("save-html"),
		serve:     c.Bool("serve"),
		port:      c.Int("port"),
		verbose:   c.Bool("verbose"),
		precommit: c.Bool("precommit"),
		saveJSON:  c.String("save-json"),
		saveText:  c.String("save-text"),
	}

	staged := c.Bool("staged")
	diffSource := c.String("diff-source")

	if opts.diffFile != "" {
		diffSource = "file"
	} else if opts.rangeVal != "" {
		diffSource = "range"
	} else if staged {
		diffSource = "staged"
	}

	if diffSource == "" {
		diffSource = "working"
	}

	opts.diffSource = diffSource

	if includeDebug {
		opts.pollInterval = c.Duration("poll-interval")
		opts.timeout = c.Duration("timeout")
		opts.saveBundle = c.String("save-bundle")
	} else {
		opts.pollInterval = defaultPollInterval
		opts.timeout = defaultTimeout
	}

	if opts.apiURL == "" {
		opts.apiURL = defaultAPIURL
	}

	if opts.output == "" {
		opts.output = defaultOutputFormat
	}

	return opts
}

// applyDefaultHTMLServe enables HTML saving/serving when the user runs with defaults.
// It only triggers when no HTML path or serve flag was provided and the output format is the default.
func applyDefaultHTMLServe(opts *reviewOptions) (string, error) {
	if opts.saveHTML != "" || opts.serve || opts.output != defaultOutputFormat {
		return "", nil
	}

	tmpFile, err := os.CreateTemp("", "lrc-review-*.html")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary HTML file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to prepare temporary HTML file: %w", err)
	}

	opts.saveHTML = tmpFile.Name()
	opts.serve = true

	return opts.saveHTML, nil
}

// pickServePort tries the requested port, then increments by 1 up to maxTries to find a free port.
func pickServePort(preferredPort, maxTries int) (int, error) {
	for i := 0; i < maxTries; i++ {
		candidate := preferredPort + i
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			ln.Close()
			return candidate, nil
		}
	}

	return 0, fmt.Errorf("no available port found starting from %d", preferredPort)
}

func runReviewWithOptions(opts reviewOptions) error {
	verbose := opts.verbose
	var tempHTMLPath string
	var commitMsgPath string

	if opts.precommit {
		gitDir, err := resolveGitDir()
		if err != nil {
			return fmt.Errorf("precommit mode requires a git repository: %w", err)
		}
		commitMsgPath = filepath.Join(gitDir, commitMessageFile)
		_ = clearCommitMessageFile(commitMsgPath)
	}

	// Load configuration from config file or overrides
	config, err := loadConfigValues(opts.apiKey, opts.apiURL, verbose)
	if err != nil {
		return err
	}

	// Determine repo name
	repoName := opts.repoName
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

	var result *diffReviewResponse

	// Collect diff
	diffContent, err := collectDiffWithOptions(opts)
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
	if bundlePath := opts.saveBundle; bundlePath != "" {
		if err := saveBundleForInspection(bundlePath, diffContent, zipData, base64Diff, verbose); err != nil {
			return fmt.Errorf("failed to save bundle: %w", err)
		}
	}

	// Submit review
	reviewID, err := submitReview(config.APIURL, config.APIKey, base64Diff, repoName, verbose)
	if err != nil {
		return fmt.Errorf("failed to submit review: %w", err)
	}

	fmt.Printf("Review submitted, ID: %s\n", reviewID)

	// In precommit mode, ensure unbuffered output
	if opts.precommit {
		// Force flush and set unbuffered
		os.Stdout.Sync()
		os.Stderr.Sync()
	}

	// Track CLI usage (best-effort, non-blocking)
	go trackCLIUsage(config.APIURL, config.APIKey, verbose)

	// In precommit mode, set up decision channels for Ctrl-C / Ctrl-S and ensure cleanup
	decisionCode := -1
	if opts.precommit {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		decisionChan := make(chan int, 1) // 0 commit, 1 abort, 2 skip-review (proceed)
		stopCtrlS := make(chan struct{})
		var stopCtrlSOnce sync.Once
		stopCtrlSFn := func() { stopCtrlSOnce.Do(func() { close(stopCtrlS) }) }

		// Ctrl-C -> abort commit
		go func() {
			<-sigChan
			decisionChan <- 1
		}()

		// Ctrl-S -> skip review but still commit; Ctrl-C captured in raw mode fallback
		go func() {
			code, err := handleCtrlKeyWithCancel(stopCtrlS)
			if err == nil && code != 0 {
				decisionChan <- code
			}
		}()

		fmt.Println("💡 Press Ctrl-C to abort commit, or Ctrl-S to skip review and commit")
		os.Stdout.Sync()

		// Poll concurrently and race with decisions
		var pollResult *diffReviewResponse
		var pollErr error
		pollDone := make(chan struct{})
		go func() {
			pollResult, pollErr = pollReview(config.APIURL, config.APIKey, reviewID, opts.pollInterval, opts.timeout, verbose)
			close(pollDone)
		}()

		select {
		case decisionCode = <-decisionChan:
			stopCtrlSFn()
		case <-pollDone:
			stopCtrlSFn()
			if pollErr != nil {
				return fmt.Errorf("failed to poll review: %w", pollErr)
			}
			result = pollResult
		}

		// If a decision happened before poll finished, act now
		if decisionCode != -1 {
			switch decisionCode {
			case 1:
				fmt.Println("\n❌ Review and commit aborted by user")
			case 2:
				fmt.Println("\n⏭️  Review skipped, proceeding with commit")
			}
			fmt.Println()
			return cli.Exit("precommit decision", decisionCode)
		}
	} else {
		// Non-precommit: just poll
		var pollErr error
		result, pollErr = pollReview(config.APIURL, config.APIKey, reviewID, opts.pollInterval, opts.timeout, verbose)
		if pollErr != nil {
			return fmt.Errorf("failed to poll review: %w", pollErr)
		}
	}

	autoHTMLPath, err := applyDefaultHTMLServe(&opts)
	if err != nil {
		return err
	}
	tempHTMLPath = autoHTMLPath
	if tempHTMLPath != "" {
		defer func() {
			if err := os.Remove(tempHTMLPath); err == nil {
				if verbose {
					log.Printf("Removed temporary HTML file: %s", tempHTMLPath)
				}
			} else if verbose {
				log.Printf("Could not remove temporary HTML file %s: %v", tempHTMLPath, err)
			}
		}()
	}

	// Save JSON response if requested
	if jsonPath := opts.saveJSON; jsonPath != "" {
		if err := saveJSONResponse(jsonPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save JSON response: %w", err)
		}
	}

	// Save formatted text output if requested
	if textPath := opts.saveText; textPath != "" {
		if err := saveTextOutput(textPath, result, verbose); err != nil {
			return fmt.Errorf("failed to save text output: %w", err)
		}
	}

	// Save HTML output if requested
	if htmlPath := opts.saveHTML; htmlPath != "" {
		if err := saveHTMLOutput(htmlPath, result, verbose, opts.precommit); err != nil {
			return fmt.Errorf("failed to save HTML output: %w", err)
		}

		// Ensure we're on a fresh line after status updates
		fmt.Printf("\n")

		if autoHTMLPath != "" {
			fmt.Printf("HTML review saved to (auto-selected): %s\n", htmlPath)
		} else {
			fmt.Printf("HTML review saved to: %s\n", htmlPath)
		}

		// Start HTTP server if --serve flag is set
		if opts.serve {
			selectedPort, err := pickServePort(opts.port, 10)
			if err != nil {
				return fmt.Errorf("failed to find available port: %w", err)
			}
			if selectedPort != opts.port {
				fmt.Printf("Port %d is busy; serving on %d instead.\n", opts.port, selectedPort)
				opts.port = selectedPort
			}

			// Precommit mode: interactive prompt for commit decision
			if opts.precommit {
				// Don't render to stdout in precommit mode, go straight to prompt
				exitCode := serveHTMLPrecommit(htmlPath, opts.port, commitMsgPath)
				os.Exit(exitCode)
			}

			// Normal mode: serve and block
			serveURL := fmt.Sprintf("http://localhost:%d", opts.port)
			fmt.Printf("Serving HTML review at: %s\n", highlightURL(serveURL))
			if err := serveHTML(htmlPath, opts.port); err != nil {
				return fmt.Errorf("failed to serve HTML: %w", err)
			}
		}
	}

	// Render result to stdout (skip in precommit mode - handled by prompt)
	if !opts.precommit {
		if err := renderResult(result, opts.output); err != nil {
			return fmt.Errorf("failed to render result: %w", err)
		}
	}

	return nil
}

func collectDiffWithOptions(opts reviewOptions) ([]byte, error) {
	diffSource := opts.diffSource
	verbose := opts.verbose

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
		rangeVal := opts.rangeVal
		if rangeVal == "" {
			return nil, fmt.Errorf("--range is required when diff-source=range")
		}
		if verbose {
			log.Printf("Collecting diff for range: %s", rangeVal)
		}
		return runGitCommand("git", "diff", rangeVal)

	case "file":
		filePath := opts.diffFile
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

// resolveGitDir returns the absolute path to the repository's .git directory.
func resolveGitDir() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to locate git directory: %w", err)
	}

	gitDir := strings.TrimSpace(string(out))
	if gitDir == "" {
		return "", fmt.Errorf("git directory path is empty")
	}

	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory: %w", err)
	}

	return filepath.Join(cwd, gitDir), nil
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

// trackCLIUsage sends a telemetry ping to the backend to track CLI usage
// This is a best-effort call and failures are silently ignored
func trackCLIUsage(apiURL, apiKey string, verbose bool) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/cli-used"

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		if verbose {
			log.Printf("Failed to create telemetry request: %v", err)
		}
		return
	}

	req.Header.Set("X-API-Key", apiKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if verbose {
			log.Printf("Failed to send telemetry: %v", err)
		}
		return
	}
	defer resp.Body.Close()

	if verbose && resp.StatusCode == http.StatusOK {
		log.Println("CLI usage tracked successfully")
	}
}

func pollReview(apiURL, apiKey, reviewID string, pollInterval, timeout time.Duration, verbose bool) (*diffReviewResponse, error) {
	endpoint := strings.TrimSuffix(apiURL, "/") + "/api/v1/diff-review/" + reviewID
	deadline := time.Now().Add(timeout)
	start := time.Now()
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	fmt.Printf("Waiting for review completion (poll every %s, timeout %s)...\n", pollInterval, timeout)
	os.Stdout.Sync()

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

		statusLine := fmt.Sprintf("Status: %s | elapsed: %s", result.Status, time.Since(start).Truncate(time.Second))
		if isTTY {
			fmt.Printf("\r%-80s", statusLine)
			os.Stdout.Sync() // Force flush for real-time updates and clear prior text
		} else {
			fmt.Println(statusLine)
		}
		if verbose {
			log.Printf("%s", statusLine)
		}

		if result.Status == "completed" {
			fmt.Println()
			return &result, nil
		}

		if result.Status == "failed" {
			fmt.Println()
			return nil, fmt.Errorf("review failed")
		}

		time.Sleep(pollInterval)
	}

	fmt.Println()
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

// loadConfigValues attempts to load configuration from ~/.lrc.toml, then applies CLI/env overrides
func loadConfigValues(apiKeyOverride, apiURLOverride string, verbose bool) (*Config, error) {
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

	// Load API key: CLI/env overrides config file
	if apiKeyOverride != "" {
		config.APIKey = apiKeyOverride
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

	// Load API URL: CLI/env overrides config file
	if apiURLOverride != "" && apiURLOverride != defaultAPIURL {
		config.APIURL = apiURLOverride
		if verbose {
			log.Println("Using API URL from CLI flag or environment variable")
		}
	} else if k != nil && k.String("api_url") != "" {
		config.APIURL = k.String("api_url")
		if verbose {
			log.Println("Using API URL from config file")
		}
	} else {
		config.APIURL = defaultAPIURL
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

func saveHTMLOutput(path string, result *diffReviewResponse, verbose bool, precommit bool) error {
	// Prepare template data
	data := prepareHTMLData(result, precommit)

	// Render HTML using template
	htmlContent, err := renderHTMLTemplate(data)
	if err != nil {
		return fmt.Errorf("failed to render HTML template: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, []byte(htmlContent), 0644); err != nil {
		return err
	}

	if verbose {
		log.Printf("HTML output saved to: %s (%d bytes)", path, len(htmlContent))
		log.Printf("Open in browser: file://%s", path)
	}

	return nil
}

// renderHTMLFile renders a single file's diff and comments as HTML

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

// handleCtrlKeyWithCancel sets up raw terminal mode to detect Ctrl-S (skip) and Ctrl-C (abort)
// Returns decision codes: 2 for Ctrl-S, 1 for Ctrl-C, 0 if nothing, or error on cancellation/failure
func handleCtrlKeyWithCancel(stop <-chan struct{}) (int, error) {
	// Try to open /dev/tty directly
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return 0, err
	}
	defer tty.Close()

	// Set terminal to raw mode
	fd := int(tty.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}

	// Ensure restoration on exit
	defer term.Restore(fd, oldState)

	// Read bytes looking for Ctrl-S (0x13) or cancellation
	buf := make([]byte, 1)
	readChan := make(chan error, 1)

	go func() {
		for {
			n, err := tty.Read(buf)
			if err != nil || n == 0 {
				readChan <- err
				return
			}
			switch buf[0] {
			case 0x13: // Ctrl-S (XOFF)
				readChan <- nil
				return
			case 0x03: // Ctrl-C (ETX)
				readChan <- fmt.Errorf("ctrl-c")
				return
			}
		}
	}()

	select {
	case err := <-readChan:
		if err == nil {
			return 2, nil
		}
		if err.Error() == "ctrl-c" {
			return 1, nil
		}
		return 0, err
	case <-stop:
		// Cancelled - restore terminal and return error
		return 0, fmt.Errorf("cancelled")
	}
}

// persistCommitMessage writes the desired commit message to a temporary file that the commit-msg hook will consume.
func persistCommitMessage(commitMsgPath, message string) error {
	if commitMsgPath == "" {
		return nil
	}

	trimmed := strings.TrimRight(message, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return clearCommitMessageFile(commitMsgPath)
	}

	normalized := trimmed + "\n"
	return os.WriteFile(commitMsgPath, []byte(normalized), 0600)
}

// clearCommitMessageFile removes any pending commit-message override file.
func clearCommitMessageFile(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	if err := os.Remove(commitMsgPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// readCommitMessageFromRequest extracts an optional commit message from a JSON request body.
func readCommitMessageFromRequest(r *http.Request) string {
	if r.Body == nil {
		return ""
	}

	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil || len(body) == 0 {
		return ""
	}

	var payload struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	return strings.TrimRight(payload.Message, "\r\n")
}

// serveHTMLPrecommit serves HTML and waits for user decision
// Returns exit code: 0 = commit, 1 = don't commit
func serveHTMLPrecommit(htmlPath string, port int, commitMsgPath string) int {
	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get absolute path: %v\n", err)
		return 1
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil {
		fmt.Fprintf(os.Stderr, "HTML file not found: %v\n", err)
		return 1
	}

	_ = clearCommitMessageFile(commitMsgPath)

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("\n")
	fmt.Printf("🌐 Review available at: %s\n", highlightURL(url))
	fmt.Printf("\n")

	// Open browser
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(url)
	}()

	// Setup HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, absPath)
	})

	type precommitDecision struct {
		code    int
		message string
	}

	decisionChan := make(chan precommitDecision, 1) // 0=commit,2=skip-from-terminal,1=abort,3=skip-from-HTML-abort
	var decideOnce sync.Once
	decide := func(code int, message string) {
		decideOnce.Do(func() {
			decisionChan <- precommitDecision{code: code, message: message}
		})
	}

	// Pre-commit action endpoints (HTML buttons call these)
	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		decide(0, msg)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		decide(3, "")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Start server in background
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	serverReady := make(chan bool, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Give server a moment to start
	go func() {
		time.Sleep(200 * time.Millisecond)
		serverReady <- true
	}()

	<-serverReady

	// Wait for decision: Enter, Ctrl-C, HTML buttons
	// Set up signal handling for Ctrl-C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		decide(1, "")
	}()

	// Read from /dev/tty directly to avoid stdin issues in git hooks (Enter fallback, cooked mode)
	go func() {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			fmt.Println("Warning: Could not open terminal, auto-proceeding")
			time.Sleep(2 * time.Second)
			decide(0, "")
			return
		}
		defer tty.Close()

		reader := bufio.NewReader(tty)

		fmt.Printf("📋 Review complete. Choose action:\n")
		fmt.Printf("   [Enter]  Continue with commit\n")
		fmt.Printf("   [Ctrl-C] Abort commit\n")
		fmt.Printf("\nOptional: type a new commit message and press Enter to use it (leave blank to keep Git's message).\n")
		fmt.Printf("> ")
		os.Stdout.Sync()

		typedMessage, _ := reader.ReadString('\n')
		typedMessage = strings.TrimRight(strings.TrimRight(typedMessage, "\n"), "\r")

		fmt.Printf("\n[Enter] Continue with commit\n")
		fmt.Printf("[Ctrl-C] Abort commit\n")
		fmt.Printf("\nYour choice: ")
		os.Stdout.Sync()

		_, err = reader.ReadString('\n')
		if err != nil {
			decide(0, typedMessage)
			return
		}
		decide(0, typedMessage)
	}()

	// Wait for any decision source
	decision := <-decisionChan

	if commitMsgPath != "" {
		if decision.code == 0 && strings.TrimSpace(decision.message) != "" {
			if err := persistCommitMessage(commitMsgPath, decision.message); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store commit message: %v\n", err)
			}
		} else {
			_ = clearCommitMessageFile(commitMsgPath)
		}
	}

	switch decision.code {
	case 0:
		fmt.Println("\n✅ Proceeding with commit")
	case 2:
		fmt.Println("\n⏭️  Review skipped from terminal; proceeding with commit")
	case 3:
		fmt.Println("\n⏭️  Skip requested from review page; aborting commit")
	case 1:
		fmt.Println("\n❌ Commit aborted by user")
	}
	fmt.Println()
	server.Close()
	return decision.code
}

// =============================================================================
// GIT HOOK MANAGEMENT
// =============================================================================

const (
	lrcMarkerBegin = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd   = "# END lrc managed section"
)

// runInstallHooks installs pre-commit and commit-msg hooks with sentinel markers
func runInstallHooks(c *cli.Context) error {
	force := c.Bool("force")

	// Check if we're in a git repository
	if !isGitRepository() {
		return fmt.Errorf("not in a git repository (no .git directory found)")
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return err
	}

	hooksDir := ".git/hooks"
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	backupDir := ".git/.lrc_backups"
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Install pre-commit hook
	preCommitPath := filepath.Join(hooksDir, "pre-commit")
	if err := installHook(preCommitPath, generatePreCommitHook(), "pre-commit", backupDir, force); err != nil {
		return fmt.Errorf("failed to install pre-commit hook: %w", err)
	}

	// Install commit-msg hook
	commitMsgPath := filepath.Join(hooksDir, "commit-msg")
	if err := installHook(commitMsgPath, generateCommitMsgHook(), "commit-msg", backupDir, force); err != nil {
		return fmt.Errorf("failed to install commit-msg hook: %w", err)
	}

	if err := installEditorWrapper(gitDir); err != nil {
		return fmt.Errorf("failed to install editor wrapper: %w", err)
	}

	fmt.Println("✅ LiveReview hooks installed successfully!")
	fmt.Println()
	fmt.Println("Pre-commit hook will:")
	fmt.Println("  • Run 'lrc review --staged --precommit' before each commit")
	fmt.Println("  • Show review progress and open browser")
	fmt.Println("  • Wait for your decision: [Enter]=commit, [Ctrl-C]=abort")
	fmt.Println("  • Can be bypassed with 'git commit --no-verify'")
	fmt.Println()
	fmt.Println("Commit-msg hook will:")
	fmt.Println("  • Add 'LiveReview Pre-Commit Check: [ran|skipped]' trailer")
	fmt.Println()
	fmt.Println("To uninstall: lrc uninstall-hooks")

	return nil
}

// runUninstallHooks removes lrc-managed hook sections
func runUninstallHooks(c *cli.Context) error {
	if !isGitRepository() {
		return fmt.Errorf("not in a git repository (no .git directory found)")
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return err
	}

	hooksDir := ".git/hooks"
	hooks := []string{"pre-commit", "commit-msg"}
	removed := 0

	for _, hookName := range hooks {
		hookPath := filepath.Join(hooksDir, hookName)
		if err := uninstallHook(hookPath, hookName); err != nil {
			fmt.Printf("⚠️  Warning: failed to uninstall %s: %v\n", hookName, err)
		} else {
			removed++
		}
	}

	if removed > 0 {
		fmt.Printf("✅ Removed lrc hooks from %d file(s)\n", removed)
	} else {
		fmt.Println("ℹ️  No lrc hooks found to remove")
	}

	if err := uninstallEditorWrapper(gitDir); err != nil {
		fmt.Printf("⚠️  Warning: failed to clean editor wrapper: %v\n", err)
	}

	return nil
}

// isGitRepository checks if current directory is in a git repository
func isGitRepository() bool {
	_, err := os.Stat(".git")
	return err == nil
}

// installHook installs or updates a hook with lrc managed section
func installHook(hookPath, lrcSection, hookName, backupDir string, force bool) error {
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s", hookName, timestamp))

	// Check if hook file exists
	existingContent, err := os.ReadFile(hookPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing hook: %w", err)
	}

	if len(existingContent) == 0 {
		// No existing hook - create new file with just lrc section
		content := "#!/bin/sh\n" + lrcSection
		if err := os.WriteFile(hookPath, []byte(content), 0755); err != nil {
			return fmt.Errorf("failed to write hook: %w", err)
		}
		fmt.Printf("✅ Created %s\n", hookName)
		return nil
	}

	// Existing hook found - create backup
	if err := os.WriteFile(backupPath, existingContent, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	fmt.Printf("📁 Backup created: %s\n", backupPath)

	// Check if lrc section already exists
	contentStr := string(existingContent)
	if strings.Contains(contentStr, lrcMarkerBegin) {
		if !force {
			fmt.Printf("ℹ️  %s already has lrc section (use --force to update)\n", hookName)
			return nil
		}
		// Replace existing lrc section
		newContent := replaceLrcSection(contentStr, lrcSection)
		if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
			return fmt.Errorf("failed to update hook: %w", err)
		}
		fmt.Printf("✅ Updated %s (replaced lrc section)\n", hookName)
		return nil
	}

	// No lrc section - append it
	var newContent string
	if !strings.HasPrefix(contentStr, "#!/") {
		// No shebang - add one
		newContent = "#!/bin/sh\n" + lrcSection + "\n" + contentStr
	} else {
		// Has shebang - insert after first line
		lines := strings.SplitN(contentStr, "\n", 2)
		if len(lines) == 1 {
			newContent = lines[0] + "\n" + lrcSection
		} else {
			newContent = lines[0] + "\n" + lrcSection + "\n" + lines[1]
		}
	}

	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Updated %s (added lrc section)\n", hookName)

	return nil
}

// uninstallHook removes lrc-managed section from a hook file
func uninstallHook(hookPath, hookName string) error {
	content, err := os.ReadFile(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Hook doesn't exist, nothing to do
		}
		return fmt.Errorf("failed to read hook: %w", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, lrcMarkerBegin) {
		// No lrc section found
		return nil
	}

	// Remove lrc section
	newContent := removeLrcSection(contentStr)

	// If file is now empty or only has shebang, delete it
	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" || trimmed == "#!/bin/sh" {
		if err := os.Remove(hookPath); err != nil {
			return fmt.Errorf("failed to remove hook file: %w", err)
		}
		fmt.Printf("🗑️  Removed %s (was empty after removing lrc section)\n", hookName)
		return nil
	}

	// Write cleaned content back
	if err := os.WriteFile(hookPath, []byte(newContent), 0755); err != nil {
		return fmt.Errorf("failed to write hook: %w", err)
	}
	fmt.Printf("✅ Removed lrc section from %s\n", hookName)

	return nil
}

// installEditorWrapper sets core.editor to an lrc-managed wrapper that injects
// the precommit-provided message when available and falls back to the user's editor.
func installEditorWrapper(gitDir string) error {
	repoRoot := filepath.Dir(gitDir)
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	// Backup existing core.editor if set
	currentEditor, _ := readGitConfig(repoRoot, "core.editor")
	if currentEditor != "" {
		_ = os.WriteFile(backupPath, []byte(currentEditor), 0600)
	}

	script := fmt.Sprintf(`#!/bin/sh
set -e

OVERRIDE_FILE="%s"

if [ -f "$OVERRIDE_FILE" ] && [ -s "$OVERRIDE_FILE" ]; then
    cat "$OVERRIDE_FILE" > "$1"
    exit 0
fi

if [ -n "$LRC_FALLBACK_EDITOR" ]; then
    exec $LRC_FALLBACK_EDITOR "$@"
fi

if [ -n "$VISUAL" ]; then
    exec "$VISUAL" "$@"
fi

if [ -n "$EDITOR" ]; then
    exec "$EDITOR" "$@"
fi

exec vi "$@"
`, filepath.Join(gitDir, commitMessageFile))

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write editor wrapper: %w", err)
	}

	if err := setGitConfig(repoRoot, "core.editor", scriptPath); err != nil {
		return fmt.Errorf("failed to set core.editor: %w", err)
	}

	return nil
}

// uninstallEditorWrapper restores the previous editor (if backed up) and removes wrapper files.
func uninstallEditorWrapper(gitDir string) error {
	repoRoot := filepath.Dir(gitDir)
	scriptPath := filepath.Join(gitDir, editorWrapperScript)
	backupPath := filepath.Join(gitDir, editorBackupFile)

	if data, err := os.ReadFile(backupPath); err == nil {
		value := strings.TrimSpace(string(data))
		if value != "" {
			_ = setGitConfig(repoRoot, "core.editor", value)
		}
	} else {
		// No backup; remove config if set
		_ = unsetGitConfig(repoRoot, "core.editor")
	}

	_ = os.Remove(scriptPath)
	_ = os.Remove(backupPath)

	return nil
}

// readGitConfig reads a single git config key from the repository root.
func readGitConfig(repoRoot, key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// setGitConfig sets a git config key in the given repository.
func setGitConfig(repoRoot, key, value string) error {
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = repoRoot
	return cmd.Run()
}

// unsetGitConfig removes a git config key in the given repository.
func unsetGitConfig(repoRoot, key string) error {
	cmd := exec.Command("git", "config", "--unset", key)
	cmd.Dir = repoRoot
	return cmd.Run()
}

// replaceLrcSection replaces the lrc-managed section in hook content
func replaceLrcSection(content, newSection string) string {
	start := strings.Index(content, lrcMarkerBegin)
	if start == -1 {
		return content
	}

	end := strings.Index(content[start:], lrcMarkerEnd)
	if end == -1 {
		return content
	}
	end += start + len(lrcMarkerEnd)

	// Find end of line after marker
	if end < len(content) && content[end] == '\n' {
		end++
	}

	return content[:start] + newSection + "\n" + content[end:]
}

// removeLrcSection removes the lrc-managed section from hook content
func removeLrcSection(content string) string {
	start := strings.Index(content, lrcMarkerBegin)
	if start == -1 {
		return content
	}

	end := strings.Index(content[start:], lrcMarkerEnd)
	if end == -1 {
		return content
	}
	end += start + len(lrcMarkerEnd)

	// Find end of line after marker
	if end < len(content) && content[end] == '\n' {
		end++
	}

	// Remove the section, preserving content before and after
	return content[:start] + content[end:]
}

// generatePreCommitHook generates the pre-commit hook script
func generatePreCommitHook() string {
	return fmt.Sprintf(`%s
# lrc_version: %s
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates

# Detect if running in TTY (check stdout, not stdin - Git redirects stdin)
if [ -t 1 ]; then
    LRC_INTERACTIVE=1
else
    LRC_INTERACTIVE=0
fi

# State file for hook coordination
STATE_FILE=".git/livereview_state"
LOCK_DIR=".git/livereview_state.lock"

# Cleanup function
cleanup_lock() {
    rmdir "$LOCK_DIR" 2>/dev/null || true
}

# Set up cleanup trap
trap cleanup_lock EXIT INT TERM

# Acquire lock with timeout (5 minutes)
MAX_WAIT=300
WAITED=0

# Check for stale locks (>5 minutes old)
if [ -d "$LOCK_DIR" ]; then
    if command -v stat >/dev/null 2>&1; then
        # Try to get lock age
        LOCK_AGE=$(($(date +%%s) - $(stat -c %%Y "$LOCK_DIR" 2>/dev/null || stat -f %%m "$LOCK_DIR" 2>/dev/null || echo 0)))
        if [ "$LOCK_AGE" -gt 300 ]; then
            echo "Removing stale lock (${LOCK_AGE}s old)"
            rmdir "$LOCK_DIR" 2>/dev/null || true
        fi
    fi
fi

# Try to acquire lock
while ! mkdir "$LOCK_DIR" 2>/dev/null; do
    if [ $WAITED -ge $MAX_WAIT ]; then
        echo "Could not acquire lock after ${MAX_WAIT}s, skipping review" >&2
        exit 0
    fi
    sleep 1
    WAITED=$((WAITED + 1))
done

# Run review
if [ "$LRC_INTERACTIVE" = "1" ]; then
    echo "Running LiveReview pre-commit check..."
    # Merge stderr to stdout and force unbuffered output
    exec 2>&1
    lrc review --staged --precommit
    REVIEW_EXIT=$?
else
    # Non-interactive (GUI) mode - run quietly
    lrc review --staged --output json >/dev/null 2>&1
    REVIEW_EXIT=$?
fi

# Check exit code
# 0 = review completed, user pressed Enter (ran normally)
# 2 = user pressed Ctrl-S during review (skipped manually)
# other = abort commit
if [ $REVIEW_EXIT -eq 0 ]; then
    # Review ran and user confirmed commit
    echo "ran:$$:$(date +%%s)" > "${STATE_FILE}.tmp"
    mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
    exit 0
elif [ $REVIEW_EXIT -eq 2 ]; then
    # User pressed Ctrl-S to skip review
    echo "skipped_manual:$$:$(date +%%s)" > "${STATE_FILE}.tmp"
    mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
    exit 0
else
    # User aborted or error occurred
    echo "skipped:$$:$(date +%%s)" > "${STATE_FILE}.tmp"
    mv "${STATE_FILE}.tmp" "$STATE_FILE" 2>/dev/null || true
    exit 1
fi
%s`, lrcMarkerBegin, version, lrcMarkerEnd)
}

// generateCommitMsgHook generates the commit-msg hook script
func generateCommitMsgHook() string {
	return fmt.Sprintf(`%s
# lrc_version: %s
# This section is managed by LiveReview CLI (lrc)
# Manual changes within markers will be lost on hook updates
STATE_FILE=".git/livereview_state"
LOCK_DIR=".git/livereview_state.lock"
COMMIT_MSG_FILE="$1"
COMMIT_MSG_OVERRIDE=".git/%s"

# Apply commit-message override from lrc (if present)
if [ -f "$COMMIT_MSG_OVERRIDE" ]; then
	if [ -s "$COMMIT_MSG_OVERRIDE" ]; then
		cat "$COMMIT_MSG_OVERRIDE" > "$COMMIT_MSG_FILE"
	fi
	rm -f "$COMMIT_MSG_OVERRIDE" 2>/dev/null || true
fi

# Read state if exists
if [ -f "$STATE_FILE" ]; then
    STATE=$(cat "$STATE_FILE" 2>/dev/null | cut -d: -f1)
    
    if [ "$STATE" = "ran" ]; then
        echo "" >> "$COMMIT_MSG_FILE"
        echo "LiveReview Pre-Commit Check: ran" >> "$COMMIT_MSG_FILE"
    elif [ "$STATE" = "skipped_manual" ]; then
        echo "" >> "$COMMIT_MSG_FILE"
        echo "LiveReview Pre-Commit Check: skipped manually" >> "$COMMIT_MSG_FILE"
    elif [ "$STATE" = "skipped" ]; then
        echo "" >> "$COMMIT_MSG_FILE"
        echo "LiveReview Pre-Commit Check: skipped" >> "$COMMIT_MSG_FILE"
    fi
    
    # Clean up state file and lock
    rm -f "$STATE_FILE" 2>/dev/null || true
    rmdir "$LOCK_DIR" 2>/dev/null || true
fi

# Always exit 0
exit 0
%s`, lrcMarkerBegin, version, commitMessageFile, lrcMarkerEnd)
}

// cleanOldBackups removes old backup files, keeping only the last N
func cleanOldBackups(backupDir string, keepLast int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Group backups by hook name
	backupsByHook := make(map[string][]os.DirEntry)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Extract hook name (before first dot)
		parts := strings.SplitN(name, ".", 2)
		if len(parts) == 2 {
			hookName := parts[0]
			backupsByHook[hookName] = append(backupsByHook[hookName], entry)
		}
	}

	// For each hook, keep only the last N backups
	for hookName, backups := range backupsByHook {
		if len(backups) <= keepLast {
			continue
		}

		// Sort by name (which includes timestamp)
		// Oldest first
		for i := 0; i < len(backups)-keepLast; i++ {
			oldPath := filepath.Join(backupDir, backups[i].Name())
			if err := os.Remove(oldPath); err != nil {
				log.Printf("Warning: failed to remove old backup %s: %v", oldPath, err)
			} else {
				log.Printf("Removed old backup: %s", backups[i].Name())
			}
		}
		log.Printf("Cleaned up old %s backups (kept last %d)", hookName, keepLast)
	}

	return nil
}
