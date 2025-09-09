package license

import "errors"

var (
	ErrLicenseMissing = errors.New("license: missing")
	ErrLicenseExpired = errors.New("license: expired")
	ErrLicenseInvalid = errors.New("license: invalid")
)

// NetworkError wraps transient network failures distinct from licence semantic errors.
type NetworkError struct{ Err error }

func (e NetworkError) Error() string { return "network: " + e.Err.Error() }
func (e NetworkError) Unwrap() error { return e.Err }
