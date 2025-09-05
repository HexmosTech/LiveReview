//go:build !vendor_prompts

package prompts

// fallbackPlaintext returns the plaintext template for a prompt/provider from the dev registry.
// Second return indicates whether a template was found.
func fallbackPlaintext(promptKey, provider string) ([]byte, bool) {
	if provider == "" {
		provider = "default"
	}
	for _, t := range PlaintextTemplates() {
		p := t.Provider
		if p == "" {
			p = "default"
		}
		if t.PromptKey == promptKey && p == provider {
			return []byte(t.Body), true
		}
	}
	return nil, false
}
