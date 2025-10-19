import React, { useState, useEffect } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';
import { ReviewEvent } from './types';

interface ReviewProgressViewProps {
  reviewId: number;
  events: ReviewEvent[];
  isLive?: boolean;
  className?: string;
}

type StageStatus = 'pending' | 'in-progress' | 'completed' | 'failed';
type StageKey = 'preparation' | 'analysis' | 'review' | 'artifact' | 'finalization';

interface StageDefinition {
  id: string;
  key: StageKey;
  title: string;
  description: string;
}

interface Stage {
  id: string;
  key: StageKey;
  title: string;
  description: string;
  status: StageStatus;
  startTime?: string;
  endTime?: string;
  substages: Substage[];
  events: ReviewEvent[];
  generatedComments?: number;
  postedComments?: number;
}

const extractPostedCommentCount = (event: ReviewEvent): number | undefined => {
  if (typeof event.details?.commentCount === 'number' && !Number.isNaN(event.details.commentCount)) {
    return event.details.commentCount;
  }

  const postedMatch = event.message.match(/posted\s+(\d+)\s+(?:individual\s+)?comments/i);
  if (postedMatch) {
    const parsed = parseInt(postedMatch[1], 10);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
  }

  const summaryMatch = event.message.match(/comments\s+posted:\s*(\d+)/i);
  if (summaryMatch) {
    const parsed = parseInt(summaryMatch[1], 10);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
  }

  return undefined;
};

