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

`RIVER_VERSION` (the `river` CLI) must match the `github.com/riverqueue/river`
version pinned in `go.mod`, so the CLI matches the library compiled into the
main binary - `scripts/check_docker_deps.py` checks it against `go.mod`
directly rather than against GitHub releases.

`RIVERUI_VERSION` is a **separate, independently-released tool** (a
standalone HTTP server, not linked into the main binary) and does not need
to - and generally won't - depend on the same underlying `river` version as
`RIVER_VERSION`/`go.mod`. Do not try to keep them in lockstep.

### Why river and riverui are built in separate Go modules

Both Dockerfiles cross-compile `river` and `riverui` in **two separate**
temp Go modules (`go mod init` + `go get` + `go build`, run twice), not one
shared module. This is deliberate, not incidental: if both tools are added
to the *same* temp module (`go mod init` once, then two `go get`s), Go's
minimum-version-selection unifies their two independent version
requirements into a single build list. Since `river` and `riverui` are
separate release trains, their pinned versions can depend on genuinely
different (and incompatible) versions of shared transitive packages -
concretely, `RIVERUI_VERSION`'s internal `river` dependency can be much
newer than `RIVER_VERSION`. When unified into one module, MVS may pick an
inconsistent mix and the build fails with a confusing compile error deep in
an unrelated vendored package (e.g. a missing interface method on a
database driver), giving no hint that the real cause is two Docker
dependency versions colliding.

This actually happened: pinning `RIVER_VERSION=v0.32.0` (to match `go.mod`)
while `RIVERUI_VERSION=v0.19.0` (latest, depending on `river v0.45.0`)
broke `Dockerfile.crosscompile`'s build once both were `go get` into one
shared module - it had only worked before because both were unpinned
`@latest`, which happened to self-select compatible versions. **If you ever
touch the river/riverui build steps, keep them in separate temp modules.**

## Checking / updating Docker dependency versions

| Command | What it does |
|---|---|
| `make check-docker-deps` | Read-only report. Exits `1` if anything unlocked is outdated. Use in CI. |
| `make update-docker-deps` | Interactive: prints the report, then asks per outdated/unlocked entry `[y]es/[N]o/[a]ll/[q]uit`. |
| `make update-docker-deps-yes` | Non-interactive: updates every outdated, unlocked entry automatically. |
| `python3 scripts/check_docker_deps.py` | Same as `make update-docker-deps`, runnable directly/independently of `make`. |
| `python3 scripts/check_docker_deps.py --check` | Same as `make check-docker-deps`. |
| `python3 scripts/check_docker_deps.py --yes` | Same as `make update-docker-deps-yes`. |
| `make verify-docker-deps [IMAGE=...]` | Smoke-tests that every pinned binary actually runs inside a *built* image (see below). |

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

## Smoke-testing the binaries in a built image

`make check-docker-deps` only compares version *numbers* against upstream -
it never builds or runs anything. To confirm the actual binaries in a built
image are present and invokable (not just that the version string is
current), build an image and run:

```
docker buildx build -f Dockerfile.crosscompile -t livereview:localtest --load .
make verify-docker-deps
```

This runs `scripts/verify_docker_deps.sh`, which `docker run`s each pinned
tool (`dbmate --version`, `river --help`, `riverui --help`,
`vl-convert --version`, `codebase-memory-mcp --version`, `dbctx --help`,
`alaws --help`) inside the image and reports pass/fail per tool, exiting
non-zero if any of them fail to run. It defaults to the image tag
`livereview:localtest`; point it at a different image with:

```
make verify-docker-deps IMAGE=ghcr.io/hexmostech/livereview:dev-abc123
```

or run it directly: `bash scripts/verify_docker_deps.sh <image>`.

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
