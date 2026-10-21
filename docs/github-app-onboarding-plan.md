# LiveReview: `/gh-login`, a 5-minute first review via a GitHub App

> **Implementation plan. Written so an engineer or a model (e.g. Sonnet) can work through it task by task.**
> Every task lists the files, what to change, a code sketch, and how to know it's done.
> Line numbers were checked against the code on 2026-10-19. If one has drifted, find the symbol by name; don't guess.

---

## 0. Rules for whoever implements this

1. **Work in the PR order in §9.** Each PR builds and passes tests on its own. Don't start a PR before the previous one is merged or at least green.
2. **Storage/network boundary (repo rule):**
   - SQL goes only in `storage/…`.
   - Outbound HTTP goes only in `network/…`.
   - Every function you add or move there gets a row in `storage/storage_status.md` / `network/network_status.md`.
   - Run `make check-status-doc` before calling a task done.
3. **No fallback behavior** (repo rule). When the connector, state or installation can't be resolved, return an error. Never fall back to "latest token", "any org" or "skip the check".
4. **Don't touch** `onelogin-node`, `ui/src/pages/Auth/Cloud.tsx` (behavior), `SelfHosted.tsx`, or the Git Providers PAT flow, except where a task explicitly says so.
5. **UI:**
   - Read `.agents/design.md` before styling anything.
   - Type-check with `cd ui && npx tsc --noEmit`.
   - **Never run `npm run build` / `make build-with-ui` without asking the human first.**
   - Every new page gets a `megaMenuData.ts` entry and a routes_guide doc.
6. **Tests:** run the narrowest test you can (`go test -count=1 ./path/ -run Name`). Don't run `make test` for a targeted change.
7. **Secrets** (App private key, client secret, webhook secret, user tokens):
   - Never log them.
   - Never send them to the UI.
   - Never put them in a URL. The one exception is the Hexmos JWT in `/?data=`, which is the existing onelogin contract.
8. Comments and naming follow the surrounding code. No new frameworks.

---

## 1. Context and decisions

Trials go cold because step 1 asks too much: create a classic PAT (`repo, read:org`), paste an AI key, install the CLI. For the Product Hunt launch the target is **GitHub login → install the LiveReview GitHub App on selected repos → first review**, on one GitHub screen with no PAT and no webhook setup. CodeRabbit works this way.

The Hexmos onelogin GitHub button is an OAuth App with `read:user,user:email` (`onelogin-node/src/shared/src/components/Home.js:110`). It gives identity but no repo access, so it can't do this.

**Decisions (from the discussion with Shrijith):**
- Add a separate path, **`<instance>/gh-login`**.
  - It does GitHub App install and authorization in one GitHub screen, then signs the user in, then shows the demo onboarding (`#/get-started/github`), then the first review.
  - "Other ways to sign in" on that page goes to the existing login, which stays exactly as it is.
- Landing-page "Start Free" CTAs point to `/gh-login`.
- **Cloud users must exist in fw-parse with a Hexmos JWT.** They are created through the same Parse function onelogin uses (`externalProviderSignup`), so a later Google or email sign-in lands in the same account.
- **Cloud first.** Everything is config-driven (no hard-coded hosts) so self-hosted, including custom domains and GHES, can turn it on later. Self-hosted doesn't use fw-parse (§8, design only, not in launch scope).
- Bringing the GitHub App into the Git Providers page comes later. For now an App connector only has to *not break* the existing screens (T3.8).

---

## 2. Glossary (tokens and IDs; mixing these up is the #1 bug source)

| Name | Looks like | Lifetime | Used for | Where it lives |
|---|---|---|---|---|
| **App JWT** | RS256 JWT, `iss=<GITHUB_APP_ID>` | ≤10 min (we use 9) | Calling `/app/...` endpoints (mint installation tokens, get app info) | Never stored; minted on demand |
| **Installation token** | `ghs_…` | 1 hour | Everything repo-related: read PRs, post comments and reviews, list installation repos | Cached in `integration_tokens.pat_token` + `expires_at` |
| **User token** | `ghu_…` (+ `ghr_…` refresh) | 8 h | Only during the callback: `/user/emails`, `/user/installations`, and handing to fw-parse | **Never stored** |
| **installation_id** | integer, e.g. `51234567` | until uninstalled | Identifies one install on one account (user or org) | `integration_tokens.provider_app_id` (text) |
| **Hexmos JWT** | HS256 JWT from fw-parse | 30 days | Hexmos identity; `ensure-cloud-user` validates it with `CLOUD_JWT_SECRET` | Parse `JWT` class; passed to the UI in `/?data=` |
| **LiveReview tokens** | access + refresh pair | existing | LiveReview session | Existing `auth_tokens` table (unchanged) |

`GET /user` **does not work** with an installation token (it returns 403 "Resource not accessible by integration"). Any code that calls `/user` to find "the bot" must special-case App connectors (T3.5).

---

## 3. Configuration (nothing hard-coded)

| What | Source | Notes |
|---|---|---|
| App ID | `GITHUB_APP_ID` (int) | If unset, **the feature is off** (no routes active, `ui-config.githubAppEnabled=false`). |
| App slug | `GITHUB_APP_SLUG` | Used for the install URL and the bot login `<slug>[bot]`. |
| OAuth client | `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_CLIENT_SECRET` | From the App settings page (not the onelogin OAuth App). |
| Private key | `GITHUB_APP_PRIVATE_KEY` | Either the PEM text (starts with `-----BEGIN`) or a file path. Replace a literal `\n` with a newline before parsing; env files often store it escaped. |
| Webhook secret | `GITHUB_APP_WEBHOOK_SECRET` | Required. |
| GitHub host | `GITHUB_APP_BASE_URL`, default `https://github.com` | API base comes from `apiBaseURL()` in `internal/providers/github/base_url.go:8` (github.com → `https://api.github.com`, GHES → `<base>/api/v3`). Export it (T2.1). |
| fw-parse | existing `FW_PARSE_BASE_URL` (no trailing `/parse`), `FW_PARSE_APP_ID` (default `impressionserver`), `FW_PARSE_ADMIN_SECRET` | Same as `network/email/invitation.go:28,72-93`. Cloud only. |
| Hexmos JWT secret | existing `CLOUD_JWT_SECRET` | Already used by `EnsureCloudUser`. |
| State signing | existing `JWT_SECRET` | HMAC key for the OAuth `state`. |
| Mode | existing `LIVEREVIEW_IS_CLOUD` → `s.deploymentConfig.IsCloud` (`internal/api/server.go:110`) | Selects the identity finisher. |
| UI | `GET /api/v1/ui-config` (`server.go:3047`) | Adds `githubAppEnabled` (bool) and `githubAppBaseUrl` (string). No IDs or secrets. |
| Landing page | one constant in hexmoshomepage `urls.js` (`LIVEREVIEW_APP`) | Replaces the inline `https://livereview.hexmos.com` strings. |

**Partial config** (some `GITHUB_APP_*` set, others missing, or an unparsable key): log one clear error at startup naming the missing vars, and treat the feature as off. Don't crash the server.

**Redirects are always relative** (`/`, `/#/…`), so they work on any host. The callback and webhook URLs live only in the GitHub App settings.

---

## 4. User flow

