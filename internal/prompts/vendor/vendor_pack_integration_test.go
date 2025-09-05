//go:build vendor_prompts

package vendorpack

import (
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

// TestRealVendorPack_DecryptsTemplates validates that with vendor_prompts build tag
// and generated assets present, we can list templates, fetch ciphertext, and JIT
// decrypt plaintext that matches a manual AES-GCM decrypt using the embedded keyring
// and manifest AAD contract.
func TestRealVendorPack_DecryptsTemplates(t *testing.T) {
	// Skip if assets are missing (e.g., developer didn't run the encrypt CLI).
	mf, err := loadManifest()
	if err != nil {
		t.Skipf("vendor assets not present: %v", err)
	}
	if len(mf.Templates) == 0 {
		t.Skip("empty manifest")
	}

	p := New()
	if p.ActiveBuildID() == "" {
		t.Fatal("empty ActiveBuildID")
	}
	listed := p.List()
	if len(listed) == 0 {
		t.Fatal("List returned 0 templates; expected at least 1")
	}

	// Select any one key from the keyring (single active key supported).
	var k []byte
	for _, v := range keyring {
		k = v
		break
	}
	if len(k) != 32 {
		t.Fatalf("invalid key size: %d", len(k))
	}

	for _, ti := range listed {
		// Fetch cipher and plaintext via pack
		ct, err := p.GetCipher(ti.PromptKey, ti.Provider)
		if err != nil {
			t.Fatalf("GetCipher(%s/%s) error: %v", ti.PromptKey, ti.Provider, err)
		}
		pt, err := p.GetPlaintext(ti.PromptKey, ti.Provider)
		if err != nil {
			t.Fatalf("GetPlaintext(%s/%s) error: %v", ti.PromptKey, ti.Provider, err)
		}
		if len(pt) == 0 {
			t.Fatalf("GetPlaintext(%s/%s) returned empty body", ti.PromptKey, ti.Provider)
		}

		// Find the corresponding manifest entry to obtain plaintext checksum for AAD.
		var me *manifestEntry
		for i := range mf.Templates {
			e := &mf.Templates[i]
			if e.PromptKey == ti.PromptKey && (e.Provider == ti.Provider || (ti.Provider == "" && e.Provider == "")) {
				me = e
				break
			}
		}
		if me == nil {
			t.Fatalf("manifest entry not found for %s/%s", ti.PromptKey, ti.Provider)
		}

		// Manual AES-GCM decrypt, should equal pt from pack.
		block, err := aes.NewCipher(k)
		if err != nil {
			t.Fatalf("NewCipher: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			t.Fatalf("NewGCM: %v", err)
		}
		if len(ct) < gcm.NonceSize() {
			t.Fatalf("ciphertext too short: %d", len(ct))
		}
		nonce := ct[:gcm.NonceSize()]
		ciphertext := ct[gcm.NonceSize():]
		aad := ti.PromptKey + "|" + buildID + "|" + me.PlaintextChecksum
		got, err := gcm.Open(nil, nonce, ciphertext, []byte(aad))
		if err != nil {
			t.Fatalf("manual decrypt failed: %v", err)
		}
		if string(got) != string(pt) {
			t.Fatalf("manual decrypt mismatch: got %d bytes, want %d bytes", len(got), len(pt))
		}
	}
}
