#!/usr/bin/env python3
"""
Checks docker/docker-deps.env (the frozen THIRD-PARTY DOCKER DEPENDENCY
versions for LiveReview Docker builds - base images and external binaries
baked into the image, NOT the LiveReview application version) against the
latest available version in each dependency's own release channel, and can
update the file in place.

See docker/DOCKER-DEPS.md for the full writeup of this system (what gets
checked, where each version is sourced from, and the locking mechanism).

Usage:
    scripts/check_docker_deps.py                 Interactive: shows a
                                                  table, then asks per
                                                  outdated, unlocked entry
                                                  whether to update it.
                                                  (Auto-degrades to a
                                                  non-blocking report if
                                                  stdin isn't a TTY.)
    scripts/check_docker_deps.py --check          Report only, never
                                                  prompts, never writes.
                                                  Exits 1 if any *unlocked*
                                                  dependency is outdated -
                                                  use this in CI.
    scripts/check_docker_deps.py --yes            Non-interactive: updates
                                                  every outdated, unlocked
                                                  entry automatically.
    scripts/check_docker_deps.py --include-pinned Also offer/apply updates
                                                  to entries listed in
                                                  PINNED_DOCKER_DEPS, for
                                                  this run only. Combine
                                                  with --check, the
                                                  interactive default, or
                                                  --yes. Does not edit the
                                                  lock list itself.
    scripts/check_docker_deps.py --skip-network   Skip all upstream lookups
                                                  entirely (used by the
                                                  automatic pre-build hook
                                                  in lrops.py when
                                                  SKIP_DOCKER_DEPS_CHECK=1).

Locking a dependency (keeping it fixed across updates):
    Add its KEY to the comma-separated PINNED_DOCKER_DEPS= line in
    docker/docker-deps.env, e.g.:
        PINNED_DOCKER_DEPS=DEBIAN_IMAGE_TAG,DBMATE_VERSION
    Locked entries are still looked up and shown in the report (so you can
    see when a locked version falls behind) but are never auto-updated by
    the interactive prompt, --yes, or the automatic pre-build check. Remove
    the KEY from the list to unlock it, or pass --include-pinned for a
    one-off override without editing the lock list.

Exit codes: 0 always, except --check mode, which exits 1 if any unlocked
dependency is outdated (0 otherwise).
"""

import argparse
import json
import os
import re
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DOCKER_DEPS_FILE = REPO_ROOT / 'docker' / 'docker-deps.env'
GO_MOD_FILE = REPO_ROOT / 'go.mod'
DOCKERFILES = [REPO_ROOT / 'Dockerfile', REPO_ROOT / 'Dockerfile.crosscompile']

TIMEOUT = 8
USER_AGENT = 'LiveReview-docker-deps-checker'


def _http_json(url):
    req = urllib.request.Request(url, headers={'User-Agent': USER_AGENT, 'Accept': 'application/json'})
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        return json.loads(resp.read().decode('utf-8'))


def _ver_key(v):
    """Sort key for 'v1.2.3' / '1.2.3' style version strings."""
    v = v.lstrip('v')
    parts = re.findall(r'\d+', v)
    return tuple(int(p) for p in parts) if parts else (0,)


def check_github_release(repo):
    """Latest release tag_name for owner/repo on GitHub."""
    data = _http_json(f'https://api.github.com/repos/{repo}/releases/latest')
    return data['tag_name']


def check_dockerhub_semver(image, major, suffix):
    """Latest X.Y.Z-{suffix} tag for a given major version of a Docker Hub library image."""
    url = f'https://registry.hub.docker.com/v2/repositories/library/{image}/tags?page_size=100&name={major}.'
    data = _http_json(url)
    pattern = re.compile(rf'^{re.escape(major)}\.(\d+)\.(\d+)-{re.escape(suffix)}$')
    best = None
    best_key = None
    for r in data.get('results', []):
        m = pattern.match(r['name'])
        if not m:
            continue
        key = tuple(int(g) for g in m.groups())
        if best_key is None or key > best_key:
            best_key, best = key, r['name']
    return best


