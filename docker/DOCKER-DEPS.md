# Frozen Docker dependency versions

> **Not the LiveReview app version.** LiveReview's own semver (Git tags,
> `scripts/lrops.py version`/`bump`, `make version*`) is a completely
> separate system. This document is only about the third-party ingredients
> baked *into* the Docker image.

`docker/docker-deps.env` is the single source of truth for every external
binary/base-image version baked into `Dockerfile` and `Dockerfile.crosscompile`
(node/golang/debian base images, dbmate, River CLI/UI, vl-convert,
codebase-memory-mcp, dbctx, AgentLaws). Bump a version there, never directly
in the Dockerfiles - both Dockerfiles declare a matching `ARG` per entry, and
`scripts/lrops.py` injects every entry as a `--build-arg` for each real
`docker`/`buildx` build it runs (single-arch and multiarch alike).

`RIVER_VERSION` must match the `github.com/riverqueue/river` version pinned
in `go.mod`, so the River CLI/UI tooling matches the library compiled into
the binary - `scripts/check_docker_deps.py` checks it against `go.mod`
directly rather than against GitHub releases.

## Checking / updating Docker dependency versions

| Command | What it does |
|---|---|
| `make check-docker-deps` | Read-only report. Exits `1` if anything unlocked is outdated. Use in CI. |
| `make update-docker-deps` | Interactive: prints the report, then asks per outdated/unlocked entry `[y]es/[N]o/[a]ll/[q]uit`. |
| `make update-docker-deps-yes` | Non-interactive: updates every outdated, unlocked entry automatically. |
| `python3 scripts/check_docker_deps.py` | Same as `make update-docker-deps`, runnable directly/independently of `make`. |
| `python3 scripts/check_docker_deps.py --check` | Same as `make check-docker-deps`. |
| `python3 scripts/check_docker_deps.py --yes` | Same as `make update-docker-deps-yes`. |

Each dependency is checked against its own real release channel: Docker Hub
tags for the base images (node/golang: latest patch within the currently
pinned major; debian: latest dated `trixie-YYYYMMDD-slim` tag), GitHub
releases for the binary tools, and `go.mod` for `RIVER_VERSION`.

## Automatic check before building

Every real Docker build (`make docker-build`, `docker-build-push`,
`docker-multiarch`, `docker-multiarch-push`, etc., via
`scripts/lrops.py:build_docker_image`) runs this check automatically right
before invoking `docker buildx build`:

- Attached to a TTY: interactive, same prompt as `make update-docker-deps`.
  Whatever you answer (including skipping everything), **the build still
  proceeds** - this never blocks a build, it only offers to update first.
- Not attached to a TTY (CI, scripts): auto-degrades to a non-blocking
  report and continues straight to the build.
- `--dry-run` builds skip this entirely (nothing is actually being built).

To skip the check altogether (no network calls, no prompt):

```
SKIP_DOCKER_DEPS_CHECK=1 make docker-multiarch-push
```

## Locking (pinning) a dependency

Some dependency should sometimes stay fixed even while others get bumped -
e.g. you've verified a specific `DEBIAN_IMAGE_TAG` against a known CVE
baseline and don't want it drifting on the next `make update-docker-deps-yes`.

Lock it by adding its `KEY` to the comma-separated `PINNED_DOCKER_DEPS=`
line at the top of `docker/docker-deps.env` (this line is always present,
even when the list is empty):

```
PINNED_DOCKER_DEPS=DEBIAN_IMAGE_TAG,DBMATE_VERSION
```

Locked entries:
- are still looked up and shown in every report, marked `🔒 locked`, so a
  locked dependency quietly falling behind stays visible
- are **never** offered or auto-applied by `make update-docker-deps`,
  `make update-docker-deps-yes`, or the automatic pre-build check
- do **not** count toward `make check-docker-deps`' exit code

To unlock, remove the `KEY` from the list. To override the lock for a single
run without editing it, pass `--include-pinned`:

```
python3 scripts/check_docker_deps.py --check --include-pinned
python3 scripts/check_docker_deps.py --yes --include-pinned
```

`PINNED_DOCKER_DEPS` itself is a control line for `check_docker_deps.py`,
not a Dockerfile `ARG` - `lrops.py` excludes it when building the
`--build-arg` list for `docker buildx build`.

## Adding a new dependency to this system

1. Add `SOME_TOOL_VERSION=...` to `docker/docker-deps.env`.
2. Add a matching `ARG SOME_TOOL_VERSION=...` (with the same default) to
   both `Dockerfile` and `Dockerfile.crosscompile`, in the stage that uses
   it, and reference `${SOME_TOOL_VERSION}` in the install/download step.
3. Add an entry to the `DOCKER_DEPS` list in `scripts/check_docker_deps.py`
   with a `checker` function that resolves "latest" for that tool (there
   are existing helpers for GitHub releases and Docker Hub tags to reuse).
