package capture

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var (
	sessionID  = time.Now().Format("20060102-150405")
	captureSeq uint64
)

// defaultEnabled controls whether capture is active when the process starts.
// Flip this to true (or call Enable) whenever you want to record fixtures.
const defaultEnabled = false

var captureEnabled atomic.Bool

func init() {
	captureEnabled.Store(defaultEnabled)
}

// envCaptureDir is the environment variable that controls capture output.
const envCaptureDir = "LIVEREVIEW_CAPTURE_DIR"

// defaultCaptureDir is the fallback location when the environment variable is unset.
const (
	defaultCaptureDir = "captures/github"
)

// Enabled reports whether capture is currently active.
func Enabled() bool {
	if !captureEnabled.Load() {
		return false
	}
	return captureDir("") != ""
}

// Enable globally turns on capture for the running process.
func Enable() {
	captureEnabled.Store(true)
}

// Disable globally turns off capture for the running process.
func Disable() {
	captureEnabled.Store(false)
}

// writeFile writes the provided bytes to the capture directory under the given
// category and extension. It creates any missing directories and logs failures.
func captureDir(namespace string) string {
	if dir := os.Getenv(envCaptureDir); dir != "" {
		return dir
	}
	if namespace != "" {
		return filepath.Join("captures", namespace)
	}
	return defaultCaptureDir
}

func writeFile(namespace, category, ext string, data []byte) {
	dir := captureDir(namespace)
	if dir == "" {
		return
	}

	seq := atomic.AddUint64(&captureSeq, 1)
	sessionDir := filepath.Join(dir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		log.Printf("[WARN] capture: failed to create directory %s: %v", sessionDir, err)
		return
	}

	filename := fmt.Sprintf("%s-%04d.%s", category, seq, ext)
	path := filepath.Join(sessionDir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("[WARN] capture: failed to write file %s: %v", path, err)
		return
	}

	log.Printf("[INFO] capture: wrote %s", path)
}

// WriteJSON marshals the payload to indented JSON and stores it in the capture
// directory. Failures are logged but otherwise ignored.
func WriteJSON(category string, payload interface{}) {
	if !Enabled() {
		return
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("[WARN] capture: failed to marshal %s payload: %v", category, err)
		return
	}

	writeFile("", category, "json", data)
}

// WriteBlob stores arbitrary bytes using the provided extension.
func WriteBlob(category, ext string, data []byte) {
	if !Enabled() {
		return
	}
	writeFile("", category, ext, data)
}

// WriteJSONForNamespace stores JSON payloads under captures/<namespace>/.
func WriteJSONForNamespace(namespace, category string, payload interface{}) {
	if !Enabled() {
		return
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Printf("[WARN] capture: failed to marshal %s payload: %v", category, err)
		return
	}

	writeFile(namespace, category, "json", data)
}

// WriteBlobForNamespace stores arbitrary bytes under captures/<namespace>/.
func WriteBlobForNamespace(namespace, category, ext string, data []byte) {
	if !Enabled() {
		return
	}
	writeFile(namespace, category, ext, data)
}
