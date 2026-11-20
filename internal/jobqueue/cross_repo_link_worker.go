package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
	"github.com/livereview/internal/blobstore"
	"github.com/livereview/internal/githubapp"
	storageblastradius "github.com/livereview/storage/blastradius"
	storagegithub "github.com/livereview/storage/providers/github"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
)

// CrossRepoLinkJobArgs runs codebase-memory-mcp's cross-repo-intelligence
// pass between a review's repo and its configured related repos (see
// docs/multi-repo-blast-radius-plan.md), folding any matched cross-repo
// impacts into the blast-radius report the blast_radius job already saved.
// Queued alongside, never instead of, that job - a review whose repo has no
// related repos configured skips immediately with no CBM calls at all, which
// is the common case.
type CrossRepoLinkJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (CrossRepoLinkJobArgs) Kind() string { return "cross_repo_link" }

type CrossRepoLinkWorker struct {
	river.WorkerDefaults[CrossRepoLinkJobArgs]
	db *sql.DB
}

// Timeout allows for indexing every related repo plus the cross-repo-intelligence pass itself.
func (w *CrossRepoLinkWorker) Timeout(job *river.Job[CrossRepoLinkJobArgs]) time.Duration {
	return 15 * time.Minute
}

// Work is best-effort and never retried: a failure here must never block or
// degrade the review's own blast-radius report, which blast_radius already
// saved on its own.
func (w *CrossRepoLinkWorker) Work(ctx context.Context, job *river.Job[CrossRepoLinkJobArgs]) error {
	orgID, reviewID := job.Args.OrgID, job.Args.ReviewID
	logger := log.With().Int64("org_id", orgID).Int64("review_id", reviewID).Logger()

	repoID, skip, err := w.loadReviewRepositoryID(ctx, orgID, reviewID)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot resolve repository row for review")
		return nil
	}
	if skip != "" {
		return nil // not a PR review on a known repository row - nothing to link
	}

	related, err := w.loadRelatedRepos(ctx, orgID, repoID)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot load related repos")
		return nil
	}
	if len(related) == 0 {
		return nil // the common case: no related repos configured for this repo
	}

	payload, err := blobstore.ReadArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius)
	if err != nil || len(payload) == 0 {
		logger.Warn().Err(err).Msg("[cross_repo_link] no blast-radius report to enrich")
		return nil
	}
	var report blastradius.Report
	if err := json.Unmarshal(payload, &report); err != nil || report.Project == "" {
		logger.Warn().Err(err).Msg("[cross_repo_link] saved report is not a scored report; skipping")
		return nil
	}

	mainRepo, skip, err := loadReviewRepo(ctx, w.db, orgID, reviewID)
	if err != nil || skip != "" {
		logger.Warn().Err(err).Str("skip", skip).Msg("[cross_repo_link] cannot re-resolve main repo checkout")
		return nil
	}
	mainCheckout, mainUnlock, err := repocache.Prepare(ctx, blastRadiusRoot, mainRepo)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] main repo checkout failed")
		return nil
	}
	defer mainUnlock()

	settings, err := repocache.LoadSettings(ctx, w.db)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cannot read settings")
		return nil
	}

	var targetProjects []string
	var unlocks []func()
	defer func() {
		for _, u := range unlocks {
			u()
		}
	}()
	for _, rel := range related {
		relRepo, err := w.loadRepoCredentials(ctx, orgID, rel.id)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] cannot load related repo credentials")
			continue
		}
		if size, ok := repocache.TooLarge(blastRadiusRoot, relRepo); ok && size > settings.MaxBytes() {
			continue
		}
		checkout, unlock, err := repocache.PrepareDefaultBranch(ctx, blastRadiusRoot, relRepo)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] related repo checkout failed")
			continue
		}
		unlocks = append(unlocks, unlock)
		relProject, err := serverscore.IndexOnly(ctx, checkout.Dir)
		if err != nil {
			logger.Warn().Err(err).Int64("related_repo_id", rel.id).Msg("[cross_repo_link] related repo indexing failed")
			continue
		}
		_ = checkout.RecordIndex(relProject)
		targetProjects = append(targetProjects, relProject)
	}
	if len(targetProjects) == 0 {
		logger.Info().Msg("[cross_repo_link] no related repos could be indexed")
		return nil
	}

	if err := serverscore.LinkRelatedProjects(ctx, mainCheckout.Dir, report.Project, targetProjects); err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] cross-repo-intelligence pass failed")
		return nil
	}

	symbolQNs, locate := indexSymbols(&report)
	if len(symbolQNs) == 0 {
		return nil
	}
	impactsBySymbol, err := serverscore.CrossRepoImpactsForSymbols(ctx, report.Project, symbolQNs)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] querying cross-repo edges failed")
		return nil
	}
	if len(impactsBySymbol) == 0 {
		return nil
	}

	changed := false
	for qn, impacts := range impactsBySymbol {
		loc, ok := locate[qn]
		if !ok {
			continue
		}
		if blastradius.RecordCrossRepoImpact(&report, loc.filePath, loc.newStart, loc.newLines, qn, impacts) {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	blastradius.Renormalize(&report)

	enriched, err := json.Marshal(&report)
	if err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] encoding enriched report failed")
		return nil
	}
	if err := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius, enriched); err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] saving enriched report failed")
		return nil
	}
	if err := storageblastradius.NewStore(w.db).ReplaceFromReport(ctx, orgID, reviewID, enriched); err != nil {
		logger.Warn().Err(err).Msg("[cross_repo_link] Postgres replication failed")
	}
	logger.Info().Int("related_repos", len(targetProjects)).Int("impacted_symbols", len(impactsBySymbol)).Msg("[cross_repo_link] report enriched")
	return nil
}

