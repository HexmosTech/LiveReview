import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Icons } from '../UIPrimitives';

export interface ToolBreakdownItem {
  toolName: string;
  creditsUsed: number;
  commentsGenerated: number;
  status: 'pending' | 'running' | 'clean' | 'completed' | 'failed' | string;
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
}

export const ToolAnalysisCard: React.FC<ToolAnalysisCardProps> = ({ data, embedded = false, isExpanded, onToggle, hideSummary = false }) => {
  const [internalExpanded, setInternalExpanded] = useState(false);
  // Use controlled expanded if provided, otherwise use internal state
  const expanded = isExpanded !== undefined ? isExpanded : internalExpanded;
  const setExpanded = onToggle !== undefined ? () => onToggle() : setInternalExpanded;
  const [sortBy, setSortBy] = useState<'status' | 'name' | 'credits'>('status');
  const [statusFilter, setStatusFilter] = useState<'all' | 'findings' | 'clean' | 'running' | 'queued' | 'failed'>(
    data.totalCommentsGenerated > 0 ? 'findings' : 'all'
  );
  const [filterOpen, setFilterOpen] = useState(false);

  const totalTools = data.toolBreakdown.length || data.toolsExecuted;
  const activeRunningTools = data.toolBreakdown.filter((t) => t.status === 'running');
  const queuedTools = data.toolBreakdown.filter((t) => t.status === 'pending');
  const completedTools = data.toolBreakdown.filter((t) => t.status !== 'running' && t.status !== 'pending');

  const isRunning = activeRunningTools.length > 0;
  const isQueued = queuedTools.length > 0 && activeRunningTools.length === 0;
  const hasFindings = data.totalCommentsGenerated > 0;
  const hasFailures = data.toolBreakdown.some((item) => item.status === 'failed');

  // Count tools by status for filter tabs
  const findingsCount = data.toolBreakdown.filter((t) => t.commentsGenerated > 0).length;
  const cleanCount = data.toolBreakdown.filter((t) => t.commentsGenerated === 0 && t.status !== 'failed' && t.status !== 'running' && t.status !== 'pending').length;
  const activeRunningCount = activeRunningTools.length;
  const queuedCount = queuedTools.length;
  const failedCount = data.toolBreakdown.filter((t) => t.status === 'failed').length;

  // Filter items
  const filteredTools = data.toolBreakdown.filter((t) => {
    if (statusFilter === 'findings') return t.commentsGenerated > 0;
    if (statusFilter === 'clean') return t.commentsGenerated === 0 && t.status !== 'failed' && t.status !== 'running' && t.status !== 'pending';
    if (statusFilter === 'running') return t.status === 'running';
    if (statusFilter === 'queued') return t.status === 'pending';
    if (statusFilter === 'failed') return t.status === 'failed';
    return true;
  });

  // Sort items
  const sortedTools = [...filteredTools].sort((a, b) => {
    if (sortBy === 'name') {
      return a.toolName.localeCompare(b.toolName);
    }
    if (sortBy === 'credits') {
      return b.creditsUsed - a.creditsUsed;
    }
    // Default: Sort by Status priority (Findings > Failed > Running > Pending > Clean)
    const getPriority = (item: ToolBreakdownItem) => {
      if (item.commentsGenerated > 0) return 1;
      if (item.status === 'failed') return 2;
      if (item.status === 'running') return 3;
      if (item.status === 'pending') return 4;
      return 5;
    };
    return getPriority(a) - getPriority(b);
  });

  return (
    <div className={`w-full ${embedded
      ? ''
      : 'bg-slate-900/90 border border-slate-700/80 rounded-xl p-4 shadow-xl text-xs overflow-hidden transition-all duration-200'
    }`}>
      {/* Summary Bar */}
      {!hideSummary && (
        <div className="flex flex-wrap items-center justify-between gap-3 select-none">
          <div className="flex items-center gap-3">
            {/* Dynamic Status Icon */}
            {isRunning ? (
              <div className="p-1.5 rounded-lg bg-blue-950/60 border border-blue-700/50">
                <svg className="w-4 h-4 text-blue-400 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
              </div>
            ) : isQueued ? (
              <div className="p-1.5 rounded-lg bg-slate-800 border border-slate-700">
                <svg className="w-4 h-4 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
            ) : hasFindings ? (
              <div className="p-1.5 rounded-lg bg-amber-950/60 border border-amber-800/60">
                <svg className="w-4 h-4 text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
            ) : (
              <div className="p-1.5 rounded-lg bg-emerald-950/60 border border-emerald-800/60">
                <svg className="w-4 h-4 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
            )}

            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-bold text-white tracking-wide">
                  Static Analysis Tools
                </h2>

                {/* Dynamic Status Badge */}
                {isRunning ? (
                  <span className="font-mono text-[11px] px-2 py-0.5 rounded bg-blue-950/80 text-blue-300 border border-blue-700/60 font-semibold animate-pulse">
                    Running ({completedTools.length}/{totalTools})
                  </span>
                ) : isQueued ? (
                  <span className="font-mono text-[11px] px-2 py-0.5 rounded bg-slate-800 text-slate-400 border border-slate-700 font-medium">
                    Queued (0/{totalTools})
                  </span>
                ) : (
                  <span className={`font-mono text-[11px] px-2 py-0.5 rounded border font-semibold ${
                    hasFailures
                      ? 'bg-red-950/60 text-red-300 border-red-800/60'
                      : hasFindings
                      ? 'bg-amber-950/60 text-amber-300 border-amber-800/60'
                      : 'bg-emerald-950/60 text-emerald-300 border-emerald-800/60'
                  }`}>
                    {hasFindings ? `${data.totalCommentsGenerated} Findings Flagged` : 'All Clean'}
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Dynamic Metric Summary */}
          <div className="flex items-center gap-4 text-xs font-mono">
            <div className="hidden sm:flex items-center gap-3 text-slate-300">
              <span><strong className="text-white">{totalTools}</strong> Tools</span>
              <span className="text-slate-600">•</span>
              <span><strong className={hasFindings ? 'text-amber-400' : 'text-slate-300'}>{data.totalCommentsGenerated}</strong> Findings</span>
              <span className="text-slate-600">•</span>
              <span className="text-slate-300"><strong className="text-slate-200">{data.totalToolCredits.toFixed(1)}</strong> cr</span>
            </div>
          </div>
        </div>
      )}

      {/* Expanded Multi-Column Panel with Framer Motion Animation */}
      <AnimatePresence>
        {expanded && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: 'auto' }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.2 }}
            className="w-full mt-1.5 pt-0.5"
          >
            {/* Controls Bar: Grouped Filter & Sort Pills Aligned to the Right */}
            <div className="flex flex-wrap items-center justify-end gap-x-4 gap-y-1.5 pb-2 mb-2">
              {/* Filter Pills */}
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-slate-400 font-medium text-[11px] shrink-0 mr-1">Filter:</span>

                <button
                  type="button"
                  onClick={() => setStatusFilter('all')}
                  className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                    statusFilter === 'all'
                      ? 'bg-slate-700 text-white font-semibold border border-slate-500'
                      : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                  }`}
                >
                  All ({totalTools})
                </button>

                {findingsCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setStatusFilter('findings')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                      statusFilter === 'findings'
                        ? 'bg-amber-950 text-amber-300 font-semibold border border-amber-700/60'
                        : 'bg-slate-800/80 text-amber-400/80 border border-slate-700 hover:text-amber-300'
                    }`}
                  >
                    Findings ({findingsCount})
                  </button>
                )}

                {activeRunningCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setStatusFilter('running')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                      statusFilter === 'running'
                        ? 'bg-blue-950 text-blue-300 font-semibold border border-blue-700/60'
                        : 'bg-slate-800/80 text-blue-400/80 border border-slate-700 hover:text-blue-300'
                    }`}
                  >
                    Running ({activeRunningCount})
                  </button>
                )}

                {queuedCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setStatusFilter('queued')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                      statusFilter === 'queued'
                        ? 'bg-slate-700 text-slate-200 font-semibold border border-slate-500'
                        : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                    }`}
                  >
                    Queued ({queuedCount})
                  </button>
                )}

                {cleanCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setStatusFilter('clean')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                      statusFilter === 'clean'
                        ? 'bg-slate-700 text-slate-200 font-semibold border border-slate-600'
                        : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                    }`}
                  >
                    Clean ({cleanCount})
                  </button>
                )}

                {failedCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setStatusFilter('failed')}
                    className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                      statusFilter === 'failed'
                        ? 'bg-red-950 text-red-300 font-semibold border border-red-700/60'
                        : 'bg-slate-800/80 text-red-400/80 border border-slate-700 hover:text-red-300'
                    }`}
                  >
                    Failed ({failedCount})
                  </button>
                )}
              </div>

              {/* Subtle Vertical Separator */}
              <span className="text-slate-700 hidden sm:inline">•</span>

              {/* Sort Pills */}
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-slate-400 font-medium text-[11px] shrink-0 mr-1">Sort:</span>
                <button
                  type="button"
                  onClick={() => setSortBy('status')}
                  className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                    sortBy === 'status'
                      ? 'bg-slate-700 text-white font-semibold border border-slate-500'
                      : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                  }`}
                  title="Sort by Status Priority"
                >
                  Status
                </button>
                <button
                  type="button"
                  onClick={() => setSortBy('name')}
                  className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                    sortBy === 'name'
                      ? 'bg-slate-700 text-white font-semibold border border-slate-500'
                      : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                  }`}
                  title="Sort by Name A–Z"
                >
                  Name
                </button>
                <button
                  type="button"
                  onClick={() => setSortBy('credits')}
                  className={`px-2.5 py-1 rounded-md text-[11px] font-medium transition-colors ${
                    sortBy === 'credits'
                      ? 'bg-slate-700 text-white font-semibold border border-slate-500'
                      : 'bg-slate-800/80 text-slate-400 border border-slate-700 hover:text-slate-200'
                  }`}
                  title="Sort by Credits High–Low"
                >
                  Credits
                </button>
              </div>
            </div>

            {/* Dynamic Tool Grid or Empty State */}
            {(() => {
              const handleToolClick = (item: ToolBreakdownItem) => {
                if (item.commentsGenerated > 0 || item.status === 'failed') {
                  const el = document.getElementById('review-events-section');
                  if (el) {
                    el.scrollIntoView({ behavior: 'smooth' });
                  }
                }
              };

              if (sortedTools.length === 0) {
                return (
                  <div className="py-6 px-4 text-center rounded-lg bg-slate-800/40 border border-slate-700/50 my-1">
                    <p className="text-slate-300 text-xs font-medium">
                      No issues found from static analysis tools matching filter "{statusFilter}".
                    </p>
                    <button
                      type="button"
                      onClick={() => setStatusFilter('all')}
                      className="mt-2 text-indigo-400 hover:text-indigo-300 text-xs font-semibold underline cursor-pointer"
                    >
                      Show all tools ({totalTools})
                    </button>
                  </div>
                );
              }

              return (
                <motion.div layout className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2.5">
                  {sortedTools.map((item) => {
                    const isProblem = item.commentsGenerated > 0 || item.status === 'failed';
                    const isClean = item.commentsGenerated === 0 && item.status !== 'failed' && item.status !== 'running' && item.status !== 'pending';
                    const isToolRunning = item.status === 'running';
                    const isToolPending = item.status === 'pending';

                    return (
                      <motion.div
                        layout
                        key={item.toolName}
                        initial={{ opacity: 0, scale: 0.95 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: 0.95 }}
                        transition={{ duration: 0.15 }}
                        onClick={() => handleToolClick(item)}
                        title={isProblem ? `Click to view ${item.toolName} findings` : undefined}
                        className={`rounded-lg p-3 flex items-center justify-between transition-all ${
                          isProblem
                            ? 'bg-slate-800/90 border border-amber-600/60 shadow-md shadow-amber-950/20 cursor-pointer hover:border-amber-500 hover:scale-[1.01]'
                            : 'bg-slate-800/60 hover:bg-slate-800 border border-slate-700/50'
                        }`}
                      >
                        <div>
                          <p className={`font-mono font-bold text-xs ${isProblem ? 'text-amber-200' : 'text-white'}`}>{item.toolName}</p>
                          <p className="text-slate-400 font-mono text-[11px] mt-0.5">{item.creditsUsed.toFixed(1)} cr</p>
                        </div>

                        {/* Per-Tool State Badge */}
                        {isToolRunning ? (
                          <span className="inline-flex items-center gap-1.5 text-xs font-mono px-2.5 py-1 rounded-md bg-blue-950/50 text-blue-300 border border-blue-700/60 animate-pulse">
                            <svg className="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
                              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                            </svg>
                            Running
                          </span>
                        ) : isToolPending ? (
                          <span className="text-xs font-mono px-2.5 py-1 rounded-md bg-slate-800 text-slate-400 border border-slate-700">
                            Queued
                          </span>
                        ) : (
                          <span
                            className={`text-xs font-mono px-2.5 py-1 rounded-md border text-center font-semibold flex items-center gap-1 ${
                              item.status === 'failed'
                                ? 'bg-red-950 text-red-300 border-red-800/80'
                                : isClean
                                ? 'bg-slate-800/80 text-slate-400 border-slate-700 font-normal'
                                : 'bg-amber-950 text-amber-300 border-amber-700/80 shadow-sm hover:bg-amber-900'
                            }`}
                          >
                            {isClean ? 'Clean' : `⚠️ ${item.commentsGenerated} findings`}
                          </span>
                        )}
                      </motion.div>
                    );
                  })}
                </motion.div>
              );
            })()}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
};






