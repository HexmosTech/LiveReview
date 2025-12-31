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
const appVersion = "v0.1.3" // Semantic version - bump this for releases

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
	pushRequestFile     = "livereview_push_request"
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
	&cli.BoolFlag{
		Name:    "skip",
		Usage:   "mark review as skipped and write attestation without contacting the API",
		EnvVars: []string{"LRC_SKIP"},
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
				Name:  "hooks",
				Usage: "Manage LiveReview Git hook integration (global dispatcher)",
				Subcommands: []*cli.Command{
					{
						Name:  "install",
						Usage: "Install global LiveReview hook dispatchers (uses core.hooksPath)",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "path",
								Usage: "custom hooksPath (defaults to core.hooksPath or ~/.git-hooks)",
							},
							&cli.BoolFlag{
								Name:  "local",
								Usage: "install into the current repo hooks path (respects core.hooksPath)",
							},
						},
						Action: runHooksInstall,
					},
					{
						Name:  "uninstall",
						Usage: "Remove LiveReview hook dispatchers and managed scripts",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "local",
								Usage: "uninstall from the current repo hooks path",
							},
						},
						Action: runHooksUninstall,
					},
					{
						Name:   "enable",
						Usage:  "Enable LiveReview hooks for the current repository",
						Action: runHooksEnable,
					},
					{
						Name:   "disable",
						Usage:  "Disable LiveReview hooks for the current repository",
						Action: runHooksDisable,
					},
					{
						Name:   "status",
						Usage:  "Show LiveReview hook status for the current repository",
						Action: runHooksStatus,
					},
				},
			},
			{
				Name:   "install-hooks",
				Usage:  "Install LiveReview hooks (deprecated; use 'lrc hooks install')",
				Hidden: true,
				Action: runHooksInstall,
			},
			{
				Name:   "uninstall-hooks",
				Usage:  "Uninstall LiveReview hooks (deprecated; use 'lrc hooks uninstall')",
				Hidden: true,
				Action: runHooksUninstall,
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
	skip         bool
	initialMsg   string
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
	// Get initial commit message from file or environment variable
	initialMsg := ""
	if msgFile := os.Getenv("LRC_INITIAL_MESSAGE_FILE"); msgFile != "" {
		if data, err := os.ReadFile(msgFile); err == nil {
			initialMsg = strings.TrimRight(string(data), "\r\n")
		}
	} else {
		initialMsg = strings.TrimRight(os.Getenv("LRC_INITIAL_MESSAGE"), "\r\n")
	}

	opts := reviewOptions{
		repoName:   c.String("repo-name"),
		rangeVal:   c.String("range"),
		diffFile:   c.String("diff-file"),
		apiURL:     c.String("api-url"),
		apiKey:     c.String("api-key"),
		output:     c.String("output"),
		saveHTML:   c.String("save-html"),
		serve:      c.Bool("serve"),
		port:       c.Int("port"),
		verbose:    c.Bool("verbose"),
		precommit:  c.Bool("precommit"),
		skip:       c.Bool("skip"),
		saveJSON:   c.String("save-json"),
		saveText:   c.String("save-text"),
		initialMsg: initialMsg,
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
		diffSource = "staged"
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
	attestationAction := ""
	attestationWritten := false
	initialMsg := sanitizeInitialMessage(opts.initialMsg)

	// Short-circuit skip: write attestation and exit without contacting the API
	if opts.skip {
		attestationAction = "skipped"
		if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
			return err
		}
		if verbose {
			log.Println("Review skipped by --skip; attestation recorded")
		} else {
			fmt.Println("LiveReview: marked review as skipped for current tree")
		}
		return nil
	}

	if opts.precommit {
		gitDir, err := resolveGitDir()
		if err != nil {
			return fmt.Errorf("precommit mode requires a git repository: %w", err)
		}
		commitMsgPath = filepath.Join(gitDir, commitMessageFile)
		_ = clearCommitMessageFile(commitMsgPath)
	}

	// If an attestation already exists for the current tree, skip running another review
	if existing, err := existingAttestationAction(); err == nil && existing != "" {
		if verbose {
			log.Printf("Attestation already present for current tree (action=%s); skipping review", existing)
		} else {
			fmt.Printf("LiveReview: attestation already present for current tree (%s); skipping review\n", existing)
		}
		return nil
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

		var pollFinished bool
		select {
		case decisionCode = <-decisionChan:
			stopCtrlSFn()
		case <-pollDone:
			pollFinished = true
		}

		if pollFinished {
			// Prefer a user decision if it arrives within a short grace window after poll finishes
			select {
			case decisionCode = <-decisionChan:
				// got user decision
			case <-time.After(300 * time.Millisecond):
				// no decision quickly; proceed with poll result
			}
			stopCtrlSFn()
			if pollErr != nil {
				return fmt.Errorf("failed to poll review: %w", pollErr)
			}
			result = pollResult
			attestationAction = "reviewed"
			if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
				return err
			}
		}

		// If a decision happened before we proceed, act now
		if decisionCode != -1 {
			switch decisionCode {
			case 1:
				fmt.Println("\n❌ Review and commit aborted by user")
				fmt.Println()
				return cli.Exit("", decisionCode)
			case 2:
				fmt.Println("\n⏭️  Review skipped, proceeding with commit")
				if err := ensureAttestation("skipped", verbose, &attestationWritten); err != nil {
					return err
				}
				fmt.Println()
				return cli.Exit("", decisionCode)
			case 3:
				fmt.Println("\n⏭️  Skip requested from review page; aborting commit")
				fmt.Println()
				return cli.Exit("", decisionCode)
			}
		}
	} else {
		// Non-precommit: just poll
		var pollErr error
		result, pollErr = pollReview(config.APIURL, config.APIKey, reviewID, opts.pollInterval, opts.timeout, verbose)
		if pollErr != nil {
			return fmt.Errorf("failed to poll review: %w", pollErr)
		}
		attestationAction = "reviewed"
		if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
			return err
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
		if err := saveHTMLOutput(htmlPath, result, verbose, opts.precommit, initialMsg); err != nil {
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
				if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
					return err
				}
				exitCode := serveHTMLPrecommit(htmlPath, opts.port, commitMsgPath, initialMsg)
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

	if err := ensureAttestation(attestationAction, verbose, &attestationWritten); err != nil {
		return err
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

type attestationPayload struct {
	Action string `json:"action"`
}

func ensureAttestation(action string, verbose bool, written *bool) error {
	if written != nil && *written {
		return nil
	}
	if strings.TrimSpace(action) == "" {
		return nil
	}

	path, err := writeAttestationForCurrentTree(action)
	if err != nil {
		return fmt.Errorf("failed to write attestation: %w", err)
	}
	if verbose {
		log.Printf("Attestation written: %s (action=%s)", path, action)
	}
	if written != nil {
		*written = true
	}
	return nil
}

// existingAttestationAction returns the attestation action for the current tree, if present.
func existingAttestationAction() (string, error) {
	treeHash, err := currentTreeHash()
	if err != nil {
		return "", err
	}
	if treeHash == "" {
		return "", nil
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return "", err
	}

	attestPath := filepath.Join(gitDir, "lrc", "attestations", fmt.Sprintf("%s.json", treeHash))
	data, err := os.ReadFile(attestPath)
	if err != nil {
		return "", nil // not present
	}

	var payload attestationPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", nil
	}

	return strings.TrimSpace(payload.Action), nil
}

func writeAttestationForCurrentTree(action string) (string, error) {
	if strings.TrimSpace(action) == "" {
		return "", fmt.Errorf("attestation action cannot be empty")
	}

	treeHash, err := currentTreeHash()
	if err != nil {
		return "", fmt.Errorf("failed to compute tree hash: %w", err)
	}
	if treeHash == "" {
		return "", fmt.Errorf("empty tree hash")
	}

	gitDir, err := resolveGitDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve git dir: %w", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir, err = filepath.Abs(gitDir)
		if err != nil {
			return "", fmt.Errorf("failed to absolutize git dir: %w", err)
		}
	}

	attestDir := filepath.Join(gitDir, "lrc", "attestations")
	if err := os.MkdirAll(attestDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create attestation directory: %w", err)
	}

	data, err := json.Marshal(attestationPayload{Action: action})
	if err != nil {
		return "", fmt.Errorf("failed to marshal attestation: %w", err)
	}

	tmpFile, err := os.CreateTemp(attestDir, fmt.Sprintf("%s.*.json", treeHash))
	if err != nil {
		return "", fmt.Errorf("failed to create temp attestation file: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write attestation: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize attestation: %w", err)
	}

	target := filepath.Join(attestDir, fmt.Sprintf("%s.json", treeHash))
	if err := os.Rename(tmpFile.Name(), target); err != nil {
		return "", fmt.Errorf("failed to move attestation into place: %w", err)
	}

	return target, nil
}