type hunkLocation struct {
	filePath           string
	newStart, newLines int
}

// indexSymbols collects every distinct symbol qualified name touched by
// report and a lookup from qualified name back to the hunk it belongs to
// (the shape RecordCrossRepoImpact needs). A symbol touched by more than one
// hunk keeps only the first hunk seen - good enough for attaching a bonus
// signal, which only needs one place to live.
func indexSymbols(report *blastradius.Report) (qns []string, locate map[string]hunkLocation) {
	locate = make(map[string]hunkLocation)
	seen := make(map[string]bool)
	for _, file := range report.Files {
		for _, hunk := range file.Hunks {
			for _, sym := range hunk.Symbols {
				if sym.QualifiedName == "" || seen[sym.QualifiedName] {
					continue
				}
				seen[sym.QualifiedName] = true
				qns = append(qns, sym.QualifiedName)
				locate[sym.QualifiedName] = hunkLocation{filePath: file.Path, newStart: hunk.NewStart, newLines: hunk.NewLines}
			}
		}
	}
	return qns, locate
}

// loadReviewRepositoryID resolves the repositories row backing a review
// (matched on connector_id + full_name, the same identity reviews.repository
// is populated with - see internal/review_processor/reviews.go). skip is
// non-empty when the review isn't a PR review on a repository LiveReview
// tracks a row for (e.g. a CLI-triggered review, or one from before
// repository sync existed).
func (w *CrossRepoLinkWorker) loadReviewRepositoryID(ctx context.Context, orgID, reviewID int64) (repoID int64, skip string, err error) {
	var repository, triggerType string
	var connectorID sql.NullInt64
	err = w.db.QueryRowContext(ctx,
		`SELECT repository, connector_id, trigger_type FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&repository, &connectorID, &triggerType)
	if err != nil {
		return 0, "", fmt.Errorf("load review: %w", err)
	}
	if triggerType == "cli_diff" || !connectorID.Valid || repository == "" {
		return 0, "not_tracked", nil
	}
	err = w.db.QueryRowContext(ctx,
		`SELECT id FROM repositories WHERE org_id = $1 AND connector_id = $2 AND full_name = $3`,
		orgID, connectorID.Int64, repository,
	).Scan(&repoID)
	if err == sql.ErrNoRows {
		return 0, "not_tracked", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("load repository row: %w", err)
	}
	return repoID, "", nil
}

type relatedRepoRef struct{ id int64 }

// loadRelatedRepos lists every repository related to repoID, read
// symmetrically exactly like ListRelatedRepositories (internal/api/repository_relations.go) -
// a relation stored in either direction counts.
func (w *CrossRepoLinkWorker) loadRelatedRepos(ctx context.Context, orgID, repoID int64) ([]relatedRepoRef, error) {
	rows, err := w.db.QueryContext(ctx, `
		SELECT CASE WHEN repository_id = $1 THEN related_repository_id ELSE repository_id END
		FROM repository_relations
		WHERE org_id = $2 AND (repository_id = $1 OR related_repository_id = $1)`, repoID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []relatedRepoRef
	for rows.Next() {
		var ref relatedRepoRef
		if err := rows.Scan(&ref.id); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// loadRepoCredentials resolves a repocache.Repo for an arbitrary org-scoped
// repository row, not keyed by review like loadReviewRepo - this is the one
// other entry point into "resolve a repo's clone URL + git credentials" that
// cross-repo linking needs, since related repos have no review/PR of their
// own. Token handling mirrors loadReviewRepo exactly (same GitHub App
// refresh, same gitCredentials precedence).
func (w *CrossRepoLinkWorker) loadRepoCredentials(ctx context.Context, orgID, repoID int64) (repocache.Repo, error) {
	var r repocache.Repo
	var connectorID int64
	var provider, cloneURL string
	err := w.db.QueryRowContext(ctx,
		`SELECT connector_id, provider, COALESCE(clone_url, '') FROM repositories WHERE id = $1 AND org_id = $2`,
		repoID, orgID,
	).Scan(&connectorID, &provider, &cloneURL)
	if err != nil {
		return r, fmt.Errorf("load repository %d: %w", repoID, err)
	}
	if cloneURL == "" {
		return r, fmt.Errorf("repository %d has no clone URL", repoID)
	}

	var tokenProvider, providerURL, accessToken, tokenType, patToken string
	err = w.db.QueryRowContext(ctx,
		`SELECT provider, provider_url, COALESCE(access_token, ''), COALESCE(token_type, ''), COALESCE(pat_token, '')
		 FROM integration_tokens WHERE id = $1 AND org_id = $2`,
		connectorID, orgID,
	).Scan(&tokenProvider, &providerURL, &accessToken, &tokenType, &patToken)
	if err != nil {
		return r, fmt.Errorf("load connector %d: %w", connectorID, err)
	}

	r = repocache.Repo{OrgID: orgID, ConnectorID: connectorID, Provider: provider, CloneURL: cloneURL}
	if tokenType == "GITHUB_APP" {
		store := storagegithub.NewTokenStore(w.db)
		rec, recErr := store.GetGitHubTokenByConnectorID(connectorID)
		if recErr != nil || rec == nil || rec.OrgID != orgID {
			return r, fmt.Errorf("load GitHub App connector %d: %v", connectorID, recErr)
		}
		fresh, tokErr := githubapp.ConnectorToken(ctx, store, rec)
		if tokErr != nil {
			return r, fmt.Errorf("GitHub App token for connector %d: %w", connectorID, tokErr)
		}
		patToken = fresh
	}
	r.Username, r.Token = gitCredentials(tokenProvider, accessToken, tokenType, patToken)
	return r, nil
}
