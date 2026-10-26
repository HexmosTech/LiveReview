package api

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/blobstore"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"gocloud.dev/blob"
)

const defaultAuditCron = "15 3 * * *"
const auditArchivePrefix = "audit-archive"

// auditMaintenanceLockKey is a process-wide Postgres advisory lock key.
const auditMaintenanceLockKey = 0x4C524155444954 // "LRAUDIT"

// AuditMaintenanceManager runs the daily retention cycle: create future
// partitions, archive months that fell out of the hot window, prune expired
// archives, and write the daily hash anchor.
type AuditMaintenanceManager struct {
	db       *sql.DB
	recorder *auditlog.Recorder
	mu       sync.Mutex
	cfg      AuditSettings
	cron     *cron.Cron
	entryID  cron.EntryID
	ctx      context.Context
	cancel   context.CancelFunc
	running  atomic.Int32
}

func NewAuditMaintenanceManager(db *sql.DB, recorder *auditlog.Recorder) *AuditMaintenanceManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &AuditMaintenanceManager{
		db:       db,
		recorder: recorder,
		cfg:      loadAuditSettings(ctx, db),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start ensures partitions exist before the server accepts traffic, then
// schedules the daily cycle.
func (m *AuditMaintenanceManager) Start() {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()
	if m.recorder != nil {
		m.recorder.SetHashChain(cfg.HashChainEnabled)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := m.ensurePartitions(ctx, 13); err != nil {
		log.Error().Err(err).Msg("audit: failed to ensure monthly partitions; DEFAULT partition will catch inserts")
	}
	cancel()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleLocked(cfg.CronExpression)
	log.Info().Str("schedule", cfg.CronExpression).Int("hot_days", cfg.HotDays).
		Int("retention_days", cfg.RetentionDays).Bool("archive", cfg.ArchiveEnabled).
		Msg("audit: maintenance manager started")
}

func (m *AuditMaintenanceManager) scheduleLocked(expr string) {
	if strings.TrimSpace(expr) == "" {
		expr = defaultAuditCron
	}
	if m.cron != nil {
		m.cron.Stop()
	}
	m.cron = cron.New(cron.WithLocation(time.UTC))
	id, err := m.cron.AddFunc(expr, func() { m.runCycle() })
	if err != nil {
		log.Warn().Err(err).Str("expr", expr).Str("fallback", defaultAuditCron).
			Msg("audit: invalid cron expression, using default")
		id, _ = m.cron.AddFunc(defaultAuditCron, func() { m.runCycle() })
	}
	m.entryID = id
	m.cron.Start()
}

func (m *AuditMaintenanceManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cron != nil {
		ctx := m.cron.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	m.cancel()
}

// Reload applies new settings without a restart.
func (m *AuditMaintenanceManager) Reload() {
	cfg := loadAuditSettings(context.Background(), m.db)
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	if m.recorder != nil {
		m.recorder.SetHashChain(cfg.HashChainEnabled)
	}
	m.mu.Lock()
	m.scheduleLocked(cfg.CronExpression)
	m.mu.Unlock()
	log.Info().Int("hot_days", cfg.HotDays).Int("retention_days", cfg.RetentionDays).
		Bool("archive", cfg.ArchiveEnabled).Msg("audit: settings reloaded")
}

func (m *AuditMaintenanceManager) settings() AuditSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *AuditMaintenanceManager) runCycle() {
	if !m.running.CompareAndSwap(0, 1) {
		log.Warn().Msg("audit: maintenance cycle already running, skipping")
		return
	}
	defer m.running.Store(0)

	cfg := m.settings()
	if !cfg.Enabled {
		log.Info().Msg("audit: maintenance disabled, skipping")
		return
	}
	ctx := m.ctx

	// Cross-instance lock: only one API instance may archive/drop partitions
	// at a time. The in-process atomic guard above only covers this process.
	conn, err := m.db.Conn(ctx)
	if err != nil {
		log.Error().Err(err).Msg("audit: could not open maintenance lock connection")
		return
	}
	defer conn.Close()
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, auditMaintenanceLockKey).Scan(&locked); err != nil {
		log.Error().Err(err).Msg("audit: could not acquire maintenance lock")
		return
	}
	if !locked {
		log.Info().Msg("audit: another instance is running maintenance, skipping")
		return
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, auditMaintenanceLockKey)
	}()

	start := time.Now()
	log.Info().Msg("audit: maintenance cycle start")

	if err := m.ensurePartitions(ctx, 13); err != nil {
		m.recordError(ctx, "ensure_partitions", err)
	}
	if cfg.ArchiveEnabled {
		if err := m.archiveOldPartitions(ctx, cfg); err != nil {
			m.recordError(ctx, "archive", err)
		}
	}
	if cfg.ArchiveEnabled {
		if err := m.archiveDefaultRows(ctx, cfg); err != nil {
			m.recordError(ctx, "archive_default", err)
		}
	}
	if err := m.pruneExpiredArchives(ctx, cfg); err != nil {
		m.recordError(ctx, "prune_archives", err)
	}
	if err := m.pruneDefaultRows(ctx, cfg); err != nil {
		m.recordError(ctx, "prune_default", err)
	}
	if cfg.AnchorEnabled {
		if err := m.writeAnchor(ctx); err != nil {
			m.recordError(ctx, "anchor", err)
		}
	}
	log.Info().Str("elapsed", time.Since(start).Round(time.Millisecond).String()).
		Msg("audit: maintenance cycle done")
}