def check_dockerhub_dated(image, prefix, suffix):
    """Latest {prefix}YYYYMMDD{suffix} tag for a Docker Hub library image."""
    url = f'https://registry.hub.docker.com/v2/repositories/library/{image}/tags?page_size=100&name={prefix}'
    data = _http_json(url)
    pattern = re.compile(rf'^{re.escape(prefix)}(\d{{8}}){re.escape(suffix)}$')
    best = None
    best_date = None
    for r in data.get('results', []):
        m = pattern.match(r['name'])
        if not m:
            continue
        if best_date is None or m.group(1) > best_date:
            best_date, best = m.group(1), r['name']
    return best


def check_gomod_version(module):
    """Version pinned for `module` in go.mod, e.g. 'v0.32.0'."""
    text = GO_MOD_FILE.read_text()
    m = re.search(rf'^\s*{re.escape(module)}\s+(v\S+)', text, re.MULTILINE)
    return m.group(1) if m else None


def _node_check(current):
    m = re.match(r'^(\d+)\.\d+\.\d+-alpine$', current)
    if not m:
        return None
    return check_dockerhub_semver('node', m.group(1), 'alpine')


def _golang_check(current):
    m = re.match(r'^(\d+\.\d+)\.\d+-bookworm$', current)
    if not m:
        return None
    return check_dockerhub_semver('golang', m.group(1), 'bookworm')


def _debian_check(current):
    return check_dockerhub_dated('debian', 'trixie-', '-slim')


def _river_check(current):
    return check_gomod_version('github.com/riverqueue/river')


DOCKER_DEPS = [
    {
        'key': 'NODE_IMAGE_TAG',
        'label': 'Node.js base image',
        'checker': _node_check,
    },
    {
        'key': 'GOLANG_IMAGE_TAG',
        'label': 'Go base image',
        'checker': _golang_check,
    },
    {
        'key': 'DEBIAN_IMAGE_TAG',
        'label': 'Debian runtime base image',
        'checker': _debian_check,
    },
    {
        'key': 'RIVER_VERSION',
        'label': 'River CLI (must match go.mod)',
        'checker': _river_check,
        'note': 'sourced from go.mod, not GitHub releases',
    },
    {
        'key': 'RIVERUI_VERSION',
        'label': 'River UI',
        'checker': lambda cur: check_github_release('riverqueue/riverui'),
    },
    {
        'key': 'DBMATE_VERSION',
        'label': 'dbmate',
        'checker': lambda cur: check_github_release('amacneil/dbmate'),
    },
    {
        'key': 'VLCONVERT_VERSION',
        'label': 'vl-convert',
        'checker': lambda cur: check_github_release('vega/vl-convert'),
    },
    {
        'key': 'CODEBASE_MEMORY_MCP_VERSION',
        'label': 'codebase-memory-mcp',
        'checker': lambda cur: check_github_release('DeusData/codebase-memory-mcp'),
    },
    {
        'key': 'DBCTX_VERSION',
        'label': 'dbctx',
        'checker': lambda cur: check_github_release('shrsv/dbctx'),
    },
    {
        'key': 'ALAWS_VERSION',
        'label': 'AgentLaws (alaws)',
        'checker': lambda cur: check_github_release('shrsv/AgentLaws'),
    },
]


def load_docker_deps():
    """Returns (deps: {KEY: value}, locked: {KEY, ...}).

    PINNED_DOCKER_DEPS is a control line (comma-separated list of KEYs that
    are locked against auto-update) rather than a dependency itself, so
    it's pulled out into `locked` and not included in `deps`.
    """
    deps = {}
    locked = set()
    if not DOCKER_DEPS_FILE.exists():
        return deps, locked
    for line in DOCKER_DEPS_FILE.read_text().splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith('#') or '=' not in stripped:
            continue
        key, _, value = stripped.partition('=')
        key = key.strip()
        value = value.strip()
        if key == 'PINNED_DOCKER_DEPS':
            locked = {k.strip() for k in value.split(',') if k.strip()}
            continue
        deps[key] = value
    return deps, locked


