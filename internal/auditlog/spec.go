package auditlog

import (
	"sync"

	"github.com/labstack/echo/v4"
)

// CaptureMode controls whether the middleware records a fallback event.
type CaptureMode int

const (
	// CaptureSuccess records a fallback row when the handler returned 2xx but
	// did not emit its own domain event. This is the default.
	CaptureSuccess CaptureMode = iota
	// CaptureAttemptsOnly records only non-2xx requests.
	CaptureAttemptsOnly
	// CaptureAlways records every request, even when a domain event exists.
	CaptureAlways
)

// Spec describes the audit semantics of one route. The zero value means
// "derive everything from the action catalog".
type Spec struct {
	Action          Action
	Category        Category
	ResourceType    string
	ResourceIDParam string
	Severity        Severity
	Sensitive       bool // fail-closed for privileged changes
	SensitiveRead   bool // GET/HEAD responses that expose or export data
	Capture         CaptureMode
	Details         func(c echo.Context) Fields
}

// Resolve fills defaults from the catalog. Unknown actions keep only the
// explicitly supplied fields; ValidateRegistry flags them.
func (s Spec) Resolve() Spec {
	if meta, ok := LookupAction(s.Action); ok {
		if s.Category == "" {
			s.Category = meta.Category
		}
		if s.ResourceType == "" {
			s.ResourceType = meta.ResourceType
		}
		if s.Severity == "" {
			s.Severity = meta.Severity
		}
		if !s.Sensitive && meta.Sensitive {
			s.Sensitive = true
		}
		if !s.SensitiveRead && meta.SensitiveRead {
			s.SensitiveRead = true
		}
	}
	if s.Category == "" {
		s.Category = CategoryOther
	}
	if s.Severity == "" {
		s.Severity = SeverityInfo
	}
	return s
}

// Registry maps "METHOD /route/template" to its Spec. The path is exactly
// what Echo reports in c.Path() and echo.Route.Path.
type Registry struct {
	mu    sync.RWMutex
	specs map[string]Spec
}

func NewRegistry() *Registry {
	return &Registry{specs: make(map[string]Spec)}
}

func registryKey(method, path string) string {
	return method + " " + path
}

// Register adds or replaces a route spec.
func (r *Registry) Register(method, path string, spec Spec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[registryKey(method, path)] = spec.Resolve()
}

// Lookup returns the spec for a method/path pair.
func (r *Registry) Lookup(method, path string) (Spec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specs[registryKey(method, path)]
	return s, ok
}

// All returns a copy of every registered spec.
func (r *Registry) All() map[string]Spec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Spec, len(r.specs))
	for k, v := range r.specs {
		out[k] = v
	}
	return out
}

// ValidateRegistry reports specs whose action/category is not in the catalog.
// This runs at startup in development and as a unit test.
func (r *Registry) ValidateRegistry() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var problems []string
	for key, spec := range r.specs {
		if _, ok := LookupAction(spec.Action); !ok {
			problems = append(problems, key+": unknown action "+string(spec.Action))
		}
		if spec.Category != CategoryOther && !KnownCategory(spec.Category) {
			problems = append(problems, key+": unknown category "+string(spec.Category))
		}
	}
	return problems
}

// RouteCoverageViolation describes a mutating route with no explicit spec.
type RouteCoverageViolation struct {
	Method string
	Path   string
	Reason string
}

// ValidateRouteCoverage reports non-GET /api/v1 routes that have no spec and
// are not on the allowlist. The allowlist is keyed "METHOD /path" with a reason
// string. Existing routes are grandfathered there; new routes must be audited.
func ValidateRouteCoverage(routes []*echo.Route, allowlist map[string]string, reg *Registry) []RouteCoverageViolation {
	var out []RouteCoverageViolation
	for _, route := range routes {
		if route == nil {
			continue
		}
		method := route.Method
		if method == echo.GET || method == echo.HEAD || method == echo.OPTIONS {
			continue
		}
		if len(route.Path) < 8 || route.Path[:8] != "/api/v1/" {
			continue
		}
		key := registryKey(method, route.Path)
		if _, ok := reg.Lookup(method, route.Path); ok {
			continue
		}
		if reason, ok := allowlist[key]; ok && reason != "" {
			continue
		}
		out = append(out, RouteCoverageViolation{
			Method: method,
			Path:   route.Path,
			Reason: "non-GET route has no audit.Spec and no allowlist reason",
		})
	}
	return out
}
