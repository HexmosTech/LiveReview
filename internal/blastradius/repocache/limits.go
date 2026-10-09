package repocache

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultRoot holds the repo cache and graph indexes, next to the local blob store.
const DefaultRoot = "./lrdata/blastradius"

// MinGB is the smallest cache size admins can set; DefaultGB applies until they change it.
const (
	MinGB     = 1
	DefaultGB = 5
)

const gb = 1 << 30

// Settings is the system_settings row "blast_radius_cache"; read fresh on every job.
type Settings struct {
	Enabled bool `json:"enabled"`
	MaxGB   int  `json:"max_gb"`
}

func (s Settings) MaxBytes() int64 { return int64(s.MaxGB) * gb }

// LoadSettings returns the saved settings, or the defaults (on, DefaultGB) when none are saved.
func LoadSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	s := Settings{Enabled: true, MaxGB: DefaultGB}
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM system_settings WHERE name = 'blast_radius_cache'`).Scan(&data)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.MaxGB < MinGB {
		s.MaxGB = MinGB
	}
	return s, nil
}

func SaveSettings(ctx context.Context, db *sql.DB, s Settings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO system_settings (name, data) VALUES ('blast_radius_cache', $1)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP`, data)
	return err
}

// Usage is what the Settings page shows: total cache size and number of cached repos.
type Usage struct {
	UsedBytes int64 `json:"used_bytes"`
	Repos     int   `json:"repos"`
}

func GetUsage(root string) Usage {
	return Usage{UsedBytes: dirSize(root), Repos: len(entries(root))}
}

// RecordIndex remembers which graph index belongs to this checkout, so eviction can delete it too.
func (c *Checkout) RecordIndex(project string) error {
	return os.WriteFile(filepath.Join(filepath.Dir(c.Dir), ".index"), []byte(project), 0o644)
}

// EntrySize is the cached repo plus its graph index (the index is usually the bigger part).
func EntrySize(root, entry string) int64 {
	size := dirSize(entry)
	for _, f := range indexFiles(root, entry) {
		if st, err := os.Stat(f); err == nil {
			size += st.Size()
		}
	}
	return size
}

// Evict deletes least-recently-used repos (by .lastused) until the cache fits in maxBytes.
// Repos a job is using right now are skipped.
func Evict(root string, maxBytes int64) int {
	removeOrphanIndexes(root)
	used := dirSize(root)
	if used <= maxBytes {
		return 0
	}
	list := entries(root)
	sort.Slice(list, func(i, j int) bool { return lastUsed(list[i]).Before(lastUsed(list[j])) })
	removed := 0
	for _, e := range list {
		if used <= maxBytes {
			break
		}
		size := EntrySize(root, e)
		if removeIfIdle(root, e) {
			used -= size
			removed++
		}
	}
	return removed
}

// TooLarge reports a repo recorded as bigger than the cache, and its recorded size.
func TooLarge(root string, r Repo) (int64, bool) {
	entry, err := EntryDir(root, r)
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(entry + ".too_large")
	if err != nil {
		return 0, false
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return size, err == nil
}

// MarkTooLarge deletes a repo that doesn't fit in the cache (caller holds its lock) and
// records its size, so later reviews skip it until the cache size is raised.
func MarkTooLarge(root string, c *Checkout, size int64) error {
	entry := filepath.Dir(c.Dir)
	removeEntry(root, entry)
	repoLocks.Delete(filepath.Join(entry, "repo")) // same key as Prepare/removeIfIdle (== c.Dir)
	return os.WriteFile(entry+".too_large", []byte(strconv.FormatInt(size, 10)), 0o644)
}

// ClearTooLarge forgets the marker once the cache size has been raised above it.
func ClearTooLarge(root string, r Repo) {
	if entry, err := EntryDir(root, r); err == nil {
		_ = os.Remove(entry + ".too_large")
	}
}

// RemoveScope deletes every cached repo of a connector (connectorID > 0) or a whole org.
func RemoveScope(root string, orgID, connectorID int64) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	dir := filepath.Join(abs, strconv.FormatInt(orgID, 10))
	if connectorID > 0 {
		dir = filepath.Join(dir, strconv.FormatInt(connectorID, 10))
	}
	for _, e := range entries(root) {
		if strings.HasPrefix(e, dir+string(filepath.Separator)) {
			removeIfIdle(root, e)
		}
	}
	// Never RemoveAll here: a repo a job is using stays until later eviction removes it.
	markers, _ := filepath.Glob(filepath.Join(dir, "*.too_large"))
	more, _ := filepath.Glob(filepath.Join(dir, "*", "*.too_large"))
	for _, m := range append(markers, more...) {
		_ = os.Remove(m)
	}
	subdirs, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, d := range subdirs {
		_ = os.Remove(d) // only succeeds when empty
	}
	_ = os.Remove(dir)
}

// removeOrphanIndexes deletes graph index files no cached repo points to (e.g. its repo
// was removed mid-job). ponytail: 1h age guard so a job still indexing keeps its files.
func removeOrphanIndexes(root string) {
	used := map[string]bool{}
	for _, e := range entries(root) {
		for _, f := range indexFiles(root, e) {
			used[f] = true
		}
	}
	abs, _ := filepath.Abs(root)
	files, _ := filepath.Glob(filepath.Join(abs, "index", "*.db*"))
	for _, f := range files {
		if used[f] {
			continue
		}
		if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) > time.Hour {
			_ = os.Remove(f)
		}
	}
}

// FreeBytes is the free space on the volume holding root.
func FreeBytes(root string) (uint64, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, err
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// entries lists cache folders <root>/<org_id>/<connector_id>/<owner>__<repo>, as absolute
// paths so they match the lock keys Prepare uses.
func entries(root string) []string {
	root, _ = filepath.Abs(root)
	matches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*"))
	var out []string
	for _, m := range matches {
		rel, _ := filepath.Rel(root, m)
		org := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if _, err := strconv.ParseInt(org, 10, 64); err != nil { // skips index/
			continue
		}
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			out = append(out, m)
		}
	}
	return out
}

func removeIfIdle(root, entry string) bool {
	key := filepath.Join(entry, "repo")
	mu, _ := repoLocks.LoadOrStore(key, &sync.Mutex{})
	if !mu.(*sync.Mutex).TryLock() {
		return false
	}
	defer mu.(*sync.Mutex).Unlock()
	removeEntry(root, entry)
	repoLocks.Delete(key) // the map only holds cached repos; a new job gets a fresh lock
	return true
}

func removeEntry(root, entry string) {
	for _, f := range indexFiles(root, entry) {
		_ = os.Remove(f)
	}
	_ = os.RemoveAll(entry)
}

// indexFiles are the graph index's SQLite files for entry (named after its project).
func indexFiles(root, entry string) []string {
	project, err := os.ReadFile(filepath.Join(entry, ".index"))
	name := strings.TrimSpace(string(project))
	if err != nil || name == "" || strings.ContainsAny(name, `/\`) {
		return nil
	}
	abs, _ := filepath.Abs(root)
	db := filepath.Join(abs, "index", name+".db")
	return []string{db, db + "-shm", db + "-wal"}
}

func lastUsed(entry string) time.Time {
	st, err := os.Stat(filepath.Join(entry, ".lastused"))
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

func dirSize(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
