//go:build !production

package jobqueue

import (
	"github.com/livereview/internal/mockllm"
	"github.com/livereview/internal/review"
)

func getMockAIFactory() (review.AIProviderFactory, bool) {
	if mockllm.IsMockAIEnabled() {
		return &mockllm.MockAIProviderFactory{}, true
	}
	return nil, false
}
