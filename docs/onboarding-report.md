# Onboarding Report

A 1-click, LLM-free report that any LiveReview org can generate to see their
adoption metrics. The report contains 58 charts across 7 sections, rendered
from parameterized SQL templates and Vega-Lite specs.

## Architecture

```
templates.json          ← 58 chart definitions (SQL + Vega-Lite spec)
templates.go            ← Go embed, types, PrepareSQL(), PrepareVegaSpec()
generate_pdf.go         ← Standalone PDF generator (goldmark-pdf pipeline)
onboarding_report_handler.go ← API handler (3 endpoints)
```

### Data flow

```
SQL template + OrgID
    ↓ PrepareSQL(orgID)
PostgreSQL query
    ↓
JSON rows
    ↓ PrepareVegaSpec(dataJSON)  ← injects data + Vega-Lite config theme
Vega-Lite spec
    ↓ renderChartPNG()           ← vl-convert vl2png --scale 2; if too tall,
    │                              shrinks config.view.continuousHeight and
    │                              re-renders (vector → raster again) until
    │                              the aspect ratio fits, instead of
    │                              resampling pixels
PNG (2x resolution, correct aspect ratio)
    ↓ base64 encode
Markdown with data URI images
    ↓ goldmark-pdf
PDF with headers, footers, page breaks
```

## Template catalog

**Source:** `~/Downloads/livereview-debug-export-27.json` (Livi chat export,
64 turns, 32 user questions, 65 interpretations, 63 unique SQL queries).

All SQL queries are parameterized with `{{.OrgID}}` (no hardcoded org IDs).
Vega-Lite specs use `"DATA_PLACEHOLDER"` for data injection.

### Sections

| # | Section | Charts |
|---|---------|--------|
| 1 | Adoption & Growth | 10 |
| 2 | Repository Analysis | 6 |
| 3 | Engineer Analysis | 4 |
| 4 | Review Quality & Findings | 13 |
| 5 | Cost & Efficiency | 9 |
| 6 | Engagement & Trust | 10 |
| 7 | Summary & Comparison | 6 |

## Vega-Lite config theme

Injected into every chart at render time by `PrepareVegaSpec()`. Controls
font sizes, axis lines, legend colors, line widths, and chart dimensions.

Key settings:

- `view.continuousWidth: 540`, `view.continuousHeight: 300` (1.8:1 ratio)
- `axis.domain: true`, `axis.grid: true` (visible axis lines and grid)
- `legend.labelColor: "#1e293b"` (dark text, always visible)
- `line.strokeWidth: 1.8`, `point.size: 25` (readable at 2x scale)

The `powerbi` theme (vl-convert default) is disabled during rendering by
setting `VL_CONVERT_THEME=none` so it doesn't override our config.

## PDF generation

Uses goldmark-pdf (same library as `chatexport/pdf.go`) with a markdown-first
pipeline:

1. Execute all SQL queries against the DB
2. Render each chart to PNG via vl-convert (2x scale); if the result is
   taller than the page allows (aspect ratio > 140mm/150mm = 0.93), shrink
   the spec's view height and re-render via vl-convert again (up to 2 more
   attempts) rather than resampling the raster — see "Chart sizing" below
3. Validate each PNG (dimensions, aspect ratio, file size)
4. Build markdown with base64 data URI images
5. Render markdown → PDF via goldmark-pdf

### Page geometry (A4 portrait)

| Dimension | Value |
|-----------|-------|
| Page | 210 × 297 mm |
| Side margins | 15 mm each |
| Header band | 50 mm (running header + rule) |
| Footer band | 30 mm (page numbers) |
| Usable content | 180 mm wide × 217 mm tall |
| Max image height | 140 mm (reserves 50 mm for title + description) |

All of these are physical mm values, but goldmark-pdf's `NewFpdf` hardcodes
the underlying gofpdf document unit to **points**, not millimeters (see the
vendored `fpdf.go`: `gofpdf.New(orientation, "pt", ...)`). Every
position/dimension constant in `generate_pdf.go` is therefore defined as a
real-world mm value multiplied by a `mmToPt` conversion factor — e.g.
`marginSideMM = 15.0 * mmToPt` — so the numbers actually passed to
`SetMargins`/`Line`/`MultiCell`/etc. are correct in the unit gofpdf uses.
Font sizes and `pdflib.Style.Spacing` are unaffected: those are always in
points regardless of document unit, by both gofpdf and goldmark-pdf
convention. Before this was applied consistently, every margin/spacing
constant was silently ~2.83x smaller than intended (a "15mm" margin was
actually 15pt ≈ 5.3mm), which is part of why the report felt cramped and
inconsistent.

### Chart sizing

goldmark-pdf scales every image to 150mm wide (page width minus double
margins), maintaining aspect ratio, and places it at the current cursor
position with no automatic page-break check for overflow — a PNG with
height/width ratio > 0.93 gets cut off by the page boundary rather than
flowing cleanly to the next page.