def check_dockerfile_arg_drift(deps):
    """Find ARG default values in Dockerfile/Dockerfile.crosscompile that no
    longer match docker/docker-deps.env.

    Each Dockerfile ARG carries its own default (so a standalone
    `docker build` still works without --build-arg), but the real value
    always comes from docker-deps.env via lrops.py's --build-arg injection.
    A stale ARG default is harmless for the actual build pipeline, but it's
    misleading to read and silently wrong for anyone running `docker build`
    by hand - so this flags any drift explicitly instead of leaving it to a
    code-review comment.

    Returns a list of {file, key, dockerfile_default, docker_deps_value}.
    """
    drift = []
    for dockerfile in DOCKERFILES:
        if not dockerfile.exists():
            continue
        text = dockerfile.read_text()
        for key, value in deps.items():
            m = re.search(rf'^ARG\s+{re.escape(key)}=(\S+)\s*$', text, re.MULTILINE)
            if m and m.group(1) != value:
                drift.append({'file': dockerfile.name, 'key': key,
                               'dockerfile_default': m.group(1), 'docker_deps_value': value})
    return drift


def write_docker_dep(key, new_value):
    """Rewrite a single KEY=value line in docker/docker-deps.env, preserving everything else.

    Writes to a temp file in the same directory and os.replace()s it into
    place, so a crash mid-write can never leave docker-deps.env truncated
    or half-written.
    """
    lines = DOCKER_DEPS_FILE.read_text().splitlines(keepends=True)
    pattern = re.compile(rf'^{re.escape(key)}=.*$')
    for i, line in enumerate(lines):
        if pattern.match(line.rstrip('\n')):
            newline = '\n' if line.endswith('\n') else ''
            lines[i] = f'{key}={new_value}{newline}'
            fd, tmp_path = tempfile.mkstemp(dir=DOCKER_DEPS_FILE.parent, prefix='.docker-deps.env.')
            try:
                with os.fdopen(fd, 'w') as f:
                    f.write(''.join(lines))
                os.replace(tmp_path, DOCKER_DEPS_FILE)
            except BaseException:
                os.unlink(tmp_path)
                raise
            return True
    return False


def gather_results(deps, locked):
    """Returns list of dicts: {key, label, current, latest, outdated, locked, error}."""
    results = []
    for dep in DOCKER_DEPS:
        key = dep['key']
        current = deps.get(key)
        entry = {'key': key, 'label': dep['label'], 'current': current, 'latest': None,
                  'outdated': False, 'locked': key in locked, 'error': None, 'note': dep.get('note')}
        if current is None:
            entry['error'] = 'not set in docker-deps.env'
            results.append(entry)
            continue
        try:
            latest = dep['checker'](current)
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, OSError) as e:
            entry['error'] = f'lookup failed: {e}'
            results.append(entry)
            continue
        except Exception as e:  # defensive: never let one bad checker abort the whole run
            entry['error'] = f'lookup failed: {e}'
            results.append(entry)
            continue

        entry['latest'] = latest
        if latest and latest != current and _ver_key(latest) > _ver_key(current):
            entry['outdated'] = True
        results.append(entry)
    return results


