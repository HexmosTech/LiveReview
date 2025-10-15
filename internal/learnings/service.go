package learnings

import (
	"context"
	"sort"
	"strings"
	"time"
)

type Service struct {
	store Store
}

func NewService(store Store) *Service { return &Service{store: store} }

// ListActiveByOrg returns all active learnings for the given org without scoping filters.
func (s *Service) ListActiveByOrg(ctx context.Context, orgID int64) ([]*Learning, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	result := make([]*Learning, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.Status != StatusActive {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}

// normalize input for hashing
func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// UpsertFromMetadata dedupes by simhash within org+scope and adds/updates
func (s *Service) UpsertFromMetadata(ctx context.Context, orgID int64, draft Draft, mrCtx *MRContext) (id, shortID string, action string, err error) {
	title := normalizeText(draft.Title)
	body := normalizeText(draft.Body)
	sim := int64(Simhash64(title + " | " + body))

	// find candidate near-duplicates
	items, errList := s.store.ListByOrg(ctx, orgID)
	if errList != nil {
		return "", "", "", errList
	}
	const maxHam = 10
	var best *Learning
	bestHam := 65
	for _, it := range items {
		if it.Scope != draft.Scope {
			continue
		}
		if draft.Scope == ScopeRepo && it.RepoID != draft.RepoID {
			continue
		}
		h := Hamming(uint64(sim), uint64(it.Simhash))
		if h < bestHam {
			best = it
			bestHam = h
		}
	}

	now := time.Now()
	if best != nil && bestHam <= maxHam {
		// update existing
		changed := false
		if len(title) > 0 && title != best.Title {
			best.Title = title
			changed = true
		}
		if len(body) > 0 && body != best.Body {
			best.Body = body
			changed = true
		}
		// merge tags
		if len(draft.Tags) > 0 {
			m := map[string]bool{}
			for _, t := range best.Tags {
				m[strings.ToLower(t)] = true
			}
			for _, t := range draft.Tags {
				if !m[strings.ToLower(t)] {
					best.Tags = append(best.Tags, t)
					m[strings.ToLower(t)] = true
				}
			}
			changed = true
		}
		best.Confidence++
		best.Simhash = sim
		best.UpdatedAt = now
		// attach first source url
		if draft.SourceURL != "" {
			best.SourceURLs = append(best.SourceURLs, draft.SourceURL)
		}
		if best.SourceContext == nil && mrCtx != nil {
			best.SourceContext = &SourceContext{Provider: mrCtx.Provider, Repository: mrCtx.Repository, PRNumber: mrCtx.PRNumber, MRNumber: mrCtx.MRNumber, CommitSHA: mrCtx.CommitSHA, FilePath: mrCtx.FilePath, LineStart: mrCtx.LineStart, LineEnd: mrCtx.LineEnd, ThreadID: mrCtx.ThreadID, CommentID: mrCtx.CommentID}
		}
		if changed {
			if err := s.store.Update(ctx, best); err != nil {
				return "", "", "", err
			}
		}
		_ = s.store.CreateEvent(ctx, &LearningEvent{LearningID: best.ID, OrgID: orgID, Action: EventUpdate, Provider: mrCtx.Provider, ThreadID: mrCtx.ThreadID, CommentID: mrCtx.CommentID, Repository: mrCtx.Repository, CommitSHA: mrCtx.CommitSHA, FilePath: mrCtx.FilePath, LineStart: mrCtx.LineStart, LineEnd: mrCtx.LineEnd, Reason: "upsert-update", Classifier: "nl"})
		return best.ID, best.ShortID, "updated", nil
	}

	// create new
	l := &Learning{
		ID:         "", // store will set
		ShortID:    ShortIDFrom(title, body, time.Now().UTC().Format(time.RFC3339Nano)),
		OrgID:      orgID,
		Scope:      draft.Scope,
		RepoID:     draft.RepoID,
		Title:      title,
		Body:       body,
		Tags:       append([]string(nil), draft.Tags...),
		Status:     StatusActive,
		Confidence: 1,
		Simhash:    sim,
	}
	if draft.SourceURL != "" {
		l.SourceURLs = []string{draft.SourceURL}
	}
	if mrCtx != nil {
		l.SourceContext = &SourceContext{Provider: mrCtx.Provider, Repository: mrCtx.Repository, PRNumber: mrCtx.PRNumber, MRNumber: mrCtx.MRNumber, CommitSHA: mrCtx.CommitSHA, FilePath: mrCtx.FilePath, LineStart: mrCtx.LineStart, LineEnd: mrCtx.LineEnd, ThreadID: mrCtx.ThreadID, CommentID: mrCtx.CommentID}
	}
	if err := s.store.Create(ctx, l); err != nil {
		return "", "", "", err
	}
	_ = s.store.CreateEvent(ctx, &LearningEvent{LearningID: l.ID, OrgID: orgID, Action: EventAdd, Provider: mrCtx.Provider, ThreadID: mrCtx.ThreadID, CommentID: mrCtx.CommentID, Repository: mrCtx.Repository, CommitSHA: mrCtx.CommitSHA, FilePath: mrCtx.FilePath, LineStart: mrCtx.LineStart, LineEnd: mrCtx.LineEnd, Reason: "upsert-add", Classifier: "nl"})
	return l.ID, l.ShortID, "added", nil
}

func (s *Service) UpdateFromMetadata(ctx context.Context, orgID int64, shortID string, deltas Deltas) (id string, action string, err error) {
	l, err := s.store.GetByShortID(ctx, orgID, shortID)
	if err != nil {
		return "", "", err
	}
	changed := false
	if deltas.Title != nil {
		l.Title = normalizeText(*deltas.Title)
		changed = true
	}
	if deltas.Body != nil {
		l.Body = normalizeText(*deltas.Body)
		changed = true
	}
	if deltas.Tags != nil {
		l.Tags = append([]string(nil), *deltas.Tags...)
		changed = true
	}
	if deltas.Scope != nil {
		l.Scope = *deltas.Scope
		changed = true
	}
	if deltas.RepoID != nil {
		l.RepoID = *deltas.RepoID
		changed = true
	}
	if changed {
		l.Simhash = int64(Simhash64(l.Title + " | " + l.Body))
		if err := s.store.Update(ctx, l); err != nil {
			return "", "", err
		}
		_ = s.store.CreateEvent(ctx, &LearningEvent{LearningID: l.ID, OrgID: orgID, Action: EventUpdate, Reason: "manual-edit", Classifier: "nl"})
	}
	return l.ID, "updated", nil
}

func (s *Service) DeleteByShortID(ctx context.Context, orgID int64, shortID string, mrCtx *MRContext) error {
	l, err := s.store.GetByShortID(ctx, orgID, shortID)
	if err != nil {
		return err
	}
	l.Status = StatusArchived
	if err := s.store.Update(ctx, l); err != nil {
		return err
	}
	// Create event with safe access to mrCtx fields
	event := &LearningEvent{
		LearningID: l.ID,
		OrgID:      orgID,
		Action:     EventDelete,
		Reason:     "nl-delete",
		Classifier: "nl",
	}

	// Only set mrCtx fields if mrCtx is not nil
	if mrCtx != nil {
		event.Provider = mrCtx.Provider
		event.ThreadID = mrCtx.ThreadID
		event.CommentID = mrCtx.CommentID
		event.Repository = mrCtx.Repository
		event.CommitSHA = mrCtx.CommitSHA
		event.FilePath = mrCtx.FilePath
		event.LineStart = mrCtx.LineStart
		event.LineEnd = mrCtx.LineEnd
	}

	_ = s.store.CreateEvent(ctx, event)
	return nil
}

type Ranked struct {
	L     *Learning
	Score float64
}

// FetchRelevant ranks org+repo learnings given MR title/desc/file names (compute-only)
func (s *Service) FetchRelevant(ctx context.Context, orgID int64, repoID string, changedFiles []string, title, desc string, limit int) ([]*Learning, error) {
	items, err := s.store.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	title = strings.ToLower(title)
	desc = strings.ToLower(desc)
	// tokenize minimally
	tf := func(text string) map[string]int {
		m := map[string]int{}
		for _, w := range strings.Fields(strings.ToLower(text)) {
			if len(w) < 3 {
				continue
			}
			m[w]++
		}
		return m
	}
	q := tf(title + " " + desc + " " + strings.Join(changedFiles, " "))
	var arr []Ranked
	for _, it := range items {
		if it.Status != StatusActive {
			continue
		}
		// scope gate
		if it.Scope == ScopeRepo && it.RepoID != repoID {
			continue
		}
		// simple score: term overlap in title/body
		text := strings.ToLower(it.Title + " " + it.Body)
		score := 0.0
		for k, v := range q {
			if strings.Contains(text, k) {
				score += float64(v)
			}
		}
		// scope boost
		if it.Scope == ScopeRepo {
			score += 2.0
		}
		// recency/confidence boost
		score += float64(it.Confidence) * 0.2
		if time.Since(it.UpdatedAt) < 30*24*time.Hour {
			score += 0.5
		}
		if score > 0 {
			arr = append(arr, Ranked{L: it, Score: score})
		}
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].Score > arr[j].Score })
	if limit <= 0 || limit > len(arr) {
		limit = len(arr)
	}
	out := make([]*Learning, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, arr[i].L)
	}
	return out, nil
}
