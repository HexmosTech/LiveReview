package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/urfave/cli/v2"
)

// Cache for the modified index.html content
var (
	cachedIndexHTML string
	cacheOnce       sync.Once
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
			&cli.StringFlag{
				Name:    "api-url",
				Aliases: []string{"a"},
				Usage:   "API URL for the frontend to connect to (e.g., http://localhost:8888)",
				Value:   "",
			},
		},
		Action: func(c *cli.Context) error {
			port := c.Int("port")
			apiURL := c.String("api-url")

			// Check for environment variable overrides
			if envPort := os.Getenv("LIVEREVIEW_FRONTEND_PORT"); envPort != "" {
				if parsedPort, err := strconv.Atoi(envPort); err == nil {
					port = parsedPort
					fmt.Printf("Using frontend port from LIVEREVIEW_FRONTEND_PORT: %d\n", port)
				}
			}

			// If no API URL provided, try to auto-detect based on port
			if apiURL == "" {
				// Check for backend port environment variable
				backendPort := 8888
				if envBackendPort := os.Getenv("LIVEREVIEW_BACKEND_PORT"); envBackendPort != "" {
					if parsedPort, err := strconv.Atoi(envBackendPort); err == nil {
						backendPort = parsedPort
					}
				}
				apiURL = fmt.Sprintf("http://localhost:%d", backendPort)
			}

			fmt.Printf("Starting LiveReview UI server on port %d...\n", port)
			fmt.Printf("API URL configured as: %s\n", apiURL)
			fmt.Printf("Open your browser to: http://localhost:%d\n", port)

			// Get the embedded filesystem for the ui/dist directory
			distFS, err := fs.Sub(uiAssets, "ui/dist")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error accessing embedded UI assets: %v\n", err)
				return err
			}

			// Prepare the modified index.html with injected API URL
			cacheOnce.Do(func() {
				cachedIndexHTML = prepareIndexHTML(distFS, apiURL)
			})

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

				// If file doesn't exist or root path, serve modified index.html for SPA routing
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(cachedIndexHTML))
			})

			return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
		},
	}
}

// prepareIndexHTML reads the embedded index.html and injects the API URL configuration
func prepareIndexHTML(distFS fs.FS, apiURL string) string {
	indexContent, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not read index.html: %v\n", err)
		return ""
	}

	htmlStr := string(indexContent)

	// Create the configuration script to inject
	configScript := fmt.Sprintf(`<script>
		// LiveReview runtime configuration
		window.LIVEREVIEW_CONFIG = {
			apiUrl: "%s"
		};
	</script>`, apiURL)

	// Insert the config script BEFORE any other scripts to ensure it loads first
	// Look for the first <script tag (case insensitive) and insert before it
	htmlLower := strings.ToLower(htmlStr)
	scriptIndex := strings.Index(htmlLower, "<script")
	if scriptIndex != -1 {
		htmlStr = htmlStr[:scriptIndex] + configScript + "\n" + htmlStr[scriptIndex:]
	} else if strings.Contains(htmlStr, "</head>") {
		htmlStr = strings.Replace(htmlStr, "</head>", configScript+"\n</head>", 1)
	} else if strings.Contains(htmlStr, "</body>") {
		htmlStr = strings.Replace(htmlStr, "</body>", configScript+"\n</body>", 1)
	} else {
		// Fallback: append to the end
		htmlStr = htmlStr + configScript
	}

	return htmlStr
}
