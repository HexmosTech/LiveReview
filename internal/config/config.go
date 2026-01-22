package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config represents the application configuration
type Config struct {
	General struct {
		DefaultProvider string `koanf:"default_provider"`
		DefaultAI       string `koanf:"default_ai"`
	} `koanf:"general"`

	Providers map[string]map[string]interface{} `koanf:"providers"`
	AI        map[string]map[string]interface{} `koanf:"ai"`
	Batch     map[string]interface{}            `koanf:"batch"`
}

// LoadConfig loads the configuration from a file
func LoadConfig(configPath string) (*Config, error) {
	var k = koanf.New(".")

	// Set up default configuration
	k.Load(confmap.Provider(map[string]interface{}{
		"general.default_provider": "gitlab",
		"general.default_ai":       "gemini",
	}, "."), nil)

	// Load from TOML file if it exists
	if configPath != "" {
		if err := k.Load(file.Provider(configPath), toml.Parser()); err != nil {
			return nil, fmt.Errorf("error loading config: %w", err)
		}
	} else {
		// Try to load from default locations - prioritize lrdata directory for containerized environments
		defaultPaths := []string{"./lrdata/livereview.toml", "./livereview.toml", "$HOME/.livereview.toml"}
		for _, path := range defaultPaths {
			path = os.ExpandEnv(path)
			if _, err := os.Stat(path); err == nil {
				if err := k.Load(file.Provider(path), toml.Parser()); err == nil {
					break
				}
			}
		}
	}

	// Load from environment variables with prefix LIVEREVIEW_
	k.Load(env.Provider("LIVEREVIEW_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(s), "_", ".", -1)
	}), nil)

	// Unmarshal into Config struct
	var config Config
	if err := k.Unmarshal("", &config); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %w", err)
	}

	return &config, nil
}

// InitConfig initializes a new configuration file
func InitConfig(configPath string) error {
	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("configuration file already exists at %s", configPath)
	}

	// Create sample configuration
	sampleConfig := `# LiveReview Configuration

[general]
default_provider = "gitlab"
default_ai = "gemini"

[providers.gitlab]
url = "https://gitlab.example.com"
token = "your-gitlab-token"

[ai.gemini]
api_key = "your-gemini-api-key"
model = "gemini-2.5-flash"
temperature = 0.2
`

	return os.WriteFile(configPath, []byte(sampleConfig), 0644)
}

// Validate validates the configuration
func Validate(config *Config) error {
	if config.General.DefaultProvider == "" {
		return fmt.Errorf("default provider is required")
	}

	if config.General.DefaultAI == "" {
		return fmt.Errorf("default AI provider is required")
	}

	providerConfig, ok := config.Providers[config.General.DefaultProvider]
	if !ok {
		return fmt.Errorf("configuration for provider %s not found", config.General.DefaultProvider)
	}

	// Validate provider config
	switch config.General.DefaultProvider {
	case "gitlab":
		if _, ok := providerConfig["url"]; !ok {
			return fmt.Errorf("gitlab url is required")
		}
		if _, ok := providerConfig["token"]; !ok {
			return fmt.Errorf("gitlab token is required")
		}
	}

	// Validate AI config
	aiConfig, ok := config.AI[config.General.DefaultAI]
	if !ok {
		return fmt.Errorf("configuration for AI provider %s not found", config.General.DefaultAI)
	}

	switch config.General.DefaultAI {
	case "gemini":
		if _, ok := aiConfig["api_key"]; !ok {
			return fmt.Errorf("gemini api_key is required")
		}
	}

	return nil
}
