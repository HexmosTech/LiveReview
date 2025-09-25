import React, { useState, useEffect } from 'react';
import { Card, Button, Input, Badge, Icons } from '../UIPrimitives';

interface EventFiltersProps {
  onFiltersChange: (filters: EventFilters) => void;
  availableBatches: string[];
  totalEvents: number;
  filteredEvents: number;
  className?: string;
}

interface EventFilters {
  eventTypes: string[];
  batchIds: string[];
  statusTypes: string[];
  timeRange: TimeRange | null;
  searchQuery: string;
  preset: string | null;
}

interface TimeRange {
  start: string;
  end: string;
}

const EVENT_TYPES = [
  { id: 'started', label: 'Started', icon: '🚀' },
  { id: 'progress', label: 'Progress', icon: '📊' },
  { id: 'batch_complete', label: 'Batch Complete', icon: '✅' },
  { id: 'retry', label: 'Retries', icon: '🔄' },
  { id: 'json_repair', label: 'JSON Repairs', icon: '⚡' },
  { id: 'timeout', label: 'Timeouts', icon: '⏱️' },
  { id: 'error', label: 'Errors', icon: '❌' },
  { id: 'completed', label: 'Completed', icon: '🎉' },
];

const STATUS_TYPES = [
  { id: 'success', label: 'Success', color: 'success' },
  { id: 'warning', label: 'Warning', color: 'warning' },
  { id: 'error', label: 'Error', color: 'danger' },
  { id: 'info', label: 'Info', color: 'info' },
];

const FILTER_PRESETS = [
  {
    id: 'all',
    label: 'All Events',
    description: 'Show all events',
    filters: {
      eventTypes: [] as string[],
      batchIds: [] as string[],
      statusTypes: [] as string[],
      timeRange: null as TimeRange | null,
      searchQuery: '',
      preset: 'all'
    }
  },
  {
    id: 'errors_only',
    label: 'Errors & Retries',
    description: 'Show only errors and retry attempts',
    filters: {
      eventTypes: ['retry', 'error', 'timeout'],
      batchIds: [],
      statusTypes: ['error', 'warning'],
      timeRange: null,
      searchQuery: '',
      preset: 'errors_only'
    }
  },
  {
    id: 'batch_summary',
    label: 'Batch Summary',
    description: 'Show batch starts, completions, and major milestones',
    filters: {
      eventTypes: ['started', 'batch_complete', 'completed'],
      batchIds: [],
      statusTypes: [],
      timeRange: null,
      searchQuery: '',
      preset: 'batch_summary'
    }
  },
  {
    id: 'resiliency_events',
    label: 'Resiliency Events',
    description: 'Show retries, JSON repairs, and recovery actions',
    filters: {
      eventTypes: ['retry', 'json_repair', 'timeout'],
      batchIds: [],
      statusTypes: [],
      timeRange: null,
      searchQuery: '',
      preset: 'resiliency_events'
    }
  },
  {
    id: 'timing_details',
    label: 'Timing Details',
    description: 'Show progress and timing-related events',
    filters: {
      eventTypes: ['progress', 'batch_complete', 'timeout'],
      batchIds: [],
      statusTypes: [],
      timeRange: null,
      searchQuery: '',
      preset: 'timing_details'
    }
  }
];

