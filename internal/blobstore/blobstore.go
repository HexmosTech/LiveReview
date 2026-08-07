// Package blobstore opens a gocloud.dev/blob.Bucket for whichever backend
// LiveReview is currently configured to use - local filesystem by default,
// or an S3-compatible bucket (real AWS S3 or Backblaze B2, since B2 speaks
// the S3 API) when configured. Config comes from the system_settings row
// managed by internal/api/storage_settings.go, not from env vars alone, so
// switching backends doesn't require a redeploy.
package blobstore

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/gcerrors"
)

// BackendFilesystem and BackendS3 are the only supported Config.Backend
// values. An empty Backend is treated as BackendFilesystem - the
// zero-config default for self-hosted instances that never touch the
// storage settings UI.
const (
	BackendFilesystem = "filesystem"
	BackendS3         = "s3"
)

// DefaultLocalDir is used when Backend is filesystem and LocalDir is unset.
const DefaultLocalDir = "./data/blobs"

// Config describes where blob data lives. It's the exact shape stored in
// the "blob_storage" system_settings row (see internal/api/storage_settings.go).
type Config struct {
	Backend  string `json:"backend"`
	LocalDir string `json:"local_dir,omitempty"`

	Bucket          string `json:"bucket,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"` // custom S3-compatible endpoint (e.g. B2); empty = real AWS S3
	Region          string `json:"region,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

// OpenBucket opens a *blob.Bucket for cfg. Callers are expected to Close it
// when done; artifact handlers open one per request rather than holding a
// long-lived bucket, matching this codebase's "no caching, fresh read"
// convention for settings (see internal/api/system_settings.go).
func OpenBucket(ctx context.Context, cfg Config) (*blob.Bucket, error) {
	switch cfg.Backend {
	case "", BackendFilesystem:
		return openFileBucket(cfg)
	case BackendS3:
		return openS3Bucket(ctx, cfg)
	default:
		return nil, fmt.Errorf("blobstore: unknown backend %q", cfg.Backend)
	}
}

func openFileBucket(cfg Config) (*blob.Bucket, error) {
	dir := cfg.LocalDir
	if dir == "" {
		dir = DefaultLocalDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("blobstore: creating local blob dir %s: %w", dir, err)
	}
	return fileblob.OpenBucket(dir, nil)
}

func openS3Bucket(ctx context.Context, cfg Config) (*blob.Bucket, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blobstore: s3 backend requires a bucket name")
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("blobstore: loading aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Required by Backblaze B2 and most other S3-compatible
			// services, which don't support AWS's virtual-hosted-style
			// (bucket.endpoint.com) addressing.
			o.UsePathStyle = true
		}
	})
	return s3blob.OpenBucket(ctx, client, cfg.Bucket, nil)
}

// IsNotExist reports whether err is a "key not found" error from a Bucket
// operation (ReadAll, Attributes, etc.), regardless of backend.
func IsNotExist(err error) bool {
	return gcerrors.Code(err) == gcerrors.NotFound
}
