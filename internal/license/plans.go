package license

// PlanType represents the subscription plan type
type PlanType string

const (
	PlanFree       PlanType = "free"
	PlanTeam       PlanType = "team"
	PlanEnterprise PlanType = "enterprise"
)

// PlanLimits defines the limits and features for each plan
type PlanLimits struct {
	PlanType         PlanType
	MaxReviewsPerDay int      // -1 for unlimited
	MaxOrganizations int      // -1 for unlimited
	MaxUsers         int      // per org, -1 for unlimited
	Features         []string // list of feature flags
}

// PlanDefinitions maps each plan type to its limits
var PlanDefinitions = map[PlanType]PlanLimits{
	PlanFree: {
		PlanType:         PlanFree,
		MaxReviewsPerDay: 3,
		MaxOrganizations: 1,
		MaxUsers:         1,
		Features: []string{
			"basic_review",
			"email_support",
		},
	},
	PlanTeam: {
		PlanType:         PlanTeam,
		MaxReviewsPerDay: -1, // unlimited
		MaxOrganizations: -1, // unlimited
		MaxUsers:         -1, // unlimited (based on seats purchased)
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"cloud_ai",
			"email_support",
			"priority_support",
		},
	},
	PlanEnterprise: {
		PlanType:         PlanEnterprise,
		MaxReviewsPerDay: -1, // unlimited
		MaxOrganizations: -1, // unlimited
		MaxUsers:         -1, // unlimited
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"cloud_ai",
			"email_support",
			"priority_support",
			"sso",
			"dedicated_support",
			"custom_integrations",
			"sla",
		},
	},
}

// GetLimits returns the PlanLimits for a given plan type
func (p PlanType) GetLimits() PlanLimits {
	limits, exists := PlanDefinitions[p]
	if !exists {
		// Default to free plan if plan type not found
		return PlanDefinitions[PlanFree]
	}
	return limits
}

// HasFeature checks if the plan includes a specific feature
func (p PlanType) HasFeature(feature string) bool {
	limits := p.GetLimits()
	for _, f := range limits.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsValid checks if the plan type is valid
func (p PlanType) IsValid() bool {
	_, exists := PlanDefinitions[p]
	return exists
}

// String returns the string representation of the plan type
func (p PlanType) String() string {
	return string(p)
}
