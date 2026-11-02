package jobqueue

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func unpackZipBase64(t *testing.T, encoded string) map[string]string {
	t.Helper()
	zipBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("failed to open zip reader: %v", err)
	}

	result := make(map[string]string)
	for _, f := range reader.File {
		if f.Method != zip.Deflate && len(f.Name) > 0 {
			t.Errorf("expected file %s to be compressed with Deflate (%d), got %d", f.Name, zip.Deflate, f.Method)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("failed to read zip entry %s: %v", f.Name, err)
		}
		result[f.Name] = string(data)
	}
	return result
}

func TestPackageDiffFiles_Basic(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(file1), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file1, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	file2 := filepath.Join(tempDir, "README.md")
	if err := os.WriteFile(file2, []byte("# Test Project\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b64Zip, err := PackageDiffFiles(tempDir, []string{"src/main.go", "README.md"})
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if len(files) != 2 {
		t.Fatalf("expected 2 files in zip, got %d", len(files))
	}
	if files["src/main.go"] != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected content for src/main.go: %q", files["src/main.go"])
	}
	if files["README.md"] != "# Test Project\n" {
		t.Errorf("unexpected content for README.md: %q", files["README.md"])
	}
}

func TestPackageDiffFiles_MaxFileSizeProtection(t *testing.T) {
	tempDir := t.TempDir()

	smallFile := filepath.Join(tempDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("small file content"), 0644); err != nil {
		t.Fatal(err)
	}

	largeFile := filepath.Join(tempDir, "large.txt")
	// 1.5 MiB file (exceeds 1 MiB limit)
	largeData := bytes.Repeat([]byte("a\n"), 750*1024)
	if err := os.WriteFile(largeFile, largeData, 0644); err != nil {
		t.Fatal(err)
	}

	b64Zip, err := PackageDiffFiles(tempDir, []string{"small.txt", "large.txt"})
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if _, ok := files["large.txt"]; ok {
		t.Errorf("expected large.txt to be skipped due to size > 1 MiB")
	}
	if files["small.txt"] != "small file content" {
		t.Errorf("expected small.txt to be included, got %q", files["small.txt"])
	}
}

func TestPackageDiffFiles_BinaryFilesSkipped(t *testing.T) {
	tempDir := t.TempDir()

	goodFile := filepath.Join(tempDir, "code.go")
	if err := os.WriteFile(goodFile, []byte("package test"), 0644); err != nil {
		t.Fatal(err)
	}

	pngFile := filepath.Join(tempDir, "icon.png")
	if err := os.WriteFile(pngFile, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0644); err != nil {
		t.Fatal(err)
	}

	nullFile := filepath.Join(tempDir, "binary.bin")
	if err := os.WriteFile(nullFile, []byte("hello\x00world"), 0644); err != nil {
		t.Fatal(err)
	}

	b64Zip, err := PackageDiffFiles(tempDir, []string{"code.go", "icon.png", "binary.bin"})
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if _, ok := files["icon.png"]; ok {
		t.Errorf("expected icon.png to be skipped as binary extension")
	}
	if _, ok := files["binary.bin"]; ok {
		t.Errorf("expected binary.bin to be skipped as binary content")
	}
	if files["code.go"] != "package test" {
		t.Errorf("expected code.go to be included, got %q", files["code.go"])
	}
}

func TestPackageDiffFiles_UnreadableFilesGraceful(t *testing.T) {
	tempDir := t.TempDir()

	realFile := filepath.Join(tempDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	// Request a non-existent file alongside a real file
	b64Zip, err := PackageDiffFiles(tempDir, []string{"real.txt", "does/not/exist.txt"})
	if err != nil {
		t.Fatalf("PackageDiffFiles should not fail on missing file, got: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if len(files) != 1 || files["real.txt"] != "ok" {
		t.Errorf("unexpected files in zip: %v", files)
	}
}

func TestPackageDiffFiles_IncludeToolsToml(t *testing.T) {
	tempDir := t.TempDir()

	policyDir := filepath.Join(tempDir, ".lrc", "policy")
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		t.Fatal(err)
	}
	toolsToml := filepath.Join(policyDir, "tools.toml")
	if err := os.WriteFile(toolsToml, []byte("[gitleaks]\nenabled = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	codeFile := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(codeFile, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	b64Zip, err := PackageDiffFiles(tempDir, []string{"main.go"})
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if len(files) != 2 {
		t.Fatalf("expected 2 files (main.go and .lrc/policy/tools.toml), got %d: %v", len(files), files)
	}
	if files[".lrc/policy/tools.toml"] != "[gitleaks]\nenabled = true\n" {
		t.Errorf("unexpected .lrc/policy/tools.toml content: %q", files[".lrc/policy/tools.toml"])
	}
}

func TestPackageDiffFiles_OnlyPolicyToolsTomlIncluded(t *testing.T) {
	tempDir := t.TempDir()

	lrcDir := filepath.Join(tempDir, ".lrc")
	if err := os.MkdirAll(lrcDir, 0755); err != nil {
		t.Fatal(err)
	}
	toolsToml := filepath.Join(lrcDir, "tools.toml")
	if err := os.WriteFile(toolsToml, []byte("[semgrep]\nenabled = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	b64Zip, err := PackageDiffFiles(tempDir, nil)
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	files := unpackZipBase64(t, b64Zip)
	if _, exists := files[".lrc/tools.toml"]; exists {
		t.Errorf("expected .lrc/tools.toml to NOT be included (strictly .lrc/policy/tools.toml only), but got: %v", files)
	}
}

func TestPackageDiffFiles_CumulativeZipLimit(t *testing.T) {
	tempDir := t.TempDir()

	// Create 20 random alphanumeric text files of 800 KiB each (~16 MiB total).
	// Because alphanumeric text has high entropy, it compresses to ~600 KiB each,
	// exceeding 10 MiB across 20 files.
	var fileNames []string
	rng := rand.New(rand.NewSource(42))
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789\n"

	for i := 0; i < 20; i++ {
		data := make([]byte, 800*1024)
		for j := range data {
			data[j] = charset[rng.Intn(len(charset))]
		}
		name := fmt.Sprintf("file_%02d.txt", i)
		fullPath := filepath.Join(tempDir, name)
		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			t.Fatal(err)
		}
		fileNames = append(fileNames, name)
	}

	b64Zip, err := PackageDiffFiles(tempDir, fileNames)
	if err != nil {
		t.Fatalf("PackageDiffFiles returned error: %v", err)
	}

	rawZip, err := base64.StdEncoding.DecodeString(b64Zip)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}

	if len(rawZip) > MaxCumulativeZipSize {
		t.Errorf("zip size %d exceeded MaxCumulativeZipSize %d", len(rawZip), MaxCumulativeZipSize)
	}

	// Verify the zip is valid and contains some files, but not all 20
	reader, err := zip.NewReader(bytes.NewReader(rawZip), int64(len(rawZip)))
	if err != nil {
		t.Fatalf("zip reader failed on capped zip: %v", err)
	}
	if len(reader.File) == 0 {
		t.Errorf("expected at least one file packaged before limit was hit")
	}
	if len(reader.File) >= 20 {
		t.Errorf("expected packaging to stop before packing all 20 large files, got %d", len(reader.File))
	}
}

func TestPackageDiffFiles_EmptyBaseDirError(t *testing.T) {
	_, err := PackageDiffFiles("", []string{"main.go"})
	if err == nil {
		t.Error("expected error for empty baseDir, got nil")
	}
}
