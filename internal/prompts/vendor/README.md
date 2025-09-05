# Vendor prompts pack

This package is compiled only when built with the `vendor_prompts` tag. Release builds embed encrypted prompt templates and a generated keyring.

Developer workflow:
- Edit plaintext templates in `internal/prompts/registry.go`.
- Run the encrypt tool to generate encrypted assets:
  - outputs `internal/prompts/vendor/templates/*.enc` and `manifest.json`
  - writes `internal/prompts/vendor/keyring_gen.go` with `buildID` and `keyring`
- Build with `-tags vendor_prompts`.

Notes:
- `keyring_gen.go` and `templates/*.enc` are ignored by git; they must be produced by CI for release builds.
- AES-256-GCM is used; AAD includes `prompt_key|build_id|plaintext_checksum`.
- The runtime render path will decrypt JIT and zeroize buffers (implemented in Phase 5).
