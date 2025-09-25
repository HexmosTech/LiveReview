import React, { useEffect, useState } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';

interface ProgressIndicatorsProps {
  reviewProgress: ReviewProgress;
  batchProgress: BatchProgress[];
  isLive?: boolean;
  className?: string;
}

interface ReviewProgress {
  phase: 'preparation' | 'analysis' | 'review' | 'completion';
  overallPercentage: number;
  totalFiles: number;
  processedFiles: number;
  successfulFiles: number;
  failedFiles: number;
  estimatedTimeRemaining?: string;
  startTime: string;
  currentBatch?: number;
  totalBatches: number;
}

interface BatchProgress {
  batchId: string;
  batchNumber: number;
  status: 'pending' | 'running' | 'completed' | 'failed';
  totalFiles: number;
  processedFiles: number;
  successCount: number;
  retryCount: number;
  errorCount: number;
  jsonRepairs: number;
  startTime?: string;
  endTime?: string;
  estimatedCompletion?: string;
}

export default function ProgressIndicators({ 
  reviewProgress, 
  batchProgress, 
  isLive = false, 
  className 
}: ProgressIndicatorsProps) {
  const [animatedPercentage, setAnimatedPercentage] = useState(0);

  useEffect(() => {
    // Animate the progress bar
    const timer = setTimeout(() => {
      setAnimatedPercentage(reviewProgress.overallPercentage);
    }, 100);
    return () => clearTimeout(timer);
  }, [reviewProgress.overallPercentage]);

  const getPhaseIcon = (phase: ReviewProgress['phase']) => {
    switch (phase) {
      case 'preparation':
        return <Icons.Settings />;
      case 'analysis':
        return <Icons.Search />;
      case 'review':
        return <Icons.Reviews />;
      case 'completion':
        return <Icons.Success />;
      default:
        return <Icons.Clock />;
    }
  };

  const getPhaseLabel = (phase: ReviewProgress['phase']) => {
    switch (phase) {
      case 'preparation':
        return 'Preparing Review';
      case 'analysis':
        return 'Analyzing Code';
      case 'review':
        return 'Generating Reviews';
      case 'completion':
        return 'Finalizing';
      default:
        return 'Unknown Phase';
    }
  };

  const getBatchStatusColor = (status: BatchProgress['status']) => {
    switch (status) {
      case 'completed':
        return 'bg-green-500';
      case 'running':
        return 'bg-blue-500';
      case 'failed':
        return 'bg-red-500';
      case 'pending':
        return 'bg-slate-500';
      default:
        return 'bg-slate-500';
    }
  };

  const calculateElapsedTime = (startTime: string) => {
    const start = new Date(startTime);
    const now = new Date();
    const diff = now.getTime() - start.getTime();
    const minutes = Math.floor(diff / 60000);
    const seconds = Math.floor((diff % 60000) / 1000);
    
    if (minutes > 60) {
      const hours = Math.floor(minutes / 60);
      return `${hours}h ${minutes % 60}m`;
    }
    return minutes > 0 ? `${minutes}m ${seconds}s` : `${seconds}s`;
  };

  const successRate = reviewProgress.processedFiles > 0 
    ? (reviewProgress.successfulFiles / reviewProgress.processedFiles) * 100 
    : 0;

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Overall Progress */}
      <Card className="border-l-4 border-l-blue-500">
        <div className="p-6">
          {/* Header */}
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-3">
              {getPhaseIcon(reviewProgress.phase)}
              <div>
                <h3 className="text-lg font-semibold text-white">
                  {getPhaseLabel(reviewProgress.phase)}
                </h3>
                <p className="text-sm text-slate-400">
                  {reviewProgress.processedFiles} of {reviewProgress.totalFiles} files processed
                </p>
              </div>
            </div>
            
            <div className="text-right">
              <div className="text-3xl font-bold text-white mb-1">
                {Math.round(animatedPercentage)}%
              </div>
              {reviewProgress.estimatedTimeRemaining && (
                <div className="text-sm text-slate-400">
                  ETA: {reviewProgress.estimatedTimeRemaining}
                </div>
              )}
            </div>
          </div>

          {/* Progress Bar */}
          <div className="space-y-2">
            <div className="w-full bg-slate-700 rounded-full h-4 overflow-hidden">
              <div 
                className="h-full bg-gradient-to-r from-blue-500 to-blue-400 rounded-full transition-all duration-1000 ease-out relative"
                style={{ width: `${animatedPercentage}%` }}
              >
                {isLive && animatedPercentage > 0 && animatedPercentage < 100 && (
                  <div className="absolute right-0 top-0 bottom-0 w-2 bg-white/30 animate-pulse"></div>
                )}
              </div>
            </div>
            
            {/* Progress breakdown */}
            <div className="flex justify-between text-xs text-slate-400">
              <span>Started {calculateElapsedTime(reviewProgress.startTime)} ago</span>
              <span>
                Batch {reviewProgress.currentBatch || 1} of {reviewProgress.totalBatches}
              </span>
            </div>
          </div>

          {/* Quick Stats */}
          <div className="grid grid-cols-3 gap-4 mt-4">
            <div className="text-center">
              <div className="text-xl font-bold text-green-400">
                {reviewProgress.successfulFiles}
              </div>
              <div className="text-xs text-slate-400">Successful</div>
            </div>
            <div className="text-center">
              <div className="text-xl font-bold text-slate-300">
                {reviewProgress.processedFiles - reviewProgress.successfulFiles - reviewProgress.failedFiles}
              </div>
              <div className="text-xs text-slate-400">In Progress</div>
            </div>
            <div className="text-center">
              <div className="text-xl font-bold text-red-400">
                {reviewProgress.failedFiles}
              </div>
              <div className="text-xs text-slate-400">Failed</div>
            </div>
          </div>
        </div>
      </Card>

      {/* Batch Progress Grid */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-semibold text-white">Batch Progress</h3>
          <div className="flex items-center gap-2 text-sm text-slate-400">
            <div className="flex items-center gap-1">
              <div className="w-3 h-3 bg-green-500 rounded"></div>
              <span>Completed</span>
            </div>
            <div className="flex items-center gap-1">
              <div className="w-3 h-3 bg-blue-500 rounded"></div>
              <span>Running</span>
            </div>
            <div className="flex items-center gap-1">
              <div className="w-3 h-3 bg-slate-500 rounded"></div>
              <span>Pending</span>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {batchProgress.map((batch) => {
            const batchPercentage = batch.totalFiles > 0 
              ? (batch.processedFiles / batch.totalFiles) * 100 
              : 0;
            const batchSuccessRate = batch.processedFiles > 0 
              ? (batch.successCount / batch.processedFiles) * 100 
              : 0;

            return (
              <Card key={batch.batchId} className="p-4">
                {/* Batch Header */}
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <div className={`w-3 h-3 rounded-full ${getBatchStatusColor(batch.status)}`}></div>
                    <span className="font-medium text-white">
                      Batch {batch.batchNumber}
                    </span>
                  </div>
                  <Badge 
                    variant={
                      batch.status === 'completed' ? 'success' :
                      batch.status === 'running' ? 'primary' :
                      batch.status === 'failed' ? 'danger' :
                      'default'
                    }
                    size="sm"
                  >
                    {batch.status}
                  </Badge>
                </div>

                {/* Progress Bar */}
                <div className="mb-3">
                  <div className="flex justify-between text-xs text-slate-400 mb-1">
                    <span>{batch.processedFiles}/{batch.totalFiles} files</span>
                    <span>{Math.round(batchPercentage)}%</span>
                  </div>
                  <div className="w-full bg-slate-700 rounded-full h-2">
                    <div 
                      className={`h-2 rounded-full transition-all duration-500 ${getBatchStatusColor(batch.status)}`}
                      style={{ width: `${batchPercentage}%` }}
                    ></div>
                  </div>
                </div>

                {/* Batch Stats */}
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div className="flex items-center justify-between">
                    <span className="text-slate-400">Success:</span>
                    <span className="text-green-400 font-medium">{batch.successCount}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-400">Errors:</span>
                    <span className="text-red-400 font-medium">{batch.errorCount}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-400">Retries:</span>
                    <span className="text-yellow-400 font-medium">{batch.retryCount}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-slate-400">Repairs:</span>
                    <span className="text-orange-400 font-medium">{batch.jsonRepairs}</span>
                  </div>
                </div>

                {/* Timing Info */}
                {batch.startTime && (
                  <div className="mt-3 pt-3 border-t border-slate-700 text-xs text-slate-400">
                    {batch.status === 'running' ? (
                      <div className="flex justify-between">
                        <span>Running for:</span>
                        <span>{calculateElapsedTime(batch.startTime)}</span>
                      </div>
                    ) : batch.endTime ? (
                      <div className="flex justify-between">
                        <span>Duration:</span>
                        <span>{calculateElapsedTime(batch.startTime)} - {calculateElapsedTime(batch.endTime)}</span>
                      </div>
                    ) : null}
                    
                    {batch.estimatedCompletion && batch.status === 'running' && (
                      <div className="flex justify-between mt-1">
                        <span>ETA:</span>
                        <span>{batch.estimatedCompletion}</span>
                      </div>
                    )}
                  </div>
                )}
              </Card>
            );
          })}
        </div>
      </div>

      {/* Performance Summary */}
      <Card className="border-l-4 border-l-green-500">
        <div className="p-4">
          <h4 className="text-sm font-medium text-white mb-3">Performance Summary</h4>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div className="text-center">
              <div className="text-lg font-bold text-green-400 mb-1">
                {successRate.toFixed(1)}%
              </div>
              <div className="text-xs text-slate-400">Success Rate</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-blue-400 mb-1">
                {batchProgress.filter(b => b.status === 'completed').length}
              </div>
              <div className="text-xs text-slate-400">Batches Done</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-orange-400 mb-1">
                {batchProgress.reduce((sum, b) => sum + b.jsonRepairs, 0)}
              </div>
              <div className="text-xs text-slate-400">JSON Repairs</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-yellow-400 mb-1">
                {batchProgress.reduce((sum, b) => sum + b.retryCount, 0)}
              </div>
              <div className="text-xs text-slate-400">Total Retries</div>
            </div>
          </div>
        </div>
      </Card>

      {/* Live indicator */}
      {isLive && (
        <div className="flex items-center justify-center gap-2 text-xs text-slate-400">
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
          <span>Live progress updates</span>
        </div>
      )}
    </div>
  );
}