func (m *AuditMaintenanceManager) recordError(ctx context.Context, stage string, err error) {
	log.Error().Err(err).Str("stage", stage).Msg("audit: maintenance stage failed")
	if m.recorder == nil {
		return
	}
	_ = m.recorder.System(ctx, 0, auditlog.ActionSystemMaintenanceErr,
		auditlog.Details("stage", stage, "error", err.Error()))
}

// ---------------------------------------------------------------------------
// Partitions

type auditPartition struct {
	name string
	from time.Time
	to   time.Time
}

func (m *AuditMaintenanceManager) listPartitions(ctx context.Context) ([]auditPartition, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class p ON p.oid = i.inhparent
		JOIN pg_class c ON c.oid = i.inhrelid
		WHERE p.relname = 'audit_events'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []auditPartition
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		from, to, ok := partitionRange(name)
		if !ok {
			continue
		}
		out = append(out, auditPartition{name: name, from: from, to: to})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].from.Before(out[j].from) })
	return out, rows.Err()
}

func partitionRange(name string) (time.Time, time.Time, bool) {
	const prefix = "audit_events_"
	if !strings.HasPrefix(name, prefix) {
		return time.Time{}, time.Time{}, false
	}
	parsed, err := time.Parse("2006_01", strings.TrimPrefix(name, prefix))
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	from := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.UTC)
	return from, from.AddDate(0, 1, 0), true
}

