<img src="./assets/gfx/png/logo-with-text.png" height=80 />

<a href="https://github.com/HexmosTech/LiveReview/actions/workflows/gitleaks.yml" target="_blank" rel="noopener noreferrer"><img alt="gitleaks.yml" title="gitleaks.yml: Secret scanning workflow" src="https://github.com/HexmosTech/LiveReview/actions/workflows/gitleaks.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/LiveReview/actions/workflows/osv-scanner.yml" target="_blank" rel="noopener noreferrer"><img alt="osv-scanner.yml" title="osv-scanner.yml: Dependency vulnerability scan" src="https://github.com/HexmosTech/LiveReview/actions/workflows/osv-scanner.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/LiveReview/actions/workflows/govulncheck.yml" target="_blank" rel="noopener noreferrer"><img alt="govulncheck.yml" title="govulncheck.yml: Go vulnerability check" src="https://github.com/HexmosTech/LiveReview/actions/workflows/govulncheck.yml/badge.svg"></a>&nbsp;<a href="https://github.com/HexmosTech/LiveReview/actions/workflows/semgrep.yml" target="_blank" rel="noopener noreferrer"><img alt="semgrep.yml" title="semgrep.yml: Static analysis security scan" src="https://github.com/HexmosTech/LiveReview/actions/workflows/semgrep.yml/badge.svg"></a>&nbsp;<img alt="dependabot-enabled" title="dependabot-enabled: Automated dependency updates are enabled" src="./assets/gfx/dependabot-enabled.svg">&nbsp;<a href="https://github.com/HexmosTech/LiveReview/actions/workflows/mcp-testcases.yml" target="_blank" rel="noopener noreferrer"><img alt="mcp-testcases.yml" title="mcp-testcases.yml: MCP integration test suite" src="https://github.com/HexmosTech/LiveReview/actions/workflows/mcp-testcases.yml/badge.svg"></a>


# Blast-Radius Aware AI Code Review for Business-Critical Systems

Your team's attention is limited. LiveReview spends it where **business risk is highest** — not spread evenly across every diff.

<p align="center">
  <video src="./assets/videos/blast-radius-demo.mp4" poster="./assets/screenshots/blast-radius/poster.webp" controls muted loop playsinline width="90%">
  </video>
</p>

<p align="center"><i>LiveReview's Blast Radius &amp; Review Priority scoring, live in the diff viewer.</i></p>

| The exact math, not a black box | Visualize blast radius at a glance | Every factor that feeds the score |
|:---:|:---:|:---:|
| <img src="./assets/screenshots/blast-radius/new-risk-score-3.webp" width="280"/> | <img src="./assets/screenshots/blast-radius/new-risk-score-4.webp" width="280"/> | <img src="./assets/screenshots/blast-radius/new-risk-score-2.webp" width="280"/> |

<p align="center">
   <b>Self-Host for Free:</b> <a href="#quick-start">Get Started in 5 Minutes</a><br/>
   <i>Want a guided rollout?</i> <a href="#transformation-program">Join the 14-Day Transformation Program</a>
</p>

---

## What Do You Need?

