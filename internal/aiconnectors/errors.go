package aiconnectors

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

type LLMErrorCategory string

const (
	ErrCategoryAuth       LLMErrorCategory = "auth_failed"
	ErrCategoryOverload   LLMErrorCategory = "overloaded"
	ErrCategoryTimeout    LLMErrorCategory = "timeout"
	ErrCategoryDeprecated LLMErrorCategory = "model_deprecated"
	ErrCategoryUnknown    LLMErrorCategory = "unknown"
)

// CategorizeLLMError maps raw SDK errors into stable UI fallback categories.
func CategorizeLLMError(err error) LLMErrorCategory {
	if err == nil {
		return ErrCategoryUnknown
	}

	// 1. langchaingo normalized errors.
	var e *llms.Error
	if errors.As(err, &e) {
		switch e.Code {
		case llms.ErrCodeAuthentication:
			return ErrCategoryAuth
		case llms.ErrCodeRateLimit, llms.ErrCodeQuotaExceeded, llms.ErrCodeProviderUnavailable:
			return ErrCategoryOverload
		case llms.ErrCodeTimeout, llms.ErrCodeCanceled:
			return ErrCategoryTimeout
		case llms.ErrCodeResourceNotFound:
			if strings.Contains(strings.ToLower(e.Message), "model") {
				return ErrCategoryDeprecated
			}
			return ErrCategoryUnknown
		}
	}

	// 2. Context and network timeouts.
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCategoryTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrCategoryTimeout
	}

	// 3. Raw HTTP status codes from unwrapped provider clients.
	var code int
	var hscErr interface{ HTTPStatusCode() int }
	var scErr interface{ StatusCode() int }
	if errors.As(err, &hscErr) {
		code = hscErr.HTTPStatusCode()
	} else if errors.As(err, &scErr) {
		code = scErr.StatusCode()
	}
	
	switch code {
	case 401, 403:
		return ErrCategoryAuth
	case 429, 500, 502, 503, 504:
		return ErrCategoryOverload
	}

	// 4. String fallback for untyped provider errors.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "deprecated"), strings.Contains(msg, "model not found"),
		strings.Contains(msg, "model_not_found"), strings.Contains(msg, "no such model"),
		strings.Contains(msg, "no longer available"):
		return ErrCategoryDeprecated
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "too many requests"),
		strings.Contains(msg, "service unavailable"), strings.Contains(msg, "bad gateway"),
		strings.Contains(msg, "high demand"), strings.Contains(msg, "quota"):
		return ErrCategoryOverload
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "api key"), strings.Contains(msg, "x-api-key"),
		strings.Contains(msg, "permission denied"), strings.Contains(msg, "do not have access"):
		return ErrCategoryAuth
	}

	return ErrCategoryUnknown
}
