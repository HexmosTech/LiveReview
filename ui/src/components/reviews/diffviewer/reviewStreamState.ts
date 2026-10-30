// Ported from git-lrc:internal/staticserve/static/components/review_stream_state.mjs#L1-L175
// (as of the git-lrc HEAD current when this port was written).
//
// Lets the Findings tab show results incrementally while a review is still
// in_progress: every time a batch of files finishes its LLM call, the
// backend emits a "batch"/"completed" event carrying that batch's raw
// comments (internal/ai/langchain/provider.go's EmitBatchComplete). This
// module extracts those comments from a page of GET .../events results and
// merges them into the file/hunk structure GetDiffReviewStatus returns, the
// same way git-lrc's local review UI has always done it.
import { DiffReviewComment, DiffReviewCommentSeverity, DiffReviewFile, ReviewEvent } from '../../../types/reviews';

const EVENT_POLL_LIMIT = 1000;
const EVENT_TIME_OVERLAP_MS = 1000;

function stableStringify(value: unknown): string {
  if (value === null || value === undefined) {
    return '';
  }
  if (Array.isArray(value)) {
    return `[${value.map(stableStringify).join(',')}]`;
  }
  if (typeof value === 'object') {
    const obj = value as Record<string, unknown>;
    return `{${Object.keys(obj).sort().map((key) => `${key}:${stableStringify(obj[key])}`).join(',')}}`;
  }
  return String(value);
}

function normalizeEventId(event: ReviewEvent | undefined, index: number): string {
  if (event?.id !== undefined && event?.id !== null) {
    return String(event.id);
  }
  return [
    event?.type || 'event',
    event?.time || 'unknown',
    event?.batchId || '',
    stableStringify(event?.data),
    index,
  ].join(':');
}

function normalizeSeverity(rawSeverity: unknown): DiffReviewCommentSeverity {
  const severity = String(rawSeverity || 'info').trim().toLowerCase();
  if (severity === 'critical' || severity === 'warning') {
    return severity as DiffReviewCommentSeverity;
  }
  return 'info';
}

function normalizeFilePath(file: Pick<DiffReviewFile, 'file_path'> | undefined): string {
  return file?.file_path || '';
}

// Builds the `since` param GetReviewEvents polling should pass for the next
// page, with the same small overlap window git-lrc's buildEventsURL applies
// (events?since=...), to avoid missing an event that landed exactly at the
// previous poll's cutoff. Call signature differs from the original .mjs
// (which returned a full URL) because LiveReview's getReviewEvents api
// client already builds the URL from (reviewId, since, limit) — this just
// supplies the `since` value.
export function sinceWithOverlap(lastSeenAt: string | null): string | undefined {
  if (!lastSeenAt) {
    return undefined;
  }
  const parsed = new Date(lastSeenAt);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }
  return new Date(parsed.getTime() - EVENT_TIME_OVERLAP_MS).toISOString();
}

export const EVENTS_POLL_LIMIT = EVENT_POLL_LIMIT;

export interface ExtractNewEventsResult {
  newEvents: ReviewEvent[];
  nextSeenEventIds: Set<string>;
  lastSeenAt: string | null;
}

export function extractNewEvents(events: ReviewEvent[] | undefined, seenEventIds: Set<string> | undefined): ExtractNewEventsResult {
  const nextSeenEventIds = new Set(seenEventIds || []);
  const newEvents: ReviewEvent[] = [];
  let lastSeenAt: string | null = null;

  (events || []).forEach((event, index) => {
    const eventId = normalizeEventId(event, index);
    const eventTime = event?.time || null;

    if (eventTime) {
      if (!lastSeenAt || new Date(eventTime).getTime() > new Date(lastSeenAt).getTime()) {
        lastSeenAt = eventTime;
      }
    }

    if (nextSeenEventIds.has(eventId)) {
      return;
    }

    nextSeenEventIds.add(eventId);
    newEvents.push(event);
  });

  return { newEvents, nextSeenEventIds, lastSeenAt };
}

