//go:build !vendor_prompts

package prompts

import _vendor "github.com/livereview/internal/prompts/vendor"

func init() {
	// Construct stub pack to trigger its init log; ignore the value.
	_ = _vendor.New()
}
