// Math Mode derivation for the diff-viewer Blast Radius panel.
//
// This is the TypeScript mirror of git-lrc's
// internal/staticserve/static/components/blast_math_model.mjs and of
// internal/blastradius.ComputeMathMode on the server. All three must produce
// the same steps and totals; tests in both repos pin the same real-review
// fixtures.
//
// Two roll-up formulas exist in real artifacts:
//   - linear: sum(symbols) + hunk signals (reports before 2026-10-01)
//   - sqrt:   sqrt(sum(symbols)) + hunk signals (git-lrc's current formula)
//
// Current reports carry SymbolCombine: "sqrt". Older artifacts are silent,
// so the mode is inferred from the recorded raw score. Cross-repo bonuses are
// added after the roll-up; when that leaves a gap between the formula and the
// recorded raw, it becomes an explicit adjustment step.

import {
  BlastRadiusHunkReport,
  BlastRadiusSignal,
} from '../types/reviews';
import { SEVERITY_WEIGHT, blendRiskScore } from './blastRadius';

export type SymbolCombineMode = 'linear' | 'sqrt';

export const BLAST_CATEGORIES = new Set(['architecture', 'graph']);
export const PRIORITY_CATEGORIES = new Set(['duplication', 'code-metrics']);

export interface MathExpression {
  text: string;
  total: number;
}

export interface MathSubtotalLine {
  name: string;
  signals: BlastRadiusSignal[];
  text: string;
  total: number;
}

export interface MathDimension {
  title: string;
  mode: SymbolCombineMode;
  declaredCombine?: string;
  perSymbol: MathSubtotalLine[];
  hunkSignals: BlastRadiusSignal[];
  hunkExpression: MathExpression | null;
  hunkSubtotal: number;
  symbolSubtotal: number;
  crossRepoBonus: number;
  combinedSymbols: number;
  sumExpression: MathExpression;
  total: number;
  raw: number;
  artifactDelta: number;
  max: number;
  maxInferred: boolean;
  norm: number;
  maxSource: { file?: string; header?: string };
  isSelf: boolean;
  stepAdd: number;
  stepSum: number;
  stepCombine: number;
  stepHunk: number;
  stepAdjust: number;
  stepScale: number;
  nextStep: number;
}

export interface MathModeResult {
  blast: MathDimension;
  priority: MathDimension;
  weights: { BlastRadius: number; ReviewPriority: number };
  blastShare: number;
  priorityShare: number;
  blended: number;
  hygieneMultiplier: number;
  structural: number;
  severityScore: number;
  severityLabel: 'critical' | 'warning' | 'info' | null;
  severityCounts: { critical: number; warning: number; info: number };
  severityShare: number;
  final: number;
  stepBlend: number;
  stepHygiene: number;
  stepSeverity: number;
  stepFinalBlend: number;
}

function shortName(qualifiedName: string | undefined): string {
  const parts = (qualifiedName || '').split('.');
  return parts[parts.length - 1] || qualifiedName || '';
}

// sumExpression spells a list of numbers out as a literal "1.8 + 1.4 - 0.9"
// arithmetic expression plus its total. Math Mode's whole point is to show
// the addition, never just assert the result.
export function sumExpression(nums: number[]): MathExpression {
  if (!nums || nums.length === 0) return { text: '0', total: 0 };
  const total = nums.reduce((a, b) => a + b, 0);
  const text = nums.reduce((acc, n, i) => {
    if (i === 0) return n.toFixed(1);
    return `${acc} ${n >= 0 ? '+' : '-'} ${Math.abs(n).toFixed(1)}`;
  }, '');
  return { text, total };
}

function filterByCategory(signals: BlastRadiusSignal[] | undefined, categories: Set<string>): BlastRadiusSignal[] {
  return (signals || []).filter((s) => categories.has(s.Category));
}

