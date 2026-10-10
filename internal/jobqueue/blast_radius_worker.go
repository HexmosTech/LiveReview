package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/blastradius/repocache"
	"github.com/livereview/internal/blastradius/serverscore"
	"github.com/livereview/internal/blobstore"
	storageblastradius "github.com/livereview/storage/blastradius"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
)

const blastRadiusRoot = repocache.DefaultRoot

// minFreeBytes: below this much free disk a clone could fill the volume Postgres shares.
// ponytail: fixed 1 GB guess; make it relative to the repo's last known size if it misfires.
const minFreeBytes = 1 << 30

// BlastRadiusJobArgs scores one PR review's diff server-side, in parallel with the AI review.
type BlastRadiusJobArgs struct {
	OrgID    int64 `json:"org_id"`
	ReviewID int64 `json:"review_id"`
}

func (BlastRadiusJobArgs) Kind() string { return "blast_radius" }

type BlastRadiusWorker struct {
	river.WorkerDefaults[BlastRadiusJobArgs]
	db *sql.DB
}

// Timeout matches git-lrc's index+score budget; a first index of a big repo takes minutes.
func (w *BlastRadiusWorker) Timeout(job *river.Job[BlastRadiusJobArgs]) time.Duration {
	return 15 * time.Minute
}

// Work is best-effort: a skip or failure is logged and never retried or surfaced on the review.
func (w *BlastRadiusWorker) Work(ctx context.Context, job *river.Job[BlastRadiusJobArgs]) error {
	orgID, reviewID := job.Args.OrgID, job.Args.ReviewID
	logger := log.With().Int64("org_id", orgID).Int64("review_id", reviewID).Logger()
	start := time.Now()

	settings, err := repocache.LoadSettings(ctx, w.db)
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] cannot read settings")
		return nil
	}
	if !settings.Enabled {
		return nil
	}

	repo, skip, err := w.loadRepo(ctx, orgID, reviewID)
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] cannot resolve repo for review")
		return nil
	}
	if skip != "" {
		logger.Info().Str("reason", skip).Msg("[blast_radius] skipped")
		return nil
	}

	if size, ok := repocache.TooLarge(blastRadiusRoot, repo); ok {
		if size > settings.MaxBytes() {
			w.saveSkipped(ctx, orgID, reviewID, "repo_too_large", size, settings)
			return nil
		}
		repocache.ClearTooLarge(blastRadiusRoot, repo) // the cache size was raised since
	}
	if free, err := repocache.FreeBytes(blastRadiusRoot); err == nil && free < minFreeBytes {
		w.saveSkipped(ctx, orgID, reviewID, "low_disk", 0, settings)
		return nil
	}

	checkout, unlock, err := repocache.Prepare(ctx, blastRadiusRoot, repo)
	if errors.Is(err, repocache.ErrBaseTooOld) {
		logger.Info().Msg("[blast_radius] skipped: PR base is older than the cached history window")
		return nil
	}
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] repo checkout failed")
		return nil
	}
	defer unlock()
	// Keep the cache within its size limit on every exit path after the checkout
	// (runs before unlock); a report computed this time is still saved.
	defer func() {
		if size := repocache.EntrySize(blastRadiusRoot, filepath.Dir(checkout.Dir)); size > settings.MaxBytes() {
			logger.Info().Int64("bytes", size).Msg("[blast_radius] repo is bigger than the cache; later reviews will skip it")
			_ = repocache.MarkTooLarge(blastRadiusRoot, checkout, size)
		} else if n := repocache.Evict(blastRadiusRoot, settings.MaxBytes()); n > 0 {
			logger.Info().Int("repos", n).Msg("[blast_radius] evicted least-recently-used repos")
		}
	}()

	diff, err := checkout.Diff(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] git diff failed")
		return nil
	}
	if len(diff) == 0 {
		logger.Info().Msg("[blast_radius] skipped: empty diff")
		return nil
	}

	report, project, err := serverscore.Score(ctx, checkout.Dir, diff)
	if project != "" {
		_ = checkout.RecordIndex(project)
	}
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] scoring failed")
		return nil
	}
	payload, err := json.Marshal(report)
	if err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] encoding report failed")
		return nil
	}
	if err := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius, payload); err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] saving artifact failed")
		return nil
	}
	if err := storageblastradius.NewStore(w.db).ReplaceFromReport(ctx, orgID, reviewID, payload); err != nil {
		logger.Warn().Err(err).Msg("[blast_radius] Postgres replication failed")
	}
	logger.Info().Str("head", checkout.HeadSHA).Dur("took", time.Since(start)).Msg("[blast_radius] report saved")
	return nil
}

// saveSkipped stores a small "skipped" artifact so the review page can say why
// there's no blast radius (and link admins to the cache size setting).
func (w *BlastRadiusWorker) saveSkipped(ctx context.Context, orgID, reviewID int64, reason string, repoBytes int64, s repocache.Settings) {
	payload, _ := json.Marshal(map[string]interface{}{
		"status":  "skipped",
		"reason":  reason,
		"repo_gb": math.Round(float64(repoBytes)/(1<<30)*10) / 10,
		"max_gb":  s.MaxGB,
	})
	if err := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactBlastRadius, payload); err != nil {
		log.Warn().Err(err).Int64("review_id", reviewID).Msg("[blast_radius] saving skipped notice failed")
	}
	log.Info().Int64("review_id", reviewID).Str("reason", reason).Msg("[blast_radius] skipped")
}

