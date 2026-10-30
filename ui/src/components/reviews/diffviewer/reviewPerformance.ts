// Ported from git-lrc:internal/staticserve/static/components/review_performance_state.mjs#L1-L114
// (as of a1c3b3c). Pure, framework-agnostic timing helpers - no React - ported ~1:1 per
// this repo's porting convention. Tracks "time to first comment" (git-lrc's rough target
// is ~30s) both at the review level and per individual comment, so a user can see which
// findings streamed in late instead of just a single aggregate number.

const FALLBACK_ACTIVITY_LINES = [
  'Preparing review workspace',
  'Scanning changed files',
  'Analyzing diff structure',
  'Streaming review activity',
];

export function getPerformanceNow(): number {
  if (typeof performance !== 'undefined' && typeof performance.now === 'function') {
    return performance.now();
  }
  return Date.now();
}

export function formatDurationShort(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs < 0) {
    return '';
  }

  const totalSeconds = Math.max(0, Math.round(durationMs / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  }
  if (minutes > 0) {
    return seconds > 0 ? `${minutes}m ${seconds}s` : `${minutes}m`;
  }
  return `${seconds}s`;
}

export function recordFirstRenderTime(
  existingTimings: Record<string, number> | undefined,
  commentKey: string | undefined | null,
  renderMs: number
): Record<string, number> {
  const current = existingTimings || {};
  if (!commentKey || !Number.isFinite(renderMs)) {
    return current;
  }
  if (Object.prototype.hasOwnProperty.call(current, commentKey)) {
    return current;
  }
  return {
    ...current,
    [commentKey]: renderMs,
  };
}

export function getFirstRenderTime(renderTimings: Record<string, number> | undefined): number | null {
  const values = Object.values(renderTimings || {}).filter((value) => Number.isFinite(value));
  if (values.length === 0) {
    return null;
  }
  return Math.min(...values);
}

export interface PerformanceSnapshotInput {
  baselineMs?: number;
  nowMs?: number;
  firstCommentMs?: number | null;
  totalComments?: number;
  completedMs?: number;
}

export interface PerformanceSummaryItem {
  key: string;
  label: string;
  value: string;
}

export interface PerformanceSnapshot {
  elapsedMs: number;
  elapsedLabel: string;
  firstCommentElapsedMs: number | null;
  firstCommentLabel: string;
  totalComments: number;
  summaryItems: PerformanceSummaryItem[];
}

export function buildPerformanceSnapshot({
  baselineMs,
  nowMs,
  firstCommentMs,
  totalComments,
  completedMs,
}: PerformanceSnapshotInput): PerformanceSnapshot {
  const effectiveNow = Number.isFinite(completedMs) ? (completedMs as number) : nowMs;
  const elapsedMs =
    Number.isFinite(baselineMs) && Number.isFinite(effectiveNow)
      ? Math.max(0, (effectiveNow as number) - (baselineMs as number))
      : 0;
  const firstCommentElapsedMs =
    Number.isFinite(firstCommentMs) && Number.isFinite(baselineMs)
      ? Math.max(0, (firstCommentMs as number) - (baselineMs as number))
      : null;
  const normalizedComments = Number.isFinite(totalComments) ? Math.max(0, totalComments as number) : 0;
  const firstCommentValue =
    firstCommentElapsedMs === null
      ? Number.isFinite(completedMs) && normalizedComments === 0
        ? 'No comments'
        : 'Waiting'
      : formatDurationShort(firstCommentElapsedMs);

  return {
    elapsedMs,
    elapsedLabel: formatDurationShort(elapsedMs),
    firstCommentElapsedMs,
    firstCommentLabel: firstCommentValue,
    totalComments: normalizedComments,
    summaryItems: [
      { key: 'first-comment', label: 'First comment', value: firstCommentValue },
      { key: 'elapsed', label: Number.isFinite(completedMs) ? 'Final time' : 'Elapsed', value: formatDurationShort(elapsedMs) },
      { key: 'comments', label: 'Comments', value: String(normalizedComments) },
    ],
  };
}

export function getCommentRenderLabel(baselineMs: number | undefined, renderedAtMs: number | undefined): string {
  if (!Number.isFinite(baselineMs) || !Number.isFinite(renderedAtMs)) {
    return '';
  }
  return `Appeared in ${formatDurationShort(Math.max(0, (renderedAtMs as number) - (baselineMs as number)))}`;
}

export function getLoadingActivityMessage(events: { message?: string }[] | undefined, elapsedMs: number): string {
  const latestMessage = [...(events || [])]
    .reverse()
    .map((event) => String(event?.message || '').trim())
    .find(Boolean);

  if (latestMessage) {
    return latestMessage;
  }

  const safeElapsedMs = Number.isFinite(elapsedMs) ? Math.max(0, elapsedMs) : 0;
  const index = Math.floor(safeElapsedMs / 3000) % FALLBACK_ACTIVITY_LINES.length;
  return FALLBACK_ACTIVITY_LINES[index];
}
