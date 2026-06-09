//go:build production

package jobqueue

import "github.com/livereview/internal/review"

func getMockAIFactory() (review.AIProviderFactory, bool) {
	return nil, false
}
