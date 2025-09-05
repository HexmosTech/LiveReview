package prompts

import (
	"bytes"
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
	if m.store != nil && c.AIConnectorID != nil {
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

	// Cache resolved var values by name
	resolved := map[string]string{}
	getVar := func(name string, joinSep string, def string) (string, error) {
		if v, ok := vars[name]; ok {
			return v, nil
		}
		if v, ok := resolved[name]; ok {
			return v, nil
		}
		// If no store is configured, we cannot resolve DB chunks; return default or empty.
		if m.store == nil {
			resolved[name] = def
			return def, nil
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

	// Streaming substitution over tplBody to avoid keeping raw markers in memory.
	// Use regex indices directly on []byte to minimize copying of the decrypted template.
	matches := varPattern.FindAllSubmatchIndex(tplBody, -1)
	var buf bytes.Buffer
	last := 0
	for _, m := range matches {
		// m indices layout: [fullStart, fullEnd, nameStart, nameEnd, optsStart, optsEnd]
		fullStart, fullEnd := m[0], m[1]
		nameStart, nameEnd := m[2], m[3]
		optsStart, optsEnd := -1, -1
		if len(m) >= 6 {
			optsStart, optsEnd = m[4], m[5]
		}

		// Write bytes before this placeholder
		if fullStart > last {
			buf.Write(tplBody[last:fullStart])
		}

		// Extract name and options
		name := string(tplBody[nameStart:nameEnd])
		joinSep := "\n\n"
		def := ""
		if optsStart != -1 && optsEnd != -1 && optsEnd > optsStart {
			optsRaw := tplBody[optsStart:optsEnd]
			sub := optPattern.FindAllSubmatch(optsRaw, -1)
			for _, seg := range sub {
				// seg[1] = key, seg[2] = value
				key := strings.TrimSpace(string(seg[1]))
				val := strings.TrimSpace(string(seg[2]))
				if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
					val = val[1 : len(val)-1]
				}
				val = decodeEscapes(val)
				switch strings.ToLower(key) {
				case "join":
					joinSep = val
				case "default":
					def = val
				}
			}
		}

		// Resolve value and write it
		val, err := getVar(name, joinSep, def)
		if err != nil {
			return "", err
		}
		buf.WriteString(val)
		last = fullEnd
	}
	// Write remaining tail
	if last < len(tplBody) {
		buf.Write(tplBody[last:])
	}
	return buf.String(), nil
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
