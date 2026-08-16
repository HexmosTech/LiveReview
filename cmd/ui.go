package cmd

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v2"
)

// Cache for the modified index.html content
var (
	cachedIndexHTML string
	cacheOnce       sync.Once
)

// compressedAsset holds a build-time-computed gzip payload for one static file, so
// compression never has to happen on the request hot path (see docs/perf-improvement.md,
// "Finding A" - nginx doing on-the-fly gzip per request was serializing entire request
// bursts, static files and tiny API JSON responses alike, behind its CPU cost).
type compressedAsset struct {
	data        []byte
	contentType string
}

// compressibleExt lists extensions worth pre-gzipping. Already-compressed formats
// (png/jpg/woff2/...) are skipped - gzip wouldn't shrink them and would just add CPU cost.
var compressibleExt = map[string]bool{
	".js":   true,
	".css":  true,
	".svg":  true,
	".json": true,
}

// hashedAssetPattern matches webpack's content-hashed output filenames (main.<hash>.js,
// vendor-framework.<hash>.js, numbered chunk files like 2559.<hash>.js, etc.) - these are
// safe to cache forever since any content change produces a new filename. Everything else
// (unhashed /public assets, index.html) must NOT get long-lived caching since their content
// can change without the filename changing.
var hashedAssetPattern = regexp.MustCompile(`\.[0-9a-f]{8,20}\.(js|css)$`)

// buildCompressedAssets walks the embedded dist filesystem once at startup and gzip-compresses
// every compressible top-level file (skipping index.html, which is regenerated per-request with
// injected runtime config, and the public/ subtree, which is low-volume and not part of the
// measured bottleneck). Doing this once at boot instead of per-request is the actual fix -
// see docs/perf-improvement.md.
func buildCompressedAssets(distFS fs.FS) map[string]compressedAsset {
	assets := make(map[string]compressedAsset)
	_ = fs.WalkDir(distFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if path == "index.html" || strings.HasPrefix(path, "public/") {
			return nil
		}
		ext := strings.ToLower(fileExt(path))
		if !compressibleExt[ext] {
			return nil
		}
		raw, readErr := fs.ReadFile(distFS, path)
		if readErr != nil {
			return nil
		}
		var buf bytes.Buffer
		gz, gzErr := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if gzErr != nil {
			return nil
		}
		if _, writeErr := gz.Write(raw); writeErr != nil {
			gz.Close()
			return nil
		}
		if closeErr := gz.Close(); closeErr != nil {
			return nil
		}
		contentType := mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		assets["/"+path] = compressedAsset{data: buf.Bytes(), contentType: contentType}
		return nil
	})
	return assets
}

// fileExt returns the lowercase extension (including the leading dot) of path.
func fileExt(path string) string {
	if idx := strings.LastIndexByte(path, '.'); idx != -1 {
		return strings.ToLower(path[idx:])
	}
	return ""
}

