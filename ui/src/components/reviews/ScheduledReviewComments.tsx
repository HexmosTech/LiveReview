import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Icons, Spinner } from '../UIPrimitives';
import { getDiffReviewStatus } from '../../api/reviews';
import { renderMarkdown } from '../../utils/renderMarkdown';
import { DiffFileData, DiffHunkData, DiffReviewCommentData, DiffReviewStatus } from '../../types/reviews';

const POLL_INTERVAL_MS = 5000;

interface ScheduledReviewCommentsProps {
  reviewId: number;
}

type DiffLine = {
  kind: 'add' | 'del' | 'context';
  text: string;
  oldLine?: number;
  newLine?: number;
};

// Parses a unified-diff hunk (header line + `+`/`-`/` `-prefixed body, as produced by
// GitHubProvider.parsePatchIntoHunks) into per-line records with old/new line numbers, so
// inline comments (keyed by new-line number) can be attached at the right spot.
const parseHunkLines = (hunk: DiffHunkData): DiffLine[] => {
  const rawLines = hunk.content.split('\n');
  const bodyLines = rawLines[0]?.startsWith('@@') ? rawLines.slice(1) : rawLines;

  let oldLine = hunk.old_start_line;
  let newLine = hunk.new_start_line;
  const out: DiffLine[] = [];

  for (const raw of bodyLines) {
    if (raw === '') continue;
    const marker = raw[0];
    const text = raw.slice(1);
    if (marker === '+') {
      out.push({ kind: 'add', text, newLine });
      newLine += 1;
    } else if (marker === '-') {
      out.push({ kind: 'del', text, oldLine });
      oldLine += 1;
    } else {
      out.push({ kind: 'context', text, oldLine, newLine });
      oldLine += 1;
      newLine += 1;
    }
  }
  return out;
};

type Severity = 'critical' | 'warning' | 'info';

const severityRank: Record<string, number> = { critical: 3, warning: 2, info: 1 };

const severityStyles: Record<Severity, { border: string; badgeBg: string; badgeText: string; label: string }> = {
  critical: { border: 'border-l-red-500', badgeBg: 'bg-red-500/15', badgeText: 'text-red-300', label: 'Critical' },
  warning: { border: 'border-l-amber-500', badgeBg: 'bg-amber-500/15', badgeText: 'text-amber-300', label: 'Warning' },
  info: { border: 'border-l-sky-500', badgeBg: 'bg-sky-500/15', badgeText: 'text-sky-300', label: 'Info' },
};

const styleForSeverity = (severity: string) => severityStyles[severity as Severity] || severityStyles.info;

