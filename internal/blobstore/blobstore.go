// Package blobstore opens a gocloud.dev/blob.Bucket for whichever backend
// LiveReview is currently configured to use - local filesystem by default,
// or a cloud backend (S3-compatible, covering both real AWS S3 and
// Backblaze B2; Google Cloud Storage; or Azure Blob Storage) when
// configured. Config comes from the system_settings row managed by
// internal/api/storage_settings.go, not from env vars alone, so switching
// backends doesn't require a redeploy.
package blobstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gcstorage "cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/api/option"

	"gocloud.dev/blob"
	"gocloud.dev/blob/azureblob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/gcsblob"
	"gocloud.dev/blob/s3blob"
	"gocloud.dev/gcerrors"
)

// Supported Config.Backend values. An empty Backend is treated as
// BackendFilesystem - the zero-config default for self-hosted instances
// that never touch the storage settings UI.
const (
	BackendFilesystem = "filesystem"
	BackendS3         = "s3"
	BackendGCS        = "gcs"
	BackendAzure      = "azure"
)

// DefaultLocalDir is used when Backend is filesystem and LocalDir is unset.
const DefaultLocalDir = "./data/blobs"

// Config describes where blob data lives. It's the exact shape stored in
// the "blob_storage" system_settings row (see internal/api/storage_settings.go).
type Config struct {
	Backend  string `json:"backend"`
	LocalDir string `json:"local_dir,omitempty"`

	// Bucket names the bucket (S3, GCS) or container (Azure) to use.
	Bucket   string `json:"bucket,omitempty"`
	Endpoint string `json:"endpoint,omitempty"` // custom S3-compatible endpoint (e.g. B2), or a custom Azure service URL; empty = provider default
	Region   string `json:"region,omitempty"`   // S3 only

	AccessKeyID     string `json:"access_key_id,omitempty"`     // S3 only
	SecretAccessKey string `json:"secret_access_key,omitempty"` // S3 only

	// GCSCredentialsJSON is the raw contents of a GCP service-account JSON
	// key. Empty means fall back to Application Default Credentials
	// (workload identity, GOOGLE_APPLICATION_CREDENTIALS, gcloud ADC, etc).
	GCSCredentialsJSON string `json:"gcs_credentials_json,omitempty"`

	// AzureAccountName/AzureAccountKey authenticate via a shared key. If
	// AzureAccountKey is empty, falls back to azidentity's default Azure
	// credential chain (managed identity, env vars, Azure CLI, etc), using
	// AzureAccountName only to build the default service URL.
	AzureAccountName string `json:"azure_account_name,omitempty"`
	AzureAccountKey  string `json:"azure_account_key,omitempty"`
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
	case BackendGCS:
		return openGCSBucket(ctx, cfg)
	case BackendAzure:
		return openAzureBucket(ctx, cfg)
	default:
		return nil, fmt.Errorf("blobstore: unknown backend %q", cfg.Backend)
	}
}

func openFileBucket(cfg Config) (*blob.Bucket, error) {
	dir := cfg.LocalDir
	if dir == "" {
		dir = DefaultLocalDir
	}
	// LocalDir is admin-supplied (internal/api/storage_settings.go); an
	// absolute path is a legitimate way to point at a mounted volume, but a
	// relative path that climbs above where it started (e.g. "../../etc")
	// serves no such purpose and only lets it escape the app's working
	// directory, so that shape is rejected.
	dir = filepath.Clean(dir)
	if dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("blobstore: local_dir must not traverse above its starting directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("blobstore: creating local blob dir %s: %w", dir, err)
	}
	return fileblob.OpenBucket(dir, &fileblob.Options{NoTempDir: true})
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

func openGCSBucket(ctx context.Context, cfg Config) (*blob.Bucket, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blobstore: gcs backend requires a bucket name")
	}

	var clientOpts []option.ClientOption
	if cfg.GCSCredentialsJSON != "" {
		clientOpts = append(clientOpts, option.WithAuthCredentialsJSON(option.ServiceAccount, []byte(cfg.GCSCredentialsJSON)))
	}
	client, err := gcstorage.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("blobstore: creating GCS client: %w", err)
	}
	return gcsblob.OpenBucket(ctx, nil, cfg.Bucket, &gcsblob.Options{Client: client})
}

func openAzureBucket(ctx context.Context, cfg Config) (*blob.Bucket, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("blobstore: azure backend requires a container name")
	}
	if cfg.AzureAccountName == "" {
		return nil, errors.New("blobstore: azure backend requires a storage account name")
	}

	serviceURL := cfg.Endpoint
	if serviceURL == "" {
		serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AzureAccountName)
	}
	containerURL := fmt.Sprintf("%s/%s", serviceURL, cfg.Bucket)

	var client *container.Client
	if cfg.AzureAccountKey != "" {
		cred, err := azblob.NewSharedKeyCredential(cfg.AzureAccountName, cfg.AzureAccountKey)
		if err != nil {
			return nil, fmt.Errorf("blobstore: azure shared key credential: %w", err)
		}
		client, err = container.NewClientWithSharedKeyCredential(containerURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("blobstore: creating azure client: %w", err)
		}
	} else {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("blobstore: azure default credential: %w", err)
		}
		client, err = container.NewClient(containerURL, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("blobstore: creating azure client: %w", err)
		}
	}
	return azureblob.OpenBucket(ctx, client, nil)
}

// IsNotExist reports whether err is a "key not found" error from a Bucket
// operation (ReadAll, Attributes, etc.), regardless of backend.
func IsNotExist(err error) bool {
	return gcerrors.Code(err) == gcerrors.NotFound
}