const STAGE_DEFINITIONS: StageDefinition[] = [
  {
    id: 'stage-1',
    key: 'preparation',
    title: 'Preparation',
    description: 'Initializing providers and configuration'
  },
  {
    id: 'stage-2',
    key: 'analysis',
    title: 'Analysis',
    description: 'Fetching MR details and analyzing changed files'
  },
  {
    id: 'stage-3',
    key: 'review',
    title: 'Review',
    description: 'AI processing and generating review comments'
  },
  {
    id: 'stage-4',
    key: 'artifact',
    title: 'Artifact Generation',
    description: 'Posting review comments to merge request'
  },
  {
    id: 'stage-5',
    key: 'finalization',
    title: 'Finalization',
    description: 'Finalizing review and cleanup'
  }
];

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
    const stageMap = new Map<StageKey, Stage>();
    STAGE_DEFINITIONS.forEach(def => {
      stageMap.set(def.key, {
        id: def.id,
        key: def.key,
        title: def.title,
        description: def.description,
        status: 'pending',
        startTime: undefined,
        endTime: undefined,
        events: [],
        substages: [],
        generatedComments: undefined,
        postedComments: undefined
      });
    });

    const batchSubstages = new Map<string, Substage>();
    const stageKeys: StageKey[] = STAGE_DEFINITIONS.map(def => def.key);

    const normalize = (value?: string) => (value || '').toLowerCase();

    const stageStartKeywords: Record<StageKey, string[]> = {
      preparation: ['stage started', 'preparation'],
      analysis: ['stage started', 'analysis'],
      review: ['stage started', 'review'],
      artifact: ['stage started', 'artifact generation'],
      finalization: ['stage started', 'finalization']
    };

    const stageCompletionKeywords: Record<StageKey, string[]> = {
      preparation: ['stage completed', 'preparation'],
      analysis: ['stage completed', 'analysis'],
      review: ['stage completed', 'review'],
      artifact: ['stage completed', 'artifact generation'],
      finalization: ['stage completed', 'finalization']
    };

    const containsAll = (message: string, keywords: string[]) => keywords.every(keyword => message.includes(keyword));

    const detectExplicitStageStart = (message: string): StageKey | null => {
      for (const key of stageKeys) {
        if (containsAll(message, stageStartKeywords[key])) {
          return key;
        }
      }
      return null;
    };

    const detectExplicitStageCompletion = (message: string): StageKey | null => {
      for (const key of stageKeys) {
        if (containsAll(message, stageCompletionKeywords[key])) {
          return key;
        }
      }
      if (message.includes('review process completed') || message.includes('finalization complete')) {
        return 'finalization';
      }
      if (message.includes('posted') && message.includes('comments')) {
        return 'artifact';
      }
      return null;
    };

    const inferStageKey = (event: ReviewEvent, message: string): StageKey | null => {
      const details = event.details || {};
      const eventType = event.eventType;

      if (details.batchId || eventType === 'batch' || eventType === 'artifact') {
        return 'artifact';
      }

      if (eventType === 'completion') {
        return 'finalization';
      }

      if (eventType === 'completed') {
        return 'finalization';
      }

      if (eventType === 'batch_complete') {
        return 'artifact';
      }

      if (eventType === 'status') {
        const statusValue = normalize(String(details.status || ''));
        if (statusValue === 'completed') {
          return 'finalization';
        }
      }

      if (eventType === 'started') {
        return 'preparation';
      }

      if (eventType === 'progress') {
        return 'analysis';
      }

      if (message.includes('finaliz') || message.includes('cleanup') || message.includes('summary')) {
        return 'finalization';
      }

      if (message.includes('artifact') || message.includes('posted') && message.includes('comment') || message.includes('batch')) {
        return 'artifact';
      }

      if (eventType === 'retry' || eventType === 'json_repair' || eventType === 'timeout') {
        if (details.batchId) {
          return 'artifact';
        }
        return 'review';
      }

      if (eventType === 'error') {
        if (details.batchId) {
          return 'artifact';
        }
        return 'review';
      }

      if (message.includes('analysis') || message.includes('diff') || message.includes('fetch') || message.includes('file') || message.includes('merge request')) {
        return 'analysis';
      }

      if (message.includes('preparation') || message.includes('provider') || message.includes('initializ')) {
        return 'preparation';
      }

      if (message.includes('review') || message.includes('llm') || message.includes('model') || message.includes('generate')) {
        return 'review';
      }

      return null;
    };

    const severityIsError = (severity?: ReviewEvent['severity']) => {
      if (!severity) return false;
      const normalized = severity.toLowerCase();
      return normalized === 'error' || normalized === 'critical';
    };

    const updateBatchSubstage = (event: ReviewEvent) => {
      let batchId = event.details?.batchId;
      if (!batchId) {
        const explicitIdMatch = event.message.match(/batch[-_]?([0-9]+)/i);
        if (explicitIdMatch) {
          batchId = explicitIdMatch[0];
        } else {
          const spacedMatch = event.message.match(/batch\s+([0-9]+)/i);
          if (spacedMatch) {
            batchId = `batch-${spacedMatch[1]}`;
          }
        }
      }
      if (!batchId) {
        return;
      }

      const rawBatchId = batchId.toString();
      const batchKey = rawBatchId.toLowerCase();
      const displayName = (() => {
        const match = rawBatchId.match(/^(?:batch[-_]?)(.+)$/i);
        if (match && match[1]) {
          return `Batch ${match[1]}`;
        }
        return rawBatchId.toLowerCase().startsWith('batch') ? rawBatchId : `Batch ${rawBatchId}`;
      })();

      if (!batchSubstages.has(batchKey)) {
        batchSubstages.set(batchKey, {
          id: `batch-${batchKey}`,
          title: displayName,
          status: 'pending',
          events: [],
          resiliencyEvents: [],
          count: undefined,
          startTime: undefined,
          endTime: undefined
        });
      }

      const substage = batchSubstages.get(batchKey)!;
      substage.events.push(event);

      const message = normalize(event.message);

      const ensureCount = (value?: number) => {
        if (typeof value !== 'number' || Number.isNaN(value)) {
          return;
        }
        const safeValue = Math.max(0, value);
        if (!substage.count) {
          substage.count = { current: safeValue, total: safeValue };
        } else {
          substage.count.current = safeValue;
          substage.count.total = Math.max(substage.count.total, safeValue);
        }
      };

      const parseCountFromMessage = (text: string) => {
        const match = text.match(/(\d+)\s+comment/);
        if (match) {
          const parsed = parseInt(match[1], 10);
          if (!Number.isNaN(parsed)) {
            return parsed;
          }
        }
        return undefined;
      };

      const markInProgress = () => {
        if (substage.status === 'pending') {
          substage.status = 'in-progress';
          substage.startTime = substage.startTime ?? event.timestamp;
        }
      };

      const markCompleted = (countHint?: number) => {
        substage.status = 'completed';
        substage.endTime = event.timestamp;
        ensureCount(countHint ?? parseCountFromMessage(event.message));
        if (substage.resiliencyEvents.length > 0) {
          substage.resiliencyEvents = substage.resiliencyEvents.map(resEvent => ({
            ...resEvent,
            resolved: true
          }));
        }
      };

      const markFailed = () => {
        if (substage.status === 'completed') {
          return;
        }
        substage.status = 'failed';
        substage.endTime = event.timestamp;
      };

      const countFromDetails = event.details?.commentCount ?? event.details?.fileCount;

      if (event.eventType === 'batch' || event.eventType === 'status') {
        const statusValue = normalize(String(event.details?.status || ''));
        if (statusValue === 'processing') {
          markInProgress();
          ensureCount(countFromDetails);
        } else if (statusValue === 'completed') {
          markCompleted(countFromDetails);
        } else if (statusValue === 'failed') {
          markFailed();
        }
      }

      if (event.eventType === 'batch_complete') {
        markCompleted(countFromDetails);
      }

      if (event.eventType === 'artifact') {
        if (message.includes('generated') || message.includes('posted')) {
          markCompleted(countFromDetails);
        }
      }

      if (event.eventType === 'log') {
        if (message.includes('started') || message.includes('processing')) {
          markInProgress();
        }
        if (message.includes('completed') || message.includes('generated') || message.includes('posted')) {
          markCompleted(countFromDetails);
        }
        if (severityIsError(event.severity)) {
          markFailed();
        }
      }

      if (event.eventType === 'completion' || event.eventType === 'completed') {
        if (message.includes(batchKey)) {
          markCompleted(countFromDetails);
        }
      }

      if (event.eventType === 'retry' || event.eventType === 'json_repair' || event.eventType === 'timeout') {
        substage.resiliencyEvents.push({
          type: event.eventType,
          details: event.message,
          attempt: event.details?.attempt,
          resolved: !severityIsError(event.severity)
        });
        markInProgress();
      }

      if (event.eventType === 'error' || severityIsError(event.severity)) {
        substage.resiliencyEvents.push({
          type: 'circuit_breaker',
          details: event.message,
          attempt: event.details?.attempt,
          resolved: false
        });
        markFailed();
      }

      // If we still do not have a count but have a hint in the message, capture it for UI feedback
      if (!substage.count) {
        ensureCount(parseCountFromMessage(event.message));
      }
    };

    events.forEach(event => {
      const message = normalize(event.message);

      let stageKey: StageKey | null = detectExplicitStageStart(message);
      if (!stageKey) {
        stageKey = detectExplicitStageCompletion(message);
      }
      if (!stageKey) {
        stageKey = inferStageKey(event, message);
      }

      if (!stageKey) {
        return;
      }

      const stage = stageMap.get(stageKey);
      if (!stage) {
        return;
      }

      stage.events.push(event);

      if (stage.status === 'pending') {
        stage.status = 'in-progress';
        stage.startTime = stage.startTime ?? event.timestamp;
      }

      const explicitStartKey = detectExplicitStageStart(message);
      if (explicitStartKey === stageKey) {
        stage.status = 'in-progress';
        stage.startTime = stage.startTime ?? event.timestamp;
      }

      if (severityIsError(event.severity) && stage.status !== 'completed') {
        stage.status = 'failed';
        stage.endTime = event.timestamp;
      }

      const explicitCompletionKey = detectExplicitStageCompletion(message);
      if (explicitCompletionKey === stageKey) {
        stage.status = 'completed';
        stage.endTime = event.timestamp;
      }

      if (stageKey === 'finalization' && event.eventType === 'completion') {
        stage.status = 'completed';
        stage.endTime = event.timestamp;
      }

      updateBatchSubstage(event);
    });

    // Finalize stage metadata
    stageMap.forEach(stage => {
      if (stage.events.length > 0 && !stage.startTime) {
        stage.startTime = stage.events[0].timestamp;
      }
      if (stage.status === 'completed' && !stage.endTime) {
        stage.endTime = stage.events[stage.events.length - 1].timestamp;
      }
      if (stage.status === 'pending' && stage.events.length > 0) {
        stage.status = 'in-progress';
      }
    });

    // Attach substages to artifact stage
    const artifactStage = stageMap.get('artifact');
    if (artifactStage) {
      const substagesArray = Array.from(batchSubstages.values()).sort((a, b) => {
        const aTime = a.startTime || (a.events[0]?.timestamp ?? '');
        const bTime = b.startTime || (b.events[0]?.timestamp ?? '');
        return aTime.localeCompare(bTime);
      });

      substagesArray.forEach(substage => {
        if (substage.events.length > 0 && !substage.startTime) {
          substage.startTime = substage.events[0].timestamp;
        }
        if (substage.status === 'completed' && !substage.endTime) {
          substage.endTime = substage.events[substage.events.length - 1].timestamp;
        }
        if (substage.status === 'pending' && substage.events.length > 0) {
          substage.status = 'in-progress';
        }
      });

      artifactStage.substages = substagesArray;

      // Update artifact stage summary counts if available
      const totalComments = substagesArray.reduce((acc, substage) => acc + (substage.count?.current || 0), 0);
      artifactStage.generatedComments = totalComments > 0 ? totalComments : undefined;

      const postedCounts = artifactStage.events
        .map(extractPostedCommentCount)
        .filter((value): value is number => typeof value === 'number' && !Number.isNaN(value));
      const postedComments = postedCounts.length > 0 ? Math.max(...postedCounts) : undefined;
      artifactStage.postedComments = postedComments;

      const hasFailures = substagesArray.some(substage => substage.status === 'failed');
      const allCompleted = substagesArray.length > 0 && substagesArray.every(substage => substage.status === 'completed');
      const anyInProgress = substagesArray.some(substage => substage.status === 'in-progress');

      if (hasFailures) {
        artifactStage.status = 'failed';
        const failureEnd = substagesArray.find(substage => substage.status === 'failed')?.endTime;
        artifactStage.endTime = failureEnd ?? artifactStage.endTime;
      } else if (allCompleted) {
        artifactStage.status = 'completed';
        const latestEnd = substagesArray.reduce((latest, substage) => {
          const candidate = substage.endTime || (substage.events[substage.events.length - 1]?.timestamp ?? '');
          if (!candidate) {
            return latest;
          }
          if (!latest) {
            return candidate;
          }
          return latest.localeCompare(candidate) >= 0 ? latest : candidate;
        }, artifactStage.endTime ?? '');
        artifactStage.endTime = latestEnd || artifactStage.endTime;
      } else if (anyInProgress && artifactStage.status === 'pending') {
        artifactStage.status = 'in-progress';
      }

      if (artifactStage.startTime === undefined && substagesArray.length > 0) {
        const firstStart = substagesArray[0].startTime || substagesArray[0].events[0]?.timestamp;
        artifactStage.startTime = firstStart ?? artifactStage.startTime;
      }

      if (totalComments > 0 || postedComments !== undefined) {
        const parts: string[] = ['Posting review comments to merge request'];
        if (totalComments > 0) {
          parts.push(`Generated ${totalComments} comment${totalComments !== 1 ? 's' : ''}`);
        }
        if (postedComments !== undefined) {
          parts.push(`Posted ${postedComments} comment${postedComments !== 1 ? 's' : ''}`);
        }
        artifactStage.description = parts.join(' • ');
      }
    }

    // Enforce stage order so later stages cannot complete before earlier ones
    const orderedKeys = STAGE_DEFINITIONS.map(def => def.key);
    orderedKeys.forEach((key, index) => {
      if (index === 0) {
        return;
      }
      const currentStage = stageMap.get(key)!;
      const previousStage = stageMap.get(orderedKeys[index - 1])!;

      if (previousStage.status !== 'completed') {
        if (currentStage.status === 'completed') {
          currentStage.status = currentStage.events.length > 0 ? 'in-progress' : 'pending';
          currentStage.endTime = undefined;
        }
        if (currentStage.status === 'pending' && currentStage.events.length > 0) {
          currentStage.status = 'in-progress';
        }
      }
    });

    return STAGE_DEFINITIONS.map(def => stageMap.get(def.key)!);
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
      const pendingBatches = stage.substages.filter(s => s.status === 'pending').length;
      const failedBatches = stage.substages.filter(s => s.status === 'failed').length;
      
      // Count total comments from completed batches using structured data
      const totalComments = stage.substages.reduce((acc, substage) => acc + (substage.count?.current ?? 0), 0);
      
      const phaseSegments: string[] = [];
      if (completedBatches > 0) {
        phaseSegments.push(`${completedBatches} completed`);
      }
      if (inProgressBatches > 0) {
        phaseSegments.push(`${inProgressBatches} in progress`);
      }
      if (pendingBatches > 0) {
        phaseSegments.push(`${pendingBatches} waiting`);
      }
      if (failedBatches > 0) {
        phaseSegments.push(`${failedBatches} failed`);
      }

      const statusPrefix = totalBatches > 0
        ? `Processing ${totalBatches} batch${totalBatches !== 1 ? 'es' : ''}`
        : '';

      const statusText = phaseSegments.length > 0
        ? `${statusPrefix}${statusPrefix ? ' ' : ''}(${phaseSegments.join(', ')})`
        : statusPrefix;

      let commentText = '';
      if (totalComments > 0) {
        const verb = completedBatches === totalBatches && totalBatches > 0 ? 'Posted' : 'Generated';
        commentText = `${verb} ${totalComments} comment${totalComments !== 1 ? 's' : ''}`;
      }

      return [statusText, commentText].filter(Boolean).join(' • ') || stage.description;
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
                                      {substage.status === 'completed' && substage.count.total !== undefined
                                        ? `${substage.count.current}/${substage.count.total}`
                                        : `${substage.count.current}`}
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
                                  // Use structured comment count from substage.count
                                  const commentCount = substage.count?.current || null;
                                  
                                  if (substage.status === 'in-progress') {
                                    return (
                                      <span className="text-blue-400 font-medium">
                                        ● Processing batch{commentCount ? `... ${commentCount} comment${commentCount !== 1 ? 's' : ''} ready` : '...'}
                                      </span>
                                    );
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
                                event.severity === 'warning' || event.severity === 'warn' ? 'bg-yellow-500' :
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