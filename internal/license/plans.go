package license

// PlanType represents the subscription plan type
type PlanType string

const (
	PlanFree30K    PlanType = "free_30k"
	PlanTeam32USD  PlanType = "team_32usd"
	PlanFree       PlanType = PlanFree30K
	PlanTeam       PlanType = PlanTeam32USD
	PlanEnterprise PlanType = "enterprise"

	PlanStarter100K PlanType = PlanTeam32USD
	PlanLOC200K     PlanType = "loc_200k"
	PlanLOC400K     PlanType = "loc_400k"
	PlanLOC800K     PlanType = "loc_800k"
	PlanLOC1600K    PlanType = "loc_1600k"
	PlanLOC3200K    PlanType = "loc_3200k"
)

// PlanLimits defines the limits and features for each plan
type PlanLimits struct {
	PlanType         PlanType
	MaxReviewsPerDay int      // -1 for unlimited
	MaxOrganizations int      // -1 for unlimited
	MaxUsers         int      // per org, -1 for unlimited
	MonthlyLOCLimit  int      // -1 for unlimited
	MonthlyPriceUSD  int      // whole USD for now
	TrialDays        int      // 0 means no trial
	Features         []string // list of feature flags
}

// PlanDefinitions maps each plan type to its limits
var PlanDefinitions = map[PlanType]PlanLimits{
	PlanFree30K: {
		PlanType:         PlanFree30K,
		MaxReviewsPerDay: 3,
		MaxOrganizations: 1,
		MaxUsers:         1,
		MonthlyLOCLimit:  30000,
		MonthlyPriceUSD:  0,
		TrialDays:        0,
		Features: []string{
			"basic_review",
			"email_support",
			"byok_required",
		},
	},
	PlanTeam32USD: {
		PlanType:         PlanTeam32USD,
		MaxReviewsPerDay: -1, // unlimited
		MaxOrganizations: -1, // unlimited
		MaxUsers:         -1, // unlimited (based on seats purchased)
		MonthlyLOCLimit:  100000,
		MonthlyPriceUSD:  32,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
		},
	},
	PlanLOC200K: {
		PlanType:         PlanLOC200K,
		MaxReviewsPerDay: -1,
		MaxOrganizations: -1,
		MaxUsers:         -1,
		MonthlyLOCLimit:  200000,
		MonthlyPriceUSD:  64,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
		},
	},
	PlanLOC400K: {
		PlanType:         PlanLOC400K,
		MaxReviewsPerDay: -1,
		MaxOrganizations: -1,
		MaxUsers:         -1,
		MonthlyLOCLimit:  400000,
		MonthlyPriceUSD:  128,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
		},
	},
	PlanLOC800K: {
		PlanType:         PlanLOC800K,
		MaxReviewsPerDay: -1,
		MaxOrganizations: -1,
		MaxUsers:         -1,
		MonthlyLOCLimit:  800000,
		MonthlyPriceUSD:  256,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
		},
	},
	PlanLOC1600K: {
		PlanType:         PlanLOC1600K,
		MaxReviewsPerDay: -1,
		MaxOrganizations: -1,
		MaxUsers:         -1,
		MonthlyLOCLimit:  1600000,
		MonthlyPriceUSD:  512,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
		},
	},
	PlanLOC3200K: {
		PlanType:         PlanLOC3200K,
		MaxReviewsPerDay: -1,
		MaxOrganizations: -1,
		MaxUsers:         -1,
		MonthlyLOCLimit:  3200000,
		MonthlyPriceUSD:  1024,
		TrialDays:        0,
		Features: []string{
			"unlimited_reviews",
			"multiple_orgs",
			"hosted_auto_model",
			"email_support",
			"priority_support",
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
