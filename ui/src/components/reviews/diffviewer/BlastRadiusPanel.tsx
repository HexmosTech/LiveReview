// Ported from git-lrc:internal/staticserve/static/components/BlastRadiusPanel.js (as of
// the git-lrc HEAD current when this port was written) — the full, drillable "why this
// score" breakdown for a hunk. Principle carried over unchanged: nothing from the report
// is discarded, it's presented prioritized and collapsed, with every layer expandable.
// Rebuilt as React/Tailwind against LiveReview's UIPrimitives instead of git-lrc's
// hand-rolled CSS + position:fixed popovers (Tooltip here covers that).
import React, { useEffect, useMemo, useState } from 'react';
import classNames from 'classnames';
import { Link } from 'react-router-dom';
import { LuArrowUpRight, LuChevronDown, LuChevronRight, LuFolderGit2 } from 'react-icons/lu';
import { Tooltip } from '../../UIPrimitives';
import {
  BlastRadiusCrossRepoChain,
  BlastRadiusCrossRepoImpact,
  BlastRadiusHunkReport,
  BlastRadiusSignal,
  BlastRadiusSymbolContribution,
} from '../../../types/reviews';
import {
  allSignals,
  blastRadiusTier,
  blendRiskScore,
  callerGroupLabel,
  CallerGroup as CallerGroupType,
  crossRepoRouteLabel,
  groupCallers,
  hasImpactBranches,
  relatedRepoDisplayName,
  relatedRepoNameVariants,
  SEVERITY_WEIGHT,
  shortName,
} from '../../../lib/blastRadius';
import { BLAST_CATEGORIES, PRIORITY_CATEGORIES, buildMathMode, MathDimension } from '../../../lib/blastMath';
import SunburstChart from './SunburstChart';
import FlameGraph from './FlameGraph';

const CALLERS_PREVIEW = 8;
const PKGS_PREVIEW = 10;
const RELATED_REPOS_PREVIEW = 10;

// crossRepoDisplayName prefers LiveReview's repositories row (full_name) and
// falls back to the engine project id for artifacts produced before the
// name/link fields existed.
const crossRepoDisplayName = relatedRepoDisplayName;

function crossRepoKey(im: BlastRadiusCrossRepoImpact): string {
  if (im.RelatedRepoID) return `id:${im.RelatedRepoID}`;
  return `name:${crossRepoDisplayName(im)}`;
}

interface RelatedRepoGroup {
  key: string;
  name: string;
  id?: number;
  webURL?: string;
  callSites: string[];
  matchKinds: string[];
}

function groupRelatedRepos(impacts?: BlastRadiusCrossRepoImpact[]): RelatedRepoGroup[] {
  const groups = new Map<string, RelatedRepoGroup>();
  (impacts || []).forEach((im) => {
    const key = crossRepoKey(im);
    let group = groups.get(key);
    if (!group) {
      group = {
        key,
        name: crossRepoDisplayName(im),
        id: im.RelatedRepoID,
        webURL: im.RelatedRepoWebURL,
        callSites: [],
        matchKinds: [],
      };
      groups.set(key, group);
    }
    if (im.CallSite && !group.callSites.includes(im.CallSite)) group.callSites.push(im.CallSite);
    if (im.MatchKind && !group.matchKinds.includes(im.MatchKind)) group.matchKinds.push(im.MatchKind);
  });
  return Array.from(groups.values());
}

const routeLabel = crossRepoRouteLabel;

const RelatedRepoCard: React.FC<{ group: RelatedRepoGroup }> = ({ group }) => {
  const [showAllRoutes, setShowAllRoutes] = useState(false);
  const routes = useMemo(
    () => Array.from(new Set(group.callSites.map(routeLabel).filter(Boolean))),
    [group.callSites]
  );
  const visibleRoutes = showAllRoutes ? routes : routes.slice(0, 2);
  const hiddenRoutes = routes.length - visibleRoutes.length;
  const versionAlias = group.matchKinds.includes('version-alias');
  return (
    <div className="min-w-[17rem] max-w-full flex-1 rounded-lg border border-slate-700 bg-slate-900/70 px-3 py-2">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <LuFolderGit2 className="h-3.5 w-3.5 shrink-0 text-blue-400" />
        {group.id ? (
          <Link
            to={`/explore/repositories/${group.id}/settings`}
            className="inline-flex min-w-0 items-center gap-1 text-xs font-medium text-blue-300 underline decoration-blue-400/70 decoration-1 underline-offset-2 hover:text-blue-200 focus:outline-none focus-visible:ring-1 focus-visible:ring-blue-400"
            aria-label={`Open ${group.name} repository settings`}
          >
            <span className="break-all font-mono">{group.name}</span>
            <LuArrowUpRight className="h-3 w-3 shrink-0" />
          </Link>
        ) : (
          <span className="break-all font-mono text-xs font-medium text-slate-200">{group.name}</span>
        )}
        <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-slate-400">
          related repo
        </span>
        {versionAlias && (
          <span className="rounded bg-amber-900/40 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-amber-300" title="Matched across a URI version segment such as /v1">
            version match
          </span>
        )}
      </div>
      {routes.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span className="text-[10px] font-semibold uppercase tracking-wide text-slate-500">
            {routes.length === 1 ? 'Route' : `Routes (${routes.length})`}
          </span>
          {visibleRoutes.map((route) => (
            <code key={route} className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[11px] text-slate-200">
              {route}
            </code>
          ))}
          {hiddenRoutes > 0 && (
            <button
              type="button"
              className="text-[11px] font-medium text-blue-300 underline decoration-dotted underline-offset-2 hover:text-blue-200"
              onClick={() => setShowAllRoutes(true)}
            >
              show all {routes.length}
            </button>
          )}
          {showAllRoutes && routes.length > 2 && (
            <button
              type="button"
              className="text-[11px] text-slate-500 underline decoration-dotted underline-offset-2 hover:text-slate-300"
              onClick={() => setShowAllRoutes(false)}
            >
              show less
            </button>
          )}
        </div>
      )}
    </div>
  );
};

