// Ported from dbctx:internal/embed (BGE-small ONNX embedder + tokenizer)
package docindex

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"

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

	onnxRuntimeVersion = "1.29.0"
	CacheDirEnv        = "DBCTX_CACHE_DIR"
	OnnxRuntimeLibEnv  = "DBCTX_ONNXRUNTIME_LIB"
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
	archiveZip
)

type runtimeAsset struct {
	URL        string
	SHA256     string
	Size       int64
	Kind       archiveKind
	MemberPath string
	LocalName  string
}

const ortRelBase = "https://github.com/microsoft/onnxruntime/releases/download/v" + onnxRuntimeVersion + "/"

var runtimeAssets = map[string]runtimeAsset{
	"linux/amd64": {
		URL:        ortRelBase + "onnxruntime-linux-x64-1.29.0.tgz",
		SHA256:     "c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba",
		Size:       11082880,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-linux-x64-1.29.0/lib/libonnxruntime.so.1.29.0",
		LocalName:  "libonnxruntime.so",
	},
	"linux/arm64": {
		URL:        ortRelBase + "onnxruntime-linux-aarch64-1.29.0.tgz",
		SHA256:     "e1799098ebc054b370f6176a450f158720f297818c613e5dc99b92e2ec82346f",
		Size:       10027600,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-linux-aarch64-1.29.0/lib/libonnxruntime.so.1.29.0",
		LocalName:  "libonnxruntime.so",
	},
	"darwin/arm64": {
		URL:        ortRelBase + "onnxruntime-osx-arm64-1.29.0.tgz",
		SHA256:     "d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92",
		Size:       41578864,
		Kind:       archiveTarGz,
		MemberPath: "onnxruntime-osx-arm64-1.29.0/lib/libonnxruntime.1.29.0.dylib",
		LocalName:  "libonnxruntime.dylib",
	},
	"windows/amd64": {
		URL:        ortRelBase + "onnxruntime-win-x64-1.29.0.zip",
		SHA256:     "c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2",
		Size:       79645520,
		Kind:       archiveZip,
		MemberPath: "onnxruntime-win-x64-1.29.0/lib/onnxruntime.dll",
		LocalName:  "onnxruntime.dll",
	},
	"windows/arm64": {
		URL:        ortRelBase + "onnxruntime-win-arm64-1.29.0.zip",
		SHA256:     "a094a49c3ced0f9fca554647cc7566ae99d93a63a8ce6bf47975561c2de7608e",
		Size:       81679033,
		Kind:       archiveZip,
		MemberPath: "onnxruntime-win-arm64-1.29.0/lib/onnxruntime.dll",
		LocalName:  "onnxruntime.dll",
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

	if !fileMatches(modelPath, modelWeights.Size) {
		if err := downloadToFile(modelWeights.URL, modelPath, modelWeights.SHA256, modelWeights.Size, "bge-small-en-v1.5 model weights"); err != nil {
			return "", "", err
		}
	}
	if !fileMatches(vocabPath, modelVocab.Size) {
		if err := downloadToFile(modelVocab.URL, vocabPath, modelVocab.SHA256, modelVocab.Size, "bge-small-en-v1.5 vocabulary"); err != nil {
			return "", "", err
		}
	}
	return modelPath, vocabPath, nil
}

func EnsureRuntimeLibrary() (string, error) {
	if lib := os.Getenv(OnnxRuntimeLibEnv); lib != "" {
		if !fileExists(lib) {
			return "", fmt.Errorf("%s=%s does not exist", OnnxRuntimeLibEnv, lib)
		}
		return lib, nil
	}

	asset, err := currentPlatformRuntimeAsset()
	if err != nil {
		return "", err
	}
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	libPath := filepath.Join(dir, "onnxruntime", onnxRuntimeVersion, asset.LocalName)
	if fileMatches(libPath, 0) {
		return libPath, nil
	}

	archivePath := filepath.Join(dir, "onnxruntime", onnxRuntimeVersion, filepath.Base(asset.URL))
	if err := downloadToFile(asset.URL, archivePath, asset.SHA256, asset.Size, fmt.Sprintf("onnxruntime %s runtime", onnxRuntimeVersion)); err != nil {
		return "", err
	}
	defer os.Remove(archivePath)

	if err := extractMember(archivePath, asset.Kind, asset.MemberPath, libPath); err != nil {
		return "", fmt.Errorf("extract onnxruntime library: %w", err)
	}
	return libPath, nil
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

	tr := tar.NewReader(gz)
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
