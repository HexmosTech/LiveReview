import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Link } from 'react-router-dom';

// ---- types mirroring internal/api/ci_rulesets_handler.go ------------------

export interface CIRuleset {
  id: number;
  org_id: number;
  name: string;
  description: string;
  jq_expr: string;
  created_by?: number;
  created_at: string;
  updated_at: string;
}

export interface CanonicalDoc {
  review_id: number;
  org_id: number;
  repository: string;
  provider: string;
  status: string;
  findings: Array<{
    severity: string;
    confidence: string;
    category: string;
    subcategory: string;
    type: string;
    file_path?: string;
    line_number?: number;
  }>;
  counts: {
    by_severity: Record<string, number>;
    by_category: Record<string, number>;
    total: number;
  };
}

export interface PreviewOutcome {
  loading: boolean;
  error?: string;
  block?: boolean;
  result?: unknown;
  document?: CanonicalDoc;
}

export interface CategoryNode {
  category: string;
  count: number;
  subcategories: Array<{ subcategory: string; count: number }>;
}

export interface Taxonomy {
  severities: string[];
  confidences: string[];
  categories: string[];
  subcategories: string[];
  types: string[];
  category_tree: CategoryNode[];
}

export const EMPTY_TAXONOMY: Taxonomy = { severities: [], confidences: [], categories: [], subcategories: [], types: [], category_tree: [] };

export const EXAMPLE_EXPRESSIONS: Array<{ label: string; expr: string }> = [
  { label: 'Any Critical', expr: '.counts.by_severity.critical > 0' },
  { label: 'Any Security', expr: '.counts.by_category.security > 0' },
  { label: 'Critical OR Security', expr: '(.counts.by_severity.critical > 0) or (.counts.by_category.security > 0)' },
  { label: 'Critical+Warning > 2', expr: '(.counts.by_severity.critical + .counts.by_severity.warning) > 2' },
  { label: 'High-confidence critical', expr: '[.findings[] | select(.severity == "critical" and .confidence == "high")] | length > 0' },
  { label: 'Never block', expr: 'false' },
];

export const JQ_MANUAL_URL = 'https://jqlang.github.io/jq/manual/';
export const GOJQ_REPO_URL = 'https://github.com/itchyny/gojq';

