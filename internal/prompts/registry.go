package prompts

// PlaintextTemplate is the shape returned for tooling to encrypt vendor templates.
type PlaintextTemplate struct {
	PromptKey string
	Provider  string // optional; empty means default
	Body      string
}

// NOTE: PlaintextTemplates() is provided by registry_stub.go under !vendor_prompts build tag.
