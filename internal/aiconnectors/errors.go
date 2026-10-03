package aiconnectors

import (
	"errors"
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

// CategorizeLLMError maps raw SDK errors from Langchain or underlying clients
// into our standard failure categories for consistent UI fallback rendering.
func CategorizeLLMError(err error) LLMErrorCategory {
	if err == nil {
		return ErrCategoryUnknown
	}



	// 1. Langchain-Go normalized errors
	var llmsErr *llms.Error
	if errors.As(err, &llmsErr) {
		if llmsErr.Code == llms.ErrCodeAuthentication || llmsErr.Code == llms.ErrCodeResourceNotFound {
			return ErrCategoryAuth
		}
		if llmsErr.Code == llms.ErrCodeRateLimit {
			return ErrCategoryOverload
		}
	}

	// 2. Generic HTTP status codes
	var scErr interface{ StatusCode() int }
	if errors.As(err, &scErr) {
		code := scErr.StatusCode()
		if code == 401 || code == 403 || code == 404 {
			return ErrCategoryAuth
		}
		if code == 429 || code == 500 || code == 502 || code == 503 || code == 504 {
			return ErrCategoryOverload
		}
	}

	var hscErr interface{ HTTPStatusCode() int }
	if errors.As(err, &hscErr) {
		code := hscErr.HTTPStatusCode()
		if code == 401 || code == 403 || code == 404 {
			return ErrCategoryAuth
		}
		if code == 429 || code == 500 || code == 502 || code == 503 || code == 504 {
			return ErrCategoryOverload
		}
	}

	// 3. Fallback string matching
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "deprecated") || strings.Contains(msg, "model not found") ||
		strings.Contains(msg, "model_not_found") || strings.Contains(msg, "no such model") ||
		strings.Contains(msg, "no longer available") {
		return ErrCategoryDeprecated
	}
	
	if strings.Contains(msg, "context deadline") || strings.Contains(msg, "timeout") {
		return ErrCategoryTimeout
	}
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "status code: 429") || strings.Contains(msg, "status code: 50") ||
		strings.Contains(msg, "error 429") || strings.Contains(msg, "error 50") ||
		strings.Contains(msg, "service unavailable") || strings.Contains(msg, "bad gateway") ||
		strings.Contains(msg, "high demand") {
		return ErrCategoryOverload
	}
	if strings.Contains(msg, "status code: 401") || strings.Contains(msg, "status code: 403") ||
		strings.Contains(msg, "status code: 404") || strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "error 401") || strings.Contains(msg, "error 403") ||
		strings.Contains(msg, "error 404") || strings.Contains(msg, "forbidden") {
		return ErrCategoryAuth
	}

	return ErrCategoryUnknown
}


