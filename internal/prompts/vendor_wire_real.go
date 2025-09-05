//go:build vendor_prompts

package prompts

import _vendor "github.com/livereview/internal/prompts/vendor"

func init() {
	// Construct real pack to trigger fail-fast behavior when assets are missing.
	_ = _vendor.New()
}
