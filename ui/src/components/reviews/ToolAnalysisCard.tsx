import React, { useState } from 'react';

export interface ToolBreakdownItem {
  toolName: string;
  creditsUsed: number;
  commentsGenerated: number;
  status: 'pending' | 'running' | 'clean' | 'completed' | 'failed' | 'partial' | string;
}

export interface ToolAccountingData {
  totalToolCredits: number;
  toolsExecuted: number;
  totalCommentsGenerated: number;
  toolBreakdown: ToolBreakdownItem[];
}

interface ToolAnalysisCardProps {
  data: ToolAccountingData;
  embedded?: boolean;
  isExpanded?: boolean;
  onToggle?: () => void;
  hideSummary?: boolean;
  onToolSelect?: (item: ToolBreakdownItem) => void;
}

const isToolClean = (item: ToolBreakdownItem): boolean =>
  item.commentsGenerated === 0 &&
  item.status !== 'failed' &&
  item.status !== 'running' &&
  item.status !== 'pending' &&
  item.status !== 'partial';

const sortByImpact = (items: ToolBreakdownItem[]): ToolBreakdownItem[] =>
  [...items].sort((a, b) => {
    const getPriority = (item: ToolBreakdownItem) => {
      if (item.status === 'failed') return 1;
      if (item.commentsGenerated > 0) return 2;
      if (item.status === 'partial') return 3;
      if (item.status === 'running') return 4;
      if (item.status === 'pending') return 5;
      return 6;
    };
    const pDiff = getPriority(a) - getPriority(b);
    if (pDiff !== 0) return pDiff;
    if (a.commentsGenerated !== b.commentsGenerated) {
      return b.commentsGenerated - a.commentsGenerated;
    }
    return a.toolName.localeCompare(b.toolName);
  });

const safeCredits = (credits: unknown): number =>
  typeof credits === 'number' && Number.isFinite(credits) ? credits : 0;

const sumGroupCredits = (items: ToolBreakdownItem[]): number =>
  items.reduce((sum, item) => sum + safeCredits(item.creditsUsed), 0);

const PAGE_SIZE = 15;

