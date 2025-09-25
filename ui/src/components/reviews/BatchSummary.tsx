import React, { useState } from 'react';
import { Card, Badge, Icons } from '../UIPrimitives';

interface BatchSummaryProps {
  batches: BatchInfo[];
  className?: string;
}

interface BatchInfo {
  id: string;
  batchNumber: number;
  startTime: string;
  endTime?: string;
  status: 'running' | 'completed' | 'failed' | 'pending';
  totalFiles: number;
  processedFiles: number;
  successCount: number;
  retryCount: number;
  jsonRepairs: number;
  timeoutCount: number;
  avgResponseTime: string;
  estimatedTimeRemaining?: string;
  files: BatchFile[];
}

interface BatchFile {
  filename: string;
  status: 'success' | 'retry' | 'failed' | 'processing' | 'pending';
  responseTime?: string;
  errorMessage?: string;
  repairApplied?: boolean;
}

export default function BatchSummary({ batches, className }: BatchSummaryProps) {
  const [expandedBatches, setExpandedBatches] = useState<Set<string>>(new Set());

  const toggleBatch = (batchId: string) => {
    const newExpanded = new Set(expandedBatches);
    if (newExpanded.has(batchId)) {
      newExpanded.delete(batchId);
    } else {
      newExpanded.add(batchId);
    }
    setExpandedBatches(newExpanded);
  };

  const getBatchStatusIcon = (status: BatchInfo['status']) => {
    switch (status) {
      case 'completed':
        return <Icons.Success />;
      case 'failed':
        return <Icons.Error />;
      case 'running':
        return <div className="animate-spin"><Icons.Refresh /></div>;
      case 'pending':
        return <Icons.Clock />;
      default:
        return <Icons.Clock />;
    }
  };

  const getBatchStatusColor = (status: BatchInfo['status']) => {
    switch (status) {
      case 'completed':
        return 'success';
      case 'failed':
        return 'danger';
      case 'running':
        return 'primary';
      case 'pending':
        return 'warning';
      default:
        return 'default';
    }
  };

  const getFileStatusIcon = (status: BatchFile['status']) => {
    switch (status) {
      case 'success':
        return <span className="text-green-400">✓</span>;
      case 'failed':
        return <span className="text-red-400">✗</span>;
      case 'retry':
        return <span className="text-yellow-400">↻</span>;
      case 'processing':
        return <span className="text-blue-400 animate-pulse">●</span>;
      case 'pending':
        return <span className="text-slate-500">○</span>;
      default:
        return <span className="text-slate-500">○</span>;
    }
  };

  const formatDuration = (startTime: string, endTime?: string) => {
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

  const getProgressPercentage = (batch: BatchInfo) => {
    if (batch.totalFiles === 0) return 0;
    return (batch.processedFiles / batch.totalFiles) * 100;
  };

  return (
    <div className={`space-y-4 ${className}`}>
      {/* Timeline Overview */}
      <div className="mb-6">
        <h3 className="text-lg font-semibold text-white mb-3">Batch Timeline</h3>
        <div className="relative">
          {/* Timeline line */}
          <div className="absolute left-4 top-0 bottom-0 w-0.5 bg-slate-600"></div>
          
          {batches.map((batch, index) => {
            const progressPercentage = getProgressPercentage(batch);
            const isExpanded = expandedBatches.has(batch.id);
            
            return (
              <div key={batch.id} className="relative flex items-start mb-6 last:mb-0">
                {/* Timeline dot */}
                <div className="relative z-10 w-8 h-8 bg-slate-800 border-2 border-slate-600 rounded-full flex items-center justify-center">
                  {getBatchStatusIcon(batch.status)}
                </div>
                
                {/* Batch content */}
                <div className="ml-4 flex-1">
                  <Card className="border-l-4 border-l-blue-500">
                    {/* Batch header */}
                    <div 
                      className="flex items-center justify-between p-4 cursor-pointer hover:bg-slate-700/50 transition-colors"
                      onClick={() => toggleBatch(batch.id)}
                    >
                      <div className="flex items-center gap-3">
                        <div className="flex items-center gap-2">
                          {isExpanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}
                          <span className="text-lg font-medium text-white">
                            Batch {batch.batchNumber}
                          </span>
                        </div>
                        <Badge variant={getBatchStatusColor(batch.status)}>
                          {batch.status}
                        </Badge>
                      </div>
                      
                      <div className="flex items-center gap-4 text-sm text-slate-300">
                        <span>{formatDuration(batch.startTime, batch.endTime)}</span>
                        <span>{batch.processedFiles}/{batch.totalFiles} files</span>
                      </div>
                    </div>

                    {/* Progress bar */}
                    <div className="px-4 pb-2">
                      <div className="w-full bg-slate-700 rounded-full h-2">
                        <div 
                          className={`h-2 rounded-full transition-all duration-300 ${
                            batch.status === 'completed' ? 'bg-green-500' :
                            batch.status === 'failed' ? 'bg-red-500' :
                            batch.status === 'running' ? 'bg-blue-500' :
                            'bg-yellow-500'
                          }`}
                          style={{ width: `${progressPercentage}%` }}
                        ></div>
                      </div>
                    </div>

                    {/* Quick stats */}
                    <div className="flex items-center gap-4 px-4 pb-4 text-sm">
                      <div className="flex items-center gap-1">
                        <Icons.Success />
                        <span className="text-green-400">{batch.successCount}</span>
                      </div>
                      {batch.retryCount > 0 && (
                        <div className="flex items-center gap-1">
                          <Icons.Refresh />
                          <span className="text-yellow-400">{batch.retryCount}</span>
                        </div>
                      )}
                      {batch.jsonRepairs > 0 && (
                        <div className="flex items-center gap-1">
                          <span className="text-orange-400">⚡</span>
                          <span className="text-orange-400">{batch.jsonRepairs}</span>
                        </div>
                      )}
                      {batch.timeoutCount > 0 && (
                        <div className="flex items-center gap-1">
                          <Icons.Error />
                          <span className="text-red-400">{batch.timeoutCount}</span>
                        </div>
                      )}
                      <div className="flex items-center gap-1 ml-auto">
                        <Icons.Clock />
                        <span className="text-slate-300">{batch.avgResponseTime}</span>
                      </div>
                    </div>

                    {/* Expanded content */}
                    {isExpanded && (
                      <div className="border-t border-slate-700 bg-slate-900/50">
                        {/* Batch details */}
                        <div className="p-4 border-b border-slate-700">
                          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                            <div>
                              <span className="text-slate-400">Start Time:</span>
                              <div className="text-white">{new Date(batch.startTime).toLocaleTimeString()}</div>
                            </div>
                            {batch.endTime && (
                              <div>
                                <span className="text-slate-400">End Time:</span>
                                <div className="text-white">{new Date(batch.endTime).toLocaleTimeString()}</div>
                              </div>
                            )}
                            <div>
                              <span className="text-slate-400">Avg Response:</span>
                              <div className="text-white">{batch.avgResponseTime}</div>
                            </div>
                            {batch.estimatedTimeRemaining && (
                              <div>
                                <span className="text-slate-400">ETA:</span>
                                <div className="text-white">{batch.estimatedTimeRemaining}</div>
                              </div>
                            )}
                          </div>
                        </div>

                        {/* File list */}
                        <div className="p-4">
                          <h4 className="text-sm font-medium text-slate-300 mb-3">Files ({batch.files.length})</h4>
                          <div className="space-y-2 max-h-48 overflow-y-auto">
                            {batch.files.map((file, fileIndex) => (
                              <div 
                                key={fileIndex}
                                className="flex items-center justify-between p-2 rounded bg-slate-800/50 hover:bg-slate-800 transition-colors"
                              >
                                <div className="flex items-center gap-3">
                                  {getFileStatusIcon(file.status)}
                                  <span className="text-sm text-slate-200 font-mono">
                                    {file.filename}
                                  </span>
                                  {file.repairApplied && (
                                    <Badge variant="warning" size="sm">
                                      Repaired
                                    </Badge>
                                  )}
                                </div>
                                
                                <div className="flex items-center gap-2 text-xs text-slate-400">
                                  {file.responseTime && (
                                    <span>{file.responseTime}</span>
                                  )}
                                  {file.errorMessage && (
                                    <span className="text-red-400 max-w-32 truncate" title={file.errorMessage}>
                                      {file.errorMessage}
                                    </span>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                  </Card>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}