func (m *AuditMaintenanceManager) ensurePartitions(ctx context.Context, monthsAhead int) error {
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i <= monthsAhead; i++ {
		from := start.AddDate(0, i, 0)
		to := from.AddDate(0, 1, 0)
		name := "audit_events_" + from.Format("2006_01")

		var exists bool
		if err := m.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1 AND relkind = 'r')`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		// Never attempt to attach a partition when the DEFAULT partition already
		// holds rows in that range: PostgreSQL rejects it. The rows remain fully
		// queryable through the parent, and retention still prunes them.
		var defaultRows int64
		if err := m.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM audit_events_default WHERE occurred_at >= $1 AND occurred_at < $2`,
			from, to).Scan(&defaultRows); err != nil {
			return err
		}
		if defaultRows > 0 {
			log.Warn().Str("partition", name).Int64("default_rows", defaultRows).
				Msg("audit: skipping partition creation because DEFAULT already holds rows in this range")
			continue
		}
		// Name and bounds are generated from time, never user input.
		// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
		stmt := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF audit_events FOR VALUES FROM ('%s') TO ('%s')`,
			name, from.Format("2006-01-02"), to.Format("2006-01-02"))
		if _, err := m.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create partition %s: %w", name, err)
		}
		if m.recorder != nil {
			_ = m.recorder.System(ctx, 0, auditlog.ActionSystemPartition,
				auditlog.Details("partition", name))
		}
		log.Info().Str("partition", name).Msg("audit: partition created")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Archive

func (m *AuditMaintenanceManager) archiveOldPartitions(ctx context.Context, cfg AuditSettings) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -cfg.HotDays)
	parts, err := m.listPartitions(ctx)
	if err != nil {
		return err
	}
	bucket, err := blobstore.OpenBucketFromDB(ctx, m.db)
	if err != nil {
		return fmt.Errorf("open blob store: %w", err)
	}
	defer bucket.Close()

	for _, p := range parts {
		if p.to.After(cutoff) {
			continue // still inside the hot window
		}
		if err := m.archivePartition(ctx, bucket, p); err != nil {
			return fmt.Errorf("archive %s: %w", p.name, err)
		}
	}
	return nil
}

func (m *AuditMaintenanceManager) archivePartition(ctx context.Context, bucket *blob.Bucket, p auditPartition) error {
	orgs, err := m.partitionOrgs(ctx, p.name)
	if err != nil {
		return err
	}
	month := p.from.Format("2006-01")
	total := int64(0)
	for _, orgID := range orgs {
		archived, err := m.archiveExists(ctx, orgID, month)
		if err != nil {
			return err
		}
		if archived {
			continue
		}
		count, digest, key, err := m.exportOrgMonth(ctx, bucket, p.name, orgID, month, nil, nil)
		if err != nil {
			return err
		}
		if err := m.recordArchive(ctx, orgID, month, key, count, digest); err != nil {
			return err
		}
		total += count
	}
	// Dropping the partition removes the rows without firing the append-only
	// row trigger. It is only reached after every org's archive upload and
	// audit_archives insert succeeded.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	if _, err := m.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+pqIdent(p.name)); err != nil {
		return fmt.Errorf("drop partition: %w", err)
	}
	if m.recorder != nil {
		_ = m.recorder.System(ctx, 0, auditlog.ActionSystemAuditArchived,
			auditlog.Details("partition", p.name, "month", month, "rows", total, "orgs", len(orgs)))
	}
	log.Info().Str("partition", p.name).Int64("rows", total).Int("orgs", len(orgs)).
		Msg("audit: partition archived and dropped")
	return nil
}

func (m *AuditMaintenanceManager) partitionOrgs(ctx context.Context, partition string) ([]int64, error) {
	// partition is a name discovered from pg_class and is quoted by pqIdent.
	// nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query
	rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT org_id FROM `+pqIdent(partition)+` ORDER BY org_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (m *AuditMaintenanceManager) archiveExists(ctx context.Context, orgID int64, month string) (bool, error) {
	var exists bool
	err := m.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM audit_archives WHERE org_id = $1 AND month = $2)`,
		orgID, month).Scan(&exists)
	return exists, err
}

func (m *AuditMaintenanceManager) exportOrgMonth(ctx context.Context, bucket *blob.Bucket, relation string, orgID int64, month string, from, to *time.Time) (int64, string, string, error) {
	tmp, err := os.CreateTemp("", "audit-archive-*.ndjson.gz")
	if err != nil {
		return 0, "", "", err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	gz := gzip.NewWriter(tmp)
	digest := sha256.New()
	multi := io.MultiWriter(gz, digest)
	enc := json.NewEncoder(multi)

	query := `SELECT ` + auditlog.EventColumns + ` FROM ` + pqIdent(relation) + ` WHERE org_id = $1`
	args := []any{orgID}
	if from != nil && to != nil {
		query += ` AND occurred_at >= $2 AND occurred_at < $3`
		args = append(args, *from, *to)
	}
	query += ` ORDER BY occurred_at ASC, id ASC`
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, "", "", err
	}
	var count int64
	for rows.Next() {
		ev, err := auditlog.ScanEvent(rows)
		if err != nil {
			rows.Close()
			return 0, "", "", err
		}
		if err := enc.Encode(ev); err != nil {
			rows.Close()
			return 0, "", "", err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, "", "", err
	}
	rows.Close()
	if err := gz.Close(); err != nil {
		return 0, "", "", err
	}
	if err := tmp.Close(); err != nil {
		return 0, "", "", err
	}

	key := fmt.Sprintf("%s/org/%d/%s.ndjson.gz", auditArchivePrefix, orgID, month)
	reader, err := os.Open(tmpName)
	if err != nil {
		return 0, "", "", err
	}
	defer reader.Close()
	w, err := bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return 0, "", "", err
	}
	if _, err := io.Copy(w, reader); err != nil {
		_ = w.Close()
		return 0, "", "", err
	}
	if err := w.Close(); err != nil {
		return 0, "", "", err
	}
	return count, hex.EncodeToString(digest.Sum(nil)), key, nil
}

