package blobstore

import (
	"context"
	"testing"
)

func TestOpenBucketFilesystemRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	bucket, err := OpenBucket(ctx, Config{Backend: BackendFilesystem, LocalDir: dir})
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}
	defer bucket.Close()

	key := "org/1/review/42/artifacts/blast-radius.json"
	want := []byte(`{"project":"demo"}`)
	if err := bucket.WriteAll(ctx, key, want, nil); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}

	got, err := bucket.ReadAll(ctx, key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("ReadAll = %q, want %q", got, want)
	}
}

func TestOpenBucketFilesystemDefaultsLocalDir(t *testing.T) {
	// Backend left empty entirely - the zero-config default path.
	dir := t.TempDir()
	t.Chdir(dir)

	ctx := context.Background()
	bucket, err := OpenBucket(ctx, Config{})
	if err != nil {
		t.Fatalf("OpenBucket with empty config: %v", err)
	}
	defer bucket.Close()

	if err := bucket.WriteAll(ctx, "k", []byte("v"), nil); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
}

func TestIsNotExist(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	bucket, err := OpenBucket(ctx, Config{Backend: BackendFilesystem, LocalDir: dir})
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}
	defer bucket.Close()

	_, err = bucket.ReadAll(ctx, "does/not/exist.json")
	if err == nil {
		t.Fatal("expected an error reading a missing key")
	}
	if !IsNotExist(err) {
		t.Fatalf("expected IsNotExist(err) to be true for a missing key, got err=%v", err)
	}
}

func TestOpenBucketUnknownBackend(t *testing.T) {
	_, err := OpenBucket(context.Background(), Config{Backend: "carrier-pigeon"})
	if err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}
