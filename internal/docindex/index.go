package docindex

import (
	"context"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
	"sync"

	"github.com/philippgille/chromem-go"
	"github.com/rs/zerolog/log"
)

// DocResult is one retrieved document chunk.
type DocResult struct {
	ID      string
	Title   string
	Content string
	Score   float32
}

// Index wraps a chromem-go Collection and ONNX Embedder for product_guidance RAG.
type Index struct {
	embedder   *OnnxEmbedder
	collection *chromem.Collection
}

// BuildIndex builds an in-memory vector index over markdown files in targetFS.
func BuildIndex(ctx context.Context, targetFS fs.FS, embedder *OnnxEmbedder) (*Index, error) {
	if embedder == nil {
		var err error
		embedder, err = NewDefaultEmbedder()
		if err != nil {
			return nil, fmt.Errorf("create default embedder for docindex: %w", err)
		}
	}

	db := chromem.NewDB()
	embeddingFunc := func(ctx context.Context, text string) ([]float32, error) {
		vecs, err := embedder.EmbedPassages([]string{text})
		if err != nil {
			return nil, err
		}
		return vecs[0], nil
	}

	collection, err := db.CreateCollection("lr_routes", nil, embeddingFunc)
	if err != nil {
		return nil, fmt.Errorf("create chromem collection: %w", err)
	}

	var documents []chromem.Document
	var docCount int

	err = fs.WalkDir(targetFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		contentBytes, err := fs.ReadFile(targetFS, path)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("docindex: failed to read doc file")
			return nil
		}

		content := string(contentBytes)
		if strings.TrimSpace(content) == "" {
			return nil
		}

		// Extract first header line or title if available
		title := path
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimPrefix(line, "# ")
				break
			}
		}

		docID := strings.TrimSuffix(path, ".md")
		documents = append(documents, chromem.Document{
			ID:       docID,
			Content:  content,
			Metadata: map[string]string{"title": title, "path": path},
		})
		docCount++
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk doc index files: %w", err)
	}

	if len(documents) > 0 {
		err = collection.AddDocuments(ctx, documents, runtimeNumCPU())
		if err != nil {
			return nil, fmt.Errorf("add documents to chromem collection: %w", err)
		}
	}

	log.Info().Int("count", docCount).Msg("docindex: product guidance vector index built successfully")

	return &Index{
		embedder:   embedder,
		collection: collection,
	}, nil
}

func runtimeNumCPU() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

// Query searches the index for the top-k most relevant document chunks matching question.
func (idx *Index) Query(ctx context.Context, question string, topK int) ([]DocResult, error) {
	if idx == nil || idx.collection == nil {
		return nil, nil
	}

	qVec, err := idx.embedder.EmbedQuery(question)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	res, err := idx.collection.QueryEmbedding(ctx, qVec, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("query chromem collection: %w", err)
	}

	out := make([]DocResult, 0, len(res))
	for _, doc := range res {
		title := doc.Metadata["title"]
		if title == "" {
			title = doc.ID
		}
		out = append(out, DocResult{
			ID:      doc.ID,
			Title:   title,
			Content: doc.Content,
			Score:   doc.Similarity,
		})
	}

	return out, nil
}

var (
	globalIndex     *Index
	globalIndexOnce sync.Once
	globalIndexErr  error
)

// InitGlobalIndex initializes a lazy global doc index background build over embedFS or disk path.
func InitGlobalIndex(ctx context.Context, docsFS fs.FS) {
	globalIndexOnce.Do(func() {
		go func() {
			idx, err := BuildIndex(context.Background(), docsFS, nil)
			if err != nil {
				log.Error().Err(err).Msg("failed to build global doc index for product guidance")
				globalIndexErr = err
				return
			}
			globalIndex = idx
		}()
	})
}

// GetGlobalIndex returns the globally initialized doc index if ready, or an error if initialization failed.
func GetGlobalIndex() (*Index, error) {
	return globalIndex, globalIndexErr
}