func currentTreeHash() (string, error) {
	out, err := runGitCommand("git", "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

func saveHTMLOutput(path string, result *diffReviewResponse, verbose bool, precommit bool, initialMsg string) error {
	// Prepare template data
	data := prepareHTMLData(result, precommit, initialMsg)

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

// sanitizeInitialMessage strips trailers and whitespace from a prefilled commit message
// and drops the message entirely if only trailers remain.
func sanitizeInitialMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	filtered := make([]string, 0, len(lines))

	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		if strings.HasPrefix(clean, "LiveReview Pre-Commit Check:") {
			continue
		}
		if strings.HasPrefix(clean, "#") {
			// Drop git template comment lines for prefill cleanliness
			continue
		}
		filtered = append(filtered, line)
	}

	result := strings.TrimSpace(strings.Join(filtered, "\n"))
	return result
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

// persistPushRequest creates a marker file to request a post-commit push.
func persistPushRequest(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	pushPath := filepath.Join(filepath.Dir(commitMsgPath), pushRequestFile)
	return os.WriteFile(pushPath, []byte("push"), 0600)
}

// clearPushRequest removes any pending push request marker.
func clearPushRequest(commitMsgPath string) error {
	if commitMsgPath == "" {
		return nil
	}

	pushPath := filepath.Join(filepath.Dir(commitMsgPath), pushRequestFile)
	if err := os.Remove(pushPath); err != nil && !os.IsNotExist(err) {
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
func serveHTMLPrecommit(htmlPath string, port int, commitMsgPath string, initialMsg string) int {
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
		push    bool
	}

	decisionChan := make(chan precommitDecision, 1) // 0=commit,2=skip-from-terminal,1=abort,3=skip-from-HTML-abort, push flag handled separately
	var decideOnce sync.Once
	decide := func(code int, message string, push bool) {
		decideOnce.Do(func() {
			decisionChan <- precommitDecision{code: code, message: message, push: push}
		})
	}

	// Pre-commit action endpoints (HTML buttons call these)
	mux.HandleFunc("/commit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		decide(0, msg, false)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/commit-push", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := readCommitMessageFromRequest(r)
		decide(0, msg, true)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/skip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		decide(3, "", false)
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
		decide(1, "", false)
	}()

	// Read from /dev/tty directly to avoid stdin issues in git hooks (Enter fallback, cooked mode)
	go func() {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			fmt.Println("Warning: Could not open terminal, auto-proceeding")
			time.Sleep(2 * time.Second)
			decide(0, initialMsg, false)
			return
		}
		defer tty.Close()

		reader := bufio.NewReader(tty)

		fmt.Printf("📋 Review complete. Choose action:\n")
		fmt.Printf("   [Enter]  Continue with commit\n")
		fmt.Printf("   [Ctrl-C] Abort commit\n")
		fmt.Printf("\nOptional: type a new commit message and press Enter to use it (leave blank to keep Git's message).\n")
		if strings.TrimSpace(initialMsg) != "" {
			fmt.Printf("(current message): %s\n", initialMsg)
		}
		fmt.Printf("> ")
		os.Stdout.Sync()

		typedMessage, _ := reader.ReadString('\n')
		typedMessage = strings.TrimRight(strings.TrimRight(typedMessage, "\n"), "\r")
		if strings.TrimSpace(typedMessage) == "" {
			typedMessage = initialMsg
		}

		fmt.Printf("\n[Enter] Continue with commit\n")
		fmt.Printf("[Ctrl-C] Abort commit\n")
		fmt.Printf("\nYour choice: ")
		os.Stdout.Sync()

		_, err = reader.ReadString('\n')
		if err != nil {
			decide(0, typedMessage, false)
			return
		}
		decide(0, typedMessage, false)
	}()

	// Wait for any decision source
	decision := <-decisionChan

	if commitMsgPath != "" {
		if decision.code == 0 {
			msgToPersist := decision.message
			if strings.TrimSpace(msgToPersist) == "" {
				msgToPersist = initialMsg
			}

			if strings.TrimSpace(msgToPersist) != "" {
				if err := persistCommitMessage(commitMsgPath, msgToPersist); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to store commit message: %v\n", err)
				}
			} else {
				_ = clearCommitMessageFile(commitMsgPath)
			}
		} else {
			_ = clearCommitMessageFile(commitMsgPath)
		}
	}

	if decision.code == 0 && decision.push {
		if err := persistPushRequest(commitMsgPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store push request: %v\n", err)
		}
	} else {
		_ = clearPushRequest(commitMsgPath)
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
	lrcMarkerBegin        = "# BEGIN lrc managed section - DO NOT EDIT"
	lrcMarkerEnd          = "# END lrc managed section"
	defaultGlobalHooksDir = ".git-hooks"
	hooksMetaFilename     = ".lrc-hooks-meta.json"
)

var managedHooks = []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"}

type hooksMeta struct {
	Path     string `json:"path"`
	PrevPath string `json:"prev_path,omitempty"`
	SetByLRC bool   `json:"set_by_lrc"`
}

func defaultGlobalHooksPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, defaultGlobalHooksDir), nil
}

