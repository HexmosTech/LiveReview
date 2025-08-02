package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"
)

// UICommand returns the CLI command for starting the UI server
func UICommand(uiAssets embed.FS) *cli.Command {
	return &cli.Command{
		Name:  "ui",
		Usage: "Start the LiveReview UI server",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "Port for the UI server",
				Value:   8081,
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")

			// Get the embedded filesystem for the ui/dist directory
			distFS, err := fs.Sub(uiAssets, "ui/dist")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error accessing embedded UI assets: %v\n", err)
				return err
			}

			// Create file server for static assets
			fileServer := http.FileServer(http.FS(distFS))

			// Handle all routes - serve index.html for SPA routing
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Try to serve the requested file
				if r.URL.Path != "/" {
					// Check if file exists in embedded filesystem
					if _, err := fs.Stat(distFS, r.URL.Path[1:]); err == nil {
						fileServer.ServeHTTP(w, r)
						return
					}
				}

				// If file doesn't exist or root path, serve index.html for SPA routing
				indexFile, err := distFS.Open("index.html")
				if err != nil {
					http.Error(w, "Could not open index.html", http.StatusInternalServerError)
					return
				}
				defer indexFile.Close()

				indexContent, err := fs.ReadFile(distFS, "index.html")
				if err != nil {
					http.Error(w, "Could not read index.html", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "text/html")
				w.Write(indexContent)
			})

			fmt.Printf("Starting LiveReview UI server on port %d...\n", port)
			fmt.Printf("Open your browser to: http://localhost:%d\n", port)

			return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		},
	}
}