```
Landing "Start Free" / pasted PR URL
   │  <instance>/gh-login?src=<cta>
   ▼
index.tsx: pathname '/gh-login' → location.replace('/#/gh-login' + search)
   ▼
#/gh-login (GhLogin.tsx)   [ Continue with GitHub · FREE ]   "Already installed? Sign in"   "Other ways to sign in"
   │  sessionStorage.redirectAfterLogin = '#/get-started/github'
   │  location.href = '/api/v1/auth/github-app/start?mode=install&src=<src>'
   ▼
GET /api/v1/auth/github-app/start   → Set-Cookie lr_gh_state=<nonce>; 302 to
   install:  https://github.com/apps/<slug>/installations/new?state=<signed>
   signin:   https://github.com/login/oauth/authorize?client_id=<id>&state=<signed>
   ▼
GitHub (user picks repos → "Install & Authorize")
   │  302 → /api/v1/auth/github-app/callback?code=…&installation_id=…&setup_action=install&state=…
   ▼
callback:
   1 verify state + cookie        2 exchange code → user token
   3 primary+verified email       4 installation ∈ GET /user/installations
   5 CLOUD: fw-parse externalProviderSignup(github_app, userToken) → {result:{jwt,data}}
            EnsureCloudUserCore(jwt, email, source="gh-login:<src>") → user_id, org_id
   6 upsert GITHUB_APP connector(s) for org_id (reject if bound to another org)
   7 mint + cache installation token; queue repo sync (async)
   8 302 /?data=<slim Parse result>
   ▼
Cloud.tsx (unchanged): parses ?data= → POST ensure-cloud-user (idempotent) → LR tokens
   → redirectAfterLogin → #/get-started/github
   ▼
#/get-started/github: repos ✓ → AI → open PRs / paste box → "Review this PR" → existing trigger-review → review page
```

