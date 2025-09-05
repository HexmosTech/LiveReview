package prompts

import (
	"context"
	"testing"

	vendorpack "github.com/livereview/internal/prompts/vendor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dummyPack is a minimal Pack that always returns a provided plaintext template.
type dummyPack struct{ tpl string }

func (d dummyPack) List() []vendorpack.TemplateInfo             { return nil }
func (d dummyPack) GetCipher(string, string) ([]byte, error)    { return nil, vendorpack.ErrNotFound }
func (d dummyPack) GetPlaintext(string, string) ([]byte, error) { return []byte(d.tpl), nil }
func (d dummyPack) ActiveBuildID() string                       { return "test" }

// TestRender_HappyPath_UsesVarsOnly ensures Render substitutes provided vars
// without DB access and without requiring real vendor assets.
func TestRender_HappyPath_UsesVarsOnly(t *testing.T) {
	tpl := "Hello {{VAR:name}}!\n\nStyle Guide:\n{{VAR:style_guide|join=\", \"}}\n\nSecurity:\n{{VAR:security_guidelines|default=\"none\"}}\n"
	m := NewManager(nil, dummyPack{tpl: tpl})

	out, err := m.Render(context.Background(), Context{OrgID: 42}, "code_review", map[string]string{
		"name":                "Alice",
		"style_guide":         "Prefer small, focused functions",
		"security_guidelines": "Never log secrets",
	})
	require.NoError(t, err)

	assert.Contains(t, out, "Hello Alice!")
	assert.Contains(t, out, "Style Guide:")
	assert.Contains(t, out, "Prefer small, focused functions")
	assert.Contains(t, out, "Security:")
	assert.Contains(t, out, "Never log secrets")
	assert.NotContains(t, out, "{{VAR:")
}

func TestParsePlaceholders_OptionsParsing(t *testing.T) {
	body := "Intro {{VAR:title|default=\"(untitled)\"}} -- list {{VAR:list|join=\", \"}} -- policy {{VAR:policy|default='be kind\\nrespect'}}"
	phs := ParsePlaceholders(body)
	require.Len(t, phs, 3)

	// title
	assert.Equal(t, "title", phs[0].Name)
	if v, ok := phs[0].Options["default"]; assert.True(t, ok) {
		assert.Equal(t, "(untitled)", v)
	}

	// list joiner
	assert.Equal(t, "list", phs[1].Name)
	if v, ok := phs[1].Options["join"]; assert.True(t, ok) {
		assert.Equal(t, ", ", v)
	}

	// policy default with escaped newline decoded
	assert.Equal(t, "policy", phs[2].Name)
	if v, ok := phs[2].Options["default"]; assert.True(t, ok) {
		assert.Equal(t, "be kind\nrespect", v)
	}
}
