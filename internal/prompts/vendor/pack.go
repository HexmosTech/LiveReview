package vendorpack

import "errors"

// TemplateInfo holds non-sensitive metadata for a vendor template.
type TemplateInfo struct {
	PromptKey      string
	BuildID        string
	CipherChecksum string
	Provider       string // optional; empty for default
}

var ErrNotFound = errors.New("vendor: template not found")

// Pack abstracts access to vendor template descriptors and encrypted blobs.
type Pack interface {
	// List returns available template descriptors.
	List() []TemplateInfo
	// GetCipher returns the encrypted blob for the given promptKey and provider.
	GetCipher(promptKey, provider string) ([]byte, error)
	// GetPlaintext returns the decrypted template body for the given promptKey and provider.
	// Implementations must validate integrity using AAD and the manifest.
	GetPlaintext(promptKey, provider string) ([]byte, error)
	// ActiveBuildID returns the build identifier for this pack.
	ActiveBuildID() string
}