// setCacheHeaders applies immutable long-lived caching to content-hashed assets and a safe
// no-cache default to everything else (see hashedAssetPattern).
func setCacheHeaders(w http.ResponseWriter, urlPath string) {
	if hashedAssetPattern.MatchString(urlPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// serveCompressedAsset writes a pre-compressed asset directly, skipping per-request gzip work.
// Returns false (does nothing) if the client doesn't advertise gzip support, so the caller can
// fall back to the uncompressed file-server path.
func serveCompressedAsset(w http.ResponseWriter, r *http.Request, urlPath string, asset compressedAsset) bool {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}
	setCacheHeaders(w, urlPath)
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Length", strconv.Itoa(len(asset.data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(asset.data)
	}
	return true
}

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
			} else if envPort := os.Getenv("FRONTEND_PORT"); envPort != "" {
				// Legacy support for existing deployments
				if parsedPort, err := strconv.Atoi(envPort); err == nil {
					port = parsedPort
					fmt.Printf("Using frontend port from FRONTEND_PORT (legacy): %d\n", port)
				}
			}

			// If no API URL provided, try to auto-detect based on deployment mode
			if apiURL == "" {
				// Check for unified API_URL first (trim whitespace to handle malformed env vars)
				if unifiedAPI := strings.TrimSpace(os.Getenv("API_URL")); unifiedAPI != "" {
					apiURL = unifiedAPI
				} else {
					// Auto-detect based on reverse proxy setting
					isReverseProxy := os.Getenv("LIVEREVIEW_REVERSE_PROXY") == "true"

					if isReverseProxy {
						// In production mode with reverse proxy, API URL should be relative
						// The frontend will construct the full URL based on current domain
						apiURL = "" // Empty means frontend will auto-detect from current URL
					} else {
						// In demo mode, use localhost with backend port
						backendPort := 8888
						if envBackendPort := os.Getenv("LIVEREVIEW_BACKEND_PORT"); envBackendPort != "" {
							if parsedPort, err := strconv.Atoi(envBackendPort); err == nil {
								backendPort = parsedPort
							}
						} else if envBackendPort := os.Getenv("BACKEND_PORT"); envBackendPort != "" {
							// Legacy support for existing deployments
							if parsedPort, err := strconv.Atoi(envBackendPort); err == nil {
								backendPort = parsedPort
							}
						}
						apiURL = fmt.Sprintf("http://localhost:%d", backendPort)
					}
				}
			}

			fmt.Printf("Starting LiveReview UI server on port %d...\n", port)
			fmt.Printf("🐛 DEBUG: LIVEREVIEW_REVERSE_PROXY=%s\n", os.Getenv("LIVEREVIEW_REVERSE_PROXY"))
			fmt.Printf("🐛 DEBUG: API_URL env var=%s\n", os.Getenv("API_URL"))
			fmt.Printf("🐛 DEBUG: Final apiURL configured as: '%s'\n", apiURL)
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

			// Pre-gzip compressible static assets once at boot instead of per-request (see
			// buildCompressedAssets doc comment / docs/perf-improvement.md "Finding A").
			compressedAssets := buildCompressedAssets(distFS)
			fmt.Printf("Pre-compressed %d static assets for gzip serving\n", len(compressedAssets))

			// Proxy API requests to the backend server
			var apiProxy http.Handler
			if apiURL != "" {
				backendURL, err := url.Parse(apiURL)
				if err == nil {
					apiProxy = httputil.NewSingleHostReverseProxy(backendURL)
				}
			}
			if apiProxy == nil {
				// Fallback: try localhost:8888
				backendURL, _ := url.Parse("http://localhost:8888")
				apiProxy = httputil.NewSingleHostReverseProxy(backendURL)
			}

			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Proxy API routes to the backend API server
				if strings.HasPrefix(r.URL.Path, "/api/") {
					apiProxy.ServeHTTP(w, r)
					return
				}
				// Try to serve the requested file
				if r.URL.Path != "/" {
					// Serve pre-compressed bytes directly when available - skips per-request gzip
					// CPU cost and sets the Cache-Control headers that were previously missing
					// entirely (see docs/perf-improvement.md "Finding A"/"Finding B").
					if asset, ok := compressedAssets[r.URL.Path]; ok {
						if serveCompressedAsset(w, r, r.URL.Path, asset) {
							return
						}
					}
					// Check if file exists in embedded filesystem
					if _, err := fs.Stat(distFS, r.URL.Path[1:]); err == nil {
						setCacheHeaders(w, r.URL.Path)
						fileServer.ServeHTTP(w, r)
						return
					}
					// Fallback: some files (e.g. slack-logo.png) are in public/ subdir of dist
					if _, err := fs.Stat(distFS, "public"+r.URL.Path); err == nil {
						r.URL.Path = "/public" + r.URL.Path
						fileServer.ServeHTTP(w, r)
						return
					}
				}

				// If file doesn't exist or root path, serve modified index.html for SPA routing
				w.Header().Set("Content-Type", "text/html")
				http.ServeContent(w, r, "index.html", time.Now(), strings.NewReader(cachedIndexHTML))
			})

			listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err != nil {
				return err
			}

			server := &http.Server{Handler: nil}
			return server.Serve(listener)
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
	var configScript string
	fmt.Printf("🐛 DEBUG prepareIndexHTML: apiURL='%s'\n", apiURL)
	if apiURL != "" {
		// Explicit API URL provided
		fmt.Printf("🐛 DEBUG: Using explicit API URL: %s\n", apiURL)
		configScript = fmt.Sprintf(`<script>
		// LiveReview runtime configuration
		window.LIVEREVIEW_CONFIG = {
			apiUrl: "%s"
		};
	</script>`, apiURL)
	} else {
		// No API URL - let frontend auto-detect from current domain
		fmt.Printf("🐛 DEBUG: Using null API URL for auto-detection\n")
		configScript = `<script>
		// LiveReview runtime configuration - frontend will auto-detect API URL
		window.LIVEREVIEW_CONFIG = {
			apiUrl: null
		};
	</script>`
	}

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
