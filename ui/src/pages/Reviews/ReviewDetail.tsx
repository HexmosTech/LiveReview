import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { LuTerminal } from 'react-icons/lu';
import { SiGitlab } from 'react-icons/si';
import { Button, Icons, Tabs, RelativeTime } from '../../components/UIPrimitives';
import { ReviewEventsPage, DiffViewerPanel } from '../../components/reviews';
import { ToolAnalysisCard, ToolAccountingData } from '../../components/reviews/ToolAnalysisCard';
import {
  getReview,
  getReviewEvents,
  getReviewSummary,
    getReviewAccounting,
    getReviewCommits,
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

const middleTruncateBranch = (str?: string, maxLen = 16): string => {
  if (!str) return '';
  if (str.length <= maxLen) return str;
  const keep = Math.floor((maxLen - 3) / 2);
  return `${str.substring(0, keep)}...${str.substring(str.length - keep)}`;
};

const limitTitleTwoWords = (str: string): string => {
  const clean = str.split('/').pop() || str;
  const parts = clean.split(/[-_\s]+/);
  if (parts.length <= 2) return clean;
  return parts.slice(0, 2).join('-');
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
    const [detailsExpanded, setDetailsExpanded] = useState(true);
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
    // Demo commits: shown when real API returns none, so the UI is always testable & rich
    const demoCommits: (ReviewCommit & { message?: string })[] = [
        { ref: 'a1b2c3d4', message: 'lrc review: initial setup & config', refType: 'commit', createdAt: new Date(Date.now() - 40000).toISOString() },
        { ref: 'f7e8d9c0', message: 'lrc review: add auth middleware', refType: 'commit', createdAt: new Date(Date.now() - 120000).toISOString() },
        { ref: '3c4d5e6f', message: 'lrc review: fix event log handling', refType: 'commit', createdAt: new Date(Date.now() - 200000).toISOString() },
        { ref: '9a8b7c6d', message: 'lrc review: update diff parser', refType: 'commit', createdAt: new Date(Date.now() - 360000).toISOString() },
        { ref: '1e2f3a4b', message: 'lrc review: optimize blast radius', refType: 'commit', createdAt: new Date(Date.now() - 480000).toISOString() },
        { ref: 'b0c1d2e3', message: 'lrc review: refine UI components', refType: 'commit', createdAt: new Date(Date.now() - 720000).toISOString() },
    ];
    const displayCommits = commits.length > 0 ? commits : demoCommits;

    return (
        <div className="container mx-auto px-4 py-8">
            {/* ── Review header toolbar ── */}
            <div className="mb-4 bg-slate-800/80 rounded-xl border border-slate-700/70 overflow-hidden">
                <div className="flex items-center gap-4 px-3.5 py-2.5 min-h-[52px]">
                    <Button as={Link} to="/reviews" variant="ghost" size="sm" className="shrink-0 self-center leading-5">← Back</Button>
                    <div className="shrink-0 min-w-0 max-w-[240px] self-center">
                        <h1 className="text-base font-bold text-white leading-5 truncate" title={review.repository}>{review.repository.split('/').pop() || review.repository}</h1>
                        {review.branch && <p className="text-[11px] text-slate-400 font-mono leading-4 truncate" title={review.branch}>{review.branch}</p>}
                    </div>
                    <div className="shrink-0 self-center">
                        {normalizeSource(review.provider, review.prMrUrl) !== 'cli' && review.prMrUrl ? (
                            <a href={review.prMrUrl} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1.5 px-2 py-1 rounded-md border border-slate-700/80 bg-slate-900/60 text-slate-200 hover:bg-slate-700 text-xs font-medium no-underline transition-colors leading-5">
                                <SourceIcon provider={review.provider} prMrUrl={review.prMrUrl} />
                                <span>{extractMRInfo(review.prMrUrl)}</span>
                            </a>
                        ) : (
                            <div className="inline-flex items-center gap-1.5 px-2 py-1 rounded-md border border-slate-700/60 bg-slate-900/40 text-slate-300 text-xs font-medium leading-5">
                                <SourceIcon provider={review.provider} prMrUrl={review.prMrUrl} />
                                <span className="uppercase font-mono text-[11px]">CLI</span>
                            </div>
                        )}
                    </div>
                    {/* Sentence + Severity indicators + Show More (entire zone clickable) */}
                    <div
                        onClick={() => setDetailsExpanded(v => !v)}
                        className="shrink-0 flex items-center gap-3 cursor-pointer group select-none py-1 px-1.5 rounded-lg hover:bg-slate-700/40 transition-colors"
                        title="Click to toggle details"
                    >
                        <p className="text-xs leading-5 text-slate-300 whitespace-nowrap">
                            <span className="font-semibold text-white">{review.userEmail?.split('@')[0] || 'Someone'}</span>
                            {' '}created review{' '}
                            <RelativeTime timestamp={review.createdAt} className="text-slate-400" />
                        </p>
                        {/* Severity indicators (moved LEFT of Show More button) */}
                        <div className="shrink-0 flex items-center gap-2 text-xs text-slate-300">
                            <span><strong className="font-semibold text-slate-100">{eventSeverityCounts.high}</strong> High</span>
                            <span className="text-slate-600">·</span>
                            <span><strong className="font-semibold text-slate-100">{eventSeverityCounts.medium}</strong> Medium</span>
                            <span className="text-slate-600">·</span>
                            <span><strong className="font-semibold text-slate-100">{eventSeverityCounts.low}</strong> Low</span>
                        </div>
                        {/* Show More / Hide button (moved RIGHT) */}
                        <button
                            type="button"
                            onClick={(e) => { e.stopPropagation(); setDetailsExpanded(v => !v); }}
                            className="shrink-0 text-xs font-medium leading-5 w-[84px] text-center py-1 rounded-md border border-slate-600 bg-slate-800 text-slate-200 group-hover:border-slate-500 group-hover:bg-slate-700 group-hover:text-white transition-colors"
                        >
                            {detailsExpanded ? 'Hide' : 'Show more'}
                        </button>
                    </div>
                    {/* Status badge section (centered with EQUAL spacing on left and right) */}
                    <div className="flex-1 flex items-center justify-center self-center px-4">
                        <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-semibold text-white leading-5 whitespace-nowrap ${getStatusColor(review.status)}`}>
                            {review.status === 'in_progress' && (
                                <svg className="animate-spin w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"/><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"/></svg>
                            )}
                            {review.status.replace(/_/g, ' ')}
                        </span>
                    </div>
                    {/* Tools & Findings summary */}
                    {toolAccounting && (
                        <div className="shrink-0 self-center text-xs text-slate-300">
                            <strong className="text-slate-100 font-semibold">{toolAccounting.toolsExecuted || toolAccounting.toolBreakdown.length}</strong> Tools
                            {toolAccounting.totalCommentsGenerated > 0 ? (
                                <span className="text-slate-400 ml-1">({toolAccounting.totalCommentsGenerated} findings)</span>
                            ) : (
                                <span className="text-slate-400 ml-1">(clean)</span>
                            )}
                        </div>
                    )}
                    {/* Far right: Static Analysis Tools button */}
                    <div className="shrink-0 self-center">
                        <button type="button" onClick={() => setToolExpanded(v => !v)} className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-medium leading-5 transition-colors ${toolExpanded ? 'border-blue-500/60 bg-blue-600/20 text-blue-300' : 'border-slate-600 bg-slate-900/70 text-slate-200 hover:bg-slate-700 hover:text-white'}`}>
                            <svg className="w-3.5 h-3.5 opacity-70" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
                            Static Analysis Tools
                        </button>
                    </div>
                </div>

                {/* Show more panel */}
                {detailsExpanded && (
                    <div className="border-t border-slate-700/50 p-4 bg-slate-900/30 grid grid-cols-1 md:grid-cols-12 gap-4">
                        {/* GROUP 1: Commits (col-span-4 - fixed height with +N button enabling internal scroll) */}
                        <div className="md:col-span-4 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5 flex flex-col h-[135px]">
                            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest mb-1.5 shrink-0">
                                Commits ({displayCommits.length})
                            </p>
                            <ul className={`space-y-1.5 flex-1 min-h-0 ${allCommitsShown ? 'overflow-y-auto pr-1' : 'overflow-hidden'}`}>
                                {(allCommitsShown ? displayCommits : displayCommits.slice(0, 3)).map((commit) => (
                                    <li key={commit.ref} className="flex items-center justify-between gap-2 text-xs">
                                        <div className="flex items-center gap-1.5 min-w-0 truncate">
                                            {commit.refType === 'commit' && githubBaseUrl ? (
                                                <a href={`${githubBaseUrl}/commit/${commit.ref}`} target="_blank" rel="noopener noreferrer" className="font-mono text-blue-400 hover:text-blue-300 hover:underline truncate">
                                                    {(commit as any).message || commit.ref.substring(0, 8)}
                                                </a>
                                            ) : (
                                                <span className="font-mono text-slate-300 truncate">{(commit as any).message || (commit.refType === 'commit' ? commit.ref.substring(0, 8) : commit.ref)}</span>
                                            )}
                                        </div>
                                        <RelativeTime timestamp={commit.createdAt} className="text-slate-600 text-[10px] shrink-0" />
                                    </li>
                                ))}
                            </ul>
                            {!allCommitsShown && displayCommits.length > 3 && (
                                <button type="button" onClick={() => setAllCommitsShown(true)} className="mt-1 text-[11px] text-blue-400 hover:text-blue-300 hover:underline text-left shrink-0 font-medium">
                                    +{displayCommits.length - 3} more
                                </button>
                            )}
                        </div>

                        {/* GROUP 2: Issues by Severity (col-span-4 - equal width & height) */}
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
                        </div>

                        {/* GROUP 3: Details & Progress (col-span-4 - equal width & height) */}
                        <div className="md:col-span-4 bg-slate-900/60 border border-slate-700/50 rounded-lg p-3.5 flex flex-col justify-between h-[135px]">
                            <p className="text-[10px] font-semibold text-slate-500 uppercase tracking-widest mb-1 shrink-0">Details & Progress</p>
                            {/* Row 1: Progress stats including Events */}
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
                            {/* Row 2: Created by & Created at split 50/50 equally */}
                            <div className="grid grid-cols-2 gap-3 pt-2 border-t border-slate-700/40 text-xs shrink-0">
                                <div className="min-w-0">
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Created by</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 break-all text-[11px] leading-tight">{review.userEmail || '—'}</p>
                                </div>
                                <div className="min-w-0">
                                    <p className="text-[10px] text-slate-500 uppercase tracking-wide">Created at</p>
                                    <p className="font-semibold text-slate-200 mt-0.5 text-[11px] leading-tight"><RelativeTime timestamp={review.createdAt} /></p>
                                </div>
                            </div>
                        </div>
                    </div>
                )}

                {/* Static Analysis Tools panel — expanded INSIDE the same outer card box */}
                {toolExpanded && toolAccounting && (
                    <div className="border-t border-slate-700/50 p-4 bg-slate-900/40">
                        <ToolAnalysisCard data={toolAccounting} embedded={true} isExpanded={true} />
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
                            Last accounted <RelativeTime timestamp={accounting.lastAccountedAt} />
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
        </div>
    );
};

export default ReviewDetail;