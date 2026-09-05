#!/usr/bin/env python3
"""
Checks scripts/docindex/docs_sources.env (the pinned commit SHAs for the
chatbot's RAG corpus - external sources baked into internal/docindex/docs/
via scripts/docindex/sync_docs_sources.sh) against each source's current
branch tip on GitHub, and can update the file in place.

Unlike scripts/check_docker_deps.py this never clones anything - it uses
`git ls-remote` to read a branch tip's commit SHA directly, one round-trip
per source, regardless of that source's repo size.

See docs/docs-sources-pinning-plan.md for the full design.

Usage:
    scripts/docindex/check_docs_sources.py         Interactive: shows a
                                                    table, then asks per
                                                    outdated entry whether
                                                    to bump it.
                                                    (Auto-degrades to a
                                                    non-blocking report if
                                                    stdin isn't a TTY.)
    scripts/docindex/check_docs_sources.py --check Report only, never
                                                    prompts, never writes.
                                                    Exits 1 if any pinned
                                                    source is behind - use
                                                    this in CI.
    scripts/docindex/check_docs_sources.py --yes   Non-interactive: bumps
                                                    every outdated entry
                                                    automatically.
    scripts/docindex/check_docs_sources.py --auto  Like --yes, but meant to
                                                    run unattended on every
                                                    build/dev-server start
                                                    (wired into
                                                    scripts/docindex/sync_docs_sources.sh):
                                                    silent when everything's
                                                    already current, and a
                                                    lookup failure (offline,
                                                    GitHub down) is a warning,
                                                    not a build-breaking
                                                    error - falls back to
                                                    whatever's already
                                                    pinned. Lookups run in
                                                    parallel.

Exit codes: 0 always, except --check mode, which exits 1 if any pinned
source is behind its remote branch tip (0 otherwise). --auto always exits 0.
"""

import argparse
import os
import re
import subprocess
import sys
import tempfile
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
DOCS_SOURCES_FILE = REPO_ROOT / 'scripts' / 'docindex' / 'docs_sources.env'

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


def load_docs_sources():
    """Returns {KEY: sha}."""
    sources = {}
    if not DOCS_SOURCES_FILE.exists():
        return sources
    for line in DOCS_SOURCES_FILE.read_text().splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith('#') or '=' not in stripped:
            continue
        key, _, value = stripped.partition('=')
        sources[key.strip()] = value.strip()
    return sources


def write_docs_source(key, new_value):
    """Rewrite a single KEY=value line in scripts/docindex/docs_sources.env, preserving everything else.

    Writes to a temp file in the same directory and os.replace()s it into
    place, so a crash mid-write can never leave the lockfile truncated.
    """
    lines = DOCS_SOURCES_FILE.read_text().splitlines(keepends=True)
    pattern = re.compile(rf'^{re.escape(key)}=.*$')
    for i, line in enumerate(lines):
        if pattern.match(line.rstrip('\n')):
            newline = '\n' if line.endswith('\n') else ''
            lines[i] = f'{key}={new_value}{newline}'
            fd, tmp_path = tempfile.mkstemp(dir=DOCS_SOURCES_FILE.parent, prefix='.docs_sources.env.')
            try:
                with os.fdopen(fd, 'w') as f:
                    f.write(''.join(lines))
                os.replace(tmp_path, DOCS_SOURCES_FILE)
            except BaseException:
                os.unlink(tmp_path)
                raise
            return True
    return False


def _check_one(src, pinned):
    """Looks up one source's remote tip. Runs in a worker thread - returns
    the result dict rather than mutating shared state."""
    key = src['key']
    current = pinned.get(key)
    entry = {'key': key, 'label': src['label'], 'current': current, 'latest': None,
              'outdated': False, 'error': None}
    if current is None:
        entry['error'] = 'not set in docs_sources.env'
        return entry
    try:
        latest = ls_remote_sha(src['url'], src['branch'])
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError) as e:
        entry['error'] = f'lookup failed: {e}'
        return entry

    entry['latest'] = latest
    entry['outdated'] = latest != current
    return entry