function buildSymbolLines(
  symbols: BlastRadiusHunkReport['Symbols'],
  categories: Set<string>,
): MathSubtotalLine[] {
  return (symbols || [])
    .map((sym) => {
      const signals = filterByCategory(sym.Signals, categories);
      if (signals.length === 0) return null;
      return {
        name: sym.Name || shortName(sym.QualifiedName),
        signals,
        ...sumExpression(signals.map((s) => s.Points || 0)),
      };
    })
    .filter((line): line is MathSubtotalLine => line !== null);
}

// detectCombineMode picks the roll-up used by the artifact's generator.
// SymbolCombine is authoritative when present; otherwise the recorded raw
// score is the ground truth and we choose the formula that reproduces it.
export function detectCombineMode(
  declared: string | undefined,
  symbolSubtotal: number,
  hunkSubtotal: number,
  raw: number,
  crossRepoBonus = 0,
): { mode: SymbolCombineMode; total: number; delta: number } {
  const linear = symbolSubtotal + hunkSubtotal;
  const saturating = Math.sqrt(symbolSubtotal) + hunkSubtotal;
  const hint = (declared || '').toLowerCase();
  if (hint === 'sqrt') {
    return { mode: 'sqrt', total: saturating, delta: raw - saturating };
  }
  if (hint === 'linear') {
    return { mode: 'linear', total: linear, delta: raw - linear };
  }
  // Cross-repo enrichment adds its bonus after the roll-up, to both the
  // symbol sum and the recorded raw score. Reconstruct what the sqrt
  // formula produced before that bonus so a historical cross-repo artifact
  // is not mistaken for linear.
  const base = Math.max(0, symbolSubtotal - crossRepoBonus);
  const saturatingWithBonus = Math.sqrt(base) + hunkSubtotal + crossRepoBonus;
  const saturatingDelta = Math.min(
    Math.abs(raw - saturating),
    Math.abs(raw - saturatingWithBonus),
  );
  if (saturatingDelta + 1e-9 < Math.abs(raw - linear)) {
    return { mode: 'sqrt', total: saturating, delta: raw - saturating };
  }
  return { mode: 'linear', total: linear, delta: raw - linear };
}

function buildDimension(opts: {
  startStep: number;
  title: string;
  symbols: BlastRadiusHunkReport['Symbols'];
  hunkSignalList: BlastRadiusSignal[] | undefined;
  categories: Set<string>;
  raw: number;
  max: number;
  norm: number;
  maxSource: { file?: string; header?: string };
  self: { file: string; header: string };
  declaredCombine?: string;
}): MathDimension {
  const {
    startStep, title, symbols, hunkSignalList, categories,
    raw, max, norm, maxSource, self, declaredCombine,
  } = opts;

  const perSymbol = buildSymbolLines(symbols, categories);
  const hunkSignals = filterByCategory(hunkSignalList, categories);
  const hunkExpression = hunkSignals.length > 0
    ? sumExpression(hunkSignals.map((s) => s.Points || 0))
    : null;
  const symbolSubtotal = perSymbol.reduce((sum, line) => sum + line.total, 0);
  const hunkSubtotal = hunkExpression ? hunkExpression.total : 0;

  let crossRepoBonus = 0;
  if (categories.has('graph')) {
    (symbols || []).forEach((sym) => {
      (sym.Signals || []).forEach((sig) => {
        if (sig.Name === 'Cross-repo impact' || sig.Name === 'Cross-repo chain') {
          crossRepoBonus += sig.Points || 0;
        }
      });
    });
  }

  const combined = detectCombineMode(declaredCombine, symbolSubtotal, hunkSubtotal, raw, crossRepoBonus);

  const linearParts = [...perSymbol.map((line) => line.total)];
  if (hunkExpression) linearParts.push(hunkSubtotal);
  const sumExpressionParts = combined.mode === 'linear'
    ? sumExpression(linearParts)
    : sumExpression(perSymbol.map((line) => line.total));

  const combinedSymbols = Math.sqrt(symbolSubtotal);

  let n = startStep;
  const stepAdd = n++;
  const stepSum = (combined.mode === 'linear'
    ? linearParts.length > 1
    : perSymbol.length > 1)
    ? n++
    : 0;
  const stepCombine = combined.mode === 'sqrt' && symbolSubtotal > 0 ? n++ : 0;
  const stepHunk = combined.mode === 'sqrt' && hunkExpression ? n++ : 0;
  const stepAdjust = Math.abs(combined.delta) > 0.05 ? n++ : 0;
  const stepScale = n++;

  const recordedMax = max || 0;
  const effectiveMax = recordedMax > 0
    ? recordedMax
    : (raw > 0 && norm > 0 ? raw / (norm / 100) : 0);
  const isSelf = Boolean(recordedMax > 0 && self && maxSource
    && maxSource.file === self.file && maxSource.header === self.header);

  return {
    title,
    mode: combined.mode,
    declaredCombine,
    perSymbol,
    hunkSignals,
    hunkExpression,
    hunkSubtotal,
    symbolSubtotal,
    crossRepoBonus,
    combinedSymbols,
    sumExpression: sumExpressionParts,
    total: combined.total,
    raw: raw || 0,
    artifactDelta: combined.delta,
    max: effectiveMax,
    maxInferred: recordedMax <= 0 && effectiveMax > 0,
    norm: norm || 0,
    maxSource: maxSource || {},
    isSelf,
    stepAdd,
    stepSum,
    stepCombine,
    stepHunk,
    stepAdjust,
    stepScale,
    nextStep: n,
  };
}

