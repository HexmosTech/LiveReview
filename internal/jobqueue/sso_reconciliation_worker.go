package jobqueue

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/livereview/internal/api/users"
	"github.com/livereview/internal/sso"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
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
	db        *sql.DB
	jwtSecret string
}

func (w *SSOReconciliationWorker) Work(ctx context.Context, job *river.Job[SSOReconciliationJobArgs]) error {
	if w.jwtSecret == "" {
		// Same "disabled, not fatal" treatment server.go gives a missing/empty
		// JWT_SECRET for SSO generally - nothing to reconcile without it. Logged
		// (rather than silently skipped) since an org with reconciliation
		// enabled would otherwise see it silently never run.
		log.Warn().Msg("sso reconciliation: JWT_SECRET not set, skipping sweep")
		return nil
	}
	box, err := sso.NewSecretBox(w.jwtSecret)
	if err != nil {
		log.Warn().Err(err).Msg("sso reconciliation: failed to build secret box, skipping sweep")
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
		log.Warn().Err(err).Int64("org_id", cfg.OrgID).Msg("sso reconciliation: failed to load full OIDC config")
		return
	}
	identities, err := identityStore.ListWithRefreshToken(cfg.OrgID, sso.ProviderOIDC)
	if err != nil {
		log.Warn().Err(err).Int64("org_id", cfg.OrgID).Msg("sso reconciliation: failed to list identities with refresh tokens")
		return
	}
	if len(identities) == 0 {
		if err := configStore.MarkReconciled(cfg.OrgID, sso.ProviderOIDC); err != nil {
			log.Warn().Err(err).Int64("org_id", cfg.OrgID).Msg("sso reconciliation: failed to mark org reconciled (no identities)")
		}
		return
	}

	oauthCfg, _, err := sso.BuildOIDCOAuth2Config(ctx, fullCfg, "")
	if err != nil {
		log.Warn().Err(err).Int64("org_id", cfg.OrgID).Msg("sso reconciliation: failed to build OIDC OAuth2 config")
		return
	}

	for _, identity := range identities {
		_, refreshErr := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: identity.RefreshToken}).Token()
		if refreshErr != nil {
			// The IdP rejected the refresh - invalid_grant/invalid_token is
			// exactly what Okta/Azure/Google return for a disabled, deleted,
			// or de-authorized account. Deactivate, same as a SCIM
			// PATCH active:false would. If the deactivation itself fails,
			// the identity is deliberately left unmarked (not MarkChecked)
			// so it's picked up again next reconciliation cycle instead of
			// silently staying active.
			if err := userService.DeactivateUserInOrgBySCIM(identity.UserID); err != nil {
				log.Error().Err(err).Int64("org_id", cfg.OrgID).Int64("user_id", identity.UserID).
					Msg("sso reconciliation: IdP rejected refresh but deactivating the user failed")
			}
			continue
		}
		if err := identityStore.MarkChecked(identity.ID); err != nil {
			log.Warn().Err(err).Int64("org_id", cfg.OrgID).Int64("identity_id", identity.ID).Msg("sso reconciliation: failed to mark identity checked")
		}
	}
	if err := configStore.MarkReconciled(cfg.OrgID, sso.ProviderOIDC); err != nil {
		log.Warn().Err(err).Int64("org_id", cfg.OrgID).Msg("sso reconciliation: failed to mark org reconciled")
	}
}
