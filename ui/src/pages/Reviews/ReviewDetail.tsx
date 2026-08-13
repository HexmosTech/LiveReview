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

const normalizeSource = (provider?: string, prMrUrl?: string): string => {
  if (provider) {
    const normalized = provider.toLowerCase();
    if (normalized === 'cli') return 'cli';
    if (normalized.startsWith('github')) return 'github';
    if (normalized.startsWith('gitlab')) return 'gitlab';
    if (normalized.startsWith('bitbucket')) return 'bitbucket';
    if (normalized.startsWith('gitea')) return 'gitea';
    if (normalized.startsWith('azuredevops')) return 'azuredevops';
  }
  if (prMrUrl) {
    const url = prMrUrl.toLowerCase();
    if (url.includes('github.com')) return 'github';
    if (url.includes('gitlab')) return 'gitlab';
    if (url.includes('bitbucket')) return 'bitbucket';
    if (url.includes('gitea')) return 'gitea';
    if (url.includes('azure')) return 'azuredevops';
  }
  return (provider || '').toLowerCase();
};

const getProviderActionLabel = (provider?: string, prMrUrl?: string): string => {
  const source = normalizeSource(provider, prMrUrl);
  if (source === 'gitlab') return 'View MR';
  if (source === 'cli') return 'CLI';
  return 'View PR';
};

const extractMRInfo = (url?: string): string => {
  if (!url) return 'View PR/MR';
  try {
    const pathParts = new URL(url).pathname.split('/').filter(Boolean);
    if (pathParts.includes('pull') && pathParts.length >= 4) {
      return `PR #${pathParts[pathParts.indexOf('pull') + 1]}`;
    }
    if (pathParts.includes('merge_requests') && pathParts.length >= 4) {
      return `MR !${pathParts[pathParts.indexOf('merge_requests') + 1]}`;
    }
    if (pathParts.includes('pull-requests') && pathParts.length >= 4) {
      return `PR #${pathParts[pathParts.indexOf('pull-requests') + 1]}`;
    }
    return 'View PR/MR';
  } catch {
    return 'View PR/MR';
  }
};

