// Ported from git-lrc:internal/staticserve/static/components/Comment.js (header/
// meta-line/body structure + per-comment vote buttons + hide/show + copy) as of the
// git-lrc HEAD current when this port was written. The severity chip is a custom
// inline-styled span rather than LiveReview's shared Badge primitive — Badge's
// info/warning/danger variants are light-mode Tailwind colors meant for a light
// surface elsewhere in the app, and read as washed-out pastel chips on this dark
// comment card; the styles below mirror git-lrc's actual .badge-info/.badge-warning/
// .badge-critical rgba values (Comment.js/styles.css), which were designed for a
// dark surface.
import React, { useCallback, useState } from 'react';
import { DiffReviewComment } from '../../../types/reviews';
import { BlastRadiusHunkReport } from '../../../types/reviews';
import { commentDomId, severityBadgeStyle } from './diffUtils';
import VoteButtons from './VoteButtons';
import RiskBadge from './RiskBadge';

interface CommentThreadProps {
  reviewId: number;
  filePath: string;
  comments: { comment: DiffReviewComment; idx: number }[];
  hunkBlastDetail?: BlastRadiusHunkReport;
  codeExcerpt?: string;
  // Opens the same BlastRadiusPanel the hunk header's own RiskBadge opens —
  // git-lrc repeats the hunk's risk pill on every comment's action row "so
  // score and comment are always assessed together" (RiskBadge.js), and both
  // copies drive the one shared panel, not independent ones.
  onOpenBreakdown?: () => void;
}

function buildMetaItems(comment: DiffReviewComment): { label: string; value: string }[] {
  const items: { label: string; value: string }[] = [];
  if (comment.confidence) items.push({ label: 'Confidence', value: comment.confidence });
  if (comment.type) items.push({ label: 'Type', value: comment.type });
  if (comment.category || comment.subcategory) {
    items.push({
      label: 'Classification',
      value: `${comment.category || 'Uncategorized'}${comment.subcategory ? ` / ${comment.subcategory}` : ''}`,
    });
  }
  return items;
}

function buildCopyText(filePath: string, comment: DiffReviewComment, codeExcerpt?: string): string {
  let copyText = '';
  if (filePath) {
    copyText += filePath;
    if (comment.line) {
      copyText += ':' + comment.line;
    }
    copyText += '\n\n';
  }
  if (codeExcerpt) {
    copyText += 'Code excerpt:\n' + codeExcerpt + '\n\n';
  }
  copyText += 'Issue:\n' + comment.content;
  return copyText;
}

