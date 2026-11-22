// Ported from dbctx:internal/embed (BGE-small ONNX embedder + tokenizer)
package docindex

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rs/zerolog/log"
	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/text/unicode/norm"
)

// Constants for BGE-small-en-v1.5 and ONNX Runtime environment
const (
	ModelID                = "bge-small-en-v1.5/onnx-fp32-cls-v1"
	Dims                   = 384
	queryInstructionPrefix = "Represent this sentence for searching relevant passages: "
	maxTokens              = 256
	hfModelBase            = "https://huggingface.co/BAAI/bge-small-en-v1.5/resolve/main"

	inputIDsName      = "input_ids"
	attentionMaskName = "attention_mask"
	tokenTypeIDsName  = "token_type_ids"
	outputHiddenName  = "last_hidden_state"
	batchSize         = 16

	// onnxRuntimeVersion is the release the primary (Microsoft) sources pin.
	onnxRuntimeVersion = "1.29.0"
	// onnxRuntimeMirrorVersion is the release the fallback mirror pins. It is
	// deliberately not tied to onnxRuntimeVersion: the mirror's v1.29.0 assets
	// have no windows/arm64 shared library, and its newer releases are
	// backwards compatible with the C API this process is compiled against.
	onnxRuntimeMirrorVersion = "1.31.0"

	CacheDirEnv       = "DBCTX_CACHE_DIR"
	OnnxRuntimeLibEnv = "DBCTX_ONNXRUNTIME_LIB"
)

type modelAsset struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64
}

var modelWeights = modelAsset{
	Name:   "model.onnx",
	URL:    hfModelBase + "/onnx/model.onnx",
	SHA256: "828e1496d7fabb79cfa4dcd84fa38625c0d3d21da474a00f08db0f559940cf35",
	Size:   133093490,
}

var modelVocab = modelAsset{
	Name:   "vocab.txt",
	URL:    hfModelBase + "/vocab.txt",
	SHA256: "07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3",
	Size:   231508,
}

type archiveKind int

const (
	archiveTarGz archiveKind = iota
	archiveTarBz2
	archiveZip
)

// runtimeSource is one place a prebuilt onnxruntime shared library can be
// fetched from. Each source carries its own pinned checksum and archive layout
// because mirrors repackage the upstream release differently: the official
// Microsoft tarballs keep a version-suffixed library under
// onnxruntime-<platform>-<version>/, while csukuangfj/onnxruntime-libs ships
// plain .zip/.tar.bz2 archives with an unversioned library name and a
// build-configuration suffix in the directory name.
type runtimeSource struct {
	// Version is the onnxruntime release the archive was built from. It also
	// names the cache subdirectory, so a library installed from one source is
	// never mistaken for another version's.
	Version    string
	URL        string
	SHA256     string
	Size       int64
	Kind       archiveKind
	MemberPath string
}

// runtimeAsset is everything needed to provide one platform's shared library,
// with its sources in preference order: primary first, mirrors after.
type runtimeAsset struct {
	LocalName string
	Sources   []runtimeSource
}

const (
	// Primary: Microsoft's own release assets.
	ortRelBase = "https://github.com/microsoft/onnxruntime/releases/download/v" + onnxRuntimeVersion + "/"
	// Fallback: csukuangfj/onnxruntime-libs, an actively maintained rebuild of
	// the same release assets. Kept because microsoft/onnxruntime (and its
	// release downloads) went 404 in late 2026; without this, every fresh
	// install silently loses the ONNX-backed features.
	ortMirrorRelBase = "https://github.com/csukuangfj/onnxruntime-libs/releases/download/v" + onnxRuntimeMirrorVersion + "/"
)

func ortOfficial(archive, member, sha256 string, size int64, kind archiveKind) runtimeSource {
	return runtimeSource{
		Version:    onnxRuntimeVersion,
		URL:        ortRelBase + archive,
		SHA256:     sha256,
		Size:       size,
		Kind:       kind,
		MemberPath: member,
	}
}

