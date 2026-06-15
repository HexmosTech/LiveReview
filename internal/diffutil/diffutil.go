package diffutil

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/lrcconfig"
	"github.com/livereview/pkg/models"
	"github.com/livereview/storage/archive"
)

const (
	maxExtractedFileBytes  = 25 << 20  // 25 MiB per extracted file
	maxExtractedTotalBytes = 200 << 20 // 200 MiB across all extracted files
)

// CalculateEffectiveDiffLOCFromLocalDiffs returns billable LOC for an operation.
// Billable LOC is defined as added + deleted lines across all hunks.
func CalculateEffectiveDiffLOCFromLocalDiffs(localDiffs []lib.LocalCodeDiff) int64 {
	var total int64
	for _, diff := range localDiffs {
		for _, hunk := range diff.Hunks {
			for _, line := range hunk.Lines {
				switch line.LineType {
				case "added", "deleted":
					total++
				}
			}
		}
	}
	return total
}

// ParseDiffZipBase64 decodes the client payload (base64 zip containing a unified diff)
// and returns the parsed local diffs and raw .lrc/ configuration files bundle.
func ParseDiffZipBase64(encoded string) ([]lib.LocalCodeDiff, lrcconfig.Bundle, error) {
	zipBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to decode diff_zip_base64: %w", err)
	}

	tempDir, err := archive.DiffReviewCreateTempWorkspace()
	if err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to create temp workspace: %w", err)
	}
	defer func() {
		if cleanupErr := archive.DiffReviewRemoveWorkspace(tempDir); cleanupErr != nil {
			log.Printf("[WARN] failed to clean up temp workspace %q: %v", tempDir, cleanupErr)
		}
	}()

	zipPath := filepath.Join(tempDir, "diff.zip")
	if err := archive.DiffReviewWriteUploadedZip(zipPath, zipBytes); err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to persist uploaded zip: %w", err)
	}

	extractedFiles, err := extractZip(zipPath, tempDir)
	if err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to extract zip: %w", err)
	}
	if len(extractedFiles) == 0 {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("zip archive contained no files")
	}

	diffContent, err := archive.DiffReviewReadExtractedDiff(extractedFiles[0])
	if err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to read extracted diff: %w", err)
	}

	parser := lib.NewLocalParser()
	localDiffs, err := parser.Parse(string(diffContent))
	if err != nil {
		return nil, lrcconfig.Bundle{}, fmt.Errorf("failed to parse diff: %w", err)
	}

	bundle, err := collectLRCBundle(tempDir)
	if err != nil {
		log.Printf("[WARN] failed to read .lrc/ bundle: %v", err)
		bundle = lrcconfig.Bundle{}
	}

	return localDiffs, bundle, nil
}

// collectLRCBundle reads the .lrc/ tree extracted under tempDir (if any)
// into an lrcconfig.Bundle keyed by path relative to .lrc/, with map keys
// using "/" separators (via filepath.ToSlash) regardless of host OS.
func collectLRCBundle(tempDir string) (lrcconfig.Bundle, error) {
	lrcDir := filepath.Join(tempDir, ".lrc")
	info, err := os.Stat(lrcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return lrcconfig.Bundle{}, nil
		}
		return lrcconfig.Bundle{}, err
	}
	if !info.IsDir() {
		return lrcconfig.Bundle{}, nil
	}

	bundle := lrcconfig.Bundle{Files: map[string][]byte{}}
	walkErr := filepath.WalkDir(lrcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Only ever read plain files: extractZip never writes symlinks or
		// other special files, but skip them defensively rather than
		// following a symlink that somehow ended up here.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(lrcDir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		bundle.Files[filepath.ToSlash(rel)] = content
		return nil
	})
	if walkErr != nil {
		return lrcconfig.Bundle{}, walkErr
	}

	return bundle, nil
}

// maxExcludedFilesListed caps how many .lrc/ignore-excluded file paths are
// named in a review summary before the rest are collapsed into "and N more",
// so a large ignore list doesn't produce an unreadable summary.
const maxExcludedFilesListed = 10

