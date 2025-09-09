package license

import "time"

// Status values for license lifecycle.
const (
	StatusMissing = "missing"
	StatusActive  = "active"
	StatusWarning = "warning"
	StatusGrace   = "grace"
	StatusExpired = "expired"
	StatusInvalid = "invalid"
)

// LicenseState represents the persisted singleton record.
type LicenseState struct {
	ID                    int        `db:"id"`
	Token                 *string    `db:"token"`
	Kid                   *string    `db:"kid"`
	Subject               *string    `db:"subject"`
	AppName               *string    `db:"app_name"`
	SeatCount             *int       `db:"seat_count"`
	Unlimited             bool       `db:"unlimited"`
	IssuedAt              *time.Time `db:"issued_at"`
	ExpiresAt             *time.Time `db:"expires_at"`
	LastValidatedAt       *time.Time `db:"last_validated_at"`
	LastValidationErrCode *string    `db:"last_validation_error_code"`
	ValidationFailures    int        `db:"validation_failures"`
	Status                string     `db:"status"`
	GraceStartedAt        *time.Time `db:"grace_started_at"`
	CreatedAt             time.Time  `db:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at"`
}

// Convenience helpers
func (l *LicenseState) IsTerminal() bool {
	return l.Status == StatusExpired || l.Status == StatusInvalid
}

func (l *LicenseState) IsMissing() bool { return l.Status == StatusMissing }