func ortMirror(archive, member, sha256 string, size int64, kind archiveKind) runtimeSource {
	return runtimeSource{
		Version:    onnxRuntimeMirrorVersion,
		URL:        ortMirrorRelBase + archive,
		SHA256:     sha256,
		Size:       size,
		Kind:       kind,
		MemberPath: member,
	}
}

var runtimeAssets = map[string]runtimeAsset{
	"linux/amd64": {
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{
			ortOfficial("onnxruntime-linux-x64-1.29.0.tgz",
				"onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0",
				"c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba", 11082880, archiveTarGz),
			ortMirror("onnxruntime-linux-x64-glibc2_17-Release-1.31.0.zip",
				"onnxruntime-linux-x64-glibc2_17-Release-1.31.0/lib/libonnxruntime.so",
				"afaf59e5619372e40b550b562e85dc802be0ea56859cb9ff3d823636a28f5bcc", 11303509, archiveZip),
		},
	},
	"linux/arm64": {
		LocalName: "libonnxruntime.so",
		Sources: []runtimeSource{
			ortOfficial("onnxruntime-linux-aarch64-1.29.0.tgz",
				"onnxruntime-linux-aarch64-1.29.0/lib/libonnxruntime.so.1.29.0",
				"e1799098ebc054b370f6176a450f158720f297818c613e5dc99b92e2ec82346f", 10027600, archiveTarGz),
			ortMirror("onnxruntime-linux-aarch64-glibc2_17-Release-1.31.0.zip",
				"onnxruntime-linux-aarch64-glibc2_17-Release-1.31.0/lib/libonnxruntime.so",
				"f05f18c4d17514ebba8959faf8cf694073183a1efcad05ed76d88dfcbf78f7ce", 14612767, archiveZip),
		},
	},
	"darwin/arm64": {
		LocalName: "libonnxruntime.dylib",
		Sources: []runtimeSource{
			ortOfficial("onnxruntime-osx-arm64-1.29.0.tgz",
				"onnxruntime-osx-arm64-1.29.0/lib/libonnxruntime.1.29.0.dylib",
				"d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92", 41578864, archiveTarGz),
			ortMirror("onnxruntime-osx-arm64-1.31.0.zip",
				"onnxruntime-osx-arm64-1.31.0/lib/libonnxruntime.dylib",
				"60a558189937147849ffc90240f57444198d3f57d7c7cf290e07b2dabd5d90a0", 8838369, archiveZip),
		},
	},
	"windows/amd64": {
		LocalName: "onnxruntime.dll",
		Sources: []runtimeSource{
			ortOfficial("onnxruntime-win-x64-1.29.0.zip",
				"onnxruntime-win-x64-1.29.0/lib/onnxruntime.dll",
				"c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2", 79645520, archiveZip),
			ortMirror("onnxruntime-win-x64-MD-Release-1.31.0.tar.bz2",
				"onnxruntime-win-x64-MD-Release-1.31.0/lib/onnxruntime.dll",
				"14076e3cfa720b0a43a0afbd848904d3d6d33e0881d2997c7296b47a54f3cd9d", 7193503, archiveTarBz2),
		},
	},
	"windows/arm64": {
		LocalName: "onnxruntime.dll",
		Sources: []runtimeSource{
			ortOfficial("onnxruntime-win-arm64-1.29.0.zip",
				"onnxruntime-win-arm64-1.29.0/lib/onnxruntime.dll",
				"a094a49c3ced0f9fca554647cc7566ae99d93a63a8ce6bf47975561c2de7608e", 81679033, archiveZip),
			// The mirror has no dynamic windows/arm64 build for 1.29.0, only
			// static libs; 1.31.0 is the first release that ships one.
			ortMirror("onnxruntime-win-arm64-MD-Release-1.31.0.tar.bz2",
				"onnxruntime-win-arm64-MD-Release-1.31.0/lib/onnxruntime.dll",
				"283eaa9aa28cc9c9583b3b1e9ae117da2ecc64b26012e744d745f7c56e727551", 6950618, archiveTarBz2),
		},
	},
}

