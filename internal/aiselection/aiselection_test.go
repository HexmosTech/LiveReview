package aiselection

import (
	"testing"

	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/license"
)

func TestPaidLeaderChoice(t *testing.T) {
	managed := &aiconnectors.ConnectorRecord{ProviderName: aidefault.ProviderName}
	own := &aiconnectors.ConnectorRecord{ProviderName: "gemini"}

	cases := []struct {
		name       string
		plan       license.PlanType
		connectors []*aiconnectors.ConnectorRecord
		want       leaderKind
	}{
		{"team: LiveReview connector first", license.PlanTeam32USD, []*aiconnectors.ConnectorRecord{managed, own}, leaderManagedConnector},
		{"loc plan: LiveReview connector first is not treated as a key", license.PlanLOC400K, []*aiconnectors.ConnectorRecord{managed, own}, leaderManagedConnector},
		{"loc plan: own key first", license.PlanLOC200K, []*aiconnectors.ConnectorRecord{own, managed}, leaderOwnKey},
		{"team: no connector uses the managed model", license.PlanTeam32USD, nil, leaderManagedDefaultTier},
		{"loc plan: no connector uses the managed model", license.PlanLOC1600K, nil, leaderManagedDefaultTier},
		{"licensed self-hosted: no connector keeps the env config", license.PlanEnterpriseSelfhosted, nil, leaderHostedEnv},
		{"licensed self-hosted: own key", license.PlanEnterpriseSelfhosted, []*aiconnectors.ConnectorRecord{own}, leaderOwnKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, connector := paidLeaderChoice(tc.plan, tc.connectors)
			if got != tc.want {
				t.Fatalf("paidLeaderChoice() = %v, want %v", got, tc.want)
			}
			if (tc.want == leaderOwnKey || tc.want == leaderManagedConnector) && connector != tc.connectors[0] {
				t.Fatalf("expected the first connector to be used")
			}
		})
	}
}
