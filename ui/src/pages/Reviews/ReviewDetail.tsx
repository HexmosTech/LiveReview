import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { Button, Icons } from '../../components/UIPrimitives';
import { ReviewEventsPage } from '../../components/reviews';
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
  ReviewEventLevel,
  ReviewEventType 
} from '../../types/reviews';

const ACCOUNTING_REFRESH_INTERVAL_MS = 15000;

const hasAccountingDetails = (value: ReviewAccounting | null): boolean => {
        if (!value) {
                return false;
        }

        return value.accountedOperations > 0 ||
                value.totalBillableLoc > 0 ||
                value.tokenTrackedOperations > 0 ||
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

    const aiExecutionMode = typeof review.metadata?.ai_execution_mode === 'string' ? review.metadata.ai_execution_mode : '';
    const aiExecutionSource = typeof review.metadata?.ai_execution_source === 'string' ? review.metadata.ai_execution_source : '';
    const aiExecutionProvider = typeof review.metadata?.ai_provider_name === 'string' ? review.metadata.ai_provider_name : '';
    const aiExecutionConnector = typeof review.metadata?.ai_connector_name === 'string' ? review.metadata.ai_connector_name : '';

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

            {/* Review Info Panel - Compact */}
            <div className="bg-slate-800 rounded-lg p-3 border border-slate-700 mb-6">
                <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
                    <div className="flex items-center gap-2">
                        <span className="text-slate-400">Repository:</span>
                        <span className="text-white font-mono text-xs break-all">{review.repository}</span>
                    </div>
                    {review.provider && (
                        <div className="flex items-center gap-2">
                            <span className="text-slate-400">Provider:</span>
                            <span className="text-white capitalize">{review.provider}</span>
                        </div>
                    )}
                    {review.userEmail && (
                        <div className="flex items-center gap-2">
                            <span className="text-slate-400">User:</span>
                            <span className="text-white">{review.userEmail}</span>
                        </div>
                    )}
                    {review.commitHash && (
                        <div className="flex items-center gap-2">
                            <span className="text-slate-400">Commit:</span>
                            <span className="text-white font-mono text-xs">{review.commitHash.substring(0, 8)}</span>
                        </div>
                    )}
                    <div className="flex items-center gap-2">
                        <span className="text-slate-400">Created:</span>
                        <span className="text-white text-xs">{new Date(review.createdAt).toLocaleString()}</span>
                    </div>
                    <div className="flex items-center gap-2">
                        <span className="text-slate-400">Last Activity:</span>
                        <span className="text-white text-xs">{new Date(review.completedAt || review.startedAt || review.createdAt).toLocaleString()}</span>
                    </div>
                    {summary && (
                        <>
                            <div className="flex items-center gap-2">
                                <span className="text-slate-400">Events:</span>
                                <span className="text-white text-xs">{Object.values(summary.eventCounts || {}).reduce((a: number, b: number) => a + b, 0)}</span>
                            </div>
                            <div className="flex items-center gap-2">
                                <span className="text-slate-400">Batches:</span>
                                <span className="text-white text-xs">{summary.batchCount}</span>
                            </div>
                        </>
                    )}
                </div>
            </div>

            {/* Accounting Panel */}
            <div className="bg-slate-800 rounded-lg p-4 border border-slate-700 mb-6">
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
                {accounting?.latestOperation && (
                    <div className="bg-slate-900 rounded-md p-3 border border-slate-700 text-xs">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-y-2 gap-x-4">
                            <p className="text-slate-300"><span className="text-slate-500">Latest operation:</span> {accounting.latestOperation.operationType}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Trigger:</span> {accounting.latestOperation.triggerSource}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Provider/Model:</span> {(accounting.latestOperation.provider || 'unknown')} / {(accounting.latestOperation.model || 'unknown')}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Pricing version:</span> {accounting.latestOperation.pricingVersion || 'unknown'}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Operation ID:</span> {accounting.latestOperation.operationId}</p>
                            <p className="text-slate-300"><span className="text-slate-500">Idempotency key:</span> {accounting.latestOperation.idempotencyKey}</p>
                            {(aiExecutionMode || aiExecutionSource) && (
                                <p className="text-slate-300"><span className="text-slate-500">AI execution:</span> {(aiExecutionMode || 'unknown')} via {(aiExecutionSource || 'unknown')}</p>
                            )}
                            {(aiExecutionProvider || aiExecutionConnector) && (
                                <p className="text-slate-300"><span className="text-slate-500">AI route:</span> {(aiExecutionProvider || 'unknown')} / {(aiExecutionConnector || 'unknown')}</p>
                            )}
                        </div>
                    </div>
                )}
            </div>

            {/* Events Timeline - Full Width */}
            <div>
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