func currentPlatformRuntimeAsset() (runtimeAsset, error) {
	key := runtime.GOOS + "/" + runtime.GOARCH
	a, ok := runtimeAssets[key]
	if !ok {
		return runtimeAsset{}, fmt.Errorf(
			"no prebuilt onnxruntime %s available for %s", onnxRuntimeVersion, key)
	}
	return a, nil
}

func CacheDir() (string, error) {
	if v := os.Getenv(CacheDirEnv); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir (set %s to override): %w", CacheDirEnv, err)
	}
	return filepath.Join(home, ".dbctx"), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fileMatches(path string, wantSize int64) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if wantSize > 0 && info.Size() != wantSize {
		return false
	}
	return true
}

func downloadToFile(url, destPath, wantSHA256 string, wantSize int64, label string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".docindex-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer tmp.Close()
	defer os.Remove(tmpPath)

	h := sha256.New()
	total := wantSize
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	var downloaded int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				tmp.Close()
				return fmt.Errorf("write %s: %w", destPath, werr)
			}
			h.Write(buf[:n])
			downloaded += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return fmt.Errorf("download %s: %w", url, rerr)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if wantSize > 0 && downloaded != wantSize {
		return fmt.Errorf("download %s: got %d bytes, want %d", url, downloaded, total)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantSHA256 != "" && got != wantSHA256 {
		return fmt.Errorf("download %s: sha256 mismatch: got %s, want %s", url, got, wantSHA256)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("install %s: %w", destPath, err)
	}
	return nil
}

func EnsureModel() (modelPath, vocabPath string, err error) {
	dir, err := CacheDir()
	if err != nil {
		return "", "", err
	}
	modelDir := filepath.Join(dir, "models", "bge-small-en-v1.5")
	modelPath = filepath.Join(modelDir, modelWeights.Name)
	vocabPath = filepath.Join(modelDir, modelVocab.Name)

	if err := ensureModelAsset(modelWeights, modelPath, "model weights"); err != nil {
		return "", "", err
	}
	if err := ensureModelAsset(modelVocab, vocabPath, "vocabulary"); err != nil {
		return "", "", err
	}
	return modelPath, vocabPath, nil
}

// ensureModelAsset makes sure one pinned model file is present, logging a cache
// hit or the download so a later "model unavailable" symptom is explainable
// from the boot logs alone.
func ensureModelAsset(asset modelAsset, destPath, label string) error {
	if fileMatches(destPath, asset.Size) {
		log.Debug().Str("path", destPath).Int64("bytes", asset.Size).
			Msgf("docindex: using cached %s", label)
		return nil
	}
	log.Info().Str("url", asset.URL).Str("path", destPath).Int64("bytes", asset.Size).
		Msgf("docindex: downloading %s", label)
	start := time.Now()
	if err := downloadToFile(asset.URL, destPath, asset.SHA256, asset.Size, "bge-small-en-v1.5 "+label); err != nil {
		return err
	}
	log.Info().Str("path", destPath).Dur("elapsed_ms", time.Since(start)).
		Msgf("docindex: %s ready", label)
	return nil
}

// runtimeLibMu serializes resolution so the two startup consumers (the
// docindex build and the server's hand-off to dbctx) cannot both download the
// same archive. It is not held across unrelated work.
var runtimeLibMu sync.Mutex

// RuntimeLibrary describes a resolved onnxruntime shared library, including
// where it came from - resolution is logged, but callers that want to show the
// choice in their own startup output can use this instead of re-deriving it.
type RuntimeLibrary struct {
	// Path is the shared library file this process will dlopen.
	Path string
	// Version is the onnxruntime release the library was built from, or empty
	// when Path came from DBCTX_ONNXRUNTIME_LIB and its version is unknown.
	Version string
	// Source identifies where the library came from: the URL of the release
	// archive it was extracted from, or OnnxRuntimeLibEnv for an operator-
	// supplied file.
	Source string
	// Cached is true when no download happened for this call.
	Cached bool
}