def gather_results(pinned):
    """Returns list of dicts: {key, label, current, latest, outdated, error}.

    Looks up all sources' remote tips concurrently (each is an independent
    `git ls-remote` network round-trip, so there's no reason to serialize
    them) - result order matches DOCS_SOURCES regardless of which finishes
    first.
    """
    with ThreadPoolExecutor(max_workers=len(DOCS_SOURCES)) as pool:
        return list(pool.map(lambda src: _check_one(src, pinned), DOCS_SOURCES))


def print_report(results):
    print(f'\nPinned docs source commits ({DOCS_SOURCES_FILE.relative_to(REPO_ROOT)}):\n')
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
        print(f'{len(outdated)} source(s) behind their remote branch tip.')
    else:
        print('All docs sources are pinned to their current branch tip.')
    if errors:
        print(f'{len(errors)} lookup(s) could not be completed (network issue or missing branch).')
    return outdated, errors


def apply_update(entry):
    ok = write_docs_source(entry['key'], entry['latest'])
    if ok:
        print(f'  updated {entry["key"]}: {entry["current"][:12]} -> {entry["latest"][:12]}')
    else:
        print(f'  ⚠️  could not find {entry["key"]} line in {DOCS_SOURCES_FILE} to update')
    return ok


def run_interactive(outdated):
    apply_to_all = False
    for entry in outdated:
        if apply_to_all:
            apply_update(entry)
            continue
        while True:
            answer = input(
                f'Bump {entry["key"]} ({entry["current"][:12]} -> {entry["latest"][:12]})? '
                f'[y]es/[N]o/[a]ll/[q]uit: '
            ).strip().lower()
            if answer in ('', 'n', 'no'):
                break
            if answer in ('y', 'yes'):
                apply_update(entry)
                break
            if answer in ('a', 'all'):
                apply_to_all = True
                apply_update(entry)
                break
            if answer in ('q', 'quit'):
                print('Stopping - no further changes will be made.')
                return
            print('Please answer y, n, a, or q.')


def run_auto(pinned):
    """Unattended mode for scripts/docindex/sync_docs_sources.sh: bump every
    outdated pin, silently, unless something actually changed or a lookup
    failed. Never fails the build - a network hiccup just means "keep
    whatever's already pinned", same as any other offline dev-server start.
    """
    results = gather_results(pinned)
    outdated = [r for r in results if r['outdated']]
    errors = [r for r in results if r['error']]

    for entry in outdated:
        apply_update(entry)
    if errors:
        for e in errors:
            print(f'  ⚠️  docs source check for {e["label"]} skipped: {e["error"]}')
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--check', action='store_true',
                       help='Report only, never prompt or write. Exits 1 if any pinned source is behind.')
    mode.add_argument('--yes', '-y', action='store_true',
                       help='Non-interactive: bump every outdated entry automatically.')
    mode.add_argument('--auto', action='store_true',
                       help='Unattended, quiet-when-unchanged, network-failure-tolerant. '
                            'For use from sync_docs_sources.sh, not interactively.')
    args = parser.parse_args()

    if not DOCS_SOURCES_FILE.exists():
        print(f'⚠️  {DOCS_SOURCES_FILE} not found - nothing to check.')
        return 0

    pinned = load_docs_sources()

    if args.auto:
        return run_auto(pinned)

    print('Checking remote branch tips for docs sources (needs network access, no cloning)...')
    results = gather_results(pinned)
    outdated, errors = print_report(results)

    if args.check:
        return 1 if outdated else 0

    if not outdated:
        return 0

    if args.yes:
        print('\nBumping all outdated pins (--yes)...')
        for entry in outdated:
            apply_update(entry)
        return 0

    if not sys.stdin.isatty():
        print('\nNon-interactive shell detected - not prompting.')
        print('Run `make update-docs-sources` (interactive) or `make update-docs-sources-yes` (auto) to bump.')
        return 0

    print()
    run_interactive(outdated)
    return 0


if __name__ == '__main__':
    sys.exit(main())