export default function EventFilters({ 
  onFiltersChange, 
  availableBatches, 
  totalEvents, 
  filteredEvents,
  className 
}: EventFiltersProps) {
  const [filters, setFilters] = useState<EventFilters>({
    eventTypes: [],
    batchIds: [],
    statusTypes: [],
    timeRange: null,
    searchQuery: '',
    preset: 'all'
  });

  const [isExpanded, setIsExpanded] = useState(false);
  const [searchDebounceTimer, setSearchDebounceTimer] = useState<NodeJS.Timeout | null>(null);

  useEffect(() => {
    onFiltersChange(filters);
  }, [filters, onFiltersChange]);

  const handlePresetChange = (presetId: string) => {
    const preset = FILTER_PRESETS.find(p => p.id === presetId);
    if (preset) {
      setFilters(preset.filters);
    }
  };

  const handleEventTypeToggle = (eventType: string) => {
    setFilters(prev => ({
      ...prev,
      eventTypes: prev.eventTypes.includes(eventType)
        ? prev.eventTypes.filter(t => t !== eventType)
        : [...prev.eventTypes, eventType],
      preset: null
    }));
  };

  const handleBatchToggle = (batchId: string) => {
    setFilters(prev => ({
      ...prev,
      batchIds: prev.batchIds.includes(batchId)
        ? prev.batchIds.filter(b => b !== batchId)
        : [...prev.batchIds, batchId],
      preset: null
    }));
  };

  const handleStatusTypeToggle = (statusType: string) => {
    setFilters(prev => ({
      ...prev,
      statusTypes: prev.statusTypes.includes(statusType)
        ? prev.statusTypes.filter(s => s !== statusType)
        : [...prev.statusTypes, statusType],
      preset: null
    }));
  };

  const handleSearchChange = (query: string) => {
    if (searchDebounceTimer) {
      clearTimeout(searchDebounceTimer);
    }

    const timer = setTimeout(() => {
      setFilters(prev => ({
        ...prev,
        searchQuery: query,
        preset: null
      }));
    }, 300);

    setSearchDebounceTimer(timer);
  };

  const handleTimeRangeChange = (timeRange: TimeRange | null) => {
    setFilters(prev => ({
      ...prev,
      timeRange,
      preset: null
    }));
  };

  const clearAllFilters = () => {
    handlePresetChange('all');
  };

  const hasActiveFilters = filters.eventTypes.length > 0 || 
                          filters.batchIds.length > 0 || 
                          filters.statusTypes.length > 0 || 
                          filters.searchQuery.length > 0 || 
                          filters.timeRange !== null;

  return (
    <div className={`space-y-4 ${className}`}>
      {/* Filter Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="flex items-center gap-2 px-3 py-2 text-sm font-medium text-white bg-slate-700 hover:bg-slate-600 rounded-lg transition-colors"
          >
            <Icons.Filter />
            <span>Filters</span>
            {isExpanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}
          </button>

          {hasActiveFilters && (
            <Badge variant="primary" size="sm">
              {filteredEvents} of {totalEvents} events
            </Badge>
          )}
        </div>

        {hasActiveFilters && (
          <Button
            variant="ghost"
            size="sm"
            onClick={clearAllFilters}
            className="text-slate-400 hover:text-white"
          >
            Clear all
          </Button>
        )}
      </div>

      {/* Quick Presets */}
      <div className="flex flex-wrap gap-2">
        {FILTER_PRESETS.map(preset => (
          <button
            key={preset.id}
            onClick={() => handlePresetChange(preset.id)}
            className={`px-3 py-2 text-sm rounded-lg border transition-colors ${
              filters.preset === preset.id
                ? 'bg-blue-600 text-white border-blue-600'
                : 'bg-transparent text-slate-300 border-slate-600 hover:border-slate-500 hover:text-white'
            }`}
            title={preset.description}
          >
            {preset.label}
          </button>
        ))}
      </div>

      {/* Expanded Filters */}
      {isExpanded && (
        <Card className="p-0">
          <div className="p-4 space-y-6">
            {/* Search */}
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-2">
                Search Events
              </label>
              <Input
                placeholder="Search in messages, filenames, error details..."
                icon={<Icons.Search />}
                onChange={(e) => handleSearchChange(e.target.value)}
                className="w-full"
              />
            </div>

            {/* Event Types */}
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-3">
                Event Types
              </label>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                {EVENT_TYPES.map(eventType => (
                  <button
                    key={eventType.id}
                    onClick={() => handleEventTypeToggle(eventType.id)}
                    className={`flex items-center gap-2 px-3 py-2 text-sm rounded-lg border transition-colors ${
                      filters.eventTypes.includes(eventType.id)
                        ? 'bg-blue-600 text-white border-blue-600'
                        : 'bg-transparent text-slate-300 border-slate-600 hover:border-slate-500'
                    }`}
                  >
                    <span>{eventType.icon}</span>
                    <span>{eventType.label}</span>
                  </button>
                ))}
              </div>
            </div>

            {/* Status Types */}
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-3">
                Severity Levels
              </label>
              <div className="flex flex-wrap gap-2">
                {STATUS_TYPES.map(statusType => (
                  <button
                    key={statusType.id}
                    onClick={() => handleStatusTypeToggle(statusType.id)}
                    className={`px-3 py-2 text-sm rounded-lg border transition-colors ${
                      filters.statusTypes.includes(statusType.id)
                        ? 'bg-blue-600 text-white border-blue-600'
                        : 'bg-transparent text-slate-300 border-slate-600 hover:border-slate-500'
                    }`}
                  >
                    {statusType.label}
                  </button>
                ))}
              </div>
            </div>

            {/* Batch IDs */}
            {availableBatches.length > 0 && (
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-3">
                  Batches ({availableBatches.length} available)
                </label>
                <div className="max-h-32 overflow-y-auto">
                  <div className="grid grid-cols-3 md:grid-cols-6 gap-2">
                    {availableBatches.map(batchId => (
                      <button
                        key={batchId}
                        onClick={() => handleBatchToggle(batchId)}
                        className={`px-2 py-1 text-xs rounded border transition-colors ${
                          filters.batchIds.includes(batchId)
                            ? 'bg-blue-600 text-white border-blue-600'
                            : 'bg-transparent text-slate-300 border-slate-600 hover:border-slate-500'
                        }`}
                      >
                        {batchId}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {/* Time Range */}
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-3">
                Time Range
              </label>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs text-slate-400 mb-1">From</label>
                  <input
                    type="datetime-local"
                    className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white focus:border-blue-500 focus:outline-none"
                    onChange={(e) => {
                      const newTimeRange = filters.timeRange || { start: '', end: '' };
                      handleTimeRangeChange({
                        ...newTimeRange,
                        start: e.target.value
                      });
                    }}
                  />
                </div>
                <div>
                  <label className="block text-xs text-slate-400 mb-1">To</label>
                  <input
                    type="datetime-local"
                    className="w-full px-3 py-2 text-sm bg-slate-700 border border-slate-600 rounded-lg text-white focus:border-blue-500 focus:outline-none"
                    onChange={(e) => {
                      const newTimeRange = filters.timeRange || { start: '', end: '' };
                      handleTimeRangeChange({
                        ...newTimeRange,
                        end: e.target.value
                      });
                    }}
                  />
                </div>
              </div>
              {filters.timeRange && (
                <button
                  onClick={() => handleTimeRangeChange(null)}
                  className="mt-2 text-xs text-slate-400 hover:text-white"
                >
                  Clear time range
                </button>
              )}
            </div>
          </div>
        </Card>
      )}

      {/* Active Filters Summary */}
      {hasActiveFilters && (
        <div className="flex flex-wrap gap-2">
          {filters.eventTypes.map(eventType => (
            <Badge
              key={`event-${eventType}`}
              variant="primary"
              size="sm"
              className="flex items-center gap-1"
            >
              {EVENT_TYPES.find(t => t.id === eventType)?.label}
              <button
                onClick={() => handleEventTypeToggle(eventType)}
                className="ml-1 hover:text-red-300"
              >
                ×
              </button>
            </Badge>
          ))}
          
          {filters.statusTypes.map(statusType => (
            <Badge
              key={`status-${statusType}`}
              variant="warning"
              size="sm"
              className="flex items-center gap-1"
            >
              {STATUS_TYPES.find(s => s.id === statusType)?.label}
              <button
                onClick={() => handleStatusTypeToggle(statusType)}
                className="ml-1 hover:text-red-300"
              >
                ×
              </button>
            </Badge>
          ))}
          
          {filters.batchIds.map(batchId => (
            <Badge
              key={`batch-${batchId}`}
              variant="info"
              size="sm"
              className="flex items-center gap-1"
            >
              {batchId}
              <button
                onClick={() => handleBatchToggle(batchId)}
                className="ml-1 hover:text-red-300"
              >
                ×
              </button>
            </Badge>
          ))}
          
          {filters.searchQuery && (
            <Badge
              variant="default"
              size="sm"
              className="flex items-center gap-1"
            >
              Search: "{filters.searchQuery}"
              <button
                onClick={() => handleSearchChange('')}
                className="ml-1 hover:text-red-300"
              >
                ×
              </button>
            </Badge>
          )}
        </div>
      )}
    </div>
  );
}