// EnsureRuntimeLibrary returns the path of the onnxruntime shared library for
// this platform. See ResolveRuntimeLibrary for the source/preference rules.
func EnsureRuntimeLibrary() (string, error) {
	lib, err := ResolveRuntimeLibrary()
	if err != nil {
		return "", err
	}
	return lib.Path, nil
}

// ResolveRuntimeLibrary returns the onnxruntime shared library for this
// platform, downloading it if necessary. DBCTX_ONNXRUNTIME_LIB always wins;
// otherwise an already-cached copy is preferred over the network, and the
// platform's sources are tried in order until one works. Every choice it makes
// (cache hit, source attempted, download, failure, final path) is logged so a
// later semantic/degradation problem can be traced without re-running
// instrumented builds.
func ResolveRuntimeLibrary() (RuntimeLibrary, error) {
	platform := runtime.GOOS + "/" + runtime.GOARCH

	if lib := os.Getenv(OnnxRuntimeLibEnv); lib != "" {
		if !fileExists(lib) {
			return RuntimeLibrary{}, fmt.Errorf("%s=%s does not exist", OnnxRuntimeLibEnv, lib)
		}
		log.Info().Str("path", lib).Str("platform", platform).Str("env", OnnxRuntimeLibEnv).
			Msg("onnxruntime: using operator-supplied library")
		return RuntimeLibrary{Path: lib, Source: OnnxRuntimeLibEnv, Cached: true}, nil
	}

	runtimeLibMu.Lock()
	defer runtimeLibMu.Unlock()

	asset, err := currentPlatformRuntimeAsset()
	if err != nil {
		return RuntimeLibrary{}, err
	}
	dir, err := CacheDir()
	if err != nil {
		return RuntimeLibrary{}, err
	}
	return ensureRuntimeLibrary(asset, dir)
}

// ensureRuntimeLibrary installs asset's shared library under cacheDir and
// returns it. A copy from any already-downloaded source wins over the network;
// otherwise sources are tried in preference order and the first success is
// returned. A source that fails (404, checksum mismatch, unreadable archive)
// moves on to the next one; the joined error is returned only when every
// source fails.
func ensureRuntimeLibrary(asset runtimeAsset, cacheDir string) (RuntimeLibrary, error) {
	cachedPath := func(src runtimeSource) string {
		return filepath.Join(cacheDir, "onnxruntime", src.Version, asset.LocalName)
	}

	for i, src := range asset.Sources {
		if p := cachedPath(src); fileMatches(p, 0) {
			log.Info().Str("path", p).Str("version", src.Version).Str("source", src.URL).
				Int("source_rank", i+1).Int("source_count", len(asset.Sources)).
				Msg("onnxruntime: using cached runtime library (no download)")
			return RuntimeLibrary{Path: p, Version: src.Version, Source: src.URL, Cached: true}, nil
		}
	}

	errs := make([]error, 0, len(asset.Sources))
	for i, src := range asset.Sources {
		libPath := cachedPath(src)
		log.Info().Str("url", src.URL).Str("version", src.Version).Int64("bytes", src.Size).
			Int("source_rank", i+1).Int("source_count", len(asset.Sources)).
			Bool("is_fallback", i > 0).Str("cache_path", libPath).
			Msg("onnxruntime: fetching runtime library")
		start := time.Now()
		if err := installRuntimeSource(src, libPath); err != nil {
			errs = append(errs, err)
			log.Warn().Err(err).Str("url", src.URL).Int("source_rank", i+1).
				Int("source_count", len(asset.Sources)).Bool("is_fallback", i > 0).
				Msg("onnxruntime: source failed, trying next source")
			continue
		}
		log.Info().Str("path", libPath).Str("version", src.Version).Str("source", src.URL).
			Dur("elapsed", time.Since(start)).Int64("archive_bytes", src.Size).
			Bool("is_fallback", i > 0).
			Msg("onnxruntime: runtime library installed")
		return RuntimeLibrary{Path: libPath, Version: src.Version, Source: src.URL}, nil
	}
	if len(errs) == 0 {
		return RuntimeLibrary{}, fmt.Errorf("no onnxruntime library source configured for %s", runtime.GOOS+"/"+runtime.GOARCH)
	}
	joined := errors.Join(errs...)
	log.Error().Err(joined).Int("source_count", len(asset.Sources)).
		Msg("onnxruntime: every source failed; ONNX-backed features will degrade")
	return RuntimeLibrary{}, fmt.Errorf("install onnxruntime %s runtime: %w", asset.LocalName, joined)
}

