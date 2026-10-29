package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/livereview/internal/api/users"
	"github.com/livereview/internal/sso"
	"github.com/riverqueue/river"
	"golang.org/x/oauth2"
)

// SSOReconciliationJobArgs is a fixed-interval sweep (see jobqueue.go's
// PeriodicJobs) that silently re-checks "is this account still valid at the
// IdP" for every OIDC org that has opted into the reconciliation poll (see
// org_sso_configs.reconciliation_enabled) - a fallback deprovisioning path
// for orgs that configure OIDC login but never wire up SCIM at their IdP.
// Per org/identity work happens inline in one Work() call, the same
// "coordinator does the fan-out itself" shape RiskModelRefitWorker already
// uses, rather than enqueueing a separate one-shot job per org.
type SSOReconciliationJobArgs struct{}

func (SSOReconciliationJobArgs) Kind() string { return "sso_reconciliation" }

type SSOReconciliationWorker struct {
	river.WorkerDefaults[SSOReconciliationJobArgs]
	db *sql.DB
}

func (w *SSOReconciliationWorker) Work(ctx context.Context, job *river.Job[SSOReconciliationJobArgs]) error {
	box, err := sso.NewSecretBox(os.Getenv("JWT_SECRET"))
	if err != nil {
		// Same "disabled, not fatal" treatment server.go gives a missing/empty
		// JWT_SECRET for SSO generally - nothing to reconcile without it.
		return nil
	}
	configStore := sso.NewStore(w.db, box)
	identityStore := sso.NewIdentityStore(w.db, box)
	userService := users.NewUserService(w.db, nil)

	due, err := configStore.ListDueForReconciliation()
	if err != nil {
		return fmt.Errorf("sso reconciliation: listing due configs: %w", err)
	}

	for _, cfg := range due {
		// Best-effort per org: one org's IdP being unreachable shouldn't
		// block the sweep for everyone else.
		w.reconcileOrg(ctx, cfg, configStore, identityStore, userService)
	}
	return nil
}

func (w *SSOReconciliationWorker) reconcileOrg(ctx context.Context, cfg *sso.Config, configStore *sso.Store, identityStore *sso.IdentityStore, userService *users.UserService) {
	fullCfg, err := configStore.Get(cfg.OrgID, sso.ProviderOIDC)
	if err != nil {
		return
	}
	identities, err := identityStore.ListWithRefreshToken(cfg.OrgID, sso.ProviderOIDC)
	if err != nil {
		return
	}
	if len(identities) == 0 {
		_ = configStore.MarkReconciled(cfg.OrgID, sso.ProviderOIDC)
		return
	}

	oauthCfg, _, err := sso.BuildOIDCOAuth2Config(ctx, fullCfg, "")
	if err != nil {
		return
	}

	for _, identity := range identities {
		_, refreshErr := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: identity.RefreshToken}).Token()
		if refreshErr != nil {
			// The IdP rejected the refresh - invalid_grant/invalid_token is
			// exactly what Okta/Azure/Google return for a disabled, deleted,
			// or de-authorized account. Deactivate, same as a SCIM
			// PATCH active:false would.
			_ = userService.DeactivateUserInOrgBySCIM(identity.UserID)
			continue
		}
		_ = identityStore.MarkChecked(identity.ID)
	}
	_ = configStore.MarkReconciled(cfg.OrgID, sso.ProviderOIDC)
}
