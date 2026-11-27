import React from 'react';

// Quick-reference for per-repository static analysis tool rules (tools.toml).
// Analogous to jqQuickHelp.tsx for CI/CD Gates.

const SECTIONS: Array<{ title: string; rows: Array<{ expr: string; note: string }> }> = [
  {
    title: 'Per-tool block',
    rows: [
      { expr: '[tool-name]', note: 'Lowercase tool name from the catalog (e.g. [ruff], [trivy], [gitleaks]).' },
      { expr: 'enabled = true', note: 'Enable this tool for PR reviews on this repository.' },
      { expr: 'enabled = false', note: 'Disable this tool for PR reviews on this repository.' },
    ],
  },
  {
    title: 'Include & Exclude path globs',
    rows: [
      { expr: 'include = ["src/**/*.py"]', note: 'Run only if at least one changed PR file matches these glob patterns.' },
      { expr: 'exclude = ["tests/**", "vendor/**"]', note: 'Ignore changed files matching these glob patterns.' },
      { expr: 'include = ["**/*.ts", "**/*.tsx"]', note: 'Separate multiple patterns with commas inside square brackets.' },
    ],
  },
  {
    title: 'Examples',
    rows: [
      {
        expr: '[ruff]\nenabled = true\ninclude = ["api/**/*.py"]\nexclude = ["api/tests/**"]',
        note: 'Run ruff only when Python files in api/ outside api/tests/ change.',
      },
      {
        expr: '[gitleaks]\nenabled = true\nexclude = ["**/*.md"]',
        note: 'Run secret scanning on code PRs while ignoring Markdown-only changes.',
      },
      {
        expr: '[tools]\neslint = true\ntfsec = false',
        note: 'Boolean shorthand under [tools] when you do not need path filters.',
      },
    ],
  },
  {
    title: 'Zero-credit skipping',
    rows: [
      {
        expr: 'No matching files → skipped (0 credits)',
        note: 'If a PR touches zero files passing your include/exclude rules, the tool does not run and deducts 0 credits.',
      },
    ],
  },
];

export const ToolsTomlQuickHelp: React.FC = () => (
  <div className="p-4 space-y-5">
    <div>
      <h3 className="text-sm font-semibold text-white">tools.toml quick help</h3>
      <p className="text-xs text-slate-400 mt-1">
        Configure which static tools run on pull request reviews and restrict them with{' '}
        <span className="text-slate-200 font-medium">include</span> /{' '}
        <span className="text-slate-200 font-medium">exclude</span> path globs.
      </p>
    </div>
    {SECTIONS.map((section) => (
      <div key={section.title}>
        <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">{section.title}</h4>
        <div className="space-y-2">
          {section.rows.map((row) => (
            <div key={row.expr} className="text-xs">
              <code className="block bg-slate-900/70 border border-slate-700 rounded px-2 py-1 text-blue-200 font-mono whitespace-pre-wrap break-words">
                {row.expr}
              </code>
              <p className="text-slate-400 mt-1">{row.note}</p>
            </div>
          ))}
        </div>
      </div>
    ))}
  </div>
);
