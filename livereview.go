package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/livereview/cmd"
)

//go:embed ui/dist/*
var uiAssets embed.FS

// Version information (set by build-time ldflags)
var (
	version   = "development" // Set by -ldflags during build
	buildTime = "unknown"     // Set by -ldflags during build
	gitCommit = "unknown"     // Set by -ldflags during build
)

func main() {
	// Set version information in cmd package
	cmd.Version = version
	cmd.BuildTime = buildTime
	cmd.GitCommit = gitCommit

	// Try to load .env file if it exists (ignore error)
	_ = cmd.LoadEnvFile(".env")

	app := &cli.App{
		Name:    "livereview",
		Usage:   "AI-powered code review tool for GitLab, GitHub, and BitBucket",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Load configuration from `FILE`",
				Value:   "lrdata/livereview.toml",
			},
		},
		Commands: []*cli.Command{
			cmd.ReviewCommand(),
			cmd.ConfigCommand(),
			cmd.APICommand(),
			cmd.UICommand(uiAssets),
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
