package cmd

import (
	"fmt"

	"github.com/livereview/internal/api"
	"github.com/urfave/cli/v2"
)

// APICommand returns the CLI command for starting the API server
func APICommand() *cli.Command {
	return &cli.Command{
		Name:  "api",
		Usage: "Start the LiveReview API server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Port for the API server",
				Value:   8888,
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")
			fmt.Printf("Starting LiveReview API server on port %d...\n", port)

			server := api.NewServer(port)
			return server.Start()
		},
	}
}