func (m *AuditMaintenanceManager) recordArchive(ctx context.Context, orgID int64, month, key string, count int64, digest string) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO audit_archives (org_id, month, object_key, row_count, sha256)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id, month) DO NOTHING`, orgID, month, key, count, digest)
	return err
}

// ---------------------------------------------------------------------------
// Pruning

// archiveDefaultRows handles historical rows that landed in the DEFAULT
// partition (notably the legacy user-management import). Without this, they
// would be pruned at the retention boundary without ever being archived.
func (m *AuditMaintenanceManager) archiveDefaultRows(ctx context.Context, cfg AuditSettings) error {
	now := time.Now().UTC()
	hotCutoff := now.AddDate(0, 0, -cfg.HotDays)
	retentionCutoff := now.AddDate(0, 0, -cfg.RetentionDays)

	rows, err := m.db.QueryContext(ctx, `
		SELECT DISTINCT org_id, date_trunc('month', occurred_at) AS month
		FROM audit_events_default
		WHERE occurred_at < $1 AND occurred_at >= $2
		ORDER BY org_id, month`, hotCutoff, retentionCutoff)
	if err != nil {
		return err
	}
	type bucketMonth struct {
		orgID int64
		month time.Time
	}
	var targets []bucketMonth
	for rows.Next() {
		var t bucketMonth
		if err := rows.Scan(&t.orgID, &t.month); err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	rows.Close()
	if len(targets) == 0 {
		return nil
	}

	bucket, err := blobstore.OpenBucketFromDB(ctx, m.db)
	if err != nil {
		return fmt.Errorf("open blob store: %w", err)
	}
	defer bucket.Close()

	for _, t := range targets {
		month := t.month.UTC().Format("2006-01")
		from := time.Date(t.month.UTC().Year(), t.month.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 1, 0)
		// Only archive months that are fully outside the hot window. A month
		// that straddles the cutoff still contains hot rows; leave those in
		// DEFAULT and let retention handle them later.
		if to.After(hotCutoff) {
			continue
		}
		archived, err := m.archiveExists(ctx, t.orgID, month)
		if err != nil {
			return err
		}
		if archived {
			// Another partition already supplied this month's archive. Do not
			// delete these rows: the existing object may not contain them.
			continue
		}
		count, digest, key, err := m.exportOrgMonth(ctx, bucket, "audit_events_default", t.orgID, month, &from, &to)
		if err != nil {
			return err
		}
		if err := m.recordArchive(ctx, t.orgID, month, key, count, digest); err != nil {
			return err
		}
		if m.recorder != nil {
			_ = m.recorder.System(ctx, 0, auditlog.ActionSystemAuditArchived,
				auditlog.Details("source", "default_partition", "org_id", t.orgID, "month", month, "rows", count))
		}
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `SET LOCAL app.audit_purge = 'on'`); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM audit_events_default
			WHERE org_id = $1 AND occurred_at >= $2 AND occurred_at < $3`,
			t.orgID, from, to); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (m *AuditMaintenanceManager) pruneExpiredArchives(ctx context.Context, cfg AuditSettings) error {
	cutoffMonth := time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays).Format("2006-01")
	rows, err := m.db.QueryContext(ctx,
		`SELECT org_id, month, object_key FROM audit_archives WHERE month < $1`, cutoffMonth)
	if err != nil {
		return err
	}
	type archiveRow struct {
		orgID int64
		month string
		key   string
	}
	var expired []archiveRow
	for rows.Next() {
		var r archiveRow
		if err := rows.Scan(&r.orgID, &r.month, &r.key); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, r)
	}
	rows.Close()
	if len(expired) == 0 {
		return nil
	}
	bucket, err := blobstore.OpenBucketFromDB(ctx, m.db)
	if err != nil {
		return err
	}
	defer bucket.Close()
	for _, r := range expired {
		if err := bucket.Delete(ctx, r.key); err != nil && !blobstore.IsNotExist(err) {
			return err
		}
		if _, err := m.db.ExecContext(ctx,
			`DELETE FROM audit_archives WHERE org_id = $1 AND month = $2`, r.orgID, r.month); err != nil {
			return err
		}
	}
	log.Info().Int("archives", len(expired)).Str("cutoff_month", cutoffMonth).
		Msg("audit: expired archives pruned")
	return nil
}

