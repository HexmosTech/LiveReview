//go:build !vendor_prompts

package vendorpack

import "github.com/rs/zerolog/log"

// Stub pack used for default builds (no encrypted vendor assets present).
type stubPack struct{}

func init() {
	log.Info().Msg("prompts: stub vendor pack active (no encrypted templates)")
}

func New() *stubPack { return &stubPack{} }

func (p *stubPack) List() []TemplateInfo { return []TemplateInfo{} }
func (p *stubPack) GetCipher(promptKey, provider string) ([]byte, error) {
	return nil, ErrNotFound
}
func (p *stubPack) ActiveBuildID() string { return "dev" }
