// Ported from git-lrc:internal/staticserve/static/components/BlastRadiusPanel.js (as of
// the git-lrc HEAD current when this port was written) — the full, drillable "why this
// score" breakdown for a hunk. Principle carried over unchanged: nothing from the report
// is discarded, it's presented prioritized and collapsed, with every layer expandable.
// Rebuilt as React/Tailwind against LiveReview's UIPrimitives instead of git-lrc's
// hand-rolled CSS + position:fixed popovers (Tooltip here covers that).
import React, { useMemo, useState } from 'react';
import classNames from 'classnames';
import { Tooltip } from '../../UIPrimitives';
import {
  BlastRadiusHunkReport,
  BlastRadiusSignal,
  BlastRadiusSymbolContribution,
} from '../../../types/reviews';
import {
  allSignals,
  blastRadiusTier,
  callerGroupLabel,
  CallerGroup as CallerGroupType,
  groupCallers,
  shortName,
} from '../../../lib/blastRadius';
import SunburstChart from './SunburstChart';
import FlameGraph from './FlameGraph';

const CALLERS_PREVIEW = 8;
const PKGS_PREVIEW = 10;

const SCORE_HINTS: Record<string, { title: string; body: string }> = {
  combined: {
    title: 'Combined score: 0-100 rank within this diff',
    body: 'Blends Blast Radius and Review Priority (default 60/40). Use this number to sort. Read Blast and Priority to see why.',
  },
  blast: {
    title: 'Blast radius: how far this change can reach',
    body: 'Counts callers up to 3 hops. Adds points for HTTP routes, repository hotspots, architectural layers, and widely used interfaces. Adds points when the file touches auth, persistence, config, build, or schema.',
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
};

const METHODOLOGY_PARAGRAPHS = [
  'Each hunk gets two scores. Both range from 0 to 100 within this diff. 100 marks the highest-risk hunk in this diff, not a universal scale. Combined blends them at 60 percent Blast Radius and 40 percent Review Priority. Sort by Combined. Read Blast and Priority to see why.',
  'Blast radius measures how far the change can reach. Inputs: callers up to 3 hops, HTTP routes, repository hotspots, architectural layers, interface implementations, cross-package callers, and file paths near auth, persistence, config, build, or schema.',
  'Review priority measures how much attention this hunk needs. Main inputs: a near-duplicate function in another file, and missing direct tests. Secondary inputs: cyclomatic complexity, loop depth, and fan-out.',
  'File co-change coupling adds a small bonus to Blast Radius when git history shows files that change together. Hygiene signals (formatting, comments, generated code, logging, test-only files, dead code) multiply the Combined score down.',
];

const BLAST_CATEGORIES = new Set(['architecture', 'graph']);
const PRIORITY_CATEGORIES = new Set(['duplication', 'code-metrics']);

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

function sumExpression(nums: number[]): { text: string; total: number } {
  if (!nums || nums.length === 0) return { text: '0', total: 0 };
  const total = nums.reduce((a, b) => a + b, 0);
  const text = nums.reduce((acc, n, i) => {
    if (i === 0) return n.toFixed(1);
    return `${acc} ${n >= 0 ? '+' : '-'} ${Math.abs(n).toFixed(1)}`;
  }, '');
  return { text, total };
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

const SignalRow: React.FC<{ s: BlastRadiusSignal; dormant?: boolean }> = ({ s, dormant }) => {
  const positive = (s.Points || 0) >= 0;
  return (
    <li className="flex items-start gap-2 py-0.5 text-xs">
      <span className={classNames('w-12 shrink-0 font-mono', dormant ? 'text-slate-600' : positive ? 'text-emerald-400' : 'text-red-400')}>
        {dormant ? '+0.0' : `${positive ? '+' : ''}${(s.Points || 0).toFixed(1)}`}
      </span>
      <span className={classNames('flex-1', dormant ? 'text-slate-600' : 'text-slate-300')}>
        {s.Name}
        {s.Detail && <span className="ml-1 text-slate-500">— {s.Detail}</span>}
      </span>
    </li>
  );
};

const SignalBlock: React.FC<{ signals: BlastRadiusSignal[]; dormant?: boolean }> = ({ signals, dormant }) => {
  if (distinctSymbolCount(signals) <= 1) {
    return <ul className="space-y-0.5">{signals.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} />)}</ul>;
  }
  const { ungrouped, groups } = groupSignalsBySymbol(signals);
  return (
    <div className="space-y-2">
      {ungrouped.length > 0 && <ul className="space-y-0.5">{ungrouped.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} />)}</ul>}
      {groups.map(([name, sigs]) => (
        <div key={name}>
          <code className="text-[11px] text-slate-500">{shortName(name)}</code>
          <ul className="space-y-0.5">{sigs.map((s, idx) => <SignalRow key={idx} s={s} dormant={dormant} />)}</ul>
        </div>
      ))}
    </div>
  );
};

