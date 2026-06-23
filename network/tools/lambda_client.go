package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// InvokeTool calls a Lambda function by its ARN, passing a payload, and returns the response body.
func InvokeTool(ctx context.Context, awsCfg aws.Config, arn string, payload []byte) ([]byte, error) {
	// Parse region from ARN (arn:aws:lambda:us-east-1:12345:function:name)
	parts := strings.Split(arn, ":")
	if len(parts) < 7 {
		return nil, fmt.Errorf("invalid lambda ARN: %s", arn)
	}
	region := parts[3]
	functionName := parts[6]

	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve aws credentials: %w", err)
	}

	urlStr := fmt.Sprintf("https://lambda.%s.amazonaws.com/2015-03-31/functions/%s/invocations", region, functionName)
	req, err := http.NewRequestWithContext(ctx, "POST", urlStr, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	payloadHash := sha256.Sum256(payload)
	payloadHashHex := fmt.Sprintf("%x", payloadHash)

	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, creds, req, payloadHashHex, "lambda", region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	outBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(outBytes))
	}

	if resp.Header.Get("X-Amz-Function-Error") != "" {
		return nil, fmt.Errorf("lambda returned function error: %s", string(outBytes))
	}

	return outBytes, nil
}