`renderChartPNG()` handles this by measuring the actual rendered PNG
dimensions and, if the ratio is too tall, adjusting the spec's
`config.view.continuousHeight` and asking vl-convert to render again — a
fresh vector→raster pass, not a resize of the existing pixels. Up to 3
attempts total, each one correcting against the previous attempt's measured
output.

An earlier version of this did a post-render pixel resize instead (manual
nearest-neighbor row resampling). It had a row-mapping bug that overshot the
source image, and independent of that bug, point-sampling anti-aliased text
during downscaling is inherently lossy — solid-color bars survived because
they have no fine detail to alias, but every axis label, tick, and title
came out as scrambled noise. Re-rendering from the vector spec avoids this
class of bug entirely: whatever size is needed, vl-convert draws it fresh
and correctly anti-aliased.

### Keeping a chart's title, description, and image together

Each chart's title/description and its image are two separate markdown
elements (a paragraph and an image), and goldmark-pdf paginates them
independently — its image renderer has no page-break awareness at all (see
`renderImage` in the vendored `renderer_funcs.go`: it places the image at
whatever `y` the cursor is on with no bounds check). Left alone, this means
a chart's title can land at the bottom of one page while its image starts
on the next.

`buildMarkdown` embeds one `<!--chart:BASE64-->` sentinel per chart, where
the payload is a small JSON struct (`chartBlockMeta`): title, description,
the chart's already-known image height (exact, from the PNG's real pixel
dimensions — see `renderChartPNG` above), and whether this is the first
chart in its section. A custom `ast.HTMLBlock` node renderer decodes it and,
using gofpdf's own text-wrapping (`SplitLines`) at the exact font/width that
will be used, computes precisely how tall the whole title+description+image
block will be. It compares that against the space actually left on the
page (`raw.GetY()` vs the bottom margin) and calls `raw.AddPage()` *before*
drawing the title if the block won't fit — never mid-block. The same
sentinel mechanism draws the title and description directly with gofpdf
(bold/italic `MultiCell` calls) rather than through markdown's bold/italic
paragraphs, both so the fit check can be exact and so every chart's text
lines up with the image's column (see "Design system" below).

This is a strong preference, not a hard guarantee: a chart whose image
alone exceeds a full page's usable height (rare — capped at 140mm) can
still split, and the fit-check's per-attempt height budget for the
description is measured, not estimated, so it should hold for effectively
all real chart descriptions in the catalog.

## Design system

The report went through a pass to make the chrome (cover, running
header/footer, section headings) and the per-chart title/description feel
like one designed piece instead of independently-styled parts:

- **One color palette**, defined once in `generate_pdf.go` (`colBrand`,
  `colInk`, `colMuted`, `colBorder`, plus an amber trio for the "no data"
  callout) and reused everywhere — cover accent rule, section underlines,
  chart titles, running header/footer, warning boxes.
- **Chart title/description/image share one column.** goldmark-pdf insets
  images to `2×marginSideMM` from the left at `pageWidth - 4×marginSideMM`
  wide (see its `renderImage`), narrower than and offset from the normal
  full-margin text column. Before this pass, chart titles/descriptions used
  the wider column, so they didn't line up with the image below them — a
  visible misalignment on every single chart. `chartColX`/`chartColWidth`
  make every custom-drawn element (title, description, hairline separators,
  the "no data" box) match the image's column exactly.
- **A hairline separator** between consecutive charts sharing a page, and a
  full-width brand-colored rule under every section heading and a short one
  under the cover title, so section starts and chart boundaries are always
  visually explicit rather than just whitespace.
- **A friendly "no data" callout** (amber, matching a conventional warning
  tone) replaces a bare `> Warning: query: pq: ...` blockquote — the
  underlying error is still logged to stderr during generation, but a
  customer-facing report shouldn't surface raw SQL/Go errors. See
  `friendlyChartError`.
- **ASCII-safe running text.** gofpdf's built-in Helvetica expects Latin-1;
  a Unicode character (an em dash, a middle dot) written as a Go string
  literal arrives as raw UTF-8 bytes, each of which then renders as its own
  mojibake glyph. Cover/header/footer text uses plain ASCII punctuation
  (`-`) instead.

## Multi-org support

`RunGeneratePDF` accepts an org by name (case-insensitive) via `--org`,
defaulting to `hexmos-internal` to match prior behavior, and derives the
output path from the org name unless `--out` is given explicitly — so
generating a report for a different org never overwrites another org's
existing PDF:

```bash
go run cmd/onboarding-pdf/main.go                          # hexmos-internal -> ~/Downloads/livereview-onboarding-report.pdf
go run cmd/onboarding-pdf/main.go --org "Ostrelle Systems"  # -> ~/Downloads/livereview-onboarding-report-ostrelle-systems.pdf
go run cmd/onboarding-pdf/main.go --org "Acme Inc" --out /tmp/acme.pdf
```