const SourceIcon: React.FC<{ provider?: string; prMrUrl?: string }> = ({ provider, prMrUrl }) => {
  switch (normalizeSource(provider, prMrUrl)) {
    case 'cli': return <LuTerminal size={14} className="shrink-0 text-slate-300" />;
    case 'github': return <span className="shrink-0 inline-flex items-center text-white"><Icons.GitHub /></span>;
    case 'gitlab': return <SiGitlab className="w-4 h-4 shrink-0" style={{ color: '#FC6D26' }} />;
    case 'bitbucket': return <span className="shrink-0 inline-flex items-center text-blue-400"><Icons.Bitbucket /></span>;
    case 'gitea': return <span className="shrink-0 inline-flex items-center text-emerald-400"><Icons.Gitea /></span>;
    case 'azuredevops': return <span className="shrink-0 inline-flex items-center text-blue-500"><Icons.AzureDevOps /></span>;
    default: return null;
  }
};

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
    const [allCommitsShown, setAllCommitsShown] = useState(false);
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

    const [toolAccounting, setToolAccounting] = useState<ToolAccountingData | null>(null);
    const [toolExpanded, setToolExpanded] = useState(false);

    // Fetch review details
    const fetchReviewDetails = useCallback(async () => {
        if (!id) return;
        try {
            setLoading(true);
            setError(null);
            setAccountingError(null);
            setAccountingRouteUnavailable(false);

            if (id === 'test' || id === 'test1' || id === 'test2' || id === 'test3') {
                const stage = id === 'test1' ? 1 : id === 'test2' ? 2 : 3;

                const testPrMrUrl = id === 'test1' 
                    ? 'https://github.com/HexmosTech/git-lrc/pull/131'
                    : id === 'test3'
                    ? 'https://git.apps.hexmos.com/hexmos/livereview/-/merge_requests/2'
                    : undefined;

                const testProvider = id === 'test1' ? 'github'
                    : id === 'test3' ? 'gitlab'
                    : 'cli';

                const testRepo = id === 'test3' ? 'livereview' : id === 'test2' ? 'repo-b' : 'git-lrc';
                const testBranch = id === 'test3' ? 'feat/rag-query' : id === 'test2' ? 'main' : 'feat/tools-integration-beta';

                setReview({
                    id: 999,
                    orgId: 1,
                    repository: testRepo,
                    branch: testBranch,
                    prMrUrl: testPrMrUrl,
                    triggerType: testProvider,
                    userEmail: 'ganeshkumar6120@gmail.com',
                    provider: testProvider,
                    status: stage === 3 ? 'completed' : 'in_progress',
                    createdAt: new Date().toISOString(),
                    completedAt: stage === 3 ? new Date().toISOString() : undefined,
                });

                setSummary({
                    reviewId: 999,
                    currentStatus: stage === 3 ? 'completed' : 'in_progress',
                    lastActivity: new Date().toISOString(),
                    batchCount: 0,
                    eventCounts: { tool_result: stage === 1 ? 0 : stage === 2 ? 5 : 15 },
                });

                if (stage === 1) {
                    // Test Stage 1: All tools Queued/Pending
                    setToolAccounting({
                        totalToolCredits: 0,
                        toolsExecuted: 0,
                        totalCommentsGenerated: 0,
                        toolBreakdown: [
                            { toolName: 'ruff', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'bandit', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'gitleaks', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'eslint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'hadolint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'actionlint', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'shellcheck', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'semgrep', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'trufflehog', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'trivy', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'spectral', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'brakeman', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'kubescape', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'zizmor', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'openapi', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                        ],
                    });
                } else if (stage === 2) {
                    // Test Stage 2: 5 Completed, 5 Running with Animated Spinners, 5 Queued
                    setToolAccounting({
                        totalToolCredits: 4.0,
                        toolsExecuted: 5,
                        totalCommentsGenerated: 14,
                        toolBreakdown: [
                            { toolName: 'ruff', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'bandit', creditsUsed: 1.0, commentsGenerated: 14, status: 'completed' },
                            { toolName: 'gitleaks', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'hadolint', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'shellcheck', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'eslint', creditsUsed: 2.0, commentsGenerated: 0, status: 'running' },
                            { toolName: 'semgrep', creditsUsed: 3.0, commentsGenerated: 0, status: 'running' },
                            { toolName: 'trufflehog', creditsUsed: 2.0, commentsGenerated: 0, status: 'running' },
                            { toolName: 'trivy', creditsUsed: 2.5, commentsGenerated: 0, status: 'running' },
                            { toolName: 'actionlint', creditsUsed: 0.5, commentsGenerated: 0, status: 'running' },
                            { toolName: 'spectral', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'brakeman', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'kubescape', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'zizmor', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                            { toolName: 'openapi', creditsUsed: 0, commentsGenerated: 0, status: 'pending' },
                        ],
                    });
                } else {
                    // Test Stage 3: All 15 Tools Completed with Findings & Failures
                    setToolAccounting({
                        totalToolCredits: 21.0,
                        toolsExecuted: 15,
                        totalCommentsGenerated: 35,
                        toolBreakdown: [
                            { toolName: 'ruff', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'bandit', creditsUsed: 1.0, commentsGenerated: 14, status: 'completed' },
                            { toolName: 'gitleaks', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'eslint', creditsUsed: 2.0, commentsGenerated: 5, status: 'completed' },
                            { toolName: 'hadolint', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'actionlint', creditsUsed: 0.5, commentsGenerated: 2, status: 'completed' },
                            { toolName: 'shellcheck', creditsUsed: 0.5, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'semgrep', creditsUsed: 3.0, commentsGenerated: 8, status: 'completed' },
                            { toolName: 'trufflehog', creditsUsed: 2.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'trivy', creditsUsed: 2.5, commentsGenerated: 3, status: 'completed' },
                            { toolName: 'spectral', creditsUsed: 1.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'brakeman', creditsUsed: 1.5, commentsGenerated: 1, status: 'completed' },
                            { toolName: 'kubescape', creditsUsed: 2.0, commentsGenerated: 0, status: 'clean' },
                            { toolName: 'zizmor', creditsUsed: 1.0, commentsGenerated: 2, status: 'completed' },
                            { toolName: 'openapi', creditsUsed: 0.5, commentsGenerated: 0, status: 'failed' },
                        ],
                    });
                }
                setEvents([
                    {
                        id: 1,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage started: preparation' }
                    },
                    {
                        id: 2,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage completed: preparation' }
                    },
                    {
                        id: 3,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage started: analysis' }
                    },
                    {
                        id: 4,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage completed: analysis' }
                    },
                    {
                        id: 5,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage started: review' }
                    },
                    {
                        id: 6,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'log',
                        level: 'info',
                        data: { message: 'ruff: Clean (0 findings)' }
                    },
                    {
                        id: 7,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'log',
                        level: 'warn',
                        data: { message: 'bandit: 14 comments generated' }
                    },
                    {
                        id: 8,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage completed: review' }
                    },
                    {
                        id: 9,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage started: artifact generation' }
                    },
                    {
                        id: 10,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'artifact',
                        level: 'info',
                        data: { message: 'Posted 14 comments to merge request' }
                    },
                    {
                        id: 11,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'status',
                        level: 'info',
                        data: { message: 'Stage completed: artifact generation' }
                    },
                    {
                        id: 12,
                        reviewId: 999,
                        orgId: 1,
                        time: new Date().toISOString(),
                        type: 'completion',
                        level: 'info',
                        data: { resultSummary: 'Review process completed', message: 'finalization complete' }
                    }
                ]);
                setLoading(false);
                return;
            }
            
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
        setAllCommitsShown(false);
    }, [id]);

    const githubBaseUrl = useMemo(() => {
        if (!review || !review.provider?.toLowerCase().startsWith('github')) return null;
        return `https://github.com/${review.repository}`;
    }, [review]);

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
            {/* Header Card */}
            <div className="mb-6 bg-slate-800/80 p-3.5 rounded-xl border border-slate-700/70 flex flex-col gap-1.5">
                {/* LEVEL 1: Back Button, Repository Title, and Sub-heading (Branch + View PR/MR) */}
                <div className="flex items-start gap-4">
                    <Button
                        as={Link}
                        to="/reviews"
                        variant="ghost"
                        className="mt-0.5 shrink-0"
                    >
                        ← Back
                    </Button>
                    <div>
                        <h1 className="text-2xl font-bold text-white tracking-tight">
                            {review.repository.split('/').pop() || review.repository}
                        </h1>
                        <div className="flex items-center gap-3 mt-0.5">
                            {review.branch && (
                                <span className="text-sm text-slate-300 font-mono font-medium no-underline">{review.branch}</span>
                            )}
                            {normalizeSource(review.provider, review.prMrUrl) !== 'cli' && review.prMrUrl ? (
                                <a 
                                    href={review.prMrUrl} 
                                    target="_blank" 
                                    rel="noopener noreferrer"
                                    className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-md border border-slate-700/80 bg-slate-800/90 text-slate-200 hover:bg-slate-700 hover:text-white text-xs font-medium transition-colors shadow-sm cursor-pointer no-underline"
                                    title="Open Pull/Merge Request"
                                >
                                    <SourceIcon provider={review.provider} prMrUrl={review.prMrUrl} />
                                    <span className="no-underline">{getProviderActionLabel(review.provider, review.prMrUrl)}</span>
                                </a>
                            ) : (
                                <div className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-md border border-slate-700/60 bg-slate-800/50 text-slate-300 text-xs font-medium">
                                    <SourceIcon provider={review.provider} prMrUrl={review.prMrUrl} />
                                    <span className="uppercase font-mono text-[11px] text-slate-300">CLI</span>
                                </div>
                            )}
                        </div>
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
                                    <ul className={`space-y-2 ${allCommitsShown && commits.length > COMMITS_PREVIEW_LIMIT ? 'max-h-64 overflow-y-auto pr-1' : ''}`}>
                                        {(allCommitsShown ? commits : commits.slice(0, COMMITS_PREVIEW_LIMIT)).map((commit) => (
                                            <li key={commit.ref} className="flex items-center justify-between gap-3 text-xs">
                                                <div className="flex items-center gap-2 min-w-0">
                                                    {commit.refType === 'commit' && githubBaseUrl ? (
                                                        <a
                                                            href={`${githubBaseUrl}/commit/${commit.ref}`}
                                                            target="_blank"
                                                            rel="noopener noreferrer"
                                                            className="font-mono text-slate-200 shrink-0 hover:text-blue-400 hover:underline"
                                                        >
                                                            {commit.ref.substring(0, 8)}
                                                        </a>
                                                    ) : (
                                                        <span className="font-mono text-slate-200 shrink-0">
                                                            {commit.refType === 'commit' ? commit.ref.substring(0, 8) : commit.ref}
                                                        </span>
                                                    )}
                                                    {commit.refType === 'range' && (
                                                        <span className="shrink-0 text-[10px] uppercase tracking-wide rounded-full px-2 py-0.5 border bg-purple-900/30 text-purple-300 border-purple-700">
                                                            range
                                                        </span>
                                                    )}
                                                </div>
                                                <span className="shrink-0 text-slate-400">{formatRelativeTime(commit.createdAt)}</span>
                                            </li>
                                        ))}
                                    </ul>
                                    {commits.length > COMMITS_PREVIEW_LIMIT && (
                                        <button
                                            type="button"
                                            onClick={() => setAllCommitsShown((prev) => !prev)}
                                            className="mt-2 text-xs text-slate-400 hover:text-white hover:underline"
                                        >
                                            {allCommitsShown ? 'Show less' : `+${commits.length - COMMITS_PREVIEW_LIMIT} more commits`}
                                        </button>
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
                        <span className="text-slate-500">
                            Last accounted {formatRelativeTime(accounting.lastAccountedAt)}
                        </span>
                    ) : (
                        <span className="text-slate-500">Auto-refresh every 15s</span>
                    )}
                </div>
                {accountingError && (
                    <div className={accountingBannerClass}>
                        {accountingError}
                    </div>
                )}
                <div className="grid grid-cols-2 md:grid-cols-6 gap-2 text-xs mb-3">
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Total LOC</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.totalBillableLoc || 0).toLocaleString()}</p>
                    </div>
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Input Tokens</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{formatInt(accounting?.totalInputTokens)}</p>
                    </div>
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Output Tokens</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{formatInt(accounting?.totalOutputTokens)}</p>
                    </div>
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Total Cost (USD)</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{formatCurrency(accounting?.totalCostUsd)}</p>
                    </div>
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Accounted Ops</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.accountedOperations || 0).toLocaleString()}</p>
                    </div>
                    <div className="bg-slate-900/80 rounded p-2.5 border border-slate-800">
                        <p className="text-slate-500">Token-tracked Ops</p>
                        <p className="text-white font-semibold text-sm mt-0.5">{(accounting?.tokenTrackedOperations || 0).toLocaleString()}</p>
                    </div>
                </div>

                {!!stageBreakdown.length && (
                    <div className="mt-3 pt-3 border-t border-slate-700/60">
                        <div className="mb-2 flex items-center justify-between">
                            <h3 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">Model Breakdown</h3>
                            <span className="text-[11px] text-slate-500">
                                {helperEnabled ? 'Leader & Helper stages' : 'Single-stage review'}
                            </span>
                        </div>
                        <div className="grid grid-cols-1 gap-2 lg:grid-cols-2">
                            {stageBreakdown.map((stage) => {
                                const routeText = getStageRouteText(stage);
                                const executionText = getStageExecutionText(stage);
                                return (
                                    <div
                                        key={`${stage.stage}-${stage.provider || 'unknown'}-${stage.model || 'unknown'}`}
                                        className="rounded border border-slate-800 bg-slate-900/90 p-2.5"
                                    >
                                        <div className="mb-2 flex items-start justify-between gap-3">
                                            <div>
                                                <p className="text-xs font-semibold text-white">{formatStageLabel(stage.stage)}</p>
                                                <p className="text-[11px] text-slate-400">
                                                    {(stage.provider || 'unknown provider')} / {(stage.model || 'unknown model')}
                                                </p>
                                            </div>
                                            {stage.pricingVersion && (
                                                <span className="rounded border border-slate-700 px-2 py-0.5 text-[10px] text-slate-400">
                                                    {stage.pricingVersion}
                                                </span>
                                            )}
                                        </div>
                                        <div className="grid grid-cols-3 gap-2 text-xs">
                                            <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                                <p className="text-[10px] text-slate-500">Input</p>
                                                <p className="text-xs font-medium text-white">{formatInt(stage.inputTokens)}</p>
                                            </div>
                                            <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                                <p className="text-[10px] text-slate-500">Output</p>
                                                <p className="text-xs font-medium text-white">{formatInt(stage.outputTokens)}</p>
                                            </div>
                                            <div className="rounded border border-slate-800 bg-slate-950 p-1.5">
                                                <p className="text-[10px] text-slate-500">Cost</p>
                                                <p className="text-xs font-medium text-white">{formatCurrency(stage.costUsd)}</p>
                                            </div>
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
            )}
        </div>
    );
};

export default ReviewDetail;