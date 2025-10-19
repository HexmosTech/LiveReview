import React, { useState, useEffect } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';

interface ReviewProgressViewProps {
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

interface Stage {
  id: string;
  title: string;
  description: string;
  status: 'pending' | 'in-progress' | 'completed' | 'failed';
  startTime?: string;
  endTime?: string;
  substages: Substage[];
  events: ReviewEvent[];
}

interface Substage {
  id: string;
  title: string;
  status: 'pending' | 'in-progress' | 'completed' | 'failed';
  count?: { current: number; total: number };
  startTime?: string;
  endTime?: string;
  events: ReviewEvent[];
  resiliencyEvents: ResiliencyEvent[];
}

interface ResiliencyEvent {
  type: 'retry' | 'json_repair' | 'timeout' | 'circuit_breaker';
  attempt?: number;
  details: string;
  resolved: boolean;
}

export default function ReviewProgressView({ reviewId, events, isLive = false, className }: ReviewProgressViewProps) {
  const [stages, setStages] = useState<Stage[]>([]);
  const [expandedStages, setExpandedStages] = useState<Set<string>>(new Set(['stage-4'])); // Default expand "Processing Batches"
  const [expandedEvents, setExpandedEvents] = useState<Set<string>>(new Set()); // Track which stages have expanded events

  useEffect(() => {
    const processedStages = processEventsIntoStages(events);
    
    // Debug logging to help diagnose categorization issues
    if (process.env.NODE_ENV === 'development') {
      console.log('[ReviewProgressView] Processing events:', {
        totalEvents: events.length,
        stageBreakdown: processedStages.map(s => ({
          title: s.title,
          status: s.status,
          eventCount: s.events.length,
          sampleEvents: s.events.slice(0, 3).map(e => e.message)
        }))
      });
    }
    
    setStages(processedStages);
  }, [events]);

  const processEventsIntoStages = (events: ReviewEvent[]): Stage[] => {
    // Initialize the 5 main stages
    const stageTemplates: Omit<Stage, 'events' | 'substages'>[] = [
      {
        id: 'stage-1',
        title: 'Preparation',
        description: 'Initializing providers and configuration',
        status: 'pending'
      },
      {
        id: 'stage-2', 
        title: 'Analysis',
        description: 'Fetching MR details and analyzing changed files',
        status: 'pending'
      },
      {
        id: 'stage-3',
        title: 'Review',
        description: 'AI processing and generating review comments',
        status: 'pending'
      },
      {
        id: 'stage-4',
        title: 'Artifact Generation',
        description: 'Posting review comments to merge request',
        status: 'pending'
      },
      {
        id: 'stage-5',
        title: 'Finalization',
        description: 'Finalizing review and cleanup',
        status: 'pending'
      }
    ];

    // Process events and map them to stages
    const processedStages: Stage[] = stageTemplates.map(template => ({
      ...template,
      events: [] as ReviewEvent[],
      substages: [] as Substage[]
    }));

    // Enhanced event mapping logic using eventType and message content
    events.forEach((event, index) => {
      const message = (event.message || '').toLowerCase();
      let targetStage = -1;
      
      // Debug logging for stage mapping
      if (process.env.NODE_ENV === 'development' && index < 5) {
        console.log(`[Event ${index}] Mapping:`, {
          message: event.message,
          eventType: event.eventType,
          severity: event.severity,
          messageLC: message
        });
      }
      
            // Priority 1: Explicit stage events (most reliable)
      if (message.includes('stage started') && message.includes('preparation') || 
          message.includes('stage completed') && message.includes('preparation')) {
        targetStage = 0;
      }
      else if (message.includes('stage started') && message.includes('analysis') || 
               message.includes('stage completed') && message.includes('analysis')) {
        targetStage = 1;
      }
      else if (message.includes('stage started') && message.includes('review') || 
               message.includes('stage completed') && message.includes('review')) {
        targetStage = 2;
      }
      else if (message.includes('stage started') && message.includes('artifact generation') || 
               message.includes('stage completed') && message.includes('artifact generation') ||
               message.includes('posted') && message.includes('comments')) {
        targetStage = 3;
      }
      else if (message.includes('stage started') && message.includes('finalization') || 
               message.includes('stage completed') && message.includes('finalization') ||
               message.includes('review process completed')) {
        targetStage = 4;
      }
      // Priority 2: Implicit stage mapping based on content
      // Stage 1: Preparation
      else if (event.eventType === 'started' || 
          message.includes('preparation') || message.includes('provider') || 
          message.includes('initializ') || message.includes('configuration')) {
        targetStage = 0;
      }
      // Stage 2: Analysis  
      else if (message.includes('analysis') || message.includes('fetch') ||
               message.includes('mr details') || message.includes('merge request') ||
               message.includes('chang') || message.includes('file') ||
               message.includes('retriev')) {
        targetStage = 1;
      }
      // Stage 3: Review
      else if (message.includes('ai') || message.includes('llm') ||
               message.includes('generat') || 
               message.includes('batch') && !message.includes('completed') && !message.includes('generated') ||
               message.includes('review') ||
               event.eventType === 'retry' || event.eventType === 'json_repair') {
        targetStage = 2;
      }
      // Stage 4: Artifact Generation (batch completion and comment posting)
      else if (message.includes('post') || 
               message.includes('batch') && (message.includes('completed') || message.includes('generated')) ||
               message.includes('comment') && (message.includes('post') || message.includes('generated')) ||
               message.includes('artifact') || message.includes('result') ||
               event.eventType === 'batch_complete' || event.eventType === 'batch' ||
               message.includes('merge request') && message.includes('comment')) {
        targetStage = 3;
      }
      // Stage 5: Finalization
      else if (event.eventType === 'completed' ||
               message.includes('finalization') || message.includes('finaliz') ||
               message.includes('cleanup') || message.includes('review process completed')) {
        targetStage = 4;
      }
      // Default to review stage for unclear events
      else {
        targetStage = 2;
      }
      
      if (targetStage >= 0) {
        processedStages[targetStage].events.push(event);
        if (processedStages[targetStage].status === 'pending') {
          processedStages[targetStage].status = 'in-progress';
          processedStages[targetStage].startTime = event.timestamp;
        }
        
        // Debug logging for assignments  
        if (process.env.NODE_ENV === 'development' && (targetStage === 3 || targetStage === 4)) {
          console.log(`[Stage Assignment] Event "${event.message}" → Stage ${targetStage} (${processedStages[targetStage].title})`);
        }
      } else if (process.env.NODE_ENV === 'development') {
        console.log(`[Stage Assignment] Event "${event.message}" → NO STAGE (defaulted to stage 2)`);
      }
    });

    // Create substages for batch processing
    if (processedStages[3].events.length > 0) {
      processedStages[3].substages = createBatchSubstages(processedStages[3].events);
    }

    // Update stage statuses based on events and progression logic
    processedStages.forEach((stage, index) => {
      if (stage.events.length > 0) {
        const hasErrors = stage.events.some(e => e.severity === 'error');
        const hasSuccess = stage.events.some(e => e.severity === 'success');
        const allMessages = stage.events.map(e => e.message.toLowerCase()).join(' ');
        
        // Check for explicit stage completion messages (most reliable)
        const hasExplicitCompletion = allMessages.includes('stage completed successfully') ||
                                     allMessages.includes('✅') ||
                                     allMessages.includes('✓');
        
        // Check for implicit completion indicators
        const hasCompletionKeywords = allMessages.includes('completed') || 
                                     allMessages.includes('finished') || 
                                     allMessages.includes('done') ||
                                     allMessages.includes('success');
        
        // Check if next stage has started (indicates this stage completed)
        const nextStageStarted = index < processedStages.length - 1 && 
                                processedStages[index + 1].events.length > 0;
        
        if (hasErrors && !hasSuccess && !hasCompletionKeywords && !hasExplicitCompletion) {
          stage.status = 'failed';
        } else if (hasExplicitCompletion || hasSuccess || nextStageStarted) {
          stage.status = 'completed';
          stage.endTime = stage.events[stage.events.length - 1].timestamp;
        } else if (hasCompletionKeywords) {
          stage.status = 'completed';
          stage.endTime = stage.events[stage.events.length - 1].timestamp;
        }
        
        // Special case: if this is the final stage (Completion) and it has any events, 
        // it should be marked as completed since there's no next stage
        if (index === 4 && stage.events.length > 0) {
          stage.status = 'completed';
          stage.endTime = stage.events[stage.events.length - 1].timestamp;
        }
        // If stage has events but no clear completion, it remains in-progress
      }
    });

    // Ensure logical progression - if a later stage is completed, earlier ones should be too
    for (let i = processedStages.length - 1; i >= 0; i--) {
      if (processedStages[i].status === 'completed') {
        for (let j = 0; j < i; j++) {
          if (processedStages[j].status === 'pending' && processedStages[j].events.length > 0) {
            processedStages[j].status = 'completed';
            processedStages[j].endTime = processedStages[j].events[processedStages[j].events.length - 1].timestamp;
          }
        }
        break; // Found the latest completed stage, earlier ones are now handled
      }
    }

    return processedStages;
  };

  const createBatchSubstages = (batchEvents: ReviewEvent[]): Substage[] => {
    const batchMap = new Map<string, Substage>();
    
    // Debug logging
    if (process.env.NODE_ENV === 'development') {
      console.log('[createBatchSubstages] Processing events:', batchEvents.length, 
        batchEvents.map(e => ({
          message: e.message,
          batchId: e.details?.batchId,
          eventType: e.eventType
        })));
    }
    
    batchEvents.forEach(event => {
      // Extract batch info from details first, then from message
      let batchId = event.details?.batchId;
      if (!batchId) {
        // Match patterns like "batch-1", "batch 1", or standalone batch ID
        const batchMatch = event.message.match(/batch[-\s]([a-zA-Z0-9-]+)/i);
        batchId = batchMatch ? batchMatch[1] : 'general';
      }
      
      if (process.env.NODE_ENV === 'development') {
        console.log('[createBatchSubstages] Event:', event.message, '→ batchId:', batchId);
      }
      
      if (!batchMap.has(batchId)) {
        batchMap.set(batchId, {
          id: `batch-${batchId}`,
          title: `Batch ${batchId}`,
          status: 'pending',
          events: [],
          resiliencyEvents: []
        });
      }

      const substage = batchMap.get(batchId)!;
      substage.events.push(event);
      
      if (substage.status === 'pending') {
        substage.status = 'in-progress';
        substage.startTime = event.timestamp;
      }

      // Process resiliency events
      if (event.eventType === 'retry') {
        substage.resiliencyEvents.push({
          type: 'retry',
          attempt: event.details?.attempt || 1,
          details: event.message,
          resolved: event.severity !== 'error'
        });
      } else if (event.eventType === 'json_repair') {
        substage.resiliencyEvents.push({
          type: 'json_repair',
          details: event.message,
          resolved: true
        });
      } else if (event.eventType === 'timeout') {
        substage.resiliencyEvents.push({
          type: 'timeout',
          details: event.message,
          resolved: event.severity !== 'error'
        });
      }

      // Update substage status
      if (event.severity === 'success' || event.message.toLowerCase().includes('completed')) {
        substage.status = 'completed';
        substage.endTime = event.timestamp;
      } else if (event.severity === 'error' && !event.message.toLowerCase().includes('retry')) {
        substage.status = 'failed';
      }
    });

    return Array.from(batchMap.values());
  };

  const getStageIcon = (status: Stage['status']) => {
    switch (status) {
      case 'completed':
        return <Icons.Success />;
      case 'failed':
        return <Icons.Error />;
      case 'in-progress':
        return <div className="animate-spin"><Icons.Refresh /></div>;
      case 'pending':
        return <Icons.Clock />;
    }
  };

  const getStageColor = (status: Stage['status']) => {
    switch (status) {
      case 'completed':
        return 'border-green-500 bg-green-900/10';
      case 'failed':
        return 'border-red-500 bg-red-900/10';
      case 'in-progress':
        return 'border-blue-500 bg-blue-900/10';
      case 'pending':
        return 'border-slate-600 bg-slate-900/10';
    }
  };

  const getResiliencyIcon = (type: ResiliencyEvent['type']) => {
    switch (type) {
      case 'retry':
        return <Icons.Refresh />;
      case 'json_repair':
        return <span className="text-orange-400">⚡</span>;
      case 'timeout':
        return <Icons.Clock />;
      case 'circuit_breaker':
        return <Icons.Warning />;
    }
  };

  const toggleStage = (stageId: string) => {
    const newExpanded = new Set(expandedStages);
    if (newExpanded.has(stageId)) {
      newExpanded.delete(stageId);
    } else {
      newExpanded.add(stageId);
    }
    setExpandedStages(newExpanded);
  };

  const formatDuration = (startTime?: string, endTime?: string) => {
    if (!startTime) return null;
    const start = new Date(startTime);
    const end = endTime ? new Date(endTime) : new Date();
    const diff = end.getTime() - start.getTime();
    const seconds = Math.floor(diff / 1000);
    const minutes = Math.floor(seconds / 60);
    
    if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    }
    return `${seconds}s`;
  };

