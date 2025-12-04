// This file exists to work around Go's package/subdirectory limitations.
// All payment-related code lives in internal/license/payment/ but uses package license.
// However, Go doesn't load subdirectory files when you import a package.
//
// To make types and functions from payment/ available when importing "internal/license",
// we need to either:
// 1. Move all files to the root license directory (messy)
// 2. Create a separate package for payment (changes imports everywhere)
// 3. Build the package explicitly to include subdirectories
//
// For now, we build with: go build ./internal/license/...
// This loads all subdirectories as part of the license package.
package license

// This file is intentionally minimal - all actual code is in the payment/ subdirectory.
// The build process must include ./internal/license/... to load payment/ files.
