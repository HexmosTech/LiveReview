import React, { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { Button, Icons } from '../../components/UIPrimitives';
import { 
  getReview, 
  getReviewEvents, 
  getReviewSummary, 
  formatRelativeTime, 
  getStatusColor, 
  getStatusText 
} from '../../api/reviews';
import { 
  Review, 
  ReviewEvent, 
  ReviewSummary, 
  ReviewEventLevel,
  ReviewEventType 
} from '../../types/reviews';

const ReviewDetail: React.FC = () => {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const [review, setReview] = useState<Review | null>(null);
    const [events, setEvents] = useState<ReviewEvent[]>([]);
    const [summary, setSummary] = useState<ReviewSummary | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [pollingEnabled, setPollingEnabled] = useState(true);
    const [levelFilter, setLevelFilter] = useState<ReviewEventLevel | ''>('');
    const [typeFilter, setTypeFilter] = useState<ReviewEventType | ''>('');
    const [lastEventTime, setLastEventTime] = useState<string | null>(null);
    const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);
        const timelineRef = useRef<HTMLDivElement | null>(null);
        const scrollStateRef = useRef({ prevScrollTop: 0, prevScrollHeight: 0, atBottom: true });

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

    // Fetch review details
    const fetchReviewDetails = useCallback(async (isPolling = false) => {
        if (!id) return;
        try {
            if (!isPolling) {
                setLoading(true);
                setError(null);
            }
            
            const reviewId = parseInt(id, 10);
            if (isNaN(reviewId)) {
                throw new Error('Invalid review ID');
            }
            
            // Fetch review info, events, and summary in parallel
            const [reviewData, eventsData, summaryData] = await Promise.all([
                getReview(reviewId),
                // Only use the since cursor when we are polling; full refresh otherwise
                getReviewEvents(reviewId, isPolling ? (lastEventTime || undefined) : undefined),
                getReviewSummary(reviewId)
            ]);

            setReview(reviewData);
            setSummary(summaryData);
            
                        // Handle events update
                        // Capture scroll state BEFORE updating the list so we can restore after DOM updates
                        const container = timelineRef.current;
                        const prevScrollTop = container?.scrollTop ?? 0;
                        const prevScrollHeight = container?.scrollHeight ?? 0;
                        const atBottom = container
                            ? prevScrollHeight - prevScrollTop - (container.clientHeight ?? 0) < 24
                            : true;
                        scrollStateRef.current = { prevScrollTop, prevScrollHeight, atBottom };
            const newEvents = (eventsData?.events as ReviewEvent[] | undefined) || [];
            if (isPolling && lastEventTime) {
                // Append new events to existing ones
                setEvents(prev => [...prev, ...newEvents]);
            } else {
                // Replace all events on initial/non-poll refresh
                setEvents(newEvents);
            }
            
            // Update last event time for next polling
            if (newEvents.length > 0) {
                const latestTime = newEvents[newEvents.length - 1].time;
                setLastEventTime(latestTime);
            }

        } catch (err) {
            console.error('Error fetching review details:', err);
            if (!isPolling) {
                setError(err instanceof Error ? err.message : 'Failed to fetch review details');
            }
        } finally {
            if (!isPolling) {
                setLoading(false);
            }
        }
    }, [id, lastEventTime]);

    // After events update, restore scroll position or stick to bottom if user was at bottom
    useEffect(() => {
        const container = timelineRef.current;
        if (!container) return;
        const { prevScrollTop, prevScrollHeight, atBottom } = scrollStateRef.current;
        if (atBottom) {
            container.scrollTop = container.scrollHeight; // stick to bottom
        } else {
            const delta = container.scrollHeight - prevScrollHeight;
            container.scrollTop = Math.max(0, prevScrollTop + delta);
        }
    }, [events]);

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
        fetchReviewDetails(false);
    }, [fetchReviewDetails]);
    
    // Setup polling for active reviews
    useEffect(() => {
        if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current);
            pollingIntervalRef.current = null;
        }
        
        if (pollingEnabled && review && ['created', 'in_progress'].includes(review.status)) {
            pollingIntervalRef.current = setInterval(() => {
                fetchReviewDetails(true);
            }, 10000); // Poll every 10 seconds (reduce load on tunnels)
        }

        return () => {
            if (pollingIntervalRef.current) {
                clearInterval(pollingIntervalRef.current);
                pollingIntervalRef.current = null;
            }
        };
    }, [pollingEnabled, review?.status, fetchReviewDetails]);

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
                            onClick={() => fetchReviewDetails(false)}
                            variant="primary"
                        >
                            Retry
                        </Button>
                    </div>
                </div>
            </div>
        );
    }

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
                    <Button
                        variant="outline"
                        onClick={() => setPollingEnabled(!pollingEnabled)}
                        className={pollingEnabled ? 'border-green-500 text-green-400' : 'border-slate-600 text-slate-300'}
                    >
                        {pollingEnabled ? 'Polling On' : 'Polling Off'}
                    </Button>
                </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                {/* Review Info */}
                <div className="lg:col-span-1">
                    <div className="bg-slate-800 rounded-lg p-6 border border-slate-700">
                        <h3 className="text-lg font-semibold text-white mb-4">Review Information</h3>
                        <div className="space-y-3">
                            <div>
                                <span className="text-slate-400 text-sm">Repository:</span>
                                <div className="text-white font-mono text-sm break-all">{review.repository}</div>
                            </div>
                            {review.userEmail && (
                                <div>
                                    <span className="text-slate-400 text-sm">User:</span>
                                    <div className="text-white">{review.userEmail}</div>
                                </div>
                            )}
                            {review.commitHash && (
                                <div>
                                    <span className="text-slate-400 text-sm">Commit:</span>
                                    <div className="text-white font-mono text-sm">{review.commitHash.substring(0, 8)}</div>
                                </div>
                            )}
                            {review.provider && (
                                <div>
                                    <span className="text-slate-400 text-sm">Provider:</span>
                                    <div className="text-white capitalize">{review.provider}</div>
                                </div>
                            )}
                            <div>
                                <span className="text-slate-400 text-sm">Created:</span>
                                <div className="text-white">{new Date(review.createdAt).toLocaleString()}</div>
                            </div>
                            <div>
                                <span className="text-slate-400 text-sm">Last Activity:</span>
                                <div className="text-white">{new Date(review.completedAt || review.startedAt || review.createdAt).toLocaleString()}</div>
                            </div>
                        </div>

                                {summary && (
                            <div className="mt-6 pt-6 border-t border-slate-700">
                                <h4 className="text-sm font-semibold text-white mb-3">Summary</h4>
                                <div className="space-y-2">
                                    <div className="flex justify-between">
                                        <span className="text-slate-400 text-sm">Total Events:</span>
                                                <span className="text-white text-sm">{Object.values(summary.eventCounts || {}).reduce((a: number, b: number) => a + b, 0)}</span>
                                    </div>
                                    <div className="flex justify-between">
                                        <span className="text-slate-400 text-sm">Batches:</span>
                                        <span className="text-white text-sm">{summary.batchCount}</span>
                                    </div>
                                </div>
                            </div>
                        )}
                    </div>
                </div>

                {/* Events Timeline */}
                <div className="lg:col-span-2">
                    <div className="bg-slate-800 rounded-lg p-6 border border-slate-700">
                        <div className="flex items-center justify-between mb-4">
                            <h3 className="text-lg font-semibold text-white">Event Timeline</h3>
                            <div className="flex items-center space-x-2">
                                {/* Event type filter */}
                                <select
                                    value={typeFilter}
                                    onChange={(e) => setTypeFilter(e.target.value as ReviewEventType | '')}
                                    className="px-3 py-1 bg-slate-700 border border-slate-600 rounded text-sm text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                                >
                                    <option value="">All Types</option>
                                    {presentTypes.has('status') && <option value="status">Status</option>}
                                    {presentTypes.has('log') && <option value="log">Logs</option>}
                                    {presentTypes.has('batch') && <option value="batch">Batches</option>}
                                    {presentTypes.has('artifact') && <option value="artifact">Artifacts</option>}
                                    {presentTypes.has('completion') && <option value="completion">Completion</option>}
                                </select>
                                
                                {/* Event level filter */}
                                <select
                                    value={levelFilter}
                                    onChange={(e) => setLevelFilter(e.target.value as ReviewEventLevel | '')}
                                    className="px-3 py-1 bg-slate-700 border border-slate-600 rounded text-sm text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                                >
                                    <option value="">All Levels</option>
                                    {presentLevels.has('info') && <option value="info">Info</option>}
                                    {presentLevels.has('warn') && <option value="warn">Warning</option>}
                                    {presentLevels.has('error') && <option value="error">Error</option>}
                                    {presentLevels.has('debug') && <option value="debug">Debug</option>}
                                </select>
                            </div>
                        </div>
                        
                        {events.length === 0 ? (
                            <div className="text-center py-8">
                                <Icons.Info />
                                <p className="text-slate-400 mt-2">No events yet</p>
                                {pollingEnabled && review?.status === 'in_progress' && (
                                    <p className="text-slate-500 text-sm mt-1">Events will appear here as the review progresses...</p>
                                )}
                            </div>
                        ) : (
                            <div className="space-y-3 max-h-96 overflow-y-auto" ref={timelineRef}>
                                {events
                                    .filter(event => {
                                        if (typeFilter && event.type !== typeFilter) return false;
                                        if (levelFilter && event.level !== levelFilter) return false;
                                        return true;
                                    })
                                    .slice()
                                    .reverse()
                                    .map((event) => (
                                        <div key={event.id} className="flex items-start space-x-3 p-3 bg-slate-700/50 rounded-lg hover:bg-slate-700/70 transition-colors">
                                            <div className="flex-shrink-0 mt-1">
                                                {getEventIcon(event.type, event.level)}
                                            </div>
                                            <div className="flex-grow min-w-0">
                                                <div className="flex items-center justify-between">
                                                    <div className="flex items-center space-x-2">
                                                        <span className="text-white font-medium capitalize">{event.type}</span>
                                                        {event.batchId && (
                                                            <span className="text-xs px-2 py-1 rounded bg-blue-600 text-white">
                                                                {event.batchId}
                                                            </span>
                                                        )}
                                                        {event.level && (
                                                            <span className={`text-xs px-2 py-1 rounded ${
                                                                event.level === 'error' ? 'bg-red-600 text-white' :
                                                                event.level === 'warn' ? 'bg-yellow-600 text-white' :
                                                                event.level === 'debug' ? 'bg-purple-600 text-white' :
                                                                'bg-slate-600 text-slate-300'
                                                            }`}>
                                                                {event.level.toUpperCase()}
                                                            </span>
                                                        )}
                                                    </div>
                                                    <div className="flex items-center space-x-2">
                                                        <span className="text-slate-400 text-xs">
                                                            {formatRelativeTime(event.time)}
                                                        </span>
                                                        <span className="text-slate-500 text-xs">
                                                            {new Date(event.time).toLocaleTimeString()}
                                                        </span>
                                                    </div>
                                                </div>
                                                <div className="text-slate-300 text-sm mt-1">
                                                    {formatEventData(event)}
                                                </div>
                                                
                                                {/* Show artifact previews if available */}
                                                {event.type === 'artifact' && (event.data.previewHead || event.data.previewTail) && (
                                                    <div className="mt-2 p-2 bg-slate-800 rounded text-xs">
                                                        {event.data.previewHead && (
                                                            <div className="text-slate-400 mb-1">
                                                                <span className="font-medium">Preview:</span>
                                                                <pre className="mt-1 text-slate-300 whitespace-pre-wrap">{event.data.previewHead}</pre>
                                                            </div>
                                                        )}
                                                        {event.data.previewTail && event.data.previewHead !== event.data.previewTail && (
                                                            <div className="text-slate-400">
                                                                <span className="font-medium">...</span>
                                                                <pre className="mt-1 text-slate-300 whitespace-pre-wrap">{event.data.previewTail}</pre>
                                                            </div>
                                                        )}
                                                        {event.data.url && (
                                                            <div className="mt-2">
                                                                <a 
                                                                    href={event.data.url} 
                                                                    target="_blank" 
                                                                    rel="noopener noreferrer"
                                                                    className="text-blue-400 hover:text-blue-300 text-xs"
                                                                >
                                                                    View Full {event.data.kind || 'Artifact'}
                                                                </a>
                                                            </div>
                                                        )}
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    ))}
                            </div>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
};

export default ReviewDetail;