| I want to... | Go to |
|---|---|
| Try LiveReview free in under 5 minutes | [Quick Start](#quick-start) |
| Understand Blast-Radius scoring | [above ↑](#blast-radius-aware-ai-code-review-for-business-critical-systems) |
| Enforce checks at commit / push / PR / CI | [Org-Wide Harness](#org-wide-harness) |
| See what risks LiveReview actually catches | [Prevent Outages, Breaches & Technical Debt](#impact-report) |
| See what LiveReview's analytics can tell my team | [Data-Backed Decisions](#data-backed-decisions) |
| Understand how LiveReview differs from other tools | [Why LiveReview](#why-livereview) |
| See the full feature set | [Features](#features) |
| Wire reviews into every commit | [Git-Native CLI](#cli) |
| Review code without leaving my editor | [IDE Extensions](#ide-extensions) |
| Connect LiveReview to Claude, Cursor, or Windsurf | [MCP Server](#mcp-server) |
| Enforce my team's own coding standards | [Repository Rules](#repository-rules) |
| Cut AI review costs in half | [Adaptive Reviews](#adaptive-reviews) |
| Compare self-hosted pricing tiers | [Self-Hosted Tiers](#self-hosted-tiers) |
| See how LiveReview stacks up vs Copilot / CodeRabbit / Graphite | [Comparisons](#comparisons) |
| Understand LiveReview's security posture | [Security](#security) |
| Get hands-on help rolling this out to my team | [14-Day Transformation Program](#transformation-program) |

---

<a id="quick-start"></a>
## Quick Start — Self-Hosted Community Edition (Free)

Get LiveReview running in under 5 minutes. The **Community Edition is free forever** and includes the full AI review engine, Blast-Radius scoring, all git providers, and unlimited reviews for up to 3 users — see the full breakdown in [Self-Hosted Tiers](#self-hosted-tiers).

### One-Command Install

```bash
# Quick demo setup (localhost only, no webhooks)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | sudo bash -s -- setup-demo
```

**Requirements:**
- `bash` shell (zsh/fish not officially supported)
- Ubuntu/MacOS
- Docker (with `docker-compose` or `docker compose`)
- `jq`

You'll need a **Free Licence** to get started — follow the guide [here](https://github.com/HexmosTech/LiveReview/wiki/Get-a-LiveReview-Licence).

**Access your installation:**
- Web UI: http://localhost:8081/
- API: http://localhost:8888/api

### Production Deployment

For teams needing external access and webhooks, follow the [Productionization Guide](https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview).

| Feature | Demo Mode | Production Mode |
|---------|-----------|-----------------|
| **Access** | localhost only | External via reverse proxy |
| **Webhooks** | Disabled (manual triggers) | Enabled (automatic triggers) |
| **Configuration** | Zero config required | Reverse proxy setup needed |
| **Perfect for** | Development, testing, demos | Teams, production deployments |

---

<a id="org-wide-harness"></a>
## Enforce Your Standards on Every Line of AI-Generated Code

Activate checks at the levels that matter to you — mix and match enforcement per repository.

| Commit | Before Push | MR / PR | CI / CD | Scheduled Checks |
|:---:|:---:|:---:|:---:|:---:|
| `git lrc review` on staged changes | Guardrail before code leaves your machine | Full AI review + Blast-Radius score on the diff | Gate merges on review completion | Periodic sweeps for drift and hotspots |

---

<a id="impact-report"></a>
## Prevent Outages, Breaches, and Technical Debt Before They Happen

Every commit LiveReview reviews gets checked against the same risk categories tracked across production codebases:

| 10 | 100+ | Every Commit |
|:---:|:---:|:---:|
| Risk Categories | Failure Patterns Tracked | Scanned Automatically |

| Pillar | What it costs you | Categories checked |
|---|---|---|
| **Outages** | What takes down production — and impacts your on-call rotation | Reliability, Correctness, Performance, Scalability |
| **Breaches** | What ends up in a disclosure letter — and a board meeting | Security, Compliance & Governance |
| **Technical Debt** | What slows every future release until someone pays it down | Maintainability, Architecture |

---

<a id="data-backed-decisions"></a>
## See It In Action: Data-Backed Decisions

LiveReview ships with **Livi**, a chat-native analytics layer that answers plain-English questions about your review activity with live charts — no dashboard-building required. Below is a sample of the kind of questions different roles ask Livi, rendered from the same chart specs used in the interactive demo on the live site.

### Adoption & Growth

| Persona | Question to Livi | Chart |
|---|---|---|
| Exec | "Is LiveReview adoption increasing across the org?" | <img src="./assets/screenshots/livi-charts/01-adoption-is-livereview-adoption-increasing-across.png" width="320"/> |
| Exec | "Which repositories have adopted LiveReview the most?" | <img src="./assets/screenshots/livi-charts/02-adoption-which-repositories-have-adopted-liverevi.png" width="320"/> |

### Repository Analysis

| Persona | Question to Livi | Chart |
|---|---|---|
| Product | "Which repos are gaining or losing engineering velocity?" | <img src="./assets/screenshots/livi-charts/03-repos-which-repos-are-gaining-or-losing-engine.png" width="320"/> |
| Product | "How much code are we reviewing in each repository?" | <img src="./assets/screenshots/livi-charts/04-repos-how-much-code-are-we-reviewing-in-each-r.png" width="320"/> |

### Engineer Analysis

| Persona | Question to Livi | Chart |
|---|---|---|
| Eng Manager | "Who are our top contributors by review volume?" | <img src="./assets/screenshots/livi-charts/05-engineers-who-are-our-top-contributors-by-review-v.png" width="320"/> |
| Eng Manager | "How does each engineer trigger their reviews?" | <img src="./assets/screenshots/livi-charts/06-engineers-how-does-each-engineer-trigger-their-rev.png" width="320"/> |

### Review Quality

| Persona | Question to Livi | Chart |
|---|---|---|
| Eng Manager | "What are the most concerning issue types this quarter?" | <img src="./assets/screenshots/livi-charts/07-quality-what-are-the-most-concerning-issue-types.png" width="320"/> |
| Eng Manager | "Are engineers actually incorporating reviews into their daily workflow?" | <img src="./assets/screenshots/livi-charts/08-quality-are-engineers-actually-incorporating-rev.png" width="320"/> |

### Cost & Efficiency

| Persona | Question to Livi | Chart |
|---|---|---|
| Exec | "How much does LiveReview cost us per day?" | <img src="./assets/screenshots/livi-charts/09-cost-how-much-does-livereview-cost-us-per-day.png" width="320"/> |
| Product | "Which AI provider gives us the best value?" | <img src="./assets/screenshots/livi-charts/10-cost-which-ai-provider-gives-us-the-best-valu.png" width="320"/> |

### Engagement & Trust

| Persona | Question to Livi | Chart |
|---|---|---|
| Eng Manager | "Are people trusting the reviews LiveReview produces?" | <img src="./assets/screenshots/livi-charts/11-engagement-are-people-trusting-the-reviews-liverevi.png" width="320"/> |
| Product | "Which engineers get the most value from LiveReview?" | <img src="./assets/screenshots/livi-charts/12-engagement-which-engineers-get-the-most-value-from-.png" width="320"/> |

### Summary & Comparison

| Persona | Question to Livi | Chart |
|---|---|---|
| Exec | "How does this week compare to last week?" | <img src="./assets/screenshots/livi-charts/13-summary-how-does-this-week-compare-to-last-week.png" width="320"/> |
| Eng Manager | "What's the overall severity mix across all our findings?" | <img src="./assets/screenshots/livi-charts/14-summary-what-s-the-overall-severity-mix-across-a.png" width="320"/> |

### Trace & Investigate

| Persona | Question to Livi | Chart |
|---|---|---|
| Engineer | "Show me reviews connected to last week's production incident." | <img src="./assets/screenshots/livi-charts/15-trace-show-me-reviews-connected-to-last-week-s.png" width="320"/> |
| Engineer | "Which files keep showing up with issues?" | <img src="./assets/screenshots/livi-charts/16-trace-which-files-keep-showing-up-with-issues.png" width="320"/> |

*Charts above use illustrative sample data. Try the fully interactive version, with live chart drill-down, at [hexmos.com/livereview](https://hexmos.com/livereview/#data-backed-decisions).*

---

<a id="why-livereview"></a>
## Why LiveReview

| | |
|---|---|
| **Blast-Radius Aware** | Reviews are ranked by how much of the system a change can actually break — not by diff size. |
| **Git-Level Guardrails** | Unread code never sneaks in. Reviews track every commit, and deliberate skips stay in the audit trail. |
| **Works With Your Git Setup** | GitHub, GitLab, Bitbucket, Gitea, or something else — LiveReview adapts to your workflow, not the other way around. |
| **Choose Your AI** | Pick the LLMs that match quality, latency, and budget today — and swap them whenever your requirements change. |
| **Works Wherever You Do** | Web, CLI, IDE. Cloud, self-hosted, or air gapped. Same guardrails and visibility everywhere. |
| **Priced to Stay Out of the Way** | The cost of review shouldn't decide whether review happens. LiveReview stays lean so teams can use it daily. |

### Why Engineering Teams Love LiveReview

- **Accelerate Delivery Cycles** — Reduce PR review time from hours to minutes, enabling your team to ship features faster and with greater confidence
- **Save Senior Engineering Time** — Liberate senior developers from routine reviews, allowing them to focus on mentorship and high-impact architectural work
- **Drive Quality Excellence** — Build a culture of quality with metrics that highlight improvements in code standards, reduced defects, and development efficiency
- **Prioritize by Risk, Not Volume** — Blast-Radius scoring means the riskiest hunks surface first, even on a huge PR

---

<a id="features"></a>
## Powerful Features for Modern Engineering Teams

<p align="center">
   <img src="./assets/screenshots/dashboard.png" alt="LiveReview Dashboard" width="80%"/>
</p>

### Track Engineering Excellence
Quantify your team's improvement with comprehensive metrics. Track review times, code quality trends, and team velocity to demonstrate engineering value to stakeholders.

### Fine-Tuned LiveReview AI Model
LiveReview comes with its own fine-tuned AI model ready to use from day one. If you prefer to use your own provider — Gemini, OpenAI, AWS Bedrock, a self-hosted Ollama model, or any other LLM — you can bring your own key (BYOK) and plug it in.

<p align="center">
   <img src="./assets/screenshots/ai_providers.png" alt="AI Provider Configuration" width="80%"/>
</p>

### Use Any Git Provider: GitHub, GitLab, Bitbucket, Gitea, Azure DevOps
Works effortlessly with GitHub, GitLab, Bitbucket, Gitea, Azure DevOps. Connect your repositories in minutes and start receiving AI-powered code reviews across all your projects.

<p align="center">
   <img src="./assets/screenshots/git_providers.png" alt="Git Provider Integration" width="80%"/>
</p>

### View All AI Reviews in One Place
Manage all your code reviews from a single, intuitive interface. Track review status, prioritize PRs, and monitor team activity with real-time updates.

<p align="center">
   <img src="./assets/screenshots/reviewlist.png" alt="LiveReview review list" width="80%"/>
</p>

### Look Under the Hood with Detailed Progress Tracking
Monitor review progress in real-time. Track which files have been reviewed, identify bottlenecks, and ensure nothing falls through the cracks.

<p align="center">
   <img src="./assets/screenshots/progress_tracker.png" alt="LiveReview Progress Tracker" width="80%"/>
</p>

### Customize Review Prompts to Fit Your Team
Tailor AI review behavior to match your team's coding standards and priorities. Create custom prompts that focus on what matters most to your organization. *(Team & Enterprise)*

<p align="center">
   <img src="./assets/screenshots/prompt_customization.png" alt="Customizing LiveReview's review prompts" width="80%"/>
</p>

### Discuss with AI in MR and See it Learn Everyday
Build an institutional knowledge base from code reviews. Capture best practices, common issues, and team learnings to continuously improve code quality. *(Team & Enterprise)*

<p align="center">
   <img src="./assets/screenshots/learnings_management.png" alt="Managing team learnings in LiveReview" width="80%"/>
</p>

### Sharp AI-Generated Pull Request Summaries
Get detailed, actionable summaries of every pull request. Understand changes at a glance with AI-generated insights that highlight key modifications, potential issues, and improvement suggestions.

<p align="center">
   <img src="./assets/screenshots/detailed_mr_summaries.png" alt="Detailed AI-generated MR/PR summaries" width="80%"/>
</p>

### Ask AI for Clarification or Debate Code Changes
Ask questions and get instant clarifications about code changes. The AI reviewer understands context and provides helpful explanations to speed up the review process.

<p align="center">
   <img src="./assets/screenshots/clarification_question.png" alt="Asking LiveReview's AI a clarification question in a merge request" width="80%"/>
</p>

---

<a id="cli"></a>
## Two CLI Tools. One LiveReview Backend.

Install `git-lrc` for commit-time reviews in any terminal. Use `claude-lrc` when you're building inside Claude Code. Both tools share the same AI review engine and monthly LOC quota.

```bash
# Typical Git Guardrails Flow
git add .
git lrc review

# Or skip explicitly (auditable in git log):
git lrc review --skip
git commit -m "message"
```

<p align="center">
   <img src="./assets/screenshots/lr_cli1.png" alt="LiveReview CLI showing inline findings" width="80%"/>
</p>

**`git-lrc` — git commit hook:**
- Git-native — works in any repo without a cloud platform connection
- Skips tracked in git log — auditable, not silent
- One-line install, 30k LOC free every month

**`claude-lrc` — Claude Code integration:**
- Review, vouch, and skip inside Claude Code without leaving the chat surface
- Natural language or slash commands (`/lrc:review`, `/lrc:skip`, `/lrc:vouch`)
- Bundled with `git-lrc` — no separate install needed

### Quick Install

**Linux/macOS:**
```bash
curl -fsSL https://hexmos.com/lrc-install.sh | bash
```

**Windows (PowerShell):**
```powershell
iwr -useb https://hexmos.com/lrc-install.ps1 | iex
```

---

<a id="ide-extensions"></a>
## IDE Extensions

Get instant AI code reviews without leaving your editor. Available for VSCode, Cursor, and Antigravity.

| IDE | Install Link |
|-----|--------------|
| **VSCode** | [Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=Hexmos.livereview) |
| **Cursor** | [Open VSX Registry](https://open-vsx.org/extension/hexmos/livereview) |
| **Antigravity** | [Open VSX Registry](https://open-vsx.org/extension/hexmos/livereview) |

---

<a id="mcp-server"></a>
## Get Actionable Engineering Intelligence with MCP and APIs

Every code review LiveReview performs adds to a growing source of **engineering intelligence**. Instead of manually piecing together pull requests, comments, and reviews, generate custom reports, identify your strongest contributors, uncover quality and security trends, or drill into engineering activity — in minutes instead of hours — via the MCP server or REST API.

### Getting your API Key

1. Go to LiveReview
2. Click on Settings
3. Navigate to API Keys
4. Generate and copy a new API key

### Configuration

Add the following block to your MCP client's configuration file:

- For eg: Claude Desktop: claude_desktop_config.json
- Other clients: Check the client's documentation for the equivalent file.

```json
{
  "mcpServers": {
    "livereview": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "https://livereview.hexmos.com/api/mcp",
        "--header",
        "X-API-KEY: ${LIVEREVIEW_API_KEY}"
      ],
      "env": {  
        "LIVEREVIEW_API_KEY": "<YOUR_LIVEREVIEW_API_KEY>"  
      }                       
    }
  }
}
```

Replace `<YOUR_LIVEREVIEW_API_KEY>` with your actual LiveReview API key.

### What you can do

Once connected to the MCP server, you can ask your assistant to interact with LiveReview.

#### Code Reviews
| Tool | Description | Example Prompt |
|------|-------------|----------------|
| `post_api_v1_connectors_trigger-review` | Trigger a new code review for a repo URL | *"Trigger a review for https://github.com/user/repo/pull/123"* |
| `get_api_v1_reviews` | List recent reviews | *"List our recent completed reviews"* |
| `get_api_v1_reviews_id_summary` | Get the AI summary and insights for a specific review | *"Summarize the review ID xyz"* |
| `get_api_v1_reviews_id_accounting` | Get the token and LOC accounting for a review | *"Show the token usage for review ID xyz"* |

#### Learnings & Prompts
| Tool | Description | Example Prompt |
|------|-------------|----------------|
| `get_api_v1_learnings` | List existing team learnings | *"List our team's active learnings"* |
| `get_api_v1_learnings_id` | Get details of a specific learning | *"Show details for learning ID abc"* |
| `put_api_v1_learnings_id` | Update an existing learning | *"Update learning ID abc to enforce snake_case"* |
| `get_api_v1_prompts_catalog` | List available prompt catalogs | *"Show the catalog of prompt rules"* |
| `get_api_v1_prompts_key_variables` | Get required variables for a prompt template | *"What variables does the base prompt need?"* |
| `get_api_v1_prompts_key_render` | Render a prompt preview with provided variables | *"Render the prompt key 'system' with..."* |

#### Billing & Quotas
| Tool | Description | Example Prompt |
|------|-------------|----------------|
| `get_api_v1_billing_status` | Check current billing status of the organization | *"What is our current billing status?"* |
| `get_api_v1_quota_status` | Check current LOC status and quota | *"How much LOC quota do we have left?"* |
| `get_api_v1_billing_usage_summary` | Get billing usage summary | *"Show a summary of our billing usage"* |
| `get_api_v1_billing_usage_operations` | Get recent billable review operations | *"List the most recent billable operations"* |
| `get_api_v1_billing_usage_members` | Get member-wise LOC usage information | *"Show the usage broken down by team member"* |
| `post_api_v1_billing_upgrade_preview` | Generate an upgrade preview for a target plan | *"Preview the cost of upgrading to team_32usd"* |

#### Integrations
| Tool | Description | Example Prompt |
|------|-------------|----------------|
| `get_api_v1_connectors` | List configured Git connectors | *"List our configured Git connectors"* |
| `get_api_v1_aiconnectors` | List configured AI provider connections | *"Which AI providers are currently active?"* |

---

<a id="repository-rules"></a>
## Enforce Your Team's Engineering Standards with Repository Rules

A good reviewer doesn't just know your language and framework — it knows *your* repository: which patterns your team prefers, which dependencies are off-limits, and which files don't need a second look. LiveReview enforces your team's engineering standards through repository rules — drop a `.lrc/` directory in your repo and LiveReview reads it on every review.

```
.lrc/
├── ignore               # files the reviewer never sees
├── rules/
│   ├── INSTRUCTIONS.md  # read first, every review
│   ├── security.md
│   └── style.md
└── policy/
    └── tools.toml       # which checks are allowed to run
```

| | |
|---|---|
| **Repository Rules** | Write down the handful of decisions that come up in every review — "prefer direct SQL over ORM abstractions", "avoid new infrastructure dependencies". `INSTRUCTIONS.md` is read first, every other `rules/*.md` file follows in order. |
| **Ignore File** | Point the reviewer away from generated code, vendored dependencies, and anything else that doesn't need a second look. Gitignore syntax, matched from your repo root — ignored files don't count toward billable lines. |
| **Policies** *(coming soon)* | Decide which tools and checks are allowed to run on this repo. Machine-readable settings that LiveReview reads directly — never sent to the AI model. |
| **Static Checks** *(coming soon)* | Pair AI review with static analyzers like semgrep and eslint, authorized through policy and run as part of the same commit-time flow. |

---

<a id="adaptive-reviews"></a>
## Cut AI Review Costs by 50% Without Compromising Quality

**Adaptive Reviews** saves 40-50% of AI inference costs using a multi-model technique: a powerful Leader Model detects complex issues, while a cost-efficient Helper Model explains them — delivering the same review quality at half the price.

| | |
|---|---|
| **Reduce Costs by 40-50%** | Use a cost-efficient Helper Model for explanations instead of a single expensive model for everything. |
| **Double Review Volume** | Review up to 2x more code with the same budget — no need to constantly monitor limits. |
| **Leader + Helper Architecture** | The Leader Model is solely dedicated to finding complex issues; the Helper Model expands those findings into detailed explanations. |
| **Maintain Review Quality** | Because issue detection is still driven by the high-end Leader Model, there's no degradation in accuracy. |

---

<a id="self-hosted-tiers"></a>
## Self-Hosted Tiers

LiveReview offers three tiers for self-hosted deployments. **Community Edition is free forever and provides genuine value for small teams and individual developers.**

| Feature | Community (Free) | Team | Enterprise |
|---------|:----------------:|:----:|:----------:|
| AI Code Reviews (with Blast-Radius scoring) | ✅ | ✅ | ✅ |
| Git Provider Integration | ✅ | ✅ | ✅ |
| AI Provider Configuration | ✅ | ✅ | ✅ |
| Dashboard & Analytics | ✅ | ✅ | ✅ |
| **Unlimited Reviews** | ✅ | ✅ | ✅ |
| Users | Up to 3 | Unlimited | Unlimited |
| Prompt Customization | ❌ | ✅ | ✅ |
| Learnings Management | ❌ | ✅ | ✅ |
| Multiple API Keys | ❌ | ✅ | ✅ |
| Priority Support | ❌ | ✅ | ✅ |
| SSO / SAML | ❌ | ❌ | ✅ |
| Audit Logs | ❌ | ❌ | ✅ |
| Compliance Reports | ❌ | ❌ | ✅ |
| Custom Integrations | ❌ | ❌ | ✅ |

**Growing team?** Upgrade to Team or Enterprise for advanced features, priority support, and unlimited users.

[Get a License](https://hexmos.com/livereview/selfhosted-access/)

---

<a id="comparisons"></a>
## How LiveReview Compares

### vs GitHub Copilot Code Review
- **Multi-Platform Support**: Works with GitHub, GitLab, Bitbucket, AND Gitea (not just GitHub)
- **Self-Hosted Option**: Your code stays private vs cloud-only risk
- **AI Choice Freedom**: Pick your AI backend vs locked into one model
- **Cost Control**: You control both costs and quality

### vs CodeRabbit
- **More Affordable**: Significantly lower pricing
- **Zero Cloud Risk**: Self-hosted option vs cloud-only deployment
- **Complete Control**: Your code never leaves your infrastructure (with self-hosted)
- **Attack Prevention**: Eliminates entire class of cloud-based vulnerabilities

### vs Graphite / Qodo Merge / CodeAnt AI
- **Deployment Flexibility**: Cloud or self-hosted — your choice
- **AI Provider Freedom**: Bring your own keys, use any LLM
- **Git-Native Protocol**: `git lrc review` integrates directly into your workflow
- **Works Everywhere**: Web, CLI, IDE — same experience across all

### vs Building Your Own
- **Ready Out-of-Box**: Skip months of development time
- **Complex Integration Covered**: Code host APIs, webhooks, dashboards all handled
- **AI Expertise Included**: Advanced prompt engineering and review logic, including Blast-Radius scoring, included
- **Ongoing Maintenance**: No need for ongoing MR/PR handling, user management
- **Focus on Product**: Your team builds features, not infrastructure

---

## Full Documentation

Visit the [Wiki](https://github.com/HexmosTech/LiveReview/wiki) for complete documentation:

<a href="https://github.com/HexmosTech/LiveReview/wiki"><img src="./assets/screenshots/wikiview.png" width="80%"></a>

---

<a id="security"></a>
## Security You Can Count On

- Security is treated as a first-class concern across LiveReview deployment models.
- We maintain documented reporting channels, response commitments, and verification references.
- Automated security checks and SBOM generation support ongoing transparency.
- For complete details, see [SECURITY.md](SECURITY.md).

### Security Scans

LiveReview includes local security scan targets in the Makefile:

```bash
make security-govulncheck
make security-govulncheck-json
make security-osv
make security-gitleaks
make security-triage
```

Scan artifacts are written under `security_issues/`.

#### Ported Workflows Are Disabled By Default

The following workflows are present but gated off by default:

- `.github/workflows/gitleaks.yml`
- `.github/workflows/osv-scanner.yml`
- `.github/workflows/govulncheck.yml`

They only run when GitHub repository variable `ENABLE_SECURITY_WORKFLOWS` is set to `true`.

Enable later:

1. Open repository settings in GitHub.
2. Go to Secrets and variables > Actions > Variables.
3. Add `ENABLE_SECURITY_WORKFLOWS` with value `true`.

Disable again:

- Set `ENABLE_SECURITY_WORKFLOWS=false` or remove the variable.

---

## FAQ

**How will LiveReview benefit our engineering organization?**
LiveReview provides tangible improvements in code quality and delivery speed while generating meaningful metrics that demonstrate the value of your engineering investments. Teams typically see a 70% reduction in review time and up to 40% increase in velocity.

**What is Blast-Radius scoring, exactly?**
It's a per-hunk score built from call-graph reach, cross-package impact, persistent-state mutation, cyclomatic/cognitive complexity, and test coverage gaps. It tells you which parts of a diff can do the most damage if something's wrong, so reviewers spend their limited attention where it matters most.

**Is my code secure with LiveReview?**
Absolutely. With self-hosted deployment, all code stays within your infrastructure with no data sent to external servers. For teams preferring managed hosting, our [cloud version](https://hexmos.com/livereview/) uses industry-standard security practices and encryption.

**What's the difference between Community and paid tiers?**
Community Edition is fully functional with unlimited reviews, all AI providers, and all git integrations — limited to 3 users. Team and Enterprise tiers add prompt customization, learnings management, priority support, and unlimited users.

---

## License

LiveReview is distributed under a modified variant of **Sustainable Use License (SUL)**.

> [!NOTE]
>
> **What this means:**
> - ✅ **Source Available** — Full source code is available for self-hosting
> - ✅ **Business Use Allowed** — Use LiveReview for your internal business operations
> - ✅ **Modifications Allowed** — Customize for your own use
> - ❌ **No Resale** — Cannot be resold or offered as a competing service
> - ❌ **No Redistribution** — Cannot redistribute modified versions commercially
>
> This license ensures LiveReview remains sustainable while giving you full access to self-host and customize for your needs.

For detailed terms, examples of permitted and prohibited uses, and definitions, see the full
[LICENSE.md](LICENSE.md).

---

<a id="transformation-program"></a>
## Want Hands-On Help Rolling This Out?

Self-hosting gets you the tool. The **14-Day Transformation Program** gets your whole team — from execs to individual contributors — actually using it well: onboarding, workflow integration, and measurable before/after engineering metrics.

<p align="center">
   <a href="https://hexmos.com/livereview/transform/">
      <img src="./assets/screenshots/transform/transformation-program-thumb.jpg" alt="The 14-Day Transformation Program" width="50%"/>
   </a>
</p>

<p align="center">
   <a href="https://hexmos.com/livereview/transform/"><b>Join the 14-Day Transformation Program →</b></a>
</p>

---

<p align="center">
   <b>Self-Host:</b> <a href="#quick-start">Get Started Free</a> | 
   <b>Cloud:</b> <a href="https://hexmos.com/livereview/">Try hexmos.com/livereview</a> | 
   <a href="https://github.com/HexmosTech/LiveReview/wiki">Documentation</a>
</p>
