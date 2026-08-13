import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { LuTerminal } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Button, Icons } from '../../components/UIPrimitives';
import { ReviewEventsPage } from '../../components/reviews';
import { ToolAnalysisCard, ToolAccountingData } from '../../components/reviews/ToolAnalysisCard';
import { 
  getReview, 
  getReviewEvents, 
  getReviewSummary, 
    getReviewAccounting,
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
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [pollingEnabled, setPollingEnabled] = useState(true);
    const [levelFilter, setLevelFilter] = useState<ReviewEventLevel | ''>('');
    const [typeFilter, setTypeFilter] = useState<ReviewEventType | ''>('');
    const [lastEventTime, setLastEventTime] = useState<string | null>(null);
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

                {/* LEVEL 2: Single Unbroken Line */}
                <div className="flex flex-nowrap items-center justify-between gap-3 text-xs whitespace-nowrap overflow-x-auto scrollbar-none pt-1">
                    {/* Left: Remaining Review Info + Progress Badge (No Provider repetition) */}
                    <div className="flex items-center gap-x-3 text-xs shrink-0">
                        {review.userEmail && (
                            <div className="flex items-center gap-1">
                                <span className="text-slate-400">User:</span>
                                <span className="text-white text-xs">{review.userEmail}</span>
                            </div>
                        )}
                        <div className="flex items-center gap-1">
                            <span className="text-slate-400">Created:</span>
                            <span className="text-white text-xs">{new Date(review.createdAt).toLocaleDateString()} {new Date(review.createdAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                        </div>
                        {summary?.lastActivity && (
                            <div className="flex items-center gap-1">
                                <span className="text-slate-400">Last Activity:</span>
                                <span className="text-white text-xs">{new Date(summary.lastActivity).toLocaleDateString()} {new Date(summary.lastActivity).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                            </div>
                        )}
                        {summary && (
                            <div className="flex items-center gap-1">
                                <span className="text-slate-400">Events:</span>
                                <span className="text-white text-xs">{Object.values(summary.eventCounts || {}).reduce((a: number, b: number) => a + b, 0)}</span>
                            </div>
                        )}
                        {summary && (
                            <div className="flex items-center gap-1">
                                <span className="text-slate-400">Batches:</span>
                                <span className="text-white text-xs">{summary.batchCount}</span>
                            </div>
                        )}
                        <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-[11px] font-semibold tracking-wide text-white uppercase shadow-sm shrink-0 ${getStatusColor(review.status)}`}>
                            {review.status.replace('_', ' ')}
                        </span>
                    </div>

                    {/* Right: Static Numbers (Tools:15 Findings:14 Credits:4 cr) & Expand Button */}
                    <div className="shrink-0 flex items-center gap-3">
                        {toolAccounting && (() => {
                            const totalTools = toolAccounting.toolBreakdown.length || toolAccounting.toolsExecuted;
                            const totalFindings = toolAccounting.totalCommentsGenerated;
                            const totalCredits = toolAccounting.totalToolCredits;
                            const hasFindings = totalFindings > 0;
                            return (
                                <div className="flex items-center gap-3">
                                    <div className="flex items-center gap-2.5 text-xs">
                                        <div className="flex items-center gap-1">
                                            <span className="text-slate-400">Tools:</span>
                                            <span className="text-white font-mono text-xs">{totalTools}</span>
                                        </div>
                                        <div className="flex items-center gap-1">
                                            <span className="text-slate-400">Findings:</span>
                                            <span className={`font-mono text-xs ${hasFindings ? 'text-amber-400 font-bold' : 'text-white'}`}>{totalFindings}</span>
                                        </div>
                                        <div className="flex items-center gap-1">
                                            <span className="text-slate-400">Credits:</span>
                                            <span className="text-white font-mono text-xs">{totalCredits % 1 === 0 ? totalCredits.toFixed(0) : totalCredits.toFixed(1)} cr</span>
                                        </div>
                                    </div>

                                    <button
                                        type="button"
                                        onClick={() => setToolExpanded(!toolExpanded)}
                                        className="inline-flex items-center gap-1.5 px-3 py-1 rounded-lg border text-xs font-medium transition-colors shrink-0 bg-slate-800 text-slate-200 border-slate-700 hover:bg-slate-700 hover:text-white"
                                    >
                                        {toolExpanded ? (
                                            /* Collapse icon - arrows pointing inward */
                                            <svg className="w-3.5 h-3.5 opacity-70" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 9L4 4m0 0h5m-5 0v5M15 9l5-5m0 0h-5m5 0v5M9 15l-5 5m0 0h5m-5 0v-5M15 15l5 5m0 0h-5m5 0v-5" />
                                            </svg>
                                        ) : (
                                            /* Expand icon - arrows pointing outward */
                                            <svg className="w-3.5 h-3.5 opacity-70" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 8V4m0 0h4M4 4l5 5m11-5h-4m4 0v4m0-4l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5h-4m4 0v-4m0 4l-5-5" />
                                            </svg>
                                        )}
                                        <span>Static Analysis Tools</span>
                                    </button>
                                </div>
                            );
                        })()}
                    </div>
                </div>

                {/* Expanded Breakdown Panel below */}
                {toolAccounting && toolExpanded && (
                    <div className="w-full mt-1.5">
                        <ToolAnalysisCard data={toolAccounting} embedded isExpanded={toolExpanded} hideSummary />
                    </div>
                )}
            </div>

            {/* Priority #1: Events Timeline & Review Findings (Dominates the screen) */}
            <div className="mb-8">
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

            {/* Tertiary Utility: AI Accounting Panel */}
            {(!toolAccounting || (accounting && hasAccountingDetails(accounting))) && (
            <div className="bg-slate-800/80 rounded-lg p-4 border border-slate-700/70 text-xs">
                <div className="flex items-center justify-between mb-3">
                    <h2 className="text-sm font-semibold text-slate-300">AI Model Accounting</h2>
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
            </div>
            )}
        </div>
    );
};

export default ReviewDetail;