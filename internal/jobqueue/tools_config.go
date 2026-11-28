package jobqueue

import (
	"os"
	"strconv"
	"time"
)

const (
	// DefaultMaxConcurrentToolsPerReview is the default cap on concurrent SAST
	// tool invocations within a single review run.
	DefaultMaxConcurrentToolsPerReview = 15

	// DefaultMaxConcurrentToolsGlobal is the default process-wide cap across all
	// concurrently executing reviews to protect AWS Lambda and Tier-3 LLM quotas.
	DefaultMaxConcurrentToolsGlobal = 30

	// DefaultToolPhaseTimeout is the deadline covering the entire inline SAST
	// execution phase (semaphore wait, Lambda calls, retries, and Tier-3
	// classification) so post-Wait finalization and PR comment posting keep budget
	// inside ReviewJobTimeout.
	DefaultToolPhaseTimeout = 5 * time.Minute

	// DefaultToolInvokeTimeout is the per-attempt AWS Lambda invocation timeout.
	DefaultToolInvokeTimeout = 90 * time.Second

	// DefaultToolClassifyTimeout bounds Tier-3 LLM taxonomy classification so a
	// slow LLM call degrades to safe fallback rather than holding up completion.
	DefaultToolClassifyTimeout = 30 * time.Second

	// DefaultToolMaxRetries is the number of retries on transient Lambda failures
	// (e.g. HTTP 5xx, 429, or network errors) within a single tool goroutine.
	DefaultToolMaxRetries = 1

	// DefaultToolRetryDelay is the backoff wait before retrying a transient failure.
	DefaultToolRetryDelay = 1 * time.Second

	// DefaultToolPostPacingDelay adds gentle pacing between consecutive PR provider
	// comment API calls to reduce secondary rate-limit bursts.
	DefaultToolPostPacingDelay = 100 * time.Millisecond

	// DefaultToolPostMaxRetries is the retry budget for rate-limited PR comment posts.
	DefaultToolPostMaxRetries = 2

	minToolPostBuffer = 2 * time.Minute
)

// ToolsExecutionConfig holds tunable concurrency, timeout, retry, and PR-posting
// parameters for inline SAST tool execution.
type ToolsExecutionConfig struct {
	MaxConcurrentPerReview int
	MaxConcurrentGlobal    int
	ToolPhaseTimeout       time.Duration
	InvokeTimeout          time.Duration
	ClassifyTimeout        time.Duration
	MaxRetries             int
	RetryDelay             time.Duration
	PostPacingDelay        time.Duration
	PostMaxRetries         int
}

// DefaultToolsExecutionConfig returns the baseline inline SAST execution config.
func DefaultToolsExecutionConfig() ToolsExecutionConfig {
	return ToolsExecutionConfig{
		MaxConcurrentPerReview: DefaultMaxConcurrentToolsPerReview,
		MaxConcurrentGlobal:    DefaultMaxConcurrentToolsGlobal,
		ToolPhaseTimeout:       DefaultToolPhaseTimeout,
		InvokeTimeout:          DefaultToolInvokeTimeout,
		ClassifyTimeout:        DefaultToolClassifyTimeout,
		MaxRetries:             DefaultToolMaxRetries,
		RetryDelay:             DefaultToolRetryDelay,
		PostPacingDelay:        DefaultToolPostPacingDelay,
		PostMaxRetries:         DefaultToolPostMaxRetries,
	}
}

// Normalize clamps all concurrency and timing bounds so semaphores never block on
// zero capacity and ToolPhaseTimeout stays comfortably below reviewJobTimeout.
func (c ToolsExecutionConfig) Normalize(reviewJobTimeout time.Duration) ToolsExecutionConfig {
	if c.MaxConcurrentPerReview <= 0 {
		c.MaxConcurrentPerReview = DefaultMaxConcurrentToolsPerReview
	}

	if c.MaxConcurrentGlobal <= 0 {
		c.MaxConcurrentGlobal = DefaultMaxConcurrentToolsGlobal
	}

	if c.InvokeTimeout <= 0 {
		c.InvokeTimeout = DefaultToolInvokeTimeout
	}
	if c.ClassifyTimeout <= 0 {
		c.ClassifyTimeout = DefaultToolClassifyTimeout
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.RetryDelay <= 0 {
		c.RetryDelay = DefaultToolRetryDelay
	}
	if c.PostPacingDelay < 0 {
		c.PostPacingDelay = 0
	}
	if c.PostMaxRetries < 0 {
		c.PostMaxRetries = 0
	}

	if c.ToolPhaseTimeout <= 0 {
		c.ToolPhaseTimeout = DefaultToolPhaseTimeout
	}
	if reviewJobTimeout > minToolPostBuffer {
		maxAllowedPhase := reviewJobTimeout - minToolPostBuffer
		if c.ToolPhaseTimeout > maxAllowedPhase {
			c.ToolPhaseTimeout = maxAllowedPhase
		}
	} else if reviewJobTimeout > 0 && c.ToolPhaseTimeout >= reviewJobTimeout {
		// Keep phase deadline strictly under the review job timeout when configured tightly in tests.
		c.ToolPhaseTimeout = max(reviewJobTimeout/2, 100*time.Millisecond)
	}

	return c
}

// toolsExecutionConfigFromEnv loads ToolsExecutionConfig from environment variables,
// falling back to DefaultToolsExecutionConfig and normalizing against reviewJobTimeout.
func toolsExecutionConfigFromEnv(reviewJobTimeout time.Duration) ToolsExecutionConfig {
	cfg := DefaultToolsExecutionConfig()

	if v := os.Getenv("LIVEREVIEW_TOOLS_MAX_CONCURRENT_PER_REVIEW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 1 {
				cfg.MaxConcurrentPerReview = 1
			} else {
				cfg.MaxConcurrentPerReview = n
			}
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_MAX_CONCURRENT_GLOBAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 1 {
				cfg.MaxConcurrentGlobal = 1
			} else {
				cfg.MaxConcurrentGlobal = n
			}
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_PHASE_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.ToolPhaseTimeout = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_INVOKE_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.InvokeTimeout = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_CLASSIFY_TIMEOUT_SECONDS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.ClassifyTimeout = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxRetries = n
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_RETRY_DELAY_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			cfg.RetryDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_POST_PACING_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= 0 {
			cfg.PostPacingDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if v := os.Getenv("LIVEREVIEW_TOOLS_POST_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.PostMaxRetries = n
		}
	}

	return cfg.Normalize(reviewJobTimeout)
}