export function buildMathMode(detail: BlastRadiusHunkReport): MathModeResult {
  const symbols = detail.Symbols || [];
  const weights = detail.Weights && typeof detail.Weights.BlastRadius === 'number'
    ? detail.Weights
    : { BlastRadius: 0.6, ReviewPriority: 0.4 };
  const self = { file: detail.FilePath, header: detail.Header };

  const blast = buildDimension({
    startStep: 1,
    title: 'Blast Radius',
    symbols,
    hunkSignalList: detail.Signals,
    categories: BLAST_CATEGORIES,
    raw: detail.BlastRadiusRaw,
    max: detail.MaxBlastRadiusRaw,
    norm: detail.BlastRadiusNorm,
    maxSource: { file: detail.MaxBlastRadiusHunkFile, header: detail.MaxBlastRadiusHunkHeader },
    self,
    declaredCombine: detail.SymbolCombine,
  });
  const priority = buildDimension({
    startStep: blast.nextStep,
    title: 'Review Priority',
    symbols,
    hunkSignalList: detail.Signals,
    categories: PRIORITY_CATEGORIES,
    raw: detail.ReviewPriorityRaw,
    max: detail.MaxReviewPriorityRaw,
    norm: detail.ReviewPriorityNorm,
    maxSource: { file: detail.MaxReviewPriorityHunkFile, header: detail.MaxReviewPriorityHunkHeader },
    self,
    declaredCombine: detail.SymbolCombine,
  });

  const blastShare = weights.BlastRadius * blast.norm;
  const priorityShare = weights.ReviewPriority * priority.norm;
  const blended = blastShare + priorityShare;
  const hygieneMultiplier = typeof detail.HygieneMultiplier === 'number' ? detail.HygieneMultiplier : 1.0;
  const structural = blended * hygieneMultiplier;
  const severityScore = typeof detail.FindingSeverity === 'number' ? detail.FindingSeverity : 0;
  const severityLabel = detail.FindingSeverityLabel || null;
  const severityCounts = detail.FindingSeverityCounts || { critical: 0, warning: 0, info: 0 };
  const severityShare = SEVERITY_WEIGHT * severityScore;
  const final = blendRiskScore(structural, severityScore);

  const stepBlend = priority.nextStep;
  const stepHygiene = stepBlend + 1;
  const stepSeverity = stepBlend + 2;
  const stepFinalBlend = stepBlend + 3;

  return {
    blast,
    priority,
    weights,
    blastShare,
    priorityShare,
    blended,
    hygieneMultiplier,
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
  };
}