// loadRepo builds the repocache.Repo for a review; skip is non-empty when the
// review isn't a PR review blast radius can run on.
func (w *BlastRadiusWorker) loadRepo(ctx context.Context, orgID, reviewID int64) (r repocache.Repo, skip string, err error) {
	var prURL, triggerType, reviewBranch string
	var connectorID, pullRequestID sql.NullInt64
	err = w.db.QueryRowContext(ctx,
		`SELECT COALESCE(pr_mr_url, ''), connector_id, pull_request_id, trigger_type, COALESCE(branch, '') FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID, orgID,
	).Scan(&prURL, &connectorID, &pullRequestID, &triggerType, &reviewBranch)
	if err != nil {
		return r, "", fmt.Errorf("load review: %w", err)
	}
	if triggerType == "cli_diff" {
		return r, "cli review (git-lrc uploads its own report)", nil
	}
	if prURL == "" || !connectorID.Valid {
		return r, "not a PR review", nil
	}

	var provider, providerURL, accessToken, tokenType, patToken string
	err = w.db.QueryRowContext(ctx,
		`SELECT provider, provider_url, COALESCE(access_token, ''), COALESCE(token_type, ''), COALESCE(pat_token, '')
		 FROM integration_tokens WHERE id = $1 AND org_id = $2`,
		connectorID.Int64, orgID,
	).Scan(&provider, &providerURL, &accessToken, &tokenType, &patToken)
	if err != nil {
		return r, "", fmt.Errorf("load connector %d: %w", connectorID.Int64, err)
	}

	cloneURL, prNumber, err := parsePRURL(prURL)
	if err != nil {
		return r, "", err
	}
	// Never send a connector's token to a host other than its own.
	if !sameHost(cloneURL, providerURL) {
		return r, "", fmt.Errorf("PR host does not match connector %d host", connectorID.Int64)
	}

	r = repocache.Repo{
		OrgID:       orgID,
		ConnectorID: connectorID.Int64,
		Provider:    provider,
		CloneURL:    cloneURL,
		PRNumber:    prNumber,
	}
	r.Username, r.Token = gitCredentials(provider, accessToken, tokenType, patToken)

	if pullRequestID.Valid {
		err = w.db.QueryRowContext(ctx,
			`SELECT COALESCE(target_branch, ''), COALESCE(source_branch, '') FROM pull_requests WHERE id = $1 AND org_id = $2`,
			pullRequestID.Int64, orgID,
		).Scan(&r.BaseBranch, &r.SourceBranch)
		if err != nil && err != sql.ErrNoRows {
			return r, "", fmt.Errorf("load pull request: %w", err)
		}
	}
	// No linked PR record: the review row holds the PR's source branch (set before this job is queued).
	if r.SourceBranch == "" {
		r.SourceBranch = reviewBranch
	}
	if strings.HasPrefix(provider, "bitbucket") && r.SourceBranch == "" {
		return r, "bitbucket PR without a known source branch", nil
	}
	return r, "", nil
}

// prURLMarkers split a PR/MR web URL into repo URL + number, per provider.
var prURLMarkers = []string{"/-/merge_requests/", "/pull-requests/", "/pullrequest/", "/pulls/", "/pull/"}

// parsePRURL turns e.g. https://github.com/o/r/pull/12 into (https://github.com/o/r.git, 12).
func parsePRURL(prURL string) (string, int, error) {
	u, err := url.Parse(strings.TrimSpace(prURL))
	if err != nil {
		return "", 0, fmt.Errorf("parse PR URL: %w", err)
	}
	// Use the last marker in the path, so owners/repos named "pull" or "pulls" don't confuse it.
	i, m := -1, ""
	for _, marker := range prURLMarkers {
		if j := strings.LastIndex(u.Path, marker); j > i {
			i, m = j, marker
		}
	}
	if i >= 0 {
		num := strings.SplitN(u.Path[i+len(m):], "/", 2)[0]
		n, err := strconv.Atoi(num)
		if err != nil || n <= 0 {
			return "", 0, fmt.Errorf("no PR number in %q", prURL)
		}
		repoURL := url.URL{Scheme: u.Scheme, Host: u.Host, Path: strings.TrimSuffix(u.Path[:i], "/")}
		clone := repoURL.String()
		if !strings.Contains(u.Path, "/_git/") { // Azure DevOps repo URLs take no .git suffix
			clone += ".git"
		}
		return clone, n, nil
	}
	return "", 0, fmt.Errorf("unrecognised PR URL %q", prURL)
}

// sameHost compares a's host with the connector's provider_url host
// (provider_url may be stored without a scheme).
func sameHost(a, providerURL string) bool {
	b := strings.TrimSpace(providerURL)
	if !strings.Contains(b, "://") {
		b = "https://" + b
	}
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && ua.Host != "" && strings.EqualFold(ua.Host, ub.Host)
}

// gitCredentials picks the same token the review flow uses (see
// internal/api buildProviderConfig): the PAT when present, else the OAuth token.
func gitCredentials(provider, accessToken, tokenType, patToken string) (username, token string) {
	token = accessToken
	if tokenType == "PAT" && patToken != "" {
		token = patToken
	}
	switch {
	case strings.HasPrefix(provider, "gitea"):
		var p struct{ Pat, Username string }
		if json.Unmarshal([]byte(strings.TrimSpace(patToken)), &p) == nil && p.Pat != "" {
			return p.Username, p.Pat
		}
	case strings.HasPrefix(provider, "bitbucket") && tokenType == "PAT":
		// Atlassian API tokens use this fixed git username; OAuth tokens use x-token-auth (repocache default).
		return "x-bitbucket-api-token-auth", token
	}
	return "", token // repocache fills in the provider's fixed username
}