func currentHooksPath() (string, error) {
	cmd := exec.Command("git", "config", "--global", "--get", "core.hooksPath")
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func currentLocalHooksPath(repoRoot string) (string, error) {
	cmd := exec.Command("git", "config", "--local", "--get", "core.hooksPath")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(out)), nil
}

func resolveRepoHooksPath(repoRoot string) (string, error) {
	localPath, _ := currentLocalHooksPath(repoRoot)
	if localPath == "" {
		return filepath.Join(repoRoot, ".git", "hooks"), nil
	}
	if filepath.IsAbs(localPath) {
		return localPath, nil
	}
	return filepath.Join(repoRoot, localPath), nil
}

func setGlobalHooksPath(path string) error {
	cmd := exec.Command("git", "config", "--global", "core.hooksPath", path)
	return cmd.Run()
}

func unsetGlobalHooksPath() error {
	cmd := exec.Command("git", "config", "--global", "--unset", "core.hooksPath")
	return cmd.Run()
}

func hooksMetaPath(hooksPath string) string {
	return filepath.Join(hooksPath, hooksMetaFilename)
}

func writeHooksMeta(hooksPath string, meta hooksMeta) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}

	_ = os.MkdirAll(hooksPath, 0755)
	_ = os.WriteFile(hooksMetaPath(hooksPath), data, 0644)
}

