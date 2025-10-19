import React, { useState, useEffect, useRef } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';
import ReviewProgressView from './ReviewProgressView';
import ReviewTimeline from './ReviewTimeline';
import { getReviewEvents } from '../../api/reviews';

interface ReviewEventsPageProps {
  reviewId: number;
  initialEvents: ReviewEvent[];
  isLive?: boolean;
  pollingInterval?: number;
  className?: string;
}

interface ReviewEvent {
  id: string;
  timestamp: string;
  eventType: 'started' | 'progress' | 'batch_complete' | 'retry' | 'json_repair' | 'timeout' | 'error' | 'completed';
  message: string;
  details?: {
    batchId?: string;
    filename?: string;
    attempt?: number;
    delay?: string;
    responseTime?: string;
    errorMessage?: string;
    repairStats?: {
      originalSize: number;
      repairedSize: number;
      commentsLost: number;
      fieldsRecovered: number;
      repairTime: string;
    };
  };
  severity: 'info' | 'success' | 'warning' | 'error';
}

type ViewMode = 'progress' | 'raw';

export default function ReviewEventsPage({ 
  reviewId, 
  initialEvents, 
  isLive = false, 
  pollingInterval = 30000, // 30 seconds instead of 2 seconds
  className 
}: ReviewEventsPageProps) {
  const [currentView, setCurrentView] = useState<ViewMode>('progress');
  const [events, setEvents] = useState<ReviewEvent[]>(initialEvents);
  const [isPolling, setIsPolling] = useState(true); // Default to ON
  const [lastEventCount, setLastEventCount] = useState(initialEvents.length);
  const scrollPositionRef = useRef<number>(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const pollingTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Save scroll position before updates
  const saveScrollPosition = () => {
    if (containerRef.current) {
      scrollPositionRef.current = containerRef.current.scrollTop;
    }
  };

  // Restore scroll position after updates
  const restoreScrollPosition = () => {
    if (containerRef.current && currentView === 'raw') {
      // For raw view, maintain scroll position
      containerRef.current.scrollTop = scrollPositionRef.current;
    }
  };

  // Smooth append for new events (no position disruption)
  const appendNewEvents = (newEvents: ReviewEvent[]) => {
    if (newEvents.length > events.length) {
      const addedEvents = newEvents.slice(events.length);
      
      // Save position before update
      saveScrollPosition();
      
      // Update events
      setEvents(newEvents);
      setLastEventCount(newEvents.length);
      
      // Restore position after React update
      setTimeout(restoreScrollPosition, 0);
      
      // Auto-scroll to bottom if user was already at bottom (raw view only)
      if (currentView === 'raw' && containerRef.current) {
        const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
        const isAtBottom = scrollTop + clientHeight >= scrollHeight - 100;
        
        if (isAtBottom) {
          setTimeout(() => {
            if (containerRef.current) {
              containerRef.current.scrollTop = containerRef.current.scrollHeight;
            }
          }, 100);
        }
      }
    }
  };

  // Polling mechanism with smooth updates
  const pollForUpdates = async () => {
    if (!isPolling) {
      console.log(`[ReviewEventsPage] Polling stopped`);
      return;
    }

    console.log(`[ReviewEventsPage] Polling for events... (next poll in ${pollingInterval}ms)`);
    try {
      // Fetch events from the backend API using existing API function
      // Request up to 1000 events to ensure we get stage completion events
      const data = await getReviewEvents(reviewId, undefined, 1000);
      // Transform backend events to frontend format  
      const backendEvents = data.events || [];
      const newEvents: ReviewEvent[] = backendEvents.map((event: any) => {
        // Generate message based on event type and data
        let message = '';
        const eventData = event.data || {};
        
        switch (event.type) {
          case 'log':
            message = eventData.message || '';
            break;
          case 'batch':
            // Generate message for batch events based on status
            if (eventData.status === 'processing') {
              const fileCount = eventData.fileCount || 0;
              message = `Batch ${event.batchId || 'unknown'} started: processing ${fileCount} file${fileCount !== 1 ? 's' : ''}`;
            } else if (eventData.status === 'completed') {
              const fileCount = eventData.fileCount || 0;
              message = `Batch ${event.batchId || 'unknown'} completed: generated ${fileCount} comment${fileCount !== 1 ? 's' : ''}`;
            } else {
              message = `Batch ${event.batchId || 'unknown'}: ${eventData.status || 'unknown status'}`;
            }
            break;
          case 'status':
            message = `Status: ${eventData.status || 'unknown'}`;
            break;
          case 'artifact':
            message = eventData.url ? `Generated: ${eventData.kind || 'artifact'}` : `Artifact: ${eventData.kind || 'unknown'}`;
            break;
          case 'completion':
            const commentCount = eventData.commentCount || 0;
            message = eventData.resultSummary || `Process completed with ${commentCount} comment${commentCount !== 1 ? 's' : ''}`;
            break;
          default:
            message = JSON.stringify(eventData);
        }
        
        return {
          id: event.id.toString(),
          timestamp: event.time,
          eventType: event.type === 'log' ? 
            (event.level === 'success' ? 'completed' : 'started') : 
            event.type,
          message: message,
          severity: event.level || 'info',
          details: {
            ...eventData,
            batchId: event.batchId || event.batch_id  // Include batch ID from database
          }
        };
      });
      
      console.log('[ReviewEventsPage] Received events:', {
        totalEvents: newEvents.length,
        sampleEvents: newEvents.slice(0, 10).map(e => ({ 
          message: e.message, 
          eventType: e.eventType,
          severity: e.severity,
          timestamp: e.timestamp 
        })),
        stageCompletionEvents: newEvents.filter(e => 
          e.message.toLowerCase().includes('stage completed successfully')
        ).map(e => e.message)
      });
      
      appendNewEvents(newEvents);
    } catch (error) {
      console.error('Failed to poll for updates:', error);
    }

    // Schedule next poll
    if (isPolling) {
      console.log(`[ReviewEventsPage] Scheduling next poll in ${pollingInterval}ms`);
      pollingTimeoutRef.current = setTimeout(pollForUpdates, pollingInterval);
    } else {
      console.log(`[ReviewEventsPage] Not scheduling next poll - polling is disabled`);
    }
  };

  useEffect(() => {
    console.log(`[ReviewEventsPage] Polling effect triggered. isPolling: ${isPolling}, reviewId: ${reviewId}`);
    
    if (isPolling) {
      console.log(`[ReviewEventsPage] Starting polling with ${pollingInterval}ms interval`);
      pollForUpdates();
    }

    return () => {
      console.log(`[ReviewEventsPage] Cleaning up polling timeout`);
      if (pollingTimeoutRef.current) {
        clearTimeout(pollingTimeoutRef.current);
        pollingTimeoutRef.current = null;
      }
    };
  }, [isPolling]); // Remove reviewId dependency to prevent restarts

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollingTimeoutRef.current) {
        clearTimeout(pollingTimeoutRef.current);
      }
    };
  }, []);

  const togglePolling = () => {
    setIsPolling(!isPolling);
  };

  const newEventsCount = events.length - lastEventCount;

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with tabs and controls */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex space-x-1">
          <button
            onClick={() => setCurrentView('progress')}
            className={`px-4 py-2 rounded-lg font-medium transition-colors ${
              currentView === 'progress'
                ? 'bg-blue-600 text-white'
                : 'bg-slate-700 text-slate-300 hover:bg-slate-600'
            }`}
          >
            <div className="w-4 h-4 inline-block mr-2">
              <Icons.Dashboard />
            </div>
            Progress
          </button>
          <button
            onClick={() => setCurrentView('raw')}
            className={`px-4 py-2 rounded-lg font-medium transition-colors ${
              currentView === 'raw'
                ? 'bg-blue-600 text-white'
                : 'bg-slate-700 text-slate-300 hover:bg-slate-600'
            }`}
          >
            <div className="w-4 h-4 inline-block mr-2">
              <Icons.List />
            </div>
            Raw Events ({events.length})
          </button>
        </div>

        {/* Single consolidated control */}
        <div className="flex items-center space-x-4">
          {newEventsCount > 0 && (
            <span className="text-sm text-blue-400">
              +{newEventsCount} new events
            </span>
          )}
          <span className="text-xs text-slate-400">
            Updates every 30s
          </span>
          <button
            onClick={togglePolling}
            className={`px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
              isPolling
                ? 'bg-green-600 text-white hover:bg-green-700'
                : 'bg-slate-600 text-slate-300 hover:bg-slate-500'
            }`}
          >
            {isPolling ? 'Live Updates On' : 'Live Updates Off'}
          </button>
        </div>
      </div>

      {/* Content area without inner scroll - use page scroll */}
      <div ref={containerRef}>
        {currentView === 'progress' ? (
          <ReviewProgressView
            reviewId={reviewId}
            events={events}
            isLive={isPolling}
          />
        ) : (
          <ReviewTimeline
            reviewId={reviewId}
            events={events}
            isLive={isPolling}
          />
        )}
      </div>

      {/* Status bar */}
      <div className="flex items-center justify-between text-xs text-slate-500 border-t border-slate-700 pt-3">
        <div className="flex items-center gap-4">
          <span>Review ID: {reviewId}</span>
          <span>View: {currentView === 'progress' ? 'Progress Stages' : 'Raw Event Log'}</span>
        </div>
        
        <div className="flex items-center gap-4">
          {isPolling && (
            <span className="text-green-400">● Auto-updating every {pollingInterval/1000}s</span>
          )}
          <span>Last updated: {new Date().toLocaleTimeString()}</span>
        </div>
      </div>
    </div>
  );
}