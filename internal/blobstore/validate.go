package blobstore

import (
	"context"
	"errors"
	"fmt"
)

// ValidateConfig normalizes and validates a storage config. An empty Backend
// is treated as the filesystem default, matching OpenBucket's own zero-config
// behavior. It mutates cfg only to fill in that default.
//
// This is the single validation rule set shared by the HTTP settings API
// (internal/api/storage_settings.go) and the `livereview storage ensure` CLI,
// so the two cannot drift.
func ValidateConfig(cfg *Config) error {
	switch cfg.Backend {
	case "":
		cfg.Backend = BackendFilesystem
	case BackendFilesystem:
	case BackendS3:
		if cfg.Bucket == "" {
			return errors.New("bucket is required for the S3-compatible backend")
		}
	case BackendGCS:
		if cfg.Bucket == "" {
			return errors.New("bucket is required for the Google Cloud Storage backend")
		}
	case BackendAzure:
		if cfg.Bucket == "" {
			return errors.New("container name is required for the Azure Blob Storage backend")
		}
		if cfg.AzureAccountName == "" {
			return errors.New("storage account name is required for the Azure Blob Storage backend")
		}
	default:
		return errors.New("unknown storage backend")
	}
	return nil
}

// CheckWritable opens cfg's bucket and round-trips a small marker object
// (write, read, delete) to prove it is reachable and writable. The caller
// controls the deadline via ctx. The HTTP "Test connection" button and
// `livereview storage ensure --test` both use this.
func CheckWritable(ctx context.Context, cfg Config) error {
	bucket, err := OpenBucket(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open bucket: %w", err)
	}
	defer bucket.Close()

	const testKey = "_livereview_storage_settings_test"
	if err := bucket.WriteAll(ctx, testKey, []byte("ok"), nil); err != nil {
		return fmt.Errorf("write test object: %w", err)
	}
	if _, err := bucket.ReadAll(ctx, testKey); err != nil {
		return fmt.Errorf("read test object back: %w", err)
	}
	if err := bucket.Delete(ctx, testKey); err != nil {
		return fmt.Errorf("delete test object: %w", err)
	}
	return nil
}
