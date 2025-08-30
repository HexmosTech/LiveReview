package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/livereview/internal/config"
)

// ConfigCommand returns the config command
func ConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Manage configuration",
		Subcommands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initialize a new configuration file",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output file path",
						Value:   "livereview.toml",
					},
				},
				Action: runConfigInit,
			},
			{
				Name:   "validate",
				Usage:  "Validate the configuration file",
				Action: runConfigValidate,
			},
		},
	}
}

func runConfigInit(c *cli.Context) error {
	outputPath := c.String("output")

	if err := config.InitConfig(outputPath); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	fmt.Printf("Created configuration file at %s\n", outputPath)
	return nil
}

func runConfigValidate(c *cli.Context) error {
	configPath := c.String("config")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	fmt.Println("Configuration is valid")
	return nil
}
