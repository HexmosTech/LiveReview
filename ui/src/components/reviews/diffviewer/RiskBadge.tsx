// Ported from git-lrc:internal/staticserve/static/components/RiskBadge.js (as of the
// git-lrc HEAD current when this port was written), rebuilt against LiveReview's Popover
// primitive instead of git-lrc's hand-rolled position:fixed viewport-clamped hover card.
import React from 'react';
import classNames from 'classnames';
import { Popover } from '../../UIPrimitives';
import { BlastRadiusHunkReport } from '../../../types/reviews';
import { allSignals, blastRadiusTier, blastRadiusTierLabel, BlastRadiusTier } from '../../../lib/blastRadius';

const TIER_CLASSES: Record<BlastRadiusTier, string> = {
  'blast-radius-high': 'bg-red-900/60 text-red-200 border-red-700',
  'blast-radius-medium': 'bg-amber-900/50 text-amber-200 border-amber-700',
  'blast-radius-low': 'bg-sky-900/50 text-sky-200 border-sky-700',
  'blast-radius-none': 'bg-slate-800 text-slate-400 border-slate-700',
};

const TIER_BAR_CLASSES: Record<BlastRadiusTier, string> = {
  'blast-radius-high': 'bg-red-500',
  'blast-radius-medium': 'bg-amber-500',
  'blast-radius-low': 'bg-sky-500',
  'blast-radius-none': 'bg-slate-500',
};

interface DimensionBarProps {
  label: string;
  value: number;
  tier: BlastRadiusTier;
  hint: string;
}

const DimensionBar: React.FC<DimensionBarProps> = ({ label, value, tier, hint }) => {
  const pct = Math.max(0, Math.min(100, Math.round(value)));
  return (
    <div className="flex items-center gap-2 text-xs" title={hint}>
      <span className="w-24 shrink-0 text-slate-400">{label}</span>
      <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-slate-700">
        <span className={classNames('block h-full rounded-full', TIER_BAR_CLASSES[tier])} style={{ width: `${pct}%` }} />
      </span>
      <span className="w-6 shrink-0 text-right text-slate-300">{pct}</span>
    </div>
  );
};

interface RiskBadgeProps {
  score: number;
  detail?: BlastRadiusHunkReport | null;
  size?: 'small' | 'large';
  onOpen?: () => void;
}

const RiskBadge: React.FC<RiskBadgeProps> = ({ score, detail, size = 'small', onOpen }) => {
  if (typeof score !== 'number') return null;
  const tier = blastRadiusTier(score);
  const clickable = Boolean(onOpen);

  const badge = (
    <button
      type="button"
      onClick={clickable ? (e) => { e.stopPropagation(); onOpen!(); } : undefined}
      aria-label={`Risk score ${Math.round(score)} out of 100`}
      className={classNames(
        'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 font-mono text-xs font-semibold',
        TIER_CLASSES[tier],
        clickable && 'cursor-pointer hover:brightness-125',
        size === 'large' ? 'text-xs' : 'text-[11px]'
      )}
    >
      {Math.round(score)}
    </button>
  );

  // git-lrc always shows a hover card, even for hunks with no detail — it
  // falls back to a one-line footer message instead of hiding the card
  // entirely (RiskBadge.js:95-97).
  const signals = detail ? allSignals(detail).slice(0, 4) : [];
  const hygiene = detail && typeof detail.HygieneMultiplier === 'number' && detail.HygieneMultiplier < 1 ? detail.HygieneMultiplier : null;
  const totalSignals = detail ? allSignals(detail).length : 0;
  const footerText = totalSignals > signals.length ? `${totalSignals} signals` : 'Full breakdown';

  return (
    <Popover
      hover
      align="left"
      trigger={badge}
      className="w-80 !p-3"
    >
      <div className="flex items-center gap-3 border-b border-slate-700 pb-2">
        <span className="font-mono text-3xl font-bold text-white">{Math.round(score)}</span>
        <div>
          <div className="text-sm font-medium text-slate-200">{blastRadiusTierLabel(score)}</div>
          <div className="text-[9px] uppercase tracking-widest text-slate-500">Risk Assessment</div>
        </div>
      </div>
      {detail ? (
        <>
          <div className="space-y-1.5 py-2">
            <DimensionBar
              label="Blast Radius"
              value={detail.BlastRadiusNorm || 0}
              tier={tier}
              hint="How widely this change can propagate: callers, entry points, architectural role"
            />
            <DimensionBar
              label="Review Priority"
              value={detail.ReviewPriorityNorm || 0}
              tier={tier}
              hint="How much reviewer attention it deserves: duplication, complexity, test coverage"
            />
          </div>
          {hygiene !== null && (
            <p className="border-t border-slate-700 pt-2 text-xs text-slate-400">
              Score dampened ×{hygiene} — change looks low-value (formatting/comments/generated)
            </p>
          )}
          {signals.length > 0 && (
            <ul className="space-y-1 border-t border-slate-700 py-2">
              {signals.map((s, i) => (
                <li key={i} className="flex items-start gap-2 text-xs">
                  <span className={classNames('font-mono', (s.Points || 0) < 0 ? 'text-red-400' : 'text-emerald-400')}>
                    {(s.Points || 0) >= 0 ? '+' : ''}{(s.Points || 0).toFixed(1)}
                  </span>
                  <span className="text-slate-300">{s.Name}</span>
                </li>
              ))}
            </ul>
          )}
        </>
      ) : (
        <p className="border-t border-slate-700 py-2 text-xs text-slate-500">
          Relative importance of this hunk within the review
        </p>
      )}
      <div className="flex items-center justify-between gap-2 border-t border-slate-700 pt-2">
        {detail && <span className="text-[10px] text-slate-500">{footerText}</span>}
        {onOpen && (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onOpen(); }}
            className={classNames(
              'rounded-md border border-sky-800/50 bg-sky-500/10 py-1 text-xs text-sky-300 hover:bg-sky-500/20',
              detail ? 'px-3' : 'w-full'
            )}
          >
            Open breakdown
          </button>
        )}
      </div>
    </Popover>
  );
};

export default RiskBadge;
