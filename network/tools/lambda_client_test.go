package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"go.uber.org/goleak"
)

func TestInvokeToolWithTimeout_BodyStallAfterHeadersBounded(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() {
		CloseIdleLambdaConnections()
		goleak.VerifyNone(t, baseline)
	})

	stallRelease := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"exit_code":0`))
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		select {
		case <-stallRelease:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(stallRelease)
		srv.Close()
	}()

	origBuilder := buildLambdaInvokeURL
	buildLambdaInvokeURL = func(region, functionName string) string {
		return srv.URL
	}
	defer func() { buildLambdaInvokeURL = origBuilder }()

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("ak", "sk", ""),
	}

	start := time.Now()
	_, err := InvokeToolWithTimeout(context.Background(), cfg, "ruff", []byte(`{"zip_file":"abc"}`), 80*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error when response body stalled after headers, got nil")
	}
	if !IsTransientToolInvokeError(err) {
		t.Fatalf("expected stalled body timeout to be classified transient, got: %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("body stall should have timed out around 80ms, took %v", elapsed)
	}
}
