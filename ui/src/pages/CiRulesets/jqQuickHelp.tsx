import React from 'react';

// Adapted from https://learnxinyminutes.com/jq/ — trimmed to the subset of
// jq that's actually useful for writing a LiveReview CI/CD gate expression
// (which only ever runs against one canonical review document, so slurping,
// $ENV, @base64, custom `def`s etc. from the original guide don't apply
// here), and re-pointed at the fields our canonical document actually has.
const SECTIONS: Array<{ title: string; rows: Array<{ expr: string; note: string }> }> = [
  {
    title: 'The basics',
    rows: [
      { expr: '.', note: 'Identity: the whole document, unchanged.' },
      { expr: '.status', note: 'Field access — this review\'s status ("completed", "in_progress", ...).' },
      { expr: '.counts.by_severity.critical', note: 'Dotted path into a nested object.' },
      { expr: '.findings[0]', note: 'Index into an array (0-based).' },
      { expr: '.findings[]', note: 'Iterate every element of an array — emits one value per finding.' },
      { expr: 'expr1 | expr2', note: 'Pipe: feed the output of expr1 into expr2, just like a shell pipe.' },
    ],
  },
  {
    title: 'Comparisons & booleans',
    rows: [
      { expr: '.counts.by_severity.critical > 0', note: 'Numeric comparison — this is the shape most gate rules take.' },
      { expr: '.severity == "critical"', note: 'String equality (used inside select(), see below).' },
      { expr: 'A and B', note: 'Boolean AND. Also: `or`, `not`.' },
      { expr: '(.counts.by_severity.critical > 0) or (.counts.by_category.security > 0)', note: 'Combine two conditions — wrap each side in parens to be safe.' },
      { expr: 'A // B', note: 'Alternative operator: use B if A is false/null/errors (handy for a field that might be missing).' },
    ],
  },
  {
    title: 'Working with the findings array',
    rows: [
      { expr: 'select(.severity == "critical")', note: 'Keeps a value only if the condition is true — filters, doesn\'t transform.' },
      { expr: '[.findings[] | select(.severity == "critical")]', note: 'Collect matching findings back into an array (needed before `length`).' },
      { expr: '[.findings[] | select(.severity == "critical")] | length', note: 'Count how many findings matched.' },
      { expr: '[.findings[] | select(.severity == "critical" and .confidence == "high")] | length > 0', note: 'Row-level filter: both conditions must hold on the *same* finding (counts.by_severity can\'t express this — it\'s pre-aggregated).' },
      { expr: 'any(.findings[]; .category == "security")', note: '`any`/`all` — shorthand for "does at least one / do all findings match?"' },
    ],
  },
  {
    title: 'What a truthy result means here',
    rows: [
      { expr: 'true / any non-false, non-null value', note: 'Blocks the build.' },
      { expr: 'false or null', note: 'Allows the build.' },
      { expr: 'false', note: 'The "never block" rule — always allows.' },
    ],
  },
];

export const JqQuickHelp: React.FC = () => (
  <div className="p-4 space-y-5">
    <div>
      <h3 className="text-sm font-semibold text-white">jq quick help</h3>
      <p className="text-xs text-slate-400 mt-1">
        Adapted from{' '}
        <a href="https://learnxinyminutes.com/jq/" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:underline">
          learnxinyminutes.com/jq
        </a>{' '}
        for LiveReview's gate document. See the full{' '}
        <a href="https://jqlang.github.io/jq/manual/" target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:underline">
          jq manual
        </a>{' '}
        for everything else jq can do.
      </p>
    </div>
    {SECTIONS.map((section) => (
      <div key={section.title}>
        <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">{section.title}</h4>
        <div className="space-y-2">
          {section.rows.map((row) => (
            <div key={row.expr} className="text-xs">
              <code className="block bg-slate-900/70 border border-slate-700 rounded px-2 py-1 text-blue-200 font-mono whitespace-pre-wrap break-words">{row.expr}</code>
              <p className="text-slate-400 mt-1">{row.note}</p>
            </div>
          ))}
        </div>
      </div>
    ))}
  </div>
);