// installRuntimeSource downloads one source's archive to the cache, verifies
// it against the source's pinned checksum, and extracts the library member to
// libPath.
func installRuntimeSource(src runtimeSource, libPath string) error {
	if err := os.MkdirAll(filepath.Dir(libPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	archivePath := filepath.Join(filepath.Dir(libPath), filepath.Base(src.URL))
	if err := downloadToFile(src.URL, archivePath, src.SHA256, src.Size,
		fmt.Sprintf("onnxruntime %s runtime", src.Version)); err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := extractMember(archivePath, src.Kind, src.MemberPath, libPath); err != nil {
		return fmt.Errorf("extract onnxruntime library from %s: %w", filepath.Base(src.URL), err)
	}
	log.Debug().Str("archive", filepath.Base(src.URL)).Str("member", src.MemberPath).
		Str("member_sha256", src.SHA256).Msg("onnxruntime: verified and extracted archive member")
	return nil
}

func extractMember(archivePath string, kind archiveKind, member, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	tmpDest := destPath + ".tmp"
	defer os.Remove(tmpDest)

	switch kind {
	case archiveTarGz:
		if err := extractTarGzMember(archivePath, member, tmpDest); err != nil {
			return err
		}
	case archiveTarBz2:
		if err := extractTarBz2Member(archivePath, member, tmpDest); err != nil {
			return err
		}
	case archiveZip:
		if err := extractZipMember(archivePath, member, tmpDest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown archive kind %v", kind)
	}
	return os.Rename(tmpDest, destPath)
}

func extractTarGzMember(archivePath, member, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	return extractTarMember(gz, archivePath, member, destPath)
}

func extractTarBz2Member(archivePath, member, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return extractTarMember(bzip2.NewReader(f), archivePath, member, destPath)
}

func extractTarMember(r io.Reader, archivePath, member, destPath string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("member %q not found in %s", member, archivePath)
		}
		if err != nil {
			return err
		}
		if hdr.Name != member || hdr.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		// Bound the copy by the size the archive declares for this member, so a
		// crafted archive can't stream unbounded data onto disk.
		if _, err := io.CopyN(out, tr, hdr.Size); err != nil && err != io.EOF {
			return err
		}
		return nil
	}
}

func extractZipMember(archivePath, member, destPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != member {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		defer out.Close()
		// Bound the copy by the size the archive declares for this member, so a
		// crafted archive can't stream unbounded data onto disk.
		if _, err := io.CopyN(out, rc, int64(f.UncompressedSize64)); err != nil && err != io.EOF {
			return err
		}
		return nil
	}
	return fmt.Errorf("member %q not found in %s", member, archivePath)
}

// Tokenizer implements WordPiece tokenization for BGE-small-en-v1.5
type Tokenizer struct {
	vocab map[string]int64
	clsID int64
	sepID int64
	padID int64
	unkID int64
}

const (
	tokCLS = "[CLS]"
	tokSEP = "[SEP]"
	tokPAD = "[PAD]"
	tokUNK = "[UNK]"

	maxCharsPerWord = 100
)

func LoadTokenizer(vocabPath string) (*Tokenizer, error) {
	f, err := os.Open(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("open vocab: %w", err)
	}
	defer f.Close()

	vocab := make(map[string]int64)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var id int64
	for sc.Scan() {
		tok := sc.Text()
		if tok == "" {
			id++
			continue
		}
		vocab[tok] = id
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read vocab: %w", err)
	}

	t := &Tokenizer{vocab: vocab}
	var ok bool
	if t.clsID, ok = vocab[tokCLS]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokCLS)
	}
	if t.sepID, ok = vocab[tokSEP]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokSEP)
	}
	if t.padID, ok = vocab[tokPAD]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokPAD)
	}
	if t.unkID, ok = vocab[tokUNK]; !ok {
		return nil, fmt.Errorf("vocab missing %s", tokUNK)
	}
	return t, nil
}

