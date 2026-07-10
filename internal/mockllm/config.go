//go:build !production

package mockllm

import (
	"os"
	"time"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Configurable Mock LLM Settings
var (
	// MockAIMinCommentCount defines the minimum number of comments to generate.
	MockAIMinCommentCount = 10

	// MockAIMaxCommentCount defines the maximum number of comments to generate.
	MockAIMaxCommentCount = 30

	// MockAIMinDelay defines the minimum simulated processing latency for a batch (e.g. 5s).
	MockAIMinDelay = 5 * time.Second

	// MockAIMaxDelay defines the maximum simulated processing latency for a batch (e.g. 60s).
	MockAIMaxDelay = 60 * time.Second

	// MockAIFailureRate defines the probability (0.0 to 1.0) of a batch failing with a simulated 503/429 error.
	MockAIFailureRate = 0.50

	// MockAIMaxTokensPerBatch simulates a context window token limit per batch (default: 8500).
	MockAIMaxTokensPerBatch = 8500
)

type MockConfig struct {
	Comments struct {
		MinCount int `koanf:"min_count"`
		MaxCount int `koanf:"max_count"`
	} `koanf:"comments"`
	Delay struct {
		Min string `koanf:"min"`
		Max string `koanf:"max"`
	} `koanf:"delay"`
	Failure struct {
		Rate float64 `koanf:"rate"`
	} `koanf:"failure"`
	Batch struct {
		MaxTokensPerBatch int `koanf:"max_tokens_per_batch"`
	} `koanf:"batch"`
}

func init() {
	loadConfig()
}

func loadConfig() {
	configPath := "internal/mockllm/mockllm.toml"
	if _, err := os.Stat(configPath); err != nil {
		// File does not exist, use defaults
		return
	}

	var k = koanf.New(".")
	if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
		// Log error and use defaults
		return
	}

	var cfg MockConfig
	if err := k.Unmarshal("", &cfg); err != nil {
		return
	}

	if cfg.Comments.MinCount > 0 {
		MockAIMinCommentCount = cfg.Comments.MinCount
	}
	if cfg.Comments.MaxCount > 0 {
		MockAIMaxCommentCount = cfg.Comments.MaxCount
	}
	if cfg.Delay.Min != "" {
		if d, err := time.ParseDuration(cfg.Delay.Min); err == nil {
			MockAIMinDelay = d
		}
	}
	if cfg.Delay.Max != "" {
		if d, err := time.ParseDuration(cfg.Delay.Max); err == nil {
			MockAIMaxDelay = d
		}
	}
	if cfg.Failure.Rate >= 0.0 && cfg.Failure.Rate <= 1.0 {
		MockAIFailureRate = cfg.Failure.Rate
	}
	if cfg.Batch.MaxTokensPerBatch > 0 {
		MockAIMaxTokensPerBatch = cfg.Batch.MaxTokensPerBatch
	}
}

// Vocabulary for pseudo-random comment generation
var technicalTerms = []string{
	"nil pointer dereference", "concurrency bottleneck", "race condition", "resource leak",
	"performance degradation", "sql injection vulnerability", "hardcoded credentials", "deadlock potential",
	"memory allocation", "unhandled error", "infinite loop", "redundant database query",
}

var reviewtemplates = []string{
	"🤖 [MOCK LLM] Potential %s detected here. Consider reviewing this execution path to ensure safety.",
	"🤖 [MOCK LLM] Optimization opportunity: this block might cause a %s under high concurrency loads.",
	"🤖 [MOCK LLM] Refactoring advised. The current structure could lead to a %s in production.",
	"🤖 [MOCK LLM] Please add validation or a unit test here to guard against a %s.",
	"🤖 [MOCK LLM] Safe design: make sure this segment is protected against %s scenarios.",
}

// IsMockAIEnabled returns true if mock AI mode is enabled via environment variable
func IsMockAIEnabled() bool {
	return os.Getenv("LIVEREVIEW_MOCK_AI") == "true"
}