**One button for everyone.** "Continue with GitHub" always starts in `signin` mode (GitHub's authorize screen; the same order CodeRabbit uses). The callback lists `/user/installations` and connects every installation of *this App* that the user can access and that isn't bound to another org (§6.3). The user is signed in either way:
- **Installation(s) found:** they land on `#/get-started/github` with their repositories.
- **None:** the account is still created (Parse user, LiveReview user and org), but no session is issued. The callback redirects straight to GitHub's install page (`mode=install`); after "Install & Authorize" it runs again with `installation_id`, connects it, and ends in `/?data=` as usual. No login screen shows in between. If the user abandons the install and later opens LiveReview logged in, get-started (never shown in that state) sends them to the install page again.

A "no in-app org picker" decision: CodeRabbit lists the user's orgs itself using an OAuth App with `read:org`. A GitHub App user token gets an empty list from `GET /user/orgs`, so we use GitHub's own account picker on the install page instead.

---

## 5. Data model

### 5.1 How an App installation is stored

One `integration_tokens` row per installation. All NOT NULL columns must be filled:

| column | value |
|---|---|
| `provider` | `'github'` (so every existing `strings.HasPrefix(provider,"github")` branch picks it up) |
| `token_type` | `'GITHUB_APP'` (existing PAT rows use `'PAT'`) |
| `provider_app_id` | installation id as text, e.g. `'51234567'` |
| `provider_url` | `GITHUB_APP_BASE_URL` (e.g. `https://github.com`) |
| `access_token` | `''` (NOT NULL; unused for App rows) |
| `pat_token` | **current installation token** (`ghs_…`), refreshed before expiry |
| `expires_at` | the installation token's `expires_at` |
| `connection_name` | `'GitHub App · <account_login>'` |
| `org_id` | the LiveReview org |
| `metadata` | `{"github_app":true,"installation_id":51234567,"account_login":"acme","account_type":"Organization","repository_selection":"selected","app_slug":"<slug>","suspended":false,"removed":false}` |

**Why the token goes in `pat_token`:** about 15 existing readers do `SELECT pat_token FROM integration_tokens WHERE id=$1`:
- `scheduled_review_worker.go:80`
- `blast_radius_worker.go:197`
- `repo_sync_worker.go:54`
- `diff_review.go:543`
- `connectors.go:492,665`
- `repository_access.go:66`
- `server.go:2868,2942`
- `auto_webhook_installer.go:258`
- the unified processor queries

Keeping a fresh installation token there (T3.4 refresher + T3.3 on-demand resolver) makes them all work without rewriting each one. Hot paths still call the resolver, so they never use a stale token.

### 5.2 Migration `db/migrations/20261020090000_github_app_installations.sql`

```sql
-- migrate:up
-- One GitHub App installation can belong to exactly one LiveReview org.
CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github_app_installation
    ON integration_tokens (provider_url, provider_app_id)
    WHERE token_type = 'GITHUB_APP';

-- How the org was created; 'gh_app' for orgs created through /gh-login. Analytics + onboarding banner only.
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS signup_path TEXT;

-- migrate:down
DROP INDEX IF EXISTS uq_integration_tokens_github_app_installation;
ALTER TABLE orgs DROP COLUMN IF EXISTS signup_path;
```

Run `dbmate up` locally. dbmate rewrites `db/schema.sql`, so commit that too.

(Self-hosted needs `gh_login_codes`. It's added in §8 later, not now.)

---

## 6. Tasks

Task IDs: **T1.x** fw-parse, **T2.x** LiveReview cross-tenant fix, **T3.x** LiveReview App backend, **T4.x** UI, **T5.x** hexmoshomepage. **T0.x** is human setup.

### Phase 0: human setup (done once, by a person, not the model)

**T0.1 Create the GitHub App** under the HexmosTech org with the final public name. The name becomes the slug and the `<slug>[bot]` identity.
- Homepage: `https://hexmos.com/livereview`
- **Callback URL:** `https://manual-talent3.apps.hexmos.com/api/v1/auth/github-app/callback`
- ✅ "Expire user authorization tokens"
- ✅ **"Request user authorization (OAuth) during installation"**. This greys out the Setup URL, which is expected.
- **Webhook URL:** `https://manual-talent3.apps.hexmos.com/api/v1/github-app/webhook`, plus a webhook secret. Make it random, 32+ bytes.
- **Repository permissions:** Pull requests R/W, Contents R, Issues R/W, Metadata R.
- **Account permissions:** Email addresses R.
- **Subscribe to events:** Pull request, Pull request review comment, Issue comment. (`installation` and `installation_repositories` are always sent.)
- "Where can this app be installed": **Any account**.
- Generate a private key and save the `.pem`. Copy the App ID, Client ID, generate a Client secret, and note the slug.

**T0.2 Local env** (`LiveReview/.env`):
```
GITHUB_APP_ID=…
GITHUB_APP_SLUG=…
GITHUB_APP_CLIENT_ID=Iv23…
GITHUB_APP_CLIENT_SECRET=…
GITHUB_APP_PRIVATE_KEY=/abs/path/to/app.private-key.pem
GITHUB_APP_WEBHOOK_SECRET=…
GITHUB_APP_BASE_URL=https://github.com
LIVEREVIEW_IS_CLOUD=true
FW_PARSE_BASE_URL=<dev fw-parse running T1.x, no trailing /parse>
CLOUD_JWT_SECRET=<same secret the dev fw-parse signs JWTs with>
```
Run: `make develop` (backend :8888), `cd ui && make run` (:8081), `make niceurl3` (manual-talent3 → :8081, which proxies `/api` → :8888).

**T0.3 Confirm `state` round-trips.** After T3.6 exists, install once and check the callback log for a `state` value. GitHub forwards `state` from the install URL. If it doesn't, stop and tell the human; don't remove the state check.

**T0.4 At go-live:**
- Change the same App's callback and webhook URLs to `https://livereview.hexmos.com/...`. The ID, keys and secrets stay the same.
- Set the env vars in prod.
- **Uninstall the dev test installations first**: they're bound to the local DB, and their webhooks would start hitting prod.

---

### Phase 1: fw-parse (PR A, repo `fw-parse`)

Goal: `POST /parse/functions/externalProviderSignup` with `externalServiceProvider: "github_app"` and an already-exchanged GitHub **user token** returns a Hexmos JWT for the token's **primary, verified** email, creating the Parse `_User` if needed. Existing providers behave exactly as before.

**T1.1 `cloud/oneLogin/githubOneLogin.js`.** Add a new exported function. Don't change `getGithubUserData`.
```js
// For the LiveReview GitHub App sign-in: only a primary *verified* email is accepted.
getGithubAppUserData: async function (accessToken) {
  const headers = { Accept: "application/vnd.github+json", Authorization: `Bearer ${accessToken}`, "X-GitHub-Api-Version": "2022-11-28" }
  const profileRes = await fetch(GITHUB_USER_API_URL, { headers })
  if (!profileRes.ok) throw new Error(`GitHub /user failed: ${profileRes.status}`)
  const profile = await profileRes.json()
  const emailsRes = await fetch(GITHUB_EMAIL_API_URL, { headers })
  if (!emailsRes.ok) throw new Error(`GitHub /user/emails failed: ${emailsRes.status}`)
  const emails = await emailsRes.json()
  const primary = Array.isArray(emails) ? emails.find((e) => e.primary === true && e.verified === true) : null
  if (!primary) throw new Error("GitHub account has no verified primary email")
  return { email: primary.email.toLowerCase(), name: profile.name || profile.login, picture: profile.avatar_url }
}
```
Unlike `getGithubUserData`, it **throws** instead of returning `null`, so the caller can't continue with `email=undefined`.

**T1.2 `cloud/oneLogin/providerMethods.js` `externalMethods` (line 20).** Add a branch:
```js
} else if (provider == "github_app") {
  simpleUserDate = await getGithubAppUserData(accessToken)
}
```
Import it next to `getGithubUserData` (line 2).

**T1.3 `cloud/oneLogin/externalProviderSignin.js` (line 8).** Add `github_app` to the provider `if` chain. Without this the function returns "Unknown provider" even after T1.2.
```js
} else if (externalServiceProvider === "github_app") {
  return await externalMethods(params)
```

**T1.4 `isAuthenticatedMiddleware.js` (line 146).** No behavior change is needed: the `github` branch only matches `"github"`, so `github_app` already skips the code exchange and goes straight to `externalMethods`. Add a short comment above the `if` chain for whoever reads it next:
```js
// "github_app" (LiveReview GitHub App) sends an already-exchanged user token, so no code exchange here.
```

**T1.5 Test** in `__tests__/` (these are integration tests against a running Parse, following `__tests__/testHelper.js`):
- `github_app` with a bogus token → `result.success === false`. (The function catches the error and returns `{success:false}` with HTTP 200.)
- Unit-test `getGithubAppUserData` with `fetch` mocked (jest `jest.mock("node-fetch")` or by stubbing `global.fetch`):
  - an unverified primary email → throws;
  - a verified primary email → returns the lowercase email.

**Done when:** a manual `curl` against dev Parse with a real `ghu_` token returns `{"result":{"jwt":"…","data":{"email":…}}}`, and a Parse `_User` with that email exists.

Out of scope: moving the hard-coded GitHub secret at `cloud/config.js:119` to env. That's separate hygiene work.

---

### Phase 2: cross-tenant fix (PR B, LiveReview; ship before the App)

**Existing bug:** `storage/providers/github/token_store.go:21` `GetLatestGitHubToken()` returns the newest GitHub connector **across all orgs**. Every GitHub webhook-path call goes through it via `FindIntegrationTokenForGitHubRepo` (`internal/provider_input/github/github_token_helpers.go:27`), so with two GitHub orgs, org B's webhook gets org A's token. The App would make this common, so fix it first, on its own.

**T2.1 Export the API base helper.** `internal/providers/github/base_url.go`: add
```go
// APIBaseURL is the exported form of apiBaseURL for callers outside this package.
func APIBaseURL(baseURL string) string { return apiBaseURL(baseURL) }
```

**T2.2 Storage: connector-scoped lookup.** `storage/providers/github/token_store.go`:
- Add `OrgID int64`, `TokenType string` and `ExpiresAt sql.NullTime` to `IntegrationTokenRecord`.
- Add:
```go
// GetGitHubTokenByConnectorID loads one GitHub connector by primary key.
func (s *TokenStore) GetGitHubTokenByConnectorID(connectorID int64) (*IntegrationTokenRecord, error) {
	const q = `
		SELECT id, provider, provider_url, COALESCE(pat_token,''), COALESCE(metadata,'{}'), org_id,
		       COALESCE(token_type,''), expires_at
		FROM integration_tokens
		WHERE id = $1 AND provider LIKE 'github%'`
	…
}
```
- **Delete** `GetLatestGitHubToken`.
- Update `storage/storage_status.md`: add the new function and mark the old one removed.

**T2.3 Carry the connector ID on the event, early.** `internal/api/webhook_orchestrator_v2.go` `ProcessWebhookEvent`:
- Move the block that stamps `connector_id` into `event.Repository.Metadata` / `event.MergeRequest.Metadata` (currently lines ~254-297, after the warrant check) to **right after** `convertToUnifiedEvent` succeeds (after line ~206, before `getBotUserInfo` at ~215). `getBotUserInfo` and `CheckResponseWarrant` need the token.
- Read `connectorID` from `auth.GetConnectorIDFromContext(c)` there. If it's missing, return the same 500 `internal_error` as today.
- Leave the later `orgID` read and the signature check where they are, just reuse the variable.

**T2.4 Resolve tokens by connector.** `internal/provider_input/github/github_token_helpers.go`:
```go
// connectorIDFromMetadata reads connector_id stamped by the webhook orchestrator. After a JSON
// round trip through the job queue it arrives as float64, so accept the numeric forms.
func connectorIDFromMetadata(meta map[string]interface{}) (int64, error) {
	switch v := meta["connector_id"].(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	}
	return 0, fmt.Errorf("connector_id missing from event metadata")
}

// FindIntegrationTokenForGitHubConnector loads the GitHub connector that received this event.
func FindIntegrationTokenForGitHubConnector(db *sql.DB, connectorID int64) (*IntegrationToken, error)
```
- Delete `FindIntegrationTokenForGitHubRepo`.
- Add `OrgID` and `TokenType` to `IntegrationToken`.
- Callers in `internal/provider_input/github/github_provider.go` (all of them, by current line):

| line | method | connector comes from |
|---|---|---|
| 145 | `FetchMergeRequestData(event)` | `event.MergeRequest.Metadata` (fall back to `event.Repository.Metadata`, which is the same value, not a different token) |
| 190-198 | `FindIntegrationTokenForRepo(repoFullName)` | **Change the signature** to `FindIntegrationTokenForEvent(event UnifiedWebhookEventV2)` and update its caller `internal/api/unified_processor_v2.go:735` |
| 258 | `PostCommentReply` | `event.Repository.Metadata` |
| 272 | `PostEmojiReaction` | `event.Repository.Metadata` |
| 286 | `PostFullReview` | `event.Repository.Metadata` |
| 307 | `FetchMRTimeline(mr)` | `mr.Metadata` |
| 409 | `GetBotUserInfo(repository)` | `repository.Metadata` |
| 440 | `PostReviewComments(mr, …)` | `mr.Metadata` |

- `internal/provider_input/github/lrc_fetch.go:28` `GetRepoConfigFiles(ctx, repoFullName, ref)` has no metadata (it implements the `lrcfetch.Provider` interface). Add in `github_token_helpers.go`:
```go
type connectorCtxKey struct{}
func WithConnectorID(ctx context.Context, id int64) context.Context { return context.WithValue(ctx, connectorCtxKey{}, id) }
func connectorIDFromContext(ctx context.Context) (int64, bool) { id, ok := ctx.Value(connectorCtxKey{}).(int64); return id, ok }
```
  The caller at `internal/api/webhook_orchestrator_v2.go:1084` wraps `ctx` with `githubinput.WithConnectorID(ctx, connectorID)`, using whatever import alias that file already uses for `internal/provider_input/github`. If the ID is missing, return an error (no fallback). `internal/review/service.go:497` calls the *other* `GetRepoConfigFiles` (the `internal/providers/github` PAT-configured provider), so leave it alone.

**T2.5 Tests.** `internal/provider_input/github/github_token_helpers_test.go`:
- `connectorIDFromMetadata` handles int64, float64, json.Number and missing.

`internal/api/webhook_orchestrator_v2_test.go` (follow the existing setup in that file):
- Two orgs, each with a GitHub PAT connector.
- A webhook on `/github-hook/<A>` must load connector A's token, never B's.

**Done when:**
- `grep -rn "GetLatestGitHubToken\|FindIntegrationTokenForGitHubRepo" --include=*.go .` is empty.
- `go build ./...` passes.
- The tests above pass.
- `make check-status-doc` passes.

---

### Phase 3: GitHub App backend (PR C, LiveReview)

**T3.1 Config package**: `internal/githubapp/config.go` (new package; no DB, no HTTP).
```go
package githubapp

type Config struct {
	AppID         int64
	Slug          string
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	BaseURL       string          // e.g. https://github.com (no trailing slash)
	PrivateKey    *rsa.PrivateKey
}

// Load reads GITHUB_APP_* once. It returns nil when the feature is off (GITHUB_APP_ID unset)
// or misconfigured (logged once, naming the missing vars).
func Load() *Config            // sync.Once around fromEnv()
func fromEnv() (*Config, error) // testable; returns (nil, nil) when GITHUB_APP_ID is empty

func (c *Config) APIBaseURL() string { return githubprovider.APIBaseURL(c.BaseURL) }
func (c *Config) IsGitHubDotCom() bool { return c.BaseURL == "https://github.com" }
func (c *Config) InstallURL(state string) string   // github.com: <base>/apps/<slug>/installations/new?state=…
                                                   // GHES:       <base>/github-apps/<slug>/installations/new?state=…
func (c *Config) AuthorizeURL(state string) string // <base>/login/oauth/authorize?client_id=…&state=…
func (c *Config) InstallationSettingsURL(accountType, accountLogin string, installationID int64) string
	// User:         <base>/settings/installations/<id>
	// Organization: <base>/organizations/<login>/settings/installations/<id>
func (c *Config) BotLogin() string { return c.Slug + "[bot]" }
```
- Key parsing: if the value starts with `-----BEGIN`, use it after replacing `\n` (the literal two characters) with newlines; otherwise `os.ReadFile` it. Parse with `jwt.ParseRSAPrivateKeyFromPEM` (`github.com/golang-jwt/jwt/v5`, already in `go.mod`).
- Test `config_test.go`:
  - unset → nil, nil;
  - partial → error naming the vars;
  - PEM inline with `\n` → parses (generate a key in the test with `rsa.GenerateKey`);
  - URLs for github.com vs GHES.

**T3.2 Network calls**: `network/providers/github/app.go` (new). Each function takes `apiBase` (or `base` for github.com web endpoints) explicitly. No literal hosts. Use the package's existing `NewHTTPClient` / `NewRequestWithContext` / `Do` (`network/providers/github/http_client_ops.go`). Set headers `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`, `User-Agent: LiveReview`.
```go
func MintAppJWT(appID int64, key *rsa.PrivateKey, now time.Time) (string, error)
	// RS256, claims: iat=now-60s, exp=now+9m, iss=strconv.FormatInt(appID,10)

type InstallationToken struct { Token string; ExpiresAt time.Time }
func CreateInstallationToken(ctx context.Context, apiBase, appJWT string, installationID int64) (*InstallationToken, error)
	// POST {apiBase}/app/installations/{id}/access_tokens, Authorization: Bearer <appJWT>; expect 201

type UserToken struct { AccessToken string }
func ExchangeUserCode(ctx context.Context, base, clientID, clientSecret, code string) (*UserToken, error)
	// POST {base}/login/oauth/access_token, form-encoded client_id, client_secret, code; Accept: application/json.
	// TRAP: GitHub answers 200 with {"error":"bad_verification_code",...} on failure. Treat a non-empty "error" as failure.

type UserEmail struct { Email string; Primary, Verified bool }
func ListUserEmails(ctx context.Context, apiBase, userToken string) ([]UserEmail, error)        // GET /user/emails

type UserProfile struct { Login, Name, AvatarURL string; ID int64 }
func GetAuthenticatedUser(ctx context.Context, apiBase, userToken string) (*UserProfile, error)  // GET /user (user token only!)

type Installation struct {
	ID int64; AppID int64; AccountLogin, AccountType, RepositorySelection string; Suspended bool
}
func ListUserInstallations(ctx context.Context, apiBase, userToken string) ([]Installation, error)
	// GET /user/installations?per_page=100, follow the Link header. Map account.login, account.type, app_id, suspended_at != null.

type InstallationRepo struct {
	ID int64; Name, FullName, HTMLURL, CloneURL, SSHURL, DefaultBranch, Description string; Private bool
}
func ListInstallationRepositories(ctx context.Context, apiBase, installationToken, cursor string) (repos []InstallationRepo, next string, err error)
	// GET /installation/repositories?per_page=100 → {"total_count":n,"repositories":[...]}. cursor = next page URL from the Link header.

type OpenPull struct { Number int; Title, HTMLURL, AuthorLogin string; UpdatedAt time.Time }
func ListOpenPulls(ctx context.Context, apiBase, installationToken, repoFullName string, limit int) ([]OpenPull, error)
	// GET /repos/{full}/pulls?state=open&sort=updated&direction=desc&per_page=<limit>

type PublicUser struct { ID int64; Login string }
func GetUserByLogin(ctx context.Context, apiBase, token, login string) (*PublicUser, error)
	// GET /users/{login}; login "<slug>[bot]" must be path-escaped
```
- Test `app_test.go` with `httptest.NewServer`:
  - `MintAppJWT` round-trips with the public key and `iss` is right;
  - `ExchangeUserCode` returns an error on a 200 + `{"error":…}` response;
  - `ListUserInstallations` follows two pages;
  - `CreateInstallationToken` parses `expires_at`.
- Add every function to `network/network_status.md`.

**T3.3 Token resolver**: `internal/githubapp/token.go`. It lives here, not in `provider_input`, so `internal/api`, `internal/provider_input/github` and `internal/jobqueue` can all import it without cycles.

Storage first, in `storage/providers/github/token_store.go`:
```go
type AppConnectorUpsert struct {
	OrgID int64; BaseURL string; InstallationID int64; AccountLogin, AccountType, RepositorySelection, AppSlug string
	Token string; ExpiresAt time.Time
}
// UpsertAppInstallationConnector inserts or refreshes the GITHUB_APP row for (BaseURL, InstallationID).
// It returns ErrInstallationBoundToOtherOrg when the row exists with a different org_id, and never moves it.
func (s *TokenStore) UpsertAppInstallationConnector(in AppConnectorUpsert) (connectorID int64, created bool, err error)
func (s *TokenStore) GetAppConnectorByInstallation(baseURL string, installationID int64) (*IntegrationTokenRecord, error)
func (s *TokenStore) UpdateCachedInstallationToken(connectorID int64, token string, expiresAt time.Time) error
func (s *TokenStore) ListAppConnectorsExpiringBefore(t time.Time) ([]IntegrationTokenRecord, error) // skips metadata.removed/suspended
func (s *TokenStore) SetAppConnectorFlags(connectorID int64, removed, suspended *bool) error         // jsonb_set on metadata; on removed=true also SET pat_token=''
func (s *TokenStore) ListAppConnectorsForOrg(orgID int64) ([]IntegrationTokenRecord, error)
var ErrInstallationBoundToOtherOrg = errors.New("github app installation is connected to another LiveReview organization")
```
Write the upsert as a transaction: `SELECT … FOR UPDATE` on `(provider_url, provider_app_id) WHERE token_type='GITHUB_APP'`. If the row exists with a different `org_id`, return the error. If it exists with the same org, update. If it doesn't exist, insert. The unique index from §5.2 backs this up against races.

Resolver:
```go
// ConnectorToken returns a usable token for any GitHub connector. PAT rows return pat_token
// unchanged; GITHUB_APP rows return the cached installation token, minting a new one when it
// expires within 5 minutes.
func ConnectorToken(ctx context.Context, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (string, error)
func RefreshConnectorToken(ctx context.Context, store *storagegithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (string, error) // unconditional mint + UpdateCachedInstallationToken
```
- Get the installation ID from `rec.Metadata` `installation_id`, or parse `provider_app_id`.
- If `Load()` is nil and the row is `GITHUB_APP`, return the error `"github app not configured on this server"`.
- If the row is removed or suspended, return the error `"github app installation removed"`.

Wire it in:
- `internal/provider_input/github/github_token_helpers.go` `FindIntegrationTokenForGitHubConnector` (T2.4): after loading, `token.PatToken, err = githubapp.ConnectorToken(...)`. Every webhook-path caller then gets a valid token.
- `internal/api/review_service.go` `prepareAuthentication` (line 350): after `findIntegrationToken`, when `token.TokenType == "GITHUB_APP"`, call the resolver (load the record by `token.ID`) and set `token.PatToken`.
- `internal/api/reviews_api.go`:
  - `ensureValidToken` (line 262): the GitHub branch accepts `TokenType == "PAT" || TokenType == "GITHUB_APP"`.
  - `buildProviderConfig` (~line 405): the condition `token.TokenType == "PAT"` becomes `token.TokenType == "PAT" || token.TokenType == "GITHUB_APP"`, so `pat_token` ends up in the provider config.

**T3.4 Background refresh**: `internal/jobqueue/github_app_token_refresh_worker.go`.
- A River periodic job:
  - `GitHubAppTokenRefreshJobArgs{}`, `Kind() = "github_app_token_refresh"`.
  - Interval 10 min, `RunOnStart: true`.
  - Register it next to the other periodic jobs in `internal/jobqueue/jobqueue.go` (~line 2510-2540), **only when `githubapp.Load() != nil`**.
  - Register the worker the way `ReconciliationSweepWorker` is registered (~line 2425).
- Work: `ListAppConnectorsExpiringBefore(now+20m)` → `RefreshConnectorToken` for each. Log failures per connector and keep going. Return nil unless the DB query fails.
- This keeps `pat_token` fresh for every legacy reader in §5.1.

**T3.5 Bot identity for App connectors.** `internal/provider_input/github/github_provider.go` `GetBotUserInfo` (line 408): if `token.TokenType == "GITHUB_APP"`:
- Login = `cfg.BotLogin()`.
- ID comes from `GetUserByLogin(apiBase, token, cfg.BotLogin())`, cached in a package-level `sync.Map` keyed by login.
- Return `IsBot: true` and `user_type: "Bot"`.
- **Don't** call `FetchGitHubBotUserInfo` (`GET /user`). It 403s for installation tokens.

Then check `CheckResponseWarrant` / the mention detection in `internal/api/unified_processor_v2.go`. If it matches mentions against `botInfo.Username`, also accept `@<slug>` without the `[bot]` suffix, because that's what users type. Add a test for both.

**T3.6 HTTP endpoints**: new file `internal/api/github_app_auth.go`, routes registered in `internal/api/server.go`. Register next to `public.POST("/auth/ensure-cloud-user", …)` at ~line 1204, only when `s.githubApp != nil`, where `s.githubApp = githubapp.Load()` is set in `NewServer`:
```go
public.GET("/auth/github-app/start", s.GitHubAppStart)
public.GET("/auth/github-app/callback", s.GitHubAppCallback)
v1.POST("/github-app/webhook", s.GitHubAppWebhook) // next to /github-hook/:connector_id (~line 1609); no connector middleware
```

*State* (`internal/api/github_app_state.go`):
```go
type ghState struct { Nonce, Src, Mode string; Exp int64 }
// sign: base64url(json) + "." + base64url(HMAC-SHA256(JWT_SECRET, "gh-app-state:"+base64url(json)))
func signGHState(s ghState, secret []byte) string
func verifyGHState(raw string, secret []byte, now time.Time) (*ghState, error) // constant-time compare, exp check
```
- `src` comes from the start query. Allow only `[a-z0-9_-]{1,32}`; anything else becomes `"unknown"`.
- `mode` is `install` or `signin` (default `install`).
- Cookie: `lr_gh_state=<nonce>; Path=/api/v1/auth/github-app; HttpOnly; SameSite=Lax; Max-Age=600`. Add `Secure` when `c.Scheme()=="https"` or `X-Forwarded-Proto: https`. **Lax, not Strict**: the return from GitHub is a cross-site top-level GET.

*`GitHubAppStart`*: build the state (exp = now+10m), set the cookie, `302` to `InstallURL(state)` or `AuthorizeURL(state)`.

*`GitHubAppCallback`*. Every failure renders one small error page (T3.9) with a message and a "Try again" link to `/#/gh-login`. **Never 500 with a stack trace.**
1. Read `state`, `code`, `installation_id`, `setup_action`.
   - If `setup_action == "request"`, an org member asked an org owner to approve. Show the page: "Installation requested. An owner of <org> needs to approve it on GitHub. You'll get an email." Stop.
2. `verifyGHState`, and the cookie nonce must equal `state.Nonce`. Delete the cookie (Max-Age=-1) either way.
3. `ExchangeUserCode` → user token.
4. `ListUserEmails` → pick `Primary && Verified`. If there's none, show the error "Your GitHub account needs a verified primary email."
5. `ListUserInstallations` → keep only `inst.AppID == cfg.AppID`.
   - If `installation_id` is present, it **must** be in that list. Otherwise reject: "That installation doesn't belong to your GitHub account." This is the ownership check; without it anyone could claim someone else's installation by editing the URL.
   - Targets = `[that one]` when present, else all of them.
   - If there are no targets in signin mode, 302 to `InstallURL(new state)`.
6. **Identity (cloud)**, when `s.deploymentConfig.IsCloud`:
   - `network/hexmos.ExternalProviderSignup(ctx, "github_app", userToken, "livereview")` (T3.7).
   - Require `result.jwt != ""` and `result.data.email`. Lowercased, it must equal the verified email from step 4; otherwise reject.
   - `auth.EnsureCloudUserCore(ctx, input{JWT, Email, FirstName, LastName, Source: "gh-login:"+src})` (T3.7) → `userID, orgID, createdOrg`.
   - If `createdOrg`, set `orgs.signup_path = 'gh_app'` (storage function in `storage/users`).
   - **Self-hosted:** not in launch scope. Return the error page "GitHub sign-in is only available on LiveReview Cloud for now." (§8 describes the later design.)
7. For each target:
   - `CreateInstallationToken`, then `UpsertAppInstallationConnector{OrgID: orgID, …}`.
   - `ErrInstallationBoundToOtherOrg` → if this was the explicit `installation_id`, show the error page "This GitHub installation is already connected to another LiveReview account." In signin mode, skip that installation and log it.
   - For a newly created connector, kick off repo sync **async**: `go s.autoWebhookInstaller.syncRepositoriesAndQueueBackfill(context.Background(), int(connectorID), details)`. See T3.8 for the App branch inside it. The `Server` field is `autoWebhookInstaller` (`server.go:142`).
8. Redirect `302` to `"/?data=" + url.QueryEscape(slimJSON)`, where
   `slimJSON = {"result":{"jwt":J,"data":{"email":E,"first_name":F,"last_name":L,"username":U,"profilePicUrl":P}}}`.
   These are exactly the fields `Cloud.tsx` reads (lines 70-85). Don't forward the full Parse user object.

*`GitHubAppWebhook`*:
1. Read the body (`io.ReadAll`).
2. Verify `X-Hub-Signature-256` = `"sha256=" + hex(HMAC-SHA256(cfg.WebhookSecret, body))` with `hmac.Equal`. Bad or missing → 401.
3. Parse `{"action":…,"installation":{"id":…}}` and the `X-GitHub-Event` header.
4. `installation` event:
   - `deleted` → `SetAppConnectorFlags(removed=true)`.
   - `suspend` → `suspended=true`; `unsuspend` → `suspended=false`.
   - `created` → nothing (the callback creates connectors).
   - Return 200.
5. `installation_repositories` event:
   - `added` → upsert those repos via `pullrequests.Store.UpsertRepository` and queue `QueueRepoPRSyncJob(…, true)`, the same as in `syncRepositoriesAndQueueBackfill`.
   - `removed` → delete those `repositories` rows for this connector (new storage function `DeleteRepositoryByProviderID(connectorID, providerRepoID)` in `storage/providers/pullrequests`).
   - Return 200.
6. Anything else:
   - `GetAppConnectorByInstallation`. If there's no row, or it's removed → **200** `{"status":"ignored"}`, so GitHub doesn't retry.
   - Otherwise set the same context that `BuildOrgContextFromConnector` sets (`internal/api/auth/middleware.go:346-391`). Extract that body into an exported `func (am *AuthMiddleware) SetConnectorOrgContext(c echo.Context, connectorID int64) error` and have the middleware call it too, so the code isn't duplicated.
   - Then `c.Request().Body = io.NopCloser(bytes.NewReader(body))` (**required**: the orchestrator reads the body again) and `return s.webhookOrchestratorV2.ProcessWebhookEvent(c)`.

*`getUIConfig`* (`server.go:3047`): add `"githubAppEnabled": s.githubApp != nil` and `"githubAppBaseUrl": <cfg.BaseURL or "">`.

*Demo endpoint*: register `GET /api/v1/github-app/demo` in a group with the full chain `RequireAuth → BuildOrgContextFromHeader → ValidateOrgAccess → BuildPermissionContext`. Copy how `connectorGroup` is built at `server.go:~1500-1508`.
- It reads the org from context, `ListAppConnectorsForOrg`, then **live** `ListInstallationRepositories` (first page, up to 100) and `ListOpenPulls` for up to 5 most recently updated repos × 5 PRs, using the resolver token.
- Live calls are used because the background sync may still be running right after install.
- Response:
```json
{
  "connected": true,
  "installations": [{"connector_id": 12, "account_login": "acme", "account_type": "Organization",
                     "repository_selection": "selected", "settings_url": "https://github.com/organizations/acme/settings/installations/51234567"}],
  "repositories": [{"full_name": "acme/api", "web_url": "https://github.com/acme/api", "private": true}],
  "open_pull_requests": [{"repo_full_name": "acme/api", "number": 42, "title": "Fix login", "web_url": "https://github.com/acme/api/pull/42",
                          "author": "octocat", "updated_at": "2026-10-19T10:00:00Z"}]
}
```
- `connected:false` (other arrays empty) when the org has no App connector. The UI uses that to show "Connect GitHub".

**Tests** `internal/api/github_app_auth_test.go`. Stub GitHub with `httptest` by pointing `cfg.BaseURL` at it, and stub Parse the same way via `FW_PARSE_BASE_URL`:
- the state cookie is missing or doesn't match → error page, no DB writes;
- expired state → rejected;
- `installation_id` not in `/user/installations` → rejected;
- no verified email → rejected;
- installation bound to org A, callback for a user of org B → rejected, row still belongs to A;
- happy path → connector row has `token_type='GITHUB_APP'`, `pat_token` set, the redirect starts with `/?data=`, and the data JSON has `result.jwt`;
- webhook with a bad signature → 401; unknown installation → 200 ignored; `installation.deleted` → `metadata.removed=true` and `pat_token=''`.

**T3.7 fw-parse client + shared EnsureCloudUser core.**
- `network/hexmos/parse_client.go`:
  - Move `getParseAppID` from `network/email/invitation.go:28` here as `ParseAppID()`, and make `invitation.go` call it.
  - Add:
```go
type ParseSignupResult struct {
	JWT  string
	Data struct { Email, FirstName, LastName, Username, ProfilePicURL string } // json: email, first_name, last_name, username, profilePicUrl
	Raw  json.RawMessage
}
func ExternalProviderSignup(ctx context.Context, provider, accessToken, app string) (*ParseSignupResult, error)
	// POST {FW_PARSE_BASE_URL}/parse/functions/externalProviderSignup
	// headers as invitation.go:91-95 (Content-Type, X-Parse-Application-Id, X-Internal-Admin-Secret if set)
	// body {"externalServiceProvider":provider,"accessToken":accessToken,"client_details":{"application":app,"deviceUniqueId":"default"}}
	// TRAP: Parse answers HTTP 200 {"result":{"success":false,"message":…}} on failure. Error unless result.jwt is non-empty.
```
  - Add it to `network/network_status.md`.
- `internal/api/auth/ensure_cloud_user.go`: split `EnsureCloudUser` (`internal/api/auth/handlers.go:628-866`):
```go
type EnsureCloudUserInput struct { JWT, Email, FirstName, LastName, Source string }
type EnsureCloudUserResult struct { UserID, OrgID int64; CreatedUser, CreatedOrg, AssignedSuperAdmin bool }
type EnsureCloudUserError struct { Status int; Msg string } // carries today's exact HTTP status + message
func (h *AuthHandlers) EnsureCloudUserCore(ctx context.Context, in EnsureCloudUserInput) (*EnsureCloudUserResult, error)
```
  - The core does what lines 637-812 do today:
    - JWT validation with `CLOUD_JWT_SECRET`;
    - the email-match check;
    - the transaction (find/create user, find/create personal org + `prompt_application_context`, default org, onboarding API key, owner role);
    - the Discord notify on `CreatedUser`.
  - The HTTP handler parses the header and body, calls the core, then does what it does today after commit (`CreateTokenPair`, `getUserOrganizations`, the same JSON). **The response shape must stay byte-for-byte compatible.** `Cloud.tsx` depends on it.
  - Move the SQL into `storage/users/cloud_user_store.go` with the same queries (`*sql.Tx` methods). Keep the queries verbatim; this is a move, not a rewrite. Update `storage/storage_status.md`.
  - Test: the existing behavior via the handler (same JSON keys), and calling the core twice is idempotent (second call: all `Created*` false).

**T3.8 Keep existing screens and jobs safe with App connectors.**
- `internal/api/auto_webhook_installer.go`:
  - `getConnectorDetails` (line 252): select `COALESCE(token_type,'')` into a new `TokenType` field.
  - `shouldAutoInstall`: return `false` for `GITHUB_APP`. App webhooks are global; never create per-repo hooks.
  - `syncRepositoriesAndQueueBackfill` (line 114): in the github case, when `connector.TokenType == "GITHUB_APP"`, call a new `github.ListInstallationRepositoriesPage(ctx, baseURL, token, cursor)` in `internal/providers/github/repo_list.go`. It wraps `network…ListInstallationRepositories` and returns the same `*providers.RepositoryPage`. Get `token` from `githubapp.ConnectorToken`.
- `EnableManualTriggerForAllProjects` / `DisableManualTriggerForAllProjects` (`server.go:2837+`): if the connector is `GITHUB_APP`, return 400 `{"error":"This connection is managed by the LiveReview GitHub App. Manage repositories on GitHub."}`.
- `GetRepositoryAccess` (`internal/api/repository_access.go:329`): for `GITHUB_APP`, build the response from the `repositories` table rows for that connector instead of calling `/user/repos`.
- Connectors list API (`internal/api/connectors.go`): include `token_type` in the response if it isn't already there.
- `ui/src/pages/GitProviders/*`: for `token_type === 'GITHUB_APP'`, render the card read-only:
  - "GitHub App · <login>";
  - a "Manage on GitHub" link;
  - hide the PAT edit and manual-trigger buttons.
- **Overlap rule (no double reviews):** in `ProcessWebhookEvent`, right after the T2.3 stamping, when the connector is a PAT GitHub connector and the same org has a `GITHUB_APP` connector with a `repositories` row of the same `full_name`, return 200 `{"status":"ignored","reason":"superseded_by_github_app"}`. Add a storage query `HasAppConnectorForRepo(orgID, fullName) (bool, error)`.
- **Manual trigger picks the right connector:** in `prepareAuthentication`, before `findIntegrationToken`, for GitHub URLs parse `owner/repo` and call a new storage query:
```sql
SELECT it.id FROM integration_tokens it
JOIN repositories r ON r.connector_id = it.id
WHERE it.org_id = $1 AND it.provider LIKE 'github%' AND r.full_name = $2
ORDER BY (it.token_type = 'GITHUB_APP') DESC, it.created_at DESC LIMIT 1
```
  If a row is found, use `s.loadIntegrationTokenByID(id)` (reviews_api.go:131); otherwise keep the current `findIntegrationToken` (unchanged behavior for orgs without a repositories row).

**T3.9 Error page.** A tiny server-rendered HTML page (inline template in `github_app_auth.go`):
- the LiveReview name, the message, a "Try again" link to `/#/gh-login`, and "Other ways to sign in" linking to `/`;
- no external assets;
- HTTP 400 for user errors, 502 when GitHub or Parse is unreachable.

**Done when:**
- all the T3 tests pass;
- `go build ./...` passes;
- `make check-status-doc` passes;
- with `GITHUB_APP_ID` unset, none of the new routes are registered (test: `GET /api/v1/auth/github-app/start` → 404), and `ui-config.githubAppEnabled=false`.

---

### Phase 4: LiveReview UI (PR D)

**T4.1 Entry from a real path.** `ui/src/index.tsx`, at the top before React renders:
```ts
// /gh-login is a shareable path; the app is a HashRouter, so move it into the hash.
if (window.location.pathname === '/gh-login' || window.location.pathname === '/gh-login/') {
    window.location.replace('/#/gh-login' + window.location.search);
}
```
The Go UI server (`cmd/ui.go:374`) and the webpack dev server (`historyApiFallback: true`) both already serve `index.html` for `/gh-login`.

**T4.2 `ui/src/App.tsx`.**
- In the mount-capture effect (~line 126), add `!hashPath.startsWith('#/gh-login')` to `isProtectedRoute`, so `#/gh-login` is never stored as the post-login redirect.
- In the unauthenticated branch (~line 288), before the `isCloudMode() && /admin` check:
```tsx
} else if (!isAuthenticated) {
    if (location.pathname.startsWith('/gh-login')) {
        body = <GhLogin />;
    } else if (isCloudMode() && location.pathname === '/admin') { …
```
- Authenticated routes:
  - `<Route path="/gh-login" element={<Navigate to={'/get-started/github' + location.search} replace />} />`;
  - `<Route path="/get-started/github" element={<GetStartedGitHub />} />`.
- Lazy-load both pages the way neighbouring pages are loaded.

**T4.3 `ui/src/pages/Auth/GhLogin.tsx`** (new).
- On mount: `fetch('/api/v1/ui-config')`. If `!githubAppEnabled`, `navigate('/', {replace:true})`, which renders the normal login.
- Read `src` from `useLocation().search` (allow only `[a-z0-9_-]{1,32}`, else `direct`).
- Layout (follow `.agents/design.md`): headline "Review your first PR in 2 minutes", a "FREE" badge, and one big primary button **Continue with GitHub** (GitHub icon). Below it, smaller: "Already installed the app? **Sign in**", then "Other ways to sign in" (link to `/#/`).
- Click handler:
```ts
sessionStorage.setItem('redirectAfterLogin', '#/get-started/github');
window.location.href = `/api/v1/auth/github-app/start?mode=${mode}&src=${encodeURIComponent(src || 'direct')}`;
```
- `Cloud.tsx` already turns `redirectAfterLogin` `#/x?y` into `navigate('/x?y')` (Cloud.tsx ~line 170). No change is needed there.

**T4.4 `ui/src/pages/GetStarted/GetStartedGitHub.tsx`** (new) + `ui/src/api/githubApp.ts` (`getGitHubAppDemo()` → `GET /api/v1/github-app/demo` via `apiClient`).
- `connected:false` → a card "Connect your GitHub repos" with a button. The user is already logged in, and the callback ends in `/?data=`, which only `Cloud.tsx` (logged out) processes, so the button must:
  1. dispatch the existing `logout()`;
  2. set `redirectAfterLogin`;
  3. go to `/api/v1/auth/github-app/start?mode=install&src=get-started`.
  
  Note under the button: "You'll be signed in with your GitHub account's email."
- `connected:true` → a 3-step stepper (use the existing stepper pattern from `.agents/design.md`):
  1. **Repos connected ✓**: list the `repositories` names (up to 10, then "+N more"). "Add more repos" opens `installations[0].settings_url` in a new tab.
  2. **AI**: **blocked on open decision 1 (§10).** Until it's decided: if the org has an AI connector (use the same API `OnboardingSteps.tsx` uses), show ✓. Otherwise show "Configure AI" linking to `#/ai`.
  3. **Review a PR**:
     - A paste box, then the `open_pull_requests` list, each with a **Review** button.
     - Both call the existing `triggerReview({ url })` from `ui/src/api/reviews.ts:32`, then navigate exactly the way `ui/src/pages/Reviews/NewReview.tsx:276` does after a successful trigger (copy its navigation and error toast handling).
     - Show the 403 LOC-quota response the same way NewReview does.
  - Footer link: "Next: catch issues before you commit — install the CLI" → the existing CLI onboarding page/link used on the dashboard.
- Empty `open_pull_requests` → "No open PRs yet. Open one on GitHub, or paste a PR URL above."

**T4.5 Dashboard banner.** On the dashboard, if `getGitHubAppDemo().connected` and the org has no reviews yet (reuse whatever the dashboard already loads for "first review" state), show a slim banner: "Finish setup: review your first PR →" linking to `#/get-started/github`. Fetch the demo endpoint only in cloud mode.

**T4.6 Mega menu.** `ui/src/components/Navbar/megaMenuData.ts`:
- Add `hasGitHubApp?: boolean` to `MegaMenuContext`. Populate it where the context is built (grep `MegaMenuContext` / `isDevMode:`).
- Add a link node `{ kind: 'link', name: 'Get Started (GitHub)', path: '/get-started/github', isVisible: (ctx) => !!ctx.hasGitHubApp }` under the most fitting existing section (next to the dashboard/onboarding entries).

**T4.7 Routes guide docs** (repo rule). Create `internal/docindex/docs/routes_guide/auth/gh-login.md` and `internal/docindex/docs/routes_guide/get-started-github.md`, following the shape in `internal/docindex/docs/routes_guide/README.md`: Route(s), Purpose, Who can access it, Key actions, Related pages.

**Done when:**
- `cd ui && npx tsc --noEmit` is clean.
- A manual run on manual-talent3 completes the E2E in §11, step 5.
- **Don't run a production UI build without asking.**

---

### Phase 5: hexmoshomepage (PR E)

**T5.1** `urls.js`: add `LIVEREVIEW_APP: "https://livereview.hexmos.com"` and export a helper (e.g. in `utils/` next to similar helpers):
```js
export const lrGhLoginUrl = (src) => `${url.LIVEREVIEW_APP}/gh-login?src=${encodeURIComponent(src)}`
```

**T5.2 Point the "Start Free" CTAs at it** (verify each line; keep pricing and self-hosted buttons as they are):
- `pages/livereview/index.tsx`: `handleBuyNow` (line ~528) → `lrGhLoginUrl("home")`. This covers the Start Free buttons at ~562, ~987 and ~2467 that call it. Check each.
- `components/livereview/StartFreeModal.tsx:146` `handleStartFreeClick` → `lrGhLoginUrl("start-free-modal")` (the "Try Cloud" path only).
- `components/livereview/PricingSection.tsx:~181`: **free plan button only** → `lrGhLoginUrl("pricing-free")`.
- `pages/livereview/blog/index.tsx:~153`, `pages/livereview/blog/[slug].tsx:~174`, `pages/livereview/why-livereview.tsx:~222,~384` → `lrGhLoginUrl("<page>")`.

**T5.3 dropped.** A hero "paste a GitHub PR URL" box was considered and rejected: visitors won't copy a PR link from GitHub and paste it. The flow is just Continue with GitHub, pick repos, then LiveReview suggests a PR.

---

## 7. Known traps (read before coding)

1. `GET /user` with an installation token → 403. Use `<slug>[bot]` (T3.5).
2. GitHub's OAuth code exchange returns **HTTP 200 with `{"error":…}`** on failure (T3.2).
3. fw-parse returns **HTTP 200 with `{"result":{"success":false}}`** on failure (T3.7).
4. `connector_id` in event metadata becomes **float64** after the job-queue JSON round trip (T2.4).
5. The orchestrator stamps `connector_id` **after** `getBotUserInfo` today; it must move earlier (T2.3).
6. The App webhook handler must **restore the request body** before calling `ProcessWebhookEvent` (T3.6).
7. The state cookie must be **SameSite=Lax**; Strict drops it on the GitHub → LiveReview redirect.
8. `setup_action=request` has no `installation_id` (an org member asked an owner) (T3.6 step 1).
9. A user who is logged in can't finish via `/?data=`. `Cloud.tsx` only runs when logged out (T4.4).
10. `#/gh-login` must be excluded from the mount-time `redirectAfterLogin` capture (T4.2).
11. The private key in env often has a literal `\n` in it (T3.1).
12. Installation tokens last 1 hour. A review job that sits in the queue for over ~55 min would carry a stale token in its provider config. Accepted for launch; the refresher keeps the DB fresh for anything that re-reads it.
13. Existing GitHub code hard-codes `https://api.github.com` in many places (e.g. `github_token_helpers.go:51`, `unified_processor_v2.go:744`, `lrc_fetch.go:34`). That's fine for cloud and github.com. GHES + App needs those switched to `APIBaseURL(token.ProviderURL)`. **Out of launch scope; don't refactor them now.** New code must not add more.

---

## 8. Self-hosted (design only; not in launch scope)

Self-hosted has no fw-parse. Its sessions are the local `tokenService.CreateTokenPair`.

When this is picked up later, the callback's identity step for `!IsCloud` will be:
1. Find the local user by verified email. By default only existing users can sign in: admins create users today. A `GITHUB_APP_ALLOW_SIGNUP=true` flag can come later.
2. If no admin exists yet, redirect to the setup screen.
3. Upsert the connector for the user's default org.
4. Create a one-time code in a new table `gh_login_codes(code_hash text primary key, user_id bigint, expires_at timestamptz, used_at timestamptz)` with a 60-second TTL and single use.
5. 302 to `/#/gh-login/complete?code=…`.
6. `POST /api/v1/auth/github-app/exchange` returns the same `LoginResponse` as `/auth/login`. The UI calls `handleLoginSuccess`.

The config (T3.1, base URL, GHES install path) is already self-hosted-ready.

---

## 9. PR order

| PR | Repo | Tasks | Can ship alone? |
|---|---|---|---|
| A | fw-parse | T1.1–T1.5 | Yes (adds a provider, no behavior change for others) |
| B | LiveReview | T2.1–T2.5 | **Yes, ship first.** Fixes the existing cross-tenant bug |
| C | LiveReview | T3.1–T3.9 | Yes (feature off unless `GITHUB_APP_ID` is set) |
| D | LiveReview UI | T4.1–T4.7 | After C |
| E | hexmoshomepage | T5.1–T5.2 | Only after C+D are live in prod with the App URLs switched (T0.4) |

---

## 10. Open decisions (Shrijith), with the implementer's default until answered

1. **AI for the demo.** The "FREE" promise needs one of:
   - (a) auto-provision the managed "LiveReview AI Model" for `gh_app` orgs, using the same insert as `ConfirmPurchase` (`internal/license/payment/subscription_service.go:1603`), with a small LOC cap;
   - (b) an inline "paste a Gemini key" field.
   
   Today `OnboardingSteps.tsx:~93` sends free-plan users to *upgrade* when they click Configure AI. *Default until decided:* T4.4 step 2 links to `#/ai`. **This is a launch blocker, not an implementation blocker.**
2. **Demo limits:** 7 days and/or 1 repo, or just the LOC quota? *Default:* the existing LOC quota only.
3. **Auto-review new PRs:** *Default:* off. Reviews come from the "Review" button and the existing comment and mention triggers. No auto-review on PR open.

---

## 11. Verification

1. Set up per T0.2 and run `make develop`, `cd ui && make run`, `make niceurl3`.
2. fw-parse: T1.5 passes. A manual `curl` returns a JWT.
3. Go:
   - `go test -count=1 ./internal/githubapp/ ./network/providers/github/ -run 'App|Config'`
   - `go test -count=1 ./internal/provider_input/github/`
   - `go test -count=1 ./internal/api/ -run 'GitHubApp|EnsureCloudUser|WebhookOrchestrator'`
4. Tenant isolation:
   - Two orgs (App + PAT, and PAT + PAT). A webhook for org A only ever loads org A's connector.
   - `grep -rn "GetLatestGitHubToken" --include=*.go .` is empty.
5. Manual E2E (private window, on `https://manual-talent3.apps.hexmos.com`):
   1. `/gh-login?src=test`, then Continue with GitHub.
   2. Install on **one** repo. You should land on `#/get-started/github` with the repo listed and the PR prefilled.
   3. Click **Review** → the review page opens → the bot comment appears on the PR as `<slug>[bot]`.
   4. The Parse `_User` row exists with the GitHub email, and there's a `JWT` row for application `livereview`.
   5. Log out, then sign in through **"Other ways to sign in" → Google** with the same email. You must land in the **same** LiveReview user and org, and the App connector is still there.
   6. "Already installed? Sign in" works for a returning user.
   7. Comment `@<slug> …` on the PR → the bot replies, and it's signed by the App, not a PAT.
   8. In GitHub settings, remove the repo from the installation, then uninstall. The `repositories` row goes away and the connector shows as removed.
   9. The Git Providers page shows the App connector as read-only and nothing on that page errors.
6. Config off: unset `GITHUB_APP_ID` → `/gh-login` shows the normal login, the new routes 404, and `grep -rn "github.com/apps\|GITHUB_APP" ui/src` shows nothing hard-coded.
7. `make check-status-doc`, and `cd ui && npx tsc --noEmit`. No production UI build without asking.

---

## 12. What does NOT change

- **onelogin-node:** no change.
- **`Cloud.tsx` / `SelfHosted.tsx` login behavior:** no change. `Cloud.tsx` simply also receives `/?data=` from the new callback.
- **The Git Providers PAT flow** for every provider: unchanged, except the cross-tenant fix (PR B) and the read-only App card (T3.8).
- **Self-hosted without `GITHUB_APP_*`:** the feature is invisible.
