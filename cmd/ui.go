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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/urfave/cli/v2"
)

// Cache for the modified index.html content
var (
	cachedIndexHTML string
	cacheOnce       sync.Once
)

// compressedAsset holds build-time-computed gzip and brotli payloads for one static file, so
// compression never has to happen on the request hot path (see docs/perf-improvement.md,
// "Finding A" - nginx doing on-the-fly gzip per request was serializing entire request
// bursts, static files and tiny API JSON responses alike, behind its CPU cost). Brotli is
// preferred when the client supports it (every modern browser does, confirmed via HAR -
// Accept-Encoding always includes "br") since it typically beats gzip by 15-20% on JS/CSS text,
// which matters directly on a throughput-constrained connection - see docs/perf-improvement.md
// round 6.
type compressedAsset struct {
	gzipData    []byte
	brotliData  []byte
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

// buildCompressedAssets walks the embedded dist filesystem once at startup and compresses every
// compressible top-level file (skipping index.html, which is regenerated per-request with
// injected runtime config, and the public/ subtree, which is low-volume and not part of the
// measured bottleneck). Doing this once at boot instead of per-request is the actual fix - see
// docs/perf-improvement.md. Compresses across a worker pool sized to GOMAXPROCS: brotli at
// BestCompression is slow enough on the largest bundled libraries (echarts, moment-timezone) that
// doing all ~100 files sequentially blocked the listener from opening for 8+ seconds on this
// box's 2 cores - real downtime on every pm2 restart, not just a one-time cost. Parallelizing
// keeps the win (compression still never happens on the request path) without that startup cost.
func buildCompressedAssets(distFS fs.FS) map[string]compressedAsset {
	type job struct {
		path string
		ext  string
	}
	var jobs []job
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
		jobs = append(jobs, job{path: path, ext: ext})
		return nil
	})

	type result struct {
		urlPath string
		asset   compressedAsset
	}
	jobCh := make(chan job)
	resultCh := make(chan result, len(jobs))

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				raw, readErr := fs.ReadFile(distFS, j.path)
				if readErr != nil {
					continue
				}
				var gzBuf bytes.Buffer
				gz, gzErr := gzip.NewWriterLevel(&gzBuf, gzip.BestCompression)
				if gzErr != nil {
					continue
				}
				if _, writeErr := gz.Write(raw); writeErr != nil {
					gz.Close()
					continue
				}
				if closeErr := gz.Close(); closeErr != nil {
					continue
				}
				var brBuf bytes.Buffer
				// Quality 10, not BestCompression(11): benchmarked on this build's largest bundled
				// file (moment-timezone, ~714KB raw) - 11 took 512ms and 10 took 105ms (~5x faster)
				// for only ~2.7% larger output. With ~100 files to compress at startup on this
				// box's 2 cores, 11 meant several seconds of the UI process not accepting
				// connections on every pm2 restart; 10 keeps nearly all the byte-size win at a
				// fraction of the startup cost.
				brWriter := brotli.NewWriterLevel(&brBuf, 10)
				if _, writeErr := brWriter.Write(raw); writeErr != nil {
					brWriter.Close()
					continue
				}
				if closeErr := brWriter.Close(); closeErr != nil {
					continue
				}
				contentType := mime.TypeByExtension(j.ext)
				if contentType == "" {
					contentType = "application/octet-stream"
				}
				resultCh <- result{
					urlPath: "/" + j.path,
					asset:   compressedAsset{gzipData: gzBuf.Bytes(), brotliData: brBuf.Bytes(), contentType: contentType},
				}
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
		wg.Wait()
		close(resultCh)
	}()

	assets := make(map[string]compressedAsset, len(jobs))
	for r := range resultCh {
		assets[r.urlPath] = r.asset
	}
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

// serveCompressedAsset writes a pre-compressed asset directly, skipping per-request compression
// work. Prefers brotli (smaller, and every modern browser advertises support for it) over gzip.
// Returns false (does nothing) if the client advertises neither, so the caller can fall back to
// the uncompressed file-server path.
func serveCompressedAsset(w http.ResponseWriter, r *http.Request, urlPath string, asset compressedAsset) bool {
	acceptEncoding := r.Header.Get("Accept-Encoding")
	var encoding string
	var data []byte
	switch {
	case strings.Contains(acceptEncoding, "br"):
		encoding, data = "br", asset.brotliData
	case strings.Contains(acceptEncoding, "gzip"):
		encoding, data = "gzip", asset.gzipData
	default:
		return false
	}
	setCacheHeaders(w, urlPath)
	w.Header().Set("Content-Type", asset.contentType)
	w.Header().Set("Content-Encoding", encoding)
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(data)
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

			// Pre-gzip/pre-brotli compressible static assets once at boot instead of per-request
			// (see buildCompressedAssets doc comment / docs/perf-improvement.md "Finding A").
			// Built in the background, not synchronously before the listener opens: this used to
			// block startup for 2-8+ seconds (depending on compression settings), and since
			// livereview-ui runs in pm2 fork mode (not cluster), a `pm2 reload` can't do true
			// zero-downtime for a single instance - that startup delay was a real window of
			// connection-refused/502s on every deploy (see docs/perf-improvement.md round 6's
			// postmortem). The listener now opens immediately; requests that arrive before
			// compression finishes transparently fall through to the existing uncompressed
			// fileServer.ServeHTTP path below (the same fallback already used for clients that
			// don't advertise gzip/brotli support), so there's no gap where the server can't
			// serve a request - only a brief window where responses are larger than they will be
			// once the cache is warm.
			var compressedAssets atomic.Pointer[map[string]compressedAsset]
			go func() {
				assets := buildCompressedAssets(distFS)
				compressedAssets.Store(&assets)
				fmt.Printf("Pre-compressed %d static assets for gzip/brotli serving\n", len(assets))
			}()

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
					// entirely (see docs/perf-improvement.md "Finding A"/"Finding B"). The pointer
					// is nil until the background compression goroutine finishes; falls through
					// to the uncompressed path below until then.
					if assets := compressedAssets.Load(); assets != nil {
						if asset, ok := (*assets)[r.URL.Path]; ok {
							if serveCompressedAsset(w, r, r.URL.Path, asset) {
								return
							}
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
