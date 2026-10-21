 ▐▛███▛█   Claude Code v2.1.302
▝▜██████▀  Opus 5.5 · Claude Pro
 ▝▝   ▝▝   ~/hex/lr/LiveReview


❯ /add-dir /home/lovestaco/hex/hexmoshomepage                                   
  ⎿  Added /home/lovestaco/hex/hexmoshomepage as a working directory for this
     session · /permissions to manage

❯ /add-dir /home/lovestaco/hex/liveapi                                          
  ⎿  Added /home/lovestaco/hex/liveapi as a working directory for this session ·
     /permissions to manage

❯ in hexmos homepage its my landing page, check for liveapi landing page there  
  was some input box of mrlink, the user could uh, share the repo link, I       
  think. And uh, on click of submit, it would go to live API directory and in   
  its front end, we have a common login uh, JS thing. Where we have GitHub, uh, 
  just sign in and uh, Google sign in and LinkedIn sign in or email sign in.    
  After that, what would happen when uh, MR link, uh, repo link was pasted in   
  live API? Like, was it uh, asking for GitHub extra uh, permissions or         
  something for that repository? Uh, can you check uh, or was it asking for API 
  key uh, or was it asking personal access token? I don't remember it properly  
  what was the flow, but there was some sign in where repo something was        
  happening there. So can you check what was that thing, uh, what was that      
  flow, how would it work? Because for live review landing page also we are     
  thinking to have some uh, MR link uh, option and uh, on click of uh, submit   
  it would go to live review sign in the Hexmo sign in once they sign in        
  thinking of asking for that particular repo access or something using GitHub  
  or something and uh, basically reviewing the MR so that is the thinking which 
  we are doing for private repositories and if it is for public repository      
  then uh, I don't I don't know if uh, some access needs to be given just check 
  that as well basically we need to improve this landing page so that we can    
  put it in product hunt launch and uh, people can immediately try it out and   
  what other mandatory required things are required to try out in the cloud.    
  For a new person, I think AI provider is one mandatory thing. Uh, what other  
  thing is there? Can you just check?                                           

  Searched for 1 pattern, ran 3 shell commands (ctrl+o to expand)

● The key part is in ViewProjects.tsx.

  Searched for 1 pattern, ran 1 shell command (ctrl+o to expand)

● Now the backend side of the opensource service, and how LiveAPI handles
  private repos normally.

  Searched for 1 pattern, listed 1 directory, ran 3 shell commands (ctrl+o to expand)

● Now LiveReview — what a new cloud user currently has to set up before a review
  can run.

  Searched for 2 patterns, ran 5 shell commands (ctrl+o to expand)

● Quick status: LiveAPI's flow is mapped out. Now I'm checking whether
  LiveReview cloud gives new users a built-in AI model, or whether they have to
  bring their own key.

  Searched for 1 pattern, ran 2 shell commands (ctrl+o to expand)

● LiveAPI never asked for extra GitHub permissions, an API key or a token in the
  landing-page flow. It only worked with public repos.

  How LiveAPI's repo-link flow works

  1. Landing page (hexmoshomepage/pages/liveapi/index.tsx:219): the repo box
     opens https://liveapi.hexmos.com?demorepo=<url>.
  2. Sign-in: if there's no JWT, DefaultLayout.tsx:88 redirects to
     /signin?demorepo=..., which sends the user to hexmos.com/signin?app=liveapi
     (the shared GitHub / Google / LinkedIn / email login). demorepo is carried
     through sign-in and the "enter details" page.
  3. After login (client/src/pages/Home/modules/ViewProjects.tsx:123), the front
     end makes two calls:
     - POST /integration with service: 'opensource' and an empty accessToken.
       The backend skips the GitHub user lookup for that service
       (liveapi-backend/integrations/service.go:123).
     - POST /generate-docs with the repo URL. The runner then clones it with no
       credentials.

  Private repos used a separate flow inside the app. When you picked a repo
  there, it sent you to install the liveapi-github GitHub App
  (client/src/services/api/github.ts:46,
  github.com/apps/liveapi-github/installations/new). GitHub's install screen
  lets the user choose "only select repositories", so that's the "repo access"
  prompt you remember. There's also an older OAuth path asking for repo, 
  read:org.

  ⚠️ While checking this I found a GitHub App CLIENT_SECRET hard-coded in
  liveapi/client/src/constants/github.ts, and that file ships to the browser.
  It's public in effect and should be rotated.

  What a new LiveReview cloud user needs today

  Reviewing by URL goes through POST /api/v1/connectors/trigger-review
  (internal/api/review_service.go:80). For it to work, all of these must be
  true:

  #: 1
  Requirement: Hexmos sign-in and an org
  Current state: Already in place
  ────────────────────────────────────────
  #: 2
  Requirement: A Git provider connector for that host
  Current state: Mandatory. Without one, findIntegrationToken fails with "URL is
   
  not from a connected Git provider" (reviews_api.go:106). For GitHub, the only
   option is a classic PAT with repo, read:org. The UI itself says "OAuth
  support will be added in a future update"
  (ui/src/components/Connector/GitHubConnector.tsx).
  ────────────────────────────────────────
  #: 3
  Requirement: A Leader AI connector
  Current state: Mandatory. The built-in "LiveReview AI Model" is only created
  in
  ConfirmPurchase (internal/license/payment/subscription_service.go:1603), and
  that's the only place I found that creates it. So free/trial users must bring
   their own API key (Gemini, OpenAI, etc.).
  ────────────────────────────────────────
  #: 4
  Requirement: LOC quota left, and the trial not in read-only mode
  Current state: Checked before each review. A blocked org gets quota_exceeded
  or
  trial_readonly.
  ────────────────────────────────────────
  #: 5
  Requirement: Webhooks
  Current state: Only needed for automatic reviews on new PRs, not for a one-off

  review by URL.

  So a Product Hunt visitor currently has to create a PAT and paste an AI key
  before seeing any result. Most will drop off there.

  Public vs. private repos

  - Public repo: reading the PR diff doesn't need the visitor's permission.
    LiveReview can fetch it with its own server-side GitHub App or bot token.
    GitHub's unauthenticated limit of 60 requests/hour per IP is too low for a
    launch. The catch: posting comments on a stranger's PR would need write
    access and would amount to spam. So show the review on a LiveReview results 
    page, not on the PR.
  - Private repo: copy the LiveAPI pattern with a LiveReview GitHub App set to
    "only select repositories". It needs Pull requests read & write, Contents
    read and Metadata read. That gives the per-repo access prompt you described
    and replaces the PAT. This is new work, since LiveReview has no GitHub App
    or OAuth connector today.

  Minimum for a "paste link and try it" launch

  1. A ?mr=<url> parameter on the landing page that survives Hexmos sign-in
     (same mechanism as demorepo).
  2. A managed AI connector created for trial and free orgs at signup, perhaps
     with a small LOC cap. This removes the AI key step.
  3. Public PRs: a server-side token fetches the diff and the review appears in
     the LiveReview UI. The visitor connects nothing.
  4. Private PRs: the GitHub App install flow (select repo, then review),
     replacing the PAT step.
  5. A friendly page for quota or trial errors instead of the raw error codes.

  Items 2 and 3 alone would let someone go from Product Hunt to their first
  review with just a sign-in. Should I sketch the implementation for those two?

✻ Brewed for 2m 6s · done 2:43 pm

※ recap: You want a "paste a PR link and try it" flow on the LiveReview landing page for Product Hunt, and I've mapped how LiveAPI did it and what LiveReview needs. Next: say whether I should sketch the free built-in AI model and the public-PR review without a token. (disable recaps in /config)

❯ /model                                                                        
  ⎿  Set model to Opus 5.5 and saved as your default for new sessions

❯ /add-dir /home/lovestaco/hex/onelogin-node                                    
  ⎿  Added /home/lovestaco/hex/onelogin-node as a working directory for this
     session · /permissions to manage

❯ dont worry about the self hosted thing just /plan the github thing, in        
  onelogin-node the singin stuff is there                                       
  shrijith — 2:46 PM                                                            
  ---                                                                           
  also i have a feeling why trial pitches going cold                            
  i think it's because plan is too big for step 1                               
  we should offer something for just 7 days                                     
  maybe 1 repo                                                                  
  and somehow make it super quick                                               
  we have to earn larger commitment                                             
  so like trial 1 -> win                                                        
  trial 2 -> win                                                                
  buy                                                                           
  we have to build confidence                                                   
  even the guy who said he'll do backed out after i set out the requirements    
  they won't give cloud also                                                    
  headache                                                                      
  have to ease this in somehow                                                  
  i'm thinking how we can make the demo 1 liner                                 
  use our smtp                                                                  
  use cloudflare tunnel to spin up temp url                                     
  have agent scripts to collect api key?                                        
  something something                                                           
  i think we need agent assisted onboarding                                     
  and it should just be something small they can try and see how it works       
  rather than a full prod deployment                                            
  ---                                                                           
  it's understandable -- they're like ok -- we put 1month effort -- it doesn't  
  work -- what's the ROI?                                                       
  they're thinking of failure case                                              
  while we're thinking of success case                                          
  ---                                                                           
  onboarding signup should look like                                            
  it takes 5 minutes                                                            
  we have agent assited onboarding for all these platforms                      
  smtp, etc we should hardcode with our own                                     
  basically minimize work to bare bare minimum                                  
  even url/domain use something from cloudflare if possible                     
  lovestaco [A\],  — 2:51 PM                                                    
  For self hosted one seat is free right @Rijul  ? guess we can add it as free  
                                                                                
  For the CTA for launch:                                                       
                                                                                
  How LiveAPI's repo-link flow works                                            
  Landing page: the repo box opens                                              
  https://liveapi.hexmos.com/?demorepo=<repo_url>.                              
  Sign-in: if there's no JWT, redirects to /signin?demorepo=..., which sends    
  the user to hexmos.com/signin?app=liveapi. demorepo is carried through        
  sign-in and the "enter details" page.                                         
  After login, the front end makes two calls:                                   
  POST /integration with service: 'opensource' and an empty accessToken. The    
  backend skips the GitHub user lookup for that service.                        
  POST /generate-docs with the repo URL. The runner then clones it with no      
  credentials.                                                                  
                                                                                
  Private repos used a separate flow inside the app. When you picked a repo     
  there, it sent you to install the liveapi-github GitHub App GitHub's install  
  screen lets the user choose "only select repositories", so that's the "repo   
  access" prompt you remember.                                                  
                                                                                
  ---                                                                           
                                                                                
  Minimum for a "paste mr link and try it" LiveReview Cloud launch 3 step       
                                                                                
  A ?mr=<url> parameter on the landing page that survives Hexmos sign-in.       
  A managed Gemini AI connector quick add.                                      
  Public PRs: a server-side token fetches the diff and the review appears in    
  the LiveReview UI. The visitor connects nothing.                              
  Private PRs: the GitHub App install flow (select repo, then review),          
  replacing the PAT step.                                                       
                                                                                
  so no cloud CTA?                                                              
  I was thinking we'll hook the cloud onboarding                                
  so that they dont need to download                                            
  or we can have both ways ig                                                   
  so that they can be comfertable to try out either                             
  shrijith — 2:54 PM                                                            
  cloud we can do                                                               
  but in cloud github -- we can easier login i'm thinking                       
  like in hexmos login include github login                                     
  and once they signup there only you select repos or something during          
  onboarding                                                                    
  easiest                                                                       
  we may need a github app or something                                         
  plus adding github login to onelogin (anyway required)                        
  we can recommend github login                                                 
  and get them to give access to some repos (they have the dialog for it)       
  then they can just trigger review right away                                  
  lovestaco [A\],  — 2:55 PM                                                    
  already there                                                                 
  Image                                                                         
  after this step select repo                                                   
  add gemini key                                                                
  shrijith — 2:55 PM                                                            
  yes                                                                           
  lovestaco [A\],  — 2:55 PM                                                    
  start review                                                                  
  shrijith — 2:56 PM                                                            
  we can have order control                                                     
  like make it recommended and big                                              
  and maybe even hide the other ones (collapse)                                 
  make that page a bit customizable                                             
  then we will get github logins                                                
  once you have github login -- somehow use it to install the github app in     
  their org                                                                     
  there they can control access                                                 
  and trigger                                                                   
  basically have a "github installation flow"                                   
  then anyone can quicly try                                                    
  we need to study github integration more clearly                              
  rn -- the "sign in with github" will only give email                          
  it won't give repo access                                                     
  that's what we should figure out                                              
  get an easy "github path"                                                     
  that'll be easiest step 1                                                     
  also -- add "FREE"                                                            
  ⎿  Loaded CLAUDE.md

● Entered plan mode
  Claude is now exploring and designing an implementation approach.

  Ran 3 shell commands (ctrl+o to expand)

● Onelogin's GitHub button is a plain OAuth App scoped to read:user,user:email,
  with code exchange handled inside Parse's externalProviderSignup function —
  LiveReview never sees a GitHub token. Next I'll look at the LiveReview side:
  login handoff, GitHub connector internals, and onboarding.

  Searched for 2 patterns, listed 2 directories, ran 4 shell commands (ctrl+o to expand)

● Status: I've traced the onelogin GitHub login and the LiveReview GitHub token
  paths. Last few checks: JWT library availability and the existing webhook
  installer, then I'll write the plan.

  Searched for 1 pattern (ctrl+o to expand)

● Updated plan
  ⎿  /plan to preview
  ⎿  User rejected Claude's plan:
     ╭─────────────────────────────────────────────────────────────────────────╮
     │ LiveReview Cloud: "GitHub path" for a 5-minute first review             │
     │                                                                         │
     │ Context                                                                 │
     │                                                                         │
     │ Trials go cold because step 1 asks too much: create a classic PAT       │
     │ (repo, read:org), paste an AI key, install the CLI. For the Product     │
     │ Hunt launch, the goal is: sign in with GitHub → pick repos in GitHub's  │
     │ own dialog → review a PR right away. No PAT, no webhook setup.          │
     │                                                                         │
     │ What exists today:                                                      │
     │ - onelogin-node (src/shared/src/components/Home.js:110): "Sign in with  │
     │   GitHub" is an OAuth App (Ov23li…) with scope=read:user,user:email.    │
     │   The code is exchanged inside the Parse cloud function                 │
     │   externalProviderSignup, so LiveReview never gets a GitHub token.      │
     │   Sign-in gives identity only, not repo access. That's expected and     │
     │   should stay that way.                                                 │
     │ - LiveReview GitHub connector is PAT-only                               │
     │   (ui/src/components/Connector/GitHubConnector.tsx, stored by           │
     │   internal/api/pat_token.go into integration_tokens with                │
     │   token_type='PAT').                                                    │
     │ - LiveAPI already did this with the liveapi-github GitHub App           │
     │   (liveapi/client/src/services/api/github.ts:46,                        │
     │   github.com/apps/<name>/installations/new). We copy that pattern.      │
     │                                                                         │
     │ Existing bug this work must fix:                                        │
     │ storage/providers/github/token_store.go:21 GetLatestGitHubToken()       │
     │ selects the newest GitHub token across all orgs (no org_id filter).     │
     │ Every webhook/comment path in                                           │
     │ internal/provider_input/github/github_provider.go (lines 145, 198, 258, │
     │ 272, 286, 307, 409, 440) and lrc_fetch.go:28 uses it through            │
     │ FindIntegrationTokenForGitHubRepo. In cloud, org A's PR events can be   │
     │ fetched and commented on with org B's credentials. The App flow has to  │
     │ resolve tokens per connector/org, so this gets fixed along the way.     │
     │                                                                         │
     │ Approach: a "LiveReview" GitHub App, installed during onboarding        │
     │                                                                         │
     │ Leave the Hexmos sign-in as identity-only. After login, LiveReview      │
     │ onboarding shows one big Connect GitHub (Free) button that opens the    │
     │ App install page. On that page the user picks "All" or "Only select     │
     │ repositories". GitHub redirects back, we store one connector per        │
     │ installation, and webhooks arrive at a single App-level endpoint. No    │
     │ per-repo webhook install and no PAT.                                    │
     │                                                                         │
     │ Phase 0: Create the GitHub App (one-time, in GitHub settings, done by a │
     │ human)                                                                  │
     │                                                                         │
     │ - Permissions: Pull requests R/W, Contents R, Metadata R, Issues R/W    │
     │   (PR conversation comments use the issues API).                        │
     │ - Events: pull_request, pull_request_review_comment, issue_comment,     │
     │   installation, installation_repositories.                              │
     │ - Webhook URL: https://<cloud>/api/v1/github-app/webhook, with a        │
     │   webhook secret.                                                       │
     │ - Setup URL: https://<cloud>/#/git/github-app/callback, with "Redirect  │
     │   on update" on.                                                        │
     │ - "Request user authorization (OAuth) during installation" on. GitHub   │
     │   then also returns a code, which we use to verify that the             │
     │   installation belongs to this user (see the security note below).      │
     │ - Env vars: GITHUB_APP_ID, GITHUB_APP_SLUG, GITHUB_APP_CLIENT_ID,       │
     │   GITHUB_APP_CLIENT_SECRET, GITHUB_APP_PRIVATE_KEY (PEM, or a path to   │
     │   it), GITHUB_APP_WEBHOOK_SECRET. Server-side only. Never ship a secret │
     │   in the UI bundle (LiveAPI's constants/github.ts makes this mistake).  │
     │                                                                         │
     │ Phase 1: Backend for installation tokens                                │
     │                                                                         │
     │ network/ (outbound HTTP only), new network/providers/github/app.go:     │
     │ - MintAppJWT(appID, key): RS256, 9-minute expiry, using                 │
     │   github.com/golang-jwt/jwt/v5 (already in go.mod).                     │
     │ - CreateInstallationToken(jwt, installationID) → POST                   │
     │   /app/installations/{id}/access_tokens. Returns the token plus         │
     │   expires_at, which is 1 hour out.                                      │
     │ - ExchangeUserCode(code) and ListUserInstallations(userToken) → GET     │
     │   /user/installations, used for the ownership check.                    │
     │ - ListInstallationRepos(instToken) → GET /installation/repositories.    │
     │   /user/repos doesn't work with installation tokens.                    │
     │ - Update network/network_status.md.                                     │
     │                                                                         │
     │ storage/, extend storage/providers/github/token_store.go:               │
     │ - CreateAppInstallationConnector(orgID, installationID, accountLogin):  │
     │   row in integration_tokens with provider='github',                     │
     │   token_type='GITHUB_APP', provider_app_id=<installation_id>,           │
     │   provider_url='https://github.com', access_token = cached installation │
     │   token, expires_at.                                                    │
     │ - GetGitHubTokenByConnector(orgID, connectorID) and                     │
     │   GetConnectorByInstallationID(installationID). Both replace            │
     │   GetLatestGitHubToken, which gets deleted.                             │
     │ - UpdateCachedInstallationToken(id, token, expiresAt), plus a delete or │
     │   mark-suspended function for installation.deleted / suspend.           │
     │ - A migration (db/migrations/): partial unique index on                 │
     │   (provider_app_id) where token_type='GITHUB_APP'. One installation     │
     │   belongs to exactly one org.                                           │
     │ - Update storage/storage_status.md.                                     │
     │                                                                         │
     │ One token resolver, new                                                 │
     │ internal/provider_input/github/token_resolver.go:                       │
     │ ResolveGitHubAccessToken(ctx, rec) (string, error): for PAT, return     │
     │ pat_token. For GITHUB_APP, return the cached access_token if it's valid │
     │ for more than 5 more minutes, otherwise mint a new one and cache it.    │
     │ Every site that reads token.PatToken (the github_provider.go lines      │
     │ above, lrc_fetch.go:33, internal/api/reviews_api.go                     │
     │ ensureValidToken/checkTokenNeedsRefresh at lines 187 and 262, and       │
     │ auto_webhook_installer.go:125) calls this resolver instead.             │
     │ internal/providers/github/github.go:39 keeps taking pat_token from its  │
     │ config map, which is just the resolved token.                           │
     │                                                                         │
     │ Endpoints in internal/api/github_app.go, registered in server.go:       │
     │ - GET /api/v1/github-app/install-url (auth + org middleware chain).     │
     │   Returns                                                               │
     │   https://github.com/apps/<slug>/installations/new?state=<signed        │
     │   state>. The state is an HMAC (JWT secret) over {org_id, user_id,      │
     │   nonce, exp} so the callback can't be replayed into a different org.   │
     │ - POST /api/v1/github-app/callback (auth + org chain). Body:            │
     │   {installation_id, code, state}. Verify the state matches the caller's │
     │   org, exchange code for a user token, and confirm installation_id      │
     │   appears in GET /user/installations. Without that check, anyone could  │
     │   claim someone else's installation ID. Then create the connector,      │
     │   enable manual trigger for its repos (reuse                            │
     │   EnableManualTriggerForAllProjects logic), and return the connector    │
     │   plus repo list.                                                       │
     │ - POST /api/v1/github-app/webhook (public; verifies X-Hub-Signature-256 │
     │   against the App secret). Look up the connector with                   │
     │   payload.installation.id → GetConnectorByInstallationID → org, then    │
     │   hand off to the existing WebhookOrchestratorV2Handler path with that  │
     │   connector, the same way /github-hook/:connector_id does today. Handle │
     │   installation.deleted/suspend and                                      │
     │   installation_repositories.added/removed (repo sync).                  │
     │ - GitHubV2Provider methods take the connector ID from the orchestrator  │
     │   instead of calling FindIntegrationTokenForGitHubRepo(db,              │
     │   repoFullName). That's the actual fix for the cross-tenant bug.        │
     │                                                                         │
     │ Phase 2: UI (LiveReview)                                                │
     │                                                                         │
     │ - ui/src/components/Connector/GitHubConnector.tsx: the default route    │
     │   becomes a Connect with GitHub App card that's large and marked        │
     │   "Recommended · Free". Manual PAT moves behind a "Use a Personal       │
     │   Access Token instead" disclosure, which stays for self-hosted and     │
     │   GHES.                                                                 │
     │ - New route #/git/github-app/callback (page under                       │
     │   ui/src/pages/GitProviders/). Reads installation_id, code and state    │
     │   from the URL, POSTs to the callback endpoint, then shows the repo     │
     │   list with Review a PR (paste URL → existing trigger-review) or opens  │
     │   the latest open PRs.                                                  │
     │ - ui/src/components/Dashboard/OnboardingSteps.tsx: add Step 1: Connect  │
     │   GitHub before "Configure AI", and make the CLI step optional ("or use │
     │   the web"). Note: today isFreePlan sends "Configure AI" to upgrade     │
     │   (line ~93). If we're marketing "FREE", free orgs need either the      │
     │   managed Gemini connector or permission to add a key. That decision is │
     │   tracked separately from this plan.                                    │
     │ - ?mr=<url> deep link: if a URL arrives before login, it's stored via   │
     │   the existing getRedirectAfterLogin (ui/src/pages/Auth/Cloud.tsx).     │
     │   After the GitHub App is connected, the review form is prefilled.      │
     │ - Repo rule: add the new page to megaMenuData.ts (Git Providers group), │
     │   and add or update internal/docindex/docs/routes_guide/git/*.md.       │
     │                                                                         │
     │ Phase 3: onelogin-node (small change, livereview only)                  │
     │                                                                         │
     │ - In src/shared/src/components/Home.js, when app === 'livereview',      │
     │   render GitHub first as a large button with a "Recommended" tag, and   │
     │   collapse the other providers (email, LinkedIn, Authentik) under       │
     │   "Other ways to sign in". Drive this from a small per-app config map   │
     │   so LiveAPI keeps its current layout.                                  │
     │ - Keep the OAuth scopes as they are (read:user,user:email). Repo access │
     │   comes from the App install in LiveReview, not from sign-in. Asking    │
     │   for repo at sign-in would scare people off and give us access to      │
     │   every repo the user has.                                              │
     │                                                                         │
     │ Out of scope / later                                                    │
     │                                                                         │
     │ - Public-PR reviews with no install at all (a server-side token,        │
     │   results shown only in the LiveReview UI). Once the user has           │
     │   authorized the App, the user-to-server token from Phase 1 can read    │
     │   public PRs, so this is a small follow-up.                             │
     │ - Managed Gemini connector for free orgs (separate decision, see the    │
     │   Phase 2 note).                                                        │
     │ - Self-hosted: unchanged, PAT stays the default there.                  │
     │                                                                         │
     │ Verification                                                            │
     │                                                                         │
     │ 1. Register a test GitHub App (e.g. livereview-dev) pointed at an       │
     │    ngrok/cloudflared URL. Fill in .env and run make develop.            │
     │ 2. Unit tests: go test ./network/providers/github/ -run App (JWT        │
     │    claims, token response parsing against httptest) and go test         │
     │    ./internal/api/ -run GitHubApp (bad or foreign state rejected,       │
     │    installation not in /user/installations rejected, bad webhook        │
     │    signature returns 401).                                              │
     │ 3. Cross-tenant regression test: two orgs, each with a GitHub           │
     │    connector. A webhook for org A's installation must resolve org A's   │
     │    connector. Then grep to confirm GetLatestGitHubToken has no          │
     │    remaining callers.                                                   │
     │ 4. Manual E2E: sign in through hexmos.com with GitHub → Connect GitHub  │
     │    → choose one repo → land on the callback → open a PR in that repo →  │
     │    a review comment gets posted. Also: remove the repo in GitHub        │
     │    settings and confirm installation_repositories.removed updates the   │
     │    repo list. Uninstall and confirm the connector is marked removed.    │
     │ 5. make check-status-doc, and npx tsc --noEmit in ui/. No full UI build │
     │    without asking first.                                                │
     ╰─────────────────────────────────────────────────────────────────────────╯

✻ Churned for 2m 3s · done 3:05 pm
❯ /add-dir /home/lovestaco/hex/fw-parse                                         
  ⎿  Added /home/lovestaco/hex/fw-parse as a working directory for this session
     · /permissions to manage

❯ anything to change in fw-parse?                                               

  Ran 5 shell commands (ctrl+o to expand)

● Short answer: nothing required in fw-parse.

● Updated plan
  ⎿  /plan to preview
  ⎿  User rejected Claude's plan:
     ╭─────────────────────────────────────────────────────────────────────────╮
     │ LiveReview Cloud: "GitHub path" for a 5-minute first review             │
     │                                                                         │
     │ Context                                                                 │
     │                                                                         │
     │ Trials go cold because step 1 asks too much: create a classic PAT       │
     │ (repo, read:org), paste an AI key, install the CLI. For the Product     │
     │ Hunt launch, the goal is: sign in with GitHub → pick repos in GitHub's  │
     │ own dialog → review a PR right away. No PAT, no webhook setup.          │
     │                                                                         │
     │ What exists today:                                                      │
     │ - onelogin-node (src/shared/src/components/Home.js:110): "Sign in with  │
     │   GitHub" is an OAuth App (Ov23li…) with scope=read:user,user:email.    │
     │   The code is exchanged inside the Parse cloud function                 │
     │   externalProviderSignup, so LiveReview never gets a GitHub token.      │
     │   Sign-in gives identity only, not repo access. That's expected and     │
     │   should stay that way.                                                 │
     │ - LiveReview GitHub connector is PAT-only                               │
     │   (ui/src/components/Connector/GitHubConnector.tsx, stored by           │
     │   internal/api/pat_token.go into integration_tokens with                │
     │   token_type='PAT').                                                    │
     │ - LiveAPI already did this with the liveapi-github GitHub App           │
     │   (liveapi/client/src/services/api/github.ts:46,                        │
     │   github.com/apps/<name>/installations/new). We copy that pattern.      │
     │                                                                         │
     │ Existing bug this work must fix:                                        │
     │ storage/providers/github/token_store.go:21 GetLatestGitHubToken()       │
     │ selects the newest GitHub token across all orgs (no org_id filter).     │
     │ Every webhook/comment path in                                           │
     │ internal/provider_input/github/github_provider.go (lines 145, 198, 258, │
     │ 272, 286, 307, 409, 440) and lrc_fetch.go:28 uses it through            │
     │ FindIntegrationTokenForGitHubRepo. In cloud, org A's PR events can be   │
     │ fetched and commented on with org B's credentials. The App flow has to  │
     │ resolve tokens per connector/org, so this gets fixed along the way.     │
     │                                                                         │
     │ Approach: a "LiveReview" GitHub App, installed during onboarding        │
     │                                                                         │
     │ Leave the Hexmos sign-in as identity-only. After login, LiveReview      │
     │ onboarding shows one big Connect GitHub (Free) button that opens the    │
     │ App install page. On that page the user picks "All" or "Only select     │
     │ repositories". GitHub redirects back, we store one connector per        │
     │ installation, and webhooks arrive at a single App-level endpoint. No    │
     │ per-repo webhook install and no PAT.                                    │
     │                                                                         │
     │ Phase 0: Create the GitHub App (one-time, in GitHub settings, done by a │
     │ human)                                                                  │
     │                                                                         │
     │ - Permissions: Pull requests R/W, Contents R, Metadata R, Issues R/W    │
     │   (PR conversation comments use the issues API).                        │
     │ - Events: pull_request, pull_request_review_comment, issue_comment,     │
     │   installation, installation_repositories.                              │
     │ - Webhook URL: https://<cloud>/api/v1/github-app/webhook, with a        │
     │   webhook secret.                                                       │
     │ - Setup URL: https://<cloud>/#/git/github-app/callback, with "Redirect  │
     │   on update" on.                                                        │
     │ - "Request user authorization (OAuth) during installation" on. GitHub   │
     │   then also returns a code, which we use to verify that the             │
     │   installation belongs to this user (see the security note below).      │
     │ - Env vars: GITHUB_APP_ID, GITHUB_APP_SLUG, GITHUB_APP_CLIENT_ID,       │
     │   GITHUB_APP_CLIENT_SECRET, GITHUB_APP_PRIVATE_KEY (PEM, or a path to   │
     │   it), GITHUB_APP_WEBHOOK_SECRET. Server-side only. Never ship a secret │
     │   in the UI bundle (LiveAPI's constants/github.ts makes this mistake).  │
     │                                                                         │
     │ Phase 1: Backend for installation tokens                                │
     │                                                                         │
     │ network/ (outbound HTTP only), new network/providers/github/app.go:     │
     │ - MintAppJWT(appID, key): RS256, 9-minute expiry, using                 │
     │   github.com/golang-jwt/jwt/v5 (already in go.mod).                     │
     │ - CreateInstallationToken(jwt, installationID) → POST                   │
     │   /app/installations/{id}/access_tokens. Returns the token plus         │
     │   expires_at, which is 1 hour out.                                      │
     │ - ExchangeUserCode(code) and ListUserInstallations(userToken) → GET     │
     │   /user/installations, used for the ownership check.                    │
     │ - ListInstallationRepos(instToken) → GET /installation/repositories.    │
     │   /user/repos doesn't work with installation tokens.                    │
     │ - Update network/network_status.md.                                     │
     │                                                                         │
     │ storage/, extend storage/providers/github/token_store.go:               │
     │ - CreateAppInstallationConnector(orgID, installationID, accountLogin):  │
     │   row in integration_tokens with provider='github',                     │
     │   token_type='GITHUB_APP', provider_app_id=<installation_id>,           │
     │   provider_url='https://github.com', access_token = cached installation │
     │   token, expires_at.                                                    │
     │ - GetGitHubTokenByConnector(orgID, connectorID) and                     │
     │   GetConnectorByInstallationID(installationID). Both replace            │
     │   GetLatestGitHubToken, which gets deleted.                             │
     │ - UpdateCachedInstallationToken(id, token, expiresAt), plus a delete or │
     │   mark-suspended function for installation.deleted / suspend.           │
     │ - A migration (db/migrations/): partial unique index on                 │
     │   (provider_app_id) where token_type='GITHUB_APP'. One installation     │
     │   belongs to exactly one org.                                           │
     │ - Update storage/storage_status.md.                                     │
     │                                                                         │
     │ One token resolver, new                                                 │
     │ internal/provider_input/github/token_resolver.go:                       │
     │ ResolveGitHubAccessToken(ctx, rec) (string, error): for PAT, return     │
     │ pat_token. For GITHUB_APP, return the cached access_token if it's valid │
     │ for more than 5 more minutes, otherwise mint a new one and cache it.    │
     │ Every site that reads token.PatToken (the github_provider.go lines      │
     │ above, lrc_fetch.go:33, internal/api/reviews_api.go                     │
     │ ensureValidToken/checkTokenNeedsRefresh at lines 187 and 262, and       │
     │ auto_webhook_installer.go:125) calls this resolver instead.             │
     │ internal/providers/github/github.go:39 keeps taking pat_token from its  │
     │ config map, which is just the resolved token.                           │
     │                                                                         │
     │ Endpoints in internal/api/github_app.go, registered in server.go:       │
     │ - GET /api/v1/github-app/install-url (auth + org middleware chain).     │
     │   Returns                                                               │
     │   https://github.com/apps/<slug>/installations/new?state=<signed        │
     │   state>. The state is an HMAC (JWT secret) over {org_id, user_id,      │
     │   nonce, exp} so the callback can't be replayed into a different org.   │
     │ - POST /api/v1/github-app/callback (auth + org chain). Body:            │
     │   {installation_id, code, state}. Verify the state matches the caller's │
     │   org, exchange code for a user token, and confirm installation_id      │
     │   appears in GET /user/installations. Without that check, anyone could  │
     │   claim someone else's installation ID. Then create the connector,      │
     │   enable manual trigger for its repos (reuse                            │
     │   EnableManualTriggerForAllProjects logic), and return the connector    │
     │   plus repo list.                                                       │
     │ - POST /api/v1/github-app/webhook (public; verifies X-Hub-Signature-256 │
     │   against the App secret). Look up the connector with                   │
     │   payload.installation.id → GetConnectorByInstallationID → org, then    │
     │   hand off to the existing WebhookOrchestratorV2Handler path with that  │
     │   connector, the same way /github-hook/:connector_id does today. Handle │
     │   installation.deleted/suspend and                                      │
     │   installation_repositories.added/removed (repo sync).                  │
     │ - GitHubV2Provider methods take the connector ID from the orchestrator  │
     │   instead of calling FindIntegrationTokenForGitHubRepo(db,              │
     │   repoFullName). That's the actual fix for the cross-tenant bug.        │
     │                                                                         │
     │ Phase 2: UI (LiveReview)                                                │
     │                                                                         │
     │ - ui/src/components/Connector/GitHubConnector.tsx: the default route    │
     │   becomes a Connect with GitHub App card that's large and marked        │
     │   "Recommended · Free". Manual PAT moves behind a "Use a Personal       │
     │   Access Token instead" disclosure, which stays for self-hosted and     │
     │   GHES.                                                                 │
     │ - New route #/git/github-app/callback (page under                       │
     │   ui/src/pages/GitProviders/). Reads installation_id, code and state    │
     │   from the URL, POSTs to the callback endpoint, then shows the repo     │
     │   list with Review a PR (paste URL → existing trigger-review) or opens  │
     │   the latest open PRs.                                                  │
     │ - ui/src/components/Dashboard/OnboardingSteps.tsx: add Step 1: Connect  │
     │   GitHub before "Configure AI", and make the CLI step optional ("or use │
     │   the web"). Note: today isFreePlan sends "Configure AI" to upgrade     │
     │   (line ~93). If we're marketing "FREE", free orgs need either the      │
     │   managed Gemini connector or permission to add a key. That decision is │
     │   tracked separately from this plan.                                    │
     │ - ?mr=<url> deep link: if a URL arrives before login, it's stored via   │
     │   the existing getRedirectAfterLogin (ui/src/pages/Auth/Cloud.tsx).     │
     │   After the GitHub App is connected, the review form is prefilled.      │
     │ - Repo rule: add the new page to megaMenuData.ts (Git Providers group), │
     │   and add or update internal/docindex/docs/routes_guide/git/*.md.       │
     │                                                                         │
     │ Phase 3: onelogin-node (small change, livereview only)                  │
     │                                                                         │
     │ - In src/shared/src/components/Home.js, when app === 'livereview',      │
     │   render GitHub first as a large button with a "Recommended" tag, and   │
     │   collapse the other providers (email, LinkedIn, Authentik) under       │
     │   "Other ways to sign in". Drive this from a small per-app config map   │
     │   so LiveAPI keeps its current layout.                                  │
     │ - Keep the OAuth scopes as they are (read:user,user:email). Repo access │
     │   comes from the App install in LiveReview, not from sign-in. Asking    │
     │   for repo at sign-in would scare people off and give us access to      │
     │   every repo the user has.                                              │
     │                                                                         │
     │ Out of scope / later                                                    │
     │                                                                         │
     │ - Public-PR reviews with no install at all (a server-side token,        │
     │   results shown only in the LiveReview UI). Once the user has           │
     │   authorized the App, the user-to-server token from Phase 1 can read    │
     │   public PRs, so this is a small follow-up.                             │
     │ - Managed Gemini connector for free orgs (separate decision, see the    │
     │   Phase 2 note).                                                        │
     │ - Self-hosted: unchanged, PAT stays the default there.                  │
     │                                                                         │
     │ Verification                                                            │
     │                                                                         │
     │ 1. Register a test GitHub App (e.g. livereview-dev) pointed at an       │
     │    ngrok/cloudflared URL. Fill in .env and run make develop.            │
     │ 2. Unit tests: go test ./network/providers/github/ -run App (JWT        │
     │    claims, token response parsing against httptest) and go test         │
     │    ./internal/api/ -run GitHubApp (bad or foreign state rejected,       │
     │    installation not in /user/installations rejected, bad webhook        │
     │    signature returns 401).                                              │
     │ 3. Cross-tenant regression test: two orgs, each with a GitHub           │
     │    connector. A webhook for org A's installation must resolve org A's   │
     │    connector. Then grep to confirm GetLatestGitHubToken has no          │
     │    remaining callers.                                                   │
     │ 4. Manual E2E: sign in through hexmos.com with GitHub → Connect GitHub  │
     │    → choose one repo → land on the callback → open a PR in that repo →  │
     │    a review comment gets posted. Also: remove the repo in GitHub        │
     │    settings and confirm installation_repositories.removed updates the   │
     │    repo list. Uninstall and confirm the connector is marked removed.    │
     │ 5. make check-status-doc, and npx tsc --noEmit in ui/. No full UI build │
     │    without asking first.                                                │
     ╰─────────────────────────────────────────────────────────────────────────╯

✻ Cooked for 37s · done 3:11 pm

❯ so whap happnes to the existing add git provider flow? the PAT flow etc?      

  Ran 1 shell command (ctrl+o to expand)

● The PAT flow stays, and so does every other provider. The GitHub App is an
  extra option, not a replacement.

  What stays as it is
  - GitLab, Bitbucket, Gitea, Azure DevOps and self-hosted GitLab: untouched.
  - Existing GitHub PAT connectors: nothing to migrate. The rows stay
    token_type='PAT', the per-connector webhooks at /github-hook/:connector_id
    keep receiving events, and ManualGitHubConnector.tsx keeps working.
  - Self-hosted and GitHub Enterprise: PAT stays the only GitHub option there. A
    GitHub App is registered once per server, so a customer's self-hosted
    LiveReview can't use our cloud App. I'd show the App card only when the
    GITHUB_APP_* env vars are set, so self-hosted installs never see it.

  What changes
  1. GitHub connector page (cloud only): "Connect with GitHub App · Recommended
     · Free" becomes the default. "Use a Personal Access Token instead" sits
     underneath as a collapsed section and opens the same PAT form as today.
  2. Token lookup: every GitHub call goes through one resolver. For a PAT row it
     returns pat_token, same as now. For an App row it mints a short-lived
     installation token. PAT users see no difference.
  3. Cross-tenant fix: this touches PAT users too. Today GetLatestGitHubToken()
     picks the newest GitHub token across all orgs. With the fix, webhooks use
     the connector_id that's already in the /github-hook/:connector_id URL.
     That's a bug fix, not a flow change, but it's the part of the change most
     likely to break existing PAT setups, so it needs the two-org regression
     test.

  One gap the plan doesn't cover yet: the same repo connected both ways
  If an org already has a PAT connector with webhooks on repo X and then
  installs the App on X too, both send pull_request events and the PR gets
  reviewed twice. Options:
  - (a) When webhook events overlap, the App connector wins and PAT-webhook
    events for that repo are ignored.
  - (b) On the App callback, detect the overlapping repos and offer to remove
    the old PAT webhooks. There's no webhook-removal helper today (I checked
    internal/), so this means writing one.
  - (c) Block installing the App on repos that already have a PAT connector.

  I'd go with (a) for launch and add (b) later as a "switch to GitHub App"
  button on the PAT connector's details page. I haven't added this section to
  the plan yet.

✻ Brewed for 31s · done 3:13 pm

❯ save the plan to a file                                                       

  Read 1 file, listed 1 directory (ctrl+o to expand)

● I'll save it as LiveReview/docs/github-app-onboarding-plan.md to match the
  other *-plan.md docs. I'm also adding the PAT coexistence section from my last
  answer.

  Ran 1 shell command (ctrl+o to expand)

● Update(docs/github-app-onboarding-plan.md)
  ⎿  User rejected update to docs/github-app-onboarding-plan.md

● Update(docs/github-app-onboarding-plan.md)
  ⎿  User rejected update to docs/github-app-onboarding-plan.md

✻ Cooked for 22s · done 3:15 pm

※ recap: I'm planning a GitHub App flow so cloud users can get a LiveReview review without making a PAT, while the existing PAT flow stays for self-hosted. Next: you pick how to stop double reviews when one repo is connected both ways, and I'll add it to the plan. (disable recaps in /config)

❯ lovestaco [A\],  — 3:14 PM                                                    
  yeah you can use                                                              
  https://docs.google.com/document/d/1lCZ5ZHktAjNCrHs26cJvzx-zi2QeoILBq1wynBqX3 
  H8/edit?usp=sharing                                                           
  Google Docs                                                                   
  LiveReview Cloud: "GitHub path" for a 5-minute first review                   
  LiveReview Cloud: "GitHub path" for a 5-minute first review Context Trials go 
  cold because step 1 asks too much: create a classic PAT (repo, read:org),     
  paste an AI key, install the CLI. For the Product Hunt launch, the goal is:   
  sign in with GitHub → pick repos in GitHub's own dialog → review a PR...      
  Image                                                                         
  I was just checking the plan                                                  
  but this apporach is wrong according to our discusssion                       
  its asking github app installation step after the hexmos login                
  after user lands to LR                                                        
  shrijith — 3:16 PM                                                            
  yes                                                                           
  it's not perfect                                                              
  the thing is -- first login only can ideally give                             
  lovestaco [A\],  — 3:17 PM                                                    
  yes CR does it                                                                
  shrijith — 3:17 PM                                                            
  basically GH login -> LiveReview GH app (select repos) -> ready               
  yeah it is our "one login" getting in the way for the demo                    
  one thing we can have is -- for demo purposes itself -- we can have           
  alternative login route                                                       
  so this login goes smooth                                                     
  with GH                                                                       
  no need to touch existing login                                               
  just add another path                                                         
  and generally link to it                                                      
  in this new path have "other ways" -- goes to standard hexmos one login way   
  something like livereview.hexmos.com/gh-login                                 
  can start here                                                                
  and get a clean login with that                                               
  lovestaco [A\],  — 3:20 PM                                                    
  if we have this then after they use google, we take the github-app            
  installation screen where they select repos?                                  
   [A\],                                                                        
  shrijith — 3:21 PM                                                            
  no                                                                            
  it's our present thing                                                        
  no change there                                                               
  only if they pick /gh-login they get this selector                            
  this new GH dialog can later be integrated into providers                     
  like we did with LiveAPI later                                                
  but that's not relevant for launch                                            
  lovestaco [A\],  — 3:21 PM                                                    
  okay if they use gh-login then whole demo onbaording and review thing will be 
  activated                                                                     
  else no demo just our existing flow                                           
  shrijith — 3:21 PM                                                            
  shouldn't be hard                                                             
   [A\],                                                                        
  shrijith — 3:21 PM                                                            
  yes kind of                                                                   
  i mean this is the least disturbing way to launch i feel                      
  later we can optimize everything else                                         
  for rapid launch now                                                          
  Even for "Start Free" buttons in landing page, we can encourage this path     
  probably                                                                      
  we get more info can you iterate the plan and update the plan md file         
  accordign to this? dont start coding                                          

● I'm merging GitHub login and App install into a single step on a new /gh-login
  route, leaving the onelogin flow untouched. First I'll check how cloud users
  are currently provisioned, since this new route bypasses Hexmos onelogin.

  Searched for 2 patterns, ran 7 shell commands (ctrl+o to expand)

● Status: provisioning is mapped out (EnsureCloudUser is keyed only by email, so
  /gh-login can reuse it). One more check on how the UI is served so /gh-login
  resolves, then I'll rewrite the plan.

  Searched for 1 pattern (ctrl+o to expand)

● I've confirmed unknown paths fall back to the SPA's index.html, so
  livereview.hexmos.com/gh-login already works. I'll now update the plan around
  this path without making any code changes.

● Write(docs/github-app-onboarding-plan.md)
  ⎿  Added 98 lines, removed 53 lines
        1 -# LiveReview Cloud: "GitHub path" for a 5-minute first review 
        1 +# LiveReview Cloud: `/gh-login`, a 5-minute first review      
        2  
        3  ## Context
        4  
        5 -Trials go cold because step 1 asks too much: create a classic 
          -PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          -the Product Hunt launch, the goal is: **sign in with GitHub → 
          -pick repos in GitHub's own dialog → review a PR right away**. 
          -No PAT, no webhook setup.                                     
        5 +Trials go cold because step 1 asks too much: create a classic 
          +PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          +the Product Hunt launch, the target is **GitHub login → LiveRe
          +view GitHub App (select repos) → ready**, one GitHub screen wi
          +th no PAT and no webhook setup. CodeRabbit works this way.    
        6  
        7 -What exists today:                                            
        8 -- **onelogin-node** (`src/shared/src/components/Home.js:110`):
          - "Sign in with GitHub" is an OAuth App (`Ov23li…`) with `scope
          -=read:user,user:email`. The code is exchanged inside the Parse
          - cloud function `externalProviderSignup`, so LiveReview never 
          -gets a GitHub token. **Sign-in gives identity only, not repo a
          -ccess.** That's expected and should stay that way.            
        9 -- **LiveReview GitHub connector is PAT-only** (`ui/src/compone
          -nts/Connector/GitHubConnector.tsx`, stored by `internal/api/pa
          -t_token.go` into `integration_tokens` with `token_type='PAT'`)
          -.                                                             
       10 -- **LiveAPI already did this** with the `liveapi-github` GitHu
          -b App (`liveapi/client/src/services/api/github.ts:46`, `github
          -.com/apps/<name>/installations/new`). We copy that pattern.   
        7 +The Hexmos onelogin gets in the way. Its GitHub button is an O
          +Auth App with `read:user,user:email` (`onelogin-node/src/share
          +d/src/components/Home.js:110`, exchanged in `fw-parse/isAuthen
          +ticatedMiddleware.js:146`). That gives us identity but no repo
          + access, so a repo-access step would always come *after* landi
          +ng in LiveReview.                                             
        8  
       12 -**Existing bug this work must fix:** `storage/providers/github
          -/token_store.go:21` `GetLatestGitHubToken()` selects the newes
          -t GitHub token **across all orgs** (no `org_id` filter). Every
          - webhook/comment path in `internal/provider_input/github/githu
          -b_provider.go` (lines 145, 198, 258, 272, 286, 307, 409, 440) 
          -and `lrc_fetch.go:28` uses it through `FindIntegrationTokenFor
          -GitHubRepo`. In cloud, org A's PR events can be fetched and co
          -mmented on with org B's credentials. The App flow has to resol
          -ve tokens per connector/org, so this gets fixed along the way.
        9 +**Decision (from the discussion with Shrijith):** don't touch 
          +the existing login. Add a separate demo path at **`livereview.
          +hexmos.com/gh-login`**:                                       
       10 +- `/gh-login` → GitHub App install + authorize (one screen) → 
          +signed in, repos connected → **demo onboarding** → first revie
          +w.                                                            
       11 +- `/gh-login` has an "Other ways to sign in" link that goes to
          + the standard Hexmos onelogin. That flow stays **exactly as it
          + is today**, with no demo onboarding.                         
       12 +- Landing-page "Start Free" CTAs point to `/gh-login`.        
       13 +- Bringing the GitHub App into Git Providers (as LiveAPI did) 
          +comes later and isn't needed for launch.                      
       14  
       14 -## Approach: a "LiveReview" GitHub App, installed during onboa
          -rding                                                         
       15 +## User flow                                                  
       16  
       16 -Leave the Hexmos sign-in as identity-only. After login, LiveRe
          -view onboarding shows one big **Connect GitHub (Free)** button
          - that opens the App install page. On that page the user picks 
          -"All" or "Only select repositories". GitHub redirects back, we
          - store one connector per installation, and webhooks arrive at 
          -a single App-level endpoint. No per-repo webhook install and n
          -o PAT.                                                        
       17 +```                                                           
       18 +hexmos.com/livereview  "Start Free" / paste PR URL            
       19 +        │  https://livereview.hexmos.com/gh-login?src=<cta>[&m
          +r=<pr-url>]                                                   
       20 +        ▼                                                     
       21 +#/gh-login     [ Continue with GitHub · FREE ]      "Other way
          +s to sign in" → existing Cloud.tsx login                      
       22 +        │  GET /api/v1/auth/github-app/start  (sets state, 302
          +)                                                             
       23 +        ▼                                                     
       24 +github.com/apps/<slug>/installations/new   ← user picks All / 
          +Only select repos → Install & Authorize                       
       25 +        │  302 ?code&installation_id&setup_action&state       
       26 +        ▼                                                     
       27 +GET /api/v1/auth/github-app/callback   (server-side: verify, p
          +rovision user+org+connector, one-time code)                   
       28 +        │  302 /#/gh-login/complete?code=<one-time>           
       29 +        ▼                                                     
       30 +#/gh-login/complete → POST /api/v1/auth/github-app/exchange → 
          +LoginResponse → handleLoginSuccess                            
       31 +        ▼                                                     
       32 +#/get-started/github   repos → open PRs → [Review this PR]  (p
          +refilled from ?mr=)  → review detail page                     
       33 +```                                                           
       34  
       18 -### Phase 0: Create the GitHub App (one-time, in GitHub settin
          -gs, done by a human)                                          
       19 -- Permissions: Pull requests **R/W**, Contents **R**, Metadata
          - **R**, Issues **R/W** (PR conversation comments use the issue
          -s API).                                                       
       35 +Returning users: `/gh-login` also shows a small "Already insta
          +lled? Sign in" link. It goes through the App's OAuth authorize
          + URL (`github.com/login/oauth/authorize?client_id=<app client 
          +id>`) and returns to the same callback with no `installation_i
          +d`. If that user turns out to have no installations, the callb
          +ack redirects them to the install URL.                        
       36 +                                                              
       37 +## Phase 0: Create the GitHub App (one-time, by a human, in Gi
          +tHub settings)                                                
       38 +- Permissions: Pull requests **R/W**, Contents **R**, Metadata
          + **R**, Issues **R/W** (PR conversation comments use the issue
          +s API). Account permission: Email addresses **R**.            
       39  - Events: `pull_request`, `pull_request_review_comment`, `issu
           e_comment`, `installation`, `installation_repositories`.
       21 -- Webhook URL: `https://<cloud>/api/v1/github-app/webhook`, wi
          -th a webhook secret.                                          
       22 -- Setup URL: `https://<cloud>/#/git/github-app/callback`, with
          - "Redirect on update" on.                                     
       23 -- **"Request user authorization (OAuth) during installation" o
          -n.** GitHub then also returns a `code`, which we use to verify
          - that the installation belongs to this user (see the security 
          -note below).                                                  
       24 -- Env vars: `GITHUB_APP_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_CL
          -IENT_ID`, `GITHUB_APP_CLIENT_SECRET`, `GITHUB_APP_PRIVATE_KEY`
          - (PEM, or a path to it), `GITHUB_APP_WEBHOOK_SECRET`. Server-s
          -ide only. Never ship a secret in the UI bundle (LiveAPI's `con
          -stants/github.ts` makes this mistake).                        
       40 +- **"Request user authorization (OAuth) during installation" o
          +n.** With it on, one install screen also yields a `code`, whic
          +h provides identity. The Callback URL is `https://livereview.h
          +exmos.com/api/v1/auth/github-app/callback`. GitHub disables th
          +e separate Setup URL in this mode, which is fine.             
       41 +- Webhook URL: `https://livereview.hexmos.com/api/v1/github-ap
          +p/webhook`, with a secret.                                    
       42 +- Env vars (server-side only, never in the UI bundle): `GITHUB
          +_APP_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_CLIENT_ID`, `GITHUB_A
          +PP_CLIENT_SECRET`, `GITHUB_APP_PRIVATE_KEY`, `GITHUB_APP_WEBHO
          +OK_SECRET`. If they're unset, `/gh-login` falls back to the no
          +rmal login page. That's the self-hosted case.                 
       43  
       26 -### Phase 1: Backend for installation tokens                  
       44 +## Phase 1: Backend                                           
       45  
       28 -**network/** (outbound HTTP only), new `network/providers/gith
          -ub/app.go`:                                                   
       46 +### network/ (outbound HTTP only): new `network/providers/gith
          +ub/app.go`                                                    
       47  - `MintAppJWT(appID, key)`: RS256, 9-minute expiry, using `git
           hub.com/golang-jwt/jwt/v5` (already in `go.mod`).
       30 -- `CreateInstallationToken(jwt, installationID)` → `POST /app/
          -installations/{id}/access_tokens`. Returns the token plus `exp
          -ires_at`, which is 1 hour out.                                
       31 -- `ExchangeUserCode(code)` and `ListUserInstallations(userToke
          -n)` → `GET /user/installations`, used for the ownership check.
       32 -- `ListInstallationRepos(instToken)` → `GET /installation/repo
          -sitories`. `/user/repos` doesn't work with installation tokens
          -.                                                             
       48 +- `CreateInstallationToken(appJWT, installationID)` → `POST /a
          +pp/installations/{id}/access_tokens` (valid 1 hour).          
       49 +- `ExchangeUserCode(code)` → user-to-server token. `GetUser`, 
          +`GetUserEmails`, and `ListUserInstallations` (`GET /user/insta
          +llations`).                                                   
       50 +- `ListInstallationRepos(instToken)` → `GET /installation/repo
          +sitories`, and `ListOpenPulls(instToken, owner, repo)` for the
          + demo page.                                                   
       51  - Update `network/network_status.md`.
       52  
       35 -**storage/**, extend `storage/providers/github/token_store.go`
          -:                                                             
       36 -- `CreateAppInstallationConnector(orgID, installationID, accou
          -ntLogin)`: row in `integration_tokens` with `provider='github'
          -`, `token_type='GITHUB_APP'`, `provider_app_id=<installation_i
          -d>`, `provider_url='https://github.com'`, `access_token` = cac
          -hed installation token, `expires_at`.                         
       37 -- `GetGitHubTokenByConnector(orgID, connectorID)` and `GetConn
          -ectorByInstallationID(installationID)`. Both replace `GetLates
          -tGitHubToken`, which gets deleted.                            
       38 -- `UpdateCachedInstallationToken(id, token, expiresAt)`, plus 
          -a delete or mark-suspended function for `installation.deleted`
          - / `suspend`.                                                 
       39 -- A migration (`db/migrations/`): partial unique index on `(pr
          -ovider_app_id)` where `token_type='GITHUB_APP'`. One installat
          -ion belongs to exactly one org.                               
       53 +### storage/                                                  
       54 +- `storage/providers/github/token_store.go`:                  
       55 +  - `UpsertAppInstallationConnector(orgID, installationID, acc
          +ountLogin)`: an `integration_tokens` row with `provider='githu
          +b'`, `token_type='GITHUB_APP'`, `provider_app_id=<installation
          +_id>`, `provider_url='https://github.com'`, plus the cached `a
          +ccess_token` and `expires_at`. It's idempotent per installatio
          +n.                                                            
       56 +  - `GetGitHubTokenByConnector(orgID, connectorID)` and `GetCo
          +nnectorByInstallationID(id)`.                                 
       57 +  - `UpdateCachedInstallationToken`, and `MarkInstallationRemo
          +ved` for `installation.deleted/suspend`.                      
       58 +- Migrations (`db/migrations/`):                              
       59 +  - Partial unique index on `integration_tokens(provider_app_i
          +d) WHERE token_type='GITHUB_APP'`, so one installation belongs
          + to exactly one org.                                          
       60 +  - `orgs.signup_path text` (values `gh_app` or NULL). This is
          + what turns on the demo onboarding.                           
       61 +  - Table `gh_login_codes(code_hash, user_id, org_id, expires_
          +at, used_at)` for the one-time code handoff (60-second TTL, si
          +ngle use).                                                    
       62 +- `storage/users/`: move the find-or-create user + personal or
          +g + super_admin role logic out of `EnsureCloudUser` (`internal
          +/api/auth/handlers.go:636`, inline SQL today) into a `Provisio
          +nCloudUser(tx, email, first, last, source)` function. `EnsureC
          +loudUser` and the new callback both call it, so there's one pr
          +ovisioning path.                                              
       63  - Update `storage/storage_status.md`.
       64  
       42 -**One token resolver**, new `internal/provider_input/github/to
          -ken_resolver.go`:                                             
       43 -`ResolveGitHubAccessToken(ctx, rec) (string, error)`: for `PAT
          -`, return `pat_token`. For `GITHUB_APP`, return the cached `ac
          -cess_token` if it's valid for more than 5 more minutes, otherw
          -ise mint a new one and cache it. Every site that reads `token.
          -PatToken` (the `github_provider.go` lines above, `lrc_fetch.go
          -:33`, `internal/api/reviews_api.go` `ensureValidToken`/`checkT
          -okenNeedsRefresh` at lines 187 and 262, and `auto_webhook_inst
          -aller.go:125`) calls this resolver instead. `internal/provider
          -s/github/github.go:39` keeps taking `pat_token` from its confi
          -g map, which is just the resolved token.                      
       65 +### Endpoints: new `internal/api/auth/github_app_login.go` (pr
          +e-auth) + `internal/api/github_app.go`                        
       66 +- `GET /api/v1/auth/github-app/start?mode=install|signin&src=&
          +mr=` (public). Generates `state` = HMAC over `{nonce, src, mr,
          + exp:10m}`, sets it as an HttpOnly cookie too, and 302s to `in
          +stallations/new?state=` or to the OAuth authorize URL.        
       67 +- `GET /api/v1/auth/github-app/callback` (public). Steps:     
       68 +  1. Check that the `state` HMAC is valid, unexpired, and matc
          +hes the cookie.                                               
       69 +  2. Exchange `code` for a user token.                        
       70 +  3. From `GET /user/emails`, take the **primary and `verified
          +: true`** email and reject the login if there isn't one. Note 
          +that Parse's GitHub path doesn't check `verified`, and we shou
          +ldn't copy that.                                              
       71 +  4. If `installation_id` is present, it must appear in `GET /
          +user/installations`. Without this check anyone could claim som
          +eone else's installation.                                     
       72 +  5. `ProvisionCloudUser(..., source="gh-login:<src>")` and se
          +t `orgs.signup_path='gh_app'` on a new org.                   
       73 +  6. `UpsertAppInstallationConnector`. If the installation is 
          +already bound to a *different* org, show the error page "This 
          +GitHub installation is connected to another LiveReview account
          +".                                                            
       74 +  7. Sync repos into projects with manual trigger enabled (reu
          +se the `EnableManualTriggerForAllProjects` logic, `server.go:1
          +471`).                                                        
       75 +  8. Store a one-time code and 302 to `/#/gh-login/complete?co
          +de=…&mr=…`. The LiveReview JWT never goes in the URL.         
       76 +- `POST /api/v1/auth/github-app/exchange {code}` (public). Con
          +sumes the code and returns the same body as `EnsureCloudUser` 
          +(`user`, `tokens`, `organizations`), via `tokenService.CreateT
          +okenPair`. Also fires the existing signup Discord notification
          + with source `gh-login`.                                      
       77 +- `POST /api/v1/github-app/webhook` (public, verifies `X-Hub-S
          +ignature-256`). Uses `installation.id` → connector → org, then
          + hands off to the existing `WebhookOrchestratorV2Handler` path
          + with that connector, as `/github-hook/:connector_id` does. Al
          +so handles `installation.*` and `installation_repositories.add
          +ed/removed`.                                                  
       78 +- `GET /api/v1/github-app/demo` (full chain: `RequireAuth` → `
          +BuildOrgContextFromHeader` → `ValidateOrgAccess` → `BuildPermi
          +ssionContext`). Returns the org's App connector, its repos, an
          +d the open PRs per repo (capped, e.g. 5 per repo, 10 repos).  
       79  
       45 -**Endpoints** in `internal/api/github_app.go`, registered in `
          -server.go`:                                                   
       46 -- `GET /api/v1/github-app/install-url` (auth + org middleware 
          -chain). Returns `https://github.com/apps/<slug>/installations/
          -new?state=<signed state>`. The state is an HMAC (JWT secret) o
          -ver `{org_id, user_id, nonce, exp}` so the callback can't be r
          -eplayed into a different org.                                 
       47 -- `POST /api/v1/github-app/callback` (auth + org chain). Body:
          - `{installation_id, code, state}`. Verify the state matches th
          -e caller's org, exchange `code` for a user token, and confirm 
          -`installation_id` appears in `GET /user/installations`. **With
          -out that check, anyone could claim someone else's installation
          - ID.** Then create the connector, enable manual trigger for it
          -s repos (reuse `EnableManualTriggerForAllProjects` logic), and
          - return the connector plus repo list.                         
       48 -- `POST /api/v1/github-app/webhook` (public; verifies `X-Hub-S
          -ignature-256` against the App secret). Look up the connector w
          -ith `payload.installation.id` → `GetConnectorByInstallationID`
          - → org, then hand off to the existing `WebhookOrchestratorV2Ha
          -ndler` path with that connector, the same way `/github-hook/:c
          -onnector_id` does today. Handle `installation.deleted/suspend`
          - and `installation_repositories.added/removed` (repo sync).   
       49 -- `GitHubV2Provider` methods take the connector ID from the or
          -chestrator instead of calling `FindIntegrationTokenForGitHubRe
          -po(db, repoFullName)`. That's the actual fix for the cross-ten
          -ant bug.                                                      
       80 +### Token resolver + cross-tenant fix (required, not optional)
       81 +- New `internal/provider_input/github/token_resolver.go` `Reso
          +lveGitHubAccessToken(ctx, rec)`. `PAT` → `pat_token`, unchange
          +d. `GITHUB_APP` → the cached token if it's valid for more than
          + 5 more minutes, otherwise mint a new one and cache it.       
       82 +- **Existing bug:** `storage/providers/github/token_store.go:2
          +1` `GetLatestGitHubToken()` picks the newest GitHub token **ac
          +ross all orgs**. Every path in `internal/provider_input/github
          +/github_provider.go` (lines 145, 198, 258, 272, 286, 307, 409,
          + 440) and `lrc_fetch.go:28` uses it. The launch will create ma
          +ny org GitHub connectors, so this *will* mix tenants. Fix: `Gi
          +tHubV2Provider` takes the connector from the webhook route/orc
          +hestrator and resolves through `GetGitHubTokenByConnector`. De
          +lete `GetLatestGitHubToken`.                                  
       83 +- Callers that read `token.PatToken` switch to the resolver: `
          +github_provider.go`, `lrc_fetch.go:33`, `internal/api/reviews_
          +api.go` `ensureValidToken`/`checkTokenNeedsRefresh` (lines 187
          + and 262; GitHub App tokens *do* expire), and `auto_webhook_in
          +staller.go:125` (App connectors skip per-repo webhook install)
          +.                                                             
       84  
       51 -### Phase 2: UI (LiveReview)                                  
       52 -- `ui/src/components/Connector/GitHubConnector.tsx`: the defau
          -lt route becomes a **Connect with GitHub App** card that's lar
          -ge and marked "Recommended · Free". Manual PAT moves behind a 
          -"Use a Personal Access Token instead" disclosure, which stays 
          -for self-hosted and GHES.                                     
       53 -- New route `#/git/github-app/callback` (page under `ui/src/pa
          -ges/GitProviders/`). Reads `installation_id`, `code` and `stat
          -e` from the URL, POSTs to the callback endpoint, then shows th
          -e repo list with **Review a PR** (paste URL → existing `trigge
          -r-review`) or opens the latest open PRs.                      
       54 -- `ui/src/components/Dashboard/OnboardingSteps.tsx`: add **Ste
          -p 1: Connect GitHub** before "Configure AI", and make the CLI 
          -step optional ("or use the web"). Note: today `isFreePlan` sen
          -ds "Configure AI" to *upgrade* (line ~93). If we're marketing 
          -"FREE", free orgs need either the managed Gemini connector or 
          -permission to add a key. That decision is tracked separately f
          -rom this plan.                                                
       55 -- `?mr=<url>` deep link: if a URL arrives before login, it's s
          -tored via the existing `getRedirectAfterLogin` (`ui/src/pages/
          -Auth/Cloud.tsx`). After the GitHub App is connected, the revie
          -w form is prefilled.                                          
       56 -- Repo rule: add the new page to `megaMenuData.ts` (Git Provid
          -ers group), and add or update `internal/docindex/docs/routes_g
          -uide/git/*.md`.                                               
       85 +## Phase 2: LiveReview UI                                     
       86 +- **`/gh-login` entry:** `cmd/ui.go:374` already serves `index
          +.html` for unknown paths. In the `App.tsx` bootstrap, if `loca
          +tion.pathname === '/gh-login'`, `replace` to `/#/gh-login` and
          + keep the query string.                                       
       87 +- **`#/gh-login` page** (`ui/src/pages/Auth/GhLogin.tsx`): Liv
          +eReview logo, "Review your first PR in 2 minutes · FREE", one 
          +large **Continue with GitHub** button (→ `/api/v1/auth/github-
          +app/start?mode=install&src&mr`), a small "Already installed? S
          +ign in" link (`mode=signin`), and an **"Other ways to sign in"
          +** link that runs the existing `handleLoginClick` from `ui/src
          +/pages/Auth/Cloud.tsx`. If the user is already logged in, it g
          +oes straight to `#/get-started/github`.                       
       88 +- **`#/gh-login/complete`:** POSTs the code to `/exchange`, th
          +en calls the existing `handleLoginSuccess` and the `organizati
          +ons/setOrganizationsFromAuth` dispatch, the same as `Cloud.tsx
          +`. Then it navigates to `#/get-started/github` (keeping `mr`).
       89 +- **`#/get-started/github` (demo onboarding)**, shown only whe
          +n the org has `signup_path='gh_app'`:                         
       90 +  1. **Repos connected ✓** lists the repos, with "Add more rep
          +os" linking to `https://github.com/settings/installations/<id>
          +`.                                                            
       91 +  2. **AI ✓**, or a one-field step. See open decision 1.      
       92 +  3. **Pick a PR**: open PRs per repo, plus a "paste a PR URL"
          + box prefilled from `?mr=`. **Review this PR** calls the exist
          +ing `POST /api/v1/connectors/trigger-review`, then navigates t
          +o the review detail page for live progress. Comments appear on
          + the PR as `LiveReview[bot]`.                                 
       93 +  4. A small "Next: install the CLI for pre-commit reviews" li
          +nk to the existing `OnboardingSteps`.                         
       94 +- The dashboard for `gh_app` orgs shows a "Continue setup" ban
          +ner pointing to `#/get-started/github` until the first review 
          +runs. Non-`gh_app` orgs see exactly what they see today.      
       95 +- Repo rules: add `#/get-started/github` to `megaMenuData.ts` 
          +(visible only for `gh_app` orgs), and add `internal/docindex/d
          +ocs/routes_guide/auth/gh-login.md` and `.../get-started-github
          +.md`.                                                         
       96  
       58 -### Phase 3: onelogin-node (small change, livereview only)    
       59 -- In `src/shared/src/components/Home.js`, when `app === 'liver
          -eview'`, render GitHub first as a large button with a "Recomme
          -nded" tag, and collapse the other providers (email, LinkedIn, 
          -Authentik) under "Other ways to sign in". Drive this from a sm
          -all per-app config map so LiveAPI keeps its current layout.   
       60 -- Keep the OAuth scopes as they are (`read:user,user:email`). 
          -Repo access comes from the App install in LiveReview, not from
          - sign-in. Asking for `repo` at sign-in would scare people off 
          -and give us access to every repo the user has.                
       97 +## Phase 3: hexmoshomepage CTAs                               
       98 +Point "Start Free" to `https://livereview.hexmos.com/gh-login?
          +src=<where>`:                                                 
       99 +- `pages/livereview/index.tsx` (`handleBuyNow` line 529, Start
          + Free at 562, 987, 2467)                                      
      100 +- `components/livereview/StartFreeModal.tsx:146` (Try Cloud; t
          +he self-hosted option stays)                                  
      101 +- `components/livereview/PricingSection.tsx:181` (free plan on
          +ly; paid checkout unchanged)                                  
      102 +- `pages/livereview/blog/index.tsx:153`, `pages/livereview/blo
          +g/[slug].tsx:174`, `pages/livereview/why-livereview.tsx:222,38
          +4`                                                            
      103 +- Optional hero input "Paste a GitHub PR URL → Review it free"
          +, modeled on LiveAPI's `RepoInput` (`pages/liveapi/index.tsx:1
          +29`). It sends `/gh-login?mr=<encoded url>&src=hero`.         
      104  
       62 -### fw-parse: no change required                              
       63 -- GitHub sign-in is handled in `isAuthenticatedMiddleware.js:1
          -46`, which exchanges the code via the OAuth App `Ov23li…`. The
          -n `cloud/oneLogin/githubOneLogin.js` reads `/user` and `/user/
          -emails`, and the token is thrown away. That's correct for iden
          -tity-only login, and the App install never goes through Parse.
       64 -- Optional: add the GitHub `login` and numeric `id` to the use
          -r data/JWT, so LiveReview can say "Connect GitHub as @login" a
          -nd preselect that account on the install page (`/installations
          -/new/permissions?target_id=<id>`).                            
       65 -- Hygiene (separate from this work): `GITHUB_SECRET_ID` is har
          -d-coded in `cloud/config.js:119`, so move it to an env var. Al
          -so, the code exchange is a GET with the secret in the query st
          -ring, so it can end up in logs. Switch it to a POST body.     
      105 +## What does NOT change                                       
      106 +- **onelogin-node and fw-parse:** no changes. The standard log
          +in keeps its current providers and order.                     
      107 +- **Git Providers page and the PAT flow:** unchanged for every
          +one, including GitLab, Bitbucket, Gitea and Azure connectors. 
          +Existing PAT connectors keep `/github-hook/:connector_id`. The
          + only effect on them is the cross-tenant fix, which is a bug f
          +ix.                                                           
      108 +- **Self-hosted:** no `GITHUB_APP_*` env vars, so `/gh-login` 
          +redirects to the normal login.                                
      109 +- If a `gh_app` org also adds a PAT connector covering the sam
          +e repo, the App connector wins and PAT-webhook events for that
          + repo are ignored (no double reviews).                        
      110 +- **gh-login users have no Hexmos/Parse user.** That's fine, b
          +ecause LiveReview keys users by email (`EnsureCloudUser`). If 
          +they later sign in through onelogin with the same email, they 
          +land in the same account.                                     
      111  
       67 -## Out of scope / later                                       
       68 -- Public-PR reviews with no install at all (a server-side toke
          -n, results shown only in the LiveReview UI). Once the user has
          - authorized the App, the user-to-server token from Phase 1 can
          - read public PRs, so this is a small follow-up.               
       69 -- Managed Gemini connector for free orgs (separate decision, s
          -ee the Phase 2 note).                                         
       70 -- Self-hosted: unchanged, PAT stays the default there.        
      112 +## Open decisions (need Shrijith's call before coding)        
      113 +1. **AI for the demo.** Options: (a) auto-provision the manage
          +d "LiveReview AI Model" connector for `gh_app` orgs, using the
          + same insert as `ConfirmPurchase` in `internal/license/payment
          +/subscription_service.go:1603`, with a small LOC cap. This is 
          +the true zero-step option. (b) An inline "paste Gemini key" fi
          +eld with a link to AI Studio. Today `OnboardingSteps.tsx:~93` 
          +sends free-plan users to *upgrade* when they click Configure A
          +I, so "FREE" needs one of these.                              
      114 +2. **Demo limits.** Shrijith suggested "7 days, maybe 1 repo".
          + Do we enforce a repo cap (e.g. review only the first N repos 
          +selected) and a trial window, or just rely on the LOC quota?  
      115 +3. **Auto-review new PRs.** Off by default (manual trigger onl
          +y, with an opt-in toggle on the demo page), or on for selected
          + repos?                                                       
      116  
      117  ## Verification
       73 -1. Register a test GitHub App (e.g. `livereview-dev`) pointed 
          -at an ngrok/cloudflared URL. Fill in `.env` and run `make deve
          -lop`.                                                         
       74 -2. Unit tests: `go test ./network/providers/github/ -run App` 
          -(JWT claims, token response parsing against `httptest`) and `g
          -o test ./internal/api/ -run GitHubApp` (bad or foreign state r
          -ejected, installation not in `/user/installations` rejected, b
          -ad webhook signature returns 401).                            
       75 -3. Cross-tenant regression test: two orgs, each with a GitHub 
          -connector. A webhook for org A's installation must resolve org
          - A's connector. Then grep to confirm `GetLatestGitHubToken` ha
          -s no remaining callers.                                       
       76 -4. Manual E2E: sign in through hexmos.com with GitHub → Connec
          -t GitHub → choose one repo → land on the callback → open a PR 
          -in that repo → a review comment gets posted. Also: remove the 
          -repo in GitHub settings and confirm `installation_repositories
          -.removed` updates the repo list. Uninstall and confirm the con
          -nector is marked removed.                                     
      118 +1. Register a dev App (`livereview-dev`) pointed at a cloudfla
          +red/ngrok URL, set the `GITHUB_APP_*` vars, and run `make deve
          +lop`.                                                         
      119 +2. Go tests: `go test ./network/providers/github/ -run App` (J
          +WT claims, token parsing against `httptest`). `go test ./inter
          +nal/api/... -run GitHubApp`: bad or expired state is rejected,
          + an unverified email is rejected, an installation missing from
          + `/user/installations` is rejected, an installation bound to a
          +nother org is rejected, a one-time code works once only, and a
          + bad webhook signature returns 401.                           
      120 +3. Tenant isolation: two orgs (one App, one PAT, plus a PAT/PA
          +T pair). A webhook for org A resolves only org A's connector. 
          +Grep confirms `GetLatestGitHubToken` has no callers left.     
      121 +4. Manual E2E in a private window: landing "Start Free" → `/gh
          +-login` → GitHub install picking one repo → land on `#/get-sta
          +rted/github` → Review a PR → bot comments on the PR. Repeat wi
          +th `?mr=<url>`. Also: "Other ways to sign in" behaves exactly 
          +as today, uninstalling the App marks the connector removed, an
          +d `installation_repositories.removed` updates the repo list.  
      122  5. `make check-status-doc`, and `npx tsc --noEmit` in `ui/`. N
           o full UI build without asking first.

  Ran 1 shell command (ctrl+o to expand)

● I rewrote the plan around a separate /gh-login path and saved it to
  LiveReview/docs/github-app-onboarding-plan.md. No code has changed.

  The new flow: Start Free → livereview.hexmos.com/gh-login → one GitHub screen
  (pick repos, then Install & Authorize) → signed in with repos connected → demo
  onboarding page → click "Review this PR" → the bot comments on the PR.

  - One screen for login and repo access: this works with the GitHub App setting
    "Request user authorization during installation". GitHub's install screen
    then also returns a login code, so we get the user's identity and their
    chosen repos together. We never need a separate sign-in.
  - Existing login untouched: onelogin-node and fw-parse don't change. The
    "Other ways to sign in" link on /gh-login goes to the current Hexmos login,
    which works exactly as today and doesn't show the demo onboarding.
  - Demo onboarding only for this path: new orgs get signup_path='gh_app', and
    only those orgs see the demo page.
  - Account creation: I'll pull the find-or-create logic out of EnsureCloudUser
    into one shared function. Users are matched by email, so someone who later
    signs in through Hexmos login with the same email lands in the same account.
  - Landing page: I listed every "Start Free" button in hexmoshomepage that
    should point to /gh-login, plus an optional "paste a PR URL" box on the
    hero.
  - Still a must-fix: GetLatestGitHubToken() picks a GitHub token from any org.
    The launch will add many GitHub-connected orgs, so this bug would mix up
    which org's credentials get used. The plan fixes it.
  - The /gh-login URL works with no server change: the Go server already serves
    the app for unknown paths, so a small redirect on page load is enough.

  Shrijith needs to decide three things before coding (also listed at the bottom
  of the plan):
  1. AI for the demo: either auto-create our managed AI model for these orgs
     with a small usage cap (no setup step at all), or ask for a Gemini key in
     one field. Today free-plan users who click "Configure AI" get sent to
     upgrade, so "FREE" needs one of these.
  2. Demo limits: a cap on repos and a 7-day window, or just the usage quota?
  3. Auto-review new PRs: off by default with a toggle, or on for the selected
     repos?

✻ Cogitated for 2m 40s · done 3:26 pm

※ recap: We're planning the 5-minute GitHub onboarding for the LiveReview Product Hunt launch, and the plan is now rewritten around a separate /gh-login path in docs/github-app-onboarding-plan.md. Next, Shrijith needs to decide on demo AI, limits, and auto-review before coding starts. (disable recaps in /config)

❯ for testing it locally i use manaul talent 3 thing so for the one time Create 
  the GitHub App (one-time, by a human, in GitHub settings) just give this      
  again with that manual talent links dont need to update the plan              
  Ran 2 shell commands (ctrl+o to expand)

● Here's the GitHub App setup for local testing through your ngrok domain
  talented-manually-turkey.ngrok-free.app (the one scripts/setup_local_dev.sh
  uses). It's ngrok → local nginx :80 → API.

  Create the dev App

  Go to https://github.com/settings/apps/new. To own it under an org instead,
  use https://github.com/organizations/<org>/settings/apps/new.

  Field: GitHub App name
  Value: livereview-dev-<yourname> (must be unique on GitHub; this becomes the
  slug)
  ────────────────────────────────────────
  Field: Homepage URL
  Value: https://talented-manually-turkey.ngrok-free.app
  ────────────────────────────────────────
  Field: Callback URL
  Value: https://talented-manually-turkey.ngrok-free.app/api/v1/auth/github-app/
  callback
  ────────────────────────────────────────
  Field: Expire user authorization tokens
  Value: ✅ on (default)
  ────────────────────────────────────────
  Field: Request user authorization (OAuth) during installation
  Value: ✅ on. This greys out Setup URL, which is expected.
  ────────────────────────────────────────
  Field: Enable Device Flow
  Value: off
  ────────────────────────────────────────
  Field: Webhook → Active
  Value: ✅
  ────────────────────────────────────────
  Field: Webhook URL
  Value:
  https://talented-manually-turkey.ngrok-free.app/api/v1/github-app/webhook
  ────────────────────────────────────────
  Field: Webhook secret
  Value: generate one: openssl rand -hex 32
  ────────────────────────────────────────
  Field: Where can this GitHub App be installed?
  Value: Any account, so a test org or a second account can install it

  Repository permissions
  - Pull requests: Read & write
  - Contents: Read-only
  - Issues: Read & write
  - Metadata: Read-only (required, preselected)

  Account permissions
  - Email addresses: Read-only

  Subscribe to events
  - Pull request
  - Pull request review comment
  - Issue comment

  installation and installation_repositories events are sent automatically, so
  they don't need to be ticked.

  Click Create GitHub App. Then, on the App's settings page:
  1. Note the App ID and Client ID at the top.
  2. Generate a new client secret and copy it right away, since GitHub only
     shows it once.
  3. Scroll down, click Generate a private key, and save the .pem it downloads
     somewhere outside the repo.

  LiveReview .env

  GITHUB_APP_ID=<app id>
  GITHUB_APP_SLUG=livereview-dev-<yourname>
  GITHUB_APP_CLIENT_ID=<client id, Iv23li...>
  GITHUB_APP_CLIENT_SECRET=<client secret>
  GITHUB_APP_PRIVATE_KEY=/home/lovestaco/keys/livereview-dev.private-key.pem
  GITHUB_APP_WEBHOOK_SECRET=<the openssl value>

  Test URLs (once it's built)

  - Entry: https://talented-manually-turkey.ngrok-free.app/gh-login?src=local
  - With a PR: https://talented-manually-turkey.ngrok-free.app/gh-login?src=loca
    l&mr=https://github.com/<owner>/<repo>/pull/<n>
  - Install page directly:
    https://github.com/apps/livereview-dev-<yourname>/installations/new
  - Webhook delivery log and redelivery: App settings → Advanced → Recent 
    Deliveries
  - To retest a fresh install: uninstall at
    https://github.com/settings/installations, and revoke the authorization at
    https://github.com/settings/apps/authorizations.

  Things to watch for
  - ngrok's warning page: on a browser's first visit, ngrok free shows a "You
    are about to visit…" page. The GitHub redirect to the callback is a browser
    navigation, so click through it once per browser session. Webhooks are
    server-to-server and aren't affected.
  - nginx routing: check that the nginx config from setup_local_dev.sh sends
    both /api/ and /gh-login to the right place. /api/ must reach the Go server
    on :8888, and /gh-login must reach the UI so the app's redirect from
    /gh-login to /#/gh-login can run.
  - Domain changes: keep the dev App separate from the production one. If the
    ngrok domain ever changes, update both the Callback URL and the Webhook URL
    in the App settings.

✻ Baked for 49s · done 3:42 pm

❯ no its https://manual-talent3.apps.hexmos.com/                                

  Ran 2 shell commands (ctrl+o to expand)

● Here it is again with https://manual-talent3.apps.hexmos.com. Start it with
  make niceurl3.

  Create the dev App

  Go to https://github.com/settings/apps/new (or
  https://github.com/organizations/<org>/settings/apps/new to own it under an
  org).

  Field: GitHub App name
  Value: livereview-dev-<yourname> (must be unique on GitHub; this becomes the
  slug)
  ────────────────────────────────────────
  Field: Homepage URL
  Value: https://manual-talent3.apps.hexmos.com
  ────────────────────────────────────────
  Field: Callback URL
  Value: https://manual-talent3.apps.hexmos.com/api/v1/auth/github-app/callback
  ────────────────────────────────────────
  Field: Expire user authorization tokens
  Value: ✅ on (default)
  ────────────────────────────────────────
  Field: Request user authorization (OAuth) during installation
  Value: ✅ on (this greys out Setup URL, which is expected)
  ────────────────────────────────────────
  Field: Enable Device Flow
  Value: off
  ────────────────────────────────────────
  Field: Webhook → Active
  Value: ✅
  ────────────────────────────────────────
  Field: Webhook URL
  Value: https://manual-talent3.apps.hexmos.com/api/v1/github-app/webhook
  ────────────────────────────────────────
  Field: Webhook secret
  Value: openssl rand -hex 32
  ────────────────────────────────────────
  Field: Where can this GitHub App be installed?
  Value: Any account

  Repository permissions: Pull requests Read & write, Contents Read-only, Issues
  Read & write, Metadata Read-only (required).
  Account permissions: Email addresses Read-only.
  Subscribe to events: Pull request, Pull request review comment, Issue comment.
  installation and installation_repositories events are sent automatically.

  Click Create GitHub App, then on the App's page:
  1. Copy the App ID and Client ID.
  2. Generate a new client secret. It's shown only once.
  3. Generate a private key and keep the .pem it downloads outside the repo.

  LiveReview .env

  GITHUB_APP_ID=<app id>
  GITHUB_APP_SLUG=livereview-dev-<yourname>
  GITHUB_APP_CLIENT_ID=<client id, Iv23li...>
  GITHUB_APP_CLIENT_SECRET=<client secret>
  GITHUB_APP_PRIVATE_KEY=/home/lovestaco/keys/livereview-dev.private-key.pem
  GITHUB_APP_WEBHOOK_SECRET=<the openssl value>

  Test URLs (once it's built)

  - Entry: https://manual-talent3.apps.hexmos.com/gh-login?src=local
  - With a PR: https://manual-talent3.apps.hexmos.com/gh-login?src=local&mr=http
    s://github.com/<owner>/<repo>/pull/<n>
  - Install page:
    https://github.com/apps/livereview-dev-<yourname>/installations/new
  - Webhook deliveries and redelivery: App settings → Advanced → Recent 
    Deliveries
  - Reset for a fresh-install test: uninstall at
    https://github.com/settings/installations and revoke at
    https://github.com/settings/apps/authorizations

  Check before testing: the niceurl3 tunnel must send both /api/... to the Go
  server and /gh-login to the UI, so the app can redirect /gh-login to
  /#/gh-login. Keep this dev App separate from the production one.

✻ Cogitated for 21s · done 3:43 pm

❯ keep both same later i'll just edit the app to livereview.hexmos.com thats it 

● That works. One App is fine, and you'll edit three URLs when you move to
  production. A few things to set up now so the switch really is just editing
  those URLs:

  Get these right at creation
  - Create it under the HexmosTech org, not your personal account:
    https://github.com/organizations/HexmosTech/settings/apps/new. Users see the
    owner on the install screen. Moving an App to another owner later is
    possible but annoying.
  - Use the final name now, e.g. LiveReview or LiveReview AI. The name becomes
    the slug (github.com/apps/<slug>) and the bot identity on PR comments
    (<name>[bot]). Renaming later changes the slug, which means updating
    GITHUB_APP_SLUG and any links that use it.
  - Leave it public ("Any account"). Testing doesn't need anything different.

  At go-live, change only these (App settings → General):

  ┌──────────┬───────────────────────────────────────────────────────────────┐
  │  Field   │                           Change to                           │
  ├──────────┼───────────────────────────────────────────────────────────────┤
  │ Homepage │ https://livereview.hexmos.com                                 │
  │  URL     │                                                               │
  ├──────────┼───────────────────────────────────────────────────────────────┤
  │ Callback │ https://livereview.hexmos.com/api/v1/auth/github-app/callback │
  │  URL     │                                                               │
  ├──────────┼───────────────────────────────────────────────────────────────┤
  │ Webhook  │ https://livereview.hexmos.com/api/v1/github-app/webhook       │
  │ URL      │                                                               │
  └──────────┴───────────────────────────────────────────────────────────────┘

  Copy the same six GITHUB_APP_* values into the prod .env. App ID, client
  ID/secret, private key and webhook secret all stay the same.

  After the switch
  - Your test installations keep existing on GitHub, but the prod DB has no
    connectors for them, because they were created in your local DB. Uninstall
    them before launch, or sign in on prod with "Already installed? Sign in".
    For that to work, the sign-in callback has to pick up the user's existing
    installations and create their connectors. That's a small addition to the
    plan's sign-in path.
  - Webhooks for your test repos go to prod as soon as the URL changes.
    Uninstall from repos you don't want prod to review.
  - You can't keep testing locally after go-live with the same App. If you need
    to later, create a second dev App.

✻ Crunched for 16s · done 3:45 pm

❯ okay in the plan thing some parts are wrong, the new login stuff are good.    
  but you're not creating jwt or email in the fw-parse in that db it should be  
  there, becuase next time the user signins in with google he needs the jwt     
  right? am i wrong?                                                            

● You're right that the user should exist in fw-parse. The reason is a little
  different from the JWT one, though. Let me check what Parse does on a later
  Google sign-in.

  Ran 1 shell command (ctrl+o to expand)

● You're half right. The login wouldn't break, but the user should still be
  created in fw-parse.

  Why login wouldn't break: on a later Google sign-in, Parse looks the user up
  by email, and if there's no match it creates the user right then
  (cloud/oneLogin/providerMethods.js:44-47). Then it issues a fresh JWT.
  LiveReview's ensure-cloud-user also matches on email, so it finds the account
  that /gh-login created. The user lands in the same account, and no earlier JWT
  is needed.

  Why the user should still be in Parse at /gh-login time:
  - Single source of truth: Parse _User is the Hexmos-wide user list (other
    Hexmos apps, user counts, profile and avatar, marketing). /gh-login users
    would be missing from it until they happened to use onelogin.
  - Two signup paths would drift: the plan has LiveReview provisioning users
    without the Hexmos JWT. That's a second signup path to keep in sync with the
    existing one, and it skips the check ensure-cloud-user makes that the JWT
    email matches the user.
  - Teammate invitations go through Parse's userInvitation function
    (internal/api/users/user_service.go:31). It's safer if the inviting user
    exists there.

  The fix: /gh-login gets a different front door but the same back door. The
  LiveReview callback, after verifying the GitHub App code and the installation,
  calls Parse externalProviderSignup server-side with a new provider type.
  Parse creates or finds the _User and returns the Hexmos JWT, and then the
  existing ensure-cloud-user path runs as it does today.

  This needs a small fw-parse change, because the current github branch in
  isAuthenticatedMiddleware.js:146 exchanges a code with the onelogin OAuth
  App's secret. Our GitHub App code would fail that exchange.
  - Add an externalServiceProvider == "github_app" branch that skips the
    exchange: LiveReview passes an already-exchanged GitHub App user token.
  - In providerMethods.js, treat github_app like github (getGithubUserData calls
    /user and /user/emails, which works because the App has the Email addresses
    permission).
  - Make it pick only a verified primary email. The current GitHub path skips
    this check.

  What it changes in the plan:
  - No separate provisioning code in LiveReview: drop my ProvisionCloudUser
    refactor and the gh_login_codes table.
  - After the callback, redirect to the existing Cloud.tsx ?data= handler, the
    same handoff onelogin uses, plus a next=#/get-started/github destination.
    Login completion reuses all the existing code.
  - One gap: Parse sets its .hexmos.com cookie on the response to the caller,
    which here is the LiveReview server, so the browser never gets it. /gh-login
    users won't get cross-app Hexmos sign-in on day one. That seems fine for
    launch.

  Should I update the plan doc with this?

✻ Cogitated for 42s · done 3:47 pm

❯ yeah we can do the fw-parse change or add api whatever it is but jwt should   
  be there so that we dont messup with different shit later.                    
  lovestaco [A\],  — 3:49 PM                                                    
  we dont need this gh-login for self hsoted right                              
  shrijith — 3:49 PM                                                            
  it can come                                                                   
  i mean later we will integrate it with providers                              
  but for speed                                                                 
  you can just do cloud ig                                                      
  lovestaco [A\],  — 3:49 PM                                                    
  yeah                                                                          
  shrijith — 3:49 PM                                                            
  just dont unnecessarily hardcode urls, etc i'd say                            
  as much as possible keep things flexible                                      
  in the plan itself                                                            
  if you say claudio will obey                                                  
  actuall you can ask it to make it compatible with self hosted with custom url 
  as well if possible                                                           
  so for slef hosted we wont need fw-parse it has some different mechanism for  
  the jwt thing update all these stuff in the plan                              

● Before updating the plan I need to see how self-hosted login and JWTs work,
  since self-hosted doesn't use fw-parse.

  Searched for 2 patterns, ran 2 shell commands (ctrl+o to expand)

● Self-hosted differs: it uses local email/password login via /auth/login
  against the users table and LiveReview's own tokenService.CreateTokenPair,
  with the instance URL sourced from instance_details.livereview_prod_url. I'll
  update the plan to reflect this.

  Searched for 1 pattern, ran 1 shell command (ctrl+o to expand)

● Write(docs/github-app-onboarding-plan.md)
  ⎿  Added 118 lines, removed 94 lines
        1 -# LiveReview Cloud: `/gh-login`, a 5-minute first review      
        1 +# LiveReview: `/gh-login`, a 5-minute first review via a GitHu
          +b App                                                         
        2  
        3  ## Context
        4  
        5 -Trials go cold because step 1 asks too much: create a classic 
          -PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          -the Product Hunt launch, the target is **GitHub login → LiveRe
          -view GitHub App (select repos) → ready**, one GitHub screen wi
          -th no PAT and no webhook setup. CodeRabbit works this way.    
        5 +Trials go cold because step 1 asks too much: create a classic 
          +PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          +the Product Hunt launch the target is **GitHub login → LiveRev
          +iew GitHub App (select repos) → ready**, one GitHub screen wit
          +h no PAT and no webhook setup. CodeRabbit works this way.     
        6  
        7 -The Hexmos onelogin gets in the way. Its GitHub button is an O
          -Auth App with `read:user,user:email` (`onelogin-node/src/share
          -d/src/components/Home.js:110`, exchanged in `fw-parse/isAuthen
          -ticatedMiddleware.js:146`). That gives us identity but no repo
          - access, so a repo-access step would always come *after* landi
          -ng in LiveReview.                                             
        7 +The Hexmos onelogin is in the way. Its GitHub button is an OAu
          +th App with `read:user,user:email` (`onelogin-node/src/shared/
          +src/components/Home.js:110`, exchanged in `fw-parse/isAuthenti
          +catedMiddleware.js:146`), so it gives us identity but no repo 
          +access.                                                       
        8  
        9 -**Decision (from the discussion with Shrijith):** don't touch 
          -the existing login. Add a separate demo path at **`livereview.
          -hexmos.com/gh-login`**:                                       
       10 -- `/gh-login` → GitHub App install + authorize (one screen) → 
          -signed in, repos connected → **demo onboarding** → first revie
          -w.                                                            
       11 -- `/gh-login` has an "Other ways to sign in" link that goes to
          - the standard Hexmos onelogin. That flow stays **exactly as it
          - is today**, with no demo onboarding.                         
        9 +**Decisions (from the discussion with Shrijith):**            
       10 +- Don't touch the existing login. Add a separate path at **`<i
          +nstance>/gh-login`** (cloud: `livereview.hexmos.com/gh-login`)
          +. It does GitHub App install and authorization in one screen, 
          +then signs the user in, then shows the **demo onboarding**, th
          +en the first review.                                          
       11 +- `/gh-login` links "Other ways to sign in" to the existing lo
          +gin, which stays **exactly as it is today** with no demo onboa
          +rding.                                                        
       12  - Landing-page "Start Free" CTAs point to `/gh-login`.
       13 -- Bringing the GitHub App into Git Providers (as LiveAPI did) 
          -comes later and isn't needed for launch.                      
       13 +- **Cloud users must exist in fw-parse with a Hexmos JWT**, cr
          +eated through the same Parse path as onelogin, so Hexmos-wide 
          +identity doesn't drift.                                       
       14 +- **Build for cloud first.** Keep everything config-driven (no
          + hard-coded hosts or URLs) so self-hosted, including custom do
          +mains and GitHub Enterprise, can turn it on later. Self-hosted
          + doesn't use fw-parse. It has its own local login and JWT, des
          +cribed below.                                                 
       15 +- Bringing the GitHub App into the Git Providers page (as Live
          +API did) comes later.                                         
       16  
       17 +## Configuration: nothing hard-coded                          
       18 +                                                              
       19 +| What | Source | Notes |                                     
       20 +|---|---|---|                                                 
       21 +| GitHub App credentials | `GITHUB_APP_ID`, `GITHUB_APP_SLUG`,
          + `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_CLIENT_SECRET`, `GITHUB_A
          +PP_PRIVATE_KEY` (path or PEM), `GITHUB_APP_WEBHOOK_SECRET` | S
          +erver-side only. If any are unset, the feature is off and `/gh
          +-login` redirects to the normal login. |                      
       22 +| GitHub host | `GITHUB_APP_BASE_URL` (default `https://github
          +.com`) | API URL comes from the existing `apiBaseURL()` (`inte
          +rnal/providers/github/base_url.go:8`; GHES → `/api/v3`). Insta
          +ll URL: `<base>/apps/<slug>/installations/new` on github.com, 
          +`<base>/github-apps/<slug>/installations/new` on GHES. |      
       23 +| Instance public URL | `instance_details.livereview_prod_url`
          + via `GetProductionURL` (`internal/api/production_url.go:23`) 
          +| Used only when we must show an absolute URL (e.g. in error p
          +ages). All redirects are **relative** (`/`, `/#/...`), so they
          + work on any host. The callback and webhook URLs live in the G
          +itHub App settings, not in code. |                            
       24 +| fw-parse (cloud only) | existing `FW_PARSE_BASE_URL` plus th
          +e same Parse headers as `network/email/invitation.go:72` | Not
          + used when `LIVEREVIEW_IS_CLOUD=false`. |                     
       25 +| Mode | existing `LIVEREVIEW_IS_CLOUD` (`internal/api/server.
          +go:100`) | Selects which identity finisher runs (cloud or self
          +-hosted). |                                                   
       26 +| UI | `GET /api/v1/ui-config` (`server.go:3047`) gains `githu
          +bAppEnabled`, `githubAppInstallPath` | The UI bundle has no Gi
          +tHub URLs or IDs. |                                           
       27 +| Landing page | one constant in hexmoshomepage (e.g. `LIVEREV
          +IEW_APP_URL` in `urls.js`) | No repeated inline `https://liver
          +eview.hexmos.com` strings. |                                  
       28 +                                                              
       29  ## User flow
       30  
       31  ```
       18 -hexmos.com/livereview  "Start Free" / paste PR URL            
       19 -        │  https://livereview.hexmos.com/gh-login?src=<cta>[&m
          -r=<pr-url>]                                                   
       20 -        ▼                                                     
       21 -#/gh-login     [ Continue with GitHub · FREE ]      "Other way
          -s to sign in" → existing Cloud.tsx login                      
       22 -        │  GET /api/v1/auth/github-app/start  (sets state, 302
          -)                                                             
       23 -        ▼                                                     
       24 -github.com/apps/<slug>/installations/new   ← user picks All / 
          -Only select repos → Install & Authorize                       
       25 -        │  302 ?code&installation_id&setup_action&state       
       26 -        ▼                                                     
       27 -GET /api/v1/auth/github-app/callback   (server-side: verify, p
          -rovision user+org+connector, one-time code)                   
       28 -        │  302 /#/gh-login/complete?code=<one-time>           
       29 -        ▼                                                     
       30 -#/gh-login/complete → POST /api/v1/auth/github-app/exchange → 
          -LoginResponse → handleLoginSuccess                            
       31 -        ▼                                                     
       32 -#/get-started/github   repos → open PRs → [Review this PR]  (p
          -refilled from ?mr=)  → review detail page                     
       32 +Landing "Start Free" / paste PR URL                           
       33 +   │  <instance>/gh-login?src=<cta>[&mr=<pr-url>]             
       34 +   ▼                                                          
       35 +#/gh-login   [ Continue with GitHub · FREE ]    "Other ways to
          + sign in" → existing login (Cloud.tsx / SelfHosted.tsx)       
       36 +   │  page stores the destination with the existing redirect-a
          +fter-login helper                                             
       37 +   │  (sessionStorage 'redirectAfterLogin' = '#/get-started/gi
          +thub?mr=…')                                                   
       38 +   │  GET /api/v1/auth/github-app/start?mode=install|signin  →
          + state cookie + 302                                           
       39 +   ▼                                                          
       40 +GitHub: install page (pick All / Only select repos → Install &
          + Authorize)   — or OAuth authorize for returning users        
       41 +   │  302 → /api/v1/auth/github-app/callback?code&installation
          +_id&setup_action&state                                        
       42 +   ▼                                                          
       43 +Callback (server): verify state → exchange code → verify insta
          +llation belongs to user                                       
       44 +   │                                                          
       45 +   ├─ CLOUD:       fw-parse externalProviderSignup(provider=gi
          +thub_app, userToken)                                          
       46 +   │               → Parse _User created/found + Hexmos JWT is
          +sued (same as onelogin)                                       
       47 +   │               → shared EnsureCloudUser core (validates JW
          +T with CLOUD_JWT_SECRET) → LR user + org                      
       48 +   │               → upsert App connector for that org        
       49 +   │               → 302 /?data=<Parse result>   ← the exact h
          +andoff onelogin uses; Cloud.tsx finishes login                
       50 +   │                                                          
       51 +   └─ SELF-HOSTED: find local user by verified email (no fw-pa
          +rse)                                                          
       52 +                   → upsert App connector for the user's org  
       53 +                   → one-time code → 302 /#/gh-login/complete?
          +code=…                                                        
       54 +                   → POST /api/v1/auth/github-app/exchange → t
          +okenService.CreateTokenPair → LoginResponse                   
       55 +   ▼                                                          
       56 +existing login completion → redirect-after-login → #/get-start
          +ed/github (repos → open PRs → Review this PR)                 
       57  ```
       58  
       35 -Returning users: `/gh-login` also shows a small "Already insta
          -lled? Sign in" link. It goes through the App's OAuth authorize
          - URL (`github.com/login/oauth/authorize?client_id=<app client 
          -id>`) and returns to the same callback with no `installation_i
          -d`. If that user turns out to have no installations, the callb
          -ack redirects them to the install URL.                        
       59 +Returning users get an "Already installed? Sign in" link (`mod
          +e=signin`), which uses the App's OAuth authorize URL. The call
          +back then gets no `installation_id`. It lists `GET /user/insta
          +llations` and upserts connectors for every installation the us
          +er can access and that isn't bound to another org. If there ar
          +e none, it redirects to the install URL.                      
       60  
       37 -## Phase 0: Create the GitHub App (one-time, by a human, in Gi
          -tHub settings)                                                
       38 -- Permissions: Pull requests **R/W**, Contents **R**, Metadata
          - **R**, Issues **R/W** (PR conversation comments use the issue
          -s API). Account permission: Email addresses **R**.            
       39 -- Events: `pull_request`, `pull_request_review_comment`, `issu
          -e_comment`, `installation`, `installation_repositories`.      
       40 -- **"Request user authorization (OAuth) during installation" o
          -n.** With it on, one install screen also yields a `code`, whic
          -h provides identity. The Callback URL is `https://livereview.h
          -exmos.com/api/v1/auth/github-app/callback`. GitHub disables th
          -e separate Setup URL in this mode, which is fine.             
       41 -- Webhook URL: `https://livereview.hexmos.com/api/v1/github-ap
          -p/webhook`, with a secret.                                    
       42 -- Env vars (server-side only, never in the UI bundle): `GITHUB
          -_APP_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_CLIENT_ID`, `GITHUB_A
          -PP_CLIENT_SECRET`, `GITHUB_APP_PRIVATE_KEY`, `GITHUB_APP_WEBHO
          -OK_SECRET`. If they're unset, `/gh-login` falls back to the no
          -rmal login page. That's the self-hosted case.                 
       61 +## Phase 0: GitHub App (one-time, by a human)                 
       62 +Create it under the **HexmosTech** org with the final name, si
          +nce the name is the slug and the `<name>[bot]` identity. For d
          +evelopment, point the URLs at `https://manual-talent3.apps.hex
          +mos.com` (`make niceurl3`). At go-live, edit the same App to `
          +https://livereview.hexmos.com`. The ID, keys and secrets stay 
          +the same.                                                     
       63 +- Callback URL: `<instance>/api/v1/auth/github-app/callback`. 
          +**Turn on "Request user authorization (OAuth) during installat
          +ion"**, so one screen does both install and login. Setup URL i
          +s disabled by that, which is expected.                        
       64 +- Webhook URL: `<instance>/api/v1/github-app/webhook`, with a 
          +secret.                                                       
       65 +- Repo permissions: Pull requests R/W, Contents R, Issues R/W,
          + Metadata R. Account permission: Email addresses R. Events: Pu
          +ll request, Pull request review comment, Issue comment. `insta
          +llation*` events are sent automatically.                      
       66 +- Installable by **any account**.                             
       67 +- Before launch, uninstall the dev test installations: they're
          + bound to the local DB, and their webhooks will go to prod onc
          +e the URLs change.                                            
       68  
       44 -## Phase 1: Backend                                           
       69 +## Phase 1: fw-parse (cloud identity), small change           
       70 +- `isAuthenticatedMiddleware.js:146`: add an `externalServiceP
          +rovider == "github_app"` branch. `accessToken` is an already-e
          +xchanged **GitHub App user token**, so skip the code exchange 
          +(which would fail, because the current branch uses the onelogi
          +n OAuth App's client secret).                                 
       71 +- `cloud/oneLogin/providerMethods.js:30`: handle `github_app` 
          +like `github` (`getGithubUserData`). Parse then creates or fin
          +ds `_User` by email and returns `getJWT(...)`, so the JWT row 
          +is stored in Parse exactly as it is for onelogin users.       
       72 +- `cloud/oneLogin/githubOneLogin.js`: pick the **primary and `
          +verified`** email from `/user/emails` (for both `github` and `
          +github_app`), and reject the login if there isn't one.        
       73 +- Leave the GitHub client secret out of this change (`cloud/co
          +nfig.js:119` is a hard-coded secret; moving it to env is separ
          +ate hygiene work).                                            
       74 +- A later Google or email sign-in through onelogin with the sa
          +me email then finds the same Parse `_User`, and `ensure-cloud-
          +user` finds the same LiveReview user. One identity everywhere.
       75  
       46 -### network/ (outbound HTTP only): new `network/providers/gith
          -ub/app.go`                                                    
       47 -- `MintAppJWT(appID, key)`: RS256, 9-minute expiry, using `git
          -hub.com/golang-jwt/jwt/v5` (already in `go.mod`).             
       48 -- `CreateInstallationToken(appJWT, installationID)` → `POST /a
          -pp/installations/{id}/access_tokens` (valid 1 hour).          
       49 -- `ExchangeUserCode(code)` → user-to-server token. `GetUser`, 
          -`GetUserEmails`, and `ListUserInstallations` (`GET /user/insta
          -llations`).                                                   
       50 -- `ListInstallationRepos(instToken)` → `GET /installation/repo
          -sitories`, and `ListOpenPulls(instToken, owner, repo)` for the
          - demo page.                                                   
       76 +## Phase 2: LiveReview backend                                
       77 +                                                              
       78 +### network/ (outbound HTTP only)                             
       79 +- `network/providers/github/app.go`: `MintAppJWT` (RS256, 9-mi
          +nute expiry, `golang-jwt/jwt/v5` is already in `go.mod`), `Cre
          +ateInstallationToken`, `ExchangeUserCode`, `GetUserEmails`, `L
          +istUserInstallations`, `ListInstallationRepos`, `ListOpenPulls
          +`. Every one takes the API base URL from config, with no liter
          +al `api.github.com`.                                          
       80 +- `network/hexmos/parse_signup.go` (cloud): `ExternalProviderS
          +ignup(ctx, provider, token, app)` → `POST {FW_PARSE_BASE_URL}/
          +functions/externalProviderSignup`, using the same header setup
          + as `network/email/invitation.go`.                            
       81  - Update `network/network_status.md`.
       82  
       83  ### storage/
       54 -- `storage/providers/github/token_store.go`:                  
       55 -  - `UpsertAppInstallationConnector(orgID, installationID, acc
          -ountLogin)`: an `integration_tokens` row with `provider='githu
          -b'`, `token_type='GITHUB_APP'`, `provider_app_id=<installation
          -_id>`, `provider_url='https://github.com'`, plus the cached `a
          -ccess_token` and `expires_at`. It's idempotent per installatio
          -n.                                                            
       56 -  - `GetGitHubTokenByConnector(orgID, connectorID)` and `GetCo
          -nnectorByInstallationID(id)`.                                 
       57 -  - `UpdateCachedInstallationToken`, and `MarkInstallationRemo
          -ved` for `installation.deleted/suspend`.                      
       58 -- Migrations (`db/migrations/`):                              
       59 -  - Partial unique index on `integration_tokens(provider_app_i
          -d) WHERE token_type='GITHUB_APP'`, so one installation belongs
          - to exactly one org.                                          
       60 -  - `orgs.signup_path text` (values `gh_app` or NULL). This is
          - what turns on the demo onboarding.                           
       61 -  - Table `gh_login_codes(code_hash, user_id, org_id, expires_
          -at, used_at)` for the one-time code handoff (60-second TTL, si
          -ngle use).                                                    
       62 -- `storage/users/`: move the find-or-create user + personal or
          -g + super_admin role logic out of `EnsureCloudUser` (`internal
          -/api/auth/handlers.go:636`, inline SQL today) into a `Provisio
          -nCloudUser(tx, email, first, last, source)` function. `EnsureC
          -loudUser` and the new callback both call it, so there's one pr
          -ovisioning path.                                              
       84 +- `storage/providers/github/token_store.go`: `UpsertAppInstall
          +ationConnector(orgID, installationID, accountLogin)` creates a
          +n `integration_tokens` row with `provider='github'`, `token_ty
          +pe='GITHUB_APP'`, `provider_app_id=<installation_id>`, `provid
          +er_url=<GITHUB_APP_BASE_URL>`, plus the cached token and `expi
          +res_at`. Also add `GetGitHubTokenByConnector(orgID, connectorI
          +D)`, `GetConnectorByInstallationID`, `UpdateCachedInstallation
          +Token`, `MarkInstallationRemoved`.                            
       85 +- Migrations: a partial unique index on `integration_tokens(pr
          +ovider_app_id) WHERE token_type='GITHUB_APP'` (one installatio
          +n belongs to exactly one org), and `orgs.signup_path text` (`g
          +h_app` or NULL). Self-hosted only: table `gh_login_codes(code_
          +hash, user_id, expires_at, used_at)` (60-second TTL, single us
          +e).                                                           
       86 +- Cloud: move the core of `EnsureCloudUser` (`internal/api/aut
          +h/handlers.go:636`: JWT validation with `CLOUD_JWT_SECRET`, th
          +e email-match check, find-or-create user + personal org + role
          +) into a function both the HTTP handler and the callback call,
          + with its SQL moved into `storage/users/`. **It's the same cod
          +e path as onelogin, not a copy.**                             
       87  - Update `storage/storage_status.md`.
       88  
       65 -### Endpoints: new `internal/api/auth/github_app_login.go` (pr
          -e-auth) + `internal/api/github_app.go`                        
       66 -- `GET /api/v1/auth/github-app/start?mode=install|signin&src=&
          -mr=` (public). Generates `state` = HMAC over `{nonce, src, mr,
          - exp:10m}`, sets it as an HttpOnly cookie too, and 302s to `in
          -stallations/new?state=` or to the OAuth authorize URL.        
       67 -- `GET /api/v1/auth/github-app/callback` (public). Steps:     
       68 -  1. Check that the `state` HMAC is valid, unexpired, and matc
          -hes the cookie.                                               
       69 -  2. Exchange `code` for a user token.                        
       70 -  3. From `GET /user/emails`, take the **primary and `verified
          -: true`** email and reject the login if there isn't one. Note 
          -that Parse's GitHub path doesn't check `verified`, and we shou
          -ldn't copy that.                                              
       71 -  4. If `installation_id` is present, it must appear in `GET /
          -user/installations`. Without this check anyone could claim som
          -eone else's installation.                                     
       72 -  5. `ProvisionCloudUser(..., source="gh-login:<src>")` and se
          -t `orgs.signup_path='gh_app'` on a new org.                   
       73 -  6. `UpsertAppInstallationConnector`. If the installation is 
          -already bound to a *different* org, show the error page "This 
          -GitHub installation is connected to another LiveReview account
          -".                                                            
       74 -  7. Sync repos into projects with manual trigger enabled (reu
          -se the `EnableManualTriggerForAllProjects` logic, `server.go:1
          -471`).                                                        
       75 -  8. Store a one-time code and 302 to `/#/gh-login/complete?co
          -de=…&mr=…`. The LiveReview JWT never goes in the URL.         
       76 -- `POST /api/v1/auth/github-app/exchange {code}` (public). Con
          -sumes the code and returns the same body as `EnsureCloudUser` 
          -(`user`, `tokens`, `organizations`), via `tokenService.CreateT
          -okenPair`. Also fires the existing signup Discord notification
          - with source `gh-login`.                                      
       77 -- `POST /api/v1/github-app/webhook` (public, verifies `X-Hub-S
          -ignature-256`). Uses `installation.id` → connector → org, then
          - hands off to the existing `WebhookOrchestratorV2Handler` path
          - with that connector, as `/github-hook/:connector_id` does. Al
          -so handles `installation.*` and `installation_repositories.add
          -ed/removed`.                                                  
       78 -- `GET /api/v1/github-app/demo` (full chain: `RequireAuth` → `
          -BuildOrgContextFromHeader` → `ValidateOrgAccess` → `BuildPermi
          -ssionContext`). Returns the org's App connector, its repos, an
          -d the open PRs per repo (capped, e.g. 5 per repo, 10 repos).  
       89 +### Endpoints                                                 
       90 +- `GET /api/v1/auth/github-app/start` (public): an HMAC state 
          +over `{nonce, src, mode, exp:10m}` (secret: `JWT_SECRET`), mir
          +rored in an HttpOnly cookie, then a 302 to the install or auth
          +orize URL built from config.                                  
       91 +- `GET /api/v1/auth/github-app/callback` (public):            
       92 +  1. The state must be valid and match the cookie.            
       93 +  2. Exchange the code for a user token.                      
       94 +  3. `installation_id` must appear in `/user/installations`. W
          +ithout this, anyone could claim someone else's installation.  
       95 +  4. **Identity finisher**, chosen by mode:                   
       96 +     - **Cloud:** `ExternalProviderSignup("github_app", userTo
          +ken, "livereview")` → Parse result with the JWT → shared Ensur
          +eCloudUser core → org. Set `orgs.signup_path='gh_app'` on a ne
          +wly created org. Fire the signup Discord notification with sou
          +rce `gh-login:<src>`.                                         
       97 +     - **Self-hosted:** find the local user by verified email.
          + By default only existing users can sign in (admins create use
          +rs today). With no admin set up yet, redirect to the existing 
          +setup screen. A `GITHUB_APP_ALLOW_SIGNUP` flag for self-hosted
          + can come later.                                              
       98 +  5. `UpsertAppInstallationConnector`. If the installation is 
          +bound to another org, show an error page: "connected to anothe
          +r LiveReview account".                                        
       99 +  6. Sync repos into projects with manual trigger enabled (reu
          +se the `EnableManualTriggerForAllProjects` logic, `server.go:1
          +471`).                                                        
      100 +  7. Redirect. Cloud: `/?data=<encoded Parse result>`, the sam
          +e handoff `onelogin-node` `redirectBackToOrigin` uses, so `ui/
          +src/pages/Auth/Cloud.tsx` finishes the login unchanged. Self-h
          +osted: `/#/gh-login/complete?code=<one-time>`.                
      101 +- `POST /api/v1/auth/github-app/exchange` (public, self-hosted
          + only): consumes the one-time code → `tokenService.CreateToken
          +Pair` → the same `LoginResponse` shape as `/auth/login`.      
      102 +- `POST /api/v1/github-app/webhook` (public, verifies `X-Hub-S
          +ignature-256`): `installation.id` → connector → org, then the 
          +existing `WebhookOrchestratorV2Handler` path, as `/github-hook
          +/:connector_id` does. It also handles `installation.*` and `in
          +stallation_repositories.*`.                                   
      103 +- `GET /api/v1/github-app/demo` (full chain: `RequireAuth` → `
          +BuildOrgContextFromHeader` → `ValidateOrgAccess` → `BuildPermi
          +ssionContext`): the org's App connector, repos, and open PRs (
          +capped).                                                      
      104 +- `getUIConfig` adds `githubAppEnabled`.                      
      105  
       80 -### Token resolver + cross-tenant fix (required, not optional)
       81 -- New `internal/provider_input/github/token_resolver.go` `Reso
          -lveGitHubAccessToken(ctx, rec)`. `PAT` → `pat_token`, unchange
          -d. `GITHUB_APP` → the cached token if it's valid for more than
          - 5 more minutes, otherwise mint a new one and cache it.       
       82 -- **Existing bug:** `storage/providers/github/token_store.go:2
          -1` `GetLatestGitHubToken()` picks the newest GitHub token **ac
          -ross all orgs**. Every path in `internal/provider_input/github
          -/github_provider.go` (lines 145, 198, 258, 272, 286, 307, 409,
          - 440) and `lrc_fetch.go:28` uses it. The launch will create ma
          -ny org GitHub connectors, so this *will* mix tenants. Fix: `Gi
          -tHubV2Provider` takes the connector from the webhook route/orc
          -hestrator and resolves through `GetGitHubTokenByConnector`. De
          -lete `GetLatestGitHubToken`.                                  
       83 -- Callers that read `token.PatToken` switch to the resolver: `
          -github_provider.go`, `lrc_fetch.go:33`, `internal/api/reviews_
          -api.go` `ensureValidToken`/`checkTokenNeedsRefresh` (lines 187
          - and 262; GitHub App tokens *do* expire), and `auto_webhook_in
          -staller.go:125` (App connectors skip per-repo webhook install)
          -.                                                             
      106 +### Token resolver + cross-tenant fix (required)              
      107 +- `internal/provider_input/github/token_resolver.go` `ResolveG
          +itHubAccessToken(ctx, rec)`: `PAT` returns `pat_token`, unchan
          +ged. `GITHUB_APP` returns the cached token if it's valid for m
          +ore than 5 more minutes, otherwise mints a new one.           
      108 +- **Existing bug:** `storage/providers/github/token_store.go:2
          +1` `GetLatestGitHubToken()` picks the newest GitHub token **ac
          +ross all orgs**. It's used by `internal/provider_input/github/
          +github_provider.go` (lines 145, 198, 258, 272, 286, 307, 409, 
          +440) and `lrc_fetch.go:28`. With many App orgs this *will* mix
          + tenants. Fix: `GitHubV2Provider` resolves the connector from 
          +the webhook route or orchestrator through `GetGitHubTokenByCon
          +nector`. Delete `GetLatestGitHubToken`.                       
      109 +- Switch `PatToken` readers to the resolver: `github_provider.
          +go`, `lrc_fetch.go:33`, `internal/api/reviews_api.go` `ensureV
          +alidToken`/`checkTokenNeedsRefresh` (lines 187 and 262; App to
          +kens expire), and `auto_webhook_installer.go:125` (App connect
          +ors skip per-repo webhook install).                           
      110 +- Overlap rule: if an org has both a PAT connector and the App
          + on the same repo, the App connector wins and PAT-webhook even
          +ts for that repo are ignored, so there are no double reviews. 
      111  
       85 -## Phase 2: LiveReview UI                                     
       86 -- **`/gh-login` entry:** `cmd/ui.go:374` already serves `index
          -.html` for unknown paths. In the `App.tsx` bootstrap, if `loca
          -tion.pathname === '/gh-login'`, `replace` to `/#/gh-login` and
          - keep the query string.                                       
       87 -- **`#/gh-login` page** (`ui/src/pages/Auth/GhLogin.tsx`): Liv
          -eReview logo, "Review your first PR in 2 minutes · FREE", one 
          -large **Continue with GitHub** button (→ `/api/v1/auth/github-
          -app/start?mode=install&src&mr`), a small "Already installed? S
          -ign in" link (`mode=signin`), and an **"Other ways to sign in"
          -** link that runs the existing `handleLoginClick` from `ui/src
          -/pages/Auth/Cloud.tsx`. If the user is already logged in, it g
          -oes straight to `#/get-started/github`.                       
       88 -- **`#/gh-login/complete`:** POSTs the code to `/exchange`, th
          -en calls the existing `handleLoginSuccess` and the `organizati
          -ons/setOrganizationsFromAuth` dispatch, the same as `Cloud.tsx
          -`. Then it navigates to `#/get-started/github` (keeping `mr`).
       89 -- **`#/get-started/github` (demo onboarding)**, shown only whe
          -n the org has `signup_path='gh_app'`:                         
       90 -  1. **Repos connected ✓** lists the repos, with "Add more rep
          -os" linking to `https://github.com/settings/installations/<id>
          -`.                                                            
       91 -  2. **AI ✓**, or a one-field step. See open decision 1.      
       92 -  3. **Pick a PR**: open PRs per repo, plus a "paste a PR URL"
          - box prefilled from `?mr=`. **Review this PR** calls the exist
          -ing `POST /api/v1/connectors/trigger-review`, then navigates t
          -o the review detail page for live progress. Comments appear on
          - the PR as `LiveReview[bot]`.                                 
       93 -  4. A small "Next: install the CLI for pre-commit reviews" li
          -nk to the existing `OnboardingSteps`.                         
       94 -- The dashboard for `gh_app` orgs shows a "Continue setup" ban
          -ner pointing to `#/get-started/github` until the first review 
          -runs. Non-`gh_app` orgs see exactly what they see today.      
       95 -- Repo rules: add `#/get-started/github` to `megaMenuData.ts` 
          -(visible only for `gh_app` orgs), and add `internal/docindex/d
          -ocs/routes_guide/auth/gh-login.md` and `.../get-started-github
          -.md`.                                                         
      112 +## Phase 3: LiveReview UI                                     
      113 +- `/gh-login` entry: `cmd/ui.go:374` already serves `index.htm
          +l` for unknown paths. In the `App.tsx` bootstrap, `pathname ==
          += '/gh-login'` → `replace('/#/gh-login' + search)`.           
      114 +- `ui/src/pages/Auth/GhLogin.tsx` (`#/gh-login`): "Review your
          + first PR in 2 minutes · FREE", a big **Continue with GitHub**
          + button, an "Already installed? Sign in" link, and "Other ways
          + to sign in", which uses the existing `Cloud.tsx` or `SelfHost
          +ed.tsx` login depending on `isCloud`. Before leaving the page 
          +it stores `#/get-started/github?mr=…` with the existing redire
          +ct-after-login helper (`ui/src/utils/authHelpers.ts`, sessionS
          +torage, which survives the same-tab GitHub round trip). It's h
          +idden, and redirects to normal login, when `githubAppEnabled` 
          +is false. If the user is already logged in, it goes straight t
          +o `#/get-started/github`.                                     
      115 +- `#/gh-login/complete` (self-hosted): exchanges the code, the
          +n calls `handleLoginSuccess`, then follows redirect-after-logi
          +n.                                                            
      116 +- `#/get-started/github` (only for `signup_path='gh_app'` orgs
          +):                                                            
      117 +  1. Repos connected ✓, with "Add more repos" linking to the i
          +nstallation settings URL returned by the API.                 
      118 +  2. AI (see open decision 1).                                
      119 +  3. Open PRs plus a "paste a PR URL" box prefilled from `mr`.
          + **Review this PR** calls the existing `POST /api/v1/connector
          +s/trigger-review`, then opens the review detail page.         
      120 +  4. A link: "Next: install the CLI".                         
      121 +- The dashboard shows a "Continue setup" banner for `gh_app` o
          +rgs until the first review runs. Other orgs see what they see 
          +today.                                                        
      122 +- Repo rules: add a `megaMenuData.ts` entry for `#/get-started
          +/github` (visible only for `gh_app` orgs), and add `internal/d
          +ocindex/docs/routes_guide/auth/gh-login.md` and `.../get-start
          +ed-github.md`.                                                
      123  
       97 -## Phase 3: hexmoshomepage CTAs                               
       98 -Point "Start Free" to `https://livereview.hexmos.com/gh-login?
          -src=<where>`:                                                 
       99 -- `pages/livereview/index.tsx` (`handleBuyNow` line 529, Start
          - Free at 562, 987, 2467)                                      
      100 -- `components/livereview/StartFreeModal.tsx:146` (Try Cloud; t
          -he self-hosted option stays)                                  
      101 -- `components/livereview/PricingSection.tsx:181` (free plan on
          -ly; paid checkout unchanged)                                  
      102 -- `pages/livereview/blog/index.tsx:153`, `pages/livereview/blo
          -g/[slug].tsx:174`, `pages/livereview/why-livereview.tsx:222,38
          -4`                                                            
      103 -- Optional hero input "Paste a GitHub PR URL → Review it free"
          -, modeled on LiveAPI's `RepoInput` (`pages/liveapi/index.tsx:1
          -29`). It sends `/gh-login?mr=<encoded url>&src=hero`.         
      124 +## Phase 4: hexmoshomepage                                    
      125 +- Add one constant for the app URL, and build `…/gh-login?src=
          +<where>` from it.                                             
      126 +- Point these at it: `pages/livereview/index.tsx` (`handleBuyN
          +ow` line 529, Start Free at 562, 987, 2467), `components/liver
          +eview/StartFreeModal.tsx:146` (Try Cloud; self-hosted unchange
          +d), `components/livereview/PricingSection.tsx:181` (free plan 
          +only), `pages/livereview/blog/index.tsx:153`, `pages/liverevie
          +w/blog/[slug].tsx:174`, `pages/livereview/why-livereview.tsx:2
          +22,384`.                                                      
      127 +- Optional: a hero input "Paste a GitHub PR URL → review it fr
          +ee" (modeled on LiveAPI's `RepoInput`, `pages/liveapi/index.ts
          +x:129`) → `/gh-login?mr=<encoded>&src=hero`.                  
      128  
      129  ## What does NOT change
      106 -- **onelogin-node and fw-parse:** no changes. The standard log
          -in keeps its current providers and order.                     
      107 -- **Git Providers page and the PAT flow:** unchanged for every
          -one, including GitLab, Bitbucket, Gitea and Azure connectors. 
          -Existing PAT connectors keep `/github-hook/:connector_id`. The
          - only effect on them is the cross-tenant fix, which is a bug f
          -ix.                                                           
      108 -- **Self-hosted:** no `GITHUB_APP_*` env vars, so `/gh-login` 
          -redirects to the normal login.                                
      109 -- If a `gh_app` org also adds a PAT connector covering the sam
          -e repo, the App connector wins and PAT-webhook events for that
          - repo are ignored (no double reviews).                        
      110 -- **gh-login users have no Hexmos/Parse user.** That's fine, b
          -ecause LiveReview keys users by email (`EnsureCloudUser`). If 
          -they later sign in through onelogin with the same email, they 
          -land in the same account.                                     
      130 +- **onelogin-node:** no change, same providers and order.     
      131 +- **Git Providers page and the PAT flow:** unchanged for all p
          +roviders. PAT connectors keep `/github-hook/:connector_id`. Th
          +ey're affected only by the cross-tenant bug fix.              
      132 +- **Self-hosted without `GITHUB_APP_*` set:** the feature is i
          +nvisible.                                                     
      133  
      112 -## Open decisions (need Shrijith's call before coding)        
      113 -1. **AI for the demo.** Options: (a) auto-provision the manage
          -d "LiveReview AI Model" connector for `gh_app` orgs, using the
          - same insert as `ConfirmPurchase` in `internal/license/payment
          -/subscription_service.go:1603`, with a small LOC cap. This is 
          -the true zero-step option. (b) An inline "paste Gemini key" fi
          -eld with a link to AI Studio. Today `OnboardingSteps.tsx:~93` 
          -sends free-plan users to *upgrade* when they click Configure A
          -I, so "FREE" needs one of these.                              
      114 -2. **Demo limits.** Shrijith suggested "7 days, maybe 1 repo".
          - Do we enforce a repo cap (e.g. review only the first N repos 
          -selected) and a trial window, or just rely on the LOC quota?  
      115 -3. **Auto-review new PRs.** Off by default (manual trigger onl
          -y, with an opt-in toggle on the demo page), or on for selected
          - repos?                                                       
      134 +## Open decisions (need Shrijith's call)                      
      135 +1. **AI for the demo.** Option (a): auto-provision the managed
          + "LiveReview AI Model" for `gh_app` orgs, using the same inser
          +t as `ConfirmPurchase` (`internal/license/payment/subscription
          +_service.go:1603`), with a small LOC cap. Option (b): an inlin
          +e "paste Gemini key" field. Today `OnboardingSteps.tsx:~93` se
          +nds free-plan users to *upgrade* when they click Configure AI,
          + so "FREE" needs one of these.                                
      136 +2. **Demo limits:** 7 days and/or a 1-repo cap, or just the LO
          +C quota?                                                      
      137 +3. **Auto-review new PRs:** off by default (manual trigger plu
          +s a toggle), or on for the selected repos?                    
      138  
      139  ## Verification
      118 -1. Register a dev App (`livereview-dev`) pointed at a cloudfla
          -red/ngrok URL, set the `GITHUB_APP_*` vars, and run `make deve
          -lop`.                                                         
      119 -2. Go tests: `go test ./network/providers/github/ -run App` (J
          -WT claims, token parsing against `httptest`). `go test ./inter
          -nal/api/... -run GitHubApp`: bad or expired state is rejected,
          - an unverified email is rejected, an installation missing from
          - `/user/installations` is rejected, an installation bound to a
          -nother org is rejected, a one-time code works once only, and a
          - bad webhook signature returns 401.                           
      120 -3. Tenant isolation: two orgs (one App, one PAT, plus a PAT/PA
          -T pair). A webhook for org A resolves only org A's connector. 
          -Grep confirms `GetLatestGitHubToken` has no callers left.     
      121 -4. Manual E2E in a private window: landing "Start Free" → `/gh
          --login` → GitHub install picking one repo → land on `#/get-sta
          -rted/github` → Review a PR → bot comments on the PR. Repeat wi
          -th `?mr=<url>`. Also: "Other ways to sign in" behaves exactly 
          -as today, uninstalling the App marks the connector removed, an
          -d `installation_repositories.removed` updates the repo list.  
      122 -5. `make check-status-doc`, and `npx tsc --noEmit` in `ui/`. N
          -o full UI build without asking first.                         
      140 +1. Dev App on `https://manual-talent3.apps.hexmos.com` (`make 
          +niceurl3`), the `GITHUB_APP_*` vars in `.env`, `LIVEREVIEW_IS_
          +CLOUD=true`, `FW_PARSE_BASE_URL` pointing at a dev fw-parse ru
          +nning the Phase 1 change. Run `make develop`.                 
      141 +2. fw-parse: a jest test for the `github_app` branch (no code 
          +exchange, rejects an unverified email, creates `_User` and ret
          +urns a JWT).                                                  
      142 +3. Go tests: `go test ./network/providers/github/ -run App` (a
          +gainst `httptest`). `go test ./internal/api/... -run GitHubApp
          +`: a bad state is rejected, an installation not in `/user/inst
          +allations` is rejected, an installation bound to another org i
          +s rejected, a bad webhook signature returns 401, and the self-
          +hosted one-time code works once only.                         
      143 +4. Tenant isolation: two orgs (App + PAT, and PAT + PAT). A we
          +bhook for org A resolves only org A's connector. Grep shows `G
          +etLatestGitHubToken` has no callers left.                     
      144 +5. Manual E2E (private window): landing "Start Free" → `/gh-lo
          +gin` → install on one repo → land on `#/get-started/github` → 
          +Review a PR → bot comment on the PR. Check that the Parse `_Us
          +er` row and JWT exist. Then log out and sign in with **Google 
          +via onelogin** using the same email: it must land in the **sam
          +e** LiveReview account and org, with the connector still there
          +. Also check: "Other ways to sign in" behaves as today, `?mr=`
          + is prefilled, and uninstalling or removing repos updates the 
          +connector.                                                    
      145 +6. Config check: with `GITHUB_APP_*` unset, `/gh-login` redire
          +cts to normal login, and there are no GitHub URLs in the UI bu
          +ndle (grep `ui/dist` is only possible after a build, which nee
          +ds approval, so grep `ui/src` instead).                       
      146 +7. `make check-status-doc`, and `npx tsc --noEmit` in `ui/`. N
          +o full UI build without asking first.                         

  Ran 1 shell command (ctrl+o to expand)

● I've updated LiveReview/docs/github-app-onboarding-plan.md. No code has
  changed.

  Cloud: the user now gets a Hexmos account and JWT in fw-parse
  - After GitHub confirms the login, the LiveReview server calls fw-parse's
    externalProviderSignup with a new github_app provider type. Parse finds or
    creates the _User and issues the JWT, the same way it does for onelogin
    users.
  - Then the same ensure-cloud-user logic runs, shared with the existing login
    rather than copied.
  - After that, the browser is sent back with /?data=…, the same handoff
    onelogin uses, so Cloud.tsx finishes the login without changes.
  - So if that user signs in with Google later, they land in the same Parse user
    and the same LiveReview account.

  The fw-parse change (Phase 1)
  - A github_app branch that skips the code exchange, since LiveReview passes a
    token it already exchanged.
  - providerMethods.js treats github_app like github.
  - Only a primary, verified email is accepted. The current GitHub path doesn't
    check this.

  Self-hosted: no fw-parse
  - It matches the local user by verified email and issues LiveReview's own
    tokens (tokenService.CreateTokenPair), handed to the browser through a
    single-use 60-second code.
  - By default only existing users can sign in this way, because admins create
    users on self-hosted today.
  - Self-hosted is compatible by design but is not the launch target. Cloud
    comes first, as Shrijith said.

  Nothing hard-coded (new config table in the plan)
  - GitHub App credentials and the GitHub host come from env vars, with GitHub
    Enterprise handled through the existing apiBaseURL().
  - The instance URL comes from livereview_prod_url, and redirects are relative
    paths, so they work on any domain.
  - fw-parse uses the existing FW_PARSE_BASE_URL.
  - The UI learns whether the feature is on from /ui-config, so the UI code
    contains no GitHub URLs.
  - The landing page builds the link from one constant.
  - If the GitHub App env vars are unset, /gh-login sends users to the normal
    login.

  The plan also now covers developing on manual-talent3.apps.hexmos.com and then
  editing the same App's URLs at go-live. A new test step checks that signing
  up through /gh-login and later through Google lands in the same account. The
  three open decisions for Shrijith are still at the bottom: AI for the demo,
  demo limits, and auto-review on new PRs.

✻ Cogitated for 2m 37s · done 3:54 pm

※ recap: We're planning a /gh-login path so new users get GitHub sign-in, repo selection and their first PR review in about five minutes for the Product Hunt launch. The plan is saved in docs/github-app-onboarding-plan.md, and next Shrijith needs to settle the three open decisions before coding.
  ⎿  Referenced file docs/github-app-onboarding-plan.md
  ⎿  Referenced file CLAUDE.md
  ⎿  Read docs/Local-Setup-Guide.md (195 lines)
  ⎿  Read ../../fw-parse/isAuthenticatedMiddleware.js (237 lines)
  ⎿  Referenced file ../../liveapi/liveapi-backend/codetodoc/codetodoc.go
  ⎿  Plan file referenced (~/.claude/plans/gleaming-munching-giraffe.md)

● Compacted while idle, before the prompt cache expired · turn off with 
  "idleCompaction": false in settings.json
❯ aight make the plan bit more detailed so that sonnet model can work on it     

  Read 1 file, listed 2 directories, ran 7 shell commands (ctrl+o to expand)

● I'm reading the actual code (handlers, storage, routes, UI) so the plan can
  give exact signatures and line numbers. Still no code changes.

  Searched for 3 patterns, listed 2 directories, ran 5 shell commands (ctrl+o to expand)

● Still reading code: I'm confirming how webhooks and manual triggers pick a
  GitHub token, so the plan can name the exact functions to change.

  Searched for 2 patterns, ran 6 shell commands (ctrl+o to expand)

● I've spotted a few non-obvious traps worth flagging, like installation tokens
  failing on GET /user and the connector ID being set too late in the webhook
  flow. Checking the remaining spots now.

  Searched for 2 patterns, listed 1 directory, ran 13 shell commands (ctrl+o to expand)

● I have what I need from the code. Two last checks, the trigger-review request
  shape and the UI caller, then I'll write the detailed plan.

  Searched for 2 patterns, ran 4 shell commands (ctrl+o to expand)

● Research is done. I'm now writing the detailed, task-by-task version of the
  plan in the same file.

● Write(docs/github-app-onboarding-plan.md)
  ⎿  Added 806 lines, removed 113 lines
        1  # LiveReview: `/gh-login`, a 5-minute first review via a GitHu
           b App
        2  
        3 -## Context                                                    
        3 +> **Implementation plan. Written so an engineer or a model (e.
          +g. Sonnet) can work through it task by task.**                
        4 +> Every task lists the files, what to change, a code sketch, a
          +nd how to know it's done.                                     
        5 +> Line numbers were checked against the code on 2026-10-19. If
          + one has drifted, find the symbol by name; don't guess.       
        6  
        5 -Trials go cold because step 1 asks too much: create a classic 
          -PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          -the Product Hunt launch the target is **GitHub login → LiveRev
          -iew GitHub App (select repos) → ready**, one GitHub screen wit
          -h no PAT and no webhook setup. CodeRabbit works this way.     
        7 +---                                                           
        8  
        7 -The Hexmos onelogin is in the way. Its GitHub button is an OAu
          -th App with `read:user,user:email` (`onelogin-node/src/shared/
          -src/components/Home.js:110`, exchanged in `fw-parse/isAuthenti
          -catedMiddleware.js:146`), so it gives us identity but no repo 
          -access.                                                       
        9 +## 0. Rules for whoever implements this                       
       10  
       11 +1. **Work in the PR order in §9.** Each PR builds and passes t
          +ests on its own. Don't start a PR before the previous one is m
          +erged or at least green.                                      
       12 +2. **Storage/network boundary (repo rule):**                  
       13 +   - SQL goes only in `storage/…`.                            
       14 +   - Outbound HTTP goes only in `network/…`.                  
       15 +   - Every function you add or move there gets a row in `stora
          +ge/storage_status.md` / `network/network_status.md`.          
       16 +   - Run `make check-status-doc` before calling a task done.  
       17 +3. **No fallback behavior** (repo rule). When the connector, s
          +tate or installation can't be resolved, return an error. Never
          + fall back to "latest token", "any org" or "skip the check".  
       18 +4. **Don't touch** `onelogin-node`, `ui/src/pages/Auth/Cloud.t
          +sx` (behavior), `SelfHosted.tsx`, or the Git Providers PAT flo
          +w, except where a task explicitly says so.                    
       19 +5. **UI:**                                                    
       20 +   - Read `.agents/design.md` before styling anything.        
       21 +   - Type-check with `cd ui && npx tsc --noEmit`.             
       22 +   - **Never run `npm run build` / `make build-with-ui` withou
          +t asking the human first.**                                   
       23 +   - Every new page gets a `megaMenuData.ts` entry and a route
          +s_guide doc.                                                  
       24 +6. **Tests:** run the narrowest test you can (`go test -count=
          +1 ./path/ -run Name`). Don't run `make test` for a targeted ch
          +ange.                                                         
       25 +7. **Secrets** (App private key, client secret, webhook secret
          +, user tokens):                                               
       26 +   - Never log them.                                          
       27 +   - Never send them to the UI.                               
       28 +   - Never put them in a URL. The one exception is the Hexmos 
          +JWT in `/?data=`, which is the existing onelogin contract.    
       29 +8. Comments and naming follow the surrounding code. No new fra
          +meworks.                                                      
       30 +                                                              
       31 +---                                                           
       32 +                                                              
       33 +## 1. Context and decisions                                   
       34 +                                                              
       35 +Trials go cold because step 1 asks too much: create a classic 
          +PAT (`repo, read:org`), paste an AI key, install the CLI. For 
          +the Product Hunt launch the target is **GitHub login → install
          + the LiveReview GitHub App on selected repos → first review**,
          + on one GitHub screen with no PAT and no webhook setup. CodeRa
          +bbit works this way.                                          
       36 +                                                              
       37 +The Hexmos onelogin GitHub button is an OAuth App with `read:u
          +ser,user:email` (`onelogin-node/src/shared/src/components/Home
          +.js:110`). It gives identity but no repo access, so it can't d
          +o this.                                                       
       38 +                                                              
       39  **Decisions (from the discussion with Shrijith):**
       10 -- Don't touch the existing login. Add a separate path at **`<i
          -nstance>/gh-login`** (cloud: `livereview.hexmos.com/gh-login`)
          -. It does GitHub App install and authorization in one screen, 
          -then signs the user in, then shows the **demo onboarding**, th
          -en the first review.                                          
       11 -- `/gh-login` links "Other ways to sign in" to the existing lo
          -gin, which stays **exactly as it is today** with no demo onboa
          -rding.                                                        
       40 +- Add a separate path, **`<instance>/gh-login`**.             
       41 +  - It does GitHub App install and authorization in one GitHub
          + screen, then signs the user in, then shows the demo onboardin
          +g (`#/get-started/github`), then the first review.            
       42 +  - "Other ways to sign in" on that page goes to the existing 
          +login, which stays exactly as it is.                          
       43  - Landing-page "Start Free" CTAs point to `/gh-login`.
       13 -- **Cloud users must exist in fw-parse with a Hexmos JWT**, cr
          -eated through the same Parse path as onelogin, so Hexmos-wide 
          -identity doesn't drift.                                       
       14 -- **Build for cloud first.** Keep everything config-driven (no
          - hard-coded hosts or URLs) so self-hosted, including custom do
          -mains and GitHub Enterprise, can turn it on later. Self-hosted
          - doesn't use fw-parse. It has its own local login and JWT, des
          -cribed below.                                                 
       15 -- Bringing the GitHub App into the Git Providers page (as Live
          -API did) comes later.                                         
       44 +- **Cloud users must exist in fw-parse with a Hexmos JWT.** Th
          +ey are created through the same Parse function onelogin uses (
          +`externalProviderSignup`), so a later Google or email sign-in 
          +lands in the same account.                                    
       45 +- **Cloud first.** Everything is config-driven (no hard-coded 
          +hosts) so self-hosted, including custom domains and GHES, can 
          +turn it on later. Self-hosted doesn't use fw-parse (§8, design
          + only, not in launch scope).                                  
       46 +- Bringing the GitHub App into the Git Providers page comes la
          +ter. For now an App connector only has to *not break* the exis
          +ting screens (T3.8).                                          
       47  
       17 -## Configuration: nothing hard-coded                          
       48 +---                                                           
       49  
       50 +## 2. Glossary (tokens and IDs; mixing these up is the #1 bug 
          +source)                                                       
       51 +                                                              
       52 +| Name | Looks like | Lifetime | Used for | Where it lives |  
       53 +|---|---|---|---|---|                                         
       54 +| **App JWT** | RS256 JWT, `iss=<GITHUB_APP_ID>` | ≤10 min (we
          + use 9) | Calling `/app/...` endpoints (mint installation toke
          +ns, get app info) | Never stored; minted on demand |          
       55 +| **Installation token** | `ghs_…` | 1 hour | Everything repo-
          +related: read PRs, post comments and reviews, list installatio
          +n repos | Cached in `integration_tokens.pat_token` + `expires_
          +at` |                                                         
       56 +| **User token** | `ghu_…` (+ `ghr_…` refresh) | 8 h | Only du
          +ring the callback: `/user/emails`, `/user/installations`, and 
          +handing to fw-parse | **Never stored** |                      
       57 +| **installation_id** | integer, e.g. `51234567` | until unins
          +talled | Identifies one install on one account (user or org) |
          + `integration_tokens.provider_app_id` (text) |                
       58 +| **Hexmos JWT** | HS256 JWT from fw-parse | 30 days | Hexmos 
          +identity; `ensure-cloud-user` validates it with `CLOUD_JWT_SEC
          +RET` | Parse `JWT` class; passed to the UI in `/?data=` |     
       59 +| **LiveReview tokens** | access + refresh pair | existing | L
          +iveReview session | Existing `auth_tokens` table (unchanged) |
       60 +                                                              
       61 +`GET /user` **does not work** with an installation token (it r
          +eturns 403 "Resource not accessible by integration"). Any code
          + that calls `/user` to find "the bot" must special-case App co
          +nnectors (T3.5).                                              
       62 +                                                              
       63 +---                                                           
       64 +                                                              
       65 +## 3. Configuration (nothing hard-coded)                      
       66 +                                                              
       67  | What | Source | Notes |
       68  |---|---|---|
       21 -| GitHub App credentials | `GITHUB_APP_ID`, `GITHUB_APP_SLUG`,
          - `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_CLIENT_SECRET`, `GITHUB_A
          -PP_PRIVATE_KEY` (path or PEM), `GITHUB_APP_WEBHOOK_SECRET` | S
          -erver-side only. If any are unset, the feature is off and `/gh
          --login` redirects to the normal login. |                      
       22 -| GitHub host | `GITHUB_APP_BASE_URL` (default `https://github
          -.com`) | API URL comes from the existing `apiBaseURL()` (`inte
          -rnal/providers/github/base_url.go:8`; GHES → `/api/v3`). Insta
          -ll URL: `<base>/apps/<slug>/installations/new` on github.com, 
          -`<base>/github-apps/<slug>/installations/new` on GHES. |      
       23 -| Instance public URL | `instance_details.livereview_prod_url`
          - via `GetProductionURL` (`internal/api/production_url.go:23`) 
          -| Used only when we must show an absolute URL (e.g. in error p
          -ages). All redirects are **relative** (`/`, `/#/...`), so they
          - work on any host. The callback and webhook URLs live in the G
          -itHub App settings, not in code. |                            
       24 -| fw-parse (cloud only) | existing `FW_PARSE_BASE_URL` plus th
          -e same Parse headers as `network/email/invitation.go:72` | Not
          - used when `LIVEREVIEW_IS_CLOUD=false`. |                     
       25 -| Mode | existing `LIVEREVIEW_IS_CLOUD` (`internal/api/server.
          -go:100`) | Selects which identity finisher runs (cloud or self
          --hosted). |                                                   
       26 -| UI | `GET /api/v1/ui-config` (`server.go:3047`) gains `githu
          -bAppEnabled`, `githubAppInstallPath` | The UI bundle has no Gi
          -tHub URLs or IDs. |                                           
       27 -| Landing page | one constant in hexmoshomepage (e.g. `LIVEREV
          -IEW_APP_URL` in `urls.js`) | No repeated inline `https://liver
          -eview.hexmos.com` strings. |                                  
       69 +| App ID | `GITHUB_APP_ID` (int) | If unset, **the feature is 
          +off** (no routes active, `ui-config.githubAppEnabled=false`). 
          +|                                                             
       70 +| App slug | `GITHUB_APP_SLUG` | Used for the install URL and 
          +the bot login `<slug>[bot]`. |                                
       71 +| OAuth client | `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_CLIENT_SE
          +CRET` | From the App settings page (not the onelogin OAuth App
          +). |                                                          
       72 +| Private key | `GITHUB_APP_PRIVATE_KEY` | Either the PEM text
          + (starts with `-----BEGIN`) or a file path. Replace a literal 
          +`\n` with a newline before parsing; env files often store it e
          +scaped. |                                                     
       73 +| Webhook secret | `GITHUB_APP_WEBHOOK_SECRET` | Required. |  
       74 +| GitHub host | `GITHUB_APP_BASE_URL`, default `https://github
          +.com` | API base comes from `apiBaseURL()` in `internal/provid
          +ers/github/base_url.go:8` (github.com → `https://api.github.co
          +m`, GHES → `<base>/api/v3`). Export it (T2.1). |              
       75 +| fw-parse | existing `FW_PARSE_BASE_URL` (no trailing `/parse
          +`), `FW_PARSE_APP_ID` (default `impressionserver`), `FW_PARSE_
          +ADMIN_SECRET` | Same as `network/email/invitation.go:28,72-93`
          +. Cloud only. |                                               
       76 +| Hexmos JWT secret | existing `CLOUD_JWT_SECRET` | Already us
          +ed by `EnsureCloudUser`. |                                    
       77 +| State signing | existing `JWT_SECRET` | HMAC key for the OAu
          +th `state`. |                                                 
       78 +| Mode | existing `LIVEREVIEW_IS_CLOUD` → `s.deploymentConfig.
          +IsCloud` (`internal/api/server.go:110`) | Selects the identity
          + finisher. |                                                  
       79 +| UI | `GET /api/v1/ui-config` (`server.go:3047`) | Adds `gith
          +ubAppEnabled` (bool) and `githubAppBaseUrl` (string). No IDs o
          +r secrets. |                                                  
       80 +| Landing page | one constant in hexmoshomepage `urls.js` (`LI
          +VEREVIEW_APP`) | Replaces the inline `https://livereview.hexmo
          +s.com` strings. |                                             
       81  
       29 -## User flow                                                  
       82 +**Partial config** (some `GITHUB_APP_*` set, others missing, o
          +r an unparsable key): log one clear error at startup naming th
          +e missing vars, and treat the feature as off. Don't crash the 
          +server.                                                       
       83  
       84 +**Redirects are always relative** (`/`, `/#/…`), so they work 
          +on any host. The callback and webhook URLs live only in the Gi
          +tHub App settings.                                            
       85 +                                                              
       86 +---                                                           
       87 +                                                              
       88 +## 4. User flow                                               
       89 +                                                              
       90  ```
       32 -Landing "Start Free" / paste PR URL                           
       91 +Landing "Start Free" / pasted PR URL                          
       92     │  <instance>/gh-login?src=<cta>[&mr=<pr-url>]
       93     ▼
       35 -#/gh-login   [ Continue with GitHub · FREE ]    "Other ways to
          - sign in" → existing login (Cloud.tsx / SelfHosted.tsx)       
       36 -   │  page stores the destination with the existing redirect-a
          -fter-login helper                                             
       37 -   │  (sessionStorage 'redirectAfterLogin' = '#/get-started/gi
          -thub?mr=…')                                                   
       38 -   │  GET /api/v1/auth/github-app/start?mode=install|signin  →
          - state cookie + 302                                           
       94 +index.tsx: pathname '/gh-login' → location.replace('/#/gh-logi
          +n' + search)                                                  
       95     ▼
       40 -GitHub: install page (pick All / Only select repos → Install &
          - Authorize)   — or OAuth authorize for returning users        
       41 -   │  302 → /api/v1/auth/github-app/callback?code&installation
          -_id&setup_action&state                                        
       96 +#/gh-login (GhLogin.tsx)   [ Continue with GitHub · FREE ]   "
          +Already installed? Sign in"   "Other ways to sign in"         
       97 +   │  sessionStorage.redirectAfterLogin = '#/get-started/githu
          +b' (+ '?mr=…')                                                
       98 +   │  location.href = '/api/v1/auth/github-app/start?mode=inst
          +all&src=<src>'                                                
       99     ▼
       43 -Callback (server): verify state → exchange code → verify insta
          -llation belongs to user                                       
       44 -   │                                                          
       45 -   ├─ CLOUD:       fw-parse externalProviderSignup(provider=gi
          -thub_app, userToken)                                          
       46 -   │               → Parse _User created/found + Hexmos JWT is
          -sued (same as onelogin)                                       
       47 -   │               → shared EnsureCloudUser core (validates JW
          -T with CLOUD_JWT_SECRET) → LR user + org                      
       48 -   │               → upsert App connector for that org        
       49 -   │               → 302 /?data=<Parse result>   ← the exact h
          -andoff onelogin uses; Cloud.tsx finishes login                
       50 -   │                                                          
       51 -   └─ SELF-HOSTED: find local user by verified email (no fw-pa
          -rse)                                                          
       52 -                   → upsert App connector for the user's org  
       53 -                   → one-time code → 302 /#/gh-login/complete?
          -code=…                                                        
       54 -                   → POST /api/v1/auth/github-app/exchange → t
          -okenService.CreateTokenPair → LoginResponse                   
      100 +GET /api/v1/auth/github-app/start   → Set-Cookie lr_gh_state=<
          +nonce>; 302 to                                                
      101 +   install:  https://github.com/apps/<slug>/installations/new?
          +state=<signed>                                                
      102 +   signin:   https://github.com/login/oauth/authorize?client_i
          +d=<id>&state=<signed>                                         
      103     ▼
       56 -existing login completion → redirect-after-login → #/get-start
          -ed/github (repos → open PRs → Review this PR)                 
      104 +GitHub (user picks repos → "Install & Authorize")             
      105 +   │  302 → /api/v1/auth/github-app/callback?code=…&installati
          +on_id=…&setup_action=install&state=…                          
      106 +   ▼                                                          
      107 +callback:                                                     
      108 +   1 verify state + cookie        2 exchange code → user token
      109 +   3 primary+verified email       4 installation ∈ GET /user/i
          +nstallations                                                  
      110 +   5 CLOUD: fw-parse externalProviderSignup(github_app, userTo
          +ken) → {result:{jwt,data}}                                    
      111 +            EnsureCloudUserCore(jwt, email, source="gh-login:<
          +src>") → user_id, org_id                                      
      112 +   6 upsert GITHUB_APP connector(s) for org_id (reject if boun
          +d to another org)                                             
      113 +   7 mint + cache installation token; queue repo sync (async) 
      114 +   8 302 /?data=<slim Parse result>                           
      115 +   ▼                                                          
      116 +Cloud.tsx (unchanged): parses ?data= → POST ensure-cloud-user 
          +(idempotent) → LR tokens                                      
      117 +   → redirectAfterLogin → #/get-started/github?mr=…           
      118 +   ▼                                                          
      119 +#/get-started/github: repos ✓ → AI → open PRs / paste box → "R
          +eview this PR" → existing trigger-review → review page        
      120  ```
      121  
       59 -Returning users get an "Already installed? Sign in" link (`mod
          -e=signin`), which uses the App's OAuth authorize URL. The call
          -back then gets no `installation_id`. It lists `GET /user/insta
          -llations` and upserts connectors for every installation the us
          -er can access and that isn't bound to another org. If there ar
          -e none, it redirects to the install URL.                      
      122 +**Returning users** click "Already installed? Sign in" (`mode=
          +signin`). GitHub then shows only the authorize screen, and the
          + callback gets `code` but **no `installation_id`**. The callba
          +ck lists `/user/installations` and upserts a connector for eve
          +ry installation of *this App* that the user can access and tha
          +t isn't bound to another org (§6.3). If there are none, it red
          +irects to the install URL.                                    
      123  
       61 -## Phase 0: GitHub App (one-time, by a human)                 
       62 -Create it under the **HexmosTech** org with the final name, si
          -nce the name is the slug and the `<name>[bot]` identity. For d
          -evelopment, point the URLs at `https://manual-talent3.apps.hex
          -mos.com` (`make niceurl3`). At go-live, edit the same App to `
          -https://livereview.hexmos.com`. The ID, keys and secrets stay 
          -the same.                                                     
       63 -- Callback URL: `<instance>/api/v1/auth/github-app/callback`. 
          -**Turn on "Request user authorization (OAuth) during installat
          -ion"**, so one screen does both install and login. Setup URL i
          -s disabled by that, which is expected.                        
       64 -- Webhook URL: `<instance>/api/v1/github-app/webhook`, with a 
          -secret.                                                       
       65 -- Repo permissions: Pull requests R/W, Contents R, Issues R/W,
          - Metadata R. Account permission: Email addresses R. Events: Pu
          -ll request, Pull request review comment, Issue comment. `insta
          -llation*` events are sent automatically.                      
       66 -- Installable by **any account**.                             
       67 -- Before launch, uninstall the dev test installations: they're
          - bound to the local DB, and their webhooks will go to prod onc
          -e the URLs change.                                            
      124 +---                                                           
      125  
       69 -## Phase 1: fw-parse (cloud identity), small change           
       70 -- `isAuthenticatedMiddleware.js:146`: add an `externalServiceP
          -rovider == "github_app"` branch. `accessToken` is an already-e
          -xchanged **GitHub App user token**, so skip the code exchange 
          -(which would fail, because the current branch uses the onelogi
          -n OAuth App's client secret).                                 
       71 -- `cloud/oneLogin/providerMethods.js:30`: handle `github_app` 
          -like `github` (`getGithubUserData`). Parse then creates or fin
          -ds `_User` by email and returns `getJWT(...)`, so the JWT row 
          -is stored in Parse exactly as it is for onelogin users.       
       72 -- `cloud/oneLogin/githubOneLogin.js`: pick the **primary and `
          -verified`** email from `/user/emails` (for both `github` and `
          -github_app`), and reject the login if there isn't one.        
       73 -- Leave the GitHub client secret out of this change (`cloud/co
          -nfig.js:119` is a hard-coded secret; moving it to env is separ
          -ate hygiene work).                                            
       74 -- A later Google or email sign-in through onelogin with the sa
          -me email then finds the same Parse `_User`, and `ensure-cloud-
          -user` finds the same LiveReview user. One identity everywhere.
      126 +## 5. Data model                                              
      127  
       76 -## Phase 2: LiveReview backend                                
      128 +### 5.1 How an App installation is stored                     
      129  
       78 -### network/ (outbound HTTP only)                             
       79 -- `network/providers/github/app.go`: `MintAppJWT` (RS256, 9-mi
          -nute expiry, `golang-jwt/jwt/v5` is already in `go.mod`), `Cre
          -ateInstallationToken`, `ExchangeUserCode`, `GetUserEmails`, `L
          -istUserInstallations`, `ListInstallationRepos`, `ListOpenPulls
          -`. Every one takes the API base URL from config, with no liter
          -al `api.github.com`.                                          
       80 -- `network/hexmos/parse_signup.go` (cloud): `ExternalProviderS
          -ignup(ctx, provider, token, app)` → `POST {FW_PARSE_BASE_URL}/
          -functions/externalProviderSignup`, using the same header setup
          - as `network/email/invitation.go`.                            
       81 -- Update `network/network_status.md`.                         
      130 +One `integration_tokens` row per installation. All NOT NULL co
          +lumns must be filled:                                         
      131  
       83 -### storage/                                                  
       84 -- `storage/providers/github/token_store.go`: `UpsertAppInstall
          -ationConnector(orgID, installationID, accountLogin)` creates a
          -n `integration_tokens` row with `provider='github'`, `token_ty
          -pe='GITHUB_APP'`, `provider_app_id=<installation_id>`, `provid
          -er_url=<GITHUB_APP_BASE_URL>`, plus the cached token and `expi
          -res_at`. Also add `GetGitHubTokenByConnector(orgID, connectorI
          -D)`, `GetConnectorByInstallationID`, `UpdateCachedInstallation
          -Token`, `MarkInstallationRemoved`.                            
       85 -- Migrations: a partial unique index on `integration_tokens(pr
          -ovider_app_id) WHERE token_type='GITHUB_APP'` (one installatio
          -n belongs to exactly one org), and `orgs.signup_path text` (`g
          -h_app` or NULL). Self-hosted only: table `gh_login_codes(code_
          -hash, user_id, expires_at, used_at)` (60-second TTL, single us
          -e).                                                           
       86 -- Cloud: move the core of `EnsureCloudUser` (`internal/api/aut
          -h/handlers.go:636`: JWT validation with `CLOUD_JWT_SECRET`, th
          -e email-match check, find-or-create user + personal org + role
          -) into a function both the HTTP handler and the callback call,
          - with its SQL moved into `storage/users/`. **It's the same cod
          -e path as onelogin, not a copy.**                             
       87 -- Update `storage/storage_status.md`.                         
      132 +| column | value |                                            
      133 +|---|---|                                                     
      134 +| `provider` | `'github'` (so every existing `strings.HasPrefi
          +x(provider,"github")` branch picks it up) |                   
      135 +| `token_type` | `'GITHUB_APP'` (existing PAT rows use `'PAT'`
          +) |                                                           
      136 +| `provider_app_id` | installation id as text, e.g. `'51234567
          +'` |                                                          
      137 +| `provider_url` | `GITHUB_APP_BASE_URL` (e.g. `https://github
          +.com`) |                                                      
      138 +| `access_token` | `''` (NOT NULL; unused for App rows) |     
      139 +| `pat_token` | **current installation token** (`ghs_…`), refr
          +eshed before expiry |                                         
      140 +| `expires_at` | the installation token's `expires_at` |      
      141 +| `connection_name` | `'GitHub App · <account_login>'` |      
      142 +| `org_id` | the LiveReview org |                             
      143 +| `metadata` | `{"github_app":true,"installation_id":51234567,
          +"account_login":"acme","account_type":"Organization","reposito
          +ry_selection":"selected","app_slug":"<slug>","suspended":false
          +,"removed":false}` |                                          
      144  
       89 -### Endpoints                                                 
       90 -- `GET /api/v1/auth/github-app/start` (public): an HMAC state 
          -over `{nonce, src, mode, exp:10m}` (secret: `JWT_SECRET`), mir
          -rored in an HttpOnly cookie, then a 302 to the install or auth
          -orize URL built from config.                                  
       91 -- `GET /api/v1/auth/github-app/callback` (public):            
       92 -  1. The state must be valid and match the cookie.            
       93 -  2. Exchange the code for a user token.                      
       94 -  3. `installation_id` must appear in `/user/installations`. W
          -ithout this, anyone could claim someone else's installation.  
       95 -  4. **Identity finisher**, chosen by mode:                   
       96 -     - **Cloud:** `ExternalProviderSignup("github_app", userTo
          -ken, "livereview")` → Parse result with the JWT → shared Ensur
          -eCloudUser core → org. Set `orgs.signup_path='gh_app'` on a ne
          -wly created org. Fire the signup Discord notification with sou
          -rce `gh-login:<src>`.                                         
       97 -     - **Self-hosted:** find the local user by verified email.
          - By default only existing users can sign in (admins create use
          -rs today). With no admin set up yet, redirect to the existing 
          -setup screen. A `GITHUB_APP_ALLOW_SIGNUP` flag for self-hosted
          - can come later.                                              
       98 -  5. `UpsertAppInstallationConnector`. If the installation is 
          -bound to another org, show an error page: "connected to anothe
          -r LiveReview account".                                        
       99 -  6. Sync repos into projects with manual trigger enabled (reu
          -se the `EnableManualTriggerForAllProjects` logic, `server.go:1
          -471`).                                                        
      100 -  7. Redirect. Cloud: `/?data=<encoded Parse result>`, the sam
          -e handoff `onelogin-node` `redirectBackToOrigin` uses, so `ui/
          -src/pages/Auth/Cloud.tsx` finishes the login unchanged. Self-h
          -osted: `/#/gh-login/complete?code=<one-time>`.                
      101 -- `POST /api/v1/auth/github-app/exchange` (public, self-hosted
          - only): consumes the one-time code → `tokenService.CreateToken
          -Pair` → the same `LoginResponse` shape as `/auth/login`.      
      102 -- `POST /api/v1/github-app/webhook` (public, verifies `X-Hub-S
          -ignature-256`): `installation.id` → connector → org, then the 
          -existing `WebhookOrchestratorV2Handler` path, as `/github-hook
          -/:connector_id` does. It also handles `installation.*` and `in
          -stallation_repositories.*`.                                   
      103 -- `GET /api/v1/github-app/demo` (full chain: `RequireAuth` → `
          -BuildOrgContextFromHeader` → `ValidateOrgAccess` → `BuildPermi
          -ssionContext`): the org's App connector, repos, and open PRs (
          -capped).                                                      
      104 -- `getUIConfig` adds `githubAppEnabled`.                      
      145 +**Why the token goes in `pat_token`:** about 15 existing reade
          +rs do `SELECT pat_token FROM integration_tokens WHERE id=$1`: 
      146 +- `scheduled_review_worker.go:80`                             
      147 +- `blast_radius_worker.go:197`                                
      148 +- `repo_sync_worker.go:54`                                    
      149 +- `diff_review.go:543`                                        
      150 +- `connectors.go:492,665`                                     
      151 +- `repository_access.go:66`                                   
      152 +- `server.go:2868,2942`                                       
      153 +- `auto_webhook_installer.go:258`                             
      154 +- the unified processor queries                               
      155  
      106 -### Token resolver + cross-tenant fix (required)              
      107 -- `internal/provider_input/github/token_resolver.go` `ResolveG
          -itHubAccessToken(ctx, rec)`: `PAT` returns `pat_token`, unchan
          -ged. `GITHUB_APP` returns the cached token if it's valid for m
          -ore than 5 more minutes, otherwise mints a new one.           
      108 -- **Existing bug:** `storage/providers/github/token_store.go:2
          -1` `GetLatestGitHubToken()` picks the newest GitHub token **ac
          -ross all orgs**. It's used by `internal/provider_input/github/
          -github_provider.go` (lines 145, 198, 258, 272, 286, 307, 409, 
          -440) and `lrc_fetch.go:28`. With many App orgs this *will* mix
          - tenants. Fix: `GitHubV2Provider` resolves the connector from 
          -the webhook route or orchestrator through `GetGitHubTokenByCon
          -nector`. Delete `GetLatestGitHubToken`.                       
      109 -- Switch `PatToken` readers to the resolver: `github_provider.
          -go`, `lrc_fetch.go:33`, `internal/api/reviews_api.go` `ensureV
          -alidToken`/`checkTokenNeedsRefresh` (lines 187 and 262; App to
          -kens expire), and `auto_webhook_installer.go:125` (App connect
          -ors skip per-repo webhook install).                           
      110 -- Overlap rule: if an org has both a PAT connector and the App
          - on the same repo, the App connector wins and PAT-webhook even
          -ts for that repo are ignored, so there are no double reviews. 
      156 +Keeping a fresh installation token there (T3.4 refresher + T3.
          +3 on-demand resolver) makes them all work without rewriting ea
          +ch one. Hot paths still call the resolver, so they never use a
          + stale token.                                                 
      157  
      112 -## Phase 3: LiveReview UI                                     
      113 -- `/gh-login` entry: `cmd/ui.go:374` already serves `index.htm
          -l` for unknown paths. In the `App.tsx` bootstrap, `pathname ==
          -= '/gh-login'` → `replace('/#/gh-login' + search)`.           
      114 -- `ui/src/pages/Auth/GhLogin.tsx` (`#/gh-login`): "Review your
          - first PR in 2 minutes · FREE", a big **Continue with GitHub**
          - button, an "Already installed? Sign in" link, and "Other ways
          - to sign in", which uses the existing `Cloud.tsx` or `SelfHost
          -ed.tsx` login depending on `isCloud`. Before leaving the page 
          -it stores `#/get-started/github?mr=…` with the existing redire
          -ct-after-login helper (`ui/src/utils/authHelpers.ts`, sessionS
          -torage, which survives the same-tab GitHub round trip). It's h
          -idden, and redirects to normal login, when `githubAppEnabled` 
          -is false. If the user is already logged in, it goes straight t
          -o `#/get-started/github`.                                     
      115 -- `#/gh-login/complete` (self-hosted): exchanges the code, the
          -n calls `handleLoginSuccess`, then follows redirect-after-logi
          -n.                                                            
      116 -- `#/get-started/github` (only for `signup_path='gh_app'` orgs
          -):                                                            
      117 -  1. Repos connected ✓, with "Add more repos" linking to the i
          -nstallation settings URL returned by the API.                 
      118 -  2. AI (see open decision 1).                                
      119 -  3. Open PRs plus a "paste a PR URL" box prefilled from `mr`.
          - **Review this PR** calls the existing `POST /api/v1/connector
          -s/trigger-review`, then opens the review detail page.         
      120 -  4. A link: "Next: install the CLI".                         
      121 -- The dashboard shows a "Continue setup" banner for `gh_app` o
          -rgs until the first review runs. Other orgs see what they see 
          -today.                                                        
      122 -- Repo rules: add a `megaMenuData.ts` entry for `#/get-started
          -/github` (visible only for `gh_app` orgs), and add `internal/d
          -ocindex/docs/routes_guide/auth/gh-login.md` and `.../get-start
          -ed-github.md`.                                                
      158 +### 5.2 Migration `db/migrations/20261020090000_github_app_ins
          +tallations.sql`                                               
      159  
      124 -## Phase 4: hexmoshomepage                                    
      125 -- Add one constant for the app URL, and build `…/gh-login?src=
          -<where>` from it.                                             
      126 -- Point these at it: `pages/livereview/index.tsx` (`handleBuyN
          -ow` line 529, Start Free at 562, 987, 2467), `components/liver
          -eview/StartFreeModal.tsx:146` (Try Cloud; self-hosted unchange
          -d), `components/livereview/PricingSection.tsx:181` (free plan 
          -only), `pages/livereview/blog/index.tsx:153`, `pages/liverevie
          -w/blog/[slug].tsx:174`, `pages/livereview/why-livereview.tsx:2
          -22,384`.                                                      
      127 -- Optional: a hero input "Paste a GitHub PR URL → review it fr
          -ee" (modeled on LiveAPI's `RepoInput`, `pages/liveapi/index.ts
          -x:129`) → `/gh-login?mr=<encoded>&src=hero`.                  
      160 +```sql                                                        
      161 +-- migrate:up                                                 
      162 +-- One GitHub App installation can belong to exactly one LiveR
          +eview org.                                                    
      163 +CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github
          +_app_installation                                             
      164 +    ON integration_tokens (provider_url, provider_app_id)     
      165 +    WHERE token_type = 'GITHUB_APP';                          
      166  
      129 -## What does NOT change                                       
      130 -- **onelogin-node:** no change, same providers and order.     
      131 -- **Git Providers page and the PAT flow:** unchanged for all p
          -roviders. PAT connectors keep `/github-hook/:connector_id`. Th
          -ey're affected only by the cross-tenant bug fix.              
      132 -- **Self-hosted without `GITHUB_APP_*` set:** the feature is i
          -nvisible.                                                     
      167 +-- How the org was created; 'gh_app' for orgs created through 
          +/gh-login. Analytics + onboarding banner only.                
      168 +ALTER TABLE orgs ADD COLUMN IF NOT EXISTS signup_path TEXT;   
      169  
      134 -## Open decisions (need Shrijith's call)                      
      135 -1. **AI for the demo.** Option (a): auto-provision the managed
          - "LiveReview AI Model" for `gh_app` orgs, using the same inser
          -t as `ConfirmPurchase` (`internal/license/payment/subscription
          -_service.go:1603`), with a small LOC cap. Option (b): an inlin
          -e "paste Gemini key" field. Today `OnboardingSteps.tsx:~93` se
          -nds free-plan users to *upgrade* when they click Configure AI,
          - so "FREE" needs one of these.                                
      136 -2. **Demo limits:** 7 days and/or a 1-repo cap, or just the LO
          -C quota?                                                      
      137 -3. **Auto-review new PRs:** off by default (manual trigger plu
          -s a toggle), or on for the selected repos?                    
      170 +-- migrate:down                                               
      171 +DROP INDEX IF EXISTS uq_integration_tokens_github_app_installa
          +tion;                                                         
      172 +ALTER TABLE orgs DROP COLUMN IF EXISTS signup_path;           
      173 +```                                                           
      174  
      139 -## Verification                                               
      140 -1. Dev App on `https://manual-talent3.apps.hexmos.com` (`make 
          -niceurl3`), the `GITHUB_APP_*` vars in `.env`, `LIVEREVIEW_IS_
          -CLOUD=true`, `FW_PARSE_BASE_URL` pointing at a dev fw-parse ru
          -nning the Phase 1 change. Run `make develop`.                 
      141 -2. fw-parse: a jest test for the `github_app` branch (no code 
          -exchange, rejects an unverified email, creates `_User` and ret
          -urns a JWT).                                                  
      142 -3. Go tests: `go test ./network/providers/github/ -run App` (a
          -gainst `httptest`). `go test ./internal/api/... -run GitHubApp
          -`: a bad state is rejected, an installation not in `/user/inst
          -allations` is rejected, an installation bound to another org i
          -s rejected, a bad webhook signature returns 401, and the self-
          -hosted one-time code works once only.                         
      143 -4. Tenant isolation: two orgs (App + PAT, and PAT + PAT). A we
          -bhook for org A resolves only org A's connector. Grep shows `G
          -etLatestGitHubToken` has no callers left.                     
      144 -5. Manual E2E (private window): landing "Start Free" → `/gh-lo
          -gin` → install on one repo → land on `#/get-started/github` → 
          -Review a PR → bot comment on the PR. Check that the Parse `_Us
          -er` row and JWT exist. Then log out and sign in with **Google 
          -via onelogin** using the same email: it must land in the **sam
          -e** LiveReview account and org, with the connector still there
          -. Also check: "Other ways to sign in" behaves as today, `?mr=`
          - is prefilled, and uninstalling or removing repos updates the 
          -connector.                                                    
      145 -6. Config check: with `GITHUB_APP_*` unset, `/gh-login` redire
          -cts to normal login, and there are no GitHub URLs in the UI bu
          -ndle (grep `ui/dist` is only possible after a build, which nee
          -ds approval, so grep `ui/src` instead).                       
      146 -7. `make check-status-doc`, and `npx tsc --noEmit` in `ui/`. N
          -o full UI build without asking first.                         
      175 +Run `dbmate up` locally. dbmate rewrites `db/schema.sql`, so c
          +ommit that too.                                               
      176 +                                                              
      177 +(Self-hosted needs `gh_login_codes`. It's added in §8 later, n
          +ot now.)                                                      
      178 +                                                              
      179 +---                                                           
      180 +                                                              
      181 +## 6. Tasks                                                   
      182 +                                                              
      183 +Task IDs: **T1.x** fw-parse, **T2.x** LiveReview cross-tenant 
          +fix, **T3.x** LiveReview App backend, **T4.x** UI, **T5.x** he
          +xmoshomepage. **T0.x** is human setup.                        
      184 +                                                              
      185 +### Phase 0: human setup (done once, by a person, not the mode
          +l)                                                            
      186 +                                                              
      187 +**T0.1 Create the GitHub App** under the HexmosTech org with t
          +he final public name. The name becomes the slug and the `<slug
          +>[bot]` identity.                                             
      188 +- Homepage: `https://hexmos.com/livereview`                   
      189 +- **Callback URL:** `https://manual-talent3.apps.hexmos.com/ap
          +i/v1/auth/github-app/callback`                                
      190 +- ✅ "Expire user authorization tokens"                       
      191 +- ✅ **"Request user authorization (OAuth) during installation
          +"**. This greys out the Setup URL, which is expected.         
      192 +- **Webhook URL:** `https://manual-talent3.apps.hexmos.com/api
          +/v1/github-app/webhook`, plus a webhook secret. Make it random
          +, 32+ bytes.                                                  
      193 +- **Repository permissions:** Pull requests R/W, Contents R, I
          +ssues R/W, Metadata R.                                        
      194 +- **Account permissions:** Email addresses R.                 
      195 +- **Subscribe to events:** Pull request, Pull request review c
          +omment, Issue comment. (`installation` and `installation_repos
          +itories` are always sent.)                                    
      196 +- "Where can this app be installed": **Any account**.         
      197 +- Generate a private key and save the `.pem`. Copy the App ID,
          + Client ID, generate a Client secret, and note the slug.      
      198 +                                                              
      199 +**T0.2 Local env** (`LiveReview/.env`):                       
      200 +```                                                           
      201 +GITHUB_APP_ID=…                                               
      202 +GITHUB_APP_SLUG=…                                             
      203 +GITHUB_APP_CLIENT_ID=Iv23…                                    
      204 +GITHUB_APP_CLIENT_SECRET=…                                    
      205 +GITHUB_APP_PRIVATE_KEY=/abs/path/to/app.private-key.pem       
      206 +GITHUB_APP_WEBHOOK_SECRET=…                                   
      207 +GITHUB_APP_BASE_URL=https://github.com                        
      208 +LIVEREVIEW_IS_CLOUD=true                                      
      209 +FW_PARSE_BASE_URL=<dev fw-parse running T1.x, no trailing /par
          +se>                                                           
      210 +CLOUD_JWT_SECRET=<same secret the dev fw-parse signs JWTs with
          +>                                                             
      211 +```                                                           
      212 +Run: `make develop` (backend :8888), `cd ui && make run` (:808
          +1), `make niceurl3` (manual-talent3 → :8081, which proxies `/a
          +pi` → :8888).                                                 
      213 +                                                              
      214 +**T0.3 Confirm `state` round-trips.** After T3.6 exists, insta
          +ll once and check the callback log for a `state` value. GitHub
          + forwards `state` from the install URL. If it doesn't, stop an
          +d tell the human; don't remove the state check.               
      215 +                                                              
      216 +**T0.4 At go-live:**                                          
      217 +- Change the same App's callback and webhook URLs to `https://
          +livereview.hexmos.com/...`. The ID, keys and secrets stay the 
          +same.                                                         
      218 +- Set the env vars in prod.                                   
      219 +- **Uninstall the dev test installations first**: they're boun
          +d to the local DB, and their webhooks would start hitting prod
          +.                                                             
      220 +                                                              
      221 +---                                                           
      222 +                                                              
      223 +### Phase 1: fw-parse (PR A, repo `fw-parse`)                 
      224 +                                                              
      225 +Goal: `POST /parse/functions/externalProviderSignup` with `ext
          +ernalServiceProvider: "github_app"` and an already-exchanged G
          +itHub **user token** returns a Hexmos JWT for the token's **pr
          +imary, verified** email, creating the Parse `_User` if needed.
          + Existing providers behave exactly as before.                 
      226 +                                                              
      227 +**T1.1 `cloud/oneLogin/githubOneLogin.js`.** Add a new exporte
          +d function. Don't change `getGithubUserData`.                 
      228 +```js                                                         
      229 +// For the LiveReview GitHub App sign-in: only a primary *veri
          +fied* email is accepted.                                      
      230 +getGithubAppUserData: async function (accessToken) {          
      231 +  const headers = { Accept: "application/vnd.github+json", Aut
          +horization: `Bearer ${accessToken}`, "X-GitHub-Api-Version": "
          +2022-11-28" }                                                 
      232 +  const profileRes = await fetch(GITHUB_USER_API_URL, { header
          +s })                                                          
      233 +  if (!profileRes.ok) throw new Error(`GitHub /user failed: ${
          +profileRes.status}`)                                          
      234 +  const profile = await profileRes.json()                     
      235 +  const emailsRes = await fetch(GITHUB_EMAIL_API_URL, { header
          +s })                                                          
      236 +  if (!emailsRes.ok) throw new Error(`GitHub /user/emails fail
          +ed: ${emailsRes.status}`)                                     
      237 +  const emails = await emailsRes.json()                       
      238 +  const primary = Array.isArray(emails) ? emails.find((e) => e
          +.primary === true && e.verified === true) : null              
      239 +  if (!primary) throw new Error("GitHub account has no verifie
          +d primary email")                                             
      240 +  return { email: primary.email.toLowerCase(), name: profile.n
          +ame || profile.login, picture: profile.avatar_url }           
      241 +}                                                             
      242 +```                                                           
      243 +Unlike `getGithubUserData`, it **throws** instead of returning
          + `null`, so the caller can't continue with `email=undefined`. 
      244 +                                                              
      245 +**T1.2 `cloud/oneLogin/providerMethods.js` `externalMethods` (
          +line 20).** Add a branch:                                     
      246 +```js                                                         
      247 +} else if (provider == "github_app") {                        
      248 +  simpleUserDate = await getGithubAppUserData(accessToken)    
      249 +}                                                             
      250 +```                                                           
      251 +Import it next to `getGithubUserData` (line 2).               
      252 +                                                              
      253 +**T1.3 `cloud/oneLogin/externalProviderSignin.js` (line 8).** 
          +Add `github_app` to the provider `if` chain. Without this the 
          +function returns "Unknown provider" even after T1.2.          
      254 +```js                                                         
      255 +} else if (externalServiceProvider === "github_app") {        
      256 +  return await externalMethods(params)                        
      257 +```                                                           
      258 +                                                              
      259 +**T1.4 `isAuthenticatedMiddleware.js` (line 146).** No behavio
          +r change is needed: the `github` branch only matches `"github"
          +`, so `github_app` already skips the code exchange and goes st
          +raight to `externalMethods`. Add a short comment above the `if
          +` chain for whoever reads it next:                            
      260 +```js                                                         
      261 +// "github_app" (LiveReview GitHub App) sends an already-excha
          +nged user token, so no code exchange here.                    
      262 +```                                                           
      263 +                                                              
      264 +**T1.5 Test** in `__tests__/` (these are integration tests aga
          +inst a running Parse, following `__tests__/testHelper.js`):   
      265 +- `github_app` with a bogus token → `result.success === false`
          +. (The function catches the error and returns `{success:false}
          +` with HTTP 200.)                                             
      266 +- Unit-test `getGithubAppUserData` with `fetch` mocked (jest `
          +jest.mock("node-fetch")` or by stubbing `global.fetch`):      
      267 +  - an unverified primary email → throws;                     
      268 +  - a verified primary email → returns the lowercase email.   
      269 +                                                              
      270 +**Done when:** a manual `curl` against dev Parse with a real `
          +ghu_` token returns `{"result":{"jwt":"…","data":{"email":…}}}
          +`, and a Parse `_User` with that email exists.                
      271 +                                                              
      272 +Out of scope: moving the hard-coded GitHub secret at `cloud/co
          +nfig.js:119` to env. That's separate hygiene work.            
      273 +                                                              
      274 +---                                                           
      275 +                                                              
      276 +### Phase 2: cross-tenant fix (PR B, LiveReview; ship before t
          +he App)                                                       
      277 +                                                              
      278 +**Existing bug:** `storage/providers/github/token_store.go:21`
          + `GetLatestGitHubToken()` returns the newest GitHub connector 
          +**across all orgs**. Every GitHub webhook-path call goes throu
          +gh it via `FindIntegrationTokenForGitHubRepo` (`internal/provi
          +der_input/github/github_token_helpers.go:27`), so with two Git
          +Hub orgs, org B's webhook gets org A's token. The App would ma
          +ke this common, so fix it first, on its own.                  
      279 +                                                              
      280 +**T2.1 Export the API base helper.** `internal/providers/githu
          +b/base_url.go`: add                                           
      281 +```go                                                         
      282 +// APIBaseURL is the exported form of apiBaseURL for callers o
          +utside this package.                                          
      283 +func APIBaseURL(baseURL string) string { return apiBaseURL(bas
          +eURL) }                                                       
      284 +```                                                           
      285 +                                                              
      286 +**T2.2 Storage: connector-scoped lookup.** `storage/providers/
          +github/token_store.go`:                                       
      287 +- Add `OrgID int64`, `TokenType string` and `ExpiresAt sql.Nul
          +lTime` to `IntegrationTokenRecord`.                           
      288 +- Add:                                                        
      289 +```go                                                         
      290 +// GetGitHubTokenByConnectorID loads one GitHub connector by p
          +rimary key.                                                   
      291 +func (s *TokenStore) GetGitHubTokenByConnectorID(connectorID i
          +nt64) (*IntegrationTokenRecord, error) {                      
      292 +  const q = `                                                 
      293 +    SELECT id, provider, provider_url, COALESCE(pat_token,''),
          + COALESCE(metadata,'{}'), org_id,                             
      294 +           COALESCE(token_type,''), expires_at                
      295 +    FROM integration_tokens                                   
      296 +    WHERE id = $1 AND provider LIKE 'github%'`                
      297 +  …                                                           
      298 +}                                                             
      299 +```                                                           
      300 +- **Delete** `GetLatestGitHubToken`.                          
      301 +- Update `storage/storage_status.md`: add the new function and
          + mark the old one removed.                                    
      302 +                                                              
      303 +**T2.3 Carry the connector ID on the event, early.** `internal
          +/api/webhook_orchestrator_v2.go` `ProcessWebhookEvent`:       
      304 +- Move the block that stamps `connector_id` into `event.Reposi
          +tory.Metadata` / `event.MergeRequest.Metadata` (currently line
          +s ~254-297, after the warrant check) to **right after** `conve
          +rtToUnifiedEvent` succeeds (after line ~206, before `getBotUse
          +rInfo` at ~215). `getBotUserInfo` and `CheckResponseWarrant` n
          +eed the token.                                                
      305 +- Read `connectorID` from `auth.GetConnectorIDFromContext(c)` 
          +there. If it's missing, return the same 500 `internal_error` a
          +s today.                                                      
      306 +- Leave the later `orgID` read and the signature check where t
          +hey are, just reuse the variable.                             
      307 +                                                              
      308 +**T2.4 Resolve tokens by connector.** `internal/provider_input
          +/github/github_token_helpers.go`:                             
      309 +```go                                                         
      310 +// connectorIDFromMetadata reads connector_id stamped by the w
          +ebhook orchestrator. After a JSON                             
      311 +// round trip through the job queue it arrives as float64, so 
          +accept the numeric forms.                                     
      312 +func connectorIDFromMetadata(meta map[string]interface{}) (int
          +64, error) {                                                  
      313 +  switch v := meta["connector_id"].(type) {                   
      314 +  case int64:                                                 
      315 +    return v, nil                                             
      316 +  case int:                                                   
      317 +    return int64(v), nil                                      
      318 +  case float64:                                               
      319 +    return int64(v), nil                                      
      320 +  case json.Number:                                           
      321 +    return v.Int64()                                          
      322 +  }                                                           
      323 +  return 0, fmt.Errorf("connector_id missing from event metada
          +ta")                                                          
      324 +}                                                             
      325 +                                                              
      326 +// FindIntegrationTokenForGitHubConnector loads the GitHub con
          +nector that received this event.                              
      327 +func FindIntegrationTokenForGitHubConnector(db *sql.DB, connec
          +torID int64) (*IntegrationToken, error)                       
      328 +```                                                           
      329 +- Delete `FindIntegrationTokenForGitHubRepo`.                 
      330 +- Add `OrgID` and `TokenType` to `IntegrationToken`.          
      331 +- Callers in `internal/provider_input/github/github_provider.g
          +o` (all of them, by current line):                            
      332 +                                                              
      333 +| line | method | connector comes from |                      
      334 +|---|---|---|                                                 
      335 +| 145 | `FetchMergeRequestData(event)` | `event.MergeRequest.M
          +etadata` (fall back to `event.Repository.Metadata`, which is t
          +he same value, not a different token) |                       
      336 +| 190-198 | `FindIntegrationTokenForRepo(repoFullName)` | **Ch
          +ange the signature** to `FindIntegrationTokenForEvent(event Un
          +ifiedWebhookEventV2)` and update its caller `internal/api/unif
          +ied_processor_v2.go:735` |                                    
      337 +| 258 | `PostCommentReply` | `event.Repository.Metadata` |    
      338 +| 272 | `PostEmojiReaction` | `event.Repository.Metadata` |   
      339 +| 286 | `PostFullReview` | `event.Repository.Metadata` |      
      340 +| 307 | `FetchMRTimeline(mr)` | `mr.Metadata` |               
      341 +| 409 | `GetBotUserInfo(repository)` | `repository.Metadata` |
      342 +| 440 | `PostReviewComments(mr, …)` | `mr.Metadata` |         
      343 +                                                              
      344 +- `internal/provider_input/github/lrc_fetch.go:28` `GetRepoCon
          +figFiles(ctx, repoFullName, ref)` has no metadata (it implemen
          +ts the `lrcfetch.Provider` interface). Add in `github_token_he
          +lpers.go`:                                                    
      345 +```go                                                         
      346 +type connectorCtxKey struct{}                                 
      347 +func WithConnectorID(ctx context.Context, id int64) context.Co
          +ntext { return context.WithValue(ctx, connectorCtxKey{}, id) }
      348 +func connectorIDFromContext(ctx context.Context) (int64, bool)
          + { id, ok := ctx.Value(connectorCtxKey{}).(int64); return id, 
          +ok }                                                          
      349 +```                                                           
      350 +  The caller at `internal/api/webhook_orchestrator_v2.go:1084`
          + wraps `ctx` with `githubinput.WithConnectorID(ctx, connectorI
          +D)`, using whatever import alias that file already uses for `i
          +nternal/provider_input/github`. If the ID is missing, return a
          +n error (no fallback). `internal/review/service.go:497` calls 
          +the *other* `GetRepoConfigFiles` (the `internal/providers/gith
          +ub` PAT-configured provider), so leave it alone.              
      351 +                                                              
      352 +**T2.5 Tests.** `internal/provider_input/github/github_token_h
          +elpers_test.go`:                                              
      353 +- `connectorIDFromMetadata` handles int64, float64, json.Numbe
          +r and missing.                                                
      354 +                                                              
      355 +`internal/api/webhook_orchestrator_v2_test.go` (follow the exi
          +sting setup in that file):                                    
      356 +- Two orgs, each with a GitHub PAT connector.                 
      357 +- A webhook on `/github-hook/<A>` must load connector A's toke
          +n, never B's.                                                 
      358 +                                                              
      359 +**Done when:**                                                
      360 +- `grep -rn "GetLatestGitHubToken\|FindIntegrationTokenForGitH
          +ubRepo" --include=*.go .` is empty.                           
      361 +- `go build ./...` passes.                                    
      362 +- The tests above pass.                                       
      363 +- `make check-status-doc` passes.                             
      364 +                                                              
      365 +---                                                           
      366 +                                                              
      367 +### Phase 3: GitHub App backend (PR C, LiveReview)            
      368 +                                                              
      369 +**T3.1 Config package**: `internal/githubapp/config.go` (new p
          +ackage; no DB, no HTTP).                                      
      370 +```go                                                         
      371 +package githubapp                                             
      372 +                                                              
      373 +type Config struct {                                          
      374 +  AppID         int64                                         
      375 +  Slug          string                                        
      376 +  ClientID      string                                        
      377 +  ClientSecret  string                                        
      378 +  WebhookSecret string                                        
      379 +  BaseURL       string          // e.g. https://github.com (no
          + trailing slash)                                              
      380 +  PrivateKey    *rsa.PrivateKey                               
      381 +}                                                             
      382 +                                                              
      383 +// Load reads GITHUB_APP_* once. It returns nil when the featu
          +re is off (GITHUB_APP_ID unset)                               
      384 +// or misconfigured (logged once, naming the missing vars).   
      385 +func Load() *Config            // sync.Once around fromEnv()  
      386 +func fromEnv() (*Config, error) // testable; returns (nil, nil
          +) when GITHUB_APP_ID is empty                                 
      387 +                                                              
      388 +func (c *Config) APIBaseURL() string { return githubprovider.A
          +PIBaseURL(c.BaseURL) }                                        
      389 +func (c *Config) IsGitHubDotCom() bool { return c.BaseURL == "
          +https://github.com" }                                         
      390 +func (c *Config) InstallURL(state string) string   // github.c
          +om: <base>/apps/<slug>/installations/new?state=…              
      391 +                                                   // GHES:   
          +    <base>/github-apps/<slug>/installations/new?state=…       
      392 +func (c *Config) AuthorizeURL(state string) string // <base>/l
          +ogin/oauth/authorize?client_id=…&state=…                      
      393 +func (c *Config) InstallationSettingsURL(accountType, accountL
          +ogin string, installationID int64) string                     
      394 +  // User:         <base>/settings/installations/<id>         
      395 +  // Organization: <base>/organizations/<login>/settings/insta
          +llations/<id>                                                 
      396 +func (c *Config) BotLogin() string { return c.Slug + "[bot]" }
      397 +```                                                           
      398 +- Key parsing: if the value starts with `-----BEGIN`, use it a
          +fter replacing `\n` (the literal two characters) with newlines
          +; otherwise `os.ReadFile` it. Parse with `jwt.ParseRSAPrivateK
          +eyFromPEM` (`github.com/golang-jwt/jwt/v5`, already in `go.mod
          +`).                                                           
      399 +- Test `config_test.go`:                                      
      400 +  - unset → nil, nil;                                         
      401 +  - partial → error naming the vars;                          
      402 +  - PEM inline with `\n` → parses (generate a key in the test 
          +with `rsa.GenerateKey`);                                      
      403 +  - URLs for github.com vs GHES.                              
      404 +                                                              
      405 +**T3.2 Network calls**: `network/providers/github/app.go` (new
          +). Each function takes `apiBase` (or `base` for github.com web
          + endpoints) explicitly. No literal hosts. Use the package's ex
          +isting `NewHTTPClient` / `NewRequestWithContext` / `Do` (`netw
          +ork/providers/github/http_client_ops.go`). Set headers `Accept
          +: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11
          +-28`, `User-Agent: LiveReview`.                               
      406 +```go                                                         
      407 +func MintAppJWT(appID int64, key *rsa.PrivateKey, now time.Tim
          +e) (string, error)                                            
      408 +  // RS256, claims: iat=now-60s, exp=now+9m, iss=strconv.Forma
          +tInt(appID,10)                                                
      409 +                                                              
      410 +type InstallationToken struct { Token string; ExpiresAt time.T
          +ime }                                                         
      411 +func CreateInstallationToken(ctx context.Context, apiBase, app
          +JWT string, installationID int64) (*InstallationToken, error) 
      412 +  // POST {apiBase}/app/installations/{id}/access_tokens, Auth
          +orization: Bearer <appJWT>; expect 201                        
      413 +                                                              
      414 +type UserToken struct { AccessToken string }                  
      415 +func ExchangeUserCode(ctx context.Context, base, clientID, cli
          +entSecret, code string) (*UserToken, error)                   
      416 +  // POST {base}/login/oauth/access_token, form-encoded client
          +_id, client_secret, code; Accept: application/json.           
      417 +  // TRAP: GitHub answers 200 with {"error":"bad_verification_
          +code",...} on failure. Treat a non-empty "error" as failure.  
      418 +                                                              
      419 +type UserEmail struct { Email string; Primary, Verified bool }
      420 +func ListUserEmails(ctx context.Context, apiBase, userToken st
          +ring) ([]UserEmail, error)        // GET /user/emails         
      421 +                                                              
      422 +type UserProfile struct { Login, Name, AvatarURL string; ID in
          +t64 }                                                         
      423 +func GetAuthenticatedUser(ctx context.Context, apiBase, userTo
          +ken string) (*UserProfile, error)  // GET /user (user token on
          +ly!)                                                          
      424 +                                                              
      425 +type Installation struct {                                    
      426 +  ID int64; AppID int64; AccountLogin, AccountType, Repository
          +Selection string; Suspended bool                              
      427 +}                                                             
      428 +func ListUserInstallations(ctx context.Context, apiBase, userT
          +oken string) ([]Installation, error)                          
      429 +  // GET /user/installations?per_page=100, follow the Link hea
          +der. Map account.login, account.type, app_id, suspended_at != 
          +null.                                                         
      430 +                                                              
      431 +type InstallationRepo struct {                                
      432 +  ID int64; Name, FullName, HTMLURL, CloneURL, SSHURL, Default
          +Branch, Description string; Private bool                      
      433 +}                                                             
      434 +func ListInstallationRepositories(ctx context.Context, apiBase
          +, installationToken, cursor string) (repos []InstallationRepo,
          + next string, err error)                                      
      435 +  // GET /installation/repositories?per_page=100 → {"total_cou
          +nt":n,"repositories":[...]}. cursor = next page URL from the L
          +ink header.                                                   
      436 +                                                              
      437 +type OpenPull struct { Number int; Title, HTMLURL, AuthorLogin
          + string; UpdatedAt time.Time }                                
      438 +func ListOpenPulls(ctx context.Context, apiBase, installationT
          +oken, repoFullName string, limit int) ([]OpenPull, error)     
      439 +  // GET /repos/{full}/pulls?state=open&sort=updated&direction
          +=desc&per_page=<limit>                                        
      440 +                                                              
      441 +type PublicUser struct { ID int64; Login string }             
      442 +func GetUserByLogin(ctx context.Context, apiBase, token, login
          + string) (*PublicUser, error)                                 
      443 +  // GET /users/{login}; login "<slug>[bot]" must be path-esca
          +ped                                                           
      444 +```                                                           
      445 +- Test `app_test.go` with `httptest.NewServer`:               
      446 +  - `MintAppJWT` round-trips with the public key and `iss` is 
          +right;                                                        
      447 +  - `ExchangeUserCode` returns an error on a 200 + `{"error":…
          +}` response;                                                  
      448 +  - `ListUserInstallations` follows two pages;                
      449 +  - `CreateInstallationToken` parses `expires_at`.            
      450 +- Add every function to `network/network_status.md`.          
      451 +                                                              
      452 +**T3.3 Token resolver**: `internal/githubapp/token.go`. It liv
          +es here, not in `provider_input`, so `internal/api`, `internal
          +/provider_input/github` and `internal/jobqueue` can all import
          + it without cycles.                                           
      453 +                                                              
      454 +Storage first, in `storage/providers/github/token_store.go`:  
      455 +```go                                                         
      456 +type AppConnectorUpsert struct {                              
      457 +  OrgID int64; BaseURL string; InstallationID int64; AccountLo
          +gin, AccountType, RepositorySelection, AppSlug string         
      458 +  Token string; ExpiresAt time.Time                           
      459 +}                                                             
      460 +// UpsertAppInstallationConnector inserts or refreshes the GIT
          +HUB_APP row for (BaseURL, InstallationID).                    
      461 +// It returns ErrInstallationBoundToOtherOrg when the row exis
          +ts with a different org_id, and never moves it.               
      462 +func (s *TokenStore) UpsertAppInstallationConnector(in AppConn
          +ectorUpsert) (connectorID int64, created bool, err error)     
      463 +func (s *TokenStore) GetAppConnectorByInstallation(baseURL str
          +ing, installationID int64) (*IntegrationTokenRecord, error)   
      464 +func (s *TokenStore) UpdateCachedInstallationToken(connectorID
          + int64, token string, expiresAt time.Time) error              
      465 +func (s *TokenStore) ListAppConnectorsExpiringBefore(t time.Ti
          +me) ([]IntegrationTokenRecord, error) // skips metadata.remove
          +d/suspended                                                   
      466 +func (s *TokenStore) SetAppConnectorFlags(connectorID int64, r
          +emoved, suspended *bool) error         // jsonb_set on metadat
          +a; on removed=true also SET pat_token=''                      
      467 +func (s *TokenStore) ListAppConnectorsForOrg(orgID int64) ([]I
          +ntegrationTokenRecord, error)                                 
      468 +var ErrInstallationBoundToOtherOrg = errors.New("github app in
          +stallation is connected to another LiveReview organization")  
      469 +```                                                           
      470 +Write the upsert as a transaction: `SELECT … FOR UPDATE` on `(
          +provider_url, provider_app_id) WHERE token_type='GITHUB_APP'`.
          + If the row exists with a different `org_id`, return the error
          +. If it exists with the same org, update. If it doesn't exist,
          + insert. The unique index from §5.2 backs this up against race
          +s.                                                            
      471 +                                                              
      472 +Resolver:                                                     
      473 +```go                                                         
      474 +// ConnectorToken returns a usable token for any GitHub connec
          +tor. PAT rows return pat_token                                
      475 +// unchanged; GITHUB_APP rows return the cached installation t
          +oken, minting a new one when it                               
      476 +// expires within 5 minutes.                                  
      477 +func ConnectorToken(ctx context.Context, store *storagegithub.
          +TokenStore, rec *storagegithub.IntegrationTokenRecord) (string
          +, error)                                                      
      478 +func RefreshConnectorToken(ctx context.Context, store *storage
          +github.TokenStore, rec *storagegithub.IntegrationTokenRecord) 
          +(string, error) // unconditional mint + UpdateCachedInstallati
          +onToken                                                       
      479 +```                                                           
      480 +- Get the installation ID from `rec.Metadata` `installation_id
          +`, or parse `provider_app_id`.                                
      481 +- If `Load()` is nil and the row is `GITHUB_APP`, return the e
          +rror `"github app not configured on this server"`.            
      482 +- If the row is removed or suspended, return the error `"githu
          +b app installation removed"`.                                 
      483 +                                                              
      484 +Wire it in:                                                   
      485 +- `internal/provider_input/github/github_token_helpers.go` `Fi
          +ndIntegrationTokenForGitHubConnector` (T2.4): after loading, `
          +token.PatToken, err = githubapp.ConnectorToken(...)`. Every we
          +bhook-path caller then gets a valid token.                    
      486 +- `internal/api/review_service.go` `prepareAuthentication` (li
          +ne 350): after `findIntegrationToken`, when `token.TokenType =
          += "GITHUB_APP"`, call the resolver (load the record by `token.
          +ID`) and set `token.PatToken`.                                
      487 +- `internal/api/reviews_api.go`:                              
      488 +  - `ensureValidToken` (line 262): the GitHub branch accepts `
          +TokenType == "PAT" || TokenType == "GITHUB_APP"`.             
      489 +  - `buildProviderConfig` (~line 405): the condition `token.To
          +kenType == "PAT"` becomes `token.TokenType == "PAT" || token.T
          +okenType == "GITHUB_APP"`, so `pat_token` ends up in the provi
          +der config.                                                   
      490 +                                                              
      491 +**T3.4 Background refresh**: `internal/jobqueue/github_app_tok
          +en_refresh_worker.go`.                                        
      492 +- A River periodic job:                                       
      493 +  - `GitHubAppTokenRefreshJobArgs{}`, `Kind() = "github_app_to
          +ken_refresh"`.                                                
      494 +  - Interval 10 min, `RunOnStart: true`.                      
      495 +  - Register it next to the other periodic jobs in `internal/j
          +obqueue/jobqueue.go` (~line 2510-2540), **only when `githubapp
          +.Load() != nil`**.                                            
      496 +  - Register the worker the way `ReconciliationSweepWorker` is
          + registered (~line 2425).                                     
      497 +- Work: `ListAppConnectorsExpiringBefore(now+20m)` → `RefreshC
          +onnectorToken` for each. Log failures per connector and keep g
          +oing. Return nil unless the DB query fails.                   
      498 +- This keeps `pat_token` fresh for every legacy reader in §5.1
          +.                                                             
      499 +                                                              
      500 +**T3.5 Bot identity for App connectors.** `internal/provider_i
          +nput/github/github_provider.go` `GetBotUserInfo` (line 408): i
          +f `token.TokenType == "GITHUB_APP"`:                          
      501 +- Login = `cfg.BotLogin()`.                                   
      502 +- ID comes from `GetUserByLogin(apiBase, token, cfg.BotLogin()
          +)`, cached in a package-level `sync.Map` keyed by login.      
      503 +- Return `IsBot: true` and `user_type: "Bot"`.                
      504 +- **Don't** call `FetchGitHubBotUserInfo` (`GET /user`). It 40
          +3s for installation tokens.                                   
      505 +                                                              
      506 +Then check `CheckResponseWarrant` / the mention detection in `
          +internal/api/unified_processor_v2.go`. If it matches mentions 
          +against `botInfo.Username`, also accept `@<slug>` without the 
          +`[bot]` suffix, because that's what users type. Add a test for
          + both.                                                        
      507 +                                                              
      508 +**T3.6 HTTP endpoints**: new file `internal/api/github_app_aut
          +h.go`, routes registered in `internal/api/server.go`. Register
          + next to `public.POST("/auth/ensure-cloud-user", …)` at ~line 
          +1204, only when `s.githubApp != nil`, where `s.githubApp = git
          +hubapp.Load()` is set in `NewServer`:                         
      509 +```go                                                         
      510 +public.GET("/auth/github-app/start", s.GitHubAppStart)        
      511 +public.GET("/auth/github-app/callback", s.GitHubAppCallback)  
      512 +v1.POST("/github-app/webhook", s.GitHubAppWebhook) // next to 
          +/github-hook/:connector_id (~line 1609); no connector middlewa
          +re                                                            
      513 +```                                                           
      514 +                                                              
      515 +*State* (`internal/api/github_app_state.go`):                 
      516 +```go                                                         
      517 +type ghState struct { Nonce, Src, Mode string; Exp int64 }    
      518 +// sign: base64url(json) + "." + base64url(HMAC-SHA256(JWT_SEC
          +RET, "gh-app-state:"+base64url(json)))                        
      519 +func signGHState(s ghState, secret []byte) string             
      520 +func verifyGHState(raw string, secret []byte, now time.Time) (
          +*ghState, error) // constant-time compare, exp check          
      521 +```                                                           
      522 +- `src` comes from the start query. Allow only `[a-z0-9_-]{1,3
          +2}`; anything else becomes `"unknown"`.                       
      523 +- `mode` is `install` or `signin` (default `install`).        
      524 +- Cookie: `lr_gh_state=<nonce>; Path=/api/v1/auth/github-app; 
          +HttpOnly; SameSite=Lax; Max-Age=600`. Add `Secure` when `c.Sch
          +eme()=="https"` or `X-Forwarded-Proto: https`. **Lax, not Stri
          +ct**: the return from GitHub is a cross-site top-level GET.   
      525 +                                                              
      526 +*`GitHubAppStart`*: build the state (exp = now+10m), set the c
          +ookie, `302` to `InstallURL(state)` or `AuthorizeURL(state)`. 
      527 +                                                              
      528 +*`GitHubAppCallback`*. Every failure renders one small error p
          +age (T3.9) with a message and a "Try again" link to `/#/gh-log
          +in`. **Never 500 with a stack trace.**                        
      529 +1. Read `state`, `code`, `installation_id`, `setup_action`.   
      530 +   - If `setup_action == "request"`, an org member asked an or
          +g owner to approve. Show the page: "Installation requested. An
          + owner of <org> needs to approve it on GitHub. You'll get an e
          +mail." Stop.                                                  
      531 +2. `verifyGHState`, and the cookie nonce must equal `state.Non
          +ce`. Delete the cookie (Max-Age=-1) either way.               
      532 +3. `ExchangeUserCode` → user token.                           
      533 +4. `ListUserEmails` → pick `Primary && Verified`. If there's n
          +one, show the error "Your GitHub account needs a verified prim
          +ary email."                                                   
      534 +5. `ListUserInstallations` → keep only `inst.AppID == cfg.AppI
          +D`.                                                           
      535 +   - If `installation_id` is present, it **must** be in that l
          +ist. Otherwise reject: "That installation doesn't belong to yo
          +ur GitHub account." This is the ownership check; without it an
          +yone could claim someone else's installation by editing the UR
          +L.                                                            
      536 +   - Targets = `[that one]` when present, else all of them.   
      537 +   - If there are no targets in signin mode, 302 to `InstallUR
          +L(new state)`.                                                
      538 +6. **Identity (cloud)**, when `s.deploymentConfig.IsCloud`:   
      539 +   - `network/hexmos.ExternalProviderSignup(ctx, "github_app",
          + userToken, "livereview")` (T3.7).                            
      540 +   - Require `result.jwt != ""` and `result.data.email`. Lower
          +cased, it must equal the verified email from step 4; otherwise
          + reject.                                                      
      541 +   - `auth.EnsureCloudUserCore(ctx, input{JWT, Email, FirstNam
          +e, LastName, Source: "gh-login:"+src})` (T3.7) → `userID, orgI
          +D, createdOrg`.                                               
      542 +   - If `createdOrg`, set `orgs.signup_path = 'gh_app'` (stora
          +ge function in `storage/users`).                              
      543 +   - **Self-hosted:** not in launch scope. Return the error pa
          +ge "GitHub sign-in is only available on LiveReview Cloud for n
          +ow." (§8 describes the later design.)                         
      544 +7. For each target:                                           
      545 +   - `CreateInstallationToken`, then `UpsertAppInstallationCon
          +nector{OrgID: orgID, …}`.                                     
      546 +   - `ErrInstallationBoundToOtherOrg` → if this was the explic
          +it `installation_id`, show the error page "This GitHub install
          +ation is already connected to another LiveReview account." In 
          +signin mode, skip that installation and log it.               
      547 +   - For a newly created connector, kick off repo sync **async
          +**: `go s.autoWebhookInstaller.syncRepositoriesAndQueueBackfil
          +l(context.Background(), int(connectorID), details)`. See T3.8 
          +for the App branch inside it. The `Server` field is `autoWebho
          +okInstaller` (`server.go:142`).                               
      548 +8. Redirect `302` to `"/?data=" + url.QueryEscape(slimJSON)`, 
          +where                                                         
      549 +   `slimJSON = {"result":{"jwt":J,"data":{"email":E,"first_nam
          +e":F,"last_name":L,"username":U,"profilePicUrl":P}}}`.        
      550 +   These are exactly the fields `Cloud.tsx` reads (lines 70-85
          +). Don't forward the full Parse user object.                  
      551 +                                                              
      552 +*`GitHubAppWebhook`*:                                         
      553 +1. Read the body (`io.ReadAll`).                              
      554 +2. Verify `X-Hub-Signature-256` = `"sha256=" + hex(HMAC-SHA256
          +(cfg.WebhookSecret, body))` with `hmac.Equal`. Bad or missing 
          +→ 401.                                                        
      555 +3. Parse `{"action":…,"installation":{"id":…}}` and the `X-Git
          +Hub-Event` header.                                            
      556 +4. `installation` event:                                      
      557 +   - `deleted` → `SetAppConnectorFlags(removed=true)`.        
      558 +   - `suspend` → `suspended=true`; `unsuspend` → `suspended=fa
          +lse`.                                                         
      559 +   - `created` → nothing (the callback creates connectors).   
      560 +   - Return 200.                                              
      561 +5. `installation_repositories` event:                         
      562 +   - `added` → upsert those repos via `pullrequests.Store.Upse
          +rtRepository` and queue `QueueRepoPRSyncJob(…, true)`, the sam
          +e as in `syncRepositoriesAndQueueBackfill`.                   
      563 +   - `removed` → delete those `repositories` rows for this con
          +nector (new storage function `DeleteRepositoryByProviderID(con
          +nectorID, providerRepoID)` in `storage/providers/pullrequests`
          +).                                                            
      564 +   - Return 200.                                              
      565 +6. Anything else:                                             
      566 +   - `GetAppConnectorByInstallation`. If there's no row, or it
          +'s removed → **200** `{"status":"ignored"}`, so GitHub doesn't
          + retry.                                                       
      567 +   - Otherwise set the same context that `BuildOrgContextFromC
          +onnector` sets (`internal/api/auth/middleware.go:346-391`). Ex
          +tract that body into an exported `func (am *AuthMiddleware) Se
          +tConnectorOrgContext(c echo.Context, connectorID int64) error`
          + and have the middleware call it too, so the code isn't duplic
          +ated.                                                         
      568 +   - Then `c.Request().Body = io.NopCloser(bytes.NewReader(bod
          +y))` (**required**: the orchestrator reads the body again) and
          + `return s.webhookOrchestratorV2.ProcessWebhookEvent(c)`.     
      569 +                                                              
      570 +*`getUIConfig`* (`server.go:3047`): add `"githubAppEnabled": s
          +.githubApp != nil` and `"githubAppBaseUrl": <cfg.BaseURL or ""
          +>`.                                                           
      571 +                                                              
      572 +*Demo endpoint*: register `GET /api/v1/github-app/demo` in a g
          +roup with the full chain `RequireAuth → BuildOrgContextFromHea
          +der → ValidateOrgAccess → BuildPermissionContext`. Copy how `c
          +onnectorGroup` is built at `server.go:~1500-1508`.            
      573 +- It reads the org from context, `ListAppConnectorsForOrg`, th
          +en **live** `ListInstallationRepositories` (first page, up to 
          +100) and `ListOpenPulls` for up to 5 most recently updated rep
          +os × 5 PRs, using the resolver token.                         
      574 +- Live calls are used because the background sync may still be
          + running right after install.                                 
      575 +- Response:                                                   
      576 +```json                                                       
      577 +{                                                             
      578 +  "connected": true,                                          
      579 +  "installations": [{"connector_id": 12, "account_login": "acm
          +e", "account_type": "Organization",                           
      580 +                     "repository_selection": "selected", "sett
          +ings_url": "https://github.com/organizations/acme/settings/ins
          +tallations/51234567"}],                                       
      581 +  "repositories": [{"full_name": "acme/api", "web_url": "https
          +://github.com/acme/api", "private": true}],                   
      582 +  "open_pull_requests": [{"repo_full_name": "acme/api", "numbe
          +r": 42, "title": "Fix login", "web_url": "https://github.com/a
          +cme/api/pull/42",                                             
      583 +                          "author": "octocat", "updated_at": "
          +2026-10-19T10:00:00Z"}]                                       
      584 +}                                                             
      585 +```                                                           
      586 +- `connected:false` (other arrays empty) when the org has no A
          +pp connector. The UI uses that to show "Connect GitHub".      
      587 +                                                              
      588 +**Tests** `internal/api/github_app_auth_test.go`. Stub GitHub 
          +with `httptest` by pointing `cfg.BaseURL` at it, and stub Pars
          +e the same way via `FW_PARSE_BASE_URL`:                       
      589 +- the state cookie is missing or doesn't match → error page, n
          +o DB writes;                                                  
      590 +- expired state → rejected;                                   
      591 +- `installation_id` not in `/user/installations` → rejected;  
      592 +- no verified email → rejected;                               
      593 +- installation bound to org A, callback for a user of org B → 
          +rejected, row still belongs to A;                             
      594 +- happy path → connector row has `token_type='GITHUB_APP'`, `p
          +at_token` set, the redirect starts with `/?data=`, and the dat
          +a JSON has `result.jwt`;                                      
      595 +- webhook with a bad signature → 401; unknown installation → 2
          +00 ignored; `installation.deleted` → `metadata.removed=true` a
          +nd `pat_token=''`.                                            
      596 +                                                              
      597 +**T3.7 fw-parse client + shared EnsureCloudUser core.**       
      598 +- `network/hexmos/parse_client.go`:                           
      599 +  - Move `getParseAppID` from `network/email/invitation.go:28`
          + here as `ParseAppID()`, and make `invitation.go` call it.    
      600 +  - Add:                                                      
      601 +```go                                                         
      602 +type ParseSignupResult struct {                               
      603 +  JWT  string                                                 
      604 +  Data struct { Email, FirstName, LastName, Username, ProfileP
          +icURL string } // json: email, first_name, last_name, username
          +, profilePicUrl                                               
      605 +  Raw  json.RawMessage                                        
      606 +}                                                             
      607 +func ExternalProviderSignup(ctx context.Context, provider, acc
          +essToken, app string) (*ParseSignupResult, error)             
      608 +  // POST {FW_PARSE_BASE_URL}/parse/functions/externalProvider
          +Signup                                                        
      609 +  // headers as invitation.go:91-95 (Content-Type, X-Parse-App
          +lication-Id, X-Internal-Admin-Secret if set)                  
      610 +  // body {"externalServiceProvider":provider,"accessToken":ac
          +cessToken,"client_details":{"application":app,"deviceUniqueId"
          +:"default"}}                                                  
      611 +  // TRAP: Parse answers HTTP 200 {"result":{"success":false,"
          +message":…}} on failure. Error unless result.jwt is non-empty.
      612 +```                                                           
      613 +  - Add it to `network/network_status.md`.                    
      614 +- `internal/api/auth/ensure_cloud_user.go`: split `EnsureCloud
          +User` (`internal/api/auth/handlers.go:628-866`):              
      615 +```go                                                         
      616 +type EnsureCloudUserInput struct { JWT, Email, FirstName, Last
          +Name, Source string }                                         
      617 +type EnsureCloudUserResult struct { UserID, OrgID int64; Creat
          +edUser, CreatedOrg, AssignedSuperAdmin bool }                 
      618 +type EnsureCloudUserError struct { Status int; Msg string } //
          + carries today's exact HTTP status + message                  
      619 +func (h *AuthHandlers) EnsureCloudUserCore(ctx context.Context
          +, in EnsureCloudUserInput) (*EnsureCloudUserResult, error)    
      620 +```                                                           
      621 +  - The core does what lines 637-812 do today:                
      622 +    - JWT validation with `CLOUD_JWT_SECRET`;                 
      623 +    - the email-match check;                                  
      624 +    - the transaction (find/create user, find/create personal 
          +org + `prompt_application_context`, default org, onboarding AP
          +I key, owner role);                                           
      625 +    - the Discord notify on `CreatedUser`.                    
      626 +  - The HTTP handler parses the header and body, calls the cor
          +e, then does what it does today after commit (`CreateTokenPair
          +`, `getUserOrganizations`, the same JSON). **The response shap
          +e must stay byte-for-byte compatible.** `Cloud.tsx` depends on
          + it.                                                          
      627 +  - Move the SQL into `storage/users/cloud_user_store.go` with
          + the same queries (`*sql.Tx` methods). Keep the queries verbat
          +im; this is a move, not a rewrite. Update `storage/storage_sta
          +tus.md`.                                                      
      628 +  - Test: the existing behavior via the handler (same JSON key
          +s), and calling the core twice is idempotent (second call: all
          + `Created*` false).                                           
      629 +                                                              
      630 +**T3.8 Keep existing screens and jobs safe with App connectors
          +.**                                                           
      631 +- `internal/api/auto_webhook_installer.go`:                   
      632 +  - `getConnectorDetails` (line 252): select `COALESCE(token_t
          +ype,'')` into a new `TokenType` field.                        
      633 +  - `shouldAutoInstall`: return `false` for `GITHUB_APP`. App 
          +webhooks are global; never create per-repo hooks.             
      634 +  - `syncRepositoriesAndQueueBackfill` (line 114): in the gith
          +ub case, when `connector.TokenType == "GITHUB_APP"`, call a ne
          +w `github.ListInstallationRepositoriesPage(ctx, baseURL, token
          +, cursor)` in `internal/providers/github/repo_list.go`. It wra
          +ps `network…ListInstallationRepositories` and returns the same
          + `*providers.RepositoryPage`. Get `token` from `githubapp.Conn
          +ectorToken`.                                                  
      635 +- `EnableManualTriggerForAllProjects` / `DisableManualTriggerF
          +orAllProjects` (`server.go:2837+`): if the connector is `GITHU
          +B_APP`, return 400 `{"error":"This connection is managed by th
          +e LiveReview GitHub App. Manage repositories on GitHub."}`.   
      636 +- `GetRepositoryAccess` (`internal/api/repository_access.go:32
          +9`): for `GITHUB_APP`, build the response from the `repositori
          +es` table rows for that connector instead of calling `/user/re
          +pos`.                                                         
      637 +- Connectors list API (`internal/api/connectors.go`): include 
          +`token_type` in the response if it isn't already there.       
      638 +- `ui/src/pages/GitProviders/*`: for `token_type === 'GITHUB_A
          +PP'`, render the card read-only:                              
      639 +  - "GitHub App · <login>";                                   
      640 +  - a "Manage on GitHub" link;                                
      641 +  - hide the PAT edit and manual-trigger buttons.             
      642 +- **Overlap rule (no double reviews):** in `ProcessWebhookEven
          +t`, right after the T2.3 stamping, when the connector is a PAT
          + GitHub connector and the same org has a `GITHUB_APP` connecto
          +r with a `repositories` row of the same `full_name`, return 20
          +0 `{"status":"ignored","reason":"superseded_by_github_app"}`. 
          +Add a storage query `HasAppConnectorForRepo(orgID, fullName) (
          +bool, error)`.                                                
      643 +- **Manual trigger picks the right connector:** in `prepareAut
          +hentication`, before `findIntegrationToken`, for GitHub URLs p
          +arse `owner/repo` and call a new storage query:               
      644 +```sql                                                        
      645 +SELECT it.id FROM integration_tokens it                       
      646 +JOIN repositories r ON r.connector_id = it.id                 
      647 +WHERE it.org_id = $1 AND it.provider LIKE 'github%' AND r.full
          +_name = $2                                                    
      648 +ORDER BY (it.token_type = 'GITHUB_APP') DESC, it.created_at DE
          +SC LIMIT 1                                                    
      649 +```                                                           
      650 +  If a row is found, use `s.loadIntegrationTokenByID(id)` (rev
          +iews_api.go:131); otherwise keep the current `findIntegrationT
          +oken` (unchanged behavior for orgs without a repositories row)
          +.                                                             
      651 +                                                              
      652 +**T3.9 Error page.** A tiny server-rendered HTML page (inline 
          +template in `github_app_auth.go`):                            
      653 +- the LiveReview name, the message, a "Try again" link to `/#/
          +gh-login`, and "Other ways to sign in" linking to `/`;        
      654 +- no external assets;                                         
      655 +- HTTP 400 for user errors, 502 when GitHub or Parse is unreac
          +hable.                                                        
      656 +                                                              
      657 +**Done when:**                                                
      658 +- all the T3 tests pass;                                      
      659 +- `go build ./...` passes;                                    
      660 +- `make check-status-doc` passes;                             
      661 +- with `GITHUB_APP_ID` unset, none of the new routes are regis
          +tered (test: `GET /api/v1/auth/github-app/start` → 404), and `
          +ui-config.githubAppEnabled=false`.                            
      662 +                                                              
      663 +---                                                           
      664 +                                                              
      665 +### Phase 4: LiveReview UI (PR D)                             
      666 +                                                              
      667 +**T4.1 Entry from a real path.** `ui/src/index.tsx`, at the to
          +p before React renders:                                       
      668 +```ts                                                         
      669 +// /gh-login is a shareable path; the app is a HashRouter, so 
          +move it into the hash.                                        
      670 +if (window.location.pathname === '/gh-login' || window.locatio
          +n.pathname === '/gh-login/') {                                
      671 +    window.location.replace('/#/gh-login' + window.location.se
          +arch);                                                        
      672 +}                                                             
      673 +```                                                           
      674 +The Go UI server (`cmd/ui.go:374`) and the webpack dev server 
          +(`historyApiFallback: true`) both already serve `index.html` f
          +or `/gh-login`.                                               
      675 +                                                              
      676 +**T4.2 `ui/src/App.tsx`.**                                    
      677 +- In the mount-capture effect (~line 126), add `!hashPath.star
          +tsWith('#/gh-login')` to `isProtectedRoute`, so `#/gh-login` i
          +s never stored as the post-login redirect.                    
      678 +- In the unauthenticated branch (~line 288), before the `isClo
          +udMode() && /admin` check:                                    
      679 +```tsx                                                        
      680 +} else if (!isAuthenticated) {                                
      681 +    if (location.pathname.startsWith('/gh-login')) {          
      682 +        body = <GhLogin />;                                   
      683 +    } else if (isCloudMode() && location.pathname === '/admin'
          +) { …                                                         
      684 +```                                                           
      685 +- Authenticated routes:                                       
      686 +  - `<Route path="/gh-login" element={<Navigate to={'/get-star
          +ted/github' + location.search} replace />} />`;               
      687 +  - `<Route path="/get-started/github" element={<GetStartedGit
          +Hub />} />`.                                                  
      688 +- Lazy-load both pages the way neighbouring pages are loaded. 
      689 +                                                              
      690 +**T4.3 `ui/src/pages/Auth/GhLogin.tsx`** (new).               
      691 +- On mount: `fetch('/api/v1/ui-config')`. If `!githubAppEnable
          +d`, `navigate('/', {replace:true})`, which renders the normal 
          +login.                                                        
      692 +- Read `src` and `mr` from `useLocation().search`. Validate `m
          +r` against `^<githubAppBaseUrl>/[^/]+/[^/]+/pull/\d+$`; drop i
          +t if invalid.                                                 
      693 +- Layout (follow `.agents/design.md`): headline "Review your f
          +irst PR in 2 minutes", a "FREE" badge, and one big primary but
          +ton **Continue with GitHub** (GitHub icon). Below it, smaller:
          + "Already installed the app? **Sign in**", then "Other ways to
          + sign in" (link to `/#/`).                                    
      694 +- Click handler:                                              
      695 +```ts                                                         
      696 +sessionStorage.setItem('redirectAfterLogin', '#/get-started/gi
          +thub' + (mr ? `?mr=${encodeURIComponent(mr)}` : ''));         
      697 +window.location.href = `/api/v1/auth/github-app/start?mode=${m
          +ode}&src=${encodeURIComponent(src || 'direct')}`;             
      698 +```                                                           
      699 +- `Cloud.tsx` already turns `redirectAfterLogin` `#/x?y` into 
          +`navigate('/x?y')` (Cloud.tsx ~line 170). No change is needed 
          +there.                                                        
      700 +                                                              
      701 +**T4.4 `ui/src/pages/GetStarted/GetStartedGitHub.tsx`** (new) 
          ++ `ui/src/api/githubApp.ts` (`getGitHubAppDemo()` → `GET /api/
          +v1/github-app/demo` via `apiClient`).                         
      702 +- `connected:false` → a card "Connect your GitHub repos" with 
          +a button. The user is already logged in, and the callback ends
          + in `/?data=`, which only `Cloud.tsx` (logged out) processes, 
          +so the button must:                                           
      703 +  1. dispatch the existing `logout()`;                        
      704 +  2. set `redirectAfterLogin`;                                
      705 +  3. go to `/api/v1/auth/github-app/start?mode=install&src=get
          +-started`.                                                    
      706 +                                                              
      707 +  Note under the button: "You'll be signed in with your GitHub
          + account's email."                                            
      708 +- `connected:true` → a 3-step stepper (use the existing steppe
          +r pattern from `.agents/design.md`):                          
      709 +  1. **Repos connected ✓**: list the `repositories` names (up 
          +to 10, then "+N more"). "Add more repos" opens `installations[
          +0].settings_url` in a new tab.                                
      710 +  2. **AI**: **blocked on open decision 1 (§10).** Until it's 
          +decided: if the org has an AI connector (use the same API `Onb
          +oardingSteps.tsx` uses), show ✓. Otherwise show "Configure AI"
          + linking to `#/ai`.                                           
      711 +  3. **Review a PR**:                                         
      712 +     - A paste box prefilled from `?mr=`, then the `open_pull_
          +requests` list, each with a **Review** button.                
      713 +     - Both call the existing `triggerReview({ url })` from `u
          +i/src/api/reviews.ts:32`, then navigate exactly the way `ui/sr
          +c/pages/Reviews/NewReview.tsx:276` does after a successful tri
          +gger (copy its navigation and error toast handling).          
      714 +     - Show the 403 LOC-quota response the same way NewReview 
          +does.                                                         
      715 +  - Footer link: "Next: catch issues before you commit — insta
          +ll the CLI" → the existing CLI onboarding page/link used on th
          +e dashboard.                                                  
      716 +- Empty `open_pull_requests` → "No open PRs yet. Open one on G
          +itHub, or paste a PR URL above."                              
      717 +                                                              
      718 +**T4.5 Dashboard banner.** On the dashboard, if `getGitHubAppD
          +emo().connected` and the org has no reviews yet (reuse whateve
          +r the dashboard already loads for "first review" state), show 
          +a slim banner: "Finish setup: review your first PR →" linking 
          +to `#/get-started/github`. Fetch the demo endpoint only in clo
          +ud mode.                                                      
      719 +                                                              
      720 +**T4.6 Mega menu.** `ui/src/components/Navbar/megaMenuData.ts`
          +:                                                             
      721 +- Add `hasGitHubApp?: boolean` to `MegaMenuContext`. Populate 
          +it where the context is built (grep `MegaMenuContext` / `isDev
          +Mode:`).                                                      
      722 +- Add a link node `{ kind: 'link', name: 'Get Started (GitHub)
          +', path: '/get-started/github', isVisible: (ctx) => !!ctx.hasG
          +itHubApp }` under the most fitting existing section (next to t
          +he dashboard/onboarding entries).                             
      723 +                                                              
      724 +**T4.7 Routes guide docs** (repo rule). Create `internal/docin
          +dex/docs/routes_guide/auth/gh-login.md` and `internal/docindex
          +/docs/routes_guide/get-started-github.md`, following the shape
          + in `internal/docindex/docs/routes_guide/README.md`: Route(s),
          + Purpose, Who can access it, Key actions, Related pages.      
      725 +                                                              
      726 +**Done when:**                                                
      727 +- `cd ui && npx tsc --noEmit` is clean.                       
      728 +- A manual run on manual-talent3 completes the E2E in §11, ste
          +p 5.                                                          
      729 +- **Don't run a production UI build without asking.**         
      730 +                                                              
      731 +---                                                           
      732 +                                                              
      733 +### Phase 5: hexmoshomepage (PR E)                            
      734 +                                                              
      735 +**T5.1** `urls.js`: add `LIVEREVIEW_APP: "https://livereview.h
          +exmos.com"` and export a helper (e.g. in `utils/` next to simi
          +lar helpers):                                                 
      736 +```js                                                         
      737 +export const lrGhLoginUrl = (src, mr) => `${url.LIVEREVIEW_APP
          +}/gh-login?src=${encodeURIComponent(src)}${mr ? `&mr=${encodeU
          +RIComponent(mr)}` : ""}`                                      
      738 +```                                                           
      739 +                                                              
      740 +**T5.2 Point the "Start Free" CTAs at it** (verify each line; 
          +keep pricing and self-hosted buttons as they are):            
      741 +- `pages/livereview/index.tsx`: `handleBuyNow` (line ~528) → `
          +lrGhLoginUrl("home")`. This covers the Start Free buttons at ~
          +562, ~987 and ~2467 that call it. Check each.                 
      742 +- `components/livereview/StartFreeModal.tsx:146` `handleStartF
          +reeClick` → `lrGhLoginUrl("start-free-modal")` (the "Try Cloud
          +" path only).                                                 
      743 +- `components/livereview/PricingSection.tsx:~181`: **free plan
          + button only** → `lrGhLoginUrl("pricing-free")`.              
      744 +- `pages/livereview/blog/index.tsx:~153`, `pages/livereview/bl
          +og/[slug].tsx:~174`, `pages/livereview/why-livereview.tsx:~222
          +,~384` → `lrGhLoginUrl("<page>")`.                            
      745 +                                                              
      746 +**T5.3 (optional, after launch-critical work)** Hero input "Pa
          +ste a GitHub PR URL → review it free", modelled on LiveAPI's `
          +RepoInput` (`pages/liveapi/index.tsx:129`). Validate `^https:/
          +/github\.com/[^/]+/[^/]+/pull/\d+`, then `lrGhLoginUrl("hero",
          + prUrl)`.                                                     
      747 +                                                              
      748 +---                                                           
      749 +                                                              
      750 +## 7. Known traps (read before coding)                        
      751 +                                                              
      752 +1. `GET /user` with an installation token → 403. Use `<slug>[b
          +ot]` (T3.5).                                                  
      753 +2. GitHub's OAuth code exchange returns **HTTP 200 with `{"err
          +or":…}`** on failure (T3.2).                                  
      754 +3. fw-parse returns **HTTP 200 with `{"result":{"success":fals
          +e}}`** on failure (T3.7).                                     
      755 +4. `connector_id` in event metadata becomes **float64** after 
          +the job-queue JSON round trip (T2.4).                         
      756 +5. The orchestrator stamps `connector_id` **after** `getBotUse
          +rInfo` today; it must move earlier (T2.3).                    
      757 +6. The App webhook handler must **restore the request body** b
          +efore calling `ProcessWebhookEvent` (T3.6).                   
      758 +7. The state cookie must be **SameSite=Lax**; Strict drops it 
          +on the GitHub → LiveReview redirect.                          
      759 +8. `setup_action=request` has no `installation_id` (an org mem
          +ber asked an owner) (T3.6 step 1).                            
      760 +9. A user who is logged in can't finish via `/?data=`. `Cloud.
          +tsx` only runs when logged out (T4.4).                        
      761 +10. `#/gh-login` must be excluded from the mount-time `redirec
          +tAfterLogin` capture (T4.2).                                  
      762 +11. The private key in env often has a literal `\n` in it (T3.
          +1).                                                           
      763 +12. Installation tokens last 1 hour. A review job that sits in
          + the queue for over ~55 min would carry a stale token in its p
          +rovider config. Accepted for launch; the refresher keeps the D
          +B fresh for anything that re-reads it.                        
      764 +13. Existing GitHub code hard-codes `https://api.github.com` i
          +n many places (e.g. `github_token_helpers.go:51`, `unified_pro
          +cessor_v2.go:744`, `lrc_fetch.go:34`). That's fine for cloud a
          +nd github.com. GHES + App needs those switched to `APIBaseURL(
          +token.ProviderURL)`. **Out of launch scope; don't refactor the
          +m now.** New code must not add more.                          
      765 +                                                              
      766 +---                                                           
      767 +                                                              
      768 +## 8. Self-hosted (design only; not in launch scope)          
      769 +                                                              
      770 +Self-hosted has no fw-parse. Its sessions are the local `token
          +Service.CreateTokenPair`.                                     
      771 +                                                              
      772 +When this is picked up later, the callback's identity step for
          + `!IsCloud` will be:                                          
      773 +1. Find the local user by verified email. By default only exis
          +ting users can sign in: admins create users today. A `GITHUB_A
          +PP_ALLOW_SIGNUP=true` flag can come later.                    
      774 +2. If no admin exists yet, redirect to the setup screen.      
      775 +3. Upsert the connector for the user's default org.           
      776 +4. Create a one-time code in a new table `gh_login_codes(code_
          +hash text primary key, user_id bigint, expires_at timestamptz,
          + used_at timestamptz)` with a 60-second TTL and single use.   
      777 +5. 302 to `/#/gh-login/complete?code=…`.                      
      778 +6. `POST /api/v1/auth/github-app/exchange` returns the same `L
          +oginResponse` as `/auth/login`. The UI calls `handleLoginSucce
          +ss`.                                                          
      779 +                                                              
      780 +The config (T3.1, base URL, GHES install path) is already self
          +-hosted-ready.                                                
      781 +                                                              
      782 +---                                                           
      783 +                                                              
      784 +## 9. PR order                                                
      785 +                                                              
      786 +| PR | Repo | Tasks | Can ship alone? |                       
      787 +|---|---|---|---|                                             
      788 +| A | fw-parse | T1.1–T1.5 | Yes (adds a provider, no behavior
          + change for others) |                                         
      789 +| B | LiveReview | T2.1–T2.5 | **Yes, ship first.** Fixes the 
          +existing cross-tenant bug |                                   
      790 +| C | LiveReview | T3.1–T3.9 | Yes (feature off unless `GITHUB
          +_APP_ID` is set) |                                            
      791 +| D | LiveReview UI | T4.1–T4.7 | After C |                   
      792 +| E | hexmoshomepage | T5.1–T5.3 | Only after C+D are live in 
          +prod with the App URLs switched (T0.4) |                      
      793 +                                                              
      794 +---                                                           
      795 +                                                              
      796 +## 10. Open decisions (Shrijith), with the implementer's defau
          +lt until answered                                             
      797 +                                                              
      798 +1. **AI for the demo.** The "FREE" promise needs one of:      
      799 +   - (a) auto-provision the managed "LiveReview AI Model" for 
          +`gh_app` orgs, using the same insert as `ConfirmPurchase` (`in
          +ternal/license/payment/subscription_service.go:1603`), with a 
          +small LOC cap;                                                
      800 +   - (b) an inline "paste a Gemini key" field.                
      801 +                                                              
      802 +   Today `OnboardingSteps.tsx:~93` sends free-plan users to *u
          +pgrade* when they click Configure AI. *Default until decided:*
          + T4.4 step 2 links to `#/ai`. **This is a launch blocker, not 
          +an implementation blocker.**                                  
      803 +2. **Demo limits:** 7 days and/or 1 repo, or just the LOC quot
          +a? *Default:* the existing LOC quota only.                    
      804 +3. **Auto-review new PRs:** *Default:* off. Reviews come from 
          +the "Review" button and the existing comment and mention trigg
          +ers. No auto-review on PR open.                               
      805 +                                                              
      806 +---                                                           
      807 +                                                              
      808 +## 11. Verification                                           
      809 +                                                              
      810 +1. Set up per T0.2 and run `make develop`, `cd ui && make run`
          +, `make niceurl3`.                                            
      811 +2. fw-parse: T1.5 passes. A manual `curl` returns a JWT.      
      812 +3. Go:                                                        
      813 +   - `go test -count=1 ./internal/githubapp/ ./network/provide
          +rs/github/ -run 'App|Config'`                                 
      814 +   - `go test -count=1 ./internal/provider_input/github/`     
      815 +   - `go test -count=1 ./internal/api/ -run 'GitHubApp|EnsureC
          +loudUser|WebhookOrchestrator'`                                
      816 +4. Tenant isolation:                                          
      817 +   - Two orgs (App + PAT, and PAT + PAT). A webhook for org A 
          +only ever loads org A's connector.                            
      818 +   - `grep -rn "GetLatestGitHubToken" --include=*.go .` is emp
          +ty.                                                           
      819 +5. Manual E2E (private window, on `https://manual-talent3.apps
          +.hexmos.com`):                                                
      820 +   1. `/gh-login?src=test&mr=<a PR URL in your test repo>`, th
          +en Continue with GitHub.                                      
      821 +   2. Install on **one** repo. You should land on `#/get-start
          +ed/github` with the repo listed and the PR prefilled.         
      822 +   3. Click **Review** → the review page opens → the bot comme
          +nt appears on the PR as `<slug>[bot]`.                        
      823 +   4. The Parse `_User` row exists with the GitHub email, and 
          +there's a `JWT` row for application `livereview`.             
      824 +   5. Log out, then sign in through **"Other ways to sign in" 
          +→ Google** with the same email. You must land in the **same** 
          +LiveReview user and org, and the App connector is still there.
      825 +   6. "Already installed? Sign in" works for a returning user.
      826 +   7. Comment `@<slug> …` on the PR → the bot replies, and it'
          +s signed by the App, not a PAT.                               
      827 +   8. In GitHub settings, remove the repo from the installatio
          +n, then uninstall. The `repositories` row goes away and the co
          +nnector shows as removed.                                     
      828 +   9. The Git Providers page shows the App connector as read-o
          +nly and nothing on that page errors.                          
      829 +6. Config off: unset `GITHUB_APP_ID` → `/gh-login` shows the n
          +ormal login, the new routes 404, and `grep -rn "github.com/app
          +s\|GITHUB_APP" ui/src` shows nothing hard-coded.              
      830 +7. `make check-status-doc`, and `cd ui && npx tsc --noEmit`. N
          +o production UI build without asking.                         
      831 +                                                              
      832 +---                                                           
      833 +                                                              
      834 +## 12. What does NOT change                                   
      835 +                                                              
      836 +- **onelogin-node:** no change.                               
      837 +- **`Cloud.tsx` / `SelfHosted.tsx` login behavior:** no change
          +. `Cloud.tsx` simply also receives `/?data=` from the new call
          +back.                                                         
      838 +- **The Git Providers PAT flow** for every provider: unchanged
          +, except the cross-tenant fix (PR B) and the read-only App car
          +d (T3.8).                                                     
      839 +- **Self-hosted without `GITHUB_APP_*`:** the feature is invis
          +ible.                                                         

  Ran 1 shell command (ctrl+o to expand)

● I've rewritten LiveReview/docs/github-app-onboarding-plan.md (839 lines) so
  Sonnet can work through it task by task; the copy in ~/.claude/plans/ is
  synced. No code has changed.

  How it's laid out
  - Rules for the implementer up front: the storage/network boundary and status
    docs, no fallbacks, no full UI build without asking, and which files not to
    touch.
  - A glossary of the tokens and IDs: the App JWT, installation token, user
    token, installation ID, Hexmos JWT and LiveReview tokens, saying where each
    one is stored. Mixing these up is the most likely bug.
  - Numbered tasks (fw-parse, cross-tenant fix, App backend, UI, homepage). Each
    gives the files with line numbers, function signatures and a short code
    sketch, the tests to write, and a "Done when" check.
  - The exact migration SQL, and what goes in each column of the connector row.
  - Five PRs in order. The cross-tenant fix is its own PR and should ship first,
    since it fixes a bug that exists today.
  - Defaults for the three open decisions, so Sonnet isn't blocked. AI for the
    demo is still marked as a launch blocker.

  Things I found in the code that the earlier plan got wrong or missed
  1. fw-parse: the middleware needs no change; github_app already skips the code
     exchange. But externalProviderSignin.js has its own list of providers and
     would answer "Unknown provider" without a one-line addition.
  2. Installation tokens can't call GET /user, which GitHub returns 403 for. The
     bot lookup on the webhook path calls it, so App connectors need the
     <slug>[bot] identity instead.
  3. The webhook handler only records the connector ID after it has already 
     looked up the bot. That step has to move earlier for the tenant fix to
     work.
  4. After a job-queue round trip, the connector ID comes back as a float, not
     an integer. The plan's helper handles this.
  5. Both GitHub's code exchange and fw-parse report failures as HTTP 200 with
     an error inside the body. The plan has explicit checks for both.
  6. About 15 existing places read the token by connector ID. Storing the
     current installation token in pat_token, plus a refresh job every 10
     minutes, keeps all of them working without rewriting each one.
  7. An App connector would break existing screens and jobs:
     - the Git Providers page;
     - automatic webhook install;
     - repo discovery, which calls /user/repos;
     - the enable-manual-trigger button.

     A new task makes these safe, with a read-only card on the Git Providers
     page.
  8. The UI shows the login screen for every URL when logged out, so #/gh-login
     can't be an ordinary route; App.tsx has to handle it directly. Also, a user
     who's already logged in can't finish via /?data=, because only the
     logged-out login screen reads it. The plan has them sign out first.

  Self-hosted is now a design-only section at the end, out of launch scope as
  Shrijith said. The config remains ready for custom domains and GitHub
  Enterprise.

✻ Worked for 10m 55s · done 5:02 pm

❯ /model                                                                        
  ⎿  Set model to Sonnet 5.5 and saved as your default for new sessions

❯ the livereview servers are running backend and frontend have auto refresh.    
  hexmoshomepage also running it also has auto refresh. fw-parse .env is there  
  you can start on your own. manual-talent thing also running                   

● Starting with PR A (fw-parse), then PR B. The GitHub App itself (T0.1) needs a
  person in GitHub settings, so I'll build everything that doesn't depend on it
  first.

  Ran 1 shell command (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      import re…)
  ⎿   cloud/oneLogin/externalProviderSignin.js |  2 ++
      cloud/oneLogin/githubOneLogin.js         | 25 +++++++++++++++++++++++
     ++
     … +17 lines (ctrl+o to expand)
  ⎿  Updated ~/hex/fw-parse/cloud/oneLogin/githubOneLogin.js (+25 -0)
      48        return null
      49      }
      50    },
      51 +                                                               
      52 +  // For the LiveReview GitHub App sign-in: only a primary *ver
         +ified* email is accepted, and any                              
      53 +  // failure throws (unlike getGithubUserData) so the caller ca
         +n't continue with an empty email.                              
      54 +  getGithubAppUserData: async function (accessToken) {         
      55 +    const headers = {                                          
      56 +      Accept: "application/vnd.github+json",                   
      57 +      Authorization: `Bearer ${accessToken}`,                  
      58 +      "X-GitHub-Api-Version": "2022-11-28",                    
      59 +    }                                                          
      60 +    const profileRes = await fetch(GITHUB_USER_API_URL, { metho
         +d: "GET", headers })                                           
      61 +    if (!profileRes.ok) {                                      
      62 +      throw new Error(`GitHub /user failed: ${profileRes.status
         +} ${profileRes.statusText}`)                                   
      63 +    }                                                          
      64 +    const profile = await profileRes.json()                    
      65 +    const emailsRes = await fetch(GITHUB_EMAIL_API_URL, { metho
         +d: "GET", headers })                                           
      66 +    if (!emailsRes.ok) {                                       
      67 +      throw new Error(`GitHub /user/emails failed: ${emailsRes.
         +status} ${emailsRes.statusText}`)                              
      68 +    }                                                          
      69 +    const emails = await emailsRes.json()                      
      70 +    const primary = Array.isArray(emails) ? emails.find((e) => 
         +e.primary === true && e.verified === true) : null              
      71 +    if (!primary) {                                            
      72 +      throw new Error("GitHub account has no verified primary e
         +mail")                                                         
      73 +    }                                                          
      74 +    return { email: primary.email.toLowerCase(), name: profile.
         +name || profile.login, picture: profile.avatar_url }           
      75 +  },                                                           
      76  }
  ⎿  Updated ~/hex/fw-parse/cloud/oneLogin/providerMethods.js (+3 -1)
      1  const { getGoogleUserData, generateGoogleTokenPayload, insertToG
         oogleTokens } = require("cloud/oneLogin/googleOneLogin")
      2 -const { getGithubUserData } = require("cloud/oneLogin/githubOneL
        -ogin")                                                          
      2 +const { getGithubUserData, getGithubAppUserData } = require("clo
        +ud/oneLogin/githubOneLogin")                                    
      3  const { getLinkedInUserData } = require("cloud/oneLogin/linkedIn
         Login")
      4  const { exchangeAuthentikCodeForToken, getUserInfo } = require("
         cloud/oneLogin/authentikOneLogin")
      5  const { publicUrls } = require("cloud/utils/publicUrlConstants")
     ...
      30    } else if (provider == "github") {
      31      simpleUserDate = await getGithubUserData(accessToken)
      32      console.log("getGithubUserData = ", simpleUserDate)
      33 +  } else if (provider == "github_app") {                       
      34 +    simpleUserDate = await getGithubAppUserData(accessToken)   
      35    } else if (provider == "linkedin") {
      36      simpleUserDate = await getLinkedInUserData(accessToken)
      37    } else if (provider == "authentik") {
  ⎿  Updated ~/hex/fw-parse/cloud/oneLogin/externalProviderSignin.js (+2 
     -0)
       9        return await externalMethods(params)
      10      } else if (externalServiceProvider === "github") {
      11        return await externalMethods(params)
      12 +    } else if (externalServiceProvider === "github_app") {     
      13 +      return await externalMethods(params)                     
      14      } else if (externalServiceProvider === "linkedin") {
      15        return await externalMethods(params)
      16      } else if (externalServiceProvider === "authentik") {
  ⎿  Updated ~/hex/fw-parse/isAuthenticatedMiddleware.js (+1 -0)
      143    } else if (functionName === publicUrls.EXTERNAL_PROVIDER_SIG
           NUP) {
      144      console.log("===> Inside External provider signup")
      145  
      146 +    // "github_app" (LiveReview GitHub App) sends an already-e
          +xchanged user token, so no code exchange here.                
      147      if (request.body.externalServiceProvider == "github") {
      148        const url = `${GITHUB_ACCESS_TOKEN_URL}?client_id=${GITH
           UB_CLIENT_ID}&client_secret=${GITHUB_SECRET_ID}&code=${request
           .body.accessToken}`
      149  

● Bash(mkdir -p __tests__/oneLogin && cat >
      __tests__/oneLogin/githubAppUserData.test.js <<'EOF'
      // Unit test for the LiveReview GitHub App sign-in lookup. Stubs globa…)
  ⎿  PASS fw-parse-test __tests__/oneLogin/githubAppUserData.test.js
       getGithubAppUserData
         ✓ returns the lowercased primary verified email (3 ms)
     … +10 lines (ctrl+o to expand)
  ⎿  Created ~/hex/fw-parse/__tests__/oneLogin/githubAppUserData.test.js
     (+41 -0)
       1 +// Unit test for the LiveReview GitHub App sign-in lookup. Stub
         +s global.fetch (captured at require time).                     
       2 +const respond = (body, ok = true, status = 200) => ({ ok, statu
         +s, statusText: ok ? "OK" : "Forbidden", json: async () => body 
         +})                                                             
       3 +                                                               
       4 +describe("getGithubAppUserData", () => {                       
       5 +  let getGithubAppUserData                                     
       6 +  const fetchMock = jest.fn()                                  
       7 +                                                               
       8 +  beforeAll(() => {                                            
       9 +    global.fetch = fetchMock                                   
      10 +    jest.isolateModules(() => {                                
      11 +      ;({ getGithubAppUserData } = require("cloud/oneLogin/gith
         +ubOneLogin"))                                                  
      12 +    })                                                         
      13 +  })                                                           
      14 +  beforeEach(() => fetchMock.mockReset())                      
      15 +                                                               
      16 +  const mockGitHub = (emails, profile = { login: "octo", name: 
         +"Octo Cat", avatar_url: "http://a/p.png" }) => {               
      17 +    fetchMock.mockImplementation(async (url) => (String(url).en
         +dsWith("/user/emails") ? respond(emails) : respond(profile)))  
      18 +  }                                                            
      19 +                                                               
      20 +  test("returns the lowercased primary verified email", async (
         +) => {                                                         
      21 +    mockGitHub([                                               
      22 +      { email: "Other@x.com", primary: false, verified: true },
      23 +      { email: "Octo@Example.com", primary: true, verified: tru
         +e },                                                           
      24 +    ])                                                         
      25 +    await expect(getGithubAppUserData("ghu_x")).resolves.toEqua
         +l({                                                            
      26 +      email: "octo@example.com",                               
      27 +      name: "Octo Cat",                                        
      28 +      picture: "http://a/p.png",                               
      29 +    })                                                         
      30 +  })                                                           
      31 +                                                               
      32 +  test("rejects a primary email that is not verified", async ()
         + => {                                                          
      33 +    mockGitHub([{ email: "octo@example.com", primary: true, ver
         +ified: false }])                                               
      34 +    await expect(getGithubAppUserData("ghu_x")).rejects.toThrow
         +("no verified primary email")                                  
      35 +  })                                                           
      36 +                                                               
      37 +  test("throws when GitHub rejects the token", async () => {   
      38 +    fetchMock.mockResolvedValue(respond({}, false, 403))       
      39 +    await expect(getGithubAppUserData("bad")).rejects.toThrow("
         +GitHub /user failed")                                          
      40 +  })                                                           
     … 1 more lines (ctrl+o to expand)

● PR A is done: the fw-parse unit tests pass. Now PR B, the cross-tenant fix in
  LiveReview.

  Ran 3 shell commands (ctrl+o to expand)

● Bash(cat >> internal/providers/github/base_url.go <<'EOF'…)
  ⎿  36:| providersgitlab.NewAuthTokenStore                      | moved
     | [NewAuthTokenStore](providers/gitlab/auth_token_store.go#L44)
                       |
     … +20 lines (ctrl+o to expand)
  ⎿  Updated internal/providers/github/base_url.go (+3 -0)
      11          }
      12          return strings.TrimSuffix(baseURL, "/") + "/api/v3"
      13  }
      14 +                                                               
      15 +// APIBaseURL is the exported form of apiBaseURL for callers ou
         +tside this package.                                            
      16 +func APIBaseURL(baseURL string) string { return apiBaseURL(base
         +URL) }                                                         
  ⎿  Updated storage/providers/github/token_store.go (+14 -8)
       8          ProviderURL string
       9          PatToken    string
      10          Metadata    []byte
      11 +        OrgID       int64                                      
      12 +        TokenType   string                                     
      13 +        ExpiresAt   sql.NullTime                               
      14  }
      15  
      16  type TokenStore struct {
     ...
      21          return &TokenStore{db: db}
      22  }
      23  
      21 -func (s *TokenStore) GetLatestGitHubToken() (*IntegrationTokenR
         -ecord, error) {                                                
      22 -        query := `                                             
      23 -                SELECT id, provider, provider_url, pat_token, C
         -OALESCE(metadata, '{}')                                        
      24 +// GetGitHubTokenByConnectorID loads one GitHub connector by pr
         +imary key. Callers must already know which                     
      25 +// connector (and so which org) a request belongs to - there is
         + deliberately no "latest token" lookup.                        
      26 +func (s *TokenStore) GetGitHubTokenByConnectorID(connectorID in
         +t64) (*IntegrationTokenRecord, error) {                        
      27 +        const query = `                                        
      28 +                SELECT id, provider, provider_url, COALESCE(pat
         +_token, ''), COALESCE(metadata, '{}'), org_id,                 
      29 +                       COALESCE(token_type, ''), expires_at    
      30                  FROM integration_tokens
      25 -                WHERE provider = 'github'                      
      26 -                  AND (provider_url = 'https://github.com' OR p
         -rovider_url = 'https://api.github.com')                        
      27 -                ORDER BY created_at DESC                       
      28 -                LIMIT 1                                        
      31 +                WHERE id = $1 AND provider LIKE 'github%'      
      32          `
      33  
      34          var rec IntegrationTokenRecord
      32 -        err := s.db.QueryRow(query).Scan(&rec.ID, &rec.Provider
         -, &rec.ProviderURL, &rec.PatToken, &rec.Metadata)              
      35 +        err := s.db.QueryRow(query, connectorID).Scan(         
      36 +                &rec.ID, &rec.Provider, &rec.ProviderURL, &rec.
         +PatToken, &rec.Metadata, &rec.OrgID,                           
      37 +                &rec.TokenType, &rec.ExpiresAt,                
      38 +        )                                                      
      39          if err != nil {
      40                  return nil, err
      41          }

● Bash(python3 - <<'EOF'
      p='storage/storage_status.md'…)
  ⎿  44:| providersgithub.NewTokenStore                          | moved
     | [NewTokenStore](providers/github/token_store.go#L20)
                       |
     … +11 lines (ctrl+o to expand)
  ⎿  Updated storage/storage_status.md (+2 -2)
      41  | providersbitbucket.NewIntegrationTokenStore            | move
          d   | [NewIntegrationTokenStore](providers/bitbucket/integratio
          n_token_store.go#L18)        |
      42  | providersbitbucket.GetTokenByRepoFullName              | move
          d   | [GetTokenByRepoFullName](providers/bitbucket/integration_
          token_store.go#L22)          |
      43  | providersbitbucket.GetLatestBitbucketToken             | move
          d   | [GetLatestBitbucketToken](providers/bitbucket/integration
          _token_store.go#L41)         |
      44 -| providersgithub.NewTokenStore                          | move
         -d   | [NewTokenStore](providers/github/token_store.go#L17)     
         -                             |                                 
      45 -| providersgithub.GetLatestGitHubToken                   | move
         -d   | [GetLatestGitHubToken](providers/github/token_store.go#L2
         -1)                           |                                 
      44 +| providersgithub.NewTokenStore                          | move
         +d   | [NewTokenStore](providers/github/token_store.go#L20)     
         +                             |                                 
      45 +| providersgithub.GetGitHubTokenByConnectorID            | adde
         +d   | [GetGitHubTokenByConnectorID](providers/github/token_stor
         +e.go#L26)                    |                                 
      46  | reviews.NewReviewStore                                 | move
          d   | [NewReviewStore](reviews/review_store.go#L9)             
                                       |
      47  | reviews.QueryRow                                       | move
          d   | [QueryRow](reviews/review_store.go#L13)                  
                                       |
      48  | reviews.Exec                                           | move
          d   | [Exec](reviews/review_store.go#L17)                      
                                       |

● Now the helpers (T2.4) and the provider callers.

● Bash(python3 - <<'EOF'
      p='internal/provider_input/github/github_token_helpers.go'…)
  ⎿          case "pull_request_review":
                     return "pull_request_review"
             case "pull_request":
     … +54 lines (ctrl+o to expand)
  ⎿  Updated internal/provider_input/github/github_token_helpers.go (+48 
     -7)
      17  // IntegrationToken holds GitHub-specific token data needed for
           API calls.
      18  type IntegrationToken struct {
      19          ID          int64
      20 +        OrgID       int64                                      
      21          Provider    string
      22          ProviderURL string
      23 +        TokenType   string                                     
      24          PatToken    string
      25          Metadata    map[string]interface{}
      26  }
      27  
      26 -// FindIntegrationTokenForGitHubRepo looks up a GitHub integrat
         -ion token.                                                     
      27 -func FindIntegrationTokenForGitHubRepo(db *sql.DB, repoFullName
         - string) (*IntegrationToken, error) {                          
      28 +// connectorIDFromMetadata reads connector_id stamped by the we
         +bhook orchestrator. After a JSON                               
      29 +// round trip through the job queue it arrives as float64, so a
         +ccept the numeric forms.                                       
      30 +func connectorIDFromMetadata(meta map[string]interface{}) (int6
         +4, error) {                                                    
      31 +        switch v := meta["connector_id"].(type) {              
      32 +        case int64:                                            
      33 +                return v, nil                                  
      34 +        case int:                                              
      35 +                return int64(v), nil                           
      36 +        case float64:                                          
      37 +                return int64(v), nil                           
      38 +        case json.Number:                                      
      39 +                return v.Int64()                               
      40 +        }                                                      
      41 +        return 0, fmt.Errorf("connector_id missing from event m
         +etadata")                                                      
      42 +}                                                              
      43 +                                                               
      44 +type connectorCtxKey struct{}                                  
      45 +                                                               
      46 +// WithConnectorID carries the receiving connector through a co
         +ntext, for provider interfaces                                 
      47 +// (e.g. lrcfetch.Provider) whose signatures have no event to r
         +ead it from.                                                   
      48 +func WithConnectorID(ctx context.Context, id int64) context.Con
         +text {                                                         
      49 +        return context.WithValue(ctx, connectorCtxKey{}, id)   
      50 +}                                                              
      51 +                                                               
      52 +func connectorIDFromContext(ctx context.Context) (int64, bool) 
         +{                                                              
      53 +        id, ok := ctx.Value(connectorCtxKey{}).(int64)         
      54 +        return id, ok                                          
     … 42 more lines (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      import re…)
  ⎿  // FetchMRTimeline fetches timeline data for a merge request.
     func (p *GitHubV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (
     *UnifiedTimelineV2, error) {
     … +11 lines (ctrl+o to expand)
  ⎿  Updated internal/provider_input/github/github_token_helpers.go (+14 
     -0)
      78          return token, nil
      79  }
      80  
      81 +// FindIntegrationTokenForEvent resolves the connector stamped 
         +on a webhook event by the orchestrator.                        
      82 +func FindIntegrationTokenForEvent(db *sql.DB, event *UnifiedWeb
         +hookEventV2) (*IntegrationToken, error) {                      
      83 +        if event == nil {                                      
      84 +                return nil, fmt.Errorf("nil event")            
      85 +        }                                                      
      86 +        if _, err := connectorIDFromMetadata(event.Repository.M
         +etadata); err == nil {                                         
      87 +                return findIntegrationTokenForMetadata(db, even
         +t.Repository.Metadata)                                         
      88 +        }                                                      
      89 +        if event.MergeRequest != nil {                         
      90 +                return findIntegrationTokenForMetadata(db, even
         +t.MergeRequest.Metadata)                                       
      91 +        }                                                      
      92 +        return nil, fmt.Errorf("connector_id missing from event
         + metadata")                                                    
      93 +}                                                              
      94 +                                                               
      95  // findIntegrationTokenForMetadata resolves the connector stamp
          ed into an event's metadata.
      96  func findIntegrationTokenForMetadata(db *sql.DB, meta map[strin
          g]interface{}) (*IntegrationToken, error) {
      97          connectorID, err := connectorIDFromMetadata(meta)
  ⎿  Updated internal/provider_input/github/github_provider.go (+10 -11)
      142                  return fmt.Errorf("no merge request in event")
      143          }
      144  
      145 -        token, err := FindIntegrationTokenForGitHubRepo(p.db, 
          -event.Repository.FullName)                                    
      145 +        token, err := FindIntegrationTokenForEvent(p.db, event
          +)                                                             
      146          if err != nil {
      147                  return fmt.Errorf("failed to get GitHub token:
            %w", err)
      148          }
     ...
      186          return nil
      187  }
      188  
      189 -// FindIntegrationTokenForRepo returns the integration token a
          -ssociated with the given repository.                          
      190 -func (p *GitHubV2Provider) FindIntegrationTokenForRepo(repoFul
          -lName string) (*IntegrationToken, error) {                    
      189 +// FindIntegrationTokenForEvent returns the integration token 
          +of the connector that received the event.                     
      190 +func (p *GitHubV2Provider) FindIntegrationTokenForEvent(event 
          +*UnifiedWebhookEventV2) (*IntegrationToken, error) {          
      191          if p == nil {
      192                  return nil, fmt.Errorf("github provider not in
           itialised")
      193          }
     ...
      195                  return nil, fmt.Errorf("github provider missin
           g database handle")
      196          }
      197  
      198 -        return FindIntegrationTokenForGitHubRepo(p.db, repoFul
          -lName)                                                        
      198 +        return FindIntegrationTokenForEvent(p.db, event)      
      199  }
      200  
      201  func recordGitHubWebhook(eventType string, headers map[string]
           string, body []byte, unified *UnifiedWebhookEventV2, err error
           ) {
     ...
      255                  return fmt.Errorf("invalid event for comment r
           eply")
      256          }
      257  
      258 -        token, err := FindIntegrationTokenForGitHubRepo(p.db, 
          -event.Repository.FullName)                                    
      258 +        token, err := FindIntegrationTokenForEvent(p.db, event
          +)                                                             
      259          if err != nil {
      260                  return fmt.Errorf("failed to get GitHub token:
            %w", err)
      261          }
     ...
      269                  return fmt.Errorf("no comment in event for emo
           ji reaction")
      270          }
      271  
      272 -        token, err := FindIntegrationTokenForGitHubRepo(p.db, 
          -event.Repository.FullName)                                    
      272 +        token, err := FindIntegrationTokenForEvent(p.db, event
          +)                                                             
      273          if err != nil {
     … 35 more lines (ctrl+o to expand)
  ⎿  Updated internal/provider_input/github/lrc_fetch.go (+5 -1)
      25  // repoFullName is "owner/repo". ref is the branch name (e.g. "
          main").
      26  // Returns (nil, false, nil) when .lrc/ does not exist on the r
          epo.
      27  func (p *GitHubV2Provider) GetRepoConfigFiles(ctx context.Conte
          xt, repoFullName, ref string) (map[string][]byte, bool, error) 
          {
      28 -        token, err := FindIntegrationTokenForGitHubRepo(p.db, r
         -epoFullName)                                                   
      28 +        connectorID, ok := connectorIDFromContext(ctx)         
      29 +        if !ok {                                               
      30 +                return nil, false, fmt.Errorf("github lrc: no c
         +onnector in context for %s", repoFullName)                     
      31 +        }                                                      
      32 +        token, err := FindIntegrationTokenForGitHubConnector(p.
         +db, connectorID)                                               
      33          if err != nil {
      34                  return nil, false, fmt.Errorf("github lrc: no t
          oken for %s: %w", repoFullName, err)
      35          }
  ⎿  Updated internal/api/unified_processor_v2.go (+1 -1)
      732                  return false, fmt.Errorf("missing repository f
           ull name for GitHub reply detection")
      733          }
      734  
      735 -        token, err := p.server.githubProviderV2.FindIntegratio
          -nTokenForRepo(repoFullName)                                   
      735 +        token, err := p.server.githubProviderV2.FindIntegratio
          +nTokenForEvent(&event)                                        
      736          if err != nil || token == nil {
      737                  return false, fmt.Errorf("failed to find GitHu
           b token: %w", err)
      738          }

● Bash(python3 - <<'EOF'
      p='internal/api/webhook_orchestrator_v2.go'…)
  ⎿  164:                    connectorID, ok :=
     auth.GetConnectorIDFromContext(c)
     178:                    if err :=
     wo.server.jobQueue.QueuePRStateSyncJob(c.Request().Conte
     xt(), orgID, connectorID, providerName, *stateEvent); err != nil {
     … +20 lines (ctrl+o to expand)
  ⎿  Updated internal/api/webhook_orchestrator_v2.go (+32 -20)
      211  
      212          log.Printf("[INFO] Unified event created: type=%s, pro
           vider=%s", event.EventType, event.Provider)
      213  
      214 +        // Stamp the receiving connector on the event before a
          +nything looks up a provider token: the                        
      215 +        // bot-info and reply-detection lookups below resolve 
          +their token from this connector.                              
      216 +        connectorID, ok := auth.GetConnectorIDFromContext(c)  
      217 +        if !ok {                                              
      218 +                log.Printf("[ERROR] Connector ID not found in 
          +context - webhook route configuration error")                 
      219 +                return c.JSON(http.StatusInternalServerError, 
          +map[string]string{                                            
      220 +                        "error": "internal_error",            
      221 +                })                                            
      222 +        }                                                     
      223 +        stampConnectorID(event, connectorID)                  
      224 +                                                              
      225          // Phase 3: Response Warrant Check
      226          botInfo, err := wo.getBotUserInfo(event)
      227          if err != nil {
     ...
      261  
      262          log.Printf("[INFO] Response warranted: scenario=%s", s
           cenario.Type)
      263  
      253 -        // Extract org ID and connector ID from context (set b
          -y BuildOrgContextFromConnector middleware)                    
      254 -        connectorID, ok := auth.GetConnectorIDFromContext(c)  
      255 -        if !ok {                                              
      256 -                log.Printf("[ERROR] Connector ID not found in 
          -context - webhook route configuration error")                 
      257 -                return c.JSON(http.StatusInternalServerError, 
          -map[string]string{                                            
      258 -                        "error": "internal_error",            
      259 -                })                                            
      260 -        }                                                     
      264 +        // Extract org ID from context (set by BuildOrgContext
          +FromConnector middleware)                                     
      265          orgID, ok := auth.GetOrgIDFromContext(c)
      266          if !ok {
      267                  log.Printf("[ERROR] Org ID not found in contex
           t - middleware configuration error")
     ...
      286          }
      287  
      288          log.Printf("[DEBUG] Processing webhook for connector_i
           d=%d, org_id=%d", connectorID, orgID)
      285 -        if event.Repository.Metadata == nil {                 
      286 -                event.Repository.Metadata = map[string]interfa
          -ce{}{}                                                        
      289 +        if event.MergeRequest != nil && strings.TrimSpace(even
          +t.Repository.ID) != "" {                                      
      290 +                event.MergeRequest.Metadata["repository_id"] =
          + strings.TrimSpace(event.Repository.ID)                       
      291          }
     … 43 more lines (ctrl+o to expand)

  Searched for 1 pattern (ctrl+o to expand)
● Bash(python3 - <<'EOF'
      p='internal/api/webhook_orchestrator_v2.go'…)
  ⎿  internal/ai/gemini/gemini.go
     internal/ai/langchain/json_repair_integration_test.go
     internal/ai/langchain/provider.go
     … +57 lines (ctrl+o to expand)
  ⎿  Updated internal/api/webhook_orchestrator_v2.go (+8 -0)
      20          "github.com/livereview/internal/lrcconfig"
      21          "github.com/livereview/internal/lrcfetch"
      22          "github.com/livereview/internal/prompts"
      23 +        githubinput "github.com/livereview/internal/provider_in
         +put/github"                                                    
      24          gitlabinput "github.com/livereview/internal/provider_in
          put/gitlab"
      25          "github.com/livereview/internal/prsync"
      26          storagelicense "github.com/livereview/storage/license"
     ...
      1094                  }
      1095          }
      1096  
      1097 +        // For GitHub, inject the receiving connector so the 
           +provider resolves that connector's token.                    
      1098 +        if strings.EqualFold(event.Provider, "github") {     
      1099 +                if id, ok := event.Repository.Metadata["conne
           +ctor_id"].(int64); ok {                                      
      1100 +                        ctx = githubinput.WithConnectorID(ctx
           +, id)                                                        
      1101 +                }                                            
      1102 +        }                                                    
      1103 +                                                             
      1104          lrcFiles, found, err := rcp.GetRepoConfigFiles(ctx, r
            epoFull, ref)
      1105          if err != nil {
      1106                  log.Printf("[WARN] .lrc fetch for webhook %s@
            %s: %v", repoFull, ref, err)

  Ran 1 shell command (ctrl+o to expand)

● The build is clean apart from the existing cmd/test failure. Now the tests
  (T2.5).

  Listed 2 directories, ran 2 shell commands (ctrl+o to expand)

● Bash(cat > internal/provider_input/github/github_token_helpers_test.go <<'EOF'
      package github…)
  ⎿  ok      github.com/livereview/internal/provider_input/github    0.266s
  ⎿  Created internal/provider_input/github/github_token_helpers_test.go
     (+137 -0)
        1 +package github                                                
        2 +                                                              
        3 +import (                                                      
        4 +        "context"                                             
        5 +        "database/sql"                                        
        6 +        "encoding/json"                                       
        7 +        "fmt"                                                 
        8 +        "testing"                                             
        9 +        "time"                                                
       10 +                                                              
       11 +        "github.com/livereview/internal/database"             
       12 +)                                                             
       13 +                                                              
       14 +func TestConnectorIDFromMetadata(t *testing.T) {              
       15 +        cases := []struct {                                   
       16 +                name    string                                
       17 +                meta    map[string]interface{}                
       18 +                want    int64                                 
       19 +                wantErr bool                                  
       20 +        }{                                                    
       21 +                {"int64", map[string]interface{}{"connector_id
          +": int64(7)}, 7, false},                                      
       22 +                {"int", map[string]interface{}{"connector_id":
          + 7}, 7, false},                                               
       23 +                {"float64 after json round trip", map[string]i
          +nterface{}{"connector_id": float64(7)}, 7, false},            
       24 +                {"json.Number", map[string]interface{}{"connec
          +tor_id": json.Number("7")}, 7, false},                        
       25 +                {"missing", map[string]interface{}{}, 0, true}
          +,                                                             
       26 +                {"nil map", nil, 0, true},                    
       27 +                {"wrong type", map[string]interface{}{"connect
          +or_id": "7"}, 0, true},                                       
       28 +        }                                                     
       29 +        for _, tc := range cases {                            
       30 +                t.Run(tc.name, func(t *testing.T) {           
       31 +                        got, err := connectorIDFromMetadata(tc
          +.meta)                                                        
       32 +                        if (err != nil) != tc.wantErr {       
       33 +                                t.Fatalf("err = %v, wantErr %v
          +", err, tc.wantErr)                                           
       34 +                        }                                     
       35 +                        if got != tc.want {                   
       36 +                                t.Fatalf("got %d, want %d", go
          +t, tc.want)                                                   
       37 +                        }                                     
       38 +                })                                            
       39 +        }                                                     
       40 +}                                                             
     … 97 more lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● PR B is done: the tenant tests pass and no callers of the old lookup remain.
  Starting PR C (the App backend) with the migration, config package and network
  layer.

● Bash(cat > db/migrations/20261020090000_github_app_installations.sql <<'EOF'
      -- migrate:up…)
  ⎿  /usr/local/bin/dbmate
     DATABASE_URL=…
     JWT_SECRET=…
     … +11 lines (ctrl+o to expand)
  ⎿  Created db/migrations/20261020090000_github_app_installations.sql (+12
     -0)
       1 +-- migrate:up                                                  
       2 +-- One GitHub App installation can belong to exactly one LiveRe
         +view org.                                                      
       3 +CREATE UNIQUE INDEX IF NOT EXISTS uq_integration_tokens_github_
         +app_installation                                               
       4 +    ON integration_tokens (provider_url, provider_app_id)      
       5 +    WHERE token_type = 'GITHUB_APP';                           
       6 +                                                               
       7 +-- How the org was created; 'gh_app' for orgs created through /
         +gh-login. Analytics + onboarding banner only.                  
       8 +ALTER TABLE orgs ADD COLUMN IF NOT EXISTS signup_path TEXT;    
       9 +                                                               
      10 +-- migrate:down                                                
      11 +DROP INDEX IF EXISTS uq_integration_tokens_github_app_installat
         +ion;                                                           
      12 +ALTER TABLE orgs DROP COLUMN IF EXISTS signup_path;            

● Bash(set -a && . ./.env && set +a && dbmate status 2>&1 | tail -4 && dbmate up
      2>&1 | tail -5; grep -n "signup_path\|uq_integration_tokens_github_app"
      db/schema.sql…)
  ⎿  [ ] 20261020090000_github_app_installations.sql

     Applied: 104
     … +12 lines (ctrl+o to expand)
  ⎿  Updated db/schema.sql (+112 -89)
      1  \restrict dbmate
      2  
      3 --- Dumped from database version 15.17 (Debian 15.17-1.pgdg13+1) 
      4 --- Dumped by pg_dump version 16.13 (Ubuntu 16.13-0ubuntu0.24.04.
        -1)                                                              
      3 +-- Dumped from database version 16.14 (Ubuntu 16.14-0ubuntu0.24.
        +04.1)                                                           
      4 +-- Dumped by pg_dump version 16.14 (Ubuntu 16.14-0ubuntu0.24.04.
        +1)                                                              
      5  
      6  SET statement_timeout = 0;
      7  SET lock_timeout = 0;
     ...
      636      tier character varying(32) NOT NULL,
      637      math_mode jsonb NOT NULL,
      638      created_at timestamp with time zone DEFAULT now() NOT NULL
           ,
      639 -    CONSTRAINT blast_radius_hunks_tier_check CHECK (((tier)::t
          -ext = ANY ((ARRAY['blast-radius-none'::character varying, 'bla
          -st-radius-low'::character varying, 'blast-radius-medium'::char
          -acter varying, 'blast-radius-high'::character varying])::text[
          -])))                                                          
      639 +    CONSTRAINT blast_radius_hunks_tier_check CHECK (((tier)::t
          +ext = ANY (ARRAY[('blast-radius-none'::character varying)::tex
          +t, ('blast-radius-low'::character varying)::text, ('blast-radi
          +us-medium'::character varying)::text, ('blast-radius-high'::ch
          +aracter varying)::text])))                                    
      640  );
      641  
      642  
     ...
      678  --
      679  
      680  CREATE VIEW public.blast_radius_reviews AS
      681 - SELECT blast_radius_hunks.review_id,                         
      682 -    blast_radius_hunks.org_id,                                
      683 -    max(blast_radius_hunks.combined) AS combined,             
      684 -    (array_agg(blast_radius_hunks.tier ORDER BY blast_radius_h
          -unks.combined DESC))[1] AS tier                               
      681 + SELECT review_id,                                            
      682 +    org_id,                                                   
      683 +    max(combined) AS combined,                                
      684 +    (array_agg(tier ORDER BY blast_radius_hunks.combined DESC)
          +)[1] AS tier                                                  
      685     FROM public.blast_radius_hunks
      686 -  GROUP BY blast_radius_hunks.review_id, blast_radius_hunks.or
          -g_id;                                                         
      686 +  GROUP BY review_id, org_id;                                 
      687  
      688  
      689  --
     ...
      748      deleted_at timestamp with time zone,
      749      search_vector tsvector,
      750      surface character varying(20) DEFAULT 'chat'::character va
           rying NOT NULL,
      751 -    CONSTRAINT chat_conversations_surface_check CHECK (((surfa
          -ce)::text = ANY ((ARRAY['chat'::character varying, 'chat_debug
          -'::character varying])::text[])))                             
      751 +    CONSTRAINT chat_conversations_surface_check CHECK (((surfa
          +ce)::text = ANY (ARRAY[('chat'::character varying)::text, ('ch
          +at_debug'::character varying)::text])))                       
      752  );
     … 310 more lines (ctrl+o to expand)

● Bash(mkdir -p internal/githubapp && cat > internal/githubapp/config.go <<'EOF'
      // Package githubapp holds configuration and token handling for the
      LiveReview GitHub…)
  ⎿  ok      github.com/livereview/internal/githubapp        0.574s
  ⎿  Created internal/githubapp/config.go (+151 -0)
        1 +// Package githubapp holds configuration and token handling fo
          +r the LiveReview GitHub App. It makes no                      
        2 +// outbound calls itself; HTTP lives in network/providers/gith
          +ub and SQL in storage/providers/github.                       
        3 +package githubapp                                             
        4 +                                                              
        5 +import (                                                      
        6 +        "crypto/rsa"                                          
        7 +        "fmt"                                                 
        8 +        "log"                                                 
        9 +        "net/url"                                             
       10 +        "os"                                                  
       11 +        "strconv"                                             
       12 +        "strings"                                             
       13 +        "sync"                                                
       14 +                                                              
       15 +        "github.com/golang-jwt/jwt/v5"                        
       16 +                                                              
       17 +        githubprovider "github.com/livereview/internal/provide
          +rs/github"                                                    
       18 +)                                                             
       19 +                                                              
       20 +const defaultBaseURL = "https://github.com"                   
       21 +                                                              
       22 +// Config is the GitHub App configuration read from GITHUB_APP
          +_* environment variables.                                     
       23 +type Config struct {                                          
       24 +        AppID         int64                                   
       25 +        Slug          string                                  
       26 +        ClientID      string                                  
       27 +        ClientSecret  string                                  
       28 +        WebhookSecret string                                  
       29 +        BaseURL       string // e.g. https://github.com, no tr
          +ailing slash                                                  
       30 +        PrivateKey    *rsa.PrivateKey                         
       31 +}                                                             
       32 +                                                              
       33 +var (                                                         
       34 +        loadOnce sync.Once                                    
       35 +        loaded   *Config                                      
       36 +)                                                             
       37 +                                                              
       38 +// Load reads GITHUB_APP_* once. It returns nil when the featu
          +re is off (GITHUB_APP_ID unset) or                            
       39 +// misconfigured; a misconfiguration is logged once, naming wh
          +at is wrong.                                                  
       40 +func Load() *Config {                                         
     … 111 more lines (ctrl+o to expand)
  ⎿  Created internal/githubapp/config_test.go (+128 -0)
        1 +package githubapp                                             
        2 +                                                              
        3 +import (                                                      
        4 +        "crypto/rand"                                         
        5 +        "crypto/rsa"                                          
        6 +        "crypto/x509"                                         
        7 +        "encoding/pem"                                        
        8 +        "os"                                                  
        9 +        "path/filepath"                                       
       10 +        "strings"                                             
       11 +        "testing"                                             
       12 +)                                                             
       13 +                                                              
       14 +func testPEM(t *testing.T) string {                           
       15 +        t.Helper()                                            
       16 +        key, err := rsa.GenerateKey(rand.Reader, 2048)        
       17 +        if err != nil {                                       
       18 +                t.Fatal(err)                                  
       19 +        }                                                     
       20 +        return string(pem.EncodeToMemory(&pem.Block{Type: "RSA
          + PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))     
       21 +}                                                             
       22 +                                                              
       23 +func setEnv(t *testing.T, kv map[string]string) {             
       24 +        t.Helper()                                            
       25 +        for _, k := range []string{"GITHUB_APP_ID", "GITHUB_AP
          +P_SLUG", "GITHUB_APP_CLIENT_ID", "GITHUB_APP_CLIENT_SECRET", "
          +GITHUB_APP_PRIVATE_KEY", "GITHUB_APP_WEBHOOK_SECRET", "GITHUB_
          +APP_BASE_URL"} {                                              
       26 +                t.Setenv(k, kv[k])                            
       27 +        }                                                     
       28 +}                                                             
       29 +                                                              
       30 +func fullEnv(t *testing.T) map[string]string {                
       31 +        return map[string]string{                             
       32 +                "GITHUB_APP_ID": "123", "GITHUB_APP_SLUG": "li
          +vereview-test", "GITHUB_APP_CLIENT_ID": "Iv1.abc",            
       33 +                "GITHUB_APP_CLIENT_SECRET": "sec", "GITHUB_APP
          +_WEBHOOK_SECRET": "whsec", "GITHUB_APP_PRIVATE_KEY": testPEM(t
          +),                                                            
       34 +        }                                                     
       35 +}                                                             
       36 +                                                              
       37 +func TestFromEnv_OffWhenAppIDUnset(t *testing.T) {            
       38 +        setEnv(t, map[string]string{})                        
       39 +        cfg, err := fromEnv()                                 
       40 +        if cfg != nil || err != nil {                         
     … 88 more lines (ctrl+o to expand)

  Searched for 1 pattern (ctrl+o to expand)

● Bash(cat > network/providers/github/app.go <<'EOF'
      package github…)
  ⎿  Created network/providers/github/app.go (+305 -0)
        1 +package github                                                
        2 +                                                              
        3 +import (                                                      
        4 +        "context"                                             
        5 +        "crypto/rsa"                                          
        6 +        "encoding/json"                                       
        7 +        "fmt"                                                 
        8 +        "io"                                                  
        9 +        "net/http"                                            
       10 +        "net/url"                                             
       11 +        "regexp"                                              
       12 +        "strconv"                                             
       13 +        "strings"                                             
       14 +        "time"                                                
       15 +                                                              
       16 +        "github.com/golang-jwt/jwt/v5"                        
       17 +)                                                             
       18 +                                                              
       19 +// GitHub App calls. Every function takes its API/web base URL
          + from the caller (config), never a literal host.              
       20 +                                                              
       21 +const appHTTPTimeout = 15 * time.Second                       
       22 +                                                              
       23 +// APIError is a non-2xx GitHub response.                     
       24 +type APIError struct {                                        
       25 +        Status int                                            
       26 +        Body   string                                         
       27 +}                                                             
       28 +                                                              
       29 +func (e *APIError) Error() string {                           
       30 +        return fmt.Sprintf("github api returned %d: %s", e.Sta
          +tus, e.Body)                                                  
       31 +}                                                             
       32 +                                                              
       33 +func doJSON(ctx context.Context, method, requestURL, authHeade
          +r string, form url.Values, wantStatus int, out interface{}) (*
          +http.Response, error) {                                       
       34 +        var body io.Reader                                    
       35 +        if form != nil {                                      
       36 +                body = strings.NewReader(form.Encode())       
       37 +        }                                                     
       38 +        req, err := NewRequestWithContext(ctx, method, request
          +URL, body)                                                    
       39 +        if err != nil {                                       
       40 +                return nil, err                               
     … 265 more lines (ctrl+o to expand)

● Bash(cat > network/providers/github/app_test.go <<'EOF'
      package github…)
  ⎿  ok      github.com/livereview/network/providers/github  0.154s
  ⎿  Created network/providers/github/app_test.go (+137 -0)
        1 +package github                                                
        2 +                                                              
        3 +import (                                                      
        4 +        "context"                                             
        5 +        "crypto/rand"                                         
        6 +        "crypto/rsa"                                          
        7 +        "fmt"                                                 
        8 +        "net/http"                                            
        9 +        "net/http/httptest"                                   
       10 +        "strings"                                             
       11 +        "testing"                                             
       12 +        "time"                                                
       13 +                                                              
       14 +        "github.com/golang-jwt/jwt/v5"                        
       15 +)                                                             
       16 +                                                              
       17 +func TestMintAppJWT_VerifiesWithPublicKeyAndSetsIssuer(t *test
          +ing.T) {                                                      
       18 +        key, err := rsa.GenerateKey(rand.Reader, 2048)        
       19 +        if err != nil {                                       
       20 +                t.Fatal(err)                                  
       21 +        }                                                     
       22 +        now := time.Now()                                     
       23 +        signed, err := MintAppJWT(4242, key, now)             
       24 +        if err != nil {                                       
       25 +                t.Fatal(err)                                  
       26 +        }                                                     
       27 +        claims := &jwt.RegisteredClaims{}                     
       28 +        tok, err := jwt.ParseWithClaims(signed, claims, func(*
          +jwt.Token) (interface{}, error) { return &key.PublicKey, nil }
          +, jwt.WithValidMethods([]string{"RS256"}))                    
       29 +        if err != nil || !tok.Valid {                         
       30 +                t.Fatalf("token did not verify: %v", err)     
       31 +        }                                                     
       32 +        if claims.Issuer != "4242" {                          
       33 +                t.Errorf("iss = %q, want 4242", claims.Issuer)
       34 +        }                                                     
       35 +        if ttl := claims.ExpiresAt.Sub(now); ttl > 10*time.Min
          +ute {                                                         
       36 +                t.Errorf("expiry %v exceeds GitHub's 10 minute
          + limit", ttl)                                                 
       37 +        }                                                     
       38 +}                                                             
       39 +                                                              
       40 +func TestExchangeUserCode_TreatsErrorBodyAsFailure(t *testing.
          +T) {                                                          
     … 97 more lines (ctrl+o to expand)

● Network layer tests pass. I'm adding the new functions to the network status
  doc with computed line numbers.

  Read 1 file (ctrl+o to expand)

● Bash(mkdir -p /tmp/claude-1000/-home-lovestaco-hex-lr/97eec983-6279-4c09-8016-
      11f5b5ca4153/scratchpad && cat >
      /tmp/claude-1000/-home-lovestaco-hex-lr/97eec983-6279-…)
  ⎿  | providersgithub.MintAppJWT | added | [MintAppJWT](providers/github/a
     pp.go#L101) |
     | providersgithub.CreateInstallationToken | added | [CreateInstallatio
     … +14 lines (ctrl+o to expand)
  ⎿  Updated network/network_status.md (+9 -0)
      71  | api.selectLeaderAIConfig | added | [selectLeaderAIConfig](../
          internal/api/reviews_api.go#L378) |
      72  | api.selectHelperAIConfig | added | [selectHelperAIConfig](../
          internal/api/reviews_api.go#L414) |
      73  | api.GetReviewAccounting | updated | [GetReviewAccounting](../
          internal/api/review_events_endpoints.go#L207) |
      74 +| providersgithub.MintAppJWT | added | [MintAppJWT](providers/g
         +ithub/app.go#L101) |                                           
      75 +| providersgithub.CreateInstallationToken | added | [CreateInst
         +allationToken](providers/github/app.go#L116) |                 
      76 +| providersgithub.ExchangeUserCode | added | [ExchangeUserCode]
         +(providers/github/app.go#L137) |                               
      77 +| providersgithub.ListUserEmails | added | [ListUserEmails](pro
         +viders/github/app.go#L167) |                                   
      78 +| providersgithub.GetAuthenticatedUser | added | [GetAuthentica
         +tedUser](providers/github/app.go#L183) |                       
      79 +| providersgithub.ListUserInstallations | added | [ListUserInst
         +allations](providers/github/app.go#L201) |                     
      80 +| providersgithub.ListInstallationRepositories | added | [ListI
         +nstallationRepositories](providers/github/app.go#L246) |       
      81 +| providersgithub.ListOpenPulls | added | [ListOpenPulls](provi
         +ders/github/app.go#L269) |                                     
      82 +| providersgithub.GetUserByLogin | added | [GetUserByLogin](pro
         +viders/github/app.go#L299) |                                   

● Now the storage functions and the token resolver (T3.3).

● Bash(python3 - <<'EOF'
      p='storage/providers/github/token_store.go'…)
  ⎿  Traceback (most recent call last):
  ⎿  Updated storage/providers/github/token_store.go (+189 -1)
       1  package github
       2  
       3 -import "database/sql"                                          
       3 +import (                                                       
       4 +        "database/sql"                                         
       5 +        "encoding/json"                                        
       6 +        "errors"                                               
       7 +        "fmt"                                                  
       8 +        "time"                                                 
       9 +)                                                              
      10  
      11 +// TokenTypeGitHubApp marks an integration_tokens row that repr
         +esents a GitHub App installation.                              
      12 +const TokenTypeGitHubApp = "GITHUB_APP"                        
      13 +                                                               
      14 +// ErrInstallationBoundToOtherOrg is returned when an installat
         +ion already belongs to a different org.                        
      15 +var ErrInstallationBoundToOtherOrg = errors.New("github app ins
         +tallation is connected to another LiveReview organization")    
      16 +                                                               
      17  type IntegrationTokenRecord struct {
      18          ID          int64
      19          Provider    string
     ...
       53          }
       54          return &rec, nil
       55  }
       56 +                                                              
       57 +const appConnectorColumns = `id, provider, provider_url, COALE
          +SCE(pat_token, ''), COALESCE(metadata, '{}'), org_id,         
       58 +        COALESCE(token_type, ''), expires_at`                 
       59 +                                                              
       60 +func scanRecord(row interface{ Scan(...interface{}) error }) (
          +*IntegrationTokenRecord, error) {                             
       61 +        var rec IntegrationTokenRecord                        
       62 +        if err := row.Scan(&rec.ID, &rec.Provider, &rec.Provid
          +erURL, &rec.PatToken, &rec.Metadata, &rec.OrgID,              
       63 +                &rec.TokenType, &rec.ExpiresAt); err != nil { 
       64 +                return nil, err                               
       65 +        }                                                     
       66 +        return &rec, nil                                      
       67 +}                                                             
       68 +                                                              
       69 +// AppConnectorUpsert describes one GitHub App installation to
          + record for an org.                                           
       70 +type AppConnectorUpsert struct {                              
       71 +        OrgID               int64                             
       72 +        BaseURL             string                            
     … 159 more lines (ctrl+o to expand)

  Searched for 1 pattern (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      import re…)
  ⎿  Status-doc link check passed for storage/storage_status.md network/net
     work_status.md.
  ⎿  Updated storage/storage_status.md (+8 -2)
      41  | providersbitbucket.NewIntegrationTokenStore            | move
          d   | [NewIntegrationTokenStore](providers/bitbucket/integratio
          n_token_store.go#L18)        |
      42  | providersbitbucket.GetTokenByRepoFullName              | move
          d   | [GetTokenByRepoFullName](providers/bitbucket/integration_
          token_store.go#L22)          |
      43  | providersbitbucket.GetLatestBitbucketToken             | move
          d   | [GetLatestBitbucketToken](providers/bitbucket/integration
          _token_store.go#L41)         |
      44 -| providersgithub.NewTokenStore                          | move
         -d   | [NewTokenStore](providers/github/token_store.go#L20)     
         -                             |                                 
      45 -| providersgithub.GetGitHubTokenByConnectorID            | adde
         -d   | [GetGitHubTokenByConnectorID](providers/github/token_stor
         -e.go#L26)                    |                                 
      44 +| providersgithub.NewTokenStore                          | move
         +d   | [NewTokenStore](providers/github/token_store.go#L32)     
         +                             |                                 
      45 +| providersgithub.GetGitHubTokenByConnectorID            | adde
         +d   | [GetGitHubTokenByConnectorID](providers/github/token_stor
         +e.go#L38)                    |                                 
      46  | reviews.NewReviewStore                                 | move
          d   | [NewReviewStore](reviews/review_store.go#L9)             
                                       |
      47  | reviews.QueryRow                                       | move
          d   | [QueryRow](reviews/review_store.go#L13)                  
                                       |
      48  | reviews.Exec                                           | move
          d   | [Exec](reviews/review_store.go#L17)                      
                                       |
     ...
      179  | seed_demo.CloneReview                                  | add
           ed   | [CloneReview](seed_demo/store.go#L158)                 
                                          |
      180  | seed_demo.DeleteReview                                 | add
           ed   | [DeleteReview](seed_demo/store.go#L220)                
                                          |
      181  | seed_demo.CurrentPlan                                  | add
           ed   | [CurrentPlan](seed_demo/store.go#L243)                 
                                          |
      182 +| providersgithub.UpsertAppInstallationConnector | added | [Up
          +sertAppInstallationConnector](providers/github/token_store.go#
          +L85) |                                                        
      183 +| providersgithub.GetAppConnectorByInstallation | added | [Get
          +AppConnectorByInstallation](providers/github/token_store.go#L1
          +51) |                                                         
      184 +| providersgithub.UpdateCachedInstallationToken | added | [Upd
          +ateCachedInstallationToken](providers/github/token_store.go#L1
          +59) |                                                         
      185 +| providersgithub.ListAppConnectorsExpiringBefore | added | [L
          +istAppConnectorsExpiringBefore](providers/github/token_store.g
          +o#L169) |                                                     
      186 +| providersgithub.ListAppConnectorsForOrg | added | [ListAppCo
          +nnectorsForOrg](providers/github/token_store.go#L180) |       
      187 +| providersgithub.SetAppConnectorFlags | added | [SetAppConnec
          +torFlags](providers/github/token_store.go#L207) |             

● Bash(cat > internal/githubapp/token.go <<'EOF'
      package githubapp…)
  ⎿  ok      github.com/livereview/internal/githubapp        0.947s
  ⎿  Created internal/githubapp/token.go (+88 -0)
       1 +package githubapp                                              
       2 +                                                               
       3 +import (                                                       
       4 +        "context"                                              
       5 +        "encoding/json"                                        
       6 +        "errors"                                               
       7 +        "fmt"                                                  
       8 +        "strconv"                                              
       9 +        "time"                                                 
      10 +                                                               
      11 +        networkgithub "github.com/livereview/network/providers/
         +github"                                                        
      12 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      13 +)                                                              
      14 +                                                               
      15 +// refreshMargin: a cached installation token that expires soon
         +er than this is replaced before use.                           
      16 +const refreshMargin = 5 * time.Minute                          
      17 +                                                               
      18 +var (                                                          
      19 +        ErrNotConfigured       = errors.New("github app not con
         +figured on this server")                                       
      20 +        ErrInstallationRemoved = errors.New("github app install
         +ation removed")                                                
      21 +)                                                              
      22 +                                                               
      23 +// ConnectorToken returns a usable API token for any GitHub con
         +nector. PAT rows return pat_token unchanged;                   
      24 +// GITHUB_APP rows return the cached installation token, mintin
         +g a new one when it expires within 5 minutes.                  
      25 +func ConnectorToken(ctx context.Context, store *storagegithub.T
         +okenStore, rec *storagegithub.IntegrationTokenRecord) (string, 
         +error) {                                                       
      26 +        if rec.TokenType != storagegithub.TokenTypeGitHubApp { 
      27 +                return rec.PatToken, nil                       
      28 +        }                                                      
      29 +        return connectorToken(ctx, Load(), store, rec, false)  
      30 +}                                                              
      31 +                                                               
      32 +// RefreshConnectorToken mints and stores a new installation to
         +ken regardless of the cached one's age.                        
      33 +func RefreshConnectorToken(ctx context.Context, store *storageg
         +ithub.TokenStore, rec *storagegithub.IntegrationTokenRecord) (s
         +tring, error) {                                                
      34 +        return connectorToken(ctx, Load(), store, rec, true)   
      35 +}                                                              
      36 +                                                               
      37 +type appConnectorMeta struct {                                 
      38 +        InstallationID int64 `json:"installation_id"`          
      39 +        Removed        bool  `json:"removed"`                  
      40 +        Suspended      bool  `json:"suspended"`                
     … 48 more lines (ctrl+o to expand)
  ⎿  Created internal/githubapp/token_test.go (+161 -0)
        1 +package githubapp                                             
        2 +                                                              
        3 +import (                                                      
        4 +        "context"                                             
        5 +        "crypto/rand"                                         
        6 +        "crypto/rsa"                                          
        7 +        "database/sql"                                        
        8 +        "fmt"                                                 
        9 +        "net/http"                                            
       10 +        "net/http/httptest"                                   
       11 +        "sync/atomic"                                         
       12 +        "testing"                                             
       13 +        "time"                                                
       14 +                                                              
       15 +        "github.com/livereview/internal/database"             
       16 +        storagegithub "github.com/livereview/storage/providers
          +/github"                                                      
       17 +)                                                             
       18 +                                                              
       19 +func testDB(t *testing.T) *sql.DB {                           
       20 +        t.Helper()                                            
       21 +        db, err := database.NewDB()                           
       22 +        if err != nil {                                       
       23 +                t.Skipf("skipping: no database available: %v",
          + err)                                                         
       24 +        }                                                     
       25 +        t.Cleanup(func() { db.Close() })                      
       26 +        return db                                             
       27 +}                                                             
       28 +                                                              
       29 +func testOrg(t *testing.T, db *sql.DB, name string) int64 {   
       30 +        t.Helper()                                            
       31 +        var id int64                                          
       32 +        if err := db.QueryRow(`INSERT INTO orgs (name, descrip
          +tion) VALUES ($1, 'githubapp test') RETURNING id`, name).Scan(
          +&id); err != nil {                                            
       33 +                t.Fatal(err)                                  
       34 +        }                                                     
       35 +        t.Cleanup(func() {                                    
       36 +                _, _ = db.Exec(`DELETE FROM integration_tokens
          + WHERE org_id = $1`, id)                                      
       37 +                _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1
          +`, id)                                                        
       38 +        })                                                    
       39 +        return id                                             
       40 +}                                                             
     … 121 more lines (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='internal/provider_input/github/github_token_helpers.go'…)
  ⎿  // buildProviderConfig builds the review.ProviderConfig a provider fac
     tory needs to instantiate a provider, given the integration token and
     the target PR/MR URL. accessToken is the fallback token used for non-P
     … +15 lines (ctrl+o to expand)
  ⎿  Updated internal/provider_input/github/github_token_helpers.go (+8 -1)
      10          "net/http"
      11          "time"
      12  
      13 +        "github.com/livereview/internal/githubapp"             
      14          networkgithub "github.com/livereview/network/providers/
          github"
      15          storagegithub "github.com/livereview/storage/providers/
          github"
      16  )
     ...
      68          token.Provider = rec.Provider
      69          token.ProviderURL = rec.ProviderURL
      70          token.TokenType = rec.TokenType
      70 -        token.PatToken = rec.PatToken                          
      71  
      72 +        // PAT connectors return pat_token as-is; GitHub App co
         +nnectors return a fresh installation token.                    
      73 +        apiToken, err := githubapp.ConnectorToken(context.Backg
         +round(), store, rec)                                           
      74 +        if err != nil {                                        
      75 +                return nil, fmt.Errorf("github connector %d: %w
         +", connectorID, err)                                           
      76 +        }                                                      
      77 +        token.PatToken = apiToken                              
      78 +                                                               
      79          if len(rec.Metadata) > 0 {
      80                  if err := json.Unmarshal(rec.Metadata, &token.M
          etadata); err != nil {
      81                          log.Printf("[WARN] Failed to parse GitH
          ub token metadata: %v", err)
  ⎿  Updated internal/api/reviews_api.go (+2 -2)
      261  // ensureValidToken ensures we have a valid token, falling bac
           k to config file if needed
      262  func ensureValidToken(token *IntegrationToken) string {
      263          // Handle GitHub PAT tokens
      264 -        if strings.HasPrefix(token.Provider, "github") && toke
          -n.TokenType == "PAT" {                                        
      264 +        if strings.HasPrefix(token.Provider, "github") && (tok
          +en.TokenType == "PAT" || token.TokenType == "GITHUB_APP") {   
      265                  log.Printf("[DEBUG] ensureValidToken: Using Gi
           tHub PAT from database: %s", maskToken(token.PatToken))
      266                  return token.PatToken
      267          }
     ...
      406  func buildProviderConfig(token *IntegrationToken, requestURL, 
           accessToken string) review.ProviderConfig {
      407          providerToken := accessToken
      408          providerConfigMap := map[string]interface{}{}
      409 -        if token.TokenType == "PAT" && token.PatToken != "" { 
      409 +        if (token.TokenType == "PAT" || token.TokenType == "GI
          +THUB_APP") && token.PatToken != "" {                          
      410                  if strings.HasPrefix(token.Provider, "github")
            {
      411                          providerToken = token.PatToken
      412                          providerConfigMap["pat_token"] = token
           .PatToken

  Searched for 1 pattern (ctrl+o to expand)

● Bash(cat >> storage/providers/github/token_store.go <<'EOF'…)
  ⎿  Updated storage/providers/github/token_store.go (+31 -0)
      229          `, string(b), clearToken, connectorID, TokenTypeGitHub
           App)
      230          return err
      231  }
      232 +                                                              
      233 +// FindPreferredConnectorForRepo returns the org's GitHub conn
          +ector that has the repository, preferring a                   
      234 +// GitHub App connector over a PAT one (the App wins on overla
          +p). sql.ErrNoRows means no connector lists it.                
      235 +func (s *TokenStore) FindPreferredConnectorForRepo(orgID int64
          +, repoFullName string) (int64, error) {                       
      236 +        var id int64                                          
      237 +        err := s.db.QueryRow(`                                
      238 +                SELECT it.id                                  
      239 +                FROM integration_tokens it                    
      240 +                JOIN repositories r ON r.connector_id = it.id 
      241 +                WHERE it.org_id = $1 AND it.provider LIKE 'git
          +hub%' AND LOWER(r.full_name) = LOWER($2)                      
      242 +                  AND COALESCE(it.metadata->>'removed', 'false
          +') <> 'true'                                                  
      243 +                ORDER BY (it.token_type = $3) DESC, it.created
          +_at DESC                                                      
      244 +                LIMIT 1                                       
      245 +        `, orgID, repoFullName, TokenTypeGitHubApp).Scan(&id) 
      246 +        return id, err                                        
      247 +}                                                             
      248 +                                                              
      249 +// HasAppConnectorForRepo reports whether the org has a live G
          +itHub App connector that lists the repository.                
      250 +func (s *TokenStore) HasAppConnectorForRepo(orgID int64, repoF
          +ullName string) (bool, error) {                               
      251 +        var exists bool                                       
      252 +        err := s.db.QueryRow(`                                
      253 +                SELECT EXISTS (                               
      254 +                        SELECT 1 FROM integration_tokens it   
      255 +                        JOIN repositories r ON r.connector_id 
          += it.id                                                       
      256 +                        WHERE it.org_id = $1 AND it.token_type
          + = $2 AND LOWER(r.full_name) = LOWER($3)                      
      257 +                          AND COALESCE(it.metadata->>'removed'
          +, 'false') <> 'true'                                          
      258 +                          AND COALESCE(it.metadata->>'suspende
          +d', 'false') <> 'true'                                        
      259 +                )                                             
      260 +        `, orgID, TokenTypeGitHubApp, repoFullName).Scan(&exis
          +ts)                                                           
      261 +        return exists, err                                    
      262 +}                                                             
  ⎿  Created internal/api/github_app_connectors.go (+71 -0)
       1 +package api                                                    
       2 +                                                               
       3 +import (                                                       
       4 +        "context"                                              
       5 +        "database/sql"                                         
       6 +        "errors"                                               
       7 +        "log"                                                  
       8 +        "net/url"                                              
       9 +        "strings"                                              
      10 +                                                               
      11 +        "github.com/livereview/internal/githubapp"             
      12 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      13 +)                                                              
      14 +                                                               
      15 +// githubPullRepo extracts "owner/repo" from a GitHub pull requ
         +est URL (/owner/repo/pull/N); "" when it isn't one.            
      16 +func githubPullRepo(rawURL string) string {                    
      17 +        u, err := url.Parse(rawURL)                            
      18 +        if err != nil {                                        
      19 +                return ""                                      
      20 +        }                                                      
      21 +        parts := strings.Split(strings.Trim(u.Path, "/"), "/") 
      22 +        if len(parts) < 4 || parts[2] != "pull" {              
      23 +                return ""                                      
      24 +        }                                                      
      25 +        return parts[0] + "/" + parts[1]                       
      26 +}                                                              
      27 +                                                               
      28 +// preferredGitHubConnector picks the connector for a pull requ
         +est URL from the org's known repositories, so a                
      29 +// GitHub App connector beats an older PAT connector for the sa
         +me repo. It returns nil when the repository is                 
      30 +// not (yet) in the repositories table or the connector is on a
         + different host; the caller then keeps its                     
      31 +// URL-based lookup.                                           
      32 +func (s *Server) preferredGitHubConnector(requestURL, baseURL s
         +tring, orgID int64) *IntegrationToken {                        
      33 +        repo := githubPullRepo(requestURL)                     
      34 +        if repo == "" {                                        
      35 +                return nil                                     
      36 +        }                                                      
      37 +        id, err := storagegithub.NewTokenStore(s.db).FindPrefer
         +redConnectorForRepo(orgID, repo)                               
      38 +        if err != nil {                                        
      39 +                if !errors.Is(err, sql.ErrNoRows) {            
      40 +                        log.Printf("[WARN] preferredGitHubConne
         +ctor: lookup failed for %s: %v", repo, err)                    
     … 31 more lines (ctrl+o to expand)
  ⎿  Updated internal/api/review_service.go (+14 -4)
      369                  ctx.logger.LogSection("INTEGRATION TOKEN")
      370                  ctx.logger.Log("Finding integration token...")
      371          }
      372 -        token, err := s.findIntegrationToken(baseURL, ctx.orgI
          -D)                                                            
      373 -        if err != nil {                                       
      372 +        token := s.preferredGitHubConnector(ctx.requestURL, ba
          +seURL, ctx.orgID)                                             
      373 +        if token == nil {                                     
      374 +                var err error                                 
      375 +                token, err = s.findIntegrationToken(baseURL, c
          +tx.orgID)                                                     
      376 +                if err != nil {                               
      377 +                        if ctx.logger != nil {                
      378 +                                ctx.logger.LogError("Failed to
          + find integration token", err)                                
      379 +                        }                                     
      380 +                        return err                            
      381 +                }                                             
      382 +        }                                                     
      383 +        if err := s.resolveGitHubAppToken(context.Background()
          +, token); err != nil {                                        
      384                  if ctx.logger != nil {
      375 -                        ctx.logger.LogError("Failed to find in
          -tegration token", err)                                        
      385 +                        ctx.logger.LogError("GitHub App token 
          +unavailable", err)                                            
      386                  }
      377 -                return err                                    
      387 +                return fmt.Errorf("the GitHub App connection f
          +or this repository is not usable: %v", err)                   
      388          }
      389          ctx.token = token
      390          if ctx.logger != nil {
  ⎿  Updated internal/api/reviews_api.go (+1 -0)
      87                  SELECT id, provider, access_token, refresh_toke
          n, expires_at, provider_app_id, client_secret, provider_url, to
          ken_type, pat_token, COALESCE(metadata, '{}')
      88                  FROM integration_tokens
      89                  WHERE provider_url LIKE '%' || $1 || '%' AND or
          g_id = $2
      90 +                  AND COALESCE(metadata->>'removed', 'false') <
         +> 'true'                                                       
      91                  ORDER BY created_at DESC
      92                  LIMIT 1
      93          `

  Ran 1 shell command (ctrl+o to expand)

● Bash(cat > internal/jobqueue/github_app_token_refresh_worker.go <<'EOF'
      package jobqueue…)
  ⎿  37:     "github.com/livereview/internal/providers"
     38:     "github.com/livereview/internal/providers/gitea"
     39:     "github.com/livereview/internal/policyengine"
     40:     "github.com/livereview/internal/seed_demo"
  ⎿  Created internal/jobqueue/github_app_token_refresh_worker.go (+54 -0)
       1 +package jobqueue                                               
       2 +                                                               
       3 +import (                                                       
       4 +        "context"                                              
       5 +        "database/sql"                                         
       6 +        "fmt"                                                  
       7 +        "log"                                                  
       8 +        "time"                                                 
       9 +                                                               
      10 +        "github.com/riverqueue/river"                          
      11 +                                                               
      12 +        "github.com/livereview/internal/githubapp"             
      13 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      14 +)                                                              
      15 +                                                               
      16 +// githubAppTokenRefreshInterval is how often cached installati
         +on tokens are checked; tokens live an hour and                 
      17 +// are renewed when they expire within githubAppTokenRefreshWin
         +dow, so a missed run still leaves slack.                       
      18 +const (                                                        
      19 +        githubAppTokenRefreshInterval = 10 * time.Minute       
      20 +        githubAppTokenRefreshWindow   = 20 * time.Minute       
      21 +)                                                              
      22 +                                                               
      23 +// GitHubAppTokenRefreshJobArgs keeps integration_tokens.pat_to
         +ken fresh for GitHub App connectors. Hot paths                 
      24 +// resolve tokens on demand (githubapp.ConnectorToken); this jo
         +b covers the many legacy readers that select                   
      25 +// pat_token straight from the table.                          
      26 +type GitHubAppTokenRefreshJobArgs struct{}                     
      27 +                                                               
      28 +func (GitHubAppTokenRefreshJobArgs) Kind() string { return "git
         +hub_app_token_refresh" }                                       
      29 +                                                               
      30 +type GitHubAppTokenRefreshWorker struct {                      
      31 +        river.WorkerDefaults[GitHubAppTokenRefreshJobArgs]     
      32 +        db *sql.DB                                             
      33 +}                                                              
      34 +                                                               
      35 +func (w *GitHubAppTokenRefreshWorker) Work(ctx context.Context,
         + job *river.Job[GitHubAppTokenRefreshJobArgs]) error {         
      36 +        store := storagegithub.NewTokenStore(w.db)             
      37 +        records, err := store.ListAppConnectorsExpiringBefore(t
         +ime.Now().Add(githubAppTokenRefreshWindow))                    
      38 +        if err != nil {                                        
      39 +                return fmt.Errorf("list expiring github app con
         +nectors: %w", err)                                             
      40 +        }                                                      
     … 14 more lines (ctrl+o to expand)
  ⎿  Updated internal/jobqueue/jobqueue.go (+12 -0)
      2428          preloadedChangesArchivalSweepWorker := &PreloadedChan
            gesArchivalSweepWorker{db: db}
      2429          preloadedChangesArchivalPurgeWorker := &PreloadedChan
            gesArchivalPurgeWorker{db: db}
      2430          seedDemoWorker := &seed_demo.Worker{DB: db}
      2431 +        river.AddWorker(workers, &GitHubAppTokenRefreshWorker
           +{db: db})                                                    
      2432          river.AddWorker(workers, &WebhookInstallWorker{pool: 
            pool, config: config, store: store, httpClient: httpClient})
      2433          river.AddWorker(workers, &WebhookRemovalWorker{pool: 
            pool, config: config, store: store, httpClient: httpClient})
      2434          river.AddWorker(workers, diffWorker)
     ...
      2529                  ))
      2530          }
      2531  
      2532 +        // Only when the GitHub App is configured (GITHUB_APP
           +_ID and friends set).                                        
      2533 +        if githubapp.Load() != nil {                         
      2534 +                periodicJobs = append(periodicJobs, river.New
           +PeriodicJob(                                                 
      2535 +                        river.PeriodicInterval(githubAppToken
           +RefreshInterval),                                            
      2536 +                        func() (river.JobArgs, *river.InsertO
           +pts) {                                                       
      2537 +                                return GitHubAppTokenRefreshJ
           +obArgs{}, &river.InsertOpts{Queue: "repo_sync", MaxAttempts: 
           +3}                                                           
      2538 +                        },                                   
      2539 +                        &river.PeriodicJobOpts{ID: "github_ap
           +p_token_refresh", RunOnStart: true},                         
      2540 +                ))                                           
      2541 +        }                                                    
      2542 +                                                             
      2543          client, err := river.NewClient(riverpgxv5.New(pool), 
            &river.Config{
      2544                  Queues:                      config.RiverQueu
            eConfig(),
      2545                  Workers:                     workers,
● Bash(sed -i
      's#^\t"github.com/livereview/internal/providers"$#\t"github.com/livereview
      /internal/githubapp"\n\t"github.com/livereview/internal/providers"#'
      internal/j…)
  ⎿  internal/jobqueue/pr_policy_force_approve.go:122:       _, err :=
     jq.client.I
     nsert(ctx, args, &river.InsertOpts{Queue: "repo_sync", MaxAttempts: 5}
     )
     … +3 lines (ctrl+o to expand)
  ⎿  Updated internal/jobqueue/jobqueue.go (+1 -0)
      34  
      35          "github.com/jackc/pgx/v5"
      36          "github.com/jackc/pgx/v5/pgxpool"
      37 +        "github.com/livereview/internal/githubapp"             
      38          "github.com/livereview/internal/providers"
      39          "github.com/livereview/internal/providers/gitea"
      40          "github.com/livereview/internal/policyengine"

● Now the bot identity (T3.5). First, how mentions are detected.

  Searched for 2 patterns, ran 2 shell commands (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='internal/providers/github/mentions.go'…)
  ⎿  ok      github.com/livereview/internal/providers/github 0.013s
  ⎿  Updated internal/providers/github/mentions.go (+9 -2)
      18                  return false
      19          }
      20  
      21 -        mentionPattern := "@" + strings.ToLower(username)      
      22 -        return strings.Contains(strings.ToLower(commentBody), m
         -entionPattern)                                                 
      21 +        body := strings.ToLower(commentBody)                   
      22 +        if strings.Contains(body, "@"+strings.ToLower(username)
         +) {                                                            
      23 +                return true                                    
      24 +        }                                                      
      25 +        // A GitHub App's bot account is "<slug>[bot]", but peo
         +ple mention it as plain "@<slug>".                             
      26 +        if slug := strings.TrimSuffix(strings.ToLower(username)
         +, "[bot]"); slug != strings.ToLower(username) && slug != "" {  
      27 +                return strings.Contains(body, "@"+slug)        
      28 +        }                                                      
      29 +        return false                                           
      30  }
  ⎿  Created internal/providers/github/mentions_test.go (+34 -0)
       1 +package github                                                 
       2 +                                                               
       3 +import (                                                       
       4 +        "testing"                                              
       5 +                                                               
       6 +        coreprocessor "github.com/livereview/internal/core_proc
         +essor"                                                         
       7 +)                                                              
       8 +                                                               
       9 +func TestDetectDirectMention(t *testing.T) {                   
      10 +        cases := []struct {                                    
      11 +                name     string                                
      12 +                username string                                
      13 +                body     string                                
      14 +                want     bool                                  
      15 +        }{                                                     
      16 +                {"plain user", "livereview-bot", "hey @LiveRevi
         +ew-Bot please look", true},                                    
      17 +                {"no mention", "livereview-bot", "nothing here"
         +, false},                                                      
      18 +                {"app bot mentioned by slug", "livereview[bot]"
         +, "@livereview can you review this?", true},                   
      19 +                {"app bot mentioned with suffix", "livereview[b
         +ot]", "@livereview[bot] review", true},                        
      20 +                {"app bot not mentioned", "livereview[bot]", "@
         +someone else", false},                                         
      21 +                {"empty username", "", "@anything", false},    
      22 +        }                                                      
      23 +        for _, tc := range cases {                             
      24 +                t.Run(tc.name, func(t *testing.T) {            
      25 +                        got := DetectDirectMention(tc.body, &co
         +reprocessor.UnifiedBotUserInfoV2{Username: tc.username})       
      26 +                        if got != tc.want {                    
      27 +                                t.Fatalf("DetectDirectMention(%
         +q, %q) = %v, want %v", tc.body, tc.username, got, tc.want)     
      28 +                        }                                      
      29 +                })                                             
      30 +        }                                                      
      31 +        if DetectDirectMention("@x", nil) {                    
      32 +                t.Fatal("nil bot info must not match")         
      33 +        }                                                      
      34 +}                                                              

● Bash(python3 - <<'EOF'
      p='internal/provider_input/github/github_provider.go'…)
  ⎿  Updated internal/provider_input/github/github_provider.go (+42 -0)
       1  package github
       2  
       3  import (
       4 +        "context"                                              
       5          "database/sql"
       6          "encoding/json"
       7          "fmt"
       8          "log"
       9          "strings"
      10 +        "sync"                                                 
      11  
      12          "github.com/livereview/internal/capture"
      13          coreprocessor "github.com/livereview/internal/core_proc
          essor"
      14 +        "github.com/livereview/internal/githubapp"             
      15          "github.com/livereview/internal/webhookutils"
      16 +        networkgithub "github.com/livereview/network/providers/
         +github"                                                        
      17 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      18  )
      19  
      20  type (
     ...
      416                  return nil, fmt.Errorf("failed to get GitHub t
           oken: %w", err)
      417          }
      418  
      419 +        if token.TokenType == storagegithub.TokenTypeGitHubApp
          + {                                                            
      420 +                return appBotInfo(token)                      
      421 +        }                                                     
      422 +                                                              
      423          botInfo, err := FetchGitHubBotUserInfo(token.PatToken)
      424          if err != nil {
      425                  return nil, fmt.Errorf("failed to get GitHub b
           ot user info: %w", err)
     ...
      611  
      612          return "", fmt.Errorf("repository_full_name missing fr
           om metadata")
      613  }
      614 +                                                              
      615 +var appBotCache sync.Map // "<api base>|<login>" -> *UnifiedBo
          +tUserInfoV2                                                   
      616 +                                                              
      617 +// appBotInfo describes the GitHub App's bot account. GET /use
          +r is rejected for installation tokens, so the                 
      618 +// account is looked up by its "<slug>[bot]" login instead; th
          +e result never changes, so it is cached.                      
      619 +func appBotInfo(token *IntegrationToken) (*UnifiedBotUserInfoV
          +2, error) {                                                   
      620 +        cfg := githubapp.Load()                               
     … 26 more lines (ctrl+o to expand)

● Now the fw-parse client and the shared EnsureCloudUser core (T3.7).

  Ran 5 shell commands (ctrl+o to expand)

● I noticed the local fw-parse .env points to a production Mongo, so I won't
  start it now to avoid writing test data into prod. I'll continue coding
  without it and flag this before any end-to-end testing. · summary

  Ran 1 shell command (ctrl+o to expand)

● Now the EnsureCloudUser refactor: SQL moves to storage/users, and the handler
  and the callback share one core.

  Ran 1 shell command (ctrl+o to expand)

● Bash(cat > storage/users/cloud_user_store.go <<'EOF'
      package users…)
  ⎿  Created storage/users/cloud_user_store.go (+117 -0)
        1 +package users                                                 
        2 +                                                              
        3 +import (                                                      
        4 +        "database/sql"                                        
        5 +        "errors"                                              
        6 +)                                                             
        7 +                                                              
        8 +// Queries behind cloud user provisioning (internal/api/auth E
          +nsureCloudUser). They are moved verbatim from the             
        9 +// handler so the HTTP endpoint and the GitHub App callback sh
          +are exactly one provisioning path.                            
       10 +                                                              
       11 +// FindUserByEmail returns the user's id and password hash; sq
          +l.ErrNoRows when there is no such user.                       
       12 +func (s *UserStore) FindUserByEmail(tx *sql.Tx, email string) 
          +(userID int64, passwordHash string, err error) {              
       13 +        err = tx.QueryRow(`SELECT id, password_hash FROM users
          + WHERE email = $1`, email).Scan(&userID, &passwordHash)       
       14 +        return userID, passwordHash, err                      
       15 +}                                                             
       16 +                                                              
       17 +// InsertCloudUser creates a user whose password is a random, 
          +unused hash (cloud users sign in via Hexmos).                 
       18 +// A nil firstName/lastName stores NULL.                      
       19 +func (s *UserStore) InsertCloudUser(tx *sql.Tx, email, passwor
          +dHash string, firstName, lastName interface{}) (int64, error) 
          +{                                                             
       20 +        var id int64                                          
       21 +        err := tx.QueryRow(`                                  
       22 +                INSERT INTO users (email, password_hash, first
          +_name, last_name, password_reset_required, created_at, updated
          +_at)                                                          
       23 +                VALUES ($1, $2, $3, $4, false, NOW(), NOW()) R
          +ETURNING id                                                   
       24 +        `, email, passwordHash, firstName, lastName).Scan(&id)
       25 +        return id, err                                        
       26 +}                                                             
       27 +                                                              
       28 +// FindOrgIDByName returns the org with this exact name; sql.E
          +rrNoRows when none exists.                                    
       29 +func (s *UserStore) FindOrgIDByName(tx *sql.Tx, name string) (
          +int64, error) {                                               
       30 +        var id int64                                          
       31 +        err := tx.QueryRow(`SELECT id FROM orgs WHERE name = $
          +1`, name).Scan(&id)                                           
       32 +        return id, err                                        
       33 +}                                                             
       34 +                                                              
       35 +// InsertPersonalOrg creates an org named after the user's ema
          +il, plus its prompt_application_context row.                  
       36 +func (s *UserStore) InsertPersonalOrg(tx *sql.Tx, email string
          +, createdByUserID int64) (int64, error) {                     
       37 +        var orgID int64                                       
       38 +        err := tx.QueryRow(`                                  
       39 +                INSERT INTO orgs (name, description, created_b
          +y_user_id, created_at, updated_at)                            
       40 +                VALUES ($1, $2, $3, NOW(), NOW()) RETURNING id
     … 77 more lines (ctrl+o to expand)

● Bash(cat > internal/api/auth/ensure_cloud_user.go <<'EOF'
      package auth…)
  ⎿  # github.com/livereview/internal/api/auth
     internal/api/auth/handlers.go:5:2: "crypto/rand" imported and not used
     internal/api/auth/handlers.go:7:2: "encoding/hex" imported and not use
     … +3 lines (ctrl+o to expand)
  ⎿  Created internal/api/auth/ensure_cloud_user.go (+291 -0)
        1 +package auth                                                  
        2 +                                                              
        3 +import (                                                      
        4 +        "crypto/rand"                                         
        5 +        "database/sql"                                        
        6 +        "encoding/hex"                                        
        7 +        "errors"                                              
        8 +        "fmt"                                                 
        9 +        "log"                                                 
       10 +        "net/http"                                            
       11 +        "os"                                                  
       12 +        "strings"                                             
       13 +        "time"                                                
       14 +                                                              
       15 +        "github.com/golang-jwt/jwt/v5"                        
       16 +        "github.com/labstack/echo/v4"                         
       17 +        "github.com/livereview/pkg/models"                    
       18 +        storageusers "github.com/livereview/storage/users"    
       19 +        "golang.org/x/crypto/bcrypt"                          
       20 +)                                                             
       21 +                                                              
       22 +// EnsureCloudUserRequest represents the request body for ensu
          +ring a cloud user exists                                      
       23 +type EnsureCloudUserRequest struct {                          
       24 +        Email     string `json:"email"`                       
       25 +        FirstName string `json:"first_name"`                  
       26 +        LastName  string `json:"last_name"`                   
       27 +        Source    string `json:"source,omitempty"`            
       28 +}                                                             
       29 +                                                              
       30 +// EnsureCloudUserInput is the non-HTTP form of the request: t
          +he Hexmos JWT plus who it is for.                             
       31 +type EnsureCloudUserInput struct {                            
       32 +        JWT       string                                      
       33 +        Email     string                                      
       34 +        FirstName string                                      
       35 +        LastName  string                                      
       36 +        Source    string                                      
       37 +}                                                             
       38 +                                                              
       39 +// EnsureCloudUserResult reports what provisioning did. All Cr
          +eated*/Assigned* flags are false on a repeat call.            
       40 +type EnsureCloudUserResult struct {                           
     … 251 more lines (ctrl+o to expand)
  ⎿  Updated internal/api/auth/handlers.go (+0 -244)
      623          return organizations, nil
      624  }
      625  
      626 -// EnsureCloudUserRequest represents the request body for ensu
          -ring a cloud user exists                                      
      627 -type EnsureCloudUserRequest struct {                          
      628 -        Email     string `json:"email"`                       
      629 -        FirstName string `json:"first_name"`                  
      630 -        LastName  string `json:"last_name"`                   
      631 -        Source    string `json:"source,omitempty"`            
      632 -}                                                             
      633 -                                                              
      634 -// EnsureCloudUser ensures that a user and a personal organiza
          -tion exist, assigning super_admin role.                       
      635 -// Idempotent: if user/org/role mapping already exist, it retu
          -rns success without changes.                                  
      636 -func (h *AuthHandlers) EnsureCloudUser(c echo.Context) error {
      637 -        // Basic JWT validation using CLOUD_JWT_SECRET (manual
          -, since this route is public)                                 
      638 -        authHeader := c.Request().Header.Get("Authorization") 
      639 -        if authHeader == "" || !strings.HasPrefix(authHeader, 
          -"Bearer ") {                                                  
      640 -                return c.JSON(http.StatusUnauthorized, map[str
          -ing]string{"error": "authorization header required"})         
      641 -        }                                                     
      642 -        tokenString := strings.TrimPrefix(authHeader, "Bearer 
          -")                                                            
      643 -        secret := os.Getenv("CLOUD_JWT_SECRET")               
      644 -        if strings.TrimSpace(secret) == "" {                  
      645 -                return c.JSON(http.StatusUnauthorized, map[str
          -ing]string{"error": "cloud jwt not configured"})              
      646 -        }                                                     
      647 -                                                              
      648 -        token, err := jwt.ParseWithClaims(tokenString, &JWTCla
          -ims{}, func(token *jwt.Token) (interface{}, error) {          
      649 -                if _, ok := token.Method.(*jwt.SigningMethodHM
          -AC); !ok {                                                    
      650 -                        return nil, fmt.Errorf("unexpected sig
          -ning method: %v", token.Header["alg"])                        
      651 -                }                                             
      652 -                return []byte(secret), nil                    
      653 -        })                                                    
      654 -        if err != nil || !token.Valid {                       
      655 -                if err == nil {                               
      656 -                        err = fmt.Errorf("invalid token")     
      657 -                }                                             
      658 -                return c.JSON(http.StatusUnauthorized, map[str
          -ing]string{"error": err.Error()})                             
      659 -        }                                                     
      660 -        claims, ok := token.Claims.(*JWTClaims)               
      661 -        if !ok {                                              
      662 -                return c.JSON(http.StatusUnauthorized, map[str
          -ing]string{"error": "invalid token claims"})                  
     … 210 more lines (ctrl+o to expand)

● Bash(sed -i '/^\t"crypto\/rand"$/d;/^\t"encoding\/hex"$/d;/^\t"github.com\/gol
      ang-jwt\/jwt\/v5"$/d' internal/api/auth/handlers.go && sed -i 's#^//
      InsertPersonalOrg…)
  ⎿  "strings"
     "time"
     … +6 lines (ctrl+o to expand)
  ⎿  Updated internal/api/auth/handlers.go (+0 -3)
       2  
       3  import (
       4          "bytes"
       5 -        "crypto/rand"                                          
       5          "database/sql"
       7 -        "encoding/hex"                                         
       6          "encoding/json"
       7          "fmt"
       8          "log"
     ...
      12          "strings"
      13          "time"
      14  
      17 -        "github.com/golang-jwt/jwt/v5"                         
      15          "github.com/labstack/echo/v4"
      16          "github.com/livereview/pkg/models"
      17          "golang.org/x/crypto/bcrypt"
  ⎿  Updated storage/users/cloud_user_store.go (+1 -1)
      32          return id, err
      33  }
      34  
      35 -// InsertPersonalOrg creates an org named after the user's emai
         -l, plus its prompt_application_context row.                    
      35 +// InsertPersonalOrg creates an org named after the user's emai
         +l (callers also InsertPromptApplicationContext).               
      36  func (s *UserStore) InsertPersonalOrg(tx *sql.Tx, email string,
           createdByUserID int64) (int64, error) {
      37          var orgID int64
      38          err := tx.QueryRow(`

  Ran 2 shell commands (ctrl+o to expand)

● Bash(cat > internal/api/auth/ensure_cloud_user_test.go <<'EOF'
      package auth…)
  ⎿  ok      github.com/livereview/internal/api/auth 0.315s
  ⎿  Created internal/api/auth/ensure_cloud_user_test.go (+140 -0)
        1 +package auth                                                  
        2 +                                                              
        3 +import (                                                      
        4 +        "database/sql"                                        
        5 +        "encoding/json"                                       
        6 +        "fmt"                                                 
        7 +        "net/http"                                            
        8 +        "net/http/httptest"                                   
        9 +        "strings"                                             
       10 +        "testing"                                             
       11 +        "time"                                                
       12 +                                                              
       13 +        "github.com/golang-jwt/jwt/v5"                        
       14 +        "github.com/labstack/echo/v4"                         
       15 +        "github.com/livereview/internal/database"             
       16 +)                                                             
       17 +                                                              
       18 +const testCloudSecret = "test-cloud-secret"                   
       19 +                                                              
       20 +func cloudTestDB(t *testing.T) *sql.DB {                      
       21 +        t.Helper()                                            
       22 +        db, err := database.NewDB()                           
       23 +        if err != nil {                                       
       24 +                t.Skipf("skipping: no database available: %v",
          + err)                                                         
       25 +        }                                                     
       26 +        t.Cleanup(func() { db.Close() })                      
       27 +        t.Setenv("CLOUD_JWT_SECRET", testCloudSecret)         
       28 +        t.Setenv("DISCORD_SIGNUP_WEBHOOK_URL", "")            
       29 +        return db                                             
       30 +}                                                             
       31 +                                                              
       32 +func hexmosJWT(t *testing.T, email string) string {           
       33 +        t.Helper()                                            
       34 +        claims := JWTClaims{Email: email, RegisteredClaims: jw
          +t.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Ad
          +d(time.Hour))}}                                               
       35 +        s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, cl
          +aims).SignedString([]byte(testCloudSecret))                   
       36 +        if err != nil {                                       
       37 +                t.Fatal(err)                                  
       38 +        }                                                     
       39 +        return s                                              
       40 +}                                                             
     … 100 more lines (ctrl+o to expand)

● Bash(S=/tmp/claude-1000/-home-lovestaco-hex-lr/97eec983-6279-4c09-8016-11f5b5c
      a4153/scratchpad/addstatus.py; python3 $S storage/storage_status.md users
      users/cloud_u…)
  ⎿  Status-doc link check passed for storage/storage_status.md network/net
     work_status.md.
     70:| users.NewUserStore                                     | moved
     … +8 lines (ctrl+o to expand)
  ⎿  Updated storage/storage_status.md (+16 -0)
      185  | providersgithub.ListAppConnectorsExpiringBefore | added | [L
           istAppConnectorsExpiringBefore](providers/github/token_store.g
           o#L169) |
      186  | providersgithub.ListAppConnectorsForOrg | added | [ListAppCo
           nnectorsForOrg](providers/github/token_store.go#L180) |
      187  | providersgithub.SetAppConnectorFlags | added | [SetAppConnec
           torFlags](providers/github/token_store.go#L207) |
      188 +| users.FindUserByEmail | added | [FindUserByEmail](users/clou
          +d_user_store.go#L12) |                                        
      189 +| users.InsertCloudUser | added | [InsertCloudUser](users/clou
          +d_user_store.go#L19) |                                        
      190 +| users.FindOrgIDByName | added | [FindOrgIDByName](users/clou
          +d_user_store.go#L29) |                                        
      191 +| users.InsertPersonalOrg | added | [InsertPersonalOrg](users/
          +cloud_user_store.go#L36) |                                    
      192 +| users.InsertPromptApplicationContext | added | [InsertPrompt
          +ApplicationContext](users/cloud_user_store.go#L49) |          
      193 +| users.SetDefaultOrgIfUnset | added | [SetDefaultOrgIfUnset](
          +users/cloud_user_store.go#L55) |                              
      194 +| users.SetOnboardingAPIKey | added | [SetOnboardingAPIKey](us
          +ers/cloud_user_store.go#L64) |                                
      195 +| users.OwnerRoleID | added | [OwnerRoleID](users/cloud_user_s
          +tore.go#L70) |                                                
      196 +| users.UserRoleID | added | [UserRoleID](users/cloud_user_sto
          +re.go#L77) |                                                  
      197 +| users.InsertFreeUserRole | added | [InsertFreeUserRole](user
          +s/cloud_user_store.go#L83) |                                  
      198 +| users.UpdateUserRole | added | [UpdateUserRole](users/cloud_
          +user_store.go#L91) |                                          
      199 +| users.DefaultOrgID | added | [DefaultOrgID](users/cloud_user
          +_store.go#L97) |                                              
      200 +| users.SetOrgSignupPath | added | [SetOrgSignupPath](users/cl
          +oud_user_store.go#L104) |                                     
      201 +| users.OrgSignupPath | added | [OrgSignupPath](users/cloud_us
          +er_store.go#L110) |                                           
      202 +| providersgithub.FindPreferredConnectorForRepo | added | [Fin
          +dPreferredConnectorForRepo](providers/github/token_store.go#L2
          +35) |                                                         
      203 +| providersgithub.HasAppConnectorForRepo | added | [HasAppConn
          +ectorForRepo](providers/github/token_store.go#L250) |         

● Now the endpoints. First, the pieces they build on: the auto installer,
  repository upsert and the server struct.

  Ran 2 shell commands (ctrl+o to expand)

● Bash(cat >> internal/providers/github/repo_list.go <<'EOF'…)
  ⎿  14:     db *sql.DB
     17:func NewStore(db *sql.DB) *Store {
  ⎿  Updated internal/providers/github/repo_list.go (+25 -0)
      12  
      13          "github.com/livereview/internal/providers"
      14          "github.com/livereview/internal/prsync"
      15 +        networkgithub "github.com/livereview/network/providers/
         +github"                                                        
      16  )
      17  
      18  // gitHubRepositoryListItem mirrors the fields needed from GET 
          /user/repos to
     ...
      112          }
      113          return &providers.RateLimitedError{Provider: "github",
            RetryAfter: retryAfter}
      114  }
      115 +                                                              
      116 +// ListInstallationRepositoriesPage fetches one page of the re
          +positories a GitHub App installation token can                
      117 +// see (GET /installation/repositories). It is the App counter
          +part of ListRepositories, which needs a user PAT.             
      118 +func ListInstallationRepositoriesPage(ctx context.Context, bas
          +eURL, installationToken, cursor string) (*providers.Repository
          +Page, error) {                                                
      119 +        items, next, err := networkgithub.ListInstallationRepo
          +sitories(ctx, apiBaseURL(baseURL), installationToken, cursor) 
      120 +        if err != nil {                                       
      121 +                return nil, err                               
      122 +        }                                                     
      123 +        page := &providers.RepositoryPage{NextCursor: next}   
      124 +        for _, item := range items {                          
      125 +                page.Repositories = append(page.Repositories, 
          +prsync.RepositorySummary{                                     
      126 +                        ProviderRepoID: strconv.FormatInt(item
          +.ID, 10),                                                     
      127 +                        FullName:       item.FullName,        
      128 +                        Name:           item.Name,            
      129 +                        WebURL:         item.HTMLURL,         
      130 +                        CloneURL:       item.CloneURL,        
      131 +                        SSHURL:         item.SSHURL,          
      132 +                        DefaultBranch:  item.DefaultBranch,   
      133 +                        IsPrivate:      item.Private,         
      134 +                        Description:    item.Description,     
      135 +                })                                            
      136 +        }                                                     
      137 +        return page, nil                                      
      138 +}                                                             
  ⎿  Updated storage/providers/pullrequests/upsert.go (+7 -0)
      190          ).Scan(&id)
      191          return id, err
      192  }
      193 +                                                              
      194 +// DeleteRepositoryByProviderID removes a connector's reposito
          +ry row (and, via cascade, its pull requests) when             
      195 +// the provider reports the repository is no longer accessible
          + - e.g. removed from a GitHub App installation.               
      196 +func (s *Store) DeleteRepositoryByProviderID(connectorID int64
          +, providerRepoID string) error {                              
      197 +        _, err := s.db.Exec(`DELETE FROM repositories WHERE co
          +nnector_id = $1 AND provider_repo_id = $2`, connectorID, provi
          +derRepoID)                                                    
      198 +        return err                                            
      199 +}                                                             

● Bash(python3 - <<'EOF'
      p='internal/api/auto_webhook_installer.go'…)
  ⎿  Updated internal/api/auto_webhook_installer.go (+41 -1)
       9          "strings"
      10          "time"
      11  
      12 +        "github.com/livereview/internal/githubapp"             
      13          "github.com/livereview/internal/providers"
      14          "github.com/livereview/internal/providers/azuredevops"
      15          "github.com/livereview/internal/providers/bitbucket"
      16          "github.com/livereview/internal/providers/gitea"
      17          "github.com/livereview/internal/providers/github"
      18          "github.com/livereview/internal/providers/gitlab"
      19 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      20          "github.com/livereview/storage/providers/pullrequests"
      21  )
      22  
     ...
      123                  var page *providers.RepositoryPage
      124                  var err error
      125                  switch {
      126 +                case connector.TokenType == storagegithub.Toke
          +nTypeGitHubApp:                                               
      127 +                        // App installations list through the 
          +installation endpoint, not /user/repos.                       
      128 +                        page, err = awi.listAppInstallationRep
          +ositories(ctx, connector, cursor)                             
      129                  case strings.HasPrefix(connector.Provider, "gi
           thub"):
      130                          page, err = github.ListRepositories(ct
           x, connector.ProviderURL, connector.PATToken, cursor)
      131                  case strings.HasPrefix(connector.Provider, "gi
           tlab"):
     ...
      173          return nil
      174  }
      175  
      176 +// listAppInstallationRepositories lists one page of repositor
          +ies for a GitHub App connector, using a valid                 
      177 +// (possibly just-minted) installation token rather than whate
          +ver pat_token held when the connector was loaded.             
      178 +func (awi *AutoWebhookInstaller) listAppInstallationRepositori
          +es(ctx context.Context, connector *ConnectorDetails, cursor st
          +ring) (*providers.RepositoryPage, error) {                    
      179 +        store := storagegithub.NewTokenStore(awi.db)          
      180 +        rec, err := store.GetGitHubTokenByConnectorID(int64(co
          +nnector.ID))                                                  
      181 +        if err != nil {                                       
      182 +                return nil, err                               
      183 +        }                                                     
      184 +        token, err := githubapp.ConnectorToken(ctx, store, rec
          +)                                                             
      185 +        if err != nil {                                       
      186 +                return nil, err                               
      187 +        }                                                     
      188 +        return github.ListInstallationRepositoriesPage(ctx, co
          +nnector.ProviderURL, token, cursor)                           
      189 +}                                                             
     … 50 more lines (ctrl+o to expand)

  Ran 2 shell commands (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='internal/api/server.go'…)
  ⎿  func (s *Server) GetRepositoryAccess(c echo.Context) error {
             // JWT authentication is already handled by the RequireAuth()
     middlewa
     re
     … +33 lines (ctrl+o to expand)
  ⎿  Updated internal/api/server.go (+6 -0)
      2847          if _, err := s.validateConnectorOwnership(c, connecto
            rId); err != nil {
      2848                  return err
      2849          }
      2850 +        if s.isGitHubAppConnector(connectorId) {             
      2851 +                return c.JSON(http.StatusBadRequest, map[stri
           +ng]string{"error": githubAppManagedMessage})                 
      2852 +        }                                                    
      2853  
      2854          // Get repository access data for this connector
      2855          repositoryData, err := s.fetchAndCacheRepositoryData(
            connectorId, false, false)
     ...
      2924          if _, err := s.validateConnectorOwnership(c, connecto
            rId); err != nil {
      2925                  return err
      2926          }
      2927 +        if s.isGitHubAppConnector(connectorId) {             
      2928 +                return c.JSON(http.StatusBadRequest, map[stri
           +ng]string{"error": githubAppManagedMessage})                 
      2929 +        }                                                    
      2930  
      2931          // Get repository access data for this connector
      2932          repositoryData, err := s.fetchAndCacheRepositoryData(
            connectorId, false, false)
  ⎿  Updated internal/api/github_app_connectors.go (+11 -0)
      69          token.PatToken = apiToken
      70          return nil
      71  }
      72 +                                                               
      73 +const githubAppManagedMessage = "This connection is managed by 
         +the LiveReview GitHub App. Manage repositories on GitHub."     
      74 +                                                               
      75 +// isGitHubAppConnector reports whether the connector is a GitH
         +ub App installation (not a PAT/OAuth connector).               
      76 +func (s *Server) isGitHubAppConnector(connectorID int) bool {  
      77 +        var tokenType sql.NullString                           
      78 +        if err := s.db.QueryRow(`SELECT token_type FROM integra
         +tion_tokens WHERE id = $1`, connectorID).Scan(&tokenType); err 
         +!= nil {                                                       
      79 +                return false                                   
      80 +        }                                                      
      81 +        return tokenType.String == storagegithub.TokenTypeGitHu
         +bApp                                                           
      82 +}                                                              

  Ran 1 shell command (ctrl+o to expand)

● Bash(cat >> storage/providers/pullrequests/upsert.go <<'EOF'…)
  ⎿  package api

     import (
     … +16 lines (ctrl+o to expand)
  ⎿  Updated storage/providers/pullrequests/upsert.go (+18 -0)
      197          _, err := s.db.Exec(`DELETE FROM repositories WHERE co
           nnector_id = $1 AND provider_repo_id = $2`, connectorID, provi
           derRepoID)
      198          return err
      199  }
      200 +                                                              
      201 +// ListRepositoryFullNamesByConnector returns the full names o
          +f a connector's repositories, sorted.                         
      202 +func (s *Store) ListRepositoryFullNamesByConnector(connectorID
          + int64) ([]string, error) {                                   
      203 +        rows, err := s.db.Query(`SELECT full_name FROM reposit
          +ories WHERE connector_id = $1 ORDER BY full_name`, connectorID
          +)                                                             
      204 +        if err != nil {                                       
      205 +                return nil, err                               
      206 +        }                                                     
      207 +        defer rows.Close()                                    
      208 +        var names []string                                    
      209 +        for rows.Next() {                                     
      210 +                var n string                                  
      211 +                if err := rows.Scan(&n); err != nil {         
      212 +                        return nil, err                       
      213 +                }                                             
      214 +                names = append(names, n)                      
      215 +        }                                                     
      216 +        return names, rows.Err()                              
      217 +}                                                             
  ⎿  Updated internal/api/repository_access.go (+21 -3)
      58  //   - error: Any error that occurred during the process
      59  func (s *Server) fetchAndCacheRepositoryData(connectorID int, f
          orceRefresh bool, shouldCache bool) (*RepositoryAccessResponse,
           error) {
      60          // Query the database for the connector information
      61 -        var provider, providerURL, patToken string             
      61 +        var provider, providerURL, patToken, tokenType string  
      62          var cachedDataJSON sql.NullString
      63          var metadataBytes []byte
      64  
      65          query := `
      66 -                SELECT provider, provider_url, pat_token, proje
         -cts_cache, COALESCE(metadata, '{}')                            
      66 +                SELECT provider, provider_url, pat_token, proje
         +cts_cache, COALESCE(metadata, '{}'), COALESCE(token_type, '')  
      67                  FROM integration_tokens
      68                  WHERE id = $1
      69          `
      70  
      71 -        err := s.db.QueryRow(query, connectorID).Scan(&provider
         -, &providerURL, &patToken, &cachedDataJSON, &metadataBytes)    
      71 +        err := s.db.QueryRow(query, connectorID).Scan(&provider
         +, &providerURL, &patToken, &cachedDataJSON, &metadataBytes, &to
         +kenType)                                                       
      72  
      73          if err != nil {
      74                  if err == sql.ErrNoRows {
     ...
       98                  UpdatedAt:          time.Now(),
       99          }
      100  
      101 +        // GitHub App connectors: the repositories are exactly
          + what the installation grants (kept in the                    
      102 +        // repositories table by the sync and the installation
          +_repositories webhook), and /user/repos does not              
      103 +        // work with installation tokens. Webhooks are app-wid
          +e, so every repo counts as automatically connected.           
      104 +        if tokenType == storagegithub.TokenTypeGitHubApp {    
      105 +                names, err := pullrequests.NewStore(s.db).List
          +RepositoryFullNamesByConnector(int64(connectorID))            
      106 +                if err != nil {                               
      107 +                        return nil, fmt.Errorf("database error
          +: %w", err)                                                   
      108 +                }                                             
      109 +                response.Projects = names                     
      110 +                response.ProjectCount = len(names)            
      111 +                for _, n := range names {                     
      112 +                        response.ProjectsWithStatus = append(r
          +esponse.ProjectsWithStatus, ProjectWithStatus{                
      113 +                                ProjectPath: n, WebhookStatus:
          + "automatic", UpdatedAt: time.Now(),                          
      114 +                        })                                    
      115 +                }                                             
      116 +                return response, nil                          
      117 +        }                                                     
     … 4 more lines (ctrl+o to expand)

● Bash(sed -i 's#^\t"github.com/livereview/internal/providers/gitlab"\n)#X#'
      internal/api/repository_access.go && python3 - <<'EOF'
      p='internal/api/repository_access.g…)
  ⎿  56:     TokenType      sql.NullString  `json:"token_type"`
     131:           token_type, scope, expires_at, metadata, created_at,
     update
     d_at,
     … +7 lines (ctrl+o to expand)
  ⎿  Updated internal/api/repository_access.go (+2 -0)
      16          "github.com/livereview/internal/providers/gitea"
      17          "github.com/livereview/internal/providers/github"
      18          "github.com/livereview/internal/providers/gitlab"
      19 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      20 +        "github.com/livereview/storage/providers/pullrequests" 
      21  )
      22  
      23  // ProjectWithStatus represents a project with its webhook stat
          us

● Bash(python3 - <<'EOF'
      p='internal/api/auth/middleware.go'…)  ⎿  }

     // BuildOrgContextFromConnector middleware extracts connector_id from
     … +10 lines (ctrl+o to expand)
  ⎿  Updated internal/api/auth/middleware.go (+39 -28)
      356                                  return echo.NewHTTPError(http.
           StatusBadRequest, "Invalid connector ID")
      357                          }
      358  
      359 -                        // Query integration_tokens to get org
          -_id for this connector                                        
      360 -                        var orgID int64                       
      361 -                        err = am.db.QueryRow(`                
      362 -                                SELECT org_id FROM integration
          -_tokens WHERE id = $1                                         
      363 -                        `, connectorID).Scan(&orgID)          
      364 -                                                              
      365 -                        if err != nil {                       
      366 -                                if err == sql.ErrNoRows {     
      367 -                                        return echo.NewHTTPErr
          -or(http.StatusNotFound, "Connector not found")                
      368 -                                }                             
      369 -                                return echo.NewHTTPError(http.
          -StatusInternalServerError, "Failed to fetch connector")       
      359 +                        if err := am.SetConnectorOrgContext(c,
          + connectorID); err != nil {                                   
      360 +                                return err                    
      361                          }
      362  
      372 -                        // Validate that the organization exis
          -ts and is active                                              
      373 -                        org := &models.Org{}                  
      374 -                        err = am.db.QueryRow(`                
      375 -                                SELECT id, name, description, 
          -created_at, updated_at                                        
      376 -                                FROM orgs WHERE id = $1       
      377 -                        `, orgID).Scan(&org.ID, &org.Name, &or
          -g.Description, &org.CreatedAt, &org.UpdatedAt)                
      363 +                        return next(c)                        
      364 +                }                                             
      365 +        }                                                     
      366 +}                                                             
      367  
      379 -                        if err != nil {                       
      380 -                                if err == sql.ErrNoRows {     
      381 -                                        return echo.NewHTTPErr
          -or(http.StatusNotFound, "Organization not found for this conne
          -ctor")                                                        
      382 -                                }                             
      383 -                                return echo.NewHTTPError(http.
          -StatusInternalServerError, "Failed to fetch organization")    
      384 -                        }                                     
      368 +// SetConnectorOrgContext looks up the org that owns the conne
          +ctor and stores connector_id, the org and org_id              
      369 +// in the request context, exactly as BuildOrgContextFromConne
          +ctor does for a URL-addressed connector. It is                
      370 +// exported for handlers that learn the connector some other w
          +ay (e.g. the GitHub App webhook, which maps an                
      371 +// installation id to its connector). The returned error is an
          + *echo.HTTPError.                                             
      372 +func (am *AuthMiddleware) SetConnectorOrgContext(c echo.Contex
          +t, connectorID int64) error {                                 
     … 40 more lines (ctrl+o to expand)

● Now the state signing and the App endpoints.

  Ran 3 shell commands (ctrl+o to expand)

● Bash(cat > internal/api/github_app_state.go <<'EOF'
      package api…)
  ⎿  ok      github.com/livereview/internal/api      0.069s
  ⎿  Created internal/api/github_app_state.go (+100 -0)
        1 +package api                                                   
        2 +                                                              
        3 +import (                                                      
        4 +        "crypto/hmac"                                         
        5 +        "crypto/rand"                                         
        6 +        "crypto/sha256"                                       
        7 +        "encoding/base64"                                     
        8 +        "encoding/hex"                                        
        9 +        "encoding/json"                                       
       10 +        "errors"                                              
       11 +        "regexp"                                              
       12 +        "strings"                                             
       13 +        "time"                                                
       14 +)                                                             
       15 +                                                              
       16 +// The OAuth/install "state" ties the GitHub round trip to the
          + browser that started it: it is HMAC-signed so it             
       17 +// cannot be forged, expires after ghStateTTL, and its nonce i
          +s mirrored in an HttpOnly cookie so a state copied            
       18 +// to another browser is rejected (login CSRF).               
       19 +                                                              
       20 +const (                                                       
       21 +        ghStateTTL        = 10 * time.Minute                  
       22 +        ghStateCookieName = "lr_gh_state"                     
       23 +        ghModeInstall     = "install"                         
       24 +        ghModeSignin      = "signin"                          
       25 +)                                                             
       26 +                                                              
       27 +var ghSrcPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)   
       28 +                                                              
       29 +type ghState struct {                                         
       30 +        Nonce string `json:"n"`                               
       31 +        Src   string `json:"s"`                               
       32 +        Mode  string `json:"m"`                               
       33 +        Exp   int64  `json:"e"`                               
       34 +}                                                             
       35 +                                                              
       36 +// sanitizeGHSrc keeps the campaign tag short and harmless; an
          +ything else becomes "unknown".                                
       37 +func sanitizeGHSrc(src string) string {                       
       38 +        src = strings.ToLower(strings.TrimSpace(src))         
       39 +        if ghSrcPattern.MatchString(src) {                    
       40 +                return src                                    
     … 60 more lines (ctrl+o to expand)
  ⎿  Created internal/api/github_app_state_test.go (+66 -0)
       1 +package api                                                    
       2 +                                                               
       3 +import (                                                       
       4 +        "strings"                                              
       5 +        "testing"                                              
       6 +        "time"                                                 
       7 +)                                                              
       8 +                                                               
       9 +func TestGHState_RoundTrip(t *testing.T) {                     
      10 +        secret := []byte("s3cret")                             
      11 +        now := time.Now()                                      
      12 +        raw, err := signGHState(ghState{Nonce: "abc", Src: "hom
         +e", Mode: ghModeInstall, Exp: now.Add(ghStateTTL).Unix()}, secr
         +et)                                                            
      13 +        if err != nil {                                        
      14 +                t.Fatal(err)                                   
      15 +        }                                                      
      16 +        got, err := verifyGHState(raw, secret, now)            
      17 +        if err != nil || got.Nonce != "abc" || got.Src != "home
         +" || got.Mode != ghModeInstall {                               
      18 +                t.Fatalf("got (%+v, %v)", got, err)            
      19 +        }                                                      
      20 +}                                                              
      21 +                                                               
      22 +func TestGHState_Rejections(t *testing.T) {                    
      23 +        secret := []byte("s3cret")                             
      24 +        now := time.Now()                                      
      25 +        good, _ := signGHState(ghState{Nonce: "abc", Exp: now.A
         +dd(time.Minute).Unix()}, secret)                               
      26 +        expired, _ := signGHState(ghState{Nonce: "abc", Exp: no
         +w.Add(-time.Second).Unix()}, secret)                           
      27 +        noNonce, _ := signGHState(ghState{Exp: now.Add(time.Min
         +ute).Unix()}, secret)                                          
      28 +        payload, sig, _ := strings.Cut(good, ".")              
      29 +        otherPayload, _, _ := strings.Cut(func() string {      
      30 +                s, _ := signGHState(ghState{Nonce: "evil", Exp:
         + now.Add(time.Minute).Unix()}, secret)                         
      31 +                return s                                       
      32 +        }(), ".")                                              
      33 +                                                               
      34 +        cases := map[string]struct {                           
      35 +                raw    string                                  
      36 +                secret []byte                                  
      37 +        }{                                                     
      38 +                "wrong secret":     {good, []byte("other")},   
      39 +                "expired":          {expired, secret},         
      40 +                "missing nonce":    {noNonce, secret},         
     … 26 more lines (ctrl+o to expand)

  Ran 1 shell command (ctrl+o to expand)

● Bash(cat >> storage/providers/pullrequests/upsert.go <<'EOF'…)
  ⎿  internal/api/github_app_auth.go
     internal/api/server.go
  ⎿  Updated storage/providers/pullrequests/upsert.go (+7 -0)
      215          }
      216          return names, rows.Err()
      217  }
      218 +                                                              
      219 +// DeleteRepositoriesByConnector removes every repository row 
          +of a connector (and, via cascade, their pull                  
      220 +// requests), e.g. when a GitHub App installation is uninstall
          +ed.                                                           
      221 +func (s *Store) DeleteRepositoriesByConnector(connectorID int6
          +4) error {                                                    
      222 +        _, err := s.db.Exec(`DELETE FROM repositories WHERE co
          +nnector_id = $1`, connectorID)                                
      223 +        return err                                            
      224 +}                                                             
  ⎿  Updated internal/api/server.go (+15 -0)
      21          "github.com/labstack/echo/v4/middleware"
      22          "github.com/livereview/internal/aiconnectors"
      23          "github.com/livereview/internal/api/auth"
      24 +        "github.com/livereview/internal/githubapp"             
      25          apimiddleware "github.com/livereview/internal/api/middl
          eware"
      26          "github.com/livereview/internal/api/organizations"
      27          "github.com/livereview/internal/api/users"
     ...
      144          versionInfo            *VersionInfo
      145          deploymentConfig       *DeploymentConfig
      146          authHandlers           *auth.AuthHandlers
      147 +        githubApp              *githubapp.Config // nil unless
          + GITHUB_APP_* is configured                                   
      148 +        githubAppSecret        []byte            // HMAC key f
          +or the GitHub App OAuth state                                 
      149          tokenService           *auth.TokenService
      150          userHandlers           *users.UserHandlers
      151          userService            *users.UserService
     ...
      367                  versionInfo:            versionInfo,
      368                  deploymentConfig:       deploymentConfig,
      369                  authHandlers:           authHandlers,
      370 +                githubApp:              githubapp.Load(),     
      371 +                githubAppSecret:        []byte(jwtSecret),    
      372                  tokenService:           tokenService,
      373                  userHandlers:           userHandlers,
      374                  userService:            userService,
     ...
      1208          // Cloud user ensure endpoint (now public; handler pe
            rforms CLOUD_JWT_SECRET validation)
      1209          public.POST("/auth/ensure-cloud-user", s.authHandlers
            .EnsureCloudUser)
      1210  
      1211 +        // GitHub App sign-in/install ("Continue with GitHub"
           +); only registered when the App is configured.               
      1212 +        if s.githubApp != nil {                              
      1213 +                s.registerGitHubAppRoutes(public, v1, authMid
           +dleware)                                                     
      1214 +        }                                                    
      1215 +                                                             
      1216          // Teams bot messages endpoint (public — Bot Framewor
            k sends activities here)
      1217          s.echo.POST("/api/messages", func(c echo.Context) err
            or {
      1218                  if s.teamsHandler == nil {
     ...
      3066                  "version":        s.versionInfo.Version,
      3067                  "mode":           s.deploymentConfig.Mode,
      3068                  "isAtlasEnabled": s.deploymentConfig.AtlasEna
            bled,
      3069 +                // The UI shows "Continue with GitHub" only w
           +hen the App is configured; it holds no App ids or URLs itself
           +.                                                            
      3070 +                "githubAppEnabled": s.githubApp != nil,      
      3071          }
     … 6 more lines (ctrl+o to expand)
  ⎿  … 1 more file changed

● Bash(gofmt -w internal/api/github_app_auth.go && go build ./internal/... 2>&1
      | head; go vet ./internal/api/ 2>&1 | head -5)
  ⎿  Updated internal/api/github_app_auth.go (+0 -1)
      538          }
      539          return c.HTMLBlob(status, buf.Bytes())
      540  }
      541 -                                                              

  Ran 1 shell command (ctrl+o to expand)

● Bash(grep -n "deploymentConfig " internal/api/server.go | head -3 && cat >
      internal/api/github_app_auth_test.go <<'EOF'
      package api…)
  ⎿  145:    deploymentConfig       *DeploymentConfig
     227:    deploymentConfig := getDeploymentConfig()
     3002:   deploymentConfig := s.deploymentConfig
     … +18 lines (ctrl+o to expand)
  ⎿  1 file changed (binary, mode only or too large to show)

● Bash(python3 - <<'EOF'
      p='internal/githubapp/token.go'…)
  ⎿  ok      github.com/livereview/internal/githubapp        0.707s
     ok      github.com/livereview/internal/api      3.068s
  ⎿  Updated internal/githubapp/token.go (+10 -4)
      23  // ConnectorToken returns a usable API token for any GitHub con
          nector. PAT rows return pat_token unchanged;
      24  // GITHUB_APP rows return the cached installation token, mintin
          g a new one when it expires within 5 minutes.
      25  func ConnectorToken(ctx context.Context, store *storagegithub.T
          okenStore, rec *storagegithub.IntegrationTokenRecord) (string, 
          error) {
      26 +        return ConnectorTokenWith(ctx, Load(), store, rec)     
      27 +}                                                              
      28 +                                                               
      29 +// ConnectorTokenWith is ConnectorToken for callers that alread
         +y hold the App configuration.                                  
      30 +func ConnectorTokenWith(ctx context.Context, cfg *Config, store
         + *storagegithub.TokenStore, rec *storagegithub.IntegrationToken
         +Record) (string, error) {                                      
      31          if rec.TokenType != storagegithub.TokenTypeGitHubApp {
      32                  return rec.PatToken, nil
      33          }
      29 -        return connectorToken(ctx, Load(), store, rec, false)  
      34 +        return connectorToken(ctx, cfg, store, rec, false)     
      35  }
      36  
      37  // RefreshConnectorToken mints and stores a new installation to
          ken regardless of the cached one's age.
     ...
      46  }
      47  
      48  func connectorToken(ctx context.Context, cfg *Config, store *st
          oragegithub.TokenStore, rec *storagegithub.IntegrationTokenReco
          rd, force bool) (string, error) {
      44 -        if cfg == nil {                                        
      45 -                return "", ErrNotConfigured                    
      46 -        }                                                      
      49          var meta appConnectorMeta
      50          if len(rec.Metadata) > 0 {
      51                  if err := json.Unmarshal(rec.Metadata, &meta); 
          err != nil {
     ...
      59                  return rec.PatToken, nil
      60          }
      61  
      62 +        // Configuration is only needed to mint; a still-valid 
         +cached token needs no App credentials.                         
      63 +        if cfg == nil {                                        
      64 +                return "", ErrNotConfigured                    
      65 +        }                                                      
      66          installationID := meta.InstallationID
      67          if installationID == 0 {
      68                  return "", fmt.Errorf("connector %d: no install
          ation id in metadata", rec.ID)
  ⎿  Updated internal/api/github_app_auth.go (+1 -1)
      465                  }
      466                  _ = json.Unmarshal(rec.Metadata, &meta)
      467  
      468 -                token, err := githubapp.ConnectorToken(ctx, st
          -ore, rec)                                                     
      468 +                token, err := githubapp.ConnectorTokenWith(ctx
          +, cfg, store, rec)                                            
      469                  if err != nil {
      470                          log.Printf("[GitHubApp] demo: connecto
           r %d unusable: %v", rec.ID, err)
      471                          continue

● Bash(python3 - <<'EOF'
      p='internal/api/webhook_orchestrator_v2.go'…)
  ⎿  3:import (
     27:)
     343:    has, err :=
     storagegithub.NewTokenStore(wo.server.db).HasAppConnec
     … +2 lines (ctrl+o to expand)
  ⎿  Updated internal/api/webhook_orchestrator_v2.go (+36 -9)
      223          }
      224          stampConnectorID(event, connectorID)
      225  
      226 +        orgID, ok := auth.GetOrgIDFromContext(c)              
      227 +        if !ok {                                              
      228 +                log.Printf("[ERROR] Org ID not found in contex
          +t - middleware configuration error")                          
      229 +                return c.JSON(http.StatusInternalServerError, 
          +map[string]string{                                            
      230 +                        "error": "internal_error",            
      231 +                })                                            
      232 +        }                                                     
      233 +                                                              
      234 +        // Overlap rule: when the org's GitHub App also covers
          + this repository, the App's webhook delivers the same         
      235 +        // event, so the PAT connector's copy is dropped to av
          +oid reviewing and replying twice.                             
      236 +        if wo.supersededByGitHubApp(event, connectorID, orgID)
          + {                                                            
      237 +                log.Printf("[INFO] Ignoring PAT webhook for %s
          +: repository is covered by the org's GitHub App (connector_id=
          +%d)", event.Repository.FullName, connectorID)                 
      238 +                return c.JSON(http.StatusOK, map[string]string
          +{                                                             
      239 +                        "status":   "ignored",                
      240 +                        "provider": providerName,             
      241 +                        "reason":   "superseded_by_github_app"
          +,                                                             
      242 +                })                                            
      243 +        }                                                     
      244 +                                                              
      245          // Phase 3: Response Warrant Check
      246          botInfo, err := wo.getBotUserInfo(event)
      247          if err != nil {
     ...
      281  
      282          log.Printf("[INFO] Response warranted: scenario=%s", s
           cenario.Type)
      283  
      265 -        // Extract org ID from context (set by BuildOrgContext
          -FromConnector middleware)                                     
      266 -        orgID, ok := auth.GetOrgIDFromContext(c)              
      267 -        if !ok {                                              
      268 -                log.Printf("[ERROR] Org ID not found in contex
          -t - middleware configuration error")                          
      269 -                return c.JSON(http.StatusInternalServerError, 
          -map[string]string{                                            
      270 -                        "error": "internal_error",            
      271 -                })                                            
      272 -        }                                                     
      273 -                                                              
      284          // Webhook signature validation (if provider supports 
           it)
      285          type signatureValidator interface {
      286                  ValidateWebhookSignature(connectorID int64, he
           aders map[string]string, body []byte) bool
     … 23 more lines (ctrl+o to expand)

● Bash(sed -i 's#^\tstoragelicense
      "github.com/livereview/storage/license"$#\tstoragegithub
      "github.com/livereview/storage/providers/github"\n\tstoragelicense
      "github.…)
  ⎿  internal/api/webhook_orchestrator_v2.go
  ⎿  Updated internal/api/webhook_orchestrator_v2.go (+1 -0)
      23          githubinput "github.com/livereview/internal/provider_in
          put/github"
      24          gitlabinput "github.com/livereview/internal/provider_in
          put/gitlab"
      25          "github.com/livereview/internal/prsync"
      26 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      27          storagelicense "github.com/livereview/storage/license"
      28  )
      29  

● Bash(gofmt -d internal/api/webhook_orchestrator_v2.go | head -20; gofmt -w
      internal/api/webhook_orchestrator_v2.go; go build ./internal/... && echo
      ok)
  ⎿  diff internal/api/webhook_orchestrator_v2.go.orig internal/api/webhook
     _orchestrator_v2.go
     --- internal/api/webhook_orchestrator_v2.go.orig
     … +13 lines (ctrl+o to expand)
  ⎿  Updated internal/api/webhook_orchestrator_v2.go (+1 -1)
      23          githubinput "github.com/livereview/internal/provider_in
          put/github"
      24          gitlabinput "github.com/livereview/internal/provider_in
          put/gitlab"
      25          "github.com/livereview/internal/prsync"
      26 -        storagegithub "github.com/livereview/storage/providers/
         -github"                                                        
      26          storagelicense "github.com/livereview/storage/license"
      27 +        storagegithub "github.com/livereview/storage/providers/
         +github"                                                        
      28  )
      29  
      30  // Phase 8: Webhook Orchestrator for coordinating provider and 
          processing layers

● Bash(cat > storage/providers/github/token_store_test.go <<'EOF'
      package github…)
  ⎿  ok      github.com/livereview/storage/providers/github  0.157s
  ⎿  Created storage/providers/github/token_store_test.go (+133 -0)
        1 +package github                                                
        2 +                                                              
        3 +import (                                                      
        4 +        "database/sql"                                        
        5 +        "fmt"                                                 
        6 +        "testing"                                             
        7 +        "time"                                                
        8 +                                                              
        9 +        "github.com/livereview/internal/database"             
       10 +)                                                             
       11 +                                                              
       12 +func testDB(t *testing.T) *sql.DB {                           
       13 +        t.Helper()                                            
       14 +        db, err := database.NewDB()                           
       15 +        if err != nil {                                       
       16 +                t.Skipf("skipping: no database available: %v",
          + err)                                                         
       17 +        }                                                     
       18 +        t.Cleanup(func() { db.Close() })                      
       19 +        return db                                             
       20 +}                                                             
       21 +                                                              
       22 +func testOrg(t *testing.T, db *sql.DB) int64 {                
       23 +        t.Helper()                                            
       24 +        var id int64                                          
       25 +        name := fmt.Sprintf("tokenstore-test-%d", time.Now().U
          +nixNano())                                                    
       26 +        if err := db.QueryRow(`INSERT INTO orgs (name, descrip
          +tion) VALUES ($1, 'token store test') RETURNING id`, name).Sca
          +n(&id); err != nil {                                          
       27 +                t.Fatal(err)                                  
       28 +        }                                                     
       29 +        t.Cleanup(func() {                                    
       30 +                _, _ = db.Exec(`DELETE FROM repositories WHERE
          + org_id = $1`, id)                                            
       31 +                _, _ = db.Exec(`DELETE FROM integration_tokens
          + WHERE org_id = $1`, id)                                      
       32 +                _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1
          +`, id)                                                        
       33 +        })                                                    
       34 +        return id                                             
       35 +}                                                             
       36 +                                                              
       37 +func addRepo(t *testing.T, db *sql.DB, orgID, connectorID int6
          +4, fullName string) {                                         
       38 +        t.Helper()                                            
       39 +        _, err := db.Exec(`                                   
       40 +                INSERT INTO repositories (org_id, connector_id
          +, provider, provider_repo_id, full_name, name, web_url)       
     … 93 more lines (ctrl+o to expand)

● Bash(grep -n "pullrequests" storage/storage_status.md | head -3;
      S=/tmp/claude-1000/-home-lovestaco-hex-lr/97eec983-6279-4c09-8016-11f5b5ca
      4153/scratchpad/addstatus.…)
  ⎿  Status-doc link check passed for storage/storage_status.md network/net
     work_status.md.
  ⎿  Updated storage/storage_status.md (+3 -0)
      201  | users.OrgSignupPath | added | [OrgSignupPath](users/cloud_us
           er_store.go#L110) |
      202  | providersgithub.FindPreferredConnectorForRepo | added | [Fin
           dPreferredConnectorForRepo](providers/github/token_store.go#L2
           35) |
      203  | providersgithub.HasAppConnectorForRepo | added | [HasAppConn
           ectorForRepo](providers/github/token_store.go#L250) |
      204 +| pullrequests.DeleteRepositoryByProviderID | added | [DeleteR
          +epositoryByProviderID](providers/pullrequests/upsert.go#L196) 
          +|                                                             
      205 +| pullrequests.ListRepositoryFullNamesByConnector | added | [L
          +istRepositoryFullNamesByConnector](providers/pullrequests/upse
          +rt.go#L202) |                                                 
      206 +| pullrequests.DeleteRepositoriesByConnector | added | [Delete
          +RepositoriesByConnector](providers/pullrequests/upsert.go#L221
          +) |                                                           

  Ran 4 shell commands (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='internal/api/github_bot_user_test.go'…)
  ⎿  ok      github.com/livereview/internal/api      0.854s
  ⎿  Updated internal/api/github_bot_user_test.go (+15 -4)
      15                  t.Fatalf("github provider not initialized")
      16          }
      17  
      18 -        insertIntegrationToken(t, server.db, "github", "https:/
         -/api.github.com", "pat-gh", map[string]interface{}{"installatio
         -n_id": 42})                                                    
      18 +        connectorID := insertIntegrationToken(t, server.db, "gi
         +thub", "https://api.github.com", "pat-gh", map[string]interface
         +{}{"installation_id": 42})                                     
      19  
      20          stubHTTPTransport(t, func(req *http.Request) (*http.Res
          ponse, error) {
      21                  if req.URL.Host != "api.github.com" || req.URL.
          Path != "/user" {
     ...
      33                  return resp, nil
      34          })
      35  
      36 -        repo := coreprocessor.UnifiedRepositoryV2{FullName: "ow
         -ner/repo"}                                                     
      36 +        // The webhook orchestrator stamps the receiving connec
         +tor on the event; the token is resolved from it.               
      37 +        repo := coreprocessor.UnifiedRepositoryV2{FullName: "ow
         +ner/repo", Metadata: map[string]interface{}{"connector_id": con
         +nectorID}}                                                     
      38          info, err := server.githubProviderV2.GetBotUserInfo(rep
          o)
      39          if err != nil {
      40                  t.Fatalf("expected bot info, got error: %v", er
          r)
     ...
      57                  t.Fatalf("github provider not initialized")
      58          }
      59  
      59 -        insertIntegrationToken(t, server.db, "github", "https:/
         -/api.github.com", "pat-gh-error", nil)                         
      60 +        connectorID := insertIntegrationToken(t, server.db, "gi
         +thub", "https://api.github.com", "pat-gh-error", nil)          
      61  
      62          stubHTTPTransport(t, func(req *http.Request) (*http.Res
          ponse, error) {
      63                  resp := &http.Response{
     ...
      68                  return resp, nil
      69          })
      70  
      70 -        repo := coreprocessor.UnifiedRepositoryV2{FullName: "ow
         -ner/repo"}                                                     
      71 +        repo := coreprocessor.UnifiedRepositoryV2{FullName: "ow
         +ner/repo", Metadata: map[string]interface{}{"connector_id": con
         +nectorID}}                                                     
      72          if _, err := server.githubProviderV2.GetBotUserInfo(rep
          o); err == nil {
      73                  t.Fatalf("expected error when GitHub API respon
          ds with failure")
      74          }
      75  }
      76 +                                                               
      77 +// Without a connector on the event there is no token to use: n
         +ever fall back to some other tenant's.                         
      78 +func TestGitHubProvider_GetBotUserInfo_RequiresConnector(t *tes
         +ting.T) {                                                      
      79 +        server := setupBotUserTestServer(t)                    
      80 +        insertIntegrationToken(t, server.db, "github", "https:/
         +/api.github.com", "pat-gh-other", nil)                         
      81 +                                                               
     … 4 more lines (ctrl+o to expand)

  Searched for 1 pattern, ran 1 shell command (ctrl+o to expand)

● The backend is complete behind the config flag. Now the UI. I'm reading the
  design guide and the existing patterns first.

  Read 1 file, listed 1 directory, ran 7 shell commands (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='internal/api/github_app_auth.go'…)
  ⎿  ok      github.com/livereview/internal/api      0.418s
  ⎿  Updated internal/api/github_app_auth.go (+13 -4)
      451                  return c.JSON(http.StatusInternalServerError, 
           map[string]string{"error": "failed to load GitHub connections"
           })
      452          }
      453  
      454 +        // ?light=1 answers only "is a GitHub App connected?" 
          +(no GitHub calls) - used for nav visibility and banners.      
      455 +        light := c.QueryParam("light") == "1"                 
      456 +                                                              
      457          ctx, cancel := context.WithTimeout(c.Request().Context
           (), 20*time.Second)
      458          defer cancel()
      459          reposForPulls := []string{}
     ...
      468                  }
      469                  _ = json.Unmarshal(rec.Metadata, &meta)
      470  
      468 -                token, err := githubapp.ConnectorTokenWith(ctx
          -, cfg, store, rec)                                            
      469 -                if err != nil {                               
      470 -                        log.Printf("[GitHubApp] demo: connecto
          -r %d unusable: %v", rec.ID, err)                              
      471 -                        continue                              
      471 +                var token string                              
      472 +                if !light {                                   
      473 +                        var err error                         
      474 +                        if token, err = githubapp.ConnectorTok
          +enWith(ctx, cfg, store, rec); err != nil {                    
      475 +                                log.Printf("[GitHubApp] demo: 
          +connector %d unusable: %v", rec.ID, err)                      
      476 +                                continue                      
      477 +                        }                                     
      478                  }
      479                  resp.Connected = true
      480                  resp.Installations = append(resp.Installations
           , githubAppDemoInstallation{
      481                          ConnectorID: rec.ID, AccountLogin: met
           a.AccountLogin, AccountType: meta.AccountType,
      482                          RepositorySelection: meta.RepositorySe
           lection, SettingsURL: cfg.InstallationSettingsURL(meta.Account
           Type, meta.AccountLogin, meta.InstallationID),
      483                  })
      484 +                if light {                                    
      485 +                        continue                              
      486 +                }                                             
      487                  repos, _, err := networkgithub.ListInstallatio
           nRepositories(ctx, cfg.APIBaseURL(), token, "")
      488                  if err != nil {
      489                          log.Printf("[GitHubApp] demo: listing 
           repositories for connector %d failed: %v", rec.ID, err)

  Ran 1 shell command (ctrl+o to expand)

● Bash(mkdir -p ui/src/pages/GetStarted && cat > ui/src/api/githubApp.ts <<'EOF'
      import apiClient from './apiClient';…)
  ⎿  ui/src/hooks/useOrgContext.tsx:26:export const useOrgContext = () => {
     678:  Git: () => (
     37:export const Button: React.FC<ButtonProps> = ({
     … +30 lines (ctrl+o to expand)
  ⎿  Created ui/src/api/githubApp.ts (+57 -0)
       1 +import apiClient from './apiClient';                           
       2 +                                                               
       3 +export interface GitHubAppInstallation {                       
       4 +  connector_id: number;                                        
       5 +  account_login: string;                                       
       6 +  account_type: string;                                        
       7 +  repository_selection: string;                                
       8 +  settings_url: string;                                        
       9 +}                                                              
      10 +                                                               
      11 +export interface GitHubAppRepository {                         
      12 +  full_name: string;                                           
      13 +  web_url: string;                                             
      14 +  private: boolean;                                            
      15 +}                                                              
      16 +                                                               
      17 +export interface GitHubAppPullRequest {                        
      18 +  repo_full_name: string;                                      
      19 +  number: number;                                              
      20 +  title: string;                                               
      21 +  web_url: string;                                             
      22 +  author: string;                                              
      23 +  updated_at: string;                                          
      24 +}                                                              
      25 +                                                               
      26 +export interface GitHubAppDemo {                               
      27 +  connected: boolean;                                          
      28 +  installations: GitHubAppInstallation[];                      
      29 +  repositories: GitHubAppRepository[];                         
      30 +  open_pull_requests: GitHubAppPullRequest[];                  
      31 +}                                                              
      32 +                                                               
      33 +/**                                                            
      34 + * What the GitHub App get-started page shows for the current o
         +rg: installations, repositories and open pull                  
      35 + * requests. `light` skips the GitHub calls and only answers wh
         +ether an App is connected (nav/banner checks).                 
      36 + */                                                            
      37 +export const getGitHubAppDemo = (light = false): Promise<GitHub
         +AppDemo> =>                                                    
      38 +  apiClient.get<GitHubAppDemo>(`/api/v1/github-app/demo${light 
         +? '?light=1' : ''}`);                                          
      39 +                                                               
      40 +export interface PublicUIConfig {                              
     … 17 more lines (ctrl+o to expand)
  ⎿  Created ui/src/hooks/useGitHubAppConnected.ts (+22 -0)
       1 +import { useQuery } from '@tanstack/react-query';              
       2 +import { getGitHubAppDemo } from '../api/githubApp';           
       3 +import { useAppSelector } from '../store/configureStore';      
       4 +import { isCloudMode } from '../utils/deploymentMode';         
       5 +                                                               
       6 +export const GITHUB_APP_CONNECTED_QUERY_KEY = ['github-app', 'c
         +onnected'] as const;                                           
       7 +                                                               
       8 +/**                                                            
       9 + * Whether the current org has a LiveReview GitHub App connecti
         +on. Uses the light variant of the endpoint (no                 
      10 + * GitHub calls). Any failure - including a 404 when the server
         + has no GitHub App configured - means "not connected".         
      11 + */                                                            
      12 +export const useGitHubAppConnected = (): boolean => {          
      13 +  const isAuthenticated = useAppSelector((state) => state.Auth.
         +isAuthenticated);                                              
      14 +  const { data } = useQuery({                                  
      15 +    queryKey: GITHUB_APP_CONNECTED_QUERY_KEY,                  
      16 +    queryFn: () => getGitHubAppDemo(true),                     
      17 +    enabled: isCloudMode() && isAuthenticated,                 
      18 +    staleTime: 60_000,                                         
      19 +    retry: false,                                              
      20 +  });                                                          
      21 +  return Boolean(data?.connected);                             
      22 +};                                                             

  Searched for 1 pattern (ctrl+o to expand)

● Now the two pages.

● Bash(cat > ui/src/pages/Auth/GhLogin.tsx <<'EOF'
      import React, { useEffect, useMemo, useState } from 'react';…)
  ⎿      ChangeUserRolePayload
     } from '../store/Organizations/types';
     … +46 lines (ctrl+o to expand)
  ⎿  Created ui/src/pages/Auth/GhLogin.tsx (+126 -0)
        1 +import React, { useEffect, useMemo, useState } from 'react';  
        2 +import { useLocation, useNavigate } from 'react-router-dom';  
        3 +import { Button, Icons } from '../../components/UIPrimitives';
        4 +import { FullScreenLoader } from '../../components/FullScreenL
          +oader';                                                       
        5 +import { getPublicUIConfig, githubAppStartUrl } from '../../ap
          +i/githubApp';                                                 
        6 +import { isCloudMode } from '../../utils/deploymentMode';     
        7 +                                                              
        8 +// "Continue with GitHub": one GitHub screen installs the Live
          +Review GitHub App and signs the user in. This page            
        9 +// only starts that flow; the server (see /api/v1/auth/github-
          +app/*) does the rest and ends in the normal Cloud             
       10 +// login. It is shown to logged-out visitors at /gh-login (see
          + App.tsx); logged-in visitors are sent on to                  
       11 +// /get-started/github instead.                               
       12 +                                                              
       13 +const SRC_PATTERN = /^[a-z0-9_-]{1,32}$/;                     
       14 +                                                              
       15 +// Same destination the server redirects to after login; kept 
          +in sessionStorage because the GitHub round trip               
       16 +// is a full-page navigation. The helper in utils/authHelpers 
          +reads it back after login.                                    
       17 +export const GET_STARTED_PATH = '#/get-started/github';       
       18 +                                                              
       19 +const GhLogin: React.FC = () => {                             
       20 +    const location = useLocation();                           
       21 +    const navigate = useNavigate();                           
       22 +    // null = still asking the server whether the GitHub App i
          +s configured.                                                 
       23 +    const [enabled, setEnabled] = useState<boolean | null>(nul
          +l);                                                           
       24 +    const [githubBase, setGithubBase] = useState<string>(''); 
       25 +                                                              
       26 +    const { src, mr } = useMemo(() => {                       
       27 +        const params = new URLSearchParams(location.search);  
       28 +        const rawSrc = (params.get('src') || '').toLowerCase()
          +;                                                             
       29 +        return { src: SRC_PATTERN.test(rawSrc) ? rawSrc : 'dir
          +ect', mr: params.get('mr') || '' };                           
       30 +    }, [location.search]);                                    
       31 +                                                              
       32 +    useEffect(() => {                                         
       33 +        let cancelled = false;                                
       34 +        getPublicUIConfig()                                   
       35 +            .then((config) => {                               
       36 +                if (cancelled) return;                        
       37 +                setEnabled(Boolean(config.githubAppEnabled) &&
          + isCloudMode());                                              
       38 +                setGithubBase(config.githubAppBaseUrl || ''); 
       39 +            })                                                
       40 +            .catch(() => {                                    
     … 86 more lines (ctrl+o to expand)
  Searched for 1 pattern (ctrl+o to expand)

● Bash(grep -n "AlertVariant =" ui/src/components/UIPrimitives.tsx; grep -n
      "useDashboardQuery" ui/src/api/dashboard.ts | head -3; cat >
      ui/src/pages/GetStarted/GetSta…)
  ⎿  612:type AlertVariant = 'info' | 'success' | 'warning' | 'error';
     227:export function useDashboardQuery() {
     done
  ⎿  Created ui/src/pages/GetStarted/GetStartedGitHub.tsx (+243 -0)
        1 +import React, { useMemo, useState } from 'react';             
        2 +import { useLocation, useNavigate } from 'react-router-dom';  
        3 +import { useQuery } from '@tanstack/react-query';             
        4 +import { Alert, Button, Card, Icons, PageHeader, Spinner } fro
          +m '../../components/UIPrimitives';                            
        5 +import { ErrorBoundary } from '../../components/ErrorBoundary'
          +;                                                             
        6 +import { useDashboardQuery } from '../../api/dashboard';      
        7 +import { getGitHubAppDemo, githubAppStartUrl } from '../../api
          +/githubApp';                                                  
        8 +import { triggerReview } from '../../api/reviews';            
        9 +import { useAppDispatch } from '../../store/configureStore';  
       10 +import { logout } from '../../store/Auth/reducer';            
       11 +                                                              
       12 +// First-run page for orgs connected through the LiveReview Gi
          +tHub App: repositories are connected, pick (or                
       13 +// paste) a pull request, and run the first review. Everything
          + is read from the server; no GitHub URLs or ids               
       14 +// live here.                                                 
       15 +                                                              
       16 +const MAX_REPOS_SHOWN = 10;                                   
       17 +                                                              
       18 +const Step: React.FC<{                                        
       19 +    index: number;                                            
       20 +    title: string;                                            
       21 +    done?: boolean;                                           
       22 +    isLast?: boolean;                                         
       23 +    children: React.ReactNode;                                
       24 +}> = ({ index, title, done = false, isLast = false, children }
          +) => (                                                        
       25 +    <li className="flex gap-4">                               
       26 +        <div className="flex flex-col items-center">          
       27 +            <span                                             
       28 +                className={`flex-shrink-0 flex items-center ju
          +stify-center w-6 h-6 rounded-full text-xs font-semibold text-w
          +hite ${                                                       
       29 +                    done ? 'bg-green-600' : 'bg-blue-600'     
       30 +                }`}                                           
       31 +            >                                                 
       32 +                {done ? <Icons.Success /> : index}            
       33 +            </span>                                           
       34 +            {!isLast && <div className="w-px flex-1 my-1 bg-sl
          +ate-700/70" />}                                               
       35 +        </div>                                                
       36 +        <div className="flex-1 pb-6 min-w-0">                 
       37 +            <h3 className="text-sm font-semibold text-white">{
          +title}</h3>                                                   
       38 +            <div className="mt-1 text-sm text-slate-300">{chil
          +dren}</div>                                                   
       39 +        </div>                                                
       40 +    </li>                                                     
     … 203 more lines (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='ui/src/App.tsx'…)
  ⎿  import './styles/index.css';
     import './styles/custom.css';
     import './styles/darkmode.css';
     … +41 lines (ctrl+o to expand)
  ⎿  Updated ui/src/App.tsx (+9 -1)
      25  const PullRequestReviewTrigger = React.lazy(() => import('./pag
          es/Reviews/PullRequestReviewTrigger'));
      26  const Login = React.lazy(() => import('./pages/Auth/Login'));
      27  const SelfHosted = React.lazy(() => import('./pages/Auth/SelfHo
          sted'));
      28 +const GhLogin = React.lazy(() => import('./pages/Auth/GhLogin')
         +);                                                             
      29 +const GetStartedGitHub = React.lazy(() => import('./pages/GetSt
         +arted/GetStartedGitHub'));                                     
      30  const Setup = React.lazy(() => import('./pages/Setup/Setup'));
      31  const CodeHostCallback = React.lazy(() => import('./pages/Auth/
          CodeHostCallback'));
      32  const OAuthCallbackHandler = React.lazy(() => import('./pages/A
          uth/OAuthCallbackHandler'));
     ...
      133              hashPath !== '' && 
      134              hashPath !== '#/' && 
      135              !hashPath.startsWith('#/oauth-callback') && 
      136 +            !hashPath.startsWith('#/gh-login') &&             
      137              !hashPath.startsWith('#/admin');
      138          
      139          if (isProtectedRoute && !isAuthenticated) {
     ...
      291      } else if (!isAuthenticated) {
      292          // Allow email/admin login screen only when explicitly
            visiting /admin in cloud
      293          // (needed for initial setup or troubleshooting); othe
           rwise use cloud login.
      291 -        if (isCloudMode() && location.pathname === '/admin') {
      294 +        if (location.pathname.startsWith('/gh-login')) {      
      295 +            // "Continue with GitHub" landing page; shows the 
          +normal login itself when the feature is off.                  
      296 +            body = <GhLogin />;                               
      297 +        } else if (isCloudMode() && location.pathname === '/ad
          +min') {                                                       
      298              body = <SelfHosted />;
      299          } else {
      300              body = <Login />;
     ...
      317                              <Routes>
      318                                  <Route path="/" element={<Home
           WithOAuthCheck />} />
      319                                  <Route path="/dashboard" eleme
           nt={<Dashboard />} />
      320 +                                <Route path="/gh-login" elemen
          +t={<Navigate to={{ pathname: '/get-started/github', search: lo
          +cation.search }} replace />} />                               
      321 +                                <Route path="/get-started/gith
          +ub" element={<GetStartedGitHub />} />                         
      322                                  <Route path="/subscribe" eleme
           nt={<Subscribe />} />
      323                                  <Route path="/subscribe/manage
           " element={<LicenseManagement />} />
      324                                  <Route path="/subscribe/subscr
           iptions/:id/assign" element={<LicenseAssignment />} />

● Bash(python3 - <<'EOF'
      p='ui/src/index.tsx'…)
  ⎿  21:import { shortcutKeyLabel } from '../../utils/platform';
     22:import { NotificationBell } from '../Notifications/NotificationBell
     ';
     … +8 lines (ctrl+o to expand)
  ⎿  Updated ui/src/index.tsx (+7 -0)
      17  import { injectStore } from './api/apiClient';
      18  import { injectStore as injectNotifyStore } from './utils/notif
          y';
      19  
      20 +// /gh-login is a shareable path ("Continue with GitHub"), but 
         +the app is a HashRouter: move it into the hash,                
      21 +// keeping the query string (src, mr). The Go UI server and the
         + webpack dev server both already serve index.html              
      22 +// for this path.                                              
      23 +if (typeof window !== 'undefined' && /^\/gh-login\/?$/.test(win
         +dow.location.pathname)) {                                      
      24 +    window.location.replace('/#/gh-login' + window.location.sea
         +rch);                                                          
      25 +}                                                              
      26 +                                                               
      27  type ClarityFunction = ((...args: unknown[]) => void) & {
      28      q?: IArguments[];
      29  };
  ⎿  Updated ui/src/components/Navbar/megaMenuData.ts (+3 -0)
       7      hasOrg: boolean;
       8      orgRole?: string;
       9      isDevMode: boolean;
      10 +    // The org has a LiveReview GitHub App connection (Cloud "C
         +ontinue with GitHub").                                         
      11 +    hasGitHubApp?: boolean;                                    
      12  };
      13  
      14  export type MegaMenuLinkNode = {
     ...
      144          items: [
      145              link('List Reviews', React.createElement(Icons.Lis
           t), '/reviews'),
      146              link('Create Review', React.createElement(Icons.Ad
           d), '/reviews/new'),
      147 +            link('Get Started with GitHub', React.createElemen
          +t(Icons.GitHub), '/get-started/github', (ctx) => Boolean(ctx.h
          +asGitHubApp)),                                                
      148              link('Create via Chatbot', React.createElement(Ico
           ns.Chat), '/chat?prefill=' + encodeURIComponent('Trigger a rev
           iew for <paste PR link here>')),
      149              link('Chat Debug', React.createElement(Icons.Chat)
           , '/chat-debug', (ctx) => ctx.isDevMode),
      150              link('Create via CLI', React.createElement(Icons.D
           ownload), '/reviews/create-cli'),
  ⎿  Updated ui/src/components/Navbar/Navbar.tsx (+2 -0)
      618      const megaMenuPanelRef = useRef<HTMLDivElement>(null);
      619      const wasMegaMenuOpenRef = useRef(false);
      620      const { isDevMode } = useSystemInfo();
      621 +    const hasGitHubApp = useGitHubAppConnected();             
      622      // On the chat page the sidebar is a fixed rail sitting in
            front of the
      623      // navbar's own left edge - inset the navbar's content by 
           the sidebar's
      624      // current width so the logo is never physically covered b
           y it, instead
     ...
      650          hasOrg: Boolean(currentOrg),
      651          orgRole: currentOrg?.role,
      652          isDevMode,
      653 +        hasGitHubApp,                                         
      654      };
      655  
      656      const filteredMegaMenuSections = buildMegaMenuSections()

● Bash(sed -i "25a import { useGitHubAppConnected } from
      '../../hooks/useGitHubAppConnected';" ui/src/components/Navbar/Navbar.tsx
      && cd ui && timeout 600 npx tsc --no…)
  ⎿                                                                        
              
                     This is not the tsc command you are looking for       
     … +11 lines (ctrl+o to expand)
  ⎿  Updated ui/src/components/Navbar/Navbar.tsx (+1 -0)
      23  import { add as addNotification, dismiss as dismissNotification
           } from '../../store/Notifications/slice';
      24  import { prefetchRoute } from '../../utils/routePrefetch';
      25  import { triggerNavigationProgress } from '../NavigationProgres
          sBar';
      26 +import { useGitHubAppConnected } from '../../hooks/useGitHubApp
         +Connected';                                                    
      27  
      28  type NavbarBillingStatusResponse = {
      29      billing: {

  Listed 2 directories, ran 2 shell commands (ctrl+o to expand)

● The LiveReview frontend isn't actually serving on 8081: ui/node_modules is
  missing, so make run died. I'll install the UI dependencies, which I need for
  type-checking anyway.

  Ran 2 shell commands (ctrl+o to expand)

● Type-check is clean. I'm starting the UI dev server so port 8081 is up, which
  also unblocks the niceurl3 tunnel waiting on it.

  Searched for 2 patterns, listed 1 directory, ran 3 shell commands (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='src/pages/GitProviders/GitProviders.tsx'…)
  ⎿                                                  <p className="text-xs
     text-slate-500">(Coming Soon)</p>
                                                 </div>
     … +14 lines (ctrl+o to expand)
  ⎿  Updated ui/src/pages/GitProviders/GitProviders.tsx (+1 -1)
      336                                                                
             size="sm" 
      337                                                                
             className="ml-2"
      338                                                              >
      339 -                                                              
          -  {formatConnectorType(connector.type)}                       
      339 +                                                              
          +  {connector.metadata?.github_app ? 'GitHub App' : formatConne
          +ctorType(connector.type)}                                     
      340                                                              </
           Badge>
      341                                                          </div>
      342                                                          <p cla
           ssName="text-sm text-slate-300">
  ⎿  Updated ui/src/pages/GitProviders/ConnectorDetails.tsx (+33 -25)
      1120                                                  <p className=
            "text-xs text-slate-500">(Coming Soon)</p>
      1121                                              </div>
      1122                                          </div>
      1123 -                                        <div className="flex 
           -items-center space-x-2">                                     
      1124 -                                            <Button          
      1125 -                                                variant={conn
           -ectorStatus === 'unconnected' ? 'primary' : 'outline'}       
      1126 -                                                size="sm"    
      1127 -                                                onClick={hand
           -leEnableManualTrigger}                                       
      1128 -                                                disabled={isE
           -nablingManualTrigger || connectorStatus === 'manual'}        
      1129 -                                                icon={isEnabl
           -ingManualTrigger ? <Spinner size="sm" /> : <Icons.Settings />
           -}                                                            
      1130 -                                                iconPosition=
           -"left"                                                       
      1131 -                                                className={co
           -nnectorStatus === 'unconnected' ? '' : 'text-xs'}            
      1132 -                                            >                
      1133 -                                                {isEnablingMa
           -nualTrigger ? 'Enabling...' :                                
      1134 -                                                 connectorSta
           -tus === 'unconnected' ? 'Enable Manual Trigger for All Projec
           -ts' : 'Re-enable Manual Trigger'}                            
      1135 -                                            </Button>        
      1136 -                                                             
      1137 -                                            <Button          
      1138 -                                                variant="outl
           -ine"                                                         
      1139 -                                                size="sm"    
      1140 -                                                onClick={hand
           -leDisableManualTrigger}                                      
      1141 -                                                disabled={isD
           -isablingManualTrigger || (statusCounts.manual + statusCounts.
           -automatic) === 0}                                            
      1142 -                                                icon={isDisab
           -lingManualTrigger ? <Spinner size="sm" /> : <Icons.Warning />
           -}                                                            
      1143 -                                                iconPosition=
           -"left"                                                       
      1144 -                                                className="te
           -xt-xs !text-red-400 hover:!text-red-300 hover:!border-red-400
           -"                                                            
      1145 -                                            >                
      1146 -                                                {isDisablingM
           -anualTrigger ? 'Disabling...' : 'Disable Manual Trigger for A
           -ll Projects'}                                                
      1147 -                                            </Button>        
      1123 +                                        {connector?.metadata?
           +.github_app ? (                                              
      1124 +                                            <p className="tex
           +t-sm text-slate-300">                                        
      1125 +                                                This connecti
           +on is managed by the LiveReview GitHub App, so reviews are tr
           +iggered                                                      
      1126 +                                                automatically
           + and no webhooks need to be set up. To add or remove reposito
           +ries,                                                        
      1127 +                                                manage the in
           +stallation on GitHub.                                        
      1128 +                                            </p>             
      1129 +                                        ) : (                
      1130 +                                            <div className="f
           +lex items-center space-x-2">                                 
      1131 +                                                <Button      
      1132 +                                                    variant={
           +connectorStatus === 'unconnected' ? 'primary' : 'outline'}   
      1133 +                                                    size="sm"
      1134 +                                                    onClick={
           +handleEnableManualTrigger}                                   
     … 25 more lines (ctrl+o to expand)
  ⎿  (timeout 5m)

● Bash(python3 - <<'EOF'
      p='src/pages/GitProviders/ConnectorDetails.tsx'…)
  ⎿                  {/* Floating Action Button for mobile */}
                     <Button
                         variant="primary"
     … +42 lines (ctrl+o to expand)
  ⎿  Updated ui/src/pages/GitProviders/ConnectorDetails.tsx (+2 -3)
      1122                                          </div>
      1123                                          {connector?.metadata?
            .github_app ? (
      1124                                              <p className="tex
            t-sm text-slate-300">
      1125 -                                                This connecti
           -on is managed by the LiveReview GitHub App, so reviews are tr
           -iggered                                                      
      1126 -                                                automatically
           - and no webhooks need to be set up. To add or remove reposito
           -ries,                                                        
      1127 -                                                manage the in
           -stallation on GitHub.                                        
      1125 +                                                This connecti
           +on is managed by the LiveReview GitHub App, so no webhooks ne
           +ed to be set up                                              
      1126 +                                                here. To add 
           +or remove repositories, manage the installation on GitHub.   
      1127                                              </p>
      1128                                          ) : (
      1129                                              <div className="f
            lex items-center space-x-2">

  Ran 1 shell command (ctrl+o to expand)

● Bash(python3 - <<'EOF'
      p='src/components/Dashboard/Dashboard.tsx'…)
  ⎿  212:    const hasRunReview = codeReviews > 0;
     213:    const hasGitHubApp = useGitHubAppConnected();
  ⎿  Updated ui/src/components/Dashboard/Dashboard.tsx (+14 -0)
      20  import { useAppDispatch, useAppSelector } from '../../store/con
          figureStore';
      21  import { isCloudMode } from '../../utils/deploymentMode';
      22  import { useOrgContext } from '../../hooks/useOrgContext';
      23 +import { useGitHubAppConnected } from '../../hooks/useGitHubApp
         +Connected';                                                    
      24  import LicenseUpgradeDialog from '../License/LicenseUpgradeDial
          og';
      25  import {
      26      add as addNotification,
     ...
      210      const hasCLI = dashboardData?.cli_installed || false;
      211      const hasAIProvider = aiConnectors > 0;
      212      const hasRunReview = codeReviews > 0;
      213 +    const hasGitHubApp = useGitHubAppConnected();             
      214  
      215      // Build install commands with API key
      216      const apiKey = dashboardData?.onboarding_api_key || '';
     ...
      386                      />
      387                  )}
      388  
      389 +                {/* Orgs connected through the GitHub App keep
          + a nudge to the guided first review until one has run */}     
      390 +                {hasGitHubApp && !hasRunReview && (           
      391 +                    <Alert variant="info" title="Finish setup"
          + className="mb-6">                                            
      392 +                        <div className="flex flex-col gap-3 sm
          +:flex-row sm:items-center sm:justify-between">                
      393 +                            <span>Your GitHub repositories are
          + connected. Review your first pull request to see LiveReview i
          +n action.</span>                                              
      394 +                            <Button size="sm" onClick={() => n
          +avigate('/get-started/github')} className="flex-shrink-0">    
      395 +                                Review your first PR          
      396 +                            </Button>                         
      397 +                        </div>                                
      398 +                    </Alert>                                  
      399 +                )}                                            
      400 +                                                              
      401                  {/* Header with aligned content and prominent 
           CTA */}
      402                  <div className="flex flex-col gap-3 sm:flex-ro
           w sm:justify-between sm:items-center mb-6">
      403                      <p className="text-sm text-slate-400">
  ⎿  (timeout 5m)

  Searched for 1 pattern (ctrl+o to expand)

● Now the route docs, then the homepage CTAs.

  Ran 2 shell commands (ctrl+o to expand)

● Bash(cd /home/lovestaco/hex/lr/LiveReview/internal/docindex/docs/routes_guide
      && mkdir -p auth && cat > auth/gh-login.md <<'EOF'
      # Continue with GitHub (sign-in land…)
  ⎿  ci-cd-gates.md
     create-review-cli.md
     create-review-mcp.md
     ok
  ⎿  Created internal/docindex/docs/routes_guide/get-started-github.md (+36
     -0)
       1 +# Get started with GitHub                                      
       2 +                                                               
       3 +**Route(s):** `/get-started/github`                            
       4 +**Component:** `ui/src/pages/GetStarted/GetStartedGitHub.tsx`  
       5 +                                                               
       6 +## Purpose                                                     
       7 +                                                               
       8 +A guided first run for organizations connected through the Live
         +Review GitHub                                                  
       9 +App: confirm which repositories are connected, make sure an AI 
         +provider is                                                    
      10 +set up, and run the first review on a real pull request.       
      11 +                                                               
      12 +If the organization has no GitHub App connection yet, the page 
         +offers                                                         
      13 +**Continue with GitHub** to install the app (this signs the use
         +r out and back                                                 
      14 +in through the GitHub flow).                                   
      15 +                                                               
      16 +## Who can access it                                           
      17 +                                                               
      18 +Any authenticated org member. It appears in the mega menu (Revi
         +ews) once the                                                  
      19 +organization has a GitHub App connection.                      
      20 +                                                               
      21 +## Key actions                                                 
      22 +                                                               
      23 +- See the connected repositories, and open the installation on 
         +GitHub to add                                                  
      24 +  or remove repositories.                                      
      25 +- **Configure AI** — go to AI Providers when no provider is con
         +figured.                                                       
      26 +- Paste a pull request URL, or pick one of the open pull reques
         +ts listed, and                                                 
      27 +  press **Review this PR** / **Review** to start a review. The 
         +review opens on                                                
      28 +  its detail page.                                             
      29 +- **Next: catch issues before you commit with the CLI** — conti
         +nues to the CLI                                                
      30 +  setup page.                                                  
      31 +                                                               
      32 +## Related pages                                               
      33 +                                                               
      34 +- [Continue with GitHub (sign-in landing page)](auth/gh-login.m
         +d)                                                             
      35 +- [Git Providers](git/git-providers.md)                        
      36 +- [Reviews](reviews/)                                          
  ⎿  Created internal/docindex/docs/routes_guide/auth/gh-login.md (+37 -0)
       1 +# Continue with GitHub (sign-in landing page)                  
       2 +                                                               
       3 +**Route(s):** `/gh-login` (a shareable path that opens `#/gh-lo
         +gin`; LiveReview Cloud only)                                   
       4 +**Component:** `ui/src/pages/Auth/GhLogin.tsx`                 
       5 +                                                               
       6 +## Purpose                                                     
       7 +                                                               
       8 +The quickest way for a new user to try LiveReview. One button, 
         +**Continue                                                     
       9 +with GitHub**, takes the user to a single GitHub screen where t
         +hey install the                                                
      10 +LiveReview GitHub App on the repositories they choose and autho
         +rize sign-in at                                                
      11 +the same time. No personal access token, no webhook setup. Afte
         +rwards they are                                                
      12 +signed in to LiveReview (the account is created if it does not 
         +exist) and                                                     
      13 +land on the [Get started with GitHub](../get-started-github.md)
         + page.                                                         
      14 +                                                               
      15 +The page only appears when the LiveReview server has the GitHub
         + App                                                           
      16 +configured. If it is not configured, or this is not LiveReview 
         +Cloud,                                                         
      17 +`/gh-login` simply shows the normal login screen.              
      18 +                                                               
      19 +## Who can access it                                           
      20 +                                                               
      21 +Anyone who is not signed in. A signed-in user who opens `/gh-lo
         +gin` is sent on                                                
      22 +to [Get started with GitHub](../get-started-github.md).        
      23 +                                                               
      24 +## Key actions                                                 
      25 +                                                               
      26 +- **Continue with GitHub** — install the GitHub App and sign in
         +.                                                              
      27 +- **Already installed the app? Sign in** — sign in with GitHub 
         +when the app is                                                
      28 +  already installed (no install step).                         
      29 +- **Other ways to sign in** — the standard Hexmos sign-in (Goog
         +le, GitHub,                                                    
      30 +  LinkedIn, email), unchanged.                                 
      31 +- An optional `mr` query parameter (a GitHub pull request URL) 
         +is carried                                                     
      32 +  through the sign-in and prefilled on the get-started page.   
      33 +                                                               
      34 +## Related pages                                               
      35 +                                                               
      36 +- [Get started with GitHub](../get-started-github.md)          
      37 +- [Git Providers](../git/git-providers.md)                     
  ⎿  Updated internal/docindex/docs/routes_guide/git/git-providers.md (+8 
     -0)
      30    connector (`enableManualTriggerForAllProjects` /
      31    `disableManualTriggerForAllProjects`).
      32  
      33 +## GitHub App connections                                      
      34 +                                                               
      35 +Organizations that signed in with **Continue with GitHub** have
         + a connector                                                   
      36 +per GitHub App installation. These are labelled "GitHub App", t
         +heir                                                           
      37 +repositories come from the installation, and webhooks are deliv
         +ered by the                                                    
      38 +app, so the manual-trigger buttons are replaced by a note. Repo
         +sitories are                                                   
      39 +added or removed on GitHub, not here.                          
      40 +                                                               
      41  ## Related pages
      42  
      43  - [Explore → Repositories](../explore/repositories.md)

● Now the homepage CTAs (T5).

  Ran 3 shell commands (ctrl+o to expand)

● Bash(ls lib | head; python3 - <<'EOF'
      p='urls.js'…)
  ⎿  historyUtils.ts
     licenceApiClient.ts
     licenceConfig.ts
     … +20 lines (ctrl+o to expand)
  ⎿  Updated ~/hex/hexmoshomepage/urls.js (+2 -0)
       4      "https://karma-src-x02msdf8-23.s3.ap-south-1.amazonaws.com/
          hexmoslogo.svg",
       5    PARSE: "https://parse.apps.hexmos.com/parse",
       6    PARSE_LOCAL: "http://localhost:1337/parse",
       7 +  // LiveReview Cloud app; every landing-page link into it is b
         +uilt from this one value.                                      
       8 +  LIVEREVIEW_APP: "https://livereview.hexmos.com",             
       9  };
      10  export default url;
  ⎿  Created ~/hex/hexmoshomepage/lib/livereviewLinks.ts (+9 -0)
      1 +import url from "../urls";                                      
      2 +                                                                
      3 +/**                                                             
      4 + * "Continue with GitHub" entry for LiveReview Cloud: one GitHub
        + screen installs the LiveReview GitHub App and                  
      5 + * signs the user in. `src` tags which page/button the visit cam
        +e from; `mr` optionally carries a GitHub pull                   
      6 + * request URL to review once they are connected.               
      7 + */                                                             
      8 +export const lrGhLoginUrl = (src: string, mr?: string): string =
        +>                                                               
      9 +  `${url.LIVEREVIEW_APP}/gh-login?src=${encodeURIComponent(src)}
        +${mr ? `&mr=${encodeURIComponent(mr)}` : ""}`;                  
  ⎿  Updated ~/hex/hexmoshomepage/pages/livereview/index.tsx (+2 -1)
      34  import { AnchorHeading } from "../../components/livereview/shar
          ed/AnchorHeading";
      35  import { YouTubeFacade } from "../../components/livereview/shar
          ed/YouTubeFacade";
      36  import { PillarNav } from "../../components/livereview/shared/P
          illarNav";
      37 +import { lrGhLoginUrl } from "../../lib/livereviewLinks";      
      38  
      39  type ExpandedImage = {
      40      src: string;
     ...
      527      };
      528  
      529      const handleBuyNow = () => {
      529 -        window.location.assign("https://livereview.hexmos.com"
          -);                                                            
      530 +        window.location.assign(lrGhLoginUrl("home"));         
      531      };
      532  
      533      const handleSelfHosted = () => {
  ⎿  Updated ~/hex/hexmoshomepage/components/livereview/StartFreeModal.tsx
     (+2 -1)
      2  import ReactDOM from "react-dom";
      3  import { FaChevronDown, FaShieldAlt, FaCode, FaDollarSign, FaLin
         kedin, FaSearch } from "react-icons/fa";
      4  import COUNTRY_OPTIONS from "../../data/countries.json";
      5 +import { lrGhLoginUrl } from "../../lib/livereviewLinks";       
      6  
      7  interface IconProps {
      8    className?: string;
     ...
      144    }
      145  
      146    const handleStartFreeClick = () => {
      146 -    window.open("https://livereview.hexmos.com", "_blank");   
      147 +    window.open(lrGhLoginUrl("start-free-modal"), "_blank");  
      148      onClose();
      149    };
      150  
  ⎿  Updated ~/hex/hexmoshomepage/components/livereview/PricingSection.tsx
     (+2 -1)
      3  import StartFreeModal from "./StartFreeModal";
      4  import FreePlanModal from "./FreePlanModal";
      5  import LOCInfoPopover from "./LOCInfoPopover";
      6 +import { lrGhLoginUrl } from "../../lib/livereviewLinks";       
      7  
      8  const FeatureRow: React.FC<{ title: React.ReactNode; desc: React
         .ReactNode; free?: boolean; premium?: boolean; enterprise?: bool
         ean; lightTheme?: boolean }> = ({ title, desc, free, premium, en
         terprise, lightTheme = false }) => (
      9    <tr className={`border-b transition-colors ${lightTheme ? "bor
         der-slate-200 hover:bg-slate-50/80" : "border-slate-800/40 hover
         :bg-slate-800/20"}`}>
     ...
      179  
      180      if (plan.id === "free") {
      181        // setFreeModalOpen(true);
      181 -      window.open("https://livereview.hexmos.com", "_blank", "
          -noopener,noreferrer");                                        
      182 +      window.open(lrGhLoginUrl("pricing-free"), "_blank", "noo
          +pener,noreferrer");                                           
      183        return;
      184      }
      185  
  ⎿  … 1 more file changed
  ⎿  (timeout 5m)

● Bash(cat >> lib/livereviewLinks.ts <<'EOF'…)
  ⎿  Updated ~/hex/hexmoshomepage/lib/livereviewLinks.ts (+3 -0)
       7   */
       8  export const lrGhLoginUrl = (src: string, mr?: string): string 
          =>
       9    `${url.LIVEREVIEW_APP}/gh-login?src=${encodeURIComponent(src)
          }${mr ? `&mr=${encodeURIComponent(mr)}` : ""}`;
      10 +                                                               
      11 +/** Any other page of the LiveReview Cloud app, e.g. lrAppUrl("
         +/#/checkout/team?period=monthly"). */                          
      12 +export const lrAppUrl = (path = ""): string => `${url.LIVEREVIE
         +W_APP}${path}`;                                                
  ⎿  Updated ~/hex/hexmoshomepage/components/livereview/PricingSection.tsx
     (+2 -2)
      3  import StartFreeModal from "./StartFreeModal";
      4  import FreePlanModal from "./FreePlanModal";
      5  import LOCInfoPopover from "./LOCInfoPopover";
      6 -import { lrGhLoginUrl } from "../../lib/livereviewLinks";       
      6 +import { lrAppUrl, lrGhLoginUrl } from "../../lib/livereviewLink
        +s";                                                             
      7  
      8  const FeatureRow: React.FC<{ title: React.ReactNode; desc: React
         .ReactNode; free?: boolean; premium?: boolean; enterprise?: bool
         ean; lightTheme?: boolean }> = ({ title, desc, free, premium, en
         terprise, lightTheme = false }) => (
      9    <tr className={`border-b transition-colors ${lightTheme ? "bor
         der-slate-200 hover:bg-slate-50/80" : "border-slate-800/40 hover
         :bg-slate-800/20"}`}>
     ...
      184      }
      185  
      186      if (plan.id === "premium") {
      187 -      window.open("https://livereview.hexmos.com/#/checkout/te
          -am?period=monthly", "_blank");                                
      187 +      window.open(lrAppUrl("/#/checkout/team?period=monthly"),
          + "_blank");                                                   
      188        return;
      189      }
      190  
  ⎿  (timeout 5m)

● I'll check the config-off behavior in a real browser: /gh-login should land on
  the normal login, since no GitHub App is configured here.

  Called claude-in-chrome 4 times (ctrl+o to expand)

● Config-off works: /gh-login falls back to the normal login. To preview the
  real page without a GitHub App, I'll stub the ui-config response in the
  browser only.

  Called claude-in-chrome 3 times, ran 1 shell command (ctrl+o to expand)

● The /gh-login feature is built across all five repos. It is switched off here
  and has not run against GitHub, because the one-time GitHub App setup (T0.1)
  needs a person and hasn't happened. Unit and integration tests, the UI
  type-check and make check-status-doc pass. In the browser, /gh-login falls
  back to the normal login while the feature is off. With a stubbed config the
  page also renders. The real flow itself hasn't run.

  Two tests fail and look unrelated to this work:
  TestConvertAzureDevOpsCommentEvent_RealPayload (a trailing-newline mismatch)
  and TestBitbucketCommentHierarchyFromCapture. I didn't dig into the Bitbucket
  one.

  What's in each repo
  - fw-parse: a github_app sign-in that accepts only a primary, verified email.
    It has unit tests (the repo has no integration test against a running
    Parse). It is not deployed.
  - LiveReview:
    - The App endpoints (start, callback, webhook, demo), the token cache and
      its 10-minute refresher, and bot identity for installations.
    - The cross-tenant fix: webhook handlers now resolve the token from the
      connector that received the event, and GetLatestGitHubToken is gone.
    - EnsureCloudUser is split into one shared core, used by both the existing
      endpoint and the new callback.
    - The migration is applied to your dev DB, and db/schema.sql was
      regenerated.
  - LiveReview UI: GhLogin, a get-started page, a Finish-setup banner on the
    dashboard, a mega menu entry and read-only GitHub App cards on the Git
    Providers page.
  - Docs: route guides for the two new pages and the status docs for the new
    storage and network functions.
  - hexmoshomepage: "Start Free" and the free-plan pricing button now go to
    /gh-login. The blog "Start Free" buttons reach it through the existing
    modal, and the modal's Try Cloud path is updated. Until the GitHub App is
    configured, /gh-login just shows the normal login, so shipping these early
    does no harm.

  Things to know
  1. fw-parse .env points at what looks like production. DB_URL_PROD is a remote
     Mongo at 45.55.127.169 with NODE_ENV=production. I did not start it, since
     a test sign-in would create a real Parse user and JWT there. Your
     LiveReview .env has FW_PARSE_BASE_URL at parse.apps.hexmos.com, which
     doesn't have my change yet. Do you want a separate dev Parse, or is writing
     a test user to that DB fine?
  2. The LiveReview frontend wasn't running. ui/node_modules was missing, so
     make run had died and port 8081 was down. I ran npm ci in ui/ and started
     the dev server on 8081, which also unblocks the niceurl3 tunnel.
  3. The AI-for-demo decision is still open. The get-started page links to AI
     Providers when none is configured, so a "FREE" first review isn't yet
     possible for a brand-new org.

  What you need to do to test it end to end
  - Create the GitHub App per T0.1 in the plan, using the manual-talent3
    callback and webhook URLs.
  - Set the GITHUB_APP_* vars in LiveReview/.env.
  - Point FW_PARSE_BASE_URL at a Parse that has the fw-parse change.
  - Then run the manual E2E in §11 of the plan.

  Nothing is committed.

✻ Brewed for 32m 13s · done 5:35 pm

❯ /export                                                                         ⎿  Export cancelled