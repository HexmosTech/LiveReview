//go:build vendor_prompts

package prompts

// PlaintextTemplates returns no templates under vendor builds because
// encrypted vendor assets are used instead. API code that calls this
// will typically only do so as a fallback when the embedded pack has
// no entries.
func PlaintextTemplates() []PlaintextTemplate { return nil }