func readHooksMeta(hooksPath string) (*hooksMeta, error) {
	data, err := os.ReadFile(hooksMetaPath(hooksPath))
	if err != nil {
		return nil, err
	}

	var meta hooksMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func removeHooksMeta(hooksPath string) error {
	return os.Remove(hooksMetaPath(hooksPath))
}

func writeManagedHookScripts(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	scripts := map[string]string{
		"pre-commit":         generatePreCommitHook(),
		"prepare-commit-msg": generatePrepareCommitMsgHook(),
		"commit-msg":         generateCommitMsgHook(),
		"post-commit":        generatePostCommitHook(),
	}

	for name, content := range scripts {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\n" + content
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			return fmt.Errorf("failed to write managed hook %s: %w", name, err)
		}
	}

	return nil
}

// runHooksInstall installs dispatchers and managed hook scripts under either global core.hooksPath or the current repo hooks path when --local is used
func runHooksInstall(c *cli.Context) error {
	localInstall := c.Bool("local")
	requestedPath := strings.TrimSpace(c.String("path"))
	var hooksPath string
	setConfig := false

	if localInstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}

		gitDir, err := resolveGitDir()
		if err != nil {
			return err
		}
		repoRoot := filepath.Dir(gitDir)
		hooksPath, err = resolveRepoHooksPath(repoRoot)
		if err != nil {
			return err
		}
	} else {
		currentPath, _ := currentHooksPath()
		defaultPath, err := defaultGlobalHooksPath()
		if err != nil {
			return fmt.Errorf("failed to determine default hooks path: %w", err)
		}

		hooksPath = requestedPath
		if hooksPath == "" {
			if currentPath != "" {
				hooksPath = currentPath
			} else {
				hooksPath = defaultPath
			}
		}

		if currentPath == "" {
			setConfig = true
		} else if requestedPath != "" && requestedPath != currentPath {
			setConfig = true
		}
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	if !localInstall && setConfig {
		if err := setGlobalHooksPath(absHooksPath); err != nil {
			return fmt.Errorf("failed to set core.hooksPath: %w", err)
		}
	}

	if err := os.MkdirAll(absHooksPath, 0755); err != nil {
		return fmt.Errorf("failed to create hooks path %s: %w", absHooksPath, err)
	}

	managedDir := filepath.Join(absHooksPath, "lrc")
	backupDir := filepath.Join(absHooksPath, ".lrc_backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	if err := writeManagedHookScripts(managedDir); err != nil {
		return err
	}

	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		dispatcher := generateDispatcherHook(hookName)
		if err := installHook(hookPath, dispatcher, hookName, backupDir, true); err != nil {
			return fmt.Errorf("failed to install dispatcher for %s: %w", hookName, err)
		}
	}

	if !localInstall {
		writeHooksMeta(absHooksPath, hooksMeta{Path: absHooksPath, PrevPath: hooksPath, SetByLRC: setConfig})
	}
	_ = cleanOldBackups(backupDir, 5)

	if localInstall {
		fmt.Printf("✅ LiveReview hooks installed in repo path: %s\n", absHooksPath)
	} else {
		fmt.Printf("✅ LiveReview global hooks installed at %s\n", absHooksPath)
	}
	fmt.Println("Dispatchers will chain repo-local hooks when present.")
	fmt.Println("Use 'lrc hooks disable' in a repo to bypass LiveReview hooks there.")

	return nil
}

