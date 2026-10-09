package aiconnectors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// mockHTTPError mocks an error that provides a StatusCode() method.
type mockHTTPError struct {
	code int
	msg  string
}

func (e *mockHTTPError) Error() string {
	return e.msg
}

func (e *mockHTTPError) StatusCode() int {
	return e.code
}

func TestCategorizeLLMError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want LLMErrorCategory
	}{
		{
			name: "Nil error",
			err:  nil,
			want: ErrCategoryUnknown,
		},
		{
			name: "Langchain Auth Error",
			err:  &llms.Error{Code: llms.ErrCodeAuthentication},
			want: ErrCategoryAuth,
		},
		{
			name: "Langchain Rate Limit Error",
			err:  &llms.Error{Code: llms.ErrCodeRateLimit},
			want: ErrCategoryOverload,
		},
		{
			name: "HTTP 401 via StatusCode()",
			err:  &mockHTTPError{code: http.StatusUnauthorized, msg: "unauthorized"},
			want: ErrCategoryAuth,
		},
		{
			name: "HTTP 503 via StatusCode()",
			err:  &mockHTTPError{code: http.StatusServiceUnavailable, msg: "service unavailable"},
			want: ErrCategoryOverload,
		},
		{
			name: "Timeout: context deadline",
			err:  fmt.Errorf("operation failed: %w", context.DeadlineExceeded),
			want: ErrCategoryTimeout,
		},
		{
			name: "String Match: high demand",
			err:  errors.New("googleapi: Error 503: This model is currently experiencing high demand."),
			want: ErrCategoryOverload,
		},
		{
			name: "String Match: error 429",
			err:  errors.New("provider returned error 429 too many requests"),
			want: ErrCategoryOverload,
		},
		{
			name: "String Match: model not found",
			err:  errors.New("model not found or deprecated"),
			want: ErrCategoryDeprecated,
		},
		{
			name: "HTTP 404 via StatusCode()",
			err:  &mockHTTPError{code: http.StatusNotFound, msg: "resource not found"},
			want: ErrCategoryUnknown,
		},
		{
			name: "String Match: error 502",
			err:  errors.New("googleapi: error 502: bad gateway"),
			want: ErrCategoryOverload,
		},
		{
			name: "Unknown Error",
			err:  errors.New("something went completely wrong"),
			want: ErrCategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CategorizeLLMError(tt.err)
			if got != tt.want {
				t.Errorf("CategorizeLLMError() = %v, want %v", got, tt.want)
			}
		})
	}
}