func (t *Tokenizer) Encode(text string) []int64 {
	ids := make([]int64, 0, 32)
	ids = append(ids, t.clsID)
	for _, word := range basicTokenize(text) {
		ids = append(ids, t.wordpiece(word)...)
		if len(ids) >= maxTokens-1 {
			break
		}
	}
	if len(ids) > maxTokens-1 {
		ids = ids[:maxTokens-1]
	}
	ids = append(ids, t.sepID)
	return ids
}

func (t *Tokenizer) EncodeBatch(texts []string) (batch [][]int64, attnMask [][]int64, tokenType [][]int64, seqLen int) {
	all := make([][]int64, len(texts))
	for i, s := range texts {
		all[i] = t.Encode(s)
		if len(all[i]) > seqLen {
			seqLen = len(all[i])
		}
	}
	batch = make([][]int64, len(texts))
	attnMask = make([][]int64, len(texts))
	tokenType = make([][]int64, len(texts))
	for i, ids := range all {
		row := make([]int64, seqLen)
		mask := make([]int64, seqLen)
		tt := make([]int64, seqLen)
		copy(row, ids)
		for j := range ids {
			mask[j] = 1
		}
		for j := len(ids); j < seqLen; j++ {
			row[j] = t.padID
		}
		batch[i], attnMask[i], tokenType[i] = row, mask, tt
	}
	return batch, attnMask, tokenType, seqLen
}

func (t *Tokenizer) wordpiece(word string) []int64 {
	runes := []rune(word)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) > maxCharsPerWord {
		return []int64{t.unkID}
	}

	var out []int64
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if id, ok := t.vocab[sub]; ok {
				out = append(out, id)
				start = end
				found = true
				break
			}
			end--
		}
		if !found {
			return []int64{t.unkID}
		}
	}
	return out
}