export const ToolAnalysisCard: React.FC<ToolAnalysisCardProps> = ({
  data,
  embedded = false,
  isExpanded,
  onToggle,
  hideSummary = false,
  onToolSelect,
}) => {
  const [internalExpanded, setInternalExpanded] = useState(false);
  const [page, setPage] = useState(0);
  const expanded = isExpanded !== undefined ? isExpanded : internalExpanded;
  const handleToggle = () => (onToggle !== undefined ? onToggle() : setInternalExpanded((prev) => !prev));

  const breakdown = data?.toolBreakdown ?? [];
  const totalComments = data?.totalCommentsGenerated ?? 0;
  const totalTools = breakdown.length || (data?.toolsExecuted ?? 0);
  const activeRunningTools = breakdown.filter((t) => t.status === 'running');
  const queuedTools = breakdown.filter((t) => t.status === 'pending');
  const completedTools = breakdown.filter((t) => t.status !== 'running' && t.status !== 'pending');

  const isRunning = activeRunningTools.length > 0;
  const isQueued = queuedTools.length > 0 && activeRunningTools.length === 0;
  const hasFindings = totalComments > 0;
  const hasFailures = breakdown.some((item) => item.status === 'failed');
  const hasPartial = breakdown.some((item) => item.status === 'partial');

  // Group tools by outcome and order by impact, then paginate at PAGE_SIZE so 100+ tools stay bounded without scrolling
  const failedAll = sortByImpact(breakdown.filter((t) => t.status === 'failed'));
  const inProgressAll = sortByImpact(
    breakdown.filter((t) => t.status === 'running' || t.status === 'pending')
  );
  const findingsAll = sortByImpact(
    breakdown.filter(
      (t) =>
        t.status !== 'failed' &&
        t.status !== 'running' &&
        t.status !== 'pending' &&
        (t.commentsGenerated > 0 || t.status === 'partial')
    )
  );
  const cleanAll = sortByImpact(
    breakdown.filter(
      (t) =>
        t.status !== 'failed' &&
        t.status !== 'running' &&
        t.status !== 'pending' &&
        isToolClean(t)
    )
  );

  const orderedTools = [...failedAll, ...inProgressAll, ...findingsAll, ...cleanAll];
  const totalItems = orderedTools.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / PAGE_SIZE));
  const currentPage = Math.min(page, totalPages - 1);
  const startIdx = currentPage * PAGE_SIZE;
  const endIdx = Math.min(startIdx + PAGE_SIZE, totalItems);
  const pageItems = orderedTools.slice(startIdx, endIdx);

  const failedGroup = pageItems.filter((t) => t.status === 'failed');
  const inProgressGroup = pageItems.filter((t) => t.status === 'running' || t.status === 'pending');
  const findingsGroup = pageItems.filter(
    (t) =>
      t.status !== 'failed' &&
      t.status !== 'running' &&
      t.status !== 'pending' &&
      (t.commentsGenerated > 0 || t.status === 'partial')
  );
  const cleanGroup = pageItems.filter(
    (t) =>
      t.status !== 'failed' &&
      t.status !== 'running' &&
      t.status !== 'pending' &&
      isToolClean(t)
  );

  const formatGroupCount = (visibleCount: number, totalGroupCount: number) =>
    visibleCount < totalGroupCount ? `${visibleCount} of ${totalGroupCount}` : `${totalGroupCount}`;

  const renderStatusDot = (item: ToolBreakdownItem) => {
    if (item.status === 'failed') {
      return <span className="w-2 h-2 rounded-full bg-red-400 shrink-0" aria-hidden="true" />;
    }
    if (item.status === 'running') {
      return <span className="w-2 h-2 rounded-full bg-blue-400 animate-pulse motion-reduce:animate-none shrink-0" aria-hidden="true" />;
    }
    if (item.commentsGenerated > 0 || item.status === 'partial') {
      return <span className="w-2 h-2 rounded-full bg-blue-400 shrink-0" aria-hidden="true" />;
    }
    if (item.status === 'pending') {
      return <span className="w-2 h-2 rounded-full bg-slate-500 shrink-0" aria-hidden="true" />;
    }
    return <span className="w-2 h-2 rounded-full bg-emerald-400/75 shrink-0" aria-hidden="true" />;
  };

  const renderTileOutcome = (item: ToolBreakdownItem) => {
    if (item.status === 'running') {
      return (
        <span className="inline-flex items-center gap-1.5 text-xs font-medium text-blue-400">
          <svg className="w-3.5 h-3.5 animate-spin motion-reduce:animate-none" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          Running
        </span>
      );
    }
    if (item.status === 'pending') {
      return <span className="text-xs text-slate-400">Queued</span>;
    }
    if (item.status === 'failed') {
      return <span className="text-xs font-medium text-red-400">Failed</span>;
    }
    if (item.status === 'partial') {
      return (
        <span className="text-xs text-slate-300 tabular-nums" title="Diff was truncated/capped at limits">
          {item.commentsGenerated > 0 ? (
            <>
              <span className="text-slate-100 font-semibold">{item.commentsGenerated}</span>{' '}
              <span className="text-slate-400">partial</span>
            </>
          ) : (
            'Partial'
          )}
        </span>
      );
    }
    return (
      <span className="text-xs tabular-nums">
        <span className="text-slate-100 font-semibold">{item.commentsGenerated}</span>{' '}
        <span className="text-slate-400">{item.commentsGenerated === 1 ? 'finding' : 'findings'}</span>
      </span>
    );
  };

  const renderTile = (item: ToolBreakdownItem, tone: 'danger' | 'primary', idx: number) => {
    const clickable = Boolean(onToolSelect);
    const cardClass =
      tone === 'danger'
        ? 'bg-red-950/25 border-red-900/50 hover:border-red-800/70'
        : 'bg-slate-900/80 border-slate-700/60 hover:border-slate-600/80';

    return (
      <div
        key={`${item.toolName}-${idx}`}
        onClick={clickable ? () => onToolSelect?.(item) : undefined}
        className={`border rounded-lg px-4 py-3 transition-colors ${cardClass} ${
          clickable ? 'cursor-pointer' : ''
        }`}
        title={
          item.status === 'partial'
            ? `${item.toolName} · Diff truncated/capped at limits`
            : `${item.toolName} · ${safeCredits(item.creditsUsed).toFixed(1)} credits`
        }
      >
        <div className="flex items-center gap-2 min-w-0">
          {renderStatusDot(item)}
          <span className="font-mono text-[13px] font-semibold text-slate-100 truncate">
            {item.toolName}
          </span>
        </div>
        <div className="mt-1.5 flex items-baseline gap-1.5 text-xs tabular-nums">
          {renderTileOutcome(item)}
          <span className="text-slate-600" aria-hidden="true">·</span>
          <span className="font-mono text-slate-400">
            {safeCredits(item.creditsUsed).toFixed(1)} cr
          </span>
        </div>
      </div>
    );
  };

  const hasPrimaryGroups = failedGroup.length > 0 || inProgressGroup.length > 0 || findingsGroup.length > 0;

  return (
    <div
      className={`w-full ${
        embedded
          ? 'flex-1 flex flex-col'
          : 'bg-slate-900/90 border border-slate-700/80 rounded-xl p-4 shadow-xl text-xs overflow-hidden transition-all duration-200'
      }`}
    >
      {/* Standalone summary bar when rendered outside ReviewDetailHeader */}
      {!hideSummary && (
        <div
          className="flex flex-wrap items-center justify-between gap-3 select-none cursor-pointer"
          onClick={handleToggle}
        >
          <div className="flex items-center gap-2.5">
            <span
              className={`w-2 h-2 rounded-full ${
                isRunning
                  ? 'bg-blue-400 animate-pulse motion-reduce:animate-none'
                  : isQueued
                  ? 'bg-slate-500'
                  : hasFailures
                  ? 'bg-red-400'
                  : hasFindings || hasPartial
                  ? 'bg-blue-400'
                  : 'bg-emerald-400'
              }`}
            />
            <h2 className="text-sm font-semibold text-white tracking-wide">Static Analysis Tools</h2>
            <span className="text-xs text-slate-400">
              {isRunning
                ? `Running ${completedTools.length}/${totalTools}`
                : isQueued
                ? `Queued 0/${totalTools}`
                : hasFailures
                ? 'Failed'
                : hasFindings
                ? `${totalComments} ${totalComments === 1 ? 'finding' : 'findings'}`
                : 'All clean'}
            </span>
          </div>

          <div className="flex items-center gap-3 text-xs font-mono text-slate-400">
            <span>
              <strong className="text-slate-200">{totalTools}</strong> {totalTools === 1 ? 'tool' : 'tools'}
            </span>
            <span className="text-slate-600">·</span>
            <span>{safeCredits(data?.totalToolCredits).toFixed(1)} cr</span>
          </div>
        </div>
      )}

      {expanded && (
        <div
          className={`w-full ${
            embedded ? 'flex-1 flex flex-col justify-between gap-3' : 'space-y-3'
          } ${hideSummary ? '' : 'mt-3 pt-3 border-t border-slate-800'}`}
        >
          <div className="space-y-3">
          {failedGroup.length > 0 && (
            <section>
              <div className="flex items-center justify-between mb-1.5">
                <span className="inline-flex items-center gap-1.5 text-[10px] font-semibold text-red-300 uppercase tracking-widest">
                  <span className="w-1.5 h-1.5 rounded-full bg-red-400" aria-hidden="true" />
                  Failed ({formatGroupCount(failedGroup.length, failedAll.length)})
                </span>
                <span className="font-mono text-[11px] text-slate-400 tabular-nums">
                  {sumGroupCredits(failedAll).toFixed(1)} cr
                </span>
              </div>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-2.5">
                {failedGroup.map((item, idx) => renderTile(item, 'danger', idx))}
              </div>
            </section>
          )}

          {inProgressGroup.length > 0 && (
            <section>
              <div className="flex items-center justify-between mb-1.5">
                <span className="inline-flex items-center gap-1.5 text-[10px] font-semibold text-slate-400 uppercase tracking-widest">
                  <span className="w-1.5 h-1.5 rounded-full bg-blue-400 animate-pulse motion-reduce:animate-none" aria-hidden="true" />
                  In Progress ({formatGroupCount(inProgressGroup.length, inProgressAll.length)})
                </span>
                <span className="font-mono text-[11px] text-slate-400 tabular-nums">
                  {activeRunningTools.length} running · {queuedTools.length} queued
                </span>
              </div>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-2.5">
                {inProgressGroup.map((item, idx) => renderTile(item, 'primary', idx))}
              </div>
            </section>
          )}

          {findingsGroup.length > 0 && (
            <section>
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-[10px] font-semibold text-slate-400 uppercase tracking-widest">
                  {findingsAll.some((t) => t.status === 'partial' && t.commentsGenerated === 0)
                    ? `Findings & Partial (${formatGroupCount(findingsGroup.length, findingsAll.length)})`
                    : `With Findings (${formatGroupCount(findingsGroup.length, findingsAll.length)})`}
                </span>
                <span className="font-mono text-[11px] text-slate-400 tabular-nums">
                  {totalComments} {totalComments === 1 ? 'finding' : 'findings'} ·{' '}
                  {sumGroupCredits(findingsAll).toFixed(1)} cr
                </span>
              </div>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-2.5">
                {findingsGroup.map((item, idx) => renderTile(item, 'primary', idx))}
              </div>
            </section>
          )}

          {cleanGroup.length > 0 && (
            <section className={hasPrimaryGroups ? 'pt-3 border-t border-slate-800/70' : ''}>
              <div className="flex items-center justify-between mb-2">
                <span className="text-[10px] font-semibold text-slate-400 uppercase tracking-widest">
                  Clean ({formatGroupCount(cleanGroup.length, cleanAll.length)})
                </span>
                <span className="font-mono text-[11px] text-slate-400 tabular-nums">
                  {sumGroupCredits(cleanAll).toFixed(1)} cr
                </span>
              </div>
              <div className="flex flex-wrap gap-2.5">
                {cleanGroup.map((item, idx) => {
                  const clickable = Boolean(onToolSelect);
                  return (
                    <div
                      key={`${item.toolName}-${idx}`}
                      onClick={clickable ? () => onToolSelect?.(item) : undefined}
                      className={`inline-flex items-center gap-2 bg-slate-900/50 border border-slate-800/90 hover:border-slate-700 rounded-lg px-3.5 py-2 transition-colors ${
                        clickable ? 'cursor-pointer' : ''
                      }`}
                      title={`${item.toolName} · Clean · ${safeCredits(item.creditsUsed).toFixed(1)} credits`}
                    >
                      {renderStatusDot(item)}
                      <span className="font-mono text-xs font-medium text-slate-200">
                        {item.toolName}
                      </span>
                      <span className="text-slate-600 text-xs" aria-hidden="true">·</span>
                      <span className="font-mono text-xs text-slate-400 tabular-nums">
                        {safeCredits(item.creditsUsed).toFixed(1)} cr
                      </span>
                    </div>
                  );
                })}
              </div>
            </section>
          )}
          </div>

          {totalPages > 1 && (
            <div className="flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-slate-800/70 text-[11px]">
              <span className="font-mono text-slate-400 tabular-nums">
                Showing {startIdx + 1}–{endIdx} of {totalItems} tools
              </span>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  aria-label="Previous page of tools"
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                  disabled={currentPage === 0}
                  className="px-2.5 py-1 rounded border border-slate-700/70 bg-slate-900/70 text-slate-300 hover:border-slate-500 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
                >
                  ← Prev
                </button>
                <span className="px-2 font-mono text-slate-300 tabular-nums">
                  {currentPage + 1} / {totalPages}
                </span>
                <button
                  type="button"
                  aria-label="Next page of tools"
                  onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                  disabled={currentPage >= totalPages - 1}
                  className="px-2.5 py-1 rounded border border-slate-700/70 bg-slate-900/70 text-slate-300 hover:border-slate-500 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
                >
                  Next →
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
