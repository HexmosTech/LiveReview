package jobqueue

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxPackagedFileSize is the maximum allowed size (1 MiB) for any single file packaged.
	MaxPackagedFileSize = 1 << 20
	// MaxCumulativeZipSize is the maximum allowed compressed size (10 MiB) for the generated zip archive.
	MaxCumulativeZipSize = 10 << 20
)

var binaryExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
	".ico": true, ".tif": true, ".tiff": true, ".webp": true, ".svgz": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true, ".lib": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true,
	".rar": true, ".jar": true, ".war": true, ".ear": true, ".class": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".bin": true, ".dat": true, ".o": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".wmv": true,
	".flv": true, ".webm": true, ".ttf": true, ".woff": true, ".woff2": true,
	".eot": true, ".pyc": true, ".pyd": true, ".pyo": true, ".wasm": true,
}

const (
	binaryCheckSampleSize = 1024 // Sample size in bytes used to detect binary content
	zipEntryOverheadBytes = 128  // Approximate header and directory overhead per zip entry
)

// isBinaryContent inspects a byte buffer to determine if it contains binary data.
func isBinaryContent(ext string, data []byte) bool {
	if binaryExtensions[ext] {
		return true
	}
	if len(data) == 0 {
		return false
	}
	sampleSize := binaryCheckSampleSize
	if len(data) < sampleSize {
		sampleSize = len(data)
	}
	sample := data[:sampleSize]
	if bytes.IndexByte(sample, 0) != -1 {
		return true
	}
	nonPrintable := 0
	for _, b := range sample {
		if (b < 32 && b != '\t' && b != '\n' && b != '\r') || b == 127 {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(sampleSize) > 0.3
}

// estimateDeflatedSize estimates the compressed size of content when deflated.
func estimateDeflatedSize(content []byte) int {
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, flate.DefaultCompression)
	if err != nil {
		log.Printf("[WARN] diff_packager: flate writer error: %v", err)
		return len(content)
	}
	if _, err := w.Write(content); err != nil {
		log.Printf("[WARN] diff_packager: flate write error: %v", err)
		return len(content)
	}
	if err := w.Close(); err != nil {
		log.Printf("[WARN] diff_packager: flate close error: %v", err)
		return len(content)
	}
	return b.Len()
}

// PackageDiffFiles packages files from baseDir into a compressed, base64-encoded zip archive.
// Returns the base64-encoded zip string, a truncated flag indicating if size limits were hit, and an error if any.
//
// In-memory bloat protections:
//   - Max file size: 1 MiB per file (files larger than 1 MiB are skipped with a debug log and truncated=true).
//   - Max cumulative zip size: 10 MiB (packaging halts once zip size reaches or exceeds 10 MiB and truncated=true).
//   - Binary and unreadable files are skipped gracefully.
//   - Repository tool configs (.lrc/policy/tools.toml) are included if present in baseDir.
func PackageDiffFiles(baseDir string, filePaths []string) (string, bool, error) {
	if baseDir == "" {
		return "", false, fmt.Errorf("baseDir cannot be empty")
	}

	seen := make(map[string]bool)
	allPaths := make([]string, 0, len(filePaths)+2)
	for _, p := range filePaths {
		cleanRel := filepath.ToSlash(filepath.Clean(p))
		cleanRel = strings.TrimPrefix(cleanRel, "/")
		if cleanRel == "" || cleanRel == "." || strings.HasPrefix(cleanRel, "../") || cleanRel == ".." {
			continue
		}
		if !seen[cleanRel] {
			seen[cleanRel] = true
			allPaths = append(allPaths, cleanRel)
		}
	}

	candidateConfigs := []string{".lrc/policy/tools.toml"}
	for _, candidate := range candidateConfigs {
		if seen[candidate] {
			continue
		}
		fullPath := filepath.Join(baseDir, filepath.FromSlash(candidate))
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			seen[candidate] = true
			allPaths = append(allPaths, candidate)
		}
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)
	var truncated bool

	for _, relPath := range allPaths {
		fullPath := filepath.Join(baseDir, filepath.FromSlash(relPath))
		relCheck, err := filepath.Rel(baseDir, fullPath)
		if err != nil || strings.HasPrefix(relCheck, "..") {
			log.Printf("[DEBUG] diff_packager: skipping path traversal attempt %s", relPath)
			continue
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			log.Printf("[DEBUG] diff_packager: skipping unreadable file %s: %v", relPath, err)
			continue
		}
		if info.IsDir() {
			continue
		}
		if info.Size() > MaxPackagedFileSize {
			log.Printf("[DEBUG] diff_packager: skipping %s: file size %d exceeds 1 MiB limit", relPath, info.Size())
			truncated = true
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			log.Printf("[DEBUG] diff_packager: skipping unreadable file %s: %v", relPath, err)
			continue
		}

		ext := strings.ToLower(filepath.Ext(relPath))
		if isBinaryContent(ext, content) {
			log.Printf("[DEBUG] diff_packager: skipping binary file %s", relPath)
			continue
		}

		estEntrySize := estimateDeflatedSize(content) + len(relPath) + zipEntryOverheadBytes
		if buf.Len()+estEntrySize > MaxCumulativeZipSize {
			log.Printf("[WARN] diff_packager: skipping %s: adding %d compressed bytes would exceed cumulative zip limit of 10 MiB (current %d bytes)", relPath, estEntrySize, buf.Len())
			truncated = true
			break
		}

		header := &zip.FileHeader{
			Name:     relPath,
			Method:   zip.Deflate,
			Modified: info.ModTime(),
		}

		w, err := zipWriter.CreateHeader(header)
		if err != nil {
			return "", false, fmt.Errorf("failed to create zip header for %s: %w", relPath, err)
		}
		if _, err := w.Write(content); err != nil {
			return "", false, fmt.Errorf("failed to write %s to zip: %w", relPath, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return "", false, fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), truncated, nil
}
