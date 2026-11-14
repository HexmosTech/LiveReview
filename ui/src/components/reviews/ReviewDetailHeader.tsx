import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { Review, ReviewCommit, ReviewEvent, ReviewSummary } from '../../types/reviews';
import { ToolAnalysisCard, ToolAccountingData } from './ToolAnalysisCard';
import { reviewStatusBadge, SourceIcon } from '../../utils/reviewDisplay';
import { getStatusText } from '../../api/reviews';
import { RelativeTime } from '../UIPrimitives';
import { RiskAssessment, RiskAssessmentDetails } from './RiskAssessment';
import {
    COMMITS_PREVIEW_LIMIT,
    formatDuration,
    isSafeExternalUrl,
} from '../../pages/Reviews/reviewDetailHelpers';

export interface ReviewDetailHeaderProps {
    review: Review;
    summary: ReviewSummary | null;
    events: ReviewEvent[];
    eventSeverityCounts: { high: number; medium: number; low: number };
    risk: RiskAssessment;
    policyApproved: boolean | null;
    commits: ReviewCommit[];
    commitsLoaded: boolean;
    githubBaseUrl: string | null;
    showToolsDropdown: boolean;
    effectiveToolAccounting: ToolAccountingData | null;
    onViewAllEvents: () => void;
}

