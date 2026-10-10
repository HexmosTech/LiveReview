// Package repocache keeps a blobless ~13-month clone of each reviewed repo on
// the lrdata volume for server-side blast radius (docs/blast-radius-server-side-plan.md).
package repocache

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// shallowSince is one month wider than codebase-memory-mcp's 1-year
// co-change window, so the shallow cut-off commit never lands inside it.
const shallowSince = "13 months ago"

// ErrBaseTooOld means the PR's base is outside the cached history window.
var ErrBaseTooOld = errors.New("repocache: PR base is older than the cached history window")

// Repo identifies one PR to check out. Username may be empty for providers
// whose HTTPS token username is fixed (see defaultUsername).
type Repo struct {
	OrgID        int64
	ConnectorID  int64
	Provider     string // github, gitlab, bitbucket, gitea, azuredevops (prefix match)
	CloneURL     string // https://host/owner/repo(.git)
	Username     string
	Token        string
	PRNumber     int
	SourceBranch string // Bitbucket only: it has no PR refs
	BaseBranch   string // empty means the remote's default branch
}

// Checkout is a prepared working tree at the PR head.
type Checkout struct {
	Dir     string
	HeadSHA string
	BaseSHA string // merge-base of the PR head and the base branch
	env     []string
}

// Prepare clones on first use, fetches the PR head + base branch and checks out the head
// detached; call unlock when done. A job on the same repo in any process waits for it.
func Prepare(ctx context.Context, root string, r Repo) (c *Checkout, unlock func(), err error) {
	prRef, err := prRef(r)
	if err != nil {
		return nil, nil, err
	}
	base, err := EntryDir(root, r)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(base, "repo")

	release, _, err := lockEntry(ctx, base, true)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			release()
		}
	}()

	// The ceiling stops git from finding a parent repo (lrdata sits inside the LiveReview checkout in dev).
	c = &Checkout{Dir: dir, env: append(gitEnv(r), "GIT_CEILING_DIRECTORIES="+base)}
	if err = c.ensureClone(ctx, r.CloneURL); err != nil {
		return nil, nil, err
	}

	baseBranch := r.BaseBranch
	if baseBranch == "" {
		if baseBranch, err = c.defaultBranch(ctx); err != nil {
			return nil, nil, err
		}
	}
	if err = shallowFallback(func(limit string) error {
		_, err := c.git(ctx, "fetch", "--filter=blob:none", limit, "--no-tags", "origin",
			"+refs/heads/"+baseBranch+":refs/remotes/origin/"+baseBranch,
			"+"+prRef+":refs/remotes/pr/head")
		return err
	}); err != nil {
		return nil, nil, err
	}
	if c.HeadSHA, err = c.git(ctx, "rev-parse", "refs/remotes/pr/head"); err != nil {
		return nil, nil, err
	}
	if c.BaseSHA, err = c.git(ctx, "merge-base", "refs/remotes/origin/"+baseBranch, c.HeadSHA); err != nil {
		return nil, nil, ErrBaseTooOld
	}
	if _, err = c.git(ctx, "checkout", "--force", "--detach", c.HeadSHA); err != nil {
		return nil, nil, err
	}
	if err = os.WriteFile(filepath.Join(base, ".lastused"), nil, 0o644); err != nil {
		return nil, nil, err
	}
	return c, release, nil
}

// EntryDir is r's cache folder: <root>/<org_id>/<connector_id>/<owner>__<repo>, absolute
// because git commands run from different dirs.
func EntryDir(root string, r Repo) (string, error) {
	slug, err := repoSlug(r.CloneURL)
	if err != nil {
		return "", err
	}
	if root, err = filepath.Abs(root); err != nil {
		return "", err
	}
	return filepath.Join(root, strconv.FormatInt(r.OrgID, 10), strconv.FormatInt(r.ConnectorID, 10), slug), nil
}

// Diff returns the PR diff (merge-base..head) with rename detection, so hunk
// keys match the diff the provider shows for renamed files.
func (c *Checkout) Diff(ctx context.Context) ([]byte, error) {
	return c.run(ctx, c.Dir, "diff", "-M", "--no-color", "--no-ext-diff", c.BaseSHA, c.HeadSHA)
}