const InlineComment: React.FC<{ comment: DiffReviewCommentData }> = ({ comment }) => {
  const style = styleForSeverity(comment.severity);
  return (
    <div className={`my-2 ml-[84px] mr-3 rounded-md border-l-2 ${style.border} bg-slate-800/90 py-2.5 pl-3 pr-3`}>
      <div className="mb-1 flex items-center gap-2">
        <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${style.badgeBg} ${style.badgeText}`}>
          {style.label}
        </span>
        {comment.category && (
          <span className="text-[11px] text-slate-500">
            {comment.category}
            {comment.subcategory ? ` · ${comment.subcategory}` : ''}
          </span>
        )}
      </div>
      <p className="whitespace-pre-wrap text-[13px] leading-relaxed text-slate-200">{comment.content}</p>
    </div>
  );
};

const DiffLineRow: React.FC<{ line: DiffLine }> = ({ line }) => {
  const bg =
    line.kind === 'add' ? 'bg-green-500/10 hover:bg-green-500/[0.14]' : line.kind === 'del' ? 'bg-red-500/10 hover:bg-red-500/[0.14]' : 'hover:bg-slate-700/30';
  const marker = line.kind === 'add' ? '+' : line.kind === 'del' ? '-' : '';
  const markerColor = line.kind === 'add' ? 'text-green-400' : line.kind === 'del' ? 'text-red-400' : 'text-slate-700';
  return (
    <div className={`flex font-mono text-[12.5px] leading-5 transition-colors ${bg}`}>
      <span className="w-10 shrink-0 select-none border-r border-slate-800/80 pr-2 text-right text-slate-600">{line.oldLine ?? ''}</span>
      <span className="w-10 shrink-0 select-none border-r border-slate-800/80 pr-2 text-right text-slate-600">{line.newLine ?? ''}</span>
      <span className={`w-5 shrink-0 select-none text-center ${markerColor}`}>{marker}</span>
      <span className="whitespace-pre pr-4 text-slate-200">{line.text || ' '}</span>
    </div>
  );
};

const DiffFileCard: React.FC<{ file: DiffFileData }> = ({ file }) => {
  const [expanded, setExpanded] = useState(true);

  const parsedHunks = useMemo(() => file.hunks.map((hunk) => ({ hunk, lines: parseHunkLines(hunk) })), [file.hunks]);

  const { additions, deletions } = useMemo(() => {
    let add = 0;
    let del = 0;
    parsedHunks.forEach(({ lines }) => {
      lines.forEach((l) => {
        if (l.kind === 'add') add += 1;
        if (l.kind === 'del') del += 1;
      });
    });
    return { additions: add, deletions: del };
  }, [parsedHunks]);

  const commentsByLine = useMemo(() => {
    const map = new Map<number, DiffReviewCommentData[]>();
    file.comments.forEach((c) => {
      const existing = map.get(c.line) || [];
      existing.push(c);
      map.set(c.line, existing);
    });
    return map;
  }, [file.comments]);

  const worstSeverity = file.comments.reduce<string | null>((worst, c) => {
    if (!worst || (severityRank[c.severity] || 0) > (severityRank[worst] || 0)) return c.severity;
    return worst;
  }, null);
  const commentBadgeStyle = worstSeverity ? styleForSeverity(worstSeverity) : null;

  return (
    <div className="mb-4 overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-center justify-between gap-3 bg-slate-900/60 px-4 py-2.5 transition-colors hover:bg-slate-900"
      >
        <div className="flex min-w-0 items-center gap-2">
          <span className="shrink-0 text-slate-500">{expanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}</span>
          <span className="truncate font-mono text-[13px] text-slate-200" title={file.file_path}>
            {file.file_path}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <span className="flex items-center gap-1.5 font-mono text-[11px]">
            {additions > 0 && <span className="text-green-400">+{additions}</span>}
            {deletions > 0 && <span className="text-red-400">-{deletions}</span>}
          </span>
          {file.comments.length > 0 && commentBadgeStyle && (
            <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${commentBadgeStyle.badgeBg} ${commentBadgeStyle.badgeText}`}>
              {file.comments.length} comment{file.comments.length === 1 ? '' : 's'}
            </span>
          )}
        </div>
      </button>
      {expanded && (
        <div className="overflow-x-auto">
          {parsedHunks.map(({ hunk, lines }, hunkIdx) => (
            <div key={hunkIdx} className="border-t border-slate-700/80">
              <div className="bg-slate-900/50 px-3 py-1.5 font-mono text-[11px] text-slate-500">
                @@ -{hunk.old_start_line},{hunk.old_line_count} +{hunk.new_start_line},{hunk.new_line_count} @@
              </div>
              {lines.map((line, lineIdx) => {
                const lineComments = line.newLine !== undefined ? commentsByLine.get(line.newLine) : undefined;
                return (
                  <React.Fragment key={lineIdx}>
                    <DiffLineRow line={line} />
                    {lineComments?.map((c, ci) => <InlineComment key={ci} comment={c} />)}
                  </React.Fragment>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};

const AISummaryCard: React.FC<{ summary: string; fileCount: number; commentCount: number; severityCounts: Record<string, number> }> = ({
  summary,
  fileCount,
  commentCount,
  severityCounts,
}) => (
  <div className="relative mb-4 overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
    <div className="h-0.5 w-full bg-gradient-to-r from-purple-500 via-purple-400/60 to-transparent" />
    <div className="p-5">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-purple-300">
          <Icons.AI />
          <span>AI Summary</span>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-[11px]">
          <span className="rounded-full border border-slate-600 bg-slate-900 px-2.5 py-1 text-slate-300">
            {fileCount} file{fileCount === 1 ? '' : 's'} changed
          </span>
          {commentCount > 0 && (
            <span className="rounded-full border border-slate-600 bg-slate-900 px-2.5 py-1 text-slate-300">
              {commentCount} comment{commentCount === 1 ? '' : 's'}
            </span>
          )}
          {(['critical', 'warning'] as const).map(
            (sev) =>
              severityCounts[sev] > 0 && (
                <span
                  key={sev}
                  className={`rounded-full px-2.5 py-1 font-medium ${styleForSeverity(sev).badgeBg} ${styleForSeverity(sev).badgeText}`}
                >
                  {severityCounts[sev]} {styleForSeverity(sev).label.toLowerCase()}
                </span>
              )
          )}
        </div>
      </div>
      {renderMarkdown(summary)}
    </div>
  </div>
);

// Renders the PR-comment-style diff view for a review, fetched via GET
// /api/v1/diff-review/:review_id. For scheduled reviews this endpoint fetches the diff live
// from the git provider on every call (the code is never stored in our database) and merges
// it server-side with the persisted comments.
const ScheduledReviewComments: React.FC<ScheduledReviewCommentsProps> = ({ reviewId }) => {
  const [data, setData] = useState<DiffReviewStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    let cancelled = false;

    const poll = async () => {
      try {
        const result = await getDiffReviewStatus(reviewId);
        if (cancelled) return;
        setData(result);
        setError(null);
        setLoading(false);
        if (result.status !== 'completed' && result.status !== 'failed') {
          pollRef.current = setTimeout(poll, POLL_INTERVAL_MS);
        }
      } catch (err) {
        if (cancelled) return;
        console.error('Error fetching diff review:', err);
        setError(err instanceof Error ? err.message : 'Failed to load comments');
        setLoading(false);
      }
    };

    void poll();

    return () => {
      cancelled = true;
      if (pollRef.current) {
        clearTimeout(pollRef.current);
        pollRef.current = null;
      }
    };
  }, [reviewId]);

  const allComments = useMemo(() => data?.files?.flatMap((f) => f.comments) ?? [], [data]);
  const severityCounts = useMemo(() => {
    const counts: Record<string, number> = { critical: 0, warning: 0, info: 0 };
    allComments.forEach((c) => {
      counts[c.severity] = (counts[c.severity] || 0) + 1;
    });
    return counts;
  }, [allComments]);

  return (
    <div>
      <div className="mb-4 flex items-center gap-2.5 rounded-md border border-purple-800/40 bg-purple-950/20 px-3.5 py-2.5 text-[13px] text-purple-200">
        <span className="shrink-0 text-purple-400">
          <Icons.Refresh />
        </span>
        <span>
          <span className="font-medium text-purple-100">Fetched live from GitHub.</span> We don&apos;t store your code — this diff is
          pulled fresh from your repository each time you open this page; only the review comments are saved.
        </span>
      </div>

      {loading && (
        <div className="flex items-center justify-center py-16">
          <Spinner size="md" />
          <span className="ml-3 text-sm text-slate-400">Loading comments…</span>
        </div>
      )}

      {!loading && error && (
        <div className="rounded-md border border-red-700/50 bg-red-950/30 px-4 py-3 text-sm text-red-200">{error}</div>
      )}

      {!loading && !error && data && data.status !== 'completed' && (
        <div className="rounded-md border border-slate-700 bg-slate-800/60 px-4 py-3 text-sm text-slate-300">
          {data.status === 'failed' ? data.message || 'This review failed.' : 'Review is still processing…'}
        </div>
      )}

      {!loading && !error && data?.status === 'completed' && (
        <>
          {data.summary && (
            <AISummaryCard
              summary={data.summary}
              fileCount={data.files?.length ?? 0}
              commentCount={allComments.length}
              severityCounts={severityCounts}
            />
          )}
          {(!data.files || data.files.length === 0) && (
            <div className="rounded-md border border-slate-700 bg-slate-800/60 px-4 py-6 text-center text-sm text-slate-400">
              No file changes to display.
            </div>
          )}
          {data.files?.map((file) => (
            <DiffFileCard key={file.file_path} file={file} />
          ))}
        </>
      )}
    </div>
  );
};

export default ScheduledReviewComments;
