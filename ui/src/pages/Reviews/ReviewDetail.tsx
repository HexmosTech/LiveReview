import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, useNavigate, useSearchParams, Link } from 'react-router-dom';
import { riskAssessment, RiskScore } from '../../components/reviews/RiskAssessment';
import { Button, Icons, Tabs } from '../../components/UIPrimitives';
import { ReviewEventsPage, DiffViewerPanel } from '../../components/reviews';
import AutoApprovePanel from '../../components/reviews/AutoApprovePanel';
import { getReviewPolicyLog } from '../../api/riskPolicy';
import { ToolAccountingData, ToolBreakdownItem } from '../../components/reviews/ToolAnalysisCard';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import { isCloudMode } from '../../utils/deploymentMode';
import {
    getReview,
    getReviewEvents,
    getReviewSummary,
    getReviewAccounting,
    getReviewCommits,
    getBlastRadiusReport,
} from '../../api/reviews';
import {
    Review,
    ReviewAccounting,
    ReviewCommit,
    ReviewEvent,
    ReviewEventLevel,
    ReviewEventType,
    ReviewSummary,
    BlastRadiusReport,
    BlastRadiusSkipped,
    isBlastRadiusSkipped,
} from '../../types/reviews';
import {
    deriveToolAccounting,
    hasAccountingDetails,
    formatEventData,
    ACCOUNTING_REFRESH_INTERVAL_MS,
    ACTIVE_POLL_INTERVAL_MS,
} from './reviewDetailHelpers';
import { getMockReviewData } from './reviewDetailMocks';
import { ReviewDetailHeader } from '../../components/reviews/ReviewDetailHeader';
import { ReviewAccountingPanel } from '../../components/reviews/ReviewAccountingPanel';

type TabId = 'findings' | 'accounting' | 'events' | 'auto-approve';

