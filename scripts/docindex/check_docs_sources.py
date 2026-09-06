#!/usr/bin/env python3
"""
Checks the commit SHAs recorded in internal/docindex/docs/.synced-commits.env
(what's currently committed/embedded for the chatbot's RAG corpus's external
sources) against each source's current branch tip on GitHub/GitLab.

There is no separate "pinned" lockfile - the committed marker file IS the
record of what's currently synced, and it's what gets compared against
live upstream. See docs/docs-sources-pinning-plan.md for why an earlier
version of this design had a second file for this and why it was dropped:
a redundant intermediate that always ended up equal to the marker in
steady state, since content only ever changes together with the marker
via an actual fetch.

Unlike scripts/check_docker_deps.py this never clones anything - it uses
`git ls-remote` to read a branch tip's commit SHA directly, one round-trip
per source (run in parallel), regardless of that source's repo size.

This script only ever reads - it never rewrites the marker file. Doing
that is scripts/docindex/sync_docs_sources.sh's job, and only ever as part
of an actual fetch (rewriting the marker without fetching the matching
content would make it lie about what's on disk).

Usage:
    scripts/docindex/check_docs_sources.py         Report only. Exits 1 if
                                                    any source is behind
                                                    its remote branch tip -
                                                    usable in CI, or just to
                                                    see what's changed
                                                    upstream since the last
                                                    `make sync-docs-sources`.
    scripts/docindex/check_docs_sources.py --print Machine-readable: prints
                                                    one KEY=sha line per
                                                    source that resolved
                                                    successfully (silently
                                                    omits ones that failed
                                                    to look up). Used by
                                                    sync_docs_sources.sh,
                                                    not meant for humans.

Exit codes: 1 if any source is behind its remote branch tip (0 otherwise).
A lookup failure is reported but does not affect the exit code - a
transient network issue shouldn't fail CI. --print always exits 0 -
failures are reported on stderr, not via exit code, since the caller
decides per-source what to do about a failed lookup (keep the existing
committed content).
"""

import argparse
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
MARKER_FILE = REPO_ROOT / 'internal' / 'docindex' / 'docs' / '.synced-commits.env'

TIMEOUT = 15

DOCS_SOURCES = [
    {
        'key': 'GIT_LRC_COMMIT',
        'label': 'git-lrc',
        'url': 'https://github.com/HexmosTech/git-lrc.git',
        'branch': 'main',
    },
    {
        'key': 'GIT_LRC_WIKI_COMMIT',
        'label': 'git-lrc wiki',
        'url': 'https://github.com/HexmosTech/git-lrc.wiki.git',
        'branch': 'master',
    },
    {
        'key': 'LIVEREVIEW_WIKI_COMMIT',
        'label': 'LiveReview wiki',
        'url': 'https://github.com/HexmosTech/LiveReview.wiki.git',
        'branch': 'master',
    },
    {
        # Private repo (SSH-auth'd) - unlike the 3 above. `git ls-remote`
        # works the same way over SSH as HTTPS; the machine running this
        # just needs an SSH key with read access to git.apps.hexmos.com.
        'key': 'HEXMOSHOMEPAGE_DOCS_COMMIT',
        'label': 'hexmoshomepage docs',
        'url': 'git@git.apps.hexmos.com:hexmos/frontend/hexmoshomepage.git',
        'branch': 'main',
    },
]


def ls_remote_sha(url, branch):
    """Current commit SHA of refs/heads/<branch> on <url>, via `git ls-remote` (no clone)."""
    result = subprocess.run(
        ['git', 'ls-remote', url, f'refs/heads/{branch}'],
        capture_output=True, text=True, timeout=TIMEOUT, check=True,
    )
    line = result.stdout.strip()
    if not line:
        raise RuntimeError(f'no ref refs/heads/{branch} found on {url}')
    return line.split()[0]


def load_marker():
    """Returns {KEY: sha} from the committed marker file - what's currently synced."""
    committed = {}
    if not MARKER_FILE.exists():
        return committed
    for line in MARKER_FILE.read_text().splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith('#') or '=' not in stripped:
            continue
        key, _, value = stripped.partition('=')
        committed[key.strip()] = value.strip()
    return committed


def _check_one(src, committed):
    """Looks up one source's remote tip. Runs in a worker thread - returns
    the result dict rather than mutating shared state."""
    key = src['key']
    current = committed.get(key)
    entry = {'key': key, 'label': src['label'], 'current': current, 'latest': None,
              'outdated': False, 'error': None}
    try:
        latest = ls_remote_sha(src['url'], src['branch'])
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError) as e:
        entry['error'] = f'lookup failed: {e}'
        return entry

    entry['latest'] = latest
    entry['outdated'] = latest != current
    return entry


def gather_results(committed):
    """Returns list of dicts: {key, label, current, latest, outdated, error}.

    Looks up all sources' remote tips concurrently (each is an independent
    `git ls-remote` network round-trip, so there's no reason to serialize
    them) - result order matches DOCS_SOURCES regardless of which finishes
    first.
    """
    with ThreadPoolExecutor(max_workers=len(DOCS_SOURCES)) as pool:
        return list(pool.map(lambda src: _check_one(src, committed), DOCS_SOURCES))


def print_report(results):
    print(f'\nSynced docs source commits ({MARKER_FILE.relative_to(REPO_ROOT)}):\n')
    width_label = max(len(r['label']) for r in results)
    for r in results:
        label = r['label'].ljust(width_label)
        current = (r['current'] or '?')[:12]
        if r['error']:
            status = f'⚠️  {r["error"]}'
        elif r['outdated']:
            status = f'→ behind - remote tip is {r["latest"][:12]}'
        else:
            status = '✓ up to date'
        print(f'  {label}  {current}  {status}')

    outdated = [r for r in results if r['outdated']]
    errors = [r for r in results if r['error']]

    print()
    if outdated:
        print(f'{len(outdated)} source(s) behind their remote branch tip - run `make sync-docs-sources` to fetch.')
    else:
        print('All docs sources are synced to their current branch tip.')
    if errors:
        print(f'{len(errors)} lookup(s) could not be completed (network issue or missing branch).')
    return outdated, errors


def run_print():
    """Machine-readable mode for sync_docs_sources.sh: prints KEY=sha for
    every source that resolved successfully, silently omitting (with a
    stderr note) any that failed - the caller keeps whatever's already
    committed for those.
    """
    results = gather_results(committed={})
    for r in results:
        if r['error']:
            print(f'# {r["label"]} lookup failed: {r["error"]}', file=sys.stderr)
            continue
        print(f'{r["key"]}={r["latest"]}')
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--print', action='store_true', dest='print_mode',
                         help='Machine-readable KEY=sha output for sync_docs_sources.sh, not for humans.')
    args = parser.parse_args()

    if args.print_mode:
        return run_print()

    committed = load_marker()
    print('Checking remote branch tips for docs sources (needs network access, no cloning)...')
    results = gather_results(committed)
    outdated, errors = print_report(results)
    return 1 if outdated else 0


if __name__ == '__main__':
    sys.exit(main())