// runHooksUninstall removes lrc-managed sections from dispatchers and managed scripts (global or local)
func runHooksUninstall(c *cli.Context) error {
	localUninstall := c.Bool("local")
	var hooksPath string

	if localUninstall {
		if !isGitRepository() {
			return fmt.Errorf("not in a git repository (no .git directory found)")
		}
		gitDir, err := resolveGitDir()
		if err != nil {
			return err
		}
		repoRoot := filepath.Dir(gitDir)
		hooksPath, err = resolveRepoHooksPath(repoRoot)
		if err != nil {
			return err
		}
	} else {
		hooksPath, _ = currentHooksPath()
		if hooksPath == "" {
			var err error
			hooksPath, err = defaultGlobalHooksPath()
			if err != nil {
				return fmt.Errorf("failed to determine hooks path: %w", err)
			}
		}
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	var meta *hooksMeta
	if !localUninstall {
		meta, _ = readHooksMeta(absHooksPath)
	}
	removed := 0
	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		if err := uninstallHook(hookPath, hookName); err != nil {
			fmt.Printf("⚠️  Warning: failed to uninstall %s: %v\n", hookName, err)
		} else {
			removed++
		}
	}

	_ = os.RemoveAll(filepath.Join(absHooksPath, "lrc"))
	_ = cleanOldBackups(filepath.Join(absHooksPath, ".lrc_backups"), 5)
	if !localUninstall {
		_ = removeHooksMeta(absHooksPath)
	}

	if !localUninstall && meta != nil && meta.SetByLRC && meta.Path == absHooksPath {
		if meta.PrevPath == "" {
			_ = unsetGlobalHooksPath()
		} else {
			_ = setGlobalHooksPath(meta.PrevPath)
		}
	}

	if removed > 0 {
		fmt.Printf("✅ Removed LiveReview sections from %d hook(s) at %s\n", removed, absHooksPath)
	} else {
		fmt.Printf("ℹ️  No LiveReview sections found in %s\n", absHooksPath)
	}

	return nil
}

