package api

// LicenseStatusResponse is the API response for current license state.
type LicenseStatusResponse struct {
	Status             string  `json:"status"`
	Subject            *string `json:"subject,omitempty"`
	AppName            *string `json:"appName,omitempty"`
	SeatCount          *int    `json:"seatCount,omitempty"`
	Unlimited          bool    `json:"unlimited"`
	ExpiresAt          *string `json:"expiresAt,omitempty"`
	LastValidatedAt    *string `json:"lastValidatedAt,omitempty"`
	LastValidationCode *string `json:"lastValidationCode,omitempty"`
	// ActiveUsers is the current count of active users in the system (self-hosted mode)
	ActiveUsers *int `json:"activeUsers,omitempty"`
	// AssignedSeats is the count of users with assigned license seats (self-hosted mode)
	AssignedSeats *int `json:"assignedSeats,omitempty"`
}