  // Calculate dynamic description for stages (called at render time)
  const getDynamicDescription = (stage: Stage): string => {
    // For Artifact Generation stage (stage 4), calculate batch/comment progress
    if (stage.id === 'stage-4' && stage.substages.length > 0) {
      const totalBatches = stage.substages.length;
      const completedBatches = stage.substages.filter(s => s.status === 'completed').length;
      const inProgressBatches = stage.substages.filter(s => s.status === 'in-progress').length;
      
      // Count total comments from completed batches
      let totalComments = 0;
      stage.substages.forEach(substage => {
        const commentEvent = substage.events.find(e => 
          e.message.toLowerCase().includes('comment') && 
          (e.message.toLowerCase().includes('completed') || e.message.toLowerCase().includes('generated'))
        );
        if (commentEvent) {
          const match = commentEvent.message.match(/(\d+)\s+comment/);
          if (match) {
            totalComments += parseInt(match[1], 10);
          }
        }
      });
      
      let statusText = '';
      if (totalBatches > 0) {
        statusText = `Processing ${totalBatches} batch${totalBatches !== 1 ? 'es' : ''}`;
        if (completedBatches > 0) {
          statusText += ` (${completedBatches}/${totalBatches} completed)`;
        } else if (inProgressBatches > 0) {
          statusText += ` (${inProgressBatches} in progress)`;
        }
      }
      
      if (totalComments > 0) {
        statusText += statusText ? ' • ' : '';
        statusText += `Posted ${totalComments} comment${totalComments !== 1 ? 's' : ''}`;
      }
      
      return statusText || stage.description;
    }
    
    return stage.description;
  };

