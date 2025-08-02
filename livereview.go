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

const (
	version = "0.1.0"
)

func main() {
	app := &cli.App{
		Name:    "livereview",
		Usage:   "AI-powered code review tool for GitLab, GitHub, and BitBucket",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Load configuration from `FILE`",
				Value:   "livereview.toml",
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