export function inferReviewStatusFromEvents(events: ReviewEvent[] | undefined): string {
  const list = events || [];
  for (let index = list.length - 1; index >= 0; index--) {
    const event = list[index] || ({} as ReviewEvent);
    const eventData = event.data || {};
    if (event.type === 'status' && eventData.status) {
      return eventData.status;
    }
    if (event.type === 'completion') {
      return eventData.errorSummary ? 'failed' : 'completed';
    }
  }
  return '';
}

// Raw shape of a comment riding on a "batch" event's data.comments[] —
// marshaled straight from pkg/models.ReviewComment, which has no json tags,
// so every field comes over PascalCase (unlike DiffReviewComment's lower
// camelCase, which is GetDiffReviewStatus's own, separately-marshaled shape).
interface RawStreamedComment {
  FilePath?: string;
  filePath?: string;
  file_path?: string;
  Line?: number;
  line?: number;
  lineNumber?: number;
  line_number?: number;
  Content?: string;
  content?: string;
  Severity?: string;
  severity?: string;
  Confidence?: string;
  confidence?: string;
  Type?: string;
  type?: string;
  Category?: string;
  category?: string;
  Subcategory?: string;
  subcategory?: string;
  IsInternal?: boolean;
  isInternal?: boolean;
}

export function normalizeStreamedComment(rawComment: RawStreamedComment | null | undefined): DiffReviewComment & { file_path: string } | null {
  if (!rawComment) {
    return null;
  }

  const isInternal = Boolean(rawComment.IsInternal ?? rawComment.isInternal);
  if (isInternal) {
    return null;
  }

  const filePath = rawComment.FilePath || rawComment.filePath || rawComment.file_path || '';
  const line = Number(rawComment.Line ?? rawComment.line ?? rawComment.lineNumber ?? rawComment.line_number);
  const content = String(rawComment.Content ?? rawComment.content ?? '').trim();

  if (!filePath || !Number.isFinite(line) || line <= 0 || !content) {
    return null;
  }

  return {
    file_path: filePath,
    line,
    content,
    severity: normalizeSeverity(rawComment.Severity ?? rawComment.severity),
    confidence: String(rawComment.Confidence ?? rawComment.confidence ?? '').trim(),
    type: String(rawComment.Type ?? rawComment.type ?? '').trim(),
    category: String(rawComment.Category ?? rawComment.category ?? 'review').trim() || 'review',
    subcategory: String(rawComment.Subcategory ?? rawComment.subcategory ?? '').trim(),
  };
}

export function extractExternalCommentsFromEvents(events: ReviewEvent[] | undefined): (DiffReviewComment & { file_path: string })[] {
  const comments: (DiffReviewComment & { file_path: string })[] = [];

  (events || []).forEach((event) => {
    const eventData = event?.data || {};
    const rawComments = (eventData as { comments?: unknown }).comments;
    if (event?.type !== 'batch' || eventData.status !== 'completed' || !Array.isArray(rawComments)) {
      return;
    }
    rawComments.forEach((comment) => {
      const normalized = normalizeStreamedComment(comment as RawStreamedComment);
      if (normalized) {
        comments.push(normalized);
      }
    });
  });

  return comments;
}

export function appendStreamedCommentsToFiles(
  files: DiffReviewFile[] | undefined,
  incomingComments: (DiffReviewComment & { file_path: string })[] | undefined
): DiffReviewFile[] {
  if (!Array.isArray(files) || files.length === 0 || !Array.isArray(incomingComments) || incomingComments.length === 0) {
    return files || [];
  }

  const nextFiles = files.map((file) => ({
    ...file,
    comments: [...(file.comments || [])],
  }));

  const fileIndexByPath = new Map<string, number>();
  nextFiles.forEach((file, index) => {
    const filePath = normalizeFilePath(file);
    if (filePath) {
      fileIndexByPath.set(filePath, index);
    }
  });

  incomingComments.forEach((comment) => {
    const fileIndex = fileIndexByPath.get(comment.file_path);
    if (fileIndex === undefined) {
      return;
    }
    nextFiles[fileIndex].comments.push(comment);
  });

  return nextFiles;
}
