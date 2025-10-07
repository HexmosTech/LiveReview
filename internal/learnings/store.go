package learnings

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	Create(ctx context.Context, l *Learning) error
	Update(ctx context.Context, l *Learning) error
	GetByID(ctx context.Context, id string) (*Learning, error)
	GetByShortID(ctx context.Context, orgID int64, shortID string) (*Learning, error)
	ListByOrg(ctx context.Context, orgID int64) ([]*Learning, error)
	CreateEvent(ctx context.Context, ev *LearningEvent) error
}

// InMemoryStore is a threadsafe in-memory store for tests
type InMemoryStore struct {
	mu       sync.RWMutex
	byID     map[string]*Learning
	byOrg    map[int64][]*Learning
	byOrgSID map[int64]map[string]*Learning
	events   []*LearningEvent
	now      func() time.Time
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		byID:     make(map[string]*Learning),
		byOrg:    make(map[int64][]*Learning),
		byOrgSID: make(map[int64]map[string]*Learning),
		now:      time.Now,
	}
}

func (s *InMemoryStore) Create(ctx context.Context, l *Learning) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.ID == "" {
		l.ID = ShortIDFrom(l.ShortID, l.Title, l.Body) // pseudo unique for tests
	}
	l.CreatedAt = s.now()
	l.UpdatedAt = l.CreatedAt
	s.byID[l.ID] = cloneLearning(l)
	s.byOrg[l.OrgID] = append(s.byOrg[l.OrgID], cloneLearning(l))
	if s.byOrgSID[l.OrgID] == nil {
		s.byOrgSID[l.OrgID] = make(map[string]*Learning)
	}
	s.byOrgSID[l.OrgID][l.ShortID] = cloneLearning(l)
	return nil
}

func (s *InMemoryStore) Update(ctx context.Context, l *Learning) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.byID[l.ID]
	if !ok {
		return ErrNotFound
	}
	l.CreatedAt = old.CreatedAt
	l.UpdatedAt = s.now()
	s.byID[l.ID] = cloneLearning(l)
	// update arrays
	arr := s.byOrg[l.OrgID]
	for i := range arr {
		if arr[i].ID == l.ID {
			arr[i] = cloneLearning(l)
			break
		}
	}
	s.byOrgSID[l.OrgID][l.ShortID] = cloneLearning(l)
	return nil
}

func (s *InMemoryStore) GetByID(ctx context.Context, id string) (*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneLearning(v), nil
}

func (s *InMemoryStore) GetByShortID(ctx context.Context, orgID int64, shortID string) (*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.byOrgSID[orgID]
	if m == nil {
		return nil, ErrNotFound
	}
	v, ok := m[shortID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneLearning(v), nil
}

func (s *InMemoryStore) ListByOrg(ctx context.Context, orgID int64) ([]*Learning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	arr := s.byOrg[orgID]
	out := make([]*Learning, 0, len(arr))
	for _, v := range arr {
		out = append(out, cloneLearning(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *InMemoryStore) CreateEvent(ctx context.Context, ev *LearningEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev.CreatedAt = s.now()
	s.events = append(s.events, ev)
	return nil
}

func cloneLearning(l *Learning) *Learning {
	if l == nil {
		return nil
	}
	cp := *l
	if l.Tags != nil {
		cp.Tags = append([]string(nil), l.Tags...)
	}
	if l.SourceURLs != nil {
		cp.SourceURLs = append([]string(nil), l.SourceURLs...)
	}
	if l.SourceContext != nil {
		sc := *l.SourceContext
		cp.SourceContext = &sc
	}
	if l.Embedding != nil {
		cp.Embedding = append([]byte(nil), l.Embedding...)
	}
	return &cp
}