export function titleCase(v: string): string {
  return v
    .replace(/[-_]+/g, ' ')
    .split(' ')
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** Same "what should the primary title read as" heuristic used by the main
 * Reviews list (ui/src/utils/reviewDisplay.tsx getPrimaryTitle), trimmed
 * to what we have in ReviewSummary. */
export function reviewPrimaryTitle(r: { mrTitle?: string; friendlyName?: string; repository: string }): string {
  return r.mrTitle?.trim() || r.friendlyName?.trim() || r.repository;
}

function taxonomyLine(label: string, values: string[]): string {
  return `- ${label}: ${values.length ? values.join(', ') : '(none seen yet in this org)'}`;
}

export function buildLLMPrompt(sampleDocJson: string, taxonomy: Taxonomy): string {
  const catLines = taxonomy.category_tree.map((c) => {
    const subs = c.subcategories.map((s) => s.subcategory).join(', ');
    return `  - ${c.category}${subs ? ` (subcategories: ${subs})` : ''}`;
  });
  return [
    "I'm writing a jq expression for LiveReview's CI/CD gate feature.",
    "LiveReview evaluates my jq expression against a JSON document that looks like this (one real review's findings):",
    '',
    sampleDocJson,
    '',
    'The full vocabulary of values actually used in this project (not just what appears in the sample above):',
    taxonomyLine('severity', taxonomy.severities),
    taxonomyLine('confidence', taxonomy.confidences),
    '- category -> subcategory hierarchy:',
    ...(catLines.length ? catLines : ['  (none seen yet in this org)']),
    taxonomyLine('type', taxonomy.types),
    '',
    'Rules:',
    '- counts.by_severity and counts.by_category are pre-aggregated counts, already zero-filled for the 3 known severities (critical/warning/info).',
    '- findings is the full list if I need row-level filtering (e.g. combining severity + confidence + category on the same finding).',
    '- A truthy result (anything except `false` or `null`) means "block the build". A falsy result means "allow".',
    '',
    'What I want to block on: <describe your condition here, e.g. "any critical finding, or 3+ security findings">',
    '',
    'Please return ONLY the jq expression (no explanation, no code fences).',
  ].join('\n');
}

export function jqQuote(v: string): string {
  return '"' + v.replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"';
}

// ---- caret-aware, viewport-clamped popover ----------------------------------
// Positions a floating panel at a fixed screen point (either the textarea
// caret, or a trigger button's rect) and clamps it so it always stays fully
// on-screen, flipping above/left when there isn't room below/right.

export interface ScreenPoint {
  x: number;
  y: number;
}

export function getCaretScreenPoint(el: HTMLTextAreaElement): ScreenPoint {
  const mirror = document.createElement('div');
  const style = window.getComputedStyle(el);
  const props: (keyof CSSStyleDeclaration)[] = [
    'boxSizing', 'width', 'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing',
    'lineHeight', 'textTransform', 'wordSpacing', 'textIndent', 'paddingTop', 'paddingRight',
    'paddingBottom', 'paddingLeft', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
  ];
  mirror.style.position = 'absolute';
  mirror.style.visibility = 'hidden';
  mirror.style.whiteSpace = 'pre-wrap';
  mirror.style.wordWrap = 'break-word';
  mirror.style.top = '0';
  mirror.style.left = '-9999px';
  props.forEach((p) => {
    (mirror.style as any)[p] = (style as any)[p];
  });
  document.body.appendChild(mirror);

  const pos = el.selectionStart ?? el.value.length;
  mirror.textContent = el.value.substring(0, pos);
  const span = document.createElement('span');
  span.textContent = el.value.substring(pos) || '.';
  mirror.appendChild(span);

  const rect = el.getBoundingClientRect();
  const x = rect.left + span.offsetLeft - el.scrollLeft;
  const y = rect.top + span.offsetTop - el.scrollTop + span.offsetHeight;

  document.body.removeChild(mirror);
  return { x: Math.min(x, rect.right), y };
}

export const FloatingPanel: React.FC<{
  open: boolean;
  point: ScreenPoint | null;
  onClose: () => void;
  widthPx?: number;
  children: React.ReactNode;
}> = ({ open, point, onClose, widthPx = 480, children }) => {
  const panelRef = useRef<HTMLDivElement>(null);
  const [style, setStyle] = useState<{ top: number; left: number }>({ top: -9999, left: -9999 });

  useLayoutEffect(() => {
    if (!open || !point || !panelRef.current) return;
    const rect = panelRef.current.getBoundingClientRect();
    const margin = 12;
    let left = point.x;
    let top = point.y;
    if (left + rect.width > window.innerWidth - margin) left = window.innerWidth - rect.width - margin;
    if (left < margin) left = margin;
    if (top + rect.height > window.innerHeight - margin) {
      const above = point.y - rect.height - 28;
      top = above > margin ? above : Math.max(margin, window.innerHeight - rect.height - margin);
    }
    setStyle({ top, left });
  }, [open, point]);

  useEffect(() => {
    if (!open) return;
    const onMouseDown = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', onMouseDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onMouseDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);

  if (!open) return null;
  return createPortal(
    <div
      ref={panelRef}
      style={{ position: 'fixed', top: style.top, left: style.left, width: widthPx }}
      className="z-[9999] max-w-[92vw] rounded-lg border border-slate-600 bg-slate-800 shadow-2xl max-h-[70vh] overflow-y-auto"
    >
      {children}
    </div>,
    document.body
  );
};

// ---- breadcrumb --------------------------------------------------------------

export interface BreadcrumbItem {
  label: string;
  to?: string;
}

export const Breadcrumb: React.FC<{
  items: BreadcrumbItem[];
  /** Called before following a crumb's link; return false to cancel the
   * navigation (e.g. to show an unsaved-changes prompt instead) -- the
   * caller is then responsible for navigating there itself once resolved. */
  onBeforeNavigate?: (to: string) => boolean;
}> = ({ items, onBeforeNavigate }) => (
  <nav className="flex items-center gap-1.5 text-sm text-slate-400 mb-1" aria-label="Breadcrumb">
    {items.map((item, i) => (
      <React.Fragment key={i}>
        {i > 0 && <span className="text-slate-600">/</span>}
        {item.to ? (
          <Link
            to={item.to}
            onClick={(e) => {
              if (onBeforeNavigate && !onBeforeNavigate(item.to!)) {
                e.preventDefault();
              }
            }}
            className="hover:text-blue-400 hover:underline truncate max-w-[16rem]"
          >
            {item.label}
          </Link>
        ) : (
          <span className="text-slate-200 truncate max-w-[16rem]">{item.label}</span>
        )}
      </React.Fragment>
    ))}
  </nav>
);

// Dark-theme-appropriate transparent-outline + colored-text pill, matching
// the visual language used for review status pills (ui/src/utils/reviewDisplay.tsx
// reviewStatusBadge) -- the shared <Badge> component's pale light-mode
// palette (bg-green-100/text-green-800 etc.) washes out against our dark
// slate backgrounds, so gate results use this instead.
export const GateResultPill: React.FC<{ block: boolean }> = ({ block }) => (
  <span
    className="text-sm font-bold tracking-wide px-3 py-1 rounded-full border"
    style={
      block
        ? { color: '#f87171', borderColor: 'rgba(239,68,68,0.5)', backgroundColor: 'rgba(239,68,68,0.12)' }
        : { color: '#4ade80', borderColor: 'rgba(34,197,94,0.5)', backgroundColor: 'rgba(34,197,94,0.12)' }
    }
  >
    {block ? 'BLOCK' : 'ALLOW'}
  </span>
);

export const ReferenceChip: React.FC<{ label: string; onClick: () => void; title?: string; active?: boolean }> = ({ label, onClick, title, active }) => (
  <button
    type="button"
    onClick={onClick}
    title={title}
    className={`text-sm px-2.5 py-1 rounded-full border transition-colors ${
      active
        ? 'bg-blue-600 border-blue-500 text-white'
        : 'bg-slate-700/60 border-slate-600 text-slate-200 hover:bg-blue-600/80 hover:border-blue-500 hover:text-white'
    }`}
  >
    {label}
  </button>
);

// ---- integration snippet ----------------------------------------------------

export type ProviderKey = 'curl' | 'github' | 'gitlab' | 'bitbucket' | 'azure' | 'generic';

export const PROVIDER_SHA_VARS: Record<ProviderKey, { label: string; shaVar: string }> = {
  curl: { label: 'cURL / Bash', shaVar: '$COMMIT_SHA' },
  github: { label: 'GitHub Actions', shaVar: '$GITHUB_SHA' },
  gitlab: { label: 'GitLab CI', shaVar: '$CI_COMMIT_SHA' },
  bitbucket: { label: 'Bitbucket Pipelines', shaVar: '$BITBUCKET_COMMIT' },
  azure: { label: 'Azure Pipelines', shaVar: '$(Build.SourceVersion)' },
  generic: { label: 'Generic / Jenkins', shaVar: '$COMMIT_SHA' },
};

// Where each CI platform's own docs put secret/variable management -- used
// to render "how do I add these secrets" instructions per provider. Steps
// are each platform's actual UI path, not a guess.
export const PROVIDER_SECRET_INSTRUCTIONS: Record<ProviderKey, string[]> = {
  // Not shown in the UI (the curl/bash tab has no "Environment variables"
  // card -- it hardcodes org/ruleset ids inline), kept only for type coverage.
  curl: [],
  github: [
    'Go to your repository on GitHub.',
    'Click Settings.',
    'In the left sidebar, click Secrets and variables -> Actions.',
    'Select the Secrets tab, under Repository secrets.',
    'Click New repository secret for each key below, paste the value, and click Add secret.',
  ],
  gitlab: [
    'Go to your project on GitLab.',
    'Click Settings -> CI/CD.',
    'Expand the Variables section.',
    'Click Add variable for each key below.',
    'Paste the value, and check Mask variable (and Protect variable if this pipeline only runs on protected branches), then click Add variable.',
  ],
  bitbucket: [
    'Go to your repository on Bitbucket.',
    'Click Repository settings (bottom of the left sidebar).',
    'Under Pipelines, click Repository variables.',
    'Click Add variable for each key below.',
    'Paste the value, check Secured for the API key, and click Add.',
  ],
  azure: [
    'Go to your pipeline in Azure DevOps (Pipelines -> select your pipeline).',
    'Click Edit, then Variables.',
    'Click New variable for each key below.',
    'Paste the value, check Keep this value secret for the API key, and click OK, then Save.',
    'Alternatively, add them once as a Variable group under Pipelines -> Library and link the group to this pipeline.',
  ],
  generic: [
    'Add each key below as a secret/credential in whatever your CI system uses '
      + '(e.g. Jenkins: Manage Jenkins -> Credentials -> add a Secret text credential; '
      + 'CircleCI: Project Settings -> Environment Variables; TeamCity: Project -> Parameters).',
    'Expose each as an environment variable of the same name to the job that runs the gate script.',
  ],
};

export function buildCurlSnippet(baseUrl: string, rulesetId: number, orgId: number, provider: ProviderKey): string {
  const sha = PROVIDER_SHA_VARS[provider].shaVar;
  return [
    '#!/usr/bin/env bash',
    'set -euo pipefail',
    '',
    '# Every call needs both the API key AND the org context header --',
    '# LiveReview requires X-Org-Context on every authenticated request,',
    '# even for a single-org API key.',
    `ORG_ID=${orgId}`,
    '',
    '# 1. Resolve the review that covers this commit.',
    'REVIEW_ID=$(curl -sf -H "X-API-Key: $LIVEREVIEW_API_KEY" \\',
    '  -H "X-Org-Context: $ORG_ID" \\',
    '  -H "Content-Type: application/json" \\',
    `  -d "{\\"commits\\":[\\"${sha}\\"]}" \\`,
    `  "${baseUrl}/api/v1/review-coverage" | jq -r '.reports[0].review_id // empty')`,
    '',
    'if [ -z "$REVIEW_ID" ]; then',
    '  echo "No LiveReview review found for this commit yet; skipping gate."',
    '  exit 0',
    'fi',
    '',
    '# 2. Evaluate the ruleset. Exit code alone is the whole gate:',
    '#    200 = allow, 422 = block, 202 = review still running (retry).',
    `curl -sf -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $ORG_ID" \\`,
    `  "${baseUrl}/api/v1/ci-rulesets/${rulesetId}/evaluate?review_id=$REVIEW_ID" \\`,
    '  || { echo "LiveReview gate blocked this build"; exit 1; }',
  ].join('\n');
}

// GATE_SECRETS is the source of truth for the "Environment variables"
// instructions table in CiRulesetIntegration.tsx (same 3 keys regardless of
// CI platform -- only the steps for *adding* them, in PROVIDER_SECRET_INSTRUCTIONS
// above, differ per provider).
export interface GateSecretSpec {
  key: string;
  describe: (baseUrl: string, rulesetId: number, orgId: number) => string;
  /** If set, the value in the instructions table is a link instead of plain text. */
  linkHref?: (baseUrl: string) => string;
}

export const GATE_SECRETS: GateSecretSpec[] = [
  {
    key: 'LIVEREVIEW_API_KEY',
    describe: () => 'Create one under Settings -> API Keys',
    linkHref: (baseUrl) => `${baseUrl}/#/settings#api-keys`,
  },
  {
    key: 'LIVEREVIEW_ORG_ID',
    describe: (_baseUrl, _rulesetId, orgId) => String(orgId),
  },
  {
    key: 'LIVEREVIEW_RULESET_ID',
    describe: (_baseUrl, rulesetId) => String(rulesetId),
  },
];

export function buildGithubActionsWorkflow(baseUrl: string, rulesetId: number, orgId: number): string {
  return [
    'name: LiveReview Gate',
    '',
    'on:',
    '  pull_request:',
    '    types: [opened, synchronize, reopened]',
    '',
    'jobs:',
    '  livereview-gate:',
    '    runs-on: ubuntu-latest',
    '    steps:',
    '      - name: Evaluate LiveReview CI/CD ruleset',
    '        env:',
    '          LIVEREVIEW_API_KEY: ${{ secrets.LIVEREVIEW_API_KEY }}',
    '          ORG_ID: ${{ secrets.LIVEREVIEW_ORG_ID }}',
    '          RULESET_ID: ${{ secrets.LIVEREVIEW_RULESET_ID }}',
    `          BASE_URL: ${baseUrl}`,
    '          PR_URL: ${{ github.event.pull_request.html_url }}',
    '          HEAD_SHA: ${{ github.event.pull_request.head.sha }}',
    '        run: |',
    '          set -euo pipefail',
    '',
    '          echo "Triggering LiveReview review for $PR_URL (commit $HEAD_SHA)"',
    '          curl -s -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $ORG_ID" \\',
    '            -H "Content-Type: application/json" \\',
    '            -d "{\\"url\\":\\"$PR_URL\\"}" \\',
    '            "$BASE_URL/api/v1/connectors/trigger-review" || true',
    '',
    '          echo "Waiting for the review to complete and evaluating the gate..."',
    '          for attempt in $(seq 1 30); do',
    '            COVERAGE_RESP=$(curl -s -w \'\\n%{http_code}\' -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $ORG_ID" \\',
    '              -H "Content-Type: application/json" \\',
    '              -d "{\\"commits\\":[\\"$HEAD_SHA\\"]}" \\',
    '              "$BASE_URL/api/v1/review-coverage")',
    '            COVERAGE_CODE=$(echo "$COVERAGE_RESP" | tail -1)',
    '            COVERAGE_BODY=$(echo "$COVERAGE_RESP" | sed \'$d\')',
    '',
    '            if [ "$COVERAGE_CODE" = "401" ] || [ "$COVERAGE_CODE" = "403" ]; then',
    '              echo "LiveReview: authentication failed (HTTP $COVERAGE_CODE) -- check the LIVEREVIEW_API_KEY/LIVEREVIEW_ORG_ID secrets"',
    '              echo "$COVERAGE_BODY"',
    '              exit 1',
    '            fi',
    '',
    '            REVIEW_ID=""',
    '            if [ "$COVERAGE_CODE" = "200" ]; then',
    '              REVIEW_ID=$(echo "$COVERAGE_BODY" | jq -r \'.reports[0].review_id // empty\')',
    '            fi',
    '',
    '            if [ -n "$REVIEW_ID" ]; then',
    '              RESP=$(curl -s -w \'\\n%{http_code}\' -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $ORG_ID" \\',
    '                "$BASE_URL/api/v1/ci-rulesets/$RULESET_ID/evaluate?review_id=$REVIEW_ID")',
    '              CODE=$(echo "$RESP" | tail -1)',
    '              BODY=$(echo "$RESP" | sed \'$d\')',
    '',
    '              case "$CODE" in',
    '                200)',
    '                  echo "LiveReview: ALLOW"',
    '                  echo "$BODY"',
    '                  exit 0',
    '                  ;;',
    '                422)',
    '                  echo "LiveReview: BLOCK -- a critical or security finding was found"',
    '                  echo "$BODY"',
    '                  exit 1',
    '                  ;;',
    '                202)',
    '                  echo "  [$attempt/30] review $REVIEW_ID still running, retrying in 10s..."',
    '                  ;;',
    '                401|403)',
    '                  echo "LiveReview: authentication failed (HTTP $CODE) -- check the LIVEREVIEW_API_KEY/LIVEREVIEW_ORG_ID secrets"',
    '                  echo "$BODY"',
    '                  exit 1',
    '                  ;;',
    '                404)',
    '                  echo "LiveReview: ruleset or review not found (HTTP 404) -- check the LIVEREVIEW_RULESET_ID secret"',
    '                  echo "$BODY"',
    '                  exit 1',
    '                  ;;',
    '                *)',
    '                  echo "  [$attempt/30] unexpected evaluate response ($CODE): $BODY"',
    '                  ;;',
    '              esac',
    '            else',
    '              echo "  [$attempt/30] no LiveReview review found yet for $HEAD_SHA, retrying in 10s..."',
    '            fi',
    '            sleep 10',
    '          done',
    '',
    '          echo "Timed out waiting for the LiveReview review to complete."',
    '          exit 1',
  ].join('\n');
}