export const ReviewDetailHeader: React.FC<ReviewDetailHeaderProps> = ({
    review,
    summary,
    events,
    eventSeverityCounts,
    risk,
    policyApproved,
    commits,
    commitsLoaded,
    githubBaseUrl,
    showToolsDropdown,
    effectiveToolAccounting,
    onViewAllEvents,
}) => {
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [toolExpanded, setToolExpanded] = useState(false);
    const [allCommitsShown, setAllCommitsShown] = useState(false);

    const statusBadge = reviewStatusBadge(review.status);

    return (
        <div className="mb-4 bg-slate-800/80 rounded-xl border border-slate-700/70 overflow-hidden">
            <div className="flex items-stretch min-h-[52px]">
                {/* Back button */}
                <Link
                    to="/reviews"
                    className="shrink-0 self-stretch flex items-center px-3.5 text-xs font-medium text-slate-300 hover:text-white hover:bg-slate-700/40 transition-colors no-underline"
                    title="Back to Reviews"
                >
                    ← Back
                </Link>

                {/* Title, Branch & Provider Logo */}
                <div
                    className={`shrink-0 flex items-center gap-2.5 px-3.5 self-stretch transition-colors ${isSafeExternalUrl(review.prMrUrl) ? 'cursor-pointer hover:bg-slate-700/40' : ''}`}
                    onClick={() => {
                        if (isSafeExternalUrl(review.prMrUrl)) {
                            window.open(review.prMrUrl, '_blank', 'noopener,noreferrer');
                        }
                    }}
                    role={isSafeExternalUrl(review.prMrUrl) ? 'link' : undefined}
                    tabIndex={isSafeExternalUrl(review.prMrUrl) ? 0 : undefined}
                    onKeyDown={(e) => {
                        if (!isSafeExternalUrl(review.prMrUrl)) return;
                        if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            window.open(review.prMrUrl, '_blank', 'noopener,noreferrer');
                        }
                    }}
                    title={isSafeExternalUrl(review.prMrUrl) ? `Open PR/MR: ${review.prMrUrl}` : review.repository}
                >
                    <div className="shrink-0 min-w-0 max-w-[240px]">
                        <h1 className="text-base font-bold text-white leading-5 truncate">
                            {review.repository.split('/').pop() || review.repository}
                        </h1>
                        {review.branch && (
                            <p className="text-[11px] text-slate-400 font-mono leading-4 truncate">
                                {review.branch}
                            </p>
                        )}
                    </div>

                    <div className="shrink-0">
                        <div className="inline-flex items-center justify-center p-1.5 rounded-md border border-slate-700/80 bg-slate-900/60 text-slate-200 leading-none">
                            <SourceIcon provider={review.provider} triggerType={review.triggerType} />
                        </div>
                    </div>
                </div>

                {/* Details toggle */}
                <div
                    onClick={() => { setDetailsExpanded(v => !v); setToolExpanded(false); }}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            setDetailsExpanded(v => !v);
                            setToolExpanded(false);
                        }
                    }}
                    role="button"
                    tabIndex={0}
                    aria-expanded={detailsExpanded}
                    aria-label="Toggle review details"
                    className="shrink-0 flex items-center gap-2.5 px-3.5 self-stretch cursor-pointer group select-none hover:bg-slate-700/30 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-500/60"
                    title="Toggle review details"
                >
                    <p className="text-xs text-slate-300 whitespace-nowrap">
                        <span className="font-semibold text-white">{review.userEmail?.split('@')[0] || 'Someone'}</span>
                        {' '}created{' '}
                        <span className="text-slate-400 inline-block min-w-[6rem]">
                            <RelativeTime timestamp={review.createdAt} />
                        </span>
                    </p>

                    <span className="text-slate-600 select-none">|</span>

                    <div className="shrink-0 flex items-center gap-2 text-xs text-slate-400">
                        <span><strong className="font-medium text-slate-200">{eventSeverityCounts.high}</strong> High</span>
                        <span className="text-slate-700">·</span>
                        <span><strong className="font-medium text-slate-200">{eventSeverityCounts.medium}</strong> Med</span>
                        <span className="text-slate-700">·</span>
                        <span><strong className="font-medium text-slate-200">{eventSeverityCounts.low}</strong> Low</span>
                    </div>

                    <span className="text-slate-600 select-none">|</span>

                    <div className="shrink-0 flex flex-col justify-center leading-tight">
                        <span className="text-[11px] text-slate-400 whitespace-nowrap">Risk assessment</span>
                        <span className={`text-xs font-medium whitespace-nowrap ${risk.tone}`}>{risk.value}</span>
                    </div>

                    {(review.metadata?.pr_link_policy || review.prMrUrl) && (
                        <div className="shrink-0 flex flex-col justify-center leading-tight">
                            <span className="text-[11px] text-slate-400 whitespace-nowrap">Auto-approve</span>
                            <span className={`text-xs font-medium whitespace-nowrap ${!review.metadata?.pr_link_policy ? 'text-red-400' : policyApproved === null ? 'text-slate-400' : policyApproved ? 'text-green-400' : 'text-red-400'}`}>
                                {!review.metadata?.pr_link_policy ? 'Not evaluated' : policyApproved === null ? 'Pending' : policyApproved ? 'Approved' : 'Not approved'}
                            </span>
                        </div>
                    )}

                    <svg
                        className={`w-3.5 h-3.5 text-slate-400 group-hover:text-slate-200 transition-all duration-200 ${detailsExpanded ? 'rotate-180' : 'rotate-0'}`}
                        fill="none" stroke="currentColor" viewBox="0 0 24 24"
                    >
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                    </svg>
                </div>

                {/* Static Analysis tools summary toggle */}
                {showToolsDropdown && (
                    <div
                        onClick={() => { setToolExpanded(v => !v); setDetailsExpanded(false); }}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault();
                                setToolExpanded(v => !v);
                                setDetailsExpanded(false);
                            }
                        }}
                        role="button"
                        tabIndex={0}
                        aria-expanded={toolExpanded}
                        aria-label="Toggle static analysis tools"
                        className="ml-auto shrink-0 flex items-center gap-2.5 px-3.5 self-stretch cursor-pointer group select-none hover:bg-slate-700/30 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-blue-500/60"
                        title="Toggle static analysis tools"
                    >
                        <p className="text-xs text-slate-300 whitespace-nowrap">
                            <strong className="font-semibold text-slate-100">{effectiveToolAccounting?.toolsExecuted ?? effectiveToolAccounting?.toolBreakdown?.length ?? 0}</strong> Tools
                            {effectiveToolAccounting?.toolBreakdown?.some(t => t.status === 'pending' || t.status === 'running') ? (
                                <span className="text-blue-400 ml-1 animate-pulse">· running</span>
                            ) : (effectiveToolAccounting?.totalCommentsGenerated ?? 0) > 0 ? (
                                <span className="text-slate-500 ml-1">· {effectiveToolAccounting?.totalCommentsGenerated} findings</span>
                            ) : (
                                <span className="text-slate-500 ml-1">· clean</span>
                            )}
                        </p>

                        <svg
                            className={`w-3.5 h-3.5 text-slate-400 group-hover:text-slate-200 transition-all duration-200 ${toolExpanded ? 'rotate-180' : 'rotate-0'}`}
                            fill="none" stroke="currentColor" viewBox="0 0 24 24"
                        >
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                        </svg>
                    </div>
                )}

                {/* Status badge */}
                <div className={`shrink-0 flex items-center px-3.5 self-stretch ${!showToolsDropdown ? 'ml-auto' : ''}`}>
                    {(review.status as string) === 'partial' ? (
                        <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-semibold leading-5 whitespace-nowrap bg-amber-950/40 text-amber-400 border border-amber-600/50">
                            Partial
                        </span>
                    ) : (
                        <span
                            className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-semibold leading-5 whitespace-nowrap"
                            style={{ color: statusBadge.color, border: `1px solid ${statusBadge.borderColor}` }}
                        >
                            {review.status === 'in_progress' && (
                                <svg className="animate-spin w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"/></svg>
                            )}
                            {getStatusText(review.status)}
                        </span>
                    )}
                </div>
            </div>

            {/* Details drawer */}
            {detailsExpanded && (
                <div className="border-t border-slate-700/50 bg-slate-900/30">
                    <div className="p-4 grid grid-cols-1 md:grid-cols-12 gap-4">
                        {/* Commits */}
                        <div className="md:col-span-4 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5 flex flex-col h-[135px]">
                            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest mb-1.5 shrink-0">
                                Commits{commitsLoaded && ` (${commits.length})`}
                            </p>
                            {!commitsLoaded && (
                                <p className="text-xs text-slate-400">Checking for commit information...</p>
                            )}
                            {commitsLoaded && commits.length === 0 && (
                                <p className="text-xs text-slate-400 leading-snug">
                                    No commit information recorded for this review yet. Plain "lrc review" (staged/working) commits sync in the background after `git commit` and may take a moment to appear; PR/MR and --commit/--range reviews are recorded immediately once submitted.
                                </p>
                            )}
                            {commits.length > 0 && (
                                <>
                                    <ul className={`space-y-1.5 flex-1 min-h-0 ${allCommitsShown && commits.length > COMMITS_PREVIEW_LIMIT ? 'overflow-y-auto pr-1' : 'overflow-hidden'}`}>
                                        {(allCommitsShown ? commits : commits.slice(0, COMMITS_PREVIEW_LIMIT)).map((commit) => (
                                            <li key={commit.ref} className="flex items-center gap-2 text-xs">
                                                <div className="flex items-center gap-1.5 min-w-0">
                                                    {commit.refType === 'commit' && githubBaseUrl ? (
                                                        <a
                                                            href={`${githubBaseUrl}/commit/${encodeURIComponent(commit.ref)}`}
                                                            target="_blank"
                                                            rel="noopener noreferrer"
                                                            className="font-mono text-blue-400 hover:text-blue-300 hover:underline shrink-0"
                                                        >
                                                            {commit.ref.substring(0, 8)}
                                                        </a>
                                                    ) : (
                                                        <span className="font-mono text-slate-300 shrink-0">
                                                            {commit.refType === 'commit' ? commit.ref.substring(0, 8) : commit.ref}
                                                        </span>
                                                    )}
                                                    {commit.refType === 'range' && (
                                                        <span className="shrink-0 text-[10px] uppercase tracking-wide rounded-full px-2 py-0.5 border bg-purple-900/30 text-purple-300 border-purple-700">
                                                            range
                                                        </span>
                                                    )}
                                                </div>
                                                <RelativeTime timestamp={commit.createdAt} className="text-slate-600 text-[10px] shrink-0 ml-auto" />
                                            </li>
                                        ))}
                                    </ul>
                                    {commits.length > COMMITS_PREVIEW_LIMIT && (
                                        <button
                                            type="button"
                                            onClick={() => setAllCommitsShown((prev) => !prev)}
                                            className="mt-1 text-[11px] text-blue-400 hover:text-blue-300 hover:underline text-left shrink-0 font-medium"
                                        >
                                            {allCommitsShown ? 'Show less' : `+${commits.length - COMMITS_PREVIEW_LIMIT} more`}
                                        </button>
                                    )}
                                </>
                            )}
                        </div>

                        {/* Issues by Severity */}
                        <div className="md:col-span-4 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5 flex flex-col justify-between h-[135px]">
                            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest mb-2 shrink-0">Issues by severity</p>
                            <div className="flex items-end gap-6 my-auto">
                                <div>
                                    <span className="text-2xl font-bold text-red-400 leading-none">{eventSeverityCounts.high}</span>
                                    <p className="text-[10px] text-slate-500 mt-1 uppercase tracking-wide font-medium">High</p>
                                </div>
                                <div>
                                    <span className="text-2xl font-bold text-amber-400 leading-none">{eventSeverityCounts.medium}</span>
                                    <p className="text-[10px] text-slate-500 mt-1 uppercase tracking-wide font-medium">Medium</p>
                                </div>
                                <div>
                                    <span className="text-2xl font-bold text-sky-400 leading-none">{eventSeverityCounts.low}</span>
                                    <p className="text-[10px] text-slate-500 mt-1 uppercase tracking-wide font-medium">Low</p>
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={onViewAllEvents}
                                className="text-[11px] text-blue-400 hover:text-blue-300 text-left shrink-0"
                            >
                                View all events →
                            </button>
                        </div>

                        {/* Details & Progress */}
                        <div className="md:col-span-4 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5 flex flex-col justify-between h-[135px]">
                            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest mb-1 shrink-0">Details & Progress</p>
                            <div className="grid grid-cols-4 gap-2 text-xs my-auto">
                                <div>
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Duration</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 truncate">{formatDuration(review.startedAt, review.completedAt) || '—'}</p>
                                </div>
                                <div>
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Batches</p>
                                    <p className="font-semibold text-slate-200 mt-0.5">{summary?.batchCount ?? '—'}</p>
                                </div>
                                <div>
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Events</p>
                                    <p className="font-semibold text-blue-400 mt-0.5">{events.length}</p>
                                </div>
                                <div>
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Activity</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 truncate">{summary?.lastActivity ? <RelativeTime timestamp={summary.lastActivity} /> : '—'}</p>
                                </div>
                            </div>
                            <div className="grid grid-cols-2 gap-3 pt-2 border-t border-slate-700/40 text-xs shrink-0">
                                <div className="min-w-0">
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Created by</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 break-all text-[11px] leading-tight">{review.userEmail || '—'}</p>
                                </div>
                                <div className="min-w-0">
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Created at</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 text-[11px] leading-tight">
                                        {(() => {
                                            try {
                                                const d = new Date(review.createdAt);
                                                return isNaN(d.getTime()) ? review.createdAt || '—' : d.toLocaleString([], {
                                                    year: 'numeric', month: 'short', day: 'numeric',
                                                    hour: '2-digit', minute: '2-digit'
                                                });
                                            } catch {
                                                return review.createdAt || '—';
                                            }
                                        })()}
                                    </p>
                                </div>
                            </div>
                        </div>

                        {/* Risk assessment + policy status */}
                        <div className="md:col-span-12 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5">
                            <div className="flex items-center justify-between mb-1.5">
                                <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest">Risk assessment</p>
                                <div className="flex items-center gap-3 text-xs">
                                    <span className={`font-semibold ${risk.tone}`}>{risk.value}</span>
                                    {review.metadata?.pr_link_policy && (
                                        <span className="flex items-center gap-1.5">
                                            <span className="text-[10px] text-slate-500 uppercase tracking-wide">Auto-approve</span>
                                            <span className={`font-semibold ${policyApproved === null ? 'text-slate-400' : policyApproved ? 'text-green-400' : 'text-red-400'}`}>
                                                {policyApproved === null ? 'Pending' : policyApproved ? 'Approved' : 'Not approved'}
                                            </span>
                                        </span>
                                    )}
                                </div>
                            </div>
                            <div className="text-xs">
                                <RiskAssessmentDetails risk={risk} connectorId={review.connectorId} />
                            </div>
                        </div>
                    </div>
                </div>
            )}

            {/* Static Analysis Tools drawer */}
            {showToolsDropdown && toolExpanded && effectiveToolAccounting && (effectiveToolAccounting.toolBreakdown?.length ?? 0) > 0 && (
                <div className="border-t border-slate-700/50 bg-slate-900/40">
                    <div className="p-4">
                        <ToolAnalysisCard data={effectiveToolAccounting} embedded={true} isExpanded={true} hideSummary={true} />
                    </div>
                </div>
            )}
        </div>
    );
};
