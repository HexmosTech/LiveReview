import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { Button, Icons, Tabs } from '../../components/UIPrimitives';
import { ReviewEventsPage, DiffViewerPanel } from '../../components/reviews';
import {
  getReview,
  getReviewEvents,
  getReviewSummary,
    getReviewAccounting,
    getReviewCommits,
  formatRelativeTime,
  getStatusColor,
  getStatusText
} from '../../api/reviews';
import {
  Review,
  ReviewEvent,
  ReviewSummary,
    ReviewAccounting,
    ReviewAccountingStage,
    ReviewCommit,
  ReviewEventLevel,
  ReviewEventType
} from '../../types/reviews';

const ACCOUNTING_REFRESH_INTERVAL_MS = 15000;
const COMMITS_PREVIEW_LIMIT = 5;

const HeaderStat: React.FC<{ label: string; value: string; className?: string }> = ({ label, value, className }) => (
    <div className="shrink-0">
        <p className="text-[11px] text-slate-400 whitespace-nowrap">{label}</p>
        <p className={`text-sm text-white whitespace-nowrap ${className || ''}`}>{value}</p>
    </div>
);

const hasAccountingDetails = (value: ReviewAccounting | null): boolean => {
        if (!value) {
                return false;
        }

        return value.accountedOperations > 0 ||
                value.totalBillableLoc > 0 ||
                value.tokenTrackedOperations > 0 ||
            !!value.stageBreakdown?.length ||
                !!value.latestOperation;
};

