package prompts

import (
	"context"
	"errors"
	"sort"
	"strings"

	vendorpack "github.com/livereview/internal/prompts/vendor"
)

// manager implements Manager using a Store and the vendor prompts pack.
type manager struct {
	store *Store
	pack  vendorpack.Pack
}

// NewManager creates a render-capable manager.
func NewManager(store *Store, pack vendorpack.Pack) Manager {
	return &manager{store: store, pack: pack}
}

func (m *manager) GetTemplateDescriptor(promptKey string, provider string) (TemplateDescriptor, error) {
	if provider == "" {
		provider = "default"
	}
	for _, t := range m.pack.List() {
		if t.PromptKey == promptKey && (t.Provider == provider || (provider == "default" && t.Provider == "")) {
			return TemplateDescriptor{PromptKey: t.PromptKey, BuildID: t.BuildID, CipherChecksum: t.CipherChecksum, Provider: t.Provider}, nil
		}
	}
	return TemplateDescriptor{}, errors.New("prompts: template not found")
}

func (m *manager) Render(ctx context.Context, c Context, promptKey string, vars map[string]string) (string, error) {
	// Resolve provider from AIConnector when present
	provider := "default"
	if c.AIConnectorID != nil {
		p, err := m.store.providerFromAIConnector(ctx, *c.AIConnectorID)
		if err == nil && p != "" {
			provider = p
		}
	}

	// We'll resolve application context lazily only if we need to fetch chunks
	var (
		appCtxID       int64
		appCtxResolved bool
	)

	// Load vendor template (plaintext via pack; decrypts JIT when vendor build)
	tplBody, err := m.pack.GetPlaintext(promptKey, provider)
	if err != nil {
		// Fallback for dev/stub builds: use plaintext registry in-repo (not present in vendor builds)
		if errors.Is(err, vendorpack.ErrNotFound) {
			if fb, ok := fallbackPlaintext(promptKey, provider); ok {
				tplBody = fb
				err = nil
			}
		}
		if err != nil {
			return "", err
		}
	}
	// Ensure zeroization of decrypted template bytes after use
	defer func() {
		for i := range tplBody {
			tplBody[i] = 0
		}
	}()

	// Parse placeholders with options
	placeholders := ParsePlaceholders(string(tplBody))

	// Cache resolved var values by name
	resolved := map[string]string{}
	getVar := func(name string, joinSep string, def string) (string, error) {
		if v, ok := vars[name]; ok {
			return v, nil
		}
		if v, ok := resolved[name]; ok {
			return v, nil
		}
		// Resolve application context on first need
		var err error
		if !appCtxResolved {
			appCtxID, err = m.store.ResolveApplicationContext(ctx, c)
			if err != nil {
				return "", err
			}
			appCtxResolved = true
		}
		chunks, err := m.store.ListChunks(ctx, c.OrgID, appCtxID, promptKey, name)
		if err != nil {
			return "", err
		}
		type entry struct {
			seq  int
			id   int64
			body string
		}
		es := make([]entry, 0, len(chunks))
		for _, ch := range chunks {
			if ch.Enabled {
				es = append(es, entry{seq: ch.SequenceIndex, id: ch.ID, body: ch.Body})
			}
		}
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].seq == es[j].seq {
				return es[i].id < es[j].id
			}
			return es[i].seq < es[j].seq
		})
		parts := make([]string, 0, len(es))
		for _, e := range es {
			parts = append(parts, e.body)
		}
		joined := strings.Join(parts, joinSep)
		if joined == "" {
			joined = def
		}
		resolved[name] = joined
		return joined, nil
	}

	// Substitute in order of appearance to honor per-occurrence options
	out := string(tplBody)
	for _, ph := range placeholders {
		joinSep := "\n\n"
		if j, ok := ph.Options["join"]; ok {
			joinSep = j
		}
		def := ""
		if d, ok := ph.Options["default"]; ok {
			def = d
		}
		val, err := getVar(ph.Name, joinSep, def)
		if err != nil {
			return "", err
		}
		out = strings.ReplaceAll(out, ph.Raw, val)
	}
	return out, nil
}

func (m *manager) ResolveApplicationContext(ctx context.Context, c Context) (int64, error) {
	return m.store.ResolveApplicationContext(ctx, c)
}

func (m *manager) ListChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string) ([]Chunk, error) {
	return m.store.ListChunks(ctx, orgID, applicationContextID, promptKey, variableName)
}
func (m *manager) CreateChunk(ctx context.Context, ch Chunk) (int64, error) {
	return m.store.CreateChunk(ctx, ch)
}
func (m *manager) UpdateChunk(ctx context.Context, ch Chunk) error {
	return m.store.UpdateChunk(ctx, ch)
}
func (m *manager) DeleteChunk(ctx context.Context, orgID, chunkID int64) error {
	return m.store.DeleteChunk(ctx, orgID, chunkID)
}
func (m *manager) ReorderChunks(ctx context.Context, orgID int64, applicationContextID int64, promptKey, variableName string, orderedIDs []int64) error {
	return m.store.ReorderChunks(ctx, orgID, applicationContextID, promptKey, variableName, orderedIDs)
}

// --- helpers are implemented in vars.go (ParsePlaceholders)
