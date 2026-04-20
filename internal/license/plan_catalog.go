package license

import (
	"encoding/json"
	"fmt"
	"sort"

	storagelicense "github.com/livereview/storage/license"
)

const (
	DefaultPlanCatalogPath = "./config/plan_catalog.json"
)

type PlanTrialPolicy struct {
	Enabled bool `json:"enabled"`
	Days    int  `json:"days"`
}

type PlanEnvelopeVisibility struct {
	ShowPrice bool `json:"show_price"`
}

type PlanCatalogEntry struct {
	Code               PlanType               `json:"code"`
	DisplayName        string                 `json:"display_name"`
	Active             bool                   `json:"active"`
	Rank               int                    `json:"rank"`
	MonthlyPriceUSD    int                    `json:"monthly_price_usd"`
	MonthlyLOCLimit    int                    `json:"monthly_loc_limit"`
	FeatureFlags       []string               `json:"feature_flags"`
	TrialPolicy        PlanTrialPolicy        `json:"trial_policy"`
	EnvelopeVisibility PlanEnvelopeVisibility `json:"envelope_visibility"`
}

type PlanCatalog struct {
	DefaultPlanCode PlanType           `json:"default_plan_code"`
	Plans           []PlanCatalogEntry `json:"plans"`
}

// DefaultPlanCatalog returns the launch catalog with paid starter as default.
func DefaultPlanCatalog() PlanCatalog {
	return PlanCatalog{
		DefaultPlanCode: PlanFree30K,
		Plans: []PlanCatalogEntry{
			{
				Code:            PlanFree30K,
				DisplayName:     "Free 30k",
				Active:          true,
				Rank:            0,
				MonthlyPriceUSD: 0,
				MonthlyLOCLimit: 30000,
				FeatureFlags: []string{
					"basic_review",
					"byok_required",
				},
				TrialPolicy:        PlanTrialPolicy{Enabled: false, Days: 0},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanTeam32USD,
				DisplayName:        "Team 32 USD",
				Active:             true,
				Rank:               10,
				MonthlyPriceUSD:    32,
				MonthlyLOCLimit:    100000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanLOC200K,
				DisplayName:        "LOC 200k",
				Active:             true,
				Rank:               20,
				MonthlyPriceUSD:    64,
				MonthlyLOCLimit:    200000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanLOC400K,
				DisplayName:        "LOC 400k",
				Active:             true,
				Rank:               30,
				MonthlyPriceUSD:    128,
				MonthlyLOCLimit:    400000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanLOC800K,
				DisplayName:        "LOC 800k",
				Active:             true,
				Rank:               40,
				MonthlyPriceUSD:    256,
				MonthlyLOCLimit:    800000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanLOC1600K,
				DisplayName:        "LOC 1.6M",
				Active:             true,
				Rank:               50,
				MonthlyPriceUSD:    512,
				MonthlyLOCLimit:    1600000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
			{
				Code:               PlanLOC3200K,
				DisplayName:        "LOC 3.2M",
				Active:             true,
				Rank:               60,
				MonthlyPriceUSD:    1024,
				MonthlyLOCLimit:    3200000,
				FeatureFlags:       []string{"hosted_auto_model", "usage_envelope_v1", "byok_optional"},
				TrialPolicy:        PlanTrialPolicy{Enabled: true, Days: 7},
				EnvelopeVisibility: PlanEnvelopeVisibility{ShowPrice: true},
			},
		},
	}
}

func LoadPlanCatalogFromFile(path string) (PlanCatalog, error) {
	store := storagelicense.NewPlanCatalogFileStore()
	content, err := store.ReadPlanCatalogFile(path)
	if err != nil {
		return PlanCatalog{}, fmt.Errorf("read plan catalog: %w", err)
	}
	return ParsePlanCatalogJSON(content)
}

func ParsePlanCatalogJSON(content []byte) (PlanCatalog, error) {
	var catalog PlanCatalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return PlanCatalog{}, fmt.Errorf("parse plan catalog: %w", err)
	}
	if err := ValidatePlanCatalog(catalog); err != nil {
		return PlanCatalog{}, err
	}
	return catalog, nil
}

func ValidatePlanCatalog(catalog PlanCatalog) error {
	if len(catalog.Plans) == 0 {
		return fmt.Errorf("plan catalog has no plans")
	}

	seen := make(map[PlanType]struct{}, len(catalog.Plans))
	active := 0
	for _, plan := range catalog.Plans {
		if plan.Code == "" {
			return fmt.Errorf("plan code is required")
		}
		if _, ok := seen[plan.Code]; ok {
			return fmt.Errorf("duplicate plan code: %s", plan.Code)
		}
		seen[plan.Code] = struct{}{}

		if plan.DisplayName == "" {
			return fmt.Errorf("display name is required for plan: %s", plan.Code)
		}
		if plan.MonthlyPriceUSD < 0 {
			return fmt.Errorf("monthly price must be >= 0 for plan: %s", plan.Code)
		}
		if plan.MonthlyLOCLimit < 0 {
			return fmt.Errorf("monthly LOC limit must be >= 0 for plan: %s", plan.Code)
		}
		if plan.Rank < 0 {
			return fmt.Errorf("rank must be >= 0 for plan: %s", plan.Code)
		}
		if plan.TrialPolicy.Enabled && plan.TrialPolicy.Days <= 0 {
			return fmt.Errorf("trial days must be > 0 for plan: %s", plan.Code)
		}
		if !plan.TrialPolicy.Enabled && plan.TrialPolicy.Days < 0 {
			return fmt.Errorf("trial days must be >= 0 for plan: %s", plan.Code)
		}
		if plan.Active {
			active++
		}
	}

	if active == 0 {
		return fmt.Errorf("at least one active plan is required")
	}

	if _, ok := seen[catalog.DefaultPlanCode]; !ok {
		return fmt.Errorf("default plan code not found: %s", catalog.DefaultPlanCode)
	}

	return nil
}

func CatalogIndex(catalog PlanCatalog) map[PlanType]PlanCatalogEntry {
	index := make(map[PlanType]PlanCatalogEntry, len(catalog.Plans))
	for _, plan := range catalog.Plans {
		index[plan.Code] = plan
	}
	return index
}

func ActivePlans(catalog PlanCatalog) []PlanCatalogEntry {
	active := make([]PlanCatalogEntry, 0, len(catalog.Plans))
	for _, plan := range catalog.Plans {
		if plan.Active {
			active = append(active, plan)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		return active[i].Rank < active[j].Rank
	})
	return active
}

// SyncPlanDefinitionsFromCatalog loads catalog file and updates in-memory plan definitions.
// This keeps rollout incremental before DB-backed catalog migrations are introduced.
func SyncPlanDefinitionsFromCatalog(path string) error {
	catalog, err := LoadPlanCatalogFromFile(path)
	if err != nil {
		return err
	}

	updated := make(map[PlanType]PlanLimits, len(catalog.Plans))
	for _, plan := range catalog.Plans {
		updated[plan.Code] = PlanLimits{
			PlanType:         plan.Code,
			MaxReviewsPerDay: -1,
			MaxOrganizations: -1,
			MaxUsers:         -1,
			MonthlyLOCLimit:  plan.MonthlyLOCLimit,
			MonthlyPriceUSD:  plan.MonthlyPriceUSD,
			TrialDays:        plan.TrialPolicy.Days,
			Features:         append([]string(nil), plan.FeatureFlags...),
		}
	}

	PlanDefinitions = updated
	return nil
}