const ReviewDetail: React.FC = () => {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const reviewId = parseInt(id || '0', 10);

    const mapEventType = (type: ReviewEventType) => type;
    const mapEventLevel = (level: ReviewEventLevel) => level;

    const [review, setReview] = useState<Review | null>(null);
    const [events, setEvents] = useState<ReviewEvent[]>([]);
    const [eventCount, setEventCount] = useState<number>(0);
    const [summary, setSummary] = useState<ReviewSummary | null>(null);
    const [accounting, setAccounting] = useState<ReviewAccounting | null>(null);
    const [accountingError, setAccountingError] = useState<string | null>(null);
    const [accountingErrorTone, setAccountingErrorTone] = useState<'info' | 'warning'>('info');
    const [accountingRouteUnavailable, setAccountingRouteUnavailable] = useState(false);
    const [commits, setCommits] = useState<ReviewCommit[]>([]);
    const [commitsLoaded, setCommitsLoaded] = useState(false);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [pollingEnabled, setPollingEnabled] = useState(true);

    const TAB_IDS: TabId[] = ['findings', 'accounting', 'events', 'auto-approve'];
    const [searchParams, setSearchParams] = useSearchParams();
    const tabParam = searchParams.get('tab');
    const activeTab: TabId = (TAB_IDS as string[]).includes(tabParam || '') ? (tabParam as TabId) : 'findings';
    const setActiveTab = useCallback((tab: TabId, opts?: { replace?: boolean }) => {
        setSearchParams(prev => {
            const next = new URLSearchParams(prev);
            if (tab === 'findings') {
                next.delete('tab');
            } else {
                next.set('tab', tab);
            }
            return next;
        }, { replace: opts?.replace ?? false });
    }, [setSearchParams]);

    const [blastReport, setBlastReport] = useState<BlastRadiusReport | BlastRadiusSkipped | null>(null);
    const [blastPoll, setBlastPoll] = useState<'checking' | 'waiting' | 'gave_up'>('checking');
    const [riskScores, setRiskScores] = useState<RiskScore[] | null>(null);
    const [commentCount, setCommentCount] = useState<number | undefined>(undefined);

    useEffect(() => {
        if (!review) return;
        let cancelled = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        let attempts = 0;
        const finished = review.status === 'completed' || review.status === 'failed' || (review.status as string) === 'partial';
        const maxAttempts = finished ? 3 : 30;
        const retryDelayMs = finished ? 2500 : 30000;
        const requestBudgetMs = 5000;

        const cacheKey = `lr:blast-report:${reviewId}`;
        let seeded = false;
        try {
            const cached = sessionStorage.getItem(cacheKey);
            if (cached) {
                setBlastReport(JSON.parse(cached) as BlastRadiusReport | BlastRadiusSkipped);
                seeded = true;
            }
        } catch {
            // A stale or corrupt entry just means one extra fetch.
        }
        if (!seeded) setBlastReport(null);
        setBlastPoll('checking');
        setRiskScores(null);
        let inflight: AbortController | null = null;
        const load = () => {
            const controller = new AbortController();
            inflight = controller;
            const abortTimer = setTimeout(() => controller.abort(), requestBudgetMs);
            getBlastRadiusReport(reviewId, controller.signal)
                .then((report) => {
                    if (cancelled) return;
                    setBlastReport(report);
                    try {
                        sessionStorage.setItem(cacheKey, JSON.stringify(report));
                    } catch {
                        // Storage full or blocked; the in-memory state is still correct.
                    }
                })
                .catch(() => {
                    if (cancelled) return;
                    if (++attempts < maxAttempts) {
                        setBlastPoll('waiting');
                        timer = setTimeout(load, retryDelayMs);
                    } else {
                        setBlastPoll('gave_up');
                    }
                })
                .finally(() => clearTimeout(abortTimer));
        };
        load();
        return () => {
            cancelled = true;
            clearTimeout(timer);
            inflight?.abort();
            inflight = null;
        };
    }, [reviewId, review?.status]);

    const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);
    const defaultedTabForReviewRef = useRef<number | null>(null);

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

    const { currentOrg, isFreePlan, isPaidPlan } = useOrgContext();
    const showToolsUI = isCloudMode() && Boolean(isPaidPlan) && !isFreePlan;
    const [dbToolMultipliers, setDbToolMultipliers] = useState<Record<string, number>>({});
    const [toolAccounting, setToolAccounting] = useState<ToolAccountingData | null>(null);

    const effectiveToolAccounting = useMemo<ToolAccountingData | null>(() => {
        if (toolAccounting) return toolAccounting;
        const derived = deriveToolAccounting(summary?.toolSummary, dbToolMultipliers);
        if (derived) return derived;
        const toolEvents = events ? events.filter(e => (e.type as string) === 'tool_dispatch' || (e.type as string) === 'tool_result') : [];
        if (toolEvents.length === 0) {
            return null;
        }

        const toolBreakdown: ToolBreakdownItem[] = [];

        toolEvents.forEach(evt => {
            let data: Record<string, any> = {};
            if (typeof evt.data === 'string') {
                try {
                    data = JSON.parse(evt.data);
                } catch {
                    data = {};
                }
            } else if (evt.data && typeof evt.data === 'object') {
                data = evt.data as Record<string, any>;
            }
            const toolName = (data.tool_name || data.toolName || data.tool || '') as string;
            if (!toolName) return;

            let existing = toolBreakdown.find(t => t.toolName.toLowerCase() === toolName.toLowerCase());
            if (!existing) {
                const multiplier = dbToolMultipliers[toolName.toLowerCase()] ?? 1.0;
                existing = {
                    toolName,
                    creditsUsed: multiplier,
                    commentsGenerated: 0,
                    status: 'pending',
                };
                toolBreakdown.push(existing);
            }

            if (evt.type === 'tool_result') {
                const count = typeof data.comments_count === 'number'
                    ? data.comments_count
                    : typeof data.findings_count === 'number'
                    ? data.findings_count
                    : Array.isArray(data.findings)
                    ? data.findings.length
                    : 0;
                existing.commentsGenerated = count;

                const eventCredits = typeof data.credits_used === 'number' ? data.credits_used
                    : typeof data.credits === 'number' ? data.credits
                    : typeof data.cost === 'number' ? data.cost
                    : typeof data.multiplier === 'number' ? data.multiplier
                    : undefined;
                if (typeof eventCredits === 'number') {
                    existing.creditsUsed = eventCredits;
                }

                let status = data.status as ToolBreakdownItem['status'] | undefined;
                if (!status) {
                    if (data.exit_code && data.exit_code !== 0) {
                        status = 'failed';
                    } else if (data.truncated) {
                        status = 'partial';
                    } else if (count > 0) {
                        status = 'completed';
                    } else {
                        status = 'clean';
                    }
                }
                existing.status = status;
            } else if (evt.type === 'tool_dispatch') {
                if (existing.status === 'pending') {
                    existing.status = 'running';
                }
            }
        });

        const totalComments = toolBreakdown.reduce((sum, t) => sum + (t.commentsGenerated || 0), 0);
        const executedCount = toolBreakdown.filter(t => t.status === 'completed' || t.status === 'clean' || t.status === 'failed' || t.status === 'partial').length;
        const totalCredits = toolBreakdown
            .filter(t => t.status === 'completed' || t.status === 'clean' || t.status === 'failed' || t.status === 'partial')
            .reduce((sum, t) => sum + (t.creditsUsed || 0), 0);

        return {
            totalToolCredits: totalCredits,
            toolsExecuted: executedCount,
            totalCommentsGenerated: totalComments,
            toolBreakdown,
        };
    }, [toolAccounting, summary?.toolSummary, events, dbToolMultipliers]);

    const hasToolData = Boolean(
        effectiveToolAccounting &&
        effectiveToolAccounting.toolBreakdown &&
        effectiveToolAccounting.toolBreakdown.length > 0
    );
    const showToolsDropdown = Boolean(hasToolData);

    useEffect(() => {
        if (!currentOrg?.id) return;
        apiClient.get<{ tools?: Array<{ name: string; multiplier: number }> }>(`/orgs/${currentOrg.id}/tools`)
            .then(res => {
                const catalog = res.tools || [];
                const multMap: Record<string, number> = {};
                catalog.forEach(t => {
                    if (t.name && typeof t.multiplier === 'number') {
                        multMap[t.name.toLowerCase()] = t.multiplier;
                    }
                });
                setDbToolMultipliers(multMap);
            })
            .catch(() => {
                // Silently fallback if offline or endpoint unavailable
            });
    }, [currentOrg?.id]);

    const fetchReviewDetails = useCallback(async () => {
        if (!id) return;
        try {
            setLoading(true);
            setError(null);
            setAccountingError(null);
            setAccountingRouteUnavailable(false);

            const mock = getMockReviewData(id);
            if (mock) {
                setReview(mock.review);
                setSummary(mock.summary);
                setToolAccounting(mock.toolAccounting);
                setEvents(mock.events);
                setLoading(false);
                return;
            }

            const currentReviewId = parseInt(id, 10);
            if (isNaN(currentReviewId)) {
                setError('Invalid review ID');
                setLoading(false);
                return;
            }

            const [reviewData, eventsData, summaryData] = await Promise.all([
                getReview(currentReviewId),
                getReviewEvents(currentReviewId, undefined, 1000).catch(() => ({ events: [] as ReviewEvent[], meta: { count: 0 } })),
                getReviewSummary(currentReviewId).catch((): ReviewSummary | null => null),
            ]);

            setReview(reviewData);
            setSummary(summaryData);

            const newEvents = (eventsData?.events as ReviewEvent[] | undefined) || [];
            setEvents(newEvents);
            if (eventsData?.meta?.count !== undefined) {
                setEventCount(eventsData.meta.count);
            } else {
                setEventCount(newEvents.length);
            }

            if (summaryData?.toolSummary) {
                const derived = deriveToolAccounting(summaryData.toolSummary, dbToolMultipliers);
                setToolAccounting(derived);
            } else {
                setToolAccounting(null);
            }
            await fetchAccountingDetails(currentReviewId, reviewData.status);

            const commitsData = await getReviewCommits(currentReviewId).catch(() => ({ commits: [] as ReviewCommit[] }));
            setCommits(commitsData.commits || []);
            setCommitsLoaded(true);
        } catch (err) {
            console.error('Error fetching review details:', err);
            setError(err instanceof Error ? err.message : 'Failed to load review details');
        } finally {
            setLoading(false);
        }
    }, [id, fetchAccountingDetails, dbToolMultipliers]);

    // Reset event cursor and list when navigating to a different review
    useEffect(() => {
        setEvents([]);
        setCommits([]);
        setCommitsLoaded(false);
        setToolAccounting(null);
    }, [id]);

    // Default to Events tab while review is running or failed; once completed, default to Findings
    useEffect(() => {
        if (!review || defaultedTabForReviewRef.current === review.id) return;
        defaultedTabForReviewRef.current = review.id;
        if (review.status !== 'completed' && (review.status as string) !== 'partial') {
            setActiveTab('events');
        }
    }, [review, setActiveTab]);

    // Jump to Findings when active review finishes
    const lastStatusRef = useRef<string | undefined>(undefined);
    const [autoApproveAttention, setAutoApproveAttention] = useState(false);
    useEffect(() => {
        const prev = lastStatusRef.current;
        lastStatusRef.current = review?.status;
        if ((prev === 'created' || prev === 'in_progress') && (review?.status === 'completed' || (review?.status as string) === 'partial')) {
            setActiveTab('findings');
            if (review.metadata?.pr_link_policy) setAutoApproveAttention(true);
        }
    }, [review?.status, review?.metadata?.pr_link_policy, setActiveTab]);

    useEffect(() => {
        if (activeTab === 'auto-approve') setAutoApproveAttention(false);
    }, [activeTab]);

    // Auto-approve policy outcome for PR-link reviews
    const [policyApproved, setPolicyApproved] = useState<boolean | null>(null);
    useEffect(() => {
        if (!review?.metadata?.pr_link_policy || review.status !== 'completed') return;
        let cancelled = false;
        let tries = 0;
        const load = () => {
            getReviewPolicyLog(reviewId)
                .then((log) => {
                    if (cancelled) return;
                    if (log.stages.length) setPolicyApproved(!log.blocked);
                    else if (++tries < 12) setTimeout(load, 5000);
                })
                .catch(() => {});
        };
        load();
        return () => { cancelled = true; };
    }, [reviewId, review?.status, review?.metadata?.pr_link_policy]);

    const githubBaseUrl = useMemo(() => {
        if (!review || !review.provider?.toLowerCase().startsWith('github')) return null;
        return `https://github.com/${review.repository}`;
    }, [review]);

    const eventSeverityCounts = useMemo(() => {
        if (summary?.severityCounts) {
            return summary.severityCounts;
        }
        let high = 0, medium = 0, low = 0;
        events.forEach(e => {
            const dataAny = e.data as any;
            const sev = (dataAny?.severity || '').toLowerCase();
            const eventType = String(e.type);
            if (e.level === 'error' || sev === 'critical' || sev === 'high') high++;
            else if (e.level === 'warn' || sev === 'warning' || sev === 'medium') medium++;
            else if (sev === 'info' || sev === 'low' || (e.level === 'info' && eventType === 'tool_result')) low++;
            else if (e.level === 'info' && eventType === 'tool_dispatch') {
                // Ignore dispatch log events
            } else if (sev === 'info' || e.level === 'info') low++;
        });
        return { high, medium, low };
    }, [summary?.severityCounts, events]);

    // Initial load
    useEffect(() => {
        fetchReviewDetails();
    }, [fetchReviewDetails]);

    // Polling loop for active reviews and accounting updates
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

        const pollInterval = (review?.status === 'created' || review?.status === 'in_progress') ? ACTIVE_POLL_INTERVAL_MS : ACCOUNTING_REFRESH_INTERVAL_MS;
        pollingIntervalRef.current = setInterval(() => {
            void fetchAccountingDetails(currentReviewId, review?.status);
            // Refresh review and summary so tools and findings show live without manual page refresh
            if (review?.status === 'created' || review?.status === 'in_progress') {
                getReview(currentReviewId).then(setReview).catch(() => {});
                getReviewSummary(currentReviewId).then((summaryData) => {
                    setSummary(summaryData);
                    if (summaryData?.toolSummary) {
                        const derived = deriveToolAccounting(summaryData.toolSummary, dbToolMultipliers);
                        if (derived) {
                            setToolAccounting(derived);
                        }
                    }
                }).catch((err) => {
                    console.warn('Review summary polling skipped:', err);
                });
            }
        }, pollInterval);

        return () => {
            if (pollingIntervalRef.current) {
                clearInterval(pollingIntervalRef.current);
                pollingIntervalRef.current = null;
            }
        };
    }, [id, pollingEnabled, accountingRouteUnavailable, review?.status, accounting, fetchAccountingDetails, dbToolMultipliers]);

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
                            icon={<Icons.Back />}
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

    const risk = riskAssessment(blastReport, blastPoll, !!review.prMrUrl, riskScores);
    const accountingBannerClass = accountingErrorTone === 'warning'
        ? 'mb-4 p-3 bg-amber-950/40 border border-amber-800/60 rounded text-xs text-amber-200'
        : 'mb-4 p-3 bg-slate-900 border border-slate-700 rounded text-xs text-slate-400';

    return (
        <div className="container mx-auto px-4 py-8">
            <ReviewDetailHeader
                key={review.id}
                review={review}
                summary={summary}
                events={events}
                eventSeverityCounts={eventSeverityCounts}
                risk={risk}
                policyApproved={policyApproved}
                commits={commits}
                commitsLoaded={commitsLoaded}
                githubBaseUrl={githubBaseUrl}
                showToolsDropdown={showToolsDropdown}
                effectiveToolAccounting={effectiveToolAccounting}
                onViewAllEvents={() => setActiveTab('events')}
            />

            <Tabs
                className="mb-6"
                activeTab={activeTab}
                onChange={(id) => setActiveTab(id as typeof activeTab)}
                tabs={[
                    { id: 'findings', label: 'Findings' },
                    { id: 'accounting', label: 'Accounting' },
                    { id: 'events', label: 'Events' },
                    ...(review.metadata?.pr_link_policy || review.prMrUrl ? [{
                        id: 'auto-approve',
                        label: (
                            <span className="inline-flex items-center gap-2">
                                Auto Approve
                                {autoApproveAttention && (
                                    <span className="relative inline-flex h-2.5 w-2.5">
                                        <span className={`absolute inline-flex h-full w-full rounded-full opacity-75 animate-ping ${policyApproved === false ? 'bg-red-400' : 'bg-green-400'}`} />
                                        <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${policyApproved === false ? 'bg-red-500' : 'bg-green-500'}`} />
                                    </span>
                                )}
                            </span>
                        ),
                    }] : []),
                ]}
            />

            {/* Auto Approve Panel */}
            {activeTab === 'auto-approve' && (review.metadata?.pr_link_policy || review.prMrUrl) && (
                <div className="mb-6 space-y-4">
                    <AutoApprovePanel reviewId={reviewId} />
                </div>
            )}

            {/* Findings Panel */}
            {activeTab === 'findings' && (
                <div className="mb-6">
                    {/* No key={review?.status} here - DiffViewerPanel has its own
                        internal polling that refetches on completion (see its
                        header comment). Remounting it on every status transition
                        used to wipe already-streamed comments and the comment nav
                        back to a loading state, then rebuild them from scratch -
                        visible as a flash of findings disappearing mid-review. */}
                    <DiffViewerPanel
                        reviewId={reviewId}
                        blastReport={blastReport && !isBlastRadiusSkipped(blastReport) ? blastReport : undefined}
                        onRiskScores={setRiskScores}
                        onCommentCount={setCommentCount}
                        reviewStartedAt={review.startedAt}
                    />
                </div>
            )}

            {/* Accounting Panel */}
            <ReviewAccountingPanel
                accounting={accounting}
                accountingError={accountingError}
                accountingBannerClass={accountingBannerClass}
                showToolsDropdown={showToolsDropdown}
                reviewMetadata={review.metadata}
                active={activeTab === 'accounting'}
            />

            {/* Events Timeline */}
            <div className={activeTab === 'events' ? '' : 'hidden'}>
                <ReviewEventsPage
                    reviewId={reviewId}
                    initialEventCount={eventCount}
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