const ReviewDetail: React.FC = () => {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const reviewId = parseInt(id || '0', 10);

    // Helper functions to map event data to new format
    const mapEventType = (type: ReviewEventType) => type;

    const mapEventLevel = (level: ReviewEventLevel) => level;
    const [review, setReview] = useState<Review | null>(null);
    const [events, setEvents] = useState<ReviewEvent[]>([]);
    const [summary, setSummary] = useState<ReviewSummary | null>(null);
    const [accounting, setAccounting] = useState<ReviewAccounting | null>(null);
    const [accountingError, setAccountingError] = useState<string | null>(null);
    const [accountingErrorTone, setAccountingErrorTone] = useState<'info' | 'warning'>('info');
    const [accountingRouteUnavailable, setAccountingRouteUnavailable] = useState(false);
    const [commits, setCommits] = useState<ReviewCommit[]>([]);
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [commitsLoaded, setCommitsLoaded] = useState(false);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [pollingEnabled, setPollingEnabled] = useState(true);
    const [levelFilter, setLevelFilter] = useState<ReviewEventLevel | ''>('');
    const [typeFilter, setTypeFilter] = useState<ReviewEventType | ''>('');
    const [lastEventTime, setLastEventTime] = useState<string | null>(null);
    const [activeTab, setActiveTab] = useState<'findings' | 'accounting' | 'events'>('findings');
    const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);

    // Status colors are imported via getStatusColor from ../../api/reviews

    const getEventIcon = (type: string, level?: string) => {
        switch (type) {
            case 'status': 
                return <div className="text-blue-400"><Icons.Info /></div>;
            case 'log': 
                if (level === 'error') return <div className="text-red-400"><Icons.Error /></div>;
                if (level === 'warn') return <div className="text-yellow-400"><Icons.Warning /></div>;
                return <div className="text-slate-400"><Icons.Info /></div>;
            case 'batch': 
                return <div className="text-purple-400"><Icons.Settings /></div>;
            case 'artifact': 
                return <div className="text-green-400"><Icons.Success /></div>;
            case 'completion': 
                return <div className="text-green-400"><Icons.Success /></div>;
            default: 
                return <div className="text-slate-400"><Icons.Info /></div>;
        }
    };

    // Format event data for display
    const formatEventData = (event: ReviewEvent) => {
        const data = event.data;
        
        switch (event.type) {
            case 'status':
                return data.status ? `Status: ${data.status}` : 'Status changed';
            case 'log':
                return data.message || 'Log entry';
            case 'batch':
                return event.batchId ? `Batch: ${event.batchId}` : `Batch processing`;
            case 'artifact':
                return data.url ? `Generated: ${data.kind || 'Artifact'}` : `Artifact: ${data.kind || 'Unknown'}`;
            case 'completion':
                return data.resultSummary ? `Completed: ${data.resultSummary}` : 'Process completed';
            default:
                return JSON.stringify(data, null, 2);
        }
    };

    const fetchAccountingDetails = useCallback(async (currentReviewId: number, reviewStatus?: Review['status']) => {
        try {
            const accountingData = await getReviewAccounting(currentReviewId);
            setAccounting(accountingData);
            setAccountingRouteUnavailable(false);

            if (hasAccountingDetails(accountingData)) {
                setAccountingError(null);
            } else {
                setAccountingErrorTone('info');
                setAccountingError('Accounting details are being prepared. This panel auto-refreshes every 15 seconds and updates when data becomes available.');
            }
        } catch (accountingErr) {
            console.warn('Accounting endpoint unavailable:', accountingErr);
            setAccounting(null);

            const status = (accountingErr as any)?.status;
            if (status === 404) {
                setAccountingRouteUnavailable(true);
                setAccountingErrorTone('warning');
                setAccountingError('Accounting details are unavailable on this server route.');
                return;
            }

            setAccountingRouteUnavailable(false);
            setAccountingErrorTone('info');
            if (reviewStatus === 'created' || reviewStatus === 'in_progress') {
                setAccountingError('Accounting details are not ready yet. This panel retries every 15 seconds and will update automatically.');
            } else {
                setAccountingError('Accounting details are temporarily unavailable. This panel retries every 15 seconds.');
            }
        }
    }, []);

    // Fetch review details
    const fetchReviewDetails = useCallback(async () => {
        if (!id) return;
        try {
            setLoading(true);
            setError(null);
            setAccountingError(null);
            setAccountingRouteUnavailable(false);
            
            const reviewId = parseInt(id, 10);
            if (isNaN(reviewId)) {
                throw new Error('Invalid review ID');
            }
            
            // Keep core review progress load independent from accounting availability.
            const [reviewData, eventsData, summaryData] = await Promise.all([
                getReview(reviewId),
                getReviewEvents(reviewId, undefined, 1000), // Get all events
                getReviewSummary(reviewId),
            ]);

            setReview(reviewData);
            setSummary(summaryData);
            await fetchAccountingDetails(reviewId, reviewData.status);

            // Relevant commits are best-effort/informational -- never block
            // or fail the rest of the page on this.
            try {
                const commitsData = await getReviewCommits(reviewId);
                setCommits(commitsData.commits || []);
            } catch (commitsErr) {
                console.warn('Review commits endpoint unavailable:', commitsErr);
                setCommits([]);
            } finally {
                setCommitsLoaded(true);
            }
            
            const newEvents = (eventsData?.events as ReviewEvent[] | undefined) || [];
            setEvents(newEvents);
            
            // Update last event time for next polling
            if (newEvents.length > 0) {
                const latestTime = newEvents[newEvents.length - 1].time;
                setLastEventTime(latestTime);
            }

        } catch (err) {
            console.error('Error fetching review details:', err);
            setError(err instanceof Error ? err.message : 'Failed to fetch review details');
        } finally {
            setLoading(false);
        }
    }, [id, fetchAccountingDetails]);



    // Reset event cursor and list when navigating to a different review
    useEffect(() => {
        setEvents([]);
        setLastEventTime(null);
        setCommits([]);
        setDetailsExpanded(false);
        setCommitsLoaded(false);
    }, [id]);

    // Derive available filter values from current events
    const presentTypes = useMemo(() => {
        const s = new Set<string>();
        events.forEach(e => s.add(e.type));
        return s;
    }, [events]);

    const presentLevels = useMemo(() => {
        const s = new Set<string>();
        events.forEach(e => { if (e.level) s.add(e.level); });
        return s;
    }, [events]);

    // Events by severity, derived from the already-loaded events list --
    // error=High, warn=Medium, info/debug=Low.
    const eventSeverityCounts = useMemo(() => {
        let high = 0, medium = 0, low = 0;
        events.forEach(e => {
            if (e.level === 'error') high++;
            else if (e.level === 'warn') medium++;
            else low++;
        });
        return { high, medium, low };
    }, [events]);

    // Initial load
    useEffect(() => {
        fetchReviewDetails();
    }, [fetchReviewDetails]);

    // Poll accounting so the panel auto-updates once usage records land.
    useEffect(() => {
        if (!id || !pollingEnabled || accountingRouteUnavailable) {
            if (pollingIntervalRef.current) {
                clearInterval(pollingIntervalRef.current);
                pollingIntervalRef.current = null;
            }
            return;
        }

        const currentReviewId = parseInt(id, 10);
        if (isNaN(currentReviewId)) {
            return;
        }

        const shouldPollAccounting =
            review?.status === 'created' ||
            review?.status === 'in_progress' ||
            !hasAccountingDetails(accounting);

        if (!shouldPollAccounting) {
            if (pollingIntervalRef.current) {
                clearInterval(pollingIntervalRef.current);
                pollingIntervalRef.current = null;
            }
            return;
        }

        if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current);
        }

        pollingIntervalRef.current = setInterval(() => {
            void fetchAccountingDetails(currentReviewId, review?.status);
        }, ACCOUNTING_REFRESH_INTERVAL_MS);

        return () => {
            if (pollingIntervalRef.current) {
                clearInterval(pollingIntervalRef.current);
                pollingIntervalRef.current = null;
            }
        };
    }, [id, pollingEnabled, accountingRouteUnavailable, review?.status, accounting, fetchAccountingDetails]);
    
    if (loading) {
        return (
            <div className="container mx-auto px-4 py-8">
                <div className="flex items-center justify-center min-h-64">
                    <div className="text-center">
                        <svg className="w-8 h-8 mx-auto mb-4 text-blue-500 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                        </svg>
                        <p className="text-slate-300">Loading review details...</p>
                    </div>
                </div>
            </div>
        );
    }

    if (error || !review) {
        return (
            <div className="container mx-auto px-4 py-8">
                <div className="text-center">
                    <Icons.Error />
                    <h3 className="text-xl font-medium text-red-300 mt-4">{error || 'Review not found'}</h3>
                    <div className="mt-6 space-x-4">
                        <Button
                            as={Link}
                            to="/reviews"
                            variant="outline"
                        >
                            Back to Reviews
                        </Button>
                        <Button
                            onClick={fetchReviewDetails}
                            variant="primary"
                        >
                            Retry
                        </Button>
                    </div>
                </div>
            </div>
        );
    }

    const formatInt = (value?: number): string => {
        if (value === undefined || value === null) {
            return 'Not tracked yet';
        }
        return value.toLocaleString();
    };

    const formatCurrency = (value?: number): string => {
        if (value === undefined || value === null) {
            return 'Not tracked yet';
        }
        return `$${value.toFixed(4)}`;
    };

    const formatDuration = (startedAt?: string, completedAt?: string): string | null => {
        if (!startedAt || !completedAt) {
            return null;
        }
        const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime();
        if (!isFinite(ms) || ms < 0) {
            return null;
        }
        const totalSeconds = Math.round(ms / 1000);
        const minutes = Math.floor(totalSeconds / 60);
        const seconds = totalSeconds % 60;
        return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
    };

    const leaderAIExecutionMode = typeof review.metadata?.leader_ai_execution_mode === 'string' ? review.metadata.leader_ai_execution_mode : '';
    const leaderAIExecutionSource = typeof review.metadata?.leader_ai_execution_source === 'string' ? review.metadata.leader_ai_execution_source : '';
    const leaderAIExecutionProvider = typeof review.metadata?.leader_ai_provider_name === 'string' ? review.metadata.leader_ai_provider_name : '';
    const leaderAIExecutionConnector = typeof review.metadata?.leader_ai_connector_name === 'string' ? review.metadata.leader_ai_connector_name : '';
    const helperAIExecutionMode = typeof review.metadata?.helper_ai_execution_mode === 'string' ? review.metadata.helper_ai_execution_mode : '';
    const helperAIExecutionSource = typeof review.metadata?.helper_ai_execution_source === 'string' ? review.metadata.helper_ai_execution_source : '';
    const helperAIExecutionProvider = typeof review.metadata?.helper_ai_provider_name === 'string' ? review.metadata.helper_ai_provider_name : '';
    const helperAIExecutionConnector = typeof review.metadata?.helper_ai_connector_name === 'string' ? review.metadata.helper_ai_connector_name : '';
    const helperEnabled = !!accounting?.helperEnabled;
    const helperMode = accounting?.helperMode || '';
    const stageBreakdown = accounting?.stageBreakdown || [];

    const formatStageLabel = (stage: string): string => {
        switch (stage) {
            case 'leader':
                return 'Leader model';
            case 'helper':
                return 'Helper model';
            default:
                return stage.charAt(0).toUpperCase() + stage.slice(1);
        }
    };

    const getStageRouteText = (stage: ReviewAccountingStage): string => {
        if (stage.stage === 'helper') {
            return [helperAIExecutionProvider || stage.provider, helperAIExecutionConnector]
                .filter(Boolean)
                .join(' / ');
        }
        return [leaderAIExecutionProvider || stage.provider, leaderAIExecutionConnector]
            .filter(Boolean)
            .join(' / ');
    };

    const getStageExecutionText = (stage: ReviewAccountingStage): string => {
        if (stage.stage === 'helper') {
            return [helperAIExecutionMode, helperAIExecutionSource].filter(Boolean).join(' via ');
        }
        return [leaderAIExecutionMode, leaderAIExecutionSource].filter(Boolean).join(' via ');
    };

    const accountingBannerClass = accountingErrorTone === 'warning'
        ? 'mb-4 rounded-md border border-amber-700 bg-amber-900/30 p-3 text-xs text-amber-200'
        : 'mb-4 rounded-md border border-sky-700 bg-sky-900/30 p-3 text-xs text-sky-200';

    return (
        <div className="container mx-auto px-4 py-8">
            {/* Header */}
            <div className="flex items-center justify-between mb-8">
                <div className="flex items-center">
                    <Button
                        as={Link}
                        to="/reviews"
                        variant="ghost"
                        className="mr-4"
                    >
                        ← Back
                    </Button>
                    <div>
                        <h1 className="text-3xl font-bold text-white">
                            {review.repository.split('/').pop() || review.repository}
                        </h1>
                        <p className="text-slate-300">
                            {review.branch && `${review.branch}`}
                            {review.prMrUrl && (
                                <span className="ml-2">
                                    <a 
                                        href={review.prMrUrl} 
                                        target="_blank" 
                                        rel="noopener noreferrer"
                                        className="text-blue-400 hover:text-blue-300"
                                    >
                                        View PR/MR
                                    </a>
                                </span>
                            )}
                        </p>
                    </div>
                </div>
                <div className="flex items-center space-x-4">
                    <span className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium text-white ${getStatusColor(review.status)}`}>
                        {review.status.replace('_', ' ').toUpperCase()}
                    </span>
                    {/* Polling control moved to ReviewEventsPage for consistency */}
                </div>
            </div>

            {/* Review Info: compact one-row header, expands inline for
                commits + review details rather than spreading everything
                across the page by default. */}
            <div className="bg-slate-800 rounded-lg border border-slate-700 mb-6">
                <button
                    type="button"
                    onClick={() => setDetailsExpanded((v) => !v)}
                    className="w-full flex items-center gap-6 px-4 py-3 text-left overflow-x-auto"
                    aria-expanded={detailsExpanded}
                >
                    <div className="flex items-center gap-2 shrink-0">
                        <div className="text-slate-400"><Icons.Git /></div>
                        <div>
                            <p className="text-sm font-semibold text-white max-w-[180px] truncate">
                                {review.repository.split('/').pop() || review.repository}
                            </p>
                            <p className="text-xs text-slate-400">#{review.id}</p>
                        </div>
                    </div>
                    <HeaderStat label="Provider" value={review.provider || '-'} className="capitalize" />
                    <HeaderStat label="Branch" value={review.branch || '-'} />
                    <HeaderStat label="Commits" value={commitsLoaded ? String(commits.length) : '...'} />
                    <HeaderStat label="Last activity" value={formatRelativeTime(review.completedAt || review.startedAt || review.createdAt)} />
                    <HeaderStat label="Events" value={String(Object.values(summary?.eventCounts || {}).reduce((a: number, b: number) => a + b, 0))} />
                    <HeaderStat label="Batches" value={String(summary?.batchCount ?? 0)} />
                    <div className="ml-auto flex items-center gap-3 shrink-0">
                        <span className={`inline-flex items-center px-3 py-1 rounded-full text-xs font-medium text-white ${getStatusColor(review.status)}`}>
                            {review.status.replace('_', ' ').toUpperCase()}
                        </span>
                        <span className="flex items-center gap-1 text-xs text-slate-300">
                            {detailsExpanded ? 'Less' : 'More'}
                            {detailsExpanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}
                        </span>
                    </div>
                </button>

                {detailsExpanded && (
                    <div className="border-t border-slate-700 px-4 py-4 grid grid-cols-1 md:grid-cols-2 gap-6 text-sm">
                        <div>
                            <h3 className="text-xs font-semibold text-white uppercase tracking-wide mb-2">
                                Commits{commitsLoaded && ` (${commits.length})`}
                            </h3>
                            {!commitsLoaded && (
                                <p className="text-xs text-slate-400">Checking for commit information...</p>
                            )}
                            {commitsLoaded && commits.length === 0 && (
                                <p className="text-xs text-slate-400">
                                    No commit information recorded for this review yet. Plain "lrc review" (staged/working) commits sync in the background after `git commit` and may take a moment to appear; PR/MR and --commit/--range reviews are recorded immediately once submitted.
                                </p>
                            )}
                            {commits.length > 0 && (
                                <>
                                    <ul className="space-y-2">
                                        {commits.slice(0, COMMITS_PREVIEW_LIMIT).map((commit) => (
                                            <li key={commit.ref} className="flex items-center justify-between gap-3 text-xs">
                                                <div className="flex items-center gap-2 min-w-0">
                                                    <span className="font-mono text-slate-200 shrink-0">
                                                        {commit.refType === 'commit' ? commit.ref.substring(0, 8) : commit.ref}
                                                    </span>
                                                    <span
                                                        className={`shrink-0 text-[10px] uppercase tracking-wide rounded-full px-2 py-0.5 border ${
                                                            commit.refType === 'range'
                                                                ? 'bg-purple-900/30 text-purple-300 border-purple-700'
                                                                : 'bg-slate-700 text-slate-300 border-slate-600'
                                                        }`}
                                                    >
                                                        {commit.refType}
                                                    </span>
                                                </div>
                                                <span className="shrink-0 text-slate-400">{formatRelativeTime(commit.createdAt)}</span>
                                            </li>
                                        ))}
                                    </ul>
                                    {commits.length > COMMITS_PREVIEW_LIMIT && (
                                        <p className="mt-2 text-xs text-slate-400">
                                            +{commits.length - COMMITS_PREVIEW_LIMIT} more commits
                                        </p>
                                    )}
                                </>
                            )}
                        </div>

                        <div>
                            <h3 className="text-xs font-semibold text-white uppercase tracking-wide mb-2">Review details</h3>
                            <dl className="space-y-1 text-xs">
                                <div className="flex justify-between gap-4">
                                    <dt className="text-slate-400">Created by</dt>
                                    <dd className="text-white text-right">{review.userEmail || '-'}</dd>
                                </div>
                                <div className="flex justify-between gap-4">
                                    <dt className="text-slate-400">Created</dt>
                                    <dd className="text-white text-right">{new Date(review.createdAt).toLocaleString()}</dd>
                                </div>
                                <div className="flex justify-between gap-4">
                                    <dt className="text-slate-400">Batches</dt>
                                    <dd className="text-white text-right">{summary?.batchCount ?? 0}</dd>
                                </div>
                                {formatDuration(review.startedAt, review.completedAt) && (
                                    <div className="flex justify-between gap-4">
                                        <dt className="text-slate-400">Duration</dt>
                                        <dd className="text-white text-right">{formatDuration(review.startedAt, review.completedAt)}</dd>
                                    </div>
                                )}
                            </dl>
                            <div className="mt-3">
                                <p className="text-xs text-slate-400 mb-1">Events by severity</p>
                                <div className="flex gap-4 text-xs">
                                    <span className="text-red-400">High {eventSeverityCounts.high}</span>
                                    <span className="text-amber-400">Medium {eventSeverityCounts.medium}</span>
                                    <span className="text-sky-400">Low {eventSeverityCounts.low}</span>
                                </div>
                            </div>
                            <button
                                type="button"
                                onClick={() => setActiveTab('events')}
                                className="mt-3 text-xs text-blue-400 hover:text-blue-300"
                            >
                                View all events →
                            </button>
                        </div>
                    </div>
                )}
            </div>

            {/* Findings / Accounting / Events */}
            <Tabs
                className="mb-6"
                activeTab={activeTab}
                onChange={(id) => setActiveTab(id as typeof activeTab)}
                tabs={[
                    { id: 'findings', label: 'Findings' },
                    { id: 'accounting', label: 'Accounting' },
                    { id: 'events', label: 'Events' },
                ]}
            />

            {activeTab === 'findings' && (
                <div className="mb-6">
                    <DiffViewerPanel reviewId={reviewId} />
                </div>
            )}

            {/* Accounting Panel */}
            <div className={`bg-slate-800 rounded-lg p-4 border border-slate-700 mb-6 ${activeTab === 'accounting' ? '' : 'hidden'}`}>
                <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold text-white">Accounting</h2>
                    {accounting?.lastAccountedAt ? (
                        <span className="text-xs text-slate-400">
                            Last accounted {formatRelativeTime(accounting.lastAccountedAt)}
                        </span>
                    ) : (
                        <span className="text-xs text-slate-400">Auto-refresh every 15s</span>
                    )}
                </div>
                {accountingError && (
                    <div className={accountingBannerClass}>
                        {accountingError}
                    </div>
                )}
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-sm mb-4">
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Total LOC</p>
                        <p className="text-white font-semibold text-base">{(accounting?.totalBillableLoc || 0).toLocaleString()}</p>
                    </div>
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Input Tokens</p>
                        <p className="text-white font-semibold text-base">{formatInt(accounting?.totalInputTokens)}</p>
                    </div>
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Output Tokens</p>
                        <p className="text-white font-semibold text-base">{formatInt(accounting?.totalOutputTokens)}</p>
                    </div>
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Total Cost (USD)</p>
                        <p className="text-white font-semibold text-base">{formatCurrency(accounting?.totalCostUsd)}</p>
                    </div>
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Accounted Operations</p>
                        <p className="text-white font-semibold text-base">{(accounting?.accountedOperations || 0).toLocaleString()}</p>
                    </div>
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700">
                        <p className="text-slate-400">Token-tracked Operations</p>
                        <p className="text-white font-semibold text-base">{(accounting?.tokenTrackedOperations || 0).toLocaleString()}</p>
                    </div>
                </div>
                <div className="mb-4 flex flex-wrap gap-2 text-xs">
                    <span className="rounded-full border border-slate-600 bg-slate-900 px-3 py-1 text-slate-200">
                        Helper {helperEnabled ? 'enabled' : 'disabled'}
                    </span>
                    {helperEnabled && helperMode && (
                        <span className="rounded-full border border-sky-700 bg-sky-900/30 px-3 py-1 text-sky-200">
                            Mode: {helperMode}
                        </span>
                    )}
                    {!!stageBreakdown.length && (
                        <span className="rounded-full border border-emerald-700 bg-emerald-900/30 px-3 py-1 text-emerald-200">
                            Stages tracked: {stageBreakdown.length}
                        </span>
                    )}
                </div>
                {!!stageBreakdown.length && (
                    <div className="mb-4">
                        <div className="mb-2 flex items-center justify-between">
                            <h3 className="text-sm font-medium text-white">Model Breakdown</h3>
                            <span className="text-xs text-slate-400">
                                {helperEnabled ? 'Leader and Helper stages' : 'Single-stage review'}
                            </span>
                        </div>
                        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                            {stageBreakdown.map((stage) => {
                                const routeText = getStageRouteText(stage);
                                const executionText = getStageExecutionText(stage);
                                return (
                                    <div
                                        key={`${stage.stage}-${stage.provider || 'unknown'}-${stage.model || 'unknown'}`}
                                        className="rounded-md border border-slate-700 bg-slate-900 p-3"
                                    >
                                        <div className="mb-2 flex items-start justify-between gap-3">
                                            <div>
                                                <p className="text-sm font-semibold text-white">{formatStageLabel(stage.stage)}</p>
                                                <p className="text-xs text-slate-400">
                                                    {(stage.provider || 'unknown provider')} / {(stage.model || 'unknown model')}
                                                </p>
                                            </div>
                                            {stage.pricingVersion && (
                                                <span className="rounded-full border border-slate-600 px-2 py-0.5 text-[11px] text-slate-300">
                                                    {stage.pricingVersion}
                                                </span>
                                            )}
                                        </div>
                                        <div className="grid grid-cols-3 gap-2 text-xs mb-3">
                                            <div className="rounded border border-slate-700 bg-slate-950 p-2">
                                                <p className="text-slate-500">Input</p>
                                                <p className="mt-1 text-sm font-medium text-white">{formatInt(stage.inputTokens)}</p>
                                            </div>
                                            <div className="rounded border border-slate-700 bg-slate-950 p-2">
                                                <p className="text-slate-500">Output</p>
                                                <p className="mt-1 text-sm font-medium text-white">{formatInt(stage.outputTokens)}</p>
                                            </div>
                                            <div className="rounded border border-slate-700 bg-slate-950 p-2">
                                                <p className="text-slate-500">Cost</p>
                                                <p className="mt-1 text-sm font-medium text-white">{formatCurrency(stage.costUsd)}</p>
                                            </div>
                                        </div>
                                        <div className="space-y-1 text-xs text-slate-300">
                                            {executionText && (
                                                <p><span className="text-slate-500">Execution:</span> {executionText}</p>
                                            )}
                                            {routeText && (
                                                <p><span className="text-slate-500">Route:</span> {routeText}</p>
                                            )}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                )}
                {accounting?.latestOperation && (
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700 text-xs">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-y-2 gap-x-4">
                            <p className="text-slate-300"><span className="text-slate-500">Latest operation:</span> {accounting.latestOperation.operationType}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Trigger:</span> {accounting.latestOperation.triggerSource}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Provider/Model:</span> {(accounting.latestOperation.provider || 'unknown')} / {(accounting.latestOperation.model || 'unknown')}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Pricing version:</span> {accounting.latestOperation.pricingVersion || 'unknown'}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Operation ID:</span> {accounting.latestOperation.operationId}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Idempotency key:</span> {accounting.latestOperation.idempotencyKey}</p>
                            {(leaderAIExecutionMode || leaderAIExecutionSource) && (
                                <p className="text-slate-300"><span className="text-slate-500">Leader execution:</span> {(leaderAIExecutionMode || 'unknown')} via {(leaderAIExecutionSource || 'unknown')}</p>
                            )}
                            {(leaderAIExecutionProvider || leaderAIExecutionConnector) && (
                                <p className="text-slate-300"><span className="text-slate-500">Leader route:</span> {(leaderAIExecutionProvider || 'unknown')} / {(leaderAIExecutionConnector || 'unknown')}</p>
                            )}
                            {helperEnabled && (helperAIExecutionMode || helperAIExecutionSource) && (
                                <p className="text-slate-300"><span className="text-slate-500">Helper execution:</span> {(helperAIExecutionMode || 'unknown')} via {(helperAIExecutionSource || 'unknown')}</p>
                            )}
                            {helperEnabled && (helperAIExecutionProvider || helperAIExecutionConnector) && (
                                <p className="text-slate-300"><span className="text-slate-500">Helper route:</span> {(helperAIExecutionProvider || 'unknown')} / {(helperAIExecutionConnector || 'unknown')}</p>
                            )}
                        </div>
                    </div>
                )}
            </div>

            {/* Events Timeline - Full Width */}
            <div className={activeTab === 'events' ? '' : 'hidden'}>
                    <ReviewEventsPage
                        reviewId={reviewId}
                        initialEvents={events.map(event => ({
                            id: event.id.toString(),
                            timestamp: event.time,
                            eventType: mapEventType(event.type) as 'log' | 'status' | 'batch' | 'artifact' | 'completion' | 'retry' | 'json_repair' | 'timeout' | 'started' | 'progress' | 'batch_complete' | 'error' | 'completed',
                            message: formatEventData(event),
                            details: {
                                batchId: event.batchId,
                                ...event.data
                            },
                            severity: mapEventLevel(event.level) as 'info' | 'success' | 'warning' | 'warn' | 'error' | 'debug'
                        }))}
                        isLive={review?.status === 'in_progress'}
                    />
            </div>
        </div>
    );
};

export default ReviewDetail;