func runHooksDisable(c *cli.Context) error {
	gitDir, err := resolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	lrcDir := filepath.Join(gitDir, "lrc")
	if err := os.MkdirAll(lrcDir, 0755); err != nil {
		return fmt.Errorf("failed to create lrc directory: %w", err)
	}

	marker := filepath.Join(lrcDir, "disabled")
	if err := os.WriteFile(marker, []byte("disabled\n"), 0644); err != nil {
		return fmt.Errorf("failed to write disable marker: %w", err)
	}

	fmt.Println("🔕 LiveReview hooks disabled for this repository")
	return nil
}

func runHooksEnable(c *cli.Context) error {
	gitDir, err := resolveGitDir()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	marker := filepath.Join(gitDir, "lrc", "disabled")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove disable marker: %w", err)
	}

	fmt.Println("🔔 LiveReview hooks enabled for this repository")
	return nil
}

func hookHasManagedSection(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), lrcMarkerBegin)
}

func runHooksStatus(c *cli.Context) error {
	hooksPath, _ := currentHooksPath()
	defaultPath, _ := defaultGlobalHooksPath()
	if hooksPath == "" {
		hooksPath = defaultPath
	}

	absHooksPath, err := filepath.Abs(hooksPath)
	if err != nil {
		return fmt.Errorf("failed to resolve hooks path: %w", err)
	}

	gitDir, gitErr := resolveGitDir()
	repoDisabled := false
	if gitErr == nil {
		repoDisabled = fileExists(filepath.Join(gitDir, "lrc", "disabled"))
	}

	fmt.Printf("hooksPath: %s\n", absHooksPath)
	if cfg, _ := currentHooksPath(); cfg != "" {
		fmt.Printf("core.hooksPath: %s\n", cfg)
	} else {
		fmt.Println("core.hooksPath: not set (using repo default unless dispatcher present)")
	}

	if gitErr == nil {
		fmt.Printf("repo: %s\n", filepath.Dir(gitDir))
		if repoDisabled {
			fmt.Println("status: disabled via .git/lrc/disabled")
		} else {
			fmt.Println("status: enabled")
		}
	} else {
		fmt.Println("repo: not detected")
	}

	for _, hookName := range managedHooks {
		hookPath := filepath.Join(absHooksPath, hookName)
		fmt.Printf("%s: ", hookName)
		if hookHasManagedSection(hookPath) {
			fmt.Println("LiveReview dispatcher present")
		} else if fileExists(hookPath) {
			fmt.Println("custom hook (no LiveReview block)")
		} else {
			fmt.Println("missing")
		}
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
			fmt.Printf("ℹ️  %s already has lrc section (use --force=false to skip updating)\n", hookName)
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
	return renderHookTemplate("hooks/pre-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
	})
}

// generatePrepareCommitMsgHook generates the prepare-commit-msg hook script
func generatePrepareCommitMsgHook() string {
	return renderHookTemplate("hooks/prepare-commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
	})
}

// generateCommitMsgHook generates the commit-msg hook script
func generateCommitMsgHook() string {
	return renderHookTemplate("hooks/commit-msg.sh", map[string]string{
		hookMarkerBeginPlaceholder:       lrcMarkerBegin,
		hookMarkerEndPlaceholder:         lrcMarkerEnd,
		hookVersionPlaceholder:           version,
		hookCommitMessageFilePlaceholder: commitMessageFile,
	})
}

// generatePostCommitHook runs a safe pull (ff-only) and push when requested.
func generatePostCommitHook() string {
	return renderHookTemplate("hooks/post-commit.sh", map[string]string{
		hookMarkerBeginPlaceholder:     lrcMarkerBegin,
		hookMarkerEndPlaceholder:       lrcMarkerEnd,
		hookVersionPlaceholder:         version,
		hookPushRequestFilePlaceholder: pushRequestFile,
	})
}

func generateDispatcherHook(hookName string) string {
	return renderHookTemplate("hooks/dispatcher.sh", map[string]string{
		hookMarkerBeginPlaceholder: lrcMarkerBegin,
		hookMarkerEndPlaceholder:   lrcMarkerEnd,
		hookVersionPlaceholder:     version,
		hookNamePlaceholder:        hookName,
	})
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
