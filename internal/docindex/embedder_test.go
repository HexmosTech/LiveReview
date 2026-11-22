package docindex

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// zipWithMember builds an in-memory .zip containing one member, mirroring the
// layout of a real onnxruntime release archive closely enough for
// extractMember.
func zipWithMember(t *testing.T, member string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(member)
	if err != nil {
		t.Fatalf("create zip member: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write zip member: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mirrorServer serves body at servePath (404 elsewhere) and counts hits.
func mirrorServer(t *testing.T, servePath string, body []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != servePath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestEnsureRuntimeLibraryPrefersCachedCopy(t *testing.T) {
	cacheDir := t.TempDir()
	cached := filepath.Join(cacheDir, "onnxruntime", "1.29.0", "libonnxruntime.so")
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cached, []byte("cached"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Any network hit would be a failure: the cached copy must satisfy the
	// request regardless of which source produced it.
	srv, hits := mirrorServer(t, "/never", nil)
	asset := runtimeAsset{
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{{
			Version:    "1.29.0",
			URL:        srv.URL + "/never",
			Kind:       archiveZip,
			MemberPath: "never",
		}},
	}

	lib, err := ensureRuntimeLibrary(asset, cacheDir)
	if err != nil {
		t.Fatalf("ensureRuntimeLibrary: %v", err)
	}
	if lib.Path != cached {
		t.Errorf("path = %q, want %q", lib.Path, cached)
	}
	if !lib.Cached || lib.Version != "1.29.0" || lib.Source != srv.URL+"/never" {
		t.Errorf("resolution = %+v, want cached 1.29.0 from the configured source", lib)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server hit %d times, want 0", n)
	}
}

func TestEnsureRuntimeLibraryFallsBackToMirror(t *testing.T) {
	const member = "onnxruntime-linux-x64-glibc2_17-Release-1.31.0/lib/libonnxruntime.so"
	content := []byte("mirror-lib")
	archive := zipWithMember(t, member, content)

	srv, hits := mirrorServer(t, "/mirror.zip", archive)
	cacheDir := t.TempDir()
	asset := runtimeAsset{
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{
			{ // primary: unavailable (404, as the deleted Microsoft release is)
				Version:    "1.29.0",
				URL:        srv.URL + "/primary.zip",
				Kind:       archiveZip,
				MemberPath: "onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0",
			},
			{
				Version:    "1.31.0",
				URL:        srv.URL + "/mirror.zip",
				SHA256:     sha256Hex(archive),
				Size:       int64(len(archive)),
				Kind:       archiveZip,
				MemberPath: member,
			},
		},
	}

	lib, err := ensureRuntimeLibrary(asset, cacheDir)
	if err != nil {
		t.Fatalf("ensureRuntimeLibrary: %v", err)
	}
	want := filepath.Join(cacheDir, "onnxruntime", "1.31.0", "libonnxruntime.so")
	if lib.Path != want {
		t.Fatalf("path = %q, want %q", lib.Path, want)
	}
	if lib.Cached {
		t.Errorf("resolution = %+v, want a fresh download", lib)
	}
	if lib.Version != "1.31.0" || lib.Source != srv.URL+"/mirror.zip" {
		t.Errorf("resolution = %+v, want the mirror source", lib)
	}
	if data, err := os.ReadFile(lib.Path); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(data, content) {
		t.Errorf("library contents = %q, want %q", data, content)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("server hit %d times, want 2 (primary 404 + mirror)", n)
	}

	// A second call must reuse the mirror's cached copy without any network.
	if _, err := ensureRuntimeLibrary(asset, cacheDir); err != nil {
		t.Fatalf("second ensureRuntimeLibrary: %v", err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("server hit %d times after cached call, want 2", n)
	}
}

func TestEnsureRuntimeLibrarySkipsSourceWithBadChecksum(t *testing.T) {
	const member = "dir/lib/libonnxruntime.so"
	content := []byte("good-lib")
	archive := zipWithMember(t, member, content)

	srv, _ := mirrorServer(t, "/good.zip", archive)
	cacheDir := t.TempDir()
	asset := runtimeAsset{
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{
			{
				Version:    "1.29.0",
				URL:        srv.URL + "/good.zip",
				SHA256:     strings.Repeat("0", 64), // simulate a tampered/mutated asset
				Kind:       archiveZip,
				MemberPath: member,
			},
			{
				Version:    "1.31.0",
				URL:        srv.URL + "/good.zip",
				SHA256:     sha256Hex(archive),
				Size:       int64(len(archive)),
				Kind:       archiveZip,
				MemberPath: member,
			},
		},
	}

	lib, err := ensureRuntimeLibrary(asset, cacheDir)
	if err != nil {
		t.Fatalf("ensureRuntimeLibrary: %v", err)
	}
	if want := filepath.Join(cacheDir, "onnxruntime", "1.31.0", "libonnxruntime.so"); lib.Path != want {
		t.Errorf("path = %q, want %q", lib.Path, want)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "onnxruntime", "1.29.0", "libonnxruntime.so")); !os.IsNotExist(err) {
		t.Errorf("checksum-rejected source left a file behind: %v", err)
	}
}

func TestEnsureRuntimeLibraryReportsEveryFailedSource(t *testing.T) {
	srv, hits := mirrorServer(t, "/never", nil)
	asset := runtimeAsset{
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{
			{Version: "1.29.0", URL: srv.URL + "/primary.zip", Kind: archiveZip, MemberPath: "a"},
			{Version: "1.31.0", URL: srv.URL + "/mirror.zip", Kind: archiveZip, MemberPath: "b"},
		},
	}

	_, err := ensureRuntimeLibrary(asset, t.TempDir())
	if err == nil {
		t.Fatal("expected an error when every source fails")
	}
	for _, want := range []string{"primary.zip", "mirror.zip"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("server hit %d times, want 2", n)
	}
}

// memberTarBz2 is a real .tar.bz2 containing dir/lib/testlib.dll ("BZ2-LIB"),
// inlined because the standard library has a bzip2 reader but no writer and a
// committed binary fixture would be rejected by the repo's binary-file
// pre-commit hook.
const memberTarBz2 = "QlpoOTFBWSZTWWtJmCEAAHr/hMEAAIhAA/+AECYAEHZkHwAAAQAIIAB0GjUofoUeiAzU8ozU8oJSNEAA0AADz7mdVQQIPARCDy2Cq4oYiIDWDem3bKx2BEJNkI0JAV5Hs821MlkHFoQ4HuRPFJt4W/4Uq64qLK7Z1n3EbYxYdIrhQyIgH8XckU4UJBrSZghA"

func TestExtractMemberBzip2Tar(t *testing.T) {
	archive, err := base64.StdEncoding.DecodeString(memberTarBz2)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "member.tar.bz2")
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "testlib.dll")
	if err := extractMember(archivePath, archiveTarBz2, "dir/lib/testlib.dll", dest); err != nil {
		t.Fatalf("extractMember: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "BZ2-LIB" {
		t.Errorf("contents = %q, want %q", got, "BZ2-LIB")
	}
}

// Every configured platform must at least be complete enough to attempt a
// download from its primary and its mirror: no blank URLs, and a cache
// filename for each.
func TestRuntimeAssetsAreWellFormed(t *testing.T) {
	for platform, asset := range runtimeAssets {
		if asset.LocalName == "" {
			t.Errorf("%s: empty LocalName", platform)
		}
		if len(asset.Sources) < 2 {
			t.Errorf("%s: %d sources, want a primary and at least one mirror", platform, len(asset.Sources))
		}
		for i, src := range asset.Sources {
			if src.URL == "" || src.SHA256 == "" || src.Size <= 0 || src.MemberPath == "" || src.Version == "" {
				t.Errorf("%s source %d: incomplete: %+v", platform, i, src)
			}
		}
	}
}