const SignalList: React.FC<{ signals: BlastRadiusSignal[] }> = ({ signals }) => {
  const [showDormant, setShowDormant] = useState(false);
  const ranked = sortedSignals(signals);
  const active = ranked.filter((s) => (s.Points || 0) !== 0);
  const dormant = ranked.filter((s) => (s.Points || 0) === 0);
  if (active.length === 0 && dormant.length === 0) {
    return <p className="text-xs text-slate-500">No signals recorded.</p>;
  }
  return (
    <div>
      <SignalBlock signals={active} />
      {showDormant && <SignalBlock signals={dormant} dormant />}
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

const DimensionCard: React.FC<{ title: string; norm: number; raw: number; max: number; signals: BlastRadiusSignal[] }> = ({ title, norm, raw, max, signals }) => (
  <div className="rounded-lg border border-slate-700 bg-slate-900 p-3">
    <div className="mb-2">
      <div className="text-sm font-semibold text-slate-200">{title}</div>
      <div className="text-xs text-slate-500">{normMathText(raw, max, norm)}</div>
    </div>
    <SignalList signals={signals} />
  </div>
);

function fileBaseName(path?: string): string {
  return (path || '').split('/').pop() || path || '';
}

function renderMathDimension(
  startStep: number,
  title: string,
  symbolLines: { name: string; signals: BlastRadiusSignal[] }[],
  hunkSignals: BlastRadiusSignal[],
  raw: number,
  max: number,
  norm: number,
  maxSource: { file?: string; header?: string },
  self: { file: string; header: string }
) {
  const perSymbol = symbolLines.map((l) => ({ name: l.name, ...sumExpression(l.signals.map((s) => s.Points || 0)) }));
  const hunkExpr = hunkSignals.length > 0 ? sumExpression(hunkSignals.map((s) => s.Points || 0)) : null;
  const subtotalNums = [...perSymbol.map((l) => l.total), ...(hunkExpr ? [hunkExpr.total] : [])];
  const hasMultipleParts = subtotalNums.length > 1;
  const totalExpr = sumExpression(subtotalNums);

  let n = startStep;
  const stepAdd = n++;
  const stepSum = hasMultipleParts ? n++ : null;
  const stepScale = n++;
  const isSelf = maxSource.file === self.file && maxSource.header === self.header;

  const node = (
    <div className="space-y-2">
      <div className="text-sm font-semibold text-slate-200">{title}</div>
      <div>
        <div className="text-xs font-medium text-slate-400">Step {stepAdd} — add up each symbol's own signals</div>
        {perSymbol.length === 0 && !hunkExpr && <div className="text-xs text-slate-500">No signals fired for this dimension.</div>}
        {perSymbol.map((l, i) => (
          <div key={i} className="text-xs text-slate-300"><code className="text-slate-400">{l.name}</code>: {l.text} = <strong>{l.total.toFixed(1)}</strong></div>
        ))}
        {hunkExpr && <div className="text-xs text-slate-300">This hunk (file-level signals): {hunkExpr.text} = <strong>{hunkExpr.total.toFixed(1)}</strong></div>}
      </div>
      {stepSum !== null && (
        <div>
          <div className="text-xs font-medium text-slate-400">Step {stepSum} — add every subtotal together</div>
          <div className="text-xs text-slate-300">{totalExpr.text} = <strong>{totalExpr.total.toFixed(1)}</strong></div>
        </div>
      )}
      <div>
        <div className="text-xs font-medium text-slate-400">Step {stepScale} — scale against the highest-scoring hunk in this diff</div>
        <div className="text-xs text-slate-300">
          {max ? <>{(raw || 0).toFixed(1)} ÷ {max.toFixed(1)} × 100 = <strong>{(norm || 0).toFixed(1)}</strong></> : 'no hunk in this diff scored above 0 for this dimension, so this is 0'}
        </div>
        {max > 0 && (
          <div className="text-xs text-slate-500">
            {isSelf
              ? `${max.toFixed(1)} is this very hunk's own score — it's the highest for this dimension anywhere in this diff.`
              : `${max.toFixed(1)} is the highest score for this dimension anywhere in this diff, from ${maxSource.file ? fileBaseName(maxSource.file) : 'another hunk'}${maxSource.header ? ` (${maxSource.header})` : ''}.`}
          </div>
        )}
      </div>
    </div>
  );
  return { node, nextStep: n };
}

const MathModeView: React.FC<{ detail: BlastRadiusHunkReport }> = ({ detail }) => {
  const symbols = detail.Symbols || [];
  const weights = detail.Weights && typeof detail.Weights.BlastRadius === 'number' ? detail.Weights : { BlastRadius: 0.6, ReviewPriority: 0.4 };

  function dimensionInputs(categories: Set<string>) {
    const symbolLines = symbols
      .map((sym) => ({ name: sym.Name || shortName(sym.QualifiedName), signals: (sym.Signals || []).filter((s) => categories.has(s.Category)) }))
      .filter((l) => l.signals.length > 0);
    const hunkSigs = (detail.Signals || []).filter((s) => categories.has(s.Category));
    return { symbolLines, hunkSigs };
  }

  const blastNorm = detail.BlastRadiusNorm || 0;
  const priorityNorm = detail.ReviewPriorityNorm || 0;
  const blastInputs = dimensionInputs(BLAST_CATEGORIES);
  const priorityInputs = dimensionInputs(PRIORITY_CATEGORIES);
  const self = { file: detail.FilePath, header: detail.Header };

  const { node: blastNode, nextStep: afterBlast } = renderMathDimension(
    1, 'Blast Radius', blastInputs.symbolLines, blastInputs.hunkSigs,
    detail.BlastRadiusRaw, detail.MaxBlastRadiusRaw, blastNorm,
    { file: detail.MaxBlastRadiusHunkFile, header: detail.MaxBlastRadiusHunkHeader }, self
  );
  const { node: priorityNode, nextStep: afterPriority } = renderMathDimension(
    afterBlast, 'Review Priority', priorityInputs.symbolLines, priorityInputs.hunkSigs,
    detail.ReviewPriorityRaw, detail.MaxReviewPriorityRaw, priorityNorm,
    { file: detail.MaxReviewPriorityHunkFile, header: detail.MaxReviewPriorityHunkHeader }, self
  );

  const hygiene = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
  const blastShare = weights.BlastRadius * blastNorm;
  const priorityShare = weights.ReviewPriority * priorityNorm;
  const blended = blastShare + priorityShare;
  const final = blended * hygiene;
  const stepBlend = afterPriority;
  const stepHygiene = afterPriority + 1;

  return (
    <div className="space-y-4 rounded-lg border border-slate-700 bg-slate-900 p-3">
      {blastNode}
      {priorityNode}
      <div className="space-y-2 border-t border-slate-700 pt-3">
        <div className="text-sm font-semibold text-slate-200">Combine Into Final Score</div>
        <div>
          <div className="text-xs font-medium text-slate-400">Step {stepBlend} — blend Blast Radius and Review Priority</div>
          <div className="text-xs text-slate-300">
            ({weights.BlastRadius.toFixed(2)} × {blastNorm.toFixed(1)}) + ({weights.ReviewPriority.toFixed(2)} × {priorityNorm.toFixed(1)}) = {blastShare.toFixed(1)} + {priorityShare.toFixed(1)} = <strong>{blended.toFixed(1)}</strong>
          </div>
        </div>
        <div>
          <div className="text-xs font-medium text-slate-400">Step {stepHygiene} — apply the hygiene multiplier</div>
          <div className="text-xs text-slate-300">{blended.toFixed(1)} × {hygiene} = <strong>{final.toFixed(1)}</strong></div>
        </div>
        <div className="border-t border-slate-700 pt-2 text-sm">
          <span className="text-slate-400">Final Score: </span>
          Rounded to the nearest whole number: <strong className="text-white">{Math.round(detail.Combined || 0)}</strong> out of 100
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
      <SignalList signals={sym.Signals || []} />
      {totalCallers > 0 && (
        <div>
          <div className="mb-1 text-xs font-medium text-slate-400">Reached from {totalCallers} caller{totalCallers !== 1 ? 's' : ''}</div>
          {callerGroups.map((group) => (
            <CallerGroupView key={group.key} group={group} oldName={sym.RenamedFrom} hoveredCaller={hoveredCaller} onHoverCaller={onHoverCaller} />
          ))}
        </div>
      )}
      <ChipList items={sym.ImpactedPackages} preview={PKGS_PREVIEW} label={`Impacted packages (${(sym.ImpactedPackages || []).length})`} />
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
  const symbols = useMemo(
    () => [...(detail.Symbols || [])].sort((a, b) => (b.BlastRadiusRaw || 0) - (a.BlastRadiusRaw || 0)),
    [detail.Symbols]
  );
  const { blast: blastSignals, priority: prioritySignals } = useMemo(
    () => splitSignalsByDimension(allSignals(detail)),
    [detail]
  );
  const [selectedIdx, setSelectedIdx] = useState(0);
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

  const tier = blastRadiusTier(detail.Combined || 0);

  return (
    <div className="space-y-4 rounded-lg border border-slate-700 bg-slate-800/60 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <ScoreChip hintKey="combined" className={classNames('font-semibold', tier === 'blast-radius-high' ? 'border-red-700 text-red-300' : tier === 'blast-radius-medium' ? 'border-amber-700 text-amber-300' : 'border-slate-600 text-slate-300')}>
          Score {Math.round(detail.Combined || 0)}
        </ScoreChip>
        <ScoreChip hintKey="blast" className="border-slate-600 text-slate-300">Blast {Math.round(detail.BlastRadiusNorm || 0)}</ScoreChip>
        <ScoreChip hintKey="priority" className="border-slate-600 text-slate-300">Priority {Math.round(detail.ReviewPriorityNorm || 0)}</ScoreChip>
        <ScoreChip hintKey="hygiene" className={hygieneMult < 1 ? 'border-amber-700 text-amber-300' : 'border-slate-700 text-slate-600'}>
          {hygieneMult < 1 ? `×${hygieneMult}` : 'Hygiene ×1.0'}
        </ScoreChip>
        <ScoreChip hintKey="coupling" className={couplingVal > 0 ? 'border-slate-600 text-slate-300' : 'border-slate-700 text-slate-600'}>
          {couplingVal > 0 ? `Coupling +${couplingVal.toFixed(1)}` : 'Coupling +0'}
        </ScoreChip>
        <MethodologyTooltip />
      </div>

      <ChipList items={detail.ImpactedPackages} preview={PKGS_PREVIEW} label={`Impacted packages (${(detail.ImpactedPackages || []).length})`} />

      <div className="flex gap-1 border-b border-slate-700 text-xs">
        <button type="button" className={classNames('px-3 py-1.5', scoreMode === 'summary' ? 'border-b-2 border-blue-500 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setScoreMode('summary')}>Summary</button>
        <button type="button" className={classNames('px-3 py-1.5', scoreMode === 'math' ? 'border-b-2 border-blue-500 text-white' : 'text-slate-400 hover:text-slate-200')} onClick={() => setScoreMode('math')}>Math Mode</button>
      </div>

      {scoreMode === 'math' ? (
        <MathModeView detail={detail} />
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <DimensionCard title="Blast Radius" norm={detail.BlastRadiusNorm} raw={detail.BlastRadiusRaw} max={detail.MaxBlastRadiusRaw} signals={blastSignals} />
          <DimensionCard title="Review Priority" norm={detail.ReviewPriorityNorm} raw={detail.ReviewPriorityRaw} max={detail.MaxReviewPriorityRaw} signals={prioritySignals} />
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
          </div>
        </div>
      )}
    </div>
  );
};

export default BlastRadiusPanel;
