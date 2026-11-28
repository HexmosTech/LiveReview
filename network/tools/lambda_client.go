package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const defaultInvokeTimeout = 90 * time.Second

// lambdaHTTPClient reuses a dedicated transport sized above DefaultMaxConcurrentToolsGlobal
// so concurrent Lambda waves reuse keep-alive TLS connections instead of hitting
// http.DefaultTransport's MaxIdleConnsPerHost=2 limit and re-handshaking TLS on 2-core hosts.
var lambdaHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   64,
	IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// CloseIdleLambdaConnections closes idle connections on the shared Lambda HTTP client
// (primarily used in tests to ensure clean connection/goroutine shutdown).
func CloseIdleLambdaConnections() {
	lambdaHTTPClient.CloseIdleConnections()
}

// invokeLambdaURL is configurable for tests while InvokeToolWithTimeout always targets AWS Lambda.
var buildLambdaInvokeURL = func(region, functionName string) string {
	return fmt.Sprintf("https://lambda.%s.amazonaws.com/2015-03-31/functions/%s/invocations", region, functionName)
}

// ToolInvokeError describes a failed AWS Lambda invocation and whether the failure
// is transient (eligible for in-goroutine retry).
type ToolInvokeError struct {
	StatusCode int
	Transient  bool
	Message    string
	Err        error
}

func (e *ToolInvokeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *ToolInvokeError) Unwrap() error {
	return e.Err
}

// IsTransientToolInvokeError reports whether err represents a transient Lambda
// invocation failure (network error, per-attempt timeout, 429, or 5xx) while the
// caller's parent context is still active.
func IsTransientToolInvokeError(err error) bool {
	if err == nil {
		return false
	}
	var invokeErr *ToolInvokeError
	if errors.As(err, &invokeErr) {
		return invokeErr.Transient
	}
	return false
}

// InvokeTool calls a Lambda function by its ARN or function name using an AWS
// SigV4 signed HTTP request with the default 90s timeout.
func InvokeTool(ctx context.Context, awsCfg aws.Config, arn string, payload []byte) ([]byte, error) {
	return InvokeToolWithTimeout(ctx, awsCfg, arn, payload, defaultInvokeTimeout)
}

// InvokeToolWithTimeout calls a Lambda function using a fresh bytes.NewReader(payload)
// and an explicit timeout per attempt, safely sharing read-only payload bytes across goroutines.
func InvokeToolWithTimeout(ctx context.Context, awsCfg aws.Config, arn string, payload []byte, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = defaultInvokeTimeout
	}

	// Parse region and function name from ARN (arn:aws:lambda:us-east-1:12345:function:name)
	// or plain function name (e.g. ruff-python-linter).
	var region, functionName string
	if strings.HasPrefix(arn, "arn:aws:lambda:") {
		parts := strings.Split(arn, ":")
		if len(parts) < 7 {
			return nil, fmt.Errorf("invalid lambda ARN: %s", arn)
		}
		region = parts[3]
		functionName = parts[6]
	} else if strings.Contains(arn, "arn") || strings.Contains(arn, ":") {
		return nil, fmt.Errorf("invalid lambda ARN: %s", arn)
	} else {
		region = awsCfg.Region
		if region == "" {
			return nil, fmt.Errorf("AWS region is not configured in database (system_settings)")
		}
		functionName = arn
	}

	if awsCfg.Credentials == nil {
		return nil, fmt.Errorf("AWS credentials are not configured in database (system_settings)")
	}
	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve aws credentials: %w", err)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	urlStr := buildLambdaInvokeURL(region, functionName)
	req, err := http.NewRequestWithContext(attemptCtx, "POST", urlStr, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	payloadHash := sha256.Sum256(payload)
	payloadHashHex := fmt.Sprintf("%x", payloadHash)

	signer := v4.NewSigner()
	err = signer.SignHTTP(attemptCtx, creds, req, payloadHashHex, "lambda", region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := lambdaHTTPClient.Do(req)
	if err != nil {
		return nil, &ToolInvokeError{
			Transient: ctx.Err() == nil,
			Message:   "http request failed",
			Err:       err,
		}
	}
	defer resp.Body.Close()

	outBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ToolInvokeError{
			Transient: ctx.Err() == nil,
			Message:   "failed to read response body",
			Err:       err,
		}
	}

	if resp.StatusCode >= 400 {
		transient := (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError) && ctx.Err() == nil
		return nil, &ToolInvokeError{
			StatusCode: resp.StatusCode,
			Transient:  transient,
			Message:    fmt.Sprintf("http %d: %s", resp.StatusCode, string(outBytes)),
		}
	}

	if resp.Header.Get("X-Amz-Function-Error") != "" {
		return nil, &ToolInvokeError{
			Transient: false,
			Message:   fmt.Sprintf("lambda returned function error: %s", string(outBytes)),
		}
	}

	return outBytes, nil
}