func (m *AuditMaintenanceManager) pruneDefaultRows(ctx context.Context, cfg AuditSettings) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -cfg.RetentionDays)
	var total int64
	for {
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `SET LOCAL app.audit_purge = 'on'`); err != nil {
			_ = tx.Rollback()
			return err
		}
		res, err := tx.ExecContext(ctx, `
			DELETE FROM audit_events_default
			WHERE id IN (
				SELECT id FROM audit_events_default
				WHERE occurred_at < $1
				LIMIT 10000
			)`, cutoff)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		n, _ := res.RowsAffected()
		if err := tx.Commit(); err != nil {
			return err
		}
		total += n
		if n == 0 {
			break
		}
	}
	if total > 0 && m.recorder != nil {
		_ = m.recorder.System(ctx, 0, auditlog.ActionSystemAuditPruned,
			auditlog.Details("rows", total, "cutoff", cutoff.Format(time.RFC3339)))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Anchor

type anchorOrg struct {
	OrgID int64  `json:"org_id"`
	Seq   int64  `json:"seq"`
	Hash  string `json:"hash"`
	At    string `json:"at,omitempty"`
}

type anchorFile struct {
	Date        string      `json:"date"`
	GeneratedAt string      `json:"generated_at"`
	Orgs        []anchorOrg `json:"orgs"`
}

func (m *AuditMaintenanceManager) writeAnchor(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT org_id FROM audit_events WHERE seq IS NOT NULL ORDER BY org_id`)
	if err != nil {
		return err
	}
	var orgIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		orgIDs = append(orgIDs, id)
	}
	rows.Close()

	now := time.Now().UTC()
	anchor := anchorFile{Date: now.Format("2006-01-02"), GeneratedAt: now.Format(time.RFC3339)}
	for _, orgID := range orgIDs {
		seq, hash, at, err := auditlog.LatestChainTip(ctx, m.db, orgID)
		if err != nil {
			return err
		}
		if seq == 0 {
			continue
		}
		entry := anchorOrg{OrgID: orgID, Seq: seq, Hash: hash}
		if !at.IsZero() {
			entry.At = at.UTC().Format(time.RFC3339)
		}
		anchor.Orgs = append(anchor.Orgs, entry)
	}
	payload, err := json.MarshalIndent(anchor, "", "  ")
	if err != nil {
		return err
	}
	bucket, err := blobstore.OpenBucketFromDB(ctx, m.db)
	if err != nil {
		return err
	}
	defer bucket.Close()
	key := fmt.Sprintf("%s/anchors/%s.json", auditArchivePrefix, anchor.Date)
	w, err := bucket.NewWriter(ctx, key, nil)
	if err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if m.recorder != nil {
		_ = m.recorder.System(ctx, 0, auditlog.ActionSystemAuditAnchor,
			auditlog.Details("key", key, "orgs", len(anchor.Orgs)))
	}
	log.Info().Str("key", key).Int("orgs", len(anchor.Orgs)).Msg("audit: anchor written")
	return nil
}

// pqIdent quotes a generated SQL identifier (partition names are derived from
// time, never user input) so a malformed name cannot break the statement.
func pqIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// RunNow triggers a maintenance cycle immediately (used by the super-admin
// "run maintenance" action and by operational scripts).
func (m *AuditMaintenanceManager) RunNow() {
	m.runCycle()
}
