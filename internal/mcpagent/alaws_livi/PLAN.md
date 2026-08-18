# Implementation Plan: Migrate Livi Prompts to AgentLaws

## Current State

**Old system** (`internal/mcpagent/prompts/`): 10 flat `.md` files, embedded via `go:embed` in `prompts.go`, assembled by string concatenation in `agent.go` (`buildCountQueryPromptHalves`, `buildFinalizePromptHalves`, `buildClassifyPrompt`, `buildSystemPrompt`).

**New system** (`internal/mcpagent/alaws_livi/`): Already has a well-structured lawbook — 66 `.md` files across 7 chapters (introduction, general, classification, planning, chart-selection, finalizing, degraded), each with proper alaws frontmatter, `<!-- alaws:commentary -->`, and numbered `<!-- alaws:laws -->`. The chapter structure mirrors the pipeline stages.

### Old prompt files and what replaces them

| Old file | Used in | Replaced by (alaws section) |
|---|---|---|
| `analytics_classify.md` | call #0 classify | `classification/` chapter |
| `analytics_plan.md` | call #2 count-query | `planning/` chapter |
| `analytics_finalize.md` | call #3 finalize | `finalizing/` + `chart-selection/` chapters |
| `analytics_repair.md` | repair retry | `degraded/repair.md` |
| `analytics_nodata.md` | no-data path | `degraded/no-data.md` |
| `analytics_schema_intro.md` | head of call #2/#3 | `general/data.md` (needs update) |
| `analytics_schema_examples.md` | tail of call #2/#3 | `general/data.md` (needs update) |
| `agent_instructions.md` | action branch (call #1) | Keep separate or new chapter |
| `chat_only.md` | chat branch | Keep separate or new chapter |
| `org_prompt.md` | all branches | Fold into `general/principles.md` |

---

## Steps

### Step 1: Add AgentLaws dependency

```bash
cd /home/lovestaco/hex/lr/LiveReview
go get github.com/shrsv/AgentLaws/pkg/alaws@latest
```

AgentLaws requires Go 1.25+, LiveReview uses Go 1.26 — compatible.

### Step 2: Create lawbook loader — `internal/mcpagent/laws.go`

Replace `prompts.go`'s `go:embed` approach with runtime lawbook loading via `alaws.Load()`.

The loader:
- Loads the lawbook from `internal/mcpagent/alaws_livi/` at agent initialization time (in `WithAnalytics`)
- Compiles it (validates structure, assigns canonical numbers)
- Pre-renders the four prompt branches as strings using `alaws.Selector` with the appropriate `SectionIDs` per branch:

| Branch | Section IDs to select | Replaces |
|---|---|---|
| **Classify** (call #0) | `livi.general`, `livi.classify` | `classifyInstructions` |
| **Plan/CountQuery** (call #2) | `livi.general`, `livi.planning`, `livi.charts` | `analyticsPlanInstructions` + chart-shape table |
| **Finalize** (call #3) | `livi.general`, `livi.charts`, `livi.finalizing` | `analyticsFinalizeInstructions` + chart-shape table |
| **Degraded** (repair/nodata) | `livi.general`, `livi.degraded` | `analyticsRepairInstructions` + `analyticsNoDataInstructions` |

Each branch's rendered laws replace the corresponding old `.md` file content.

### Step 3: Handle schema injection (the splice point)

The old system splices live dbctx schema text between `analyticsSchemaIntro` and `analyticsSchemaExamples`. The new system needs the same splice — the lawbook's `general/data.md` and `general/variables.md` cover the static rules, but the live table listing is still dynamic.

**Solution**: The `countQueryPrompt` and `finalizePrompt` methods keep their current splice pattern — `head + tableText + tail` — but `head` and `tail` are now assembled from rendered laws instead of raw `.md` embeds. The schema intro/examples content gets folded into the `general` chapter's laws (or kept as a separate "data" section selected into both plan and finalize branches).

Specifically:
- `head` = rendered laws from `livi.general` (includes data rules, org_id filter, SQL dialect) + org/user header + orgID filter instruction
- `tableText` = live dbctx schema (unchanged, still dynamic per turn)
- `tail` = rendered laws from `livi.planning` (for call #2) or `livi.finalizing` + `livi.charts` (for call #3)

### Step 4: Update `agent.go` and `analytics.go`

- `WithAnalytics`: instead of calling `buildCountQueryPromptHalves`/`buildFinalizePromptHalves` (which concatenate embedded strings), call the new lawbook loader to pre-render the four branch prompts
- `buildClassifyPrompt`: replaced by lawbook-rendered classify branch
- `buildSystemPrompt` (the action/chat branches): the action branch uses the old `agent_instructions.md` content (not analytics-specific); the chat branch uses `chat_only.md`. These can either stay as-is (they're not analytics laws) or be folded into the lawbook as a separate "non-analytics" chapter
- Remove `prompts.go` and the `prompts/` directory entirely once migration is complete

### Step 5: Complete the lawbook content

The existing `alaws_livi/` lawbook covers classification, planning, chart-selection, finalizing, and degraded paths. What's missing or needs review:

- **`general/data.md`**: needs to absorb `analytics_schema_intro.md`'s SQL dialect rules and `analytics_schema_examples.md`'s worked examples/enum facts. The worked examples and enum values (reviews.status values, trigger_type values, etc.) should become numbered laws since they're rules the model must follow, not just commentary.
- **`agent_instructions.md`** content (tool-calling rules, domain context, Vega-Lite format rules): either add as a new chapter or keep separate since it governs the non-analytics action branch
- **`org_prompt.md`** content (org-name mention rules): fold into `general/principles.md` or keep separate
- **`chat_only.md`** content: fold into a new `general/conversation.md` section or keep separate

### Step 6: Verify law completeness against old prompts

Compare every rule in the old prompt files against the lawbook:

1. **`analytics_plan.md` vs `planning/` chapter**: ensure every rule from the old prompt has a corresponding numbered law. Key rules to verify:
   - "One entry per distinct thing" fan-out rule
   - "count_sql must count the rows the answer will have"
   - "Default to a grouped answer" rule
   - Rhythm/habit exception (group by day, not author)

2. **`analytics_finalize.md` vs `chart-selection/` + `finalizing/`**: ensure every row in the old chart-shape table has a law. The old table has ~18 rows (trend line, horizon graph, percentile band, sorted bar, histogram, strip plot, pie/donut, stacked area/bar, scatter/bubble, Pareto, heatmap, calendar heatmap, slope graph, connected scatterplot, waterfall, diverging bar, small multiples, change matrix).

3. **`analytics_finalize.md` rules vs `finalizing/chart-construction.md`**: verify rules about axis labels, timeUnit matching, layer vs facet, field existence, rolling averages in SQL, etc.

4. **`analytics_finalize.md` description rules vs `finalizing/describing.md`**: verify short lines, active voice, named org, quoted numbers, frame of reference.

5. **Chapters 4, 5, 6 from cto_chart_ideas.html** (per lovestaco's Slack instruction):
   - Ch 4 (ranked leaderboard): verify `chart-selection/ranking/` covers the "most and least" pattern
   - Ch 5 (concentration/Pareto): verify `chart-selection/concentration/` covers cumulative % line
   - Ch 6 (period-over-period comparison): verify `chart-selection/comparison/` covers two-period slope graph

### Step 7: Build-time compilation check

Add a `TestLawbookCompiles` test in `internal/mcpagent/laws_test.go` that runs `alaws.Compile()` on the lawbook and asserts zero error-severity diagnostics. This catches broken lawbook structure at `go test` time.

### Step 8: Remove old prompts

Once the new system passes tests:
- Delete `internal/mcpagent/prompts/` directory (10 files)
- Delete `prompts.go` (the `go:embed` declarations)
- Update any remaining references in `agent.go`, `analytics.go`, `classify.go`

---

## File Change Summary

| File | Action |
|---|---|
| `go.mod` / `go.sum` | Add `github.com/shrsv/AgentLaws/pkg/alaws` dependency |
| `internal/mcpagent/laws.go` | **New** — lawbook loader, branch prompt renderer |
| `internal/mcpagent/laws_test.go` | **New** — compilation test |
| `internal/mcpagent/prompts.go` | **Delete** |
| `internal/mcpagent/prompts/` (10 files) | **Delete** |
| `internal/mcpagent/agent.go` | Update `WithAnalytics`, remove old `build*Halves` functions |
| `internal/mcpagent/analytics.go` | Update prompt assembly to use lawbook-rendered strings |
| `internal/mcpagent/classify.go` | Update `buildClassifyPrompt` to use lawbook |
| `internal/mcpagent/alaws_livi/general/data.md` | **Update** — absorb schema intro/examples content |
| `internal/mcpagent/alaws_livi/` (other files) | **Review** — ensure completeness vs old prompts |

---

## Risks & Considerations

1. **Rendered law text vs raw markdown**: `alaws.Laws.Render()` outputs numbered law text (e.g. `"3.1 Livi must..."`). The old prompts were raw prose without numbering. The model will see numbered laws — this is intentional (enables citation) but changes what the model receives. Test that the model still follows the instructions correctly with the new numbered format.

2. **Commentary is NOT sent to the model**: Only `<!-- alaws:laws -->` content is rendered for agent prompts. The old `analytics_finalize.md` had its chart-shape table in the main body (not in a laws section). That table content needs to either become numbered laws or be placed in the laws section of the appropriate chart-selection files. The chart-shape table is the most critical content to verify — it's the decision matrix for which mark/encoding to use.

3. **Schema text remains dynamic**: The dbctx-generated table text is still spliced at runtime. The lawbook provides the static framing; the dynamic schema block sits between head/tail as before.

4. **Non-analytics branches**: `agent_instructions.md` (172 lines of tool-calling/domain rules) and `chat_only.md` are not analytics-specific. They can stay as standalone files or be absorbed into the lawbook — decision needed from lovestaco.

5. **Embedding the lawbook**: Two options for bundling `alaws_livi/` into the binary:
   - **Option A**: `go:embed alaws_livi/**` the entire directory tree, then load from an `fs.FS` at runtime. AgentLaws may support `alaws.Load` from an `fs.FS` — check the API.
   - **Option B**: Compile the lawbook at build time (`alaws compile`) and embed the compiled JSON artifact. Simpler runtime but requires a build step.
   - **Option C**: Load from the filesystem at runtime (not embedded). Works for dev but breaks production deployment unless the directory is included in the deploy artifact.
   - Option A is preferred — keeps the source `.md` files as the authoritative artifact and embeds them naturally with `go:embed`.

6. **The `{{variable}}` placeholder system**: AgentLaws supports `{{org_id}}`, `{{org_name}}`, etc. as placeholders in laws. The old system uses Go's `fmt.Sprintf` for org_id injection. The new system should use AgentLaws' native `{{variable}}` substitution via `RenderOptions.Vars` — cleaner and keeps the variable injection point visible in the law text itself.