  // Simplified and accurate progress calculation
  const overallProgress = stages.reduce((total, stage) => {
    if (stage.status === 'completed') return total + 20; // Each stage is 20% (100% / 5 stages)
    if (stage.status === 'in-progress') return total + 10; // In-progress gets half credit (10%)
    if (stage.status === 'failed') return total + 5; // Failed stages get minimal progress (5%)
    return total; // Pending stages contribute 0%
  }, 0);

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Overall Progress Header */}
      <Card className="border-l-4 border-l-blue-500">
        <div className="p-6">
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="text-xl font-semibold text-white">Review Progress</h2>
              <p className="text-slate-400">Processing merge request review</p>
            </div>
            <div className="text-right">
              <div className="text-2xl font-bold text-white">{Math.round(overallProgress)}%</div>
              <div className="text-sm text-slate-400">Complete</div>
            </div>
          </div>
          
          {/* Overall progress bar */}
          <div className="w-full bg-slate-700 rounded-full h-3 mb-2">
            <div 
              className="bg-blue-500 h-3 rounded-full transition-all duration-500"
              style={{ width: `${overallProgress}%` }}
            ></div>
          </div>
          
          <div className="flex justify-between text-sm">
            <span className="text-slate-300">
              <span className="font-medium">Stage status:</span>{' '}
              <span className="text-slate-400">
                {stages.filter(s => s.status === 'completed').length} completed, {stages.filter(s => s.status === 'in-progress').length} in progress, {stages.filter(s => s.status === 'pending').length} pending
              </span>
            </span>
            {isLive && <span className="text-green-400 font-medium">● Live</span>}
          </div>
        </div>
      </Card>

      {/* Stage Timeline */}
      <div className="relative">
        {/* Timeline line */}
        <div className="absolute left-8 top-0 bottom-0 w-0.5 bg-slate-600"></div>
        
        {stages.map((stage, index) => {
          const isExpanded = expandedStages.has(stage.id);
          const duration = formatDuration(stage.startTime, stage.endTime);
          
          return (
            <div key={stage.id} className="relative mb-6">
              {/* Stage header */}
              <div className={`ml-16 border-l-4 rounded-lg ${getStageColor(stage.status)}`}>
                <div 
                  className="p-4 cursor-pointer hover:bg-slate-800/50 transition-colors"
                  onClick={() => toggleStage(stage.id)}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex items-center gap-2">
                        {isExpanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}
                        <span className="text-lg font-medium text-white">{stage.title}</span>
                      </div>
                      <Badge 
                        variant={
                          stage.status === 'completed' ? 'success' :
                          stage.status === 'failed' ? 'danger' :
                          stage.status === 'in-progress' ? 'primary' :
                          'default'
                        }
                      >
                        {stage.status}
                      </Badge>
                    </div>
                    
                    <div className="flex items-center gap-4 text-sm text-slate-400">
                      {duration && <span>{duration}</span>}
                      <span>{stage.events.filter(e => e.message && e.message.trim().length > 0).length} events</span>
                    </div>
                  </div>
                  
                  <p className="text-slate-400 text-sm mt-1">{getDynamicDescription(stage)}</p>
                </div>
                
                {/* Expanded content */}
                {isExpanded && (
                  <div className="border-t border-slate-700 bg-slate-900/30">
                    {/* Substages */}
                    {stage.substages.length > 0 && (
                      <div className="p-4">
                        <h4 className="text-sm font-medium text-slate-300 mb-3">
                          Batches ({stage.substages.filter(s => s.status === 'completed').length}/{stage.substages.length} completed)
                        </h4>
                        <div className="space-y-3">
                          {stage.substages.map(substage => (
                            <div key={substage.id} className="bg-slate-800/50 rounded-lg p-3">
                              <div className="flex items-center justify-between mb-2">
                                <div className="flex items-center gap-2">
                                  {getStageIcon(substage.status)}
                                  <span className="font-medium text-white">{substage.title}</span>
                                  {substage.count && (
                                    <Badge variant="info" size="sm">
                                      {substage.count.current}/{substage.count.total}
                                    </Badge>
                                  )}
                                  <Badge 
                                    variant={
                                      substage.status === 'completed' ? 'success' :
                                      substage.status === 'failed' ? 'danger' :
                                      substage.status === 'in-progress' ? 'primary' :
                                      'default'
                                    }
                                    size="sm"
                                  >
                                    {substage.status}
                                  </Badge>
                                </div>
                                <div className="text-xs text-slate-400">
                                  {formatDuration(substage.startTime, substage.endTime) || 'Starting...'}
                                </div>
                              </div>
                              
                              {/* Resiliency events */}
                              {substage.resiliencyEvents.length > 0 && (
                                <div className="flex flex-wrap gap-2 mb-2">
                                  {substage.resiliencyEvents.map((resEvent, idx) => (
                                    <div 
                                      key={idx}
                                      className={`flex items-center gap-1 px-2 py-1 rounded text-xs ${
                                        resEvent.resolved 
                                          ? 'bg-yellow-900/30 text-yellow-300 border border-yellow-700/30' 
                                          : 'bg-red-900/30 text-red-300 border border-red-700/30'
                                      }`}
                                    >
                                      {getResiliencyIcon(resEvent.type)}
                                      <span>
                                        {resEvent.type.replace('_', ' ')}
                                        {resEvent.attempt && ` (${resEvent.attempt}x)`}
                                      </span>
                                      {resEvent.resolved && <Icons.Success />}
                                    </div>
                                  ))}
                                </div>
                              )}
                              
                              <div className="text-xs">
                                {(() => {
                                  // Extract comment count from batch events
                                  const commentEvent = substage.events.find(e => 
                                    e.message.toLowerCase().includes('completed') && 
                                    e.message.toLowerCase().includes('comment')
                                  );
                                  const commentMatch = commentEvent?.message.match(/(\d+)\s+comment/);
                                  const commentCount = commentMatch ? parseInt(commentMatch[1], 10) : null;
                                  
                                  if (substage.status === 'in-progress') {
                                    return <span className="text-blue-400 font-medium">● Processing batch...</span>;
                                  }
                                  if (substage.status === 'completed' && commentCount !== null) {
                                    return <span className="text-green-400">✓ Generated {commentCount} comment{commentCount !== 1 ? 's' : ''}</span>;
                                  }
                                  if (substage.status === 'completed') {
                                    return <span className="text-green-400">✓ Completed</span>;
                                  }
                                  if (substage.status === 'failed') {
                                    return <span className="text-red-400">✗ Failed</span>;
                                  }
                                  return <span className="text-slate-500">Waiting to start...</span>;
                                })()}
                              </div>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                    
                    {/* Events section with expandable view */}
                    {stage.events.length > 0 && (
                      <div className="p-4 border-t border-slate-700">
                        <div className="flex items-center justify-between mb-3">
                          <h4 className="text-sm font-medium text-slate-300">
                            Events ({stage.events.length})
                          </h4>
                          {stage.events.length > 5 && (
                            <button
                              onClick={() => {
                                const newExpanded = new Set(expandedEvents);
                                if (newExpanded.has(stage.id)) {
                                  newExpanded.delete(stage.id);
                                } else {
                                  newExpanded.add(stage.id);
                                }
                                setExpandedEvents(newExpanded);
                              }}
                              className="text-xs text-blue-400 hover:text-blue-300"
                            >
                              {expandedEvents.has(stage.id) ? 'Show Less' : 'Show All'}
                            </button>
                          )}
                        </div>
                        <div className="space-y-2 max-h-96 overflow-y-auto">
                          {(expandedEvents.has(stage.id) ? stage.events : stage.events.slice(-5))
                            .filter(event => event.message && event.message.trim().length > 0)
                            .map(event => (
                            <div key={event.id} className="flex items-center gap-3 text-sm">
                              <div className={`w-2 h-2 rounded-full ${
                                event.severity === 'error' ? 'bg-red-500' :
                                event.severity === 'warning' ? 'bg-yellow-500' :
                                event.severity === 'success' ? 'bg-green-500' :
                                'bg-blue-500'
                              }`}></div>
                              <span className="text-slate-300 flex-1">{event.message}</span>
                              <span className="text-slate-500 text-xs">
                                {new Date(event.timestamp).toLocaleTimeString()}
                              </span>
                            </div>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
              
              {/* Timeline dot */}
              <div className={`absolute left-6 top-4 w-4 h-4 rounded-full border-2 border-slate-800 flex items-center justify-center ${
                stage.status === 'completed' ? 'bg-green-500' :
                stage.status === 'failed' ? 'bg-red-500' :
                stage.status === 'in-progress' ? 'bg-blue-500' :
                'bg-slate-600'
              }`}>
                {stage.status === 'in-progress' && (
                  <div className="w-2 h-2 bg-white rounded-full animate-pulse"></div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}