func basicTokenize(text string) []string {
	text = strings.ToLower(text)
	text = stripAccents(text)

	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case r == 0 || r == 0xfffd || unicode.Is(unicode.Cc, r):
			continue
		case unicode.IsSpace(r):
			flush()
		case isPunct(r):
			flush()
			tokens = append(tokens, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func isPunct(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func stripAccents(s string) string {
	decomposed := norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var envOnce struct {
	sync.Once
	err error
}

func ensureEnvironment(libPath string) error {
	envOnce.Do(func() {
		// onnxruntime_go's environment is a process-wide singleton (dbctx, run
		// concurrently at startup, may have already initialized it) - reuse it
		// instead of erroring, since InitializeEnvironment refuses a second call.
		if ort.IsInitialized() {
			return
		}
		ort.SetSharedLibraryPath(libPath)
		envOnce.err = ort.InitializeEnvironment()
	})
	return envOnce.err
}

// OnnxEmbedder runs BGE-small-en-v1.5 via ONNX Runtime
type OnnxEmbedder struct {
	mu   sync.Mutex
	tok  *Tokenizer
	sess *ort.DynamicAdvancedSession
}

func NewOnnxEmbedder(libPath, modelPath, vocabPath string) (*OnnxEmbedder, error) {
	if err := ensureEnvironment(libPath); err != nil {
		return nil, fmt.Errorf("initialize onnxruntime environment: %w", err)
	}

	tok, err := LoadTokenizer(vocabPath)
	if err != nil {
		return nil, err
	}

	sess, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{inputIDsName, attentionMaskName, tokenTypeIDsName},
		[]string{outputHiddenName}, nil)
	if err != nil {
		return nil, fmt.Errorf("load onnx model %s: %w", modelPath, err)
	}

	return &OnnxEmbedder{tok: tok, sess: sess}, nil
}

func NewDefaultEmbedder() (*OnnxEmbedder, error) {
	libPath, err := EnsureRuntimeLibrary()
	if err != nil {
		return nil, fmt.Errorf("onnxruntime library: %w", err)
	}
	modelPath, vocabPath, err := EnsureModel()
	if err != nil {
		return nil, fmt.Errorf("model weights: %w", err)
	}
	log.Info().Str("lib", libPath).Str("model", modelPath).Str("vocab", vocabPath).
		Msg("docindex: ONNX embedder ready (BGE-small, " + ModelID + ")")
	return NewOnnxEmbedder(libPath, modelPath, vocabPath)
}

func (e *OnnxEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sess != nil {
		err := e.sess.Destroy()
		e.sess = nil
		return err
	}
	return nil
}

func (e *OnnxEmbedder) Dims() int { return Dims }

func (e *OnnxEmbedder) EmbedPassages(texts []string) ([][]float32, error) {
	return e.embed(texts)
}

func (e *OnnxEmbedder) EmbedQuery(text string) ([]float32, error) {
	vecs, err := e.embed([]string{queryInstructionPrefix + text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *OnnxEmbedder) embed(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := e.embedBatch(texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (e *OnnxEmbedder) embedBatch(texts []string) ([][]float32, error) {
	batch, attnMask, _, seqLen := e.tok.EncodeBatch(texts)
	n := len(texts)

	flatIDs := make([]int64, n*seqLen)
	flatAttn := make([]int64, n*seqLen)
	flatType := make([]int64, n*seqLen)
	for i := 0; i < n; i++ {
		copy(flatIDs[i*seqLen:(i+1)*seqLen], batch[i])
		copy(flatAttn[i*seqLen:(i+1)*seqLen], attnMask[i])
	}

	shape := ort.NewShape(int64(n), int64(seqLen))
	idsT, err := ort.NewTensor(shape, flatIDs)
	if err != nil {
		return nil, fmt.Errorf("build input_ids tensor: %w", err)
	}
	defer idsT.Destroy()
	attnT, err := ort.NewTensor(shape, flatAttn)
	if err != nil {
		return nil, fmt.Errorf("build attention_mask tensor: %w", err)
	}
	defer attnT.Destroy()
	typeT, err := ort.NewTensor(shape, flatType)
	if err != nil {
		return nil, fmt.Errorf("build token_type_ids tensor: %w", err)
	}
	defer typeT.Destroy()

	outShape := ort.NewShape(int64(n), int64(seqLen), int64(Dims))
	outT, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return nil, fmt.Errorf("allocate output tensor: %w", err)
	}
	defer outT.Destroy()

	e.mu.Lock()
	runErr := e.sess.Run([]ort.Value{idsT, attnT, typeT}, []ort.Value{outT})
	e.mu.Unlock()
	if runErr != nil {
		return nil, fmt.Errorf("onnx inference: %w", runErr)
	}

	data := outT.GetData()
	result := make([][]float32, n)
	for i := 0; i < n; i++ {
		vec := make([]float32, Dims)
		off := i * seqLen * Dims
		copy(vec, data[off:off+Dims])
		normalize(vec)
		result[i] = vec
	}
	return result, nil
}

func normalize(vec []float32) {
	var sumSq float64
	for _, v := range vec {
		sumSq += float64(v) * float64(v)
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return
	}
	for i, v := range vec {
		vec[i] = float32(float64(v) / norm)
	}
}
