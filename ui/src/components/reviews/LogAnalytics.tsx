import React from 'react';
import { Card, Badge, StatCard, Icons } from '../UIPrimitives';

interface LogAnalyticsProps {
  reviewId: number;
  stats: {
    totalBatches: number;
    completedBatches: number;
    successRate: number;
    avgResponseTime: string;
    jsonRepairsNeeded: number;
    retriesRequired: number;
    timeoutsEncountered: number;
    circuitBreakerTrips: number;
    totalRequests: number;
    successfulRequests: number;
    errorCount: number;
  };
}

export default function LogAnalytics({ reviewId, stats }: LogAnalyticsProps) {
  const progressPercentage = stats.totalBatches > 0 
    ? (stats.completedBatches / stats.totalBatches) * 100 
    : 0;

  const getHealthStatus = () => {
    if (stats.successRate >= 95 && stats.jsonRepairsNeeded === 0) return 'excellent';
    if (stats.successRate >= 85 && stats.jsonRepairsNeeded <= 2) return 'good';
    if (stats.successRate >= 70) return 'fair';
    return 'poor';
  };

  const healthStatus = getHealthStatus();

  const getHealthBadgeVariant = () => {
    switch (healthStatus) {
      case 'excellent': return 'success';
      case 'good': return 'primary';
      case 'fair': return 'warning';
      case 'poor': return 'danger';
      default: return 'default';
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-2xl font-semibold text-white">Review Analytics</h2>
          <p className="text-sm text-slate-400">Review ID: {reviewId}</p>
        </div>
        <Badge 
          variant={getHealthBadgeVariant()}
          size="md"
          className="text-sm px-3 py-1"
        >
          {healthStatus.charAt(0).toUpperCase() + healthStatus.slice(1)} Health
        </Badge>
      </div>

      {/* Overall Progress Card */}
      <Card 
        title="Overall Progress" 
        badge={`${Math.round(progressPercentage)}%`}
        badgeColor={progressPercentage >= 90 ? 'bg-green-100 text-green-800' : 
                   progressPercentage >= 70 ? 'bg-blue-100 text-blue-800' : 
                   'bg-yellow-100 text-yellow-800'}
        className="border-l-4 border-l-blue-500"
      >
        <div className="space-y-3">
          {/* Progress Bar */}
          <div className="w-full bg-slate-700 rounded-full h-3">
            <div 
              className="bg-blue-500 h-3 rounded-full transition-all duration-300"
              style={{ width: `${progressPercentage}%` }}
            ></div>
          </div>
          <div className="flex justify-between text-sm text-slate-400">
            <span>{stats.completedBatches} of {stats.totalBatches} batches completed</span>
            <span>Success Rate: {stats.successRate.toFixed(1)}%</span>
          </div>
        </div>
      </Card>

      {/* Summary Cards Grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {/* Success Rate Card */}
        <StatCard
          title="Success Rate"
          value={`${stats.successRate.toFixed(1)}%`}
          description={`${stats.successfulRequests}/${stats.totalRequests} requests`}
          icon={<Icons.Success />}
          variant={stats.successRate >= 90 ? 'primary' : 'default'}
        />

        {/* Average Response Time Card */}
        <StatCard
          title="Avg Response Time"
          value={stats.avgResponseTime}
          description="Per batch average"
          icon={<Icons.Clock />}
          variant="default"
        />

        {/* JSON Repairs Card */}
        <StatCard
          title="JSON Repairs"
          value={stats.jsonRepairsNeeded}
          description={stats.jsonRepairsNeeded > 0 ? 'Repairs applied' : 'No repairs needed'}
          icon={<span className="text-lg">⚡</span>}
          variant={stats.jsonRepairsNeeded > 0 ? 'default' : 'primary'}
        />

        {/* Retries Card */}
        <StatCard
          title="Retries"
          value={stats.retriesRequired}
          description="Retry attempts"
          icon={<Icons.Refresh />}
          variant={stats.retriesRequired > 0 ? 'default' : 'primary'}
        />
      </div>

      {/* Detailed Statistics */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Batch Statistics */}
        <Card title="Batch Statistics" className="border-l-4 border-l-green-500">
          <div className="space-y-3">
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Total Batches</span>
              <Badge variant="default">{stats.totalBatches}</Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Completed</span>
              <Badge variant="success">{stats.completedBatches}</Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Success Rate</span>
              <Badge 
                variant={stats.successRate >= 90 ? "success" : stats.successRate >= 70 ? "primary" : "danger"}
              >
                {stats.successRate.toFixed(1)}%
              </Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Avg Response Time</span>
              <Badge variant="info">{stats.avgResponseTime}</Badge>
            </div>
          </div>
        </Card>

        {/* Resiliency Statistics */}
        <Card title="Resiliency Events" className="border-l-4 border-l-orange-500">
          <div className="space-y-3">
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">JSON Repairs</span>
              <Badge 
                variant={stats.jsonRepairsNeeded > 0 ? "warning" : "success"}
              >
                {stats.jsonRepairsNeeded}
              </Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Retries Required</span>
              <Badge 
                variant={stats.retriesRequired > 0 ? "warning" : "success"}
              >
                {stats.retriesRequired}
              </Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Timeouts</span>
              <Badge 
                variant={stats.timeoutsEncountered > 0 ? "danger" : "success"}
              >
                {stats.timeoutsEncountered}
              </Badge>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-sm text-slate-300">Circuit Breaker Trips</span>
              <Badge 
                variant={stats.circuitBreakerTrips > 0 ? "danger" : "success"}
              >
                {stats.circuitBreakerTrips}
              </Badge>
            </div>
          </div>
        </Card>
      </div>

      {/* Health Insights */}
      <Card title="Health Insights" className="border-l-4 border-l-purple-500">
        <div className="space-y-3">
          {healthStatus === 'excellent' && (
            <div className="flex items-center gap-3 text-green-300 bg-green-900/20 p-3 rounded-lg border border-green-700/30">
              <Icons.Success />
              <span className="text-sm">
                Excellent performance! All LLM responses are being processed successfully without issues.
              </span>
            </div>
          )}
          {healthStatus === 'good' && (
            <div className="flex items-center gap-3 text-blue-300 bg-blue-900/20 p-3 rounded-lg border border-blue-700/30">
              <Icons.Success />
              <span className="text-sm">
                Good performance with minimal JSON repairs needed. System is operating within normal parameters.
              </span>
            </div>
          )}
          {stats.jsonRepairsNeeded > 0 && (
            <div className="flex items-center gap-3 text-orange-300 bg-orange-900/20 p-3 rounded-lg border border-orange-700/30">
              <span className="text-lg">⚡</span>
              <span className="text-sm">
                JSON repair system successfully fixed {stats.jsonRepairsNeeded} malformed response(s). 
                Consider reviewing LLM prompt quality.
              </span>
            </div>
          )}
          {stats.retriesRequired > 0 && (
            <div className="flex items-center gap-3 text-yellow-300 bg-yellow-900/20 p-3 rounded-lg border border-yellow-700/30">
              <Icons.Refresh />
              <span className="text-sm">
                {stats.retriesRequired} retry attempt(s) were needed. Check network connectivity and LLM service status.
              </span>
            </div>
          )}
          {stats.timeoutsEncountered > 0 && (
            <div className="flex items-center gap-3 text-red-300 bg-red-900/20 p-3 rounded-lg border border-red-700/30">
              <Icons.Error />
              <span className="text-sm">
                {stats.timeoutsEncountered} timeout(s) occurred. Consider increasing timeout values or checking LLM service performance.
              </span>
            </div>
          )}
          {healthStatus === 'excellent' && stats.jsonRepairsNeeded === 0 && stats.retriesRequired === 0 && stats.timeoutsEncountered === 0 && (
            <div className="flex items-center gap-3 text-slate-300 bg-slate-800/50 p-3 rounded-lg border border-slate-600/30">
              <Icons.Info />
              <span className="text-sm">
                System is running optimally with no resiliency events detected.
              </span>
            </div>
          )}
        </div>
      </Card>
    </div>
  );
}