def print_report(results, include_pinned=False):
    print('\nFrozen Docker dependency versions (docker/docker-deps.env):\n')
    width_label = max(len(r['label']) for r in results)
    width_current = max(len(r['current'] or '?') for r in results)
    for r in results:
        label = r['label'].ljust(width_label)
        current = (r['current'] or '?').ljust(width_current)
        if r['error']:
            status = f'⚠️  {r["error"]}'
        elif r['outdated'] and r['locked']:
            status = f'🔒 locked (newer available: {r["latest"]})'
        elif r['locked']:
            status = '🔒 locked, up to date'
        elif r['outdated']:
            status = f'→ newer available: {r["latest"]}'
        else:
            status = '✓ up to date'
        suffix = f'  ({r["note"]})' if r.get('note') else ''
        print(f'  {label}  {current}  {status}{suffix}')

    # "Actionable" = outdated and not locked, unless the caller explicitly
    # wants locked entries included (e.g. --include-pinned).
    actionable = [r for r in results if r['outdated'] and (include_pinned or not r['locked'])]
    locked_outdated = [r for r in results if r['outdated'] and r['locked'] and not include_pinned]
    errors = [r for r in results if r['error']]

    print()
    if actionable:
        print(f'{len(actionable)} update(s) available.')
    else:
        print('No updates available.' if locked_outdated else 'All Docker dependency versions are current.')
    if locked_outdated:
        print(f'{len(locked_outdated)} locked dependenc{"y is" if len(locked_outdated) == 1 else "ies are"} '
              f'behind but will not be touched (in PINNED_DOCKER_DEPS). Use --include-pinned to override.')
    if errors:
        print(f'{len(errors)} lookup(s) could not be completed (network issue or rate limit).')
    return actionable, errors


def apply_update(entry):
    ok = write_docker_dep(entry['key'], entry['latest'])
    if ok:
        print(f'  updated {entry["key"]}: {entry["current"]} -> {entry["latest"]}')
    else:
        print(f'  ⚠️  could not find {entry["key"]} line in {DOCKER_DEPS_FILE} to update')
    return ok


def run_interactive(actionable):
    apply_to_all = False
    for entry in actionable:
        if apply_to_all:
            apply_update(entry)
            continue
        while True:
            answer = input(
                f'Update {entry["key"]} ({entry["current"]} -> {entry["latest"]})? '
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


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--check', action='store_true',
                       help='Report only, never prompt or write. Exits 1 if anything unlocked is outdated.')
    mode.add_argument('--yes', '-y', action='store_true',
                       help='Non-interactive: update every outdated, unlocked entry automatically.')
    parser.add_argument('--include-pinned', action='store_true',
                         help='Also offer/apply updates to entries listed in PINNED_DOCKER_DEPS, '
                              'for this run only. Does not edit the lock list itself.')
    parser.add_argument('--skip-network', action='store_true',
                         help='Skip upstream lookups entirely (used by build-time hooks when '
                              'SKIP_DOCKER_DEPS_CHECK=1). Always exits 0 and makes no changes.')
    args = parser.parse_args()

    if not DOCKER_DEPS_FILE.exists():
        print(f'⚠️  {DOCKER_DEPS_FILE} not found - nothing to check.')
        return 0

    deps, locked = load_docker_deps()

    # Local, no-network check: Dockerfile ARG defaults must match
    # docker-deps.env. Runs even under --skip-network since it's free.
    drift = check_dockerfile_arg_drift(deps)
    if drift:
        print('⚠️  Dockerfile ARG defaults out of sync with docker/docker-deps.env:')
        for d in drift:
            print(f'  {d["file"]}: ARG {d["key"]}={d["dockerfile_default"]} '
                  f'(docker-deps.env has {d["docker_deps_value"]})')
        print()

    if args.skip_network:
        print('⏭️  Skipping Docker dependency version check (SKIP_DOCKER_DEPS_CHECK=1).')
        return 1 if (args.check and drift) else 0

    print('Checking latest upstream versions for Docker dependencies (needs network access)...')
    results = gather_results(deps, locked)
    actionable, errors = print_report(results, include_pinned=args.include_pinned)

    if args.check:
        return 1 if (actionable or drift) else 0

    if not actionable:
        return 0

    if args.yes:
        print('\nApplying all updates (--yes)...')
        for entry in actionable:
            apply_update(entry)
        return 0

    # Default: interactive if attached to a TTY, otherwise degrade to a
    # non-blocking report so this never hangs an automated build.
    if not sys.stdin.isatty():
        print('\nNon-interactive shell detected - not prompting.')
        print('Run `make update-docker-deps` (interactive) or `make update-docker-deps-yes` (auto) to update.')
        return 0

    print()
    run_interactive(actionable)
    return 0


if __name__ == '__main__':
    sys.exit(main())