const CommentCard: React.FC<{
  id: string; reviewId: number; filePath: string; comment: DiffReviewComment;
  hunkBlastDetail?: BlastRadiusHunkReport;
  codeExcerpt?: string;
  onOpenBreakdown?: () => void;
}> = ({ id, reviewId, filePath, comment, hunkBlastDetail, codeExcerpt, onOpenBreakdown }) => {
  const [hidden, setHidden] = useState(false);
  const [copyLabel, setCopyLabel] = useState<string | null>(null);

  const metaItems = buildMetaItems(comment);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(buildCopyText(filePath, comment, codeExcerpt)).then(() => {
      setCopyLabel('Copied!');
      window.setTimeout(() => setCopyLabel(null), 2000);
    });
  }, [filePath, comment, codeExcerpt]);

  if (hidden) {
    return (
      <div className="flex items-center gap-2 rounded-md border border-dashed border-slate-700 bg-slate-900/60 px-3 py-2">
        <span className="font-mono text-xs text-slate-600">{filePath}:{comment.line}</span>
        <span className="flex-1 text-xs text-slate-600">
          Issue hidden — {(comment.content || '').slice(0, 80)}{(comment.content || '').length > 80 ? '…' : ''}
        </span>
        <button
          type="button"
          onClick={() => setHidden(false)}
          className="inline-flex items-center gap-1.5 rounded border border-slate-600 px-2 py-0.5 text-[11px] text-slate-400 hover:text-slate-200"
        >
          <svg width={13} height={13} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
            <path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z" />
            <circle cx="12" cy="12" r="3" />
          </svg>
          Show
        </button>
      </div>
    );
  }

  return (
    <div id={id} className="scroll-mt-24 rounded-lg border border-slate-700/80 bg-slate-900 p-5 shadow-lg shadow-black/40 target:border-blue-500">
      {/* Header row — mb: 10px, gap: 12px matches git-lrc .comment-header */}
      <div className="mb-2.5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2.5">
          <span
            className="inline-flex items-center rounded-full px-2.5 py-[3px] text-[11px] font-extrabold uppercase"
            style={severityBadgeStyle(comment.severity)}
          >
            {(comment.severity || 'info').toUpperCase()}
          </span>
          <span className="font-mono text-xs text-slate-500">{filePath}:{comment.line}</span>
        </div>
        <div className="flex items-center gap-1.5">
          {hunkBlastDetail && typeof hunkBlastDetail.Combined === 'number' && (
            <RiskBadge score={hunkBlastDetail.Combined} detail={hunkBlastDetail} size="small" onOpen={onOpenBreakdown} />
          )}
          <VoteButtons
            reviewId={reviewId}
            sourceType="comment"
            commentContent={comment.content}
            codeExcerpt={codeExcerpt}
            filePath={filePath}
            severity={comment.severity}
          />
          <button
            type="button" onClick={() => setHidden(true)}
            title="Hide this issue"
            className="inline-flex h-7 w-7 items-center justify-center rounded border border-slate-700 text-slate-400 hover:border-slate-600 hover:bg-slate-800 hover:text-slate-200"
          >
            <svg width={13} height={13} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
              <path d="M10.58 10.58a2 2 0 0 0 2.84 2.84" />
              <path d="M9.36 5.37A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.03 13.03 0 0 1-3.08 4.25" />
              <path d="M6.61 6.61C3.61 8.13 2 12 2 12s3 7 10 7a9.76 9.76 0 0 0 4.39-1.02" />
              <line x1="2" y1="2" x2="22" y2="22" />
            </svg>
          </button>
          <button
            type="button" onClick={handleCopy}
            title="Copy issue to clipboard"
            className={copyLabel === 'Copied!' ? 'inline-flex h-7 w-7 items-center justify-center rounded border border-emerald-500/40 bg-emerald-500/10 text-emerald-300' : 'inline-flex h-7 w-7 items-center justify-center rounded border border-blue-500/30 bg-blue-500/10 text-sky-300 hover:border-blue-500/50 hover:bg-blue-500/20'}
          >
            {copyLabel === 'Copied!' ? (
              <svg width={13} height={13} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 13l4 4L19 7" />
              </svg>
            ) : (
              <svg width={13} height={13} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
                <path d="M8 16H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v2" />
                <path d="M10 10h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-8a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2z" />
              </svg>
            )}
          </button>
        </div>
      </div>
      {/* Meta row — gap: 8px, mb: 12px matches git-lrc .comment-meta-line */}
      {metaItems.length > 0 && (
        <div className="mb-3.5 flex flex-wrap items-center gap-2">
          {metaItems.map((item, i) => (
            <React.Fragment key={item.label}>
              {i > 0 && <span className="select-none text-[11px] text-slate-500/55">•</span>}
              <span className="inline-flex items-center gap-1.5">
                <span className="text-[11px] font-bold uppercase tracking-[0.04em] text-slate-500">{item.label}</span>
                <span className="text-[12px] font-medium text-slate-300">{item.value}</span>
              </span>
            </React.Fragment>
          ))}
        </div>
      )}
      {/* Body — subtle border-top (rgba 12%), font-size 14px, line-height 1.72 matches git-lrc .comment-body */}
      <p
        className="whitespace-pre-wrap pt-3.5 text-[14px] font-medium leading-[1.72] tracking-[0.01em] text-slate-200"
        style={{ borderTop: '1px solid rgba(110, 118, 129, 0.12)' }}
      >
        {comment.content}
      </p>
    </div>
  );
};

const CommentThread: React.FC<CommentThreadProps> = ({ reviewId, filePath, comments, hunkBlastDetail, codeExcerpt, onOpenBreakdown }) => {
  if (!comments.length) return null;
  return (
    <div className="space-y-3 px-3 py-3">
      {comments.map(({ comment, idx }) => (
        <CommentCard
          key={idx}
          id={commentDomId(filePath, comment, idx)}
          reviewId={reviewId}
          filePath={filePath}
          comment={comment}
          hunkBlastDetail={hunkBlastDetail}
          codeExcerpt={codeExcerpt}
          onOpenBreakdown={onOpenBreakdown}
        />
      ))}
    </div>
  );
};

export default CommentThread;
