import React, { useMemo } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';

interface TimingChartProps {
  batches: BatchTiming[];
  totalDuration?: string;
  className?: string;
}

interface BatchTiming {
  id: string;
  batchNumber: number;
  startTime: string;
  endTime?: string;
  status: 'completed' | 'running' | 'failed' | 'pending';
  files: FileTiming[];
  avgResponseTime: string;
  totalFiles: number;
  completedFiles: number;
}

interface FileTiming {
  filename: string;
  startTime: string;
  endTime?: string;
  responseTime?: string;
  status: 'completed' | 'processing' | 'failed' | 'pending';
}

export default function TimingChart({ batches, totalDuration, className }: TimingChartProps) {
  const chartData = useMemo(() => {
    if (batches.length === 0) return null;

    // Find overall start and end times
    const allStartTimes = batches.map(b => new Date(b.startTime).getTime());
    const allEndTimes = batches
      .filter(b => b.endTime)
      .map(b => new Date(b.endTime!).getTime());
    
    const overallStart = Math.min(...allStartTimes);
    const overallEnd = allEndTimes.length > 0 ? Math.max(...allEndTimes) : Date.now();
    const totalDurationMs = overallEnd - overallStart;

    return {
      overallStart,
      overallEnd,
      totalDurationMs,
      batches: batches.map(batch => {
        const batchStart = new Date(batch.startTime).getTime();
        const batchEnd = batch.endTime ? new Date(batch.endTime).getTime() : Date.now();
        const startOffset = ((batchStart - overallStart) / totalDurationMs) * 100;
        const duration = ((batchEnd - batchStart) / totalDurationMs) * 100;

        return {
          ...batch,
          startOffset,
          duration,
          batchDurationMs: batchEnd - batchStart,
          isRunning: !batch.endTime,
        };
      }),
    };
  }, [batches]);

  const formatDuration = (durationMs: number) => {
    const seconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);

    if (hours > 0) {
      return `${hours}h ${minutes % 60}m ${seconds % 60}s`;
    } else if (minutes > 0) {
      return `${minutes}m ${seconds % 60}s`;
    } else {
      return `${seconds}s`;
    }
  };

  const formatTime = (timestamp: string) => {
    return new Date(timestamp).toLocaleTimeString([], { 
      hour: '2-digit', 
      minute: '2-digit', 
      second: '2-digit' 
    });
  };

  const getBatchColor = (status: BatchTiming['status']) => {
    switch (status) {
      case 'completed':
        return 'bg-green-500';
      case 'running':
        return 'bg-blue-500';
      case 'failed':
        return 'bg-red-500';
      case 'pending':
        return 'bg-yellow-500';
      default:
        return 'bg-slate-500';
    }
  };

  const getBatchStatusIcon = (status: BatchTiming['status']) => {
    switch (status) {
      case 'completed':
        return <Icons.Success />;
      case 'running':
        return <div className="animate-spin"><Icons.Refresh /></div>;
      case 'failed':
        return <Icons.Error />;
      case 'pending':
        return <Icons.Clock />;
      default:
        return <Icons.Clock />;
    }
  };

  if (!chartData) {
    return (
      <Card className={`p-6 ${className}`}>
        <div className="text-center text-slate-400">
          <Icons.EmptyState />
          <p className="mt-2">No timing data available</p>
        </div>
      </Card>
    );
  }

  return (
    <div className={`space-y-4 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-white">Batch Timing Chart</h3>
          <p className="text-sm text-slate-400">
            {batches.length} batches • {totalDuration || formatDuration(chartData.totalDurationMs)} total
          </p>
        </div>
        
        {/* Legend */}
        <div className="flex items-center gap-4 text-xs">
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-green-500 rounded"></div>
            <span className="text-slate-300">Completed</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-blue-500 rounded"></div>
            <span className="text-slate-300">Running</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-red-500 rounded"></div>
            <span className="text-slate-300">Failed</span>
          </div>
          <div className="flex items-center gap-1">
            <div className="w-3 h-3 bg-yellow-500 rounded"></div>
            <span className="text-slate-300">Pending</span>
          </div>
        </div>
      </div>

      {/* Timeline Chart */}
      <Card className="p-0">
        <div className="p-4">
          {/* Time axis */}
          <div className="flex justify-between text-xs text-slate-400 mb-2">
            <span>{formatTime(new Date(chartData.overallStart).toISOString())}</span>
            <span>Duration: {formatDuration(chartData.totalDurationMs)}</span>
            <span>{formatTime(new Date(chartData.overallEnd).toISOString())}</span>
          </div>

          {/* Chart area */}
          <div className="space-y-3">
            {chartData.batches.map((batch, index) => (
              <div key={batch.id} className="relative">
                {/* Batch row */}
                <div className="flex items-center gap-3 mb-2">
                  <div className="w-20 text-sm text-slate-300 shrink-0">
                    Batch {batch.batchNumber}
                  </div>
                  
                  {/* Timeline bar container */}
                  <div className="flex-1 relative h-8 bg-slate-800 rounded border border-slate-700">
                    {/* Background grid lines */}
                    <div className="absolute inset-0 flex">
                      {[...Array(10)].map((_, i) => (
                        <div 
                          key={i} 
                          className="flex-1 border-r border-slate-700 last:border-r-0"
                        ></div>
                      ))}
                    </div>
                    
                    {/* Batch duration bar */}
                    <div 
                      className={`absolute top-0 bottom-0 rounded ${getBatchColor(batch.status)} ${
                        batch.isRunning ? 'animate-pulse' : ''
                      } transition-all duration-300`}
                      style={{
                        left: `${batch.startOffset}%`,
                        width: `${batch.duration}%`,
                      }}
                      title={`Batch ${batch.batchNumber}: ${formatDuration(batch.batchDurationMs)}`}
                    >
                      {/* Progress indicator for running batches */}
                      {batch.isRunning && batch.completedFiles > 0 && (
                        <div 
                          className="absolute top-0 bottom-0 bg-white/20 rounded transition-all duration-300"
                          style={{ 
                            width: `${(batch.completedFiles / batch.totalFiles) * 100}%` 
                          }}
                        ></div>
                      )}
                    </div>
                    
                    {/* Overlap indicators */}
                    {index > 0 && (
                      (() => {
                        const prevBatch = chartData.batches[index - 1];
                        const currentStart = batch.startOffset;
                        const prevEnd = prevBatch.startOffset + prevBatch.duration;
                        
                        if (currentStart < prevEnd && prevBatch.status !== 'pending') {
                          return (
                            <div 
                              className="absolute top-0 bottom-0 bg-orange-500/30 border-2 border-orange-500 rounded"
                              style={{
                                left: `${currentStart}%`,
                                width: `${Math.min(prevEnd - currentStart, batch.duration)}%`,
                              }}
                              title="Batch overlap detected"
                            ></div>
                          );
                        }
                        return null;
                      })()
                    )}
                  </div>
                  
                  {/* Batch info */}
                  <div className="w-32 text-right shrink-0">
                    <div className="flex items-center justify-end gap-2">
                      {getBatchStatusIcon(batch.status)}
                      <span className="text-xs text-slate-300">
                        {formatDuration(batch.batchDurationMs)}
                      </span>
                    </div>
                    <div className="text-xs text-slate-400">
                      {batch.completedFiles}/{batch.totalFiles} files
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </Card>

      {/* Summary Statistics */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card className="p-4">
          <div className="text-center">
            <div className="text-2xl font-bold text-white mb-1">
              {batches.length}
            </div>
            <div className="text-sm text-slate-400">Total Batches</div>
          </div>
        </Card>
        
        <Card className="p-4">
          <div className="text-center">
            <div className="text-2xl font-bold text-white mb-1">
              {batches.filter(b => b.status === 'completed').length}
            </div>
            <div className="text-sm text-slate-400">Completed</div>
          </div>
        </Card>
        
        <Card className="p-4">
          <div className="text-center">
            <div className="text-2xl font-bold text-white mb-1">
              {batches.reduce((sum, b) => sum + b.totalFiles, 0)}
            </div>
            <div className="text-sm text-slate-400">Total Files</div>
          </div>
        </Card>
        
        <Card className="p-4">
          <div className="text-center">
            <div className="text-2xl font-bold text-white mb-1">
              {chartData.batches.filter((_, i) => 
                i > 0 && chartData.batches[i].startOffset < 
                (chartData.batches[i - 1].startOffset + chartData.batches[i - 1].duration)
              ).length}
            </div>
            <div className="text-sm text-slate-400">Overlapping</div>
          </div>
        </Card>
      </div>

      {/* Performance Insights */}
      {chartData.batches.some((_, i) => 
        i > 0 && chartData.batches[i].startOffset < 
        (chartData.batches[i - 1].startOffset + chartData.batches[i - 1].duration)
      ) && (
        <Card className="border-l-4 border-l-orange-500">
          <div className="flex items-center gap-3 p-4">
            <Icons.Warning />
            <div>
              <div className="text-sm font-medium text-orange-300">
                Batch Overlap Detected
              </div>
              <div className="text-xs text-slate-400">
                Some batches are running concurrently. This may impact performance or resource usage.
              </div>
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}