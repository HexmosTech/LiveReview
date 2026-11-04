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
	"regexp"
	"sort"
	"strconv"
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
	bundle := lrcconfig.Bundle{Files: map[string][]byte{}}
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

var hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@`)

// ParseUnifiedHunks splits a single-file unified diff into DiffHunks, one per
// "@@ ... @@" section (moved from the Azure DevOps provider for reuse).
func ParseUnifiedHunks(patch string) []models.DiffHunk {
	if patch == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	var hunks []models.DiffHunk
	var currentHunk *models.DiffHunk
	var hunkContent strings.Builder

	for _, line := range lines {
		if match := hunkHeaderRegex.FindStringSubmatch(line); match != nil {
			if currentHunk != nil {
				currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
				hunks = append(hunks, *currentHunk)
				hunkContent.Reset()
			}

			oldStart, _ := strconv.Atoi(match[1])
			oldCount := 1
			if match[2] != "" {
				oldCount, _ = strconv.Atoi(match[2])
			}
			newStart, _ := strconv.Atoi(match[3])
			newCount := 1
			if match[4] != "" {
				newCount, _ = strconv.Atoi(match[4])
			}

			currentHunk = &models.DiffHunk{
				OldStartLine: oldStart,
				OldLineCount: oldCount,
				NewStartLine: newStart,
				NewLineCount: newCount,
			}
			hunkContent.WriteString(line + "\n")
		} else if currentHunk != nil {
			hunkContent.WriteString(line + "\n")
		}
	}

	if currentHunk != nil {
		currentHunk.Content = strings.TrimSuffix(hunkContent.String(), "\n")
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

// ConvertModelDiffsToLocal converts []*models.CodeDiff to []lib.LocalCodeDiff.
func ConvertModelDiffsToLocal(diffs []*models.CodeDiff) []lib.LocalCodeDiff {
	res := make([]lib.LocalCodeDiff, 0, len(diffs))
	for _, d := range diffs {
		if d == nil {
			continue
		}
		oldPath := d.OldFilePath
		if oldPath == "" {
			oldPath = d.FilePath
		}
		newPath := d.FilePath
		if newPath == "" {
			newPath = oldPath
		}
		hunks := make([]lib.LocalDiffHunk, 0, len(d.Hunks))
		for _, h := range d.Hunks {
			localHunk := lib.LocalDiffHunk{
				OldStartLine: h.OldStartLine,
				OldLineCount: h.OldLineCount,
				NewStartLine: h.NewStartLine,
				NewLineCount: h.NewLineCount,
			}
			lines := strings.Split(h.Content, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "@@") {
					localHunk.HeaderText = strings.TrimSpace(line)
					continue
				}
				if len(line) == 0 {
					continue
				}
				lineType := "context"
				switch line[0] {
				case '+':
					lineType = "added"
				case '-':
					lineType = "deleted"
				}
				localHunk.Lines = append(localHunk.Lines, lib.LocalDiffLine{
					Content:  line[1:],
					LineType: lineType,
				})
			}
			hunks = append(hunks, localHunk)
		}
		res = append(res, lib.LocalCodeDiff{
			OldPath: oldPath,
			NewPath: newPath,
			Hunks:   hunks,
		})
	}
	return res
}

// BuildUnifiedDiffFromModelDiffs formats []*models.CodeDiff into a git unified diff string.
func BuildUnifiedDiffFromModelDiffs(diffs []*models.CodeDiff) string {
	var buf strings.Builder
	for _, d := range diffs {
		if d == nil {
			continue
		}
		oldPath := d.OldFilePath
		if oldPath == "" {
			oldPath = d.FilePath
		}
		newPath := d.FilePath
		if newPath == "" {
			newPath = oldPath
		}
		if oldPath == "" && newPath == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", oldPath, newPath))
		if d.IsNew {
			buf.WriteString("new file mode 100644\n")
			buf.WriteString("--- /dev/null\n")
			buf.WriteString(fmt.Sprintf("+++ b/%s\n", newPath))
		} else if d.IsDeleted {
			buf.WriteString("deleted file mode 100644\n")
			buf.WriteString(fmt.Sprintf("--- a/%s\n", oldPath))
			buf.WriteString("+++ /dev/null\n")
		} else {
			buf.WriteString(fmt.Sprintf("--- a/%s\n", oldPath))
			buf.WriteString(fmt.Sprintf("+++ b/%s\n", newPath))
		}
		for _, hunk := range d.Hunks {
			if hunk.Content == "" {
				continue
			}
			trimmedLeft := strings.TrimLeft(hunk.Content, " \t\r\n")
			if !strings.HasPrefix(trimmedLeft, "@@") {
				buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", hunk.OldStartLine, hunk.OldLineCount, hunk.NewStartLine, hunk.NewLineCount))
				buf.WriteString(hunk.Content)
			} else {
				buf.WriteString(trimmedLeft)
			}
			if !strings.HasSuffix(hunk.Content, "\n") {
				buf.WriteByte('\n')
			}
		}
	}
	return buf.String()
}

const (
	maxZipPackageFileBytes  = 25 << 20  // 25 MiB max per file when packaging zip
	maxZipPackageTotalBytes = 200 << 20 // 200 MiB max total payload across files
)

// PackageModelDiffsToZipBase64WithExtras creates a base64-encoded zip archive containing
// diff.txt, the changed files, and any optional extra files (e.g. .lrc tree).
// Bounds are enforced for memory safety.
func PackageModelDiffsToZipBase64WithExtras(diffs []*models.CodeDiff, extras map[string][]byte) (string, error) {
	if len(diffs) == 0 && len(extras) == 0 {
		return "", nil
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// 1. Write diff.txt first so extractZip sees it as the primary diff file
	unifiedDiff := BuildUnifiedDiffFromModelDiffs(diffs)
	diffWriter, err := zw.Create("diff.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create diff.txt entry: %w", err)
	}
	if _, err := diffWriter.Write([]byte(unifiedDiff)); err != nil {
		return "", fmt.Errorf("failed to write diff.txt: %w", err)
	}

	var totalBytes int64 = int64(len(unifiedDiff))

	// 2. Write each changed file content (if not deleted)
	for _, d := range diffs {
		if d == nil || d.IsDeleted {
			continue
		}
		filePath := d.FilePath
		if filePath == "" {
			continue
		}
		cleaned := filepath.Clean(filePath)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			continue // skip unsafe paths
		}

		var fileContent []byte
		if d.NewContent != "" {
			fileContent = []byte(d.NewContent)
		} else {
			var fileBuf strings.Builder
			for _, h := range d.Hunks {
				lines := strings.Split(h.Content, "\n")
				for _, l := range lines {
					if strings.HasPrefix(l, "@@") {
						continue
					}
					if len(l) > 0 && (l[0] == '+' || l[0] == ' ') {
						fileBuf.WriteString(l[1:])
						fileBuf.WriteByte('\n')
					}
				}
			}
			fileContent = []byte(fileBuf.String())
		}

		if int64(len(fileContent)) > maxZipPackageFileBytes {
			fileContent = fileContent[:maxZipPackageFileBytes]
		}
		if totalBytes+int64(len(fileContent)) > maxZipPackageTotalBytes {
			log.Printf("[WARN] diffutil: total packaged size exceeds limit of %d bytes, skipping remaining files starting at %s", maxZipPackageTotalBytes, cleaned)
			break // memory safety limit reached
		}
		totalBytes += int64(len(fileContent))

		fw, err := zw.Create(filepath.ToSlash(cleaned))
		if err != nil {
			log.Printf("[WARN] diffutil: failed to create zip entry for %s: %v", cleaned, err)
			continue
		}
		if _, err := fw.Write(fileContent); err != nil {
			log.Printf("[WARN] diffutil: failed to write zip entry for %s: %v", cleaned, err)
			continue
		}
	}

	// 3. Write any extras (e.g. .lrc/ configs)
	if extras != nil {
		extraNames := make([]string, 0, len(extras))
		for name := range extras {
			extraNames = append(extraNames, name)
		}
		sort.Strings(extraNames)
		for _, name := range extraNames {
			cleaned := filepath.Clean(name)
			if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
				continue
			}
			data := extras[name]
			if totalBytes+int64(len(data)) > maxZipPackageTotalBytes {
				log.Printf("[WARN] diffutil: total packaged size exceeds limit of %d bytes, skipping extra file %s", maxZipPackageTotalBytes, cleaned)
				break
			}
			totalBytes += int64(len(data))

			ew, err := zw.Create(filepath.ToSlash(cleaned))
			if err != nil {
				log.Printf("[WARN] diffutil: failed to create extra zip entry for %s: %v", cleaned, err)
				continue
			}
			if _, err := ew.Write(data); err != nil {
				log.Printf("[WARN] diffutil: failed to write extra zip entry for %s: %v", cleaned, err)
				continue
			}
		}
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("failed to close zip writer: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// PackageModelDiffsToZipBase64 creates a base64-encoded zip archive from []*models.CodeDiff.
func PackageModelDiffsToZipBase64(diffs []*models.CodeDiff) (string, error) {
	return PackageModelDiffsToZipBase64WithExtras(diffs, nil)
}
