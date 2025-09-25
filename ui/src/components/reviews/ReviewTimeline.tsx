import React, { useState, useEffect } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';

interface ReviewTimelineProps {
  reviewId: number;
  events: ReviewEvent[];
  isLive?: boolean;
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

export default function ReviewTimeline({ reviewId, events, isLive = false, className }: ReviewTimelineProps) {
  const [isAutoScrollEnabled, setIsAutoScrollEnabled] = useState(true);

  useEffect(() => {
    if (isLive && isAutoScrollEnabled && events.length > 0) {
      // Auto-scroll to bottom when new events arrive
      const timelineContainer = document.getElementById(`timeline-${reviewId}`);
      if (timelineContainer) {
        timelineContainer.scrollTop = timelineContainer.scrollHeight;
      }
    }
  }, [events, isLive, isAutoScrollEnabled, reviewId]);

  const getEventIcon = (eventType: ReviewEvent['eventType']) => {
    switch (eventType) {
      case 'started':
        return <Icons.Success />;
      case 'progress':
        return <div className="w-2 h-2 bg-blue-500 rounded-full animate-pulse"></div>;
      case 'batch_complete':
        return <Icons.Success />;
      case 'retry':
        return <Icons.Refresh />;
      case 'json_repair':
        return <span className="text-orange-400">⚡</span>;
      case 'timeout':
        return <Icons.Clock />;
      case 'error':
        return <Icons.Error />;
      case 'completed':
        return <Icons.Success />;
      default:
        return <Icons.Info />;
    }
  };

  const getSeverityColor = (severity: ReviewEvent['severity']) => {
    switch (severity) {
      case 'success':
        return 'text-green-400 border-green-600 bg-green-900/20';
      case 'warning':
        return 'text-yellow-400 border-yellow-600 bg-yellow-900/20';
      case 'error':
        return 'text-red-400 border-red-600 bg-red-900/20';
      case 'info':
      default:
        return 'text-blue-400 border-blue-600 bg-blue-900/20';
    }
  };

  const formatTimestamp = (timestamp: string) => {
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], { 
      hour: '2-digit', 
      minute: '2-digit', 
      second: '2-digit',
      hour12: false 
    });
  };

  const getRelativeTime = (timestamp: string) => {
    const now = new Date();
    const eventTime = new Date(timestamp);
    const diffMs = now.getTime() - eventTime.getTime();
    const diffSeconds = Math.floor(diffMs / 1000);
    const diffMinutes = Math.floor(diffSeconds / 60);
    const diffHours = Math.floor(diffMinutes / 60);

    if (diffSeconds < 60) return 'just now';
    if (diffMinutes < 60) return `${diffMinutes}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    return eventTime.toLocaleDateString();
  };



  return (
    <div className={`space-y-4 ${className}`}>
      {/* Header with filters */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-white">Review Timeline</h3>
          <p className="text-sm text-slate-400">
            {isLive ? 'Live updates enabled' : `${events.length} events`}
          </p>
        </div>
        
        <div className="flex items-center gap-2">
          {isLive && (
            <button
              onClick={() => setIsAutoScrollEnabled(!isAutoScrollEnabled)}
              className={`px-3 py-1 text-xs rounded-full border transition-colors ${
                isAutoScrollEnabled
                  ? 'bg-blue-600 text-white border-blue-600'
                  : 'bg-transparent text-slate-400 border-slate-600 hover:border-slate-500'
              }`}
            >
              Auto-scroll {isAutoScrollEnabled ? 'ON' : 'OFF'}
            </button>
          )}
        </div>
      </div>



      {/* Timeline */}
      <Card className="p-0 overflow-hidden">
        <div 
          id={`timeline-${reviewId}`}
        >
          {events.length === 0 ? (
            <div className="p-8 text-center text-slate-400">
              <Icons.EmptyState />
              <p className="mt-2">No events to display</p>
            </div>
          ) : (
            <div className="relative">
              {/* Timeline line */}
              <div className="absolute left-8 top-0 bottom-0 w-0.5 bg-slate-700"></div>
              
              {events.map((event, index) => (
                <div key={event.id} className="relative flex items-start gap-4 p-4 hover:bg-slate-800/30 transition-colors">
                  {/* Timeline dot */}
                  <div className={`
                    relative z-10 w-6 h-6 rounded-full flex items-center justify-center border-2 
                    ${getSeverityColor(event.severity)} 
                    ${index === 0 && isLive ? 'animate-pulse' : ''}
                  `}>
                    {getEventIcon(event.eventType)}
                  </div>
                  
                  {/* Event content */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-start justify-between gap-2 mb-1">
                      <div className="flex items-center gap-2 min-w-0 flex-1">
                        <span className="text-sm font-medium text-white break-words">
                          {event.message}
                        </span>
                        {event.details?.batchId && (
                          <Badge variant="default" size="sm">
                            {event.details.batchId}
                          </Badge>
                        )}
                      </div>
                      <div className="flex items-center gap-2 text-xs text-slate-400 shrink-0">
                        <span>{formatTimestamp(event.timestamp)}</span>
                        <span>•</span>
                        <span>{getRelativeTime(event.timestamp)}</span>
                      </div>
                    </div>
                    
                    {/* Event details */}
                    {event.details && (
                      <div className="mt-2 text-sm text-slate-300 space-y-1">
                        {event.details.filename && (
                          <div className="font-mono text-xs text-slate-400">
                            File: {event.details.filename}
                          </div>
                        )}
                        
                        {event.details.attempt && (
                          <div className="flex items-center gap-4 text-xs">
                            <span>Attempt: {event.details.attempt}</span>
                            {event.details.delay && <span>Delay: {event.details.delay}</span>}
                          </div>
                        )}
                        
                        {event.details.responseTime && (
                          <div className="text-xs">
                            Response Time: {event.details.responseTime}
                          </div>
                        )}
                        
                        {event.details.repairStats && (
                          <div className="bg-slate-800/50 p-2 rounded text-xs space-y-1">
                            <div className="font-medium text-orange-400">JSON Repair Applied</div>
                            <div className="grid grid-cols-2 gap-2 text-slate-400">
                              <span>Size: {event.details.repairStats.originalSize} → {event.details.repairStats.repairedSize}</span>
                              <span>Time: {event.details.repairStats.repairTime}</span>
                              <span>Comments lost: {event.details.repairStats.commentsLost}</span>
                              <span>Fields recovered: {event.details.repairStats.fieldsRecovered}</span>
                            </div>
                          </div>
                        )}
                        
                        {event.details.errorMessage && (
                          <div className="bg-red-900/20 border border-red-700/30 p-2 rounded text-xs text-red-300 break-words">
                            {event.details.errorMessage}
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
      </Card>

      {/* Live status indicator */}
      {isLive && (
        <div className="flex items-center justify-center gap-2 text-xs text-slate-400">
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
          <span>Live monitoring active</span>
        </div>
      )}
    </div>
  );
}