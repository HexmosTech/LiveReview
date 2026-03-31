package license

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPlanCatalogIsValid(t *testing.T) {
	catalog := DefaultPlanCatalog()
	if err := ValidatePlanCatalog(catalog); err != nil {
		t.Fatalf("default catalog should be valid: %v", err)
	}
}

func TestValidatePlanCatalogRejectsDuplicateCodes(t *testing.T) {
	catalog := DefaultPlanCatalog()
	catalog.Plans = append(catalog.Plans, catalog.Plans[0])

	err := ValidatePlanCatalog(catalog)
	if err == nil {
		t.Fatal("expected duplicate code validation error")
	}
}

func TestParsePlanCatalogJSON(t *testing.T) {
	jsonText := `{
		"default_plan_code": "free_30k",
		"plans": [
			{
				"code": "free_30k",
				"display_name": "Free 30k",
				"active": true,
				"rank": 0,
				"monthly_price_usd": 0,
				"monthly_loc_limit": 30000,
				"feature_flags": ["basic_review", "byok_required"],
				"trial_policy": {"enabled": false, "days": 0},
				"envelope_visibility": {"show_price": true}
			}
		]
	}`

	catalog, err := ParsePlanCatalogJSON([]byte(jsonText))
	if err != nil {
		t.Fatalf("expected valid parse, got err: %v", err)
	}

	if catalog.DefaultPlanCode != PlanFree30K {
		t.Fatalf("unexpected default plan: %s", catalog.DefaultPlanCode)
	}

	if len(catalog.Plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(catalog.Plans))
	}
}

func TestSyncPlanDefinitionsFromCatalog(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "catalog.json")
	jsonText := `{
		"default_plan_code": "team_32usd",
		"plans": [
			{
				"code": "team_32usd",
				"display_name": "Team 32 USD",
				"active": true,
				"rank": 10,
				"monthly_price_usd": 32,
				"monthly_loc_limit": 100000,
				"feature_flags": ["hosted_auto_model", "usage_envelope_v1", "byok_optional"],
				"trial_policy": {"enabled": false, "days": 0},
				"envelope_visibility": {"show_price": true}
			}
		]
	}`
	if err := os.WriteFile(path, []byte(jsonText), 0644); err != nil {
		t.Fatalf("write temp catalog: %v", err)
	}

	original := PlanDefinitions
	t.Cleanup(func() {
		PlanDefinitions = original
	})

	if err := SyncPlanDefinitionsFromCatalog(path); err != nil {
		t.Fatalf("sync catalog: %v", err)
	}

	limits, ok := PlanDefinitions[PlanTeam32USD]
	if !ok {
		t.Fatalf("expected team plan in definitions")
	}

	if limits.MonthlyLOCLimit != 100000 {
		t.Fatalf("unexpected loc limit: %d", limits.MonthlyLOCLimit)
	}

	if limits.MonthlyPriceUSD != 32 {
		t.Fatalf("unexpected price: %d", limits.MonthlyPriceUSD)
	}
}

func TestSyncPlanDefinitionsFromCatalogMissingFile(t *testing.T) {
	original := PlanDefinitions
	t.Cleanup(func() {
		PlanDefinitions = original
	})

	err := SyncPlanDefinitionsFromCatalog(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing catalog file")
	}
}