func FormatExcludedFiles(excluded []string) string {
	if len(excluded) <= maxExcludedFilesListed {
		return strings.Join(excluded, ", ")
	}
	shown := excluded[:maxExcludedFilesListed]
	return fmt.Sprintf("%s, and %d more", strings.Join(shown, ", "), len(excluded)-maxExcludedFilesListed)
}

func extractZip(zipPath, dest string) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var extracted []string
	var totalExtracted int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if int64(f.UncompressedSize64) > maxExtractedFileBytes {
			return extracted, fmt.Errorf("zip entry too large: %s", f.Name)
		}
		if totalExtracted+int64(f.UncompressedSize64) > maxExtractedTotalBytes {
			return extracted, fmt.Errorf("zip exceeds maximum extracted size")
		}
		cleaned := filepath.Clean(f.Name)
		targetPath := filepath.Join(dest, cleaned)
		if !strings.HasPrefix(targetPath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path %s", f.Name)
		}
		if err := archive.DiffReviewEnsureParentDir(targetPath); err != nil {
			return extracted, err
		}
		rc, err := f.Open()
		if err != nil {
			return extracted, err
		}

		out, err := archive.DiffReviewOpenExtractedFile(targetPath, f.Mode())
		if err != nil {
			_ = rc.Close()
			return extracted, err
		}
		written, err := io.CopyN(out, rc, maxExtractedFileBytes+1)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			out.Close()
			_ = rc.Close()
			return extracted, err
		}
		if written > maxExtractedFileBytes {
			out.Close()
			_ = rc.Close()
			return extracted, fmt.Errorf("zip entry exceeds per-file limit: %s", f.Name)
		}
		totalExtracted += written
		if totalExtracted > maxExtractedTotalBytes {
			out.Close()
			_ = rc.Close()
			return extracted, fmt.Errorf("zip exceeds maximum extracted size")
		}
		out.Close()
		_ = rc.Close()

		extracted = append(extracted, targetPath)
	}
	return extracted, nil
}

// ConvertLocalDiffs converts []lib.LocalCodeDiff to []*models.CodeDiff
func ConvertLocalDiffs(localDiffs []lib.LocalCodeDiff) []*models.CodeDiff {
	converted := make([]*models.CodeDiff, 0, len(localDiffs))
	for _, ld := range localDiffs {
		converted = append(converted, ConvertLocalToModelDiff(ld))
	}
	return converted
}

// ConvertLocalToModelDiff converts a single lib.LocalCodeDiff to *models.CodeDiff
func ConvertLocalToModelDiff(local lib.LocalCodeDiff) *models.CodeDiff {
	hunks := make([]models.DiffHunk, 0, len(local.Hunks))
	for _, h := range local.Hunks {
		hunks = append(hunks, ConvertLocalHunk(h))
	}

	filePath := local.NewPath
	if strings.TrimSpace(filePath) == "" {
		filePath = local.OldPath
	}

	return &models.CodeDiff{
		FilePath:    filePath,
		OldContent:  "",
		NewContent:  "",
		Hunks:       hunks,
		CommitID:    "",
		FileType:    filepath.Ext(filePath),
		IsDeleted:   false,
		IsNew:       false,
		IsRenamed:   false,
		OldFilePath: local.OldPath,
	}
}

// ConvertLocalHunk converts a single lib.LocalDiffHunk to models.DiffHunk
func ConvertLocalHunk(h lib.LocalDiffHunk) models.DiffHunk {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStartLine, h.OldLineCount, h.NewStartLine, h.NewLineCount))
	if strings.TrimSpace(h.HeaderText) != "" {
		buf.WriteByte(' ')
		buf.WriteString(strings.TrimSpace(h.HeaderText))
	}
	buf.WriteByte('\n')

	for _, line := range h.Lines {
		prefix := " "
		switch line.LineType {
		case "added":
			prefix = "+"
		case "deleted":
			prefix = "-"
		}
		buf.WriteString(prefix)
		buf.WriteString(line.Content)
		buf.WriteByte('\n')
	}

	content := strings.TrimSuffix(buf.String(), "\n")
	return models.DiffHunk{
		OldStartLine: h.OldStartLine,
		OldLineCount: h.OldLineCount,
		NewStartLine: h.NewStartLine,
		NewLineCount: h.NewLineCount,
		Content:      content,
	}
}
