//go:build vendor_prompts

package prompts

func fallbackPlaintext(promptKey, provider string) ([]byte, bool) {
	return nil, false
}