`GenerateOnboardingPDF` and `buildMarkdown` both take the org name (in
addition to org ID) and thread it through the cover page, running
header/footer, and `substituteOrgName`.

`substituteOrgName` works around a template-catalog issue: ~70
description/query_summary fields in `templates.json` have the literal
string `"hexmos-internal"` baked in from the one-time extraction (see
`scripts/extract_onboarding_templates.py`) rather than a real placeholder
token. Left alone, a report generated for another org would render correct
data but describe some charts as being about "the hexmos-internal
organization." `substituteOrgName` does a literal string replace against
the org actually being reported on before the description is rendered — a
targeted runtime fix, not a change to the templates themselves. Fixing this
properly in the templates (a placeholder token expanded per-org) is a
follow-up if it's worth the ~70-site edit to `templates.json`.

## API endpoints

Registered in `server.go` under the standard auth middleware chain
(RequireAuth → BuildOrgContext → ValidateOrgAccess → BuildPermissionContext).

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/v1/reports/onboarding/sections` | List sections with chart counts |
| GET | `/api/v1/reports/onboarding/charts` | All charts for the org |
| GET | `/api/v1/reports/onboarding/charts/:section` | Charts for one section |

Each chart endpoint executes the SQL, renders the Vega-Lite spec, and returns
the chart data + rendered PNG. Errors are isolated per chart (one failed chart
doesn't break the rest).

## Frontend

`ui/src/pages/Reports/OnboardingReport.tsx` — fetches charts section by
section with a progress bar, renders each with `InteractiveChart`, and shows
a completion notification when done.

Route: `/reports/onboarding`
Menu: Mega menu → Reports → Onboarding Report

## Standalone PDF generation

```bash
export $(grep DATABASE_URL .env | xargs)
go run cmd/onboarding-pdf/main.go
# Output: ~/Downloads/livereview-onboarding-report.pdf

go run cmd/onboarding-pdf/main.go --org "Ostrelle Systems"
# Output: ~/Downloads/livereview-onboarding-report-ostrelle-systems.pdf
```

The script:
1. Connects to the DB using `DATABASE_URL`
2. Finds the org_id for `--org` (default `hexmos-internal`), case-insensitive
3. Executes all 58 chart templates
4. Renders and validates PNGs, re-rendering any that don't fit the page (see
   "Chart sizing")
5. Produces a PDF via goldmark-pdf to `--out`, or a path derived from the
   org name under `~/Downloads` if not given

## Files

| File | Purpose |
|------|---------|
| `scripts/extract_onboarding_templates.py` | One-time extraction from Livi chat export |
| `internal/onboardingreport/templates.json` | 58 parameterized chart templates |
| `internal/onboardingreport/templates.go` | Go embed, types, PrepareSQL, PrepareVegaSpec |
| `internal/onboardingreport/generate_pdf.go` | Standalone PDF generator |
| `cmd/onboarding-pdf/main.go` | CLI entry point |
| `internal/api/onboarding_report_handler.go` | API handler |
| `internal/api/server.go` | Route registration (lines ~1509-1521) |
| `ui/src/pages/Reports/OnboardingReport.tsx` | Frontend component |
| `ui/src/App.tsx` | Route (line ~323) |
| `ui/src/components/Navbar/megaMenuData.ts` | Menu entry (line ~208) |

## Adding a new chart

1. Add the SQL + Vega-Lite spec to `templates.json`
2. Use `{{.OrgID}}` in SQL, `"DATA_PLACEHOLDER"` in the Vega-Lite data
3. The config theme is injected automatically by `PrepareVegaSpec()`
4. Run `go run cmd/onboarding-pdf/main.go` to verify rendering
5. Check stderr for validation warnings

## Known limitations

- PDF uses Helvetica (gofpdf built-in) instead of Liberation Sans (embedded
  in chatexport) because the font data is unexported from the chatexport package.
- The `<!--pagebreak-->` sentinel requires a custom `ast.HTMLBlock` renderer
  (same pattern as chatexport/pdf.go).
- Charts with very many categories (e.g., 20+ repositories) may still end up
  with a short view height to fit the page, compressing bar/point spacing —
  but since this comes from re-rendering the vector spec at that height
  (not resampling pixels), labels stay crisp even when tight.
- The height-shrink only applies to specs with a top-level
  `config.view.continuousHeight` (the common case, injected by
  `PrepareVegaSpec()`). A concat/facet spec without one is accepted as
  rendered rather than distorted.
- A chart with a very high category count on a categorical axis (e.g. "Top
  Repositories Reviewed by Each Engineer" faceted per engineer) can still
  end up too dense to read once squeezed into the page's 150mm width — this
  is a data-density/chart-design limitation, not a pagination or rendering
  bug, and isn't addressed by the page-fit logic above.
- ~70 fields in `templates.json` have `"hexmos-internal"` baked in as
  literal text (see "Multi-org support"); `substituteOrgName` patches
  descriptions at render time, but a proper fix would template the
  catalog itself.