// ensureClone clones into c.Dir unless a usable repo is already there; a
// broken leftover dir is removed and re-cloned.
func (c *Checkout) ensureClone(ctx context.Context, cloneURL string) error {
	if _, err := os.Stat(filepath.Join(c.Dir, ".git")); err == nil {
		if _, err := c.git(ctx, "rev-parse", "--git-dir"); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(c.Dir), 0o755); err != nil {
		return err
	}
	err := shallowFallback(func(limit string) error {
		if err := os.RemoveAll(c.Dir); err != nil {
			return err
		}
		_, err := c.gitIn(ctx, filepath.Dir(c.Dir), "clone", "--filter=blob:none", limit,
			"--single-branch", "--no-tags", "--no-checkout", cloneURL, c.Dir)
		return err
	})
	if err != nil {
		_ = os.RemoveAll(c.Dir)
		return err
	}
	// Rename detection in the graph engine's git log would lazily fetch old blobs one by one.
	for _, kv := range [][2]string{{"diff.renames", "false"}, {"checkout.workers", "8"}} {
		if _, err := c.git(ctx, "config", kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// shallowFallback runs fn with --shallow-since and retries with a fixed depth when that fails:
// inactive repos ("shallow info") and hosts without it, e.g. Azure DevOps ("does not support").
func shallowFallback(fn func(limit string) error) error {
	err := fn("--shallow-since=" + shallowSince)
	if err != nil && (strings.Contains(err.Error(), "shallow info") ||
		strings.Contains(err.Error(), "does not support --shallow-since")) {
		err = fn("--depth=200")
	}
	return err
}

func (c *Checkout) defaultBranch(ctx context.Context) (string, error) {
	out, err := c.git(ctx, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return "", err
	}
	if m := symrefHead.FindStringSubmatch(out); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("repocache: could not resolve default branch")
}

var symrefHead = regexp.MustCompile(`ref: refs/heads/(\S+)\s+HEAD`)

func (c *Checkout) git(ctx context.Context, args ...string) (string, error) {
	return c.gitIn(ctx, c.Dir, args...)
}

func (c *Checkout) gitIn(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := c.run(ctx, dir, args...)
	return strings.TrimSpace(string(out)), err
}

func (c *Checkout) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = c.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gitEnv ignores the host's git config/credential helpers and passes the token as an
// HTTP Basic header via GIT_CONFIG_* env, so it never lands in argv, .git/config or logs.
func gitEnv(r Repo) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") { // an inherited GIT_DIR etc. would redirect every command
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=https:http", "LC_ALL=C",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if r.Token == "" {
		return env
	}
	user := r.Username
	if user == "" {
		user = defaultUsername(r.Provider)
	}
	cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + r.Token))
	// No redirects: curl would resend the auth header to whatever host a redirect points at.
	return append(env,
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic "+cred,
		"GIT_CONFIG_KEY_1=http.followRedirects",
		"GIT_CONFIG_VALUE_1=false",
	)
}

func defaultUsername(provider string) string {
	switch {
	case strings.HasPrefix(provider, "github"):
		return "x-access-token"
	case strings.HasPrefix(provider, "gitlab"):
		return "oauth2"
	case strings.HasPrefix(provider, "bitbucket"):
		return "x-token-auth"
	case strings.HasPrefix(provider, "azure"):
		return "pat"
	}
	return "git"
}

func prRef(r Repo) (string, error) {
	n := strconv.Itoa(r.PRNumber)
	switch {
	case strings.HasPrefix(r.Provider, "github"), strings.HasPrefix(r.Provider, "gitea"):
		return "refs/pull/" + n + "/head", nil
	case strings.HasPrefix(r.Provider, "gitlab"):
		return "refs/merge-requests/" + n + "/head", nil
	case strings.HasPrefix(r.Provider, "azure"):
		// Azure DevOps only publishes the merge ref, not the PR head.
		return "refs/pull/" + n + "/merge", nil
	case strings.HasPrefix(r.Provider, "bitbucket"):
		if r.SourceBranch == "" {
			return "", fmt.Errorf("repocache: bitbucket needs the PR source branch")
		}
		return "refs/heads/" + r.SourceBranch, nil
	}
	return "", fmt.Errorf("repocache: unsupported provider %q", r.Provider)
}

var unsafeSlugChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// repoSlug turns https://host/owner/repo.git into "owner__repo" (one dir name).
// URLs with credentials are rejected, since git would save them in .git/config.
func repoSlug(cloneURL string) (string, error) {
	u, err := url.Parse(cloneURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("repocache: clone URL must be http(s) without credentials")
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	slug := unsafeSlugChars.ReplaceAllString(strings.ReplaceAll(path, "/", "__"), "_")
	if slug == "" || strings.Trim(slug, ".") == "" {
		return "", fmt.Errorf("repocache: clone URL has no repo path")
	}
	return slug, nil
}