const RelatedRepoChips: React.FC<{ impacts?: BlastRadiusCrossRepoImpact[]; preview: number; label: string }> = ({ impacts, preview, label }) => {
  const [showAll, setShowAll] = useState(false);
  const groups = useMemo(() => groupRelatedRepos(impacts), [impacts]);
  if (groups.length === 0) return null;
  const visible = showAll ? groups : groups.slice(0, preview);
  const hidden = groups.length - visible.length;
  return (
    <div>
      <div className="mb-1.5 text-xs text-slate-400">{label}</div>
      <div className="flex flex-wrap gap-2">
        {visible.map((group) => <RelatedRepoCard key={group.key} group={group} />)}
      </div>
      {hidden > 0 && <button type="button" className="mt-1.5 text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(true)}>+{hidden} more</button>}
      {showAll && groups.length > preview && <button type="button" className="mt-1.5 text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(false)}>show less</button>}
    </div>
  );
};

function chainKey(chain: BlastRadiusCrossRepoChain): string {
  return (chain.Hops || []).map((h) => `${h.RelatedRepo}|${h.CallSite}|${h.Handler || ''}`).join(';');
}

function dedupeChains(chains?: BlastRadiusCrossRepoChain[]): BlastRadiusCrossRepoChain[] {
  const seen = new Set<string>();
  const out: BlastRadiusCrossRepoChain[] = [];
  (chains || []).forEach((chain) => {
    if (!chain.Hops || chain.Hops.length < 2) return;
    const key = chainKey(chain);
    if (seen.has(key)) return;
    seen.add(key);
    out.push(chain);
  });
  return out;
}

const ChainRepoLabel: React.FC<{ hop: BlastRadiusCrossRepoChain['Hops'][number] }> = ({ hop }) => {
  const name = relatedRepoDisplayName(hop);
  if (hop.RelatedRepoID) {
    return (
      <Link
        to={`/explore/repositories/${hop.RelatedRepoID}/settings`}
        className="inline-flex items-center gap-0.5 font-mono text-blue-300 underline decoration-blue-400/70 decoration-1 underline-offset-2 hover:text-blue-200"
      >
        {name}
        <LuArrowUpRight className="h-3 w-3" />
      </Link>
    );
  }
  return <span className="font-mono text-slate-300">{name}</span>;
};

const ChainList: React.FC<{ chains?: BlastRadiusCrossRepoChain[]; label: string }> = ({ chains, label }) => {
  const [open, setOpen] = useState(false);
  const items = useMemo(() => dedupeChains(chains), [chains]);
  if (items.length === 0) return null;
  return (
    <div>
      <button
        type="button"
        className="flex items-center gap-1 text-xs text-slate-400 hover:text-slate-200"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        {open ? <LuChevronDown className="h-3.5 w-3.5" /> : <LuChevronRight className="h-3.5 w-3.5" />}
        {label} ({items.length})
      </button>
      {open && (
        <ul className="mt-1.5 space-y-1.5">
          {items.map((chain) => (
            <li key={chainKey(chain)} className="rounded border border-slate-700 bg-slate-900/60 px-2 py-1.5 text-[11px] leading-5 text-slate-300">
              <span className="text-slate-500">This repo</span>
              {chain.Hops.map((hop, idx) => (
                <React.Fragment key={`${chainKey(chain)}:${idx}`}>
                  <span className="mx-1 text-slate-500">→</span>
                  <ChainRepoLabel hop={hop} />
                  {hop.CallSite && <span className="ml-1 font-mono text-slate-500">{routeLabel(hop.CallSite)}</span>}
                </React.Fragment>
              ))}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

const SCORE_HINTS: Record<string, { title: string; body: string }> = {
  combined: {
    title: 'Combined score: 0-100 rank within this diff',
    body: 'Blends Blast Radius and Review Priority (default 60/40). Use this number to sort. Read Blast and Priority to see why.',
  },
  blast: {
    title: 'Blast radius: how far this change can reach',
    body: 'Counts callers up to 3 hops. Adds points for HTTP routes, repository hotspots, architectural layers, widely used interfaces, and calls that cross into linked repositories. Adds points when the file touches auth, persistence, config, build, or schema.',
  },
  priority: {
    title: 'Review priority: how much attention this hunk needs',
    body: 'Goes up when a near-duplicate function exists in another file, or when this symbol has no direct tests. Adds small points for cyclomatic complexity, loop depth, and fan-out.',
  },
  hygiene: {
    title: 'Hygiene dampener: this hunk looks small or low-value',
    body: 'Detected from the diff content and file path. Multiplies the Combined score down. A small change to a key function must still rank low.',
  },
  coupling: {
    title: 'File co-change coupling bonus',
    body: 'Files that changed together in git history get a small bonus. Captures hidden coupling when no code reference connects them.',
  },
  severity: {
    title: 'Finding severity: the most serious issue this hunk introduces',
    body: 'Critical, warning, and info findings add points on top of the structural score (default 10% weight) so a critical security bug outranks a trivial one.',
  },
};

const METHODOLOGY_PARAGRAPHS = [
  'Each hunk gets two scores. Both range from 0 to 100 within this diff. 100 marks the highest-risk hunk in this diff, not a universal scale. Combined blends them at 60 percent Blast Radius and 40 percent Review Priority. Sort by Combined. Read Blast and Priority to see why.',
  'Blast radius measures how far the change can reach. Inputs: callers up to 3 hops, HTTP routes, repository hotspots, architectural layers, interface implementations, cross-package callers, calls into linked repositories (with an extended chain when those repos call further linked repos), and file paths near auth, persistence, config, build, or schema.',
  'Review priority measures how much attention this hunk needs. Main inputs: a near-duplicate function in another file, and missing direct tests. Secondary inputs: cyclomatic complexity, loop depth, and fan-out.',
  'File co-change coupling adds a small bonus to Blast Radius when git history shows files that change together. Hygiene signals (formatting, comments, generated code, logging, test-only files, dead code) multiply the Combined score down.',
  'Finding severity adds a third dimension: the most serious issue the hunk introduces. Critical findings add the most, warnings less, info least — blended in at 10 percent weight so a critical security bug in a small change still outranks a trivial one in a big change.',
];

const SEVERITY_LABELS: Record<string, string> = { critical: 'Critical', warning: 'Warning', info: 'Info' };


function splitSignalsByDimension(signals: BlastRadiusSignal[]) {
  const blast: BlastRadiusSignal[] = [];
  const priority: BlastRadiusSignal[] = [];
  const other: BlastRadiusSignal[] = [];
  signals.forEach((s) => {
    if (BLAST_CATEGORIES.has(s.Category)) blast.push(s);
    else if (PRIORITY_CATEGORIES.has(s.Category)) priority.push(s);
    else other.push(s);
  });
  return { blast, priority, other };
}


function sortedSignals(signals: BlastRadiusSignal[]): BlastRadiusSignal[] {
  return [...(signals || [])].sort((a, b) => Math.abs(b.Points || 0) - Math.abs(a.Points || 0));
}

function distinctSymbolCount(signals: BlastRadiusSignal[]): number {
  return new Set(signals.map((s) => s._symbolName).filter(Boolean)).size;
}

function groupSignalsBySymbol(signals: BlastRadiusSignal[]) {
  const ungrouped: BlastRadiusSignal[] = [];
  const bySymbol = new Map<string, BlastRadiusSignal[]>();
  signals.forEach((s) => {
    if (!s._symbolName) {
      ungrouped.push(s);
      return;
    }
    if (!bySymbol.has(s._symbolName)) bySymbol.set(s._symbolName, []);
    bySymbol.get(s._symbolName)!.push(s);
  });
  const weight = (sigs: BlastRadiusSignal[]) => sigs.reduce((sum, s) => sum + Math.abs(s.Points || 0), 0);
  const groups = [...bySymbol.entries()].sort((a, b) => weight(b[1]) - weight(a[1]));
  return { ungrouped, groups };
}

const DETAIL_LIST_PREVIEW = 6;

// Some signal Detail strings are comma-separated lists (impacted route
// names, package paths, caller names) that can run to dozens of entries
// with no truncation applied server-side — clamp them client-side with a
// "+N more"/"show less" toggle instead of dumping the whole wall of text
// inline, the same preview pattern already used for ChipList/CallerGroupView.
// The cross-repo signal's Detail is generated server-side as plain text
// ("Called from org/service"). This turns every related-repo name inside that
// text into the same in-app repository link the chips already use, so the
// single sentence that names the repo is navigable too.
const renderLinkedRepoText = (text: string, impacts?: BlastRadiusCrossRepoImpact[]): React.ReactNode => {
  const entries = new Map<string, { display: string; id?: number }>();
  (impacts || []).forEach((im) => {
    const display = crossRepoDisplayName(im);
    if (!display) return;
    relatedRepoNameVariants(im).forEach((variant) => {
      if (!entries.has(variant)) entries.set(variant, { display, id: im.RelatedRepoID });
    });
  });
  const names = Array.from(entries.keys()).sort((a, b) => b.length - a.length);
  if (names.length === 0) return text;
  const pattern = new RegExp(`(${names.map((n) => n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'g');
  return text.split(pattern).map((part, idx) => {
    const entry = entries.get(part);
    if (!entry) return <React.Fragment key={idx}>{part}</React.Fragment>;
    if (!entry.id) return <span key={idx} className="break-all font-mono text-slate-300">{entry.display}</span>;
    return (
      <Link
        key={idx}
        to={`/explore/repositories/${entry.id}/settings`}
        className="inline-flex items-center gap-0.5 break-all font-mono text-blue-300 underline decoration-blue-400/70 decoration-1 underline-offset-2 hover:text-blue-200"
      >
        {entry.display}
        <LuArrowUpRight className="h-3 w-3 shrink-0" />
      </Link>
    );
  });
};

const SignalDetailText: React.FC<{ detail: string; impacts?: BlastRadiusCrossRepoImpact[] }> = ({ detail, impacts }) => {
  const [showAll, setShowAll] = useState(false);
  const parts = detail.split(', ');
  if (parts.length <= DETAIL_LIST_PREVIEW) return <>{renderLinkedRepoText(detail, impacts)}</>;
  if (showAll) {
    return (
      <>
        {renderLinkedRepoText(detail, impacts)}{' '}
        <button type="button" className="text-slate-400 underline decoration-dotted hover:text-slate-200" onClick={(e) => { e.stopPropagation(); setShowAll(false); }}>
          show less
        </button>
      </>
    );
  }
  const hidden = parts.length - DETAIL_LIST_PREVIEW;
  return (
    <>
      {renderLinkedRepoText(parts.slice(0, DETAIL_LIST_PREVIEW).join(', '), impacts)}{' '}
      <button type="button" className="text-slate-400 underline decoration-dotted hover:text-slate-200" onClick={(e) => { e.stopPropagation(); setShowAll(true); }}>
        +{hidden} more
      </button>
    </>
  );
};

const SignalRow: React.FC<{ s: BlastRadiusSignal; dormant?: boolean; impacts?: BlastRadiusCrossRepoImpact[] }> = ({ s, dormant, impacts }) => {
  const positive = (s.Points || 0) >= 0;
  return (
    <li className="flex items-start gap-2 py-0.5 text-xs">
      <span className={classNames('w-12 shrink-0 font-mono', dormant ? 'text-slate-600' : positive ? 'text-emerald-400' : 'text-red-400')}>
        {dormant ? '+0.0' : `${positive ? '+' : ''}${(s.Points || 0).toFixed(1)}`}
      </span>
      <span className={classNames('flex-1', dormant ? 'text-slate-600' : 'text-slate-300')}>
        {s.Name}
        {s.Detail && <span className="ml-1 text-slate-500">— <SignalDetailText detail={s.Detail} impacts={impacts} /></span>}
      </span>
    </li>
  );
};

const SignalBlock: React.FC<{ signals: BlastRadiusSignal[]; dormant?: boolean; impacts?: BlastRadiusCrossRepoImpact[] }> = ({ signals, dormant, impacts }) => {
  if (distinctSymbolCount(signals) <= 1) {
    return <ul className="space-y-0.5">{signals.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} impacts={impacts} />)}</ul>;
  }
  const { ungrouped, groups } = groupSignalsBySymbol(signals);
  return (
    <div className="space-y-2">
      {ungrouped.length > 0 && <ul className="space-y-0.5">{ungrouped.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} impacts={impacts} />)}</ul>}
      {groups.map(([name, sigs]) => (
        <div key={name}>
          <code className="text-[11px] text-slate-500">{shortName(name)}</code>
          <ul className="space-y-0.5">{sigs.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} impacts={impacts} />)}</ul>
        </div>
      ))}
    </div>
  );
};

const SignalList: React.FC<{ signals: BlastRadiusSignal[]; impacts?: BlastRadiusCrossRepoImpact[] }> = ({ signals, impacts }) => {
  const [showDormant, setShowDormant] = useState(false);
  const ranked = sortedSignals(signals);
  const active = ranked.filter((s) => (s.Points || 0) !== 0);
  const dormant = ranked.filter((s) => (s.Points || 0) === 0);
  if (active.length === 0 && dormant.length === 0) {
    return <p className="text-xs text-slate-500">No signals recorded.</p>;
  }
  return (
    <div>
      <SignalBlock signals={active} impacts={impacts} />
      {showDormant && <SignalBlock signals={dormant} dormant impacts={impacts} />}
      {dormant.length > 0 && (
        <button type="button" className="mt-1 text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowDormant((v) => !v)}>
          {showDormant ? 'hide checked-but-inactive signals' : `${dormant.length} signal${dormant.length !== 1 ? 's' : ''} checked, not contributing`}
        </button>
      )}
    </div>
  );
};

const ScoreChip: React.FC<{ hintKey: string; className?: string; children: React.ReactNode }> = ({ hintKey, className, children }) => {
  const hint = SCORE_HINTS[hintKey];
  return (
    <Tooltip content={<div className="max-w-xs"><p className="font-medium text-white">{hint?.title}</p><p className="mt-1 text-slate-300">{hint?.body}</p></div>}>
      <span className={classNames('inline-flex items-center rounded-full border px-2.5 py-1 font-mono text-xs', className)}>{children}</span>
    </Tooltip>
  );
};

function normMathText(raw: number, max: number, norm: number): string {
  const r = (raw || 0).toFixed(1);
  if (!max) return 'No signals fired for this hunk, or any other hunk in this diff — scores 0/100.';
  return `${r} points here. The highest-scoring hunk in this diff scored ${max.toFixed(1)}, so this one ranks ${Math.round(norm || 0)}/100 by comparison.`;
}

const DimensionCard: React.FC<{ title: string; norm: number; raw: number; max: number; signals: BlastRadiusSignal[]; impacts?: BlastRadiusCrossRepoImpact[] }> = ({ title, norm, raw, max, signals, impacts }) => (
  <div className="rounded-lg border border-slate-700 bg-slate-900 p-3">
    <div className="mb-2">
      <div className="text-sm font-semibold text-slate-200">{title}</div>
      <div className="text-xs text-slate-500">{normMathText(raw, max, norm)}</div>
    </div>
    <SignalList signals={signals} impacts={impacts} />
  </div>
);

// SeverityCard is the third "dimension" next to Blast Radius and Review
// Priority, but simpler: severity is already a direct 0-100 value
// (critical=100, warning=25, info=5), so there's no raw -> norm scaling to
// spell out — just the finding counts and the points they add.
const SeverityCard: React.FC<{
  severityScore: number;
  severityLabel: string | null | undefined;
  severityCounts: { critical: number; warning: number; info: number };
}> = ({ severityScore, severityLabel, severityCounts }) => {
  const points = SEVERITY_WEIGHT * severityScore;
  const parts: string[] = [];
  if (severityCounts.critical > 0) parts.push(`${severityCounts.critical} critical`);
  if (severityCounts.warning > 0) parts.push(`${severityCounts.warning} warning`);
  if (severityCounts.info > 0) parts.push(`${severityCounts.info} info`);
  const detailText = severityLabel
    ? `${parts.join(', ')} — severity adds ${Math.round(SEVERITY_WEIGHT * 100)}% of its 0-100 score to the final rank`
    : 'No findings on this hunk — severity adds nothing';
  return (
    <div className="rounded-lg border border-slate-700 bg-slate-900 p-3">
      <div className="mb-2">
        <div className="text-sm font-semibold text-slate-200">Finding Severity</div>
        <div className="text-xs text-slate-500">
          {severityLabel ? `most severe: ${SEVERITY_LABELS[severityLabel]} → ${severityScore}/100` : 'no findings → 0/100'}
        </div>
      </div>
      <ul className="space-y-0.5">
        <li className="flex items-start gap-2 py-0.5 text-xs">
          <span className={classNames('w-12 shrink-0 font-mono', severityLabel ? 'text-emerald-400' : 'text-slate-600')}>
            {severityLabel ? `+${points.toFixed(1)}` : '+0.0'}
          </span>
          <span className={classNames('flex-1', severityLabel ? 'text-slate-300' : 'text-slate-600')}>
            Finding severity
            <span className="ml-1 text-slate-500">— {detailText}</span>
          </span>
        </li>
      </ul>
    </div>
  );
};

function fileBaseName(path?: string): string {
  return (path || '').split('/').pop() || path || '';
}

function renderMathDimension(d: MathDimension) {
  const { perSymbol, hunkExpression, mode } = d;
  return (
    <div className="rounded-md border border-slate-700 bg-slate-900 p-3">
      <div className="mb-2 border-b border-slate-700 pb-1.5 text-xs font-bold text-slate-200">{d.title}</div>
      <div className="mb-2.5">
        <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepAdd} — add up each symbol's own signals</div>
        {perSymbol.length === 0 && !hunkExpression && <div className="pl-1 text-xs text-slate-500">No signals fired for this dimension.</div>}
        {perSymbol.map((l, i) => (
          <div key={i} className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            <span className="font-semibold text-slate-500">{l.name}</span>: {l.text} = <strong className="text-slate-100">{l.total.toFixed(1)}</strong>
          </div>
        ))}
        {mode === 'linear' && hunkExpression && (
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            <span className="font-semibold text-slate-500">This hunk (file-level signals)</span>: {hunkExpression.text} = <strong className="text-slate-100">{hunkExpression.total.toFixed(1)}</strong>
          </div>
        )}
      </div>
      {d.stepSum > 0 && (
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepSum} — add every subtotal together</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">{d.sumExpression.text} = <strong className="text-slate-100">{d.sumExpression.total.toFixed(1)}</strong></div>
        </div>
      )}
      {d.stepCombine > 0 && (
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepCombine} — saturating combine (sqrt of the summed symbols)</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            √({d.symbolSubtotal.toFixed(1)}) = <strong className="text-slate-100">{d.combinedSymbols.toFixed(1)}</strong>
          </div>
          <div className="pl-1 text-xs text-slate-500">Dampens "many low-risk symbols changed" without changing any single symbol's own signals.</div>
        </div>
      )}
      {d.stepHunk > 0 && (
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepHunk} — add this hunk's file-level signals</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            {d.combinedSymbols.toFixed(1)} + {d.hunkSubtotal.toFixed(1)} = <strong className="text-slate-100">{d.total.toFixed(1)}</strong>
          </div>
        </div>
      )}
      {d.stepAdjust > 0 && (
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepAdjust} — apply post-scoring adjustments</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            {d.total.toFixed(1)} {d.artifactDelta >= 0 ? '+' : '-'} {Math.abs(d.artifactDelta).toFixed(1)} = <strong className="text-slate-100">{d.raw.toFixed(1)}</strong>
          </div>
          <div className="pl-1 text-xs text-slate-500">Cross-repo bonuses and their roll-up effects are applied after the structural total.</div>
        </div>
      )}
      <div>
        <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {d.stepScale} — scale against the highest-scoring hunk in this diff</div>
        <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
          {d.max ? <>{d.raw.toFixed(1)} ÷ {d.max.toFixed(1)} × 100 = <strong className="text-slate-100">{d.norm.toFixed(1)}</strong></> : <>no hunk in this diff scored above 0 for this dimension, so this is <strong className="text-slate-100">0</strong></>}
        </div>
        {d.max > 0 && (
          <div className="pl-1 text-xs text-slate-500">
            {d.maxInferred
              ? `${d.max.toFixed(1)} is the highest score for this dimension in this diff, recovered from the recorded 0-100 rank.`
              : d.isSelf
                ? `${d.max.toFixed(1)} is this very hunk's own score — it's the highest for this dimension anywhere in this diff.`
                : `${d.max.toFixed(1)} is the highest score for this dimension anywhere in this diff, from ${d.maxSource.file ? fileBaseName(d.maxSource.file) : 'another hunk'}${d.maxSource.header ? ` (${d.maxSource.header})` : ''}.`}
          </div>
        )}
      </div>
    </div>
  );
}

const MathModeView: React.FC<{ detail: BlastRadiusHunkReport }> = ({ detail }) => {
  const mm = buildMathMode(detail);
  const {
    blast,
    priority,
    weights,
    blastShare,
    priorityShare,
    blended,
    hygieneMultiplier: hygiene,
    structural,
    severityScore,
    severityLabel,
    severityCounts,
    severityShare,
    final,
    stepBlend,
    stepHygiene,
    stepSeverity,
    stepFinalBlend,
  } = mm;
  const blastNode = renderMathDimension(blast);
  const priorityNode = renderMathDimension(priority);

  const sevParts: string[] = [];
  if (severityCounts.critical > 0) sevParts.push(`${severityCounts.critical} critical`);
  if (severityCounts.warning > 0) sevParts.push(`${severityCounts.warning} warning`);
  if (severityCounts.info > 0) sevParts.push(`${severityCounts.info} info`);
  const sevDetail = severityLabel
    ? `${sevParts.join(', ')} — most severe maps to ${severityScore.toFixed(0)}/100`
    : 'no findings on this hunk';

  return (
    <div className="flex flex-col gap-3.5">
      {blastNode}
      {priorityNode}
      <div className="rounded-md border border-slate-700 bg-slate-900 p-3">
        <div className="mb-2 border-b border-slate-700 pb-1.5 text-xs font-bold text-slate-200">Combine Into Final Score</div>
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {stepBlend} — blend Blast Radius and Review Priority</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            ({weights.BlastRadius.toFixed(2)} × {blast.norm.toFixed(1)}) + ({weights.ReviewPriority.toFixed(2)} × {priority.norm.toFixed(1)}) = {blastShare.toFixed(1)} + {priorityShare.toFixed(1)} = <strong className="text-slate-100">{blended.toFixed(1)}</strong>
          </div>
        </div>
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {stepHygiene} — apply the hygiene multiplier</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">{blended.toFixed(1)} × {hygiene} = <strong className="text-slate-100">{structural.toFixed(1)}</strong></div>
        </div>
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {stepSeverity} — score the finding severity</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">{sevDetail} = <strong className="text-slate-100">{severityScore.toFixed(1)}</strong></div>
        </div>
        <div className="mb-2.5">
          <div className="mb-1 text-[10px] font-bold uppercase tracking-wide text-sky-400">Step {stepFinalBlend} — blend in finding severity ({Math.round((1 - SEVERITY_WEIGHT) * 100)}% structure, {Math.round(SEVERITY_WEIGHT * 100)}% severity)</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            ({(1 - SEVERITY_WEIGHT).toFixed(2)} × {structural.toFixed(1)}) + ({SEVERITY_WEIGHT.toFixed(2)} × {severityScore.toFixed(1)}) = {((1 - SEVERITY_WEIGHT) * structural).toFixed(1)} + {severityShare.toFixed(1)} = <strong className="text-slate-100">{final.toFixed(1)}</strong>
          </div>
        </div>
        {/* .math-step.final — top border + larger blue final number, distinct
            from the other steps above it. */}
        <div className="border-t border-slate-700 pt-2.5">
          <div className="mb-1 text-xs font-bold text-slate-200">Final Score</div>
          <div className="pl-1 font-mono text-xs leading-relaxed text-slate-300">
            Rounded to the nearest whole number: <strong className="text-base text-sky-400">{Math.round(final)}</strong> out of 100
          </div>
        </div>
      </div>
    </div>
  );
};

function metricChips(sym: BlastRadiusSymbolContribution) {
  const chips: { label: string; title: string; warn?: boolean }[] = [];
  if (sym.IsEntryPoint) chips.push({ label: 'entry point', title: 'This symbol is a service entry point' });
  if ((sym.Complexity || 0) > 0) chips.push({ label: `complexity ${sym.Complexity}`, title: 'Cyclomatic complexity' });
  if ((sym.Cognitive || 0) > 0) chips.push({ label: `cognitive ${sym.Cognitive}`, title: 'Cognitive complexity' });
  if ((sym.LoopDepth || 0) > 0) chips.push({ label: `loop depth ${sym.LoopDepth}`, title: 'Deepest loop nesting' });
  if ((sym.OutDegree || 0) > 0) chips.push({ label: `calls out ${sym.OutDegree}`, title: 'Functions this symbol calls (fan-out)' });
  chips.push((sym.TestCount || 0) > 0
    ? { label: `${sym.TestCount} test${sym.TestCount !== 1 ? 's' : ''}`, title: 'Direct test coverage' }
    : { label: 'no tests', title: 'No direct test coverage found', warn: true });
  return chips;
}

const CallerGroupView: React.FC<{
  group: CallerGroupType;
  oldName?: string;
  hoveredCaller?: string | null;
  onHoverCaller?: (name: string | null) => void;
}> = ({ group, oldName, hoveredCaller, onHoverCaller }) => {
  const [showAll, setShowAll] = useState(false);
  const callers = group.callers;
  const visible = showAll ? callers : callers.slice(0, CALLERS_PREVIEW);
  const hidden = callers.length - visible.length;
  return (
    <div className={classNames('mb-2', group.preRename && 'opacity-70')}>
      <div className="mb-1 flex items-center gap-2 text-xs font-medium text-slate-400">
        {callerGroupLabel(group, oldName)}
        <span className="rounded-full bg-slate-700 px-1.5 text-[10px] text-slate-300">{callers.length}</span>
      </div>
      <div className="flex flex-wrap gap-1">
        {visible.map((c) => (
          <span
            key={c.QualifiedName}
            title={c.QualifiedName}
            onMouseEnter={() => onHoverCaller?.(c.QualifiedName)}
            onMouseLeave={() => onHoverCaller?.(null)}
            className={classNames(
              'rounded px-1.5 py-0.5 font-mono text-[11px] text-slate-300',
              hoveredCaller === c.QualifiedName ? 'bg-blue-500/30 ring-1 ring-blue-400' : 'bg-slate-800'
            )}
          >
            {shortName(c.QualifiedName)}
          </span>
        ))}
        {hidden > 0 && (
          <button type="button" className="text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(true)}>+{hidden} more</button>
        )}
        {showAll && callers.length > CALLERS_PREVIEW && (
          <button type="button" className="text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(false)}>show less</button>
        )}
      </div>
    </div>
  );
};

const ChipList: React.FC<{ items?: string[]; preview: number; label: string }> = ({ items, preview, label }) => {
  const [showAll, setShowAll] = useState(false);
  if (!items || items.length === 0) return null;
  const visible = showAll ? items : items.slice(0, preview);
  const hidden = items.length - visible.length;
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-slate-500">{label}</span>
      {visible.map((item) => (
        <span key={item} className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[11px] text-slate-300">{item}</span>
      ))}
      {hidden > 0 && <button type="button" className="text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(true)}>+{hidden} more</button>}
      {showAll && items.length > preview && <button type="button" className="text-[11px] text-slate-500 hover:text-slate-300" onClick={() => setShowAll(false)}>show less</button>}
    </div>
  );
};

const SymbolDetail: React.FC<{
  sym: BlastRadiusSymbolContribution;
  hoveredCaller?: string | null;
  onHoverCaller?: (name: string | null) => void;
}> = ({ sym, hoveredCaller, onHoverCaller }) => {
  const callerGroups = groupCallers(sym.Callers);
  const totalCallers = (sym.Callers || []).length;
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-1.5">
        {metricChips(sym).map((chip) => (
          <span key={chip.label} title={chip.title} className={classNames('rounded-full border px-2 py-0.5 text-[11px]', chip.warn ? 'border-amber-700 text-amber-300' : 'border-slate-600 text-slate-300')}>
            {chip.label}
          </span>
        ))}
      </div>
      <SignalList signals={sym.Signals || []} impacts={sym.ImpactedRelatedRepos} />
      {totalCallers > 0 && (
        <div>
          <div className="mb-1 text-xs font-medium text-slate-400">Reached from {totalCallers} caller{totalCallers !== 1 ? 's' : ''}</div>
          {callerGroups.map((group) => (
            <CallerGroupView key={group.key} group={group} oldName={sym.RenamedFrom} hoveredCaller={hoveredCaller} onHoverCaller={onHoverCaller} />
          ))}
        </div>
      )}
      <ChipList items={sym.ImpactedPackages} preview={PKGS_PREVIEW} label={`Impacted packages (${(sym.ImpactedPackages || []).length})`} />
      <RelatedRepoChips impacts={sym.ImpactedRelatedRepos} preview={RELATED_REPOS_PREVIEW} label={`Impacted related repos (${groupRelatedRepos(sym.ImpactedRelatedRepos).length})`} />
      <ChainList chains={sym.ImpactedRepoChains} label="Extended impact chain" />
    </div>
  );
};

const MethodologyTooltip: React.FC = () => (
  <Tooltip content={<div className="max-w-sm space-y-2">{METHODOLOGY_PARAGRAPHS.map((p, i) => <p key={i}>{p}</p>)}</div>}>
    <button type="button" className="text-xs text-slate-400 underline decoration-dotted hover:text-slate-200">How is this scored?</button>
  </Tooltip>
);

interface BlastRadiusPanelProps {
  detail: BlastRadiusHunkReport;
}

const BlastRadiusPanel: React.FC<BlastRadiusPanelProps> = ({ detail }) => {
  const hygieneMult = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
  const couplingVal = typeof detail.FileCouplingBonus === 'number' ? detail.FileCouplingBonus : 0;
  const severityScore = typeof detail.FindingSeverity === 'number' ? detail.FindingSeverity : 0;
  const severityLabel = detail.FindingSeverityLabel || null;
  const severityCounts = detail.FindingSeverityCounts || { critical: 0, warning: 0, info: 0 };
  const severityPoints = SEVERITY_WEIGHT * severityScore;
  const sortScore = blendRiskScore(detail.Combined || 0, severityScore);
  const symbols = useMemo(
    () => [...(detail.Symbols || [])].sort((a, b) => (b.BlastRadiusRaw || 0) - (a.BlastRadiusRaw || 0)),
    [detail.Symbols]
  );
  const { blast: blastSignals, priority: prioritySignals } = useMemo(
    () => splitSignalsByDimension(allSignals(detail)),
    [detail]
  );
  // Default to a symbol that actually has something to draw: classes often
  // outrank their methods on text references but have no call edges, which
  // made the chart look missing until the user stepped to the method.
  const defaultSymbolIdx = useMemo(() => {
    const i = symbols.findIndex((s) => (s.Callers?.length || 0) > 0 || hasImpactBranches(s));
    return i >= 0 ? i : 0;
  }, [symbols]);
  const [selectedIdx, setSelectedIdx] = useState(defaultSymbolIdx);
  useEffect(() => { setSelectedIdx(defaultSymbolIdx); }, [defaultSymbolIdx]);
  const [vizMode, setVizMode] = useState<'sunburst' | 'flamegraph'>('sunburst');
  const [scoreMode, setScoreMode] = useState<'summary' | 'math'>('summary');
  // Shared between the caller list and the sunburst/flamegraph so hovering a
  // caller chip highlights its chart segment and vice versa (git-lrc's
  // caller-list<->chart cross-highlight — structurally absent before this).
  const [hoveredCaller, setHoveredCaller] = useState<string | null>(null);

  const chartSymbol = useMemo(() => {
    const sym = symbols[selectedIdx];
    if (!sym || !sym.RenamedFrom) return sym;
    return { ...sym, Callers: (sym.Callers || []).filter((c) => !c.PreRename) };
  }, [symbols, selectedIdx]);

  const tier = blastRadiusTier(sortScore);

  return (
    <div className="space-y-4 rounded-lg border border-slate-700 bg-slate-800/60 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <ScoreChip hintKey="combined" className={classNames('font-semibold', tier === 'blast-radius-high' ? 'border-red-700 text-red-300' : tier === 'blast-radius-medium' ? 'border-amber-700 text-amber-300' : 'border-slate-600 text-slate-300')}>
          Score {Math.round(sortScore)}
        </ScoreChip>
        <ScoreChip hintKey="blast" className="border-slate-600 text-slate-300">Blast {Math.round(detail.BlastRadiusNorm || 0)}</ScoreChip>
        <ScoreChip hintKey="priority" className="border-slate-600 text-slate-300">Priority {Math.round(detail.ReviewPriorityNorm || 0)}</ScoreChip>
        <ScoreChip hintKey="hygiene" className={hygieneMult < 1 ? 'border-amber-700 text-amber-300' : 'border-slate-700 text-slate-600'}>
          {hygieneMult < 1 ? `×${hygieneMult}` : 'Hygiene ×1.0'}
        </ScoreChip>
        <ScoreChip hintKey="coupling" className={couplingVal > 0 ? 'border-slate-600 text-slate-300' : 'border-slate-700 text-slate-600'}>
          {couplingVal > 0 ? `Coupling +${couplingVal.toFixed(1)}` : 'Coupling +0'}
        </ScoreChip>
        <ScoreChip
          hintKey="severity"
          className={severityLabel
            ? severityLabel === 'critical' ? 'border-red-700 text-red-300' : severityLabel === 'warning' ? 'border-amber-700 text-amber-300' : 'border-sky-700 text-sky-300'
            : 'border-slate-700 text-slate-600'}
        >
          {severityLabel ? `${SEVERITY_LABELS[severityLabel]} +${severityPoints.toFixed(1)}` : 'Severity +0'}
        </ScoreChip>
        <MethodologyTooltip />
      </div>

      <ChipList items={detail.ImpactedPackages} preview={PKGS_PREVIEW} label={`Impacted packages (${(detail.ImpactedPackages || []).length})`} />
      <RelatedRepoChips impacts={detail.ImpactedRelatedRepos} preview={RELATED_REPOS_PREVIEW} label={`Impacted related repos (${groupRelatedRepos(detail.ImpactedRelatedRepos).length})`} />
      <ChainList chains={detail.ImpactedRepoChains} label="Extended impact chain" />

      <div className="flex gap-1 border-b border-slate-700 text-xs">
        <button type="button" className={classNames('px-3 py-1.5', scoreMode === 'summary' ? 'border-b-2 border-blue-500 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setScoreMode('summary')}>Summary</button>
        <button type="button" className={classNames('px-3 py-1.5', scoreMode === 'math' ? 'border-b-2 border-blue-500 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setScoreMode('math')}>Math Mode</button>
      </div>

      {scoreMode === 'math' ? (
        <MathModeView detail={detail} />
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <DimensionCard title="Blast Radius" norm={detail.BlastRadiusNorm} raw={detail.BlastRadiusRaw} max={detail.MaxBlastRadiusRaw} signals={blastSignals} impacts={detail.ImpactedRelatedRepos} />
          <DimensionCard title="Review Priority" norm={detail.ReviewPriorityNorm} raw={detail.ReviewPriorityRaw} max={detail.MaxReviewPriorityRaw} signals={prioritySignals} impacts={detail.ImpactedRelatedRepos} />
          <SeverityCard severityScore={severityScore} severityLabel={severityLabel} severityCounts={severityCounts} />
        </div>
      )}

      {symbols.length > 0 && (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div>
            {symbols.length > 1 && (
              <div className="mb-2 flex items-center justify-between rounded-md border border-slate-700 bg-slate-900 px-2 py-1 text-xs">
                <button type="button" onClick={() => { setHoveredCaller(null); setSelectedIdx((i) => (i === 0 ? symbols.length - 1 : i - 1)); }} className="text-slate-400 hover:text-slate-200">◂</button>
                <span className="text-slate-300">
                  <span className="font-mono">{symbols[selectedIdx].Name || shortName(symbols[selectedIdx].QualifiedName)}</span>
                  {' · '}
                  {symbols[selectedIdx].Label}
                  {' · '}
                  {selectedIdx + 1} of {symbols.length}
                </span>
                <button type="button" onClick={() => { setHoveredCaller(null); setSelectedIdx((i) => (i === symbols.length - 1 ? 0 : i + 1)); }} className="text-slate-400 hover:text-slate-200">▸</button>
              </div>
            )}
            <SymbolDetail sym={symbols[selectedIdx]} hoveredCaller={hoveredCaller} onHoverCaller={setHoveredCaller} />
          </div>
          <div>
            <div className="mb-2 flex gap-1 text-xs">
              <button type="button" className={classNames('rounded px-2 py-1', vizMode === 'sunburst' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setVizMode('sunburst')}>Sunburst</button>
              <button type="button" className={classNames('rounded px-2 py-1', vizMode === 'flamegraph' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setVizMode('flamegraph')}>Flamegraph</button>
            </div>
            {vizMode === 'sunburst'
              ? <SunburstChart symbol={chartSymbol} width={380} height={380} hovered={hoveredCaller} onHover={setHoveredCaller} />
              : <FlameGraph symbol={chartSymbol} width={380} height={380} hovered={hoveredCaller} onHover={setHoveredCaller} />}
            {hasImpactBranches(symbols[selectedIdx]) && (
              <p className="mt-2 text-[11px] text-slate-500">
                <span className="mr-1 inline-block h-2 w-2 rounded-sm bg-blue-600 align-middle" />
                Blue branches are routes this change reaches in related repositories, not callers.
              </p>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default BlastRadiusPanel;
