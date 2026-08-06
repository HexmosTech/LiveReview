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
}

function buildMetaItems(comment: DiffReviewComment): { label: string; value: string }[] {
  const items: { label: string; value: string }[] = [];
  if (comment.confidence) items.push({ label: 'CONFIDENCE', value: comment.confidence });
  if (comment.type) items.push({ label: 'TYPE', value: comment.type });
  if (comment.category || comment.subcategory) {
    items.push({
      label: 'CLASSIFICATION',
      value: `${comment.category || 'Uncategorized'}${comment.subcategory ? ` / ${comment.subcategory}` : ''}`,
    });
  }
  return items;
}

function buildCopyText(filePath: string, comment: DiffReviewComment): string {
  const parts = [`${filePath}:${comment.line}`];
  const severity = (comment.severity || 'info').toUpperCase();
  parts.push(`[${severity}]`);
  parts.push(comment.content);
  return parts.join(' ');
}

const CommentCard: React.FC<{
  id: string; reviewId: number; filePath: string; comment: DiffReviewComment;
  hunkBlastDetail?: BlastRadiusHunkReport;
}> = ({ id, reviewId, filePath, comment, hunkBlastDetail }) => {
  const [hidden, setHidden] = useState(false);
  const [copyLabel, setCopyLabel] = useState<string | null>(null);

  const metaItems = buildMetaItems(comment);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(buildCopyText(filePath, comment)).then(() => {
      setCopyLabel('Copied!');
      window.setTimeout(() => setCopyLabel(null), 2000);
    });
  }, [filePath, comment]);

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
          className="rounded border border-slate-600 px-2 py-0.5 text-[11px] text-slate-500 hover:text-slate-300"
        >
          Show
        </button>
      </div>
    );
  }

  return (
    <div id={id} className="scroll-mt-24 rounded-md border border-slate-700 bg-slate-900 p-3 target:border-blue-500">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          {hunkBlastDetail && typeof hunkBlastDetail.Combined === 'number' && (
            <RiskBadge score={hunkBlastDetail.Combined} detail={hunkBlastDetail} size="small" />
          )}
          <span
            className="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
            style={severityBadgeStyle(comment.severity)}
          >
            {(comment.severity || 'info').toUpperCase()}
          </span>
          <span className="font-mono text-xs text-slate-500">{filePath}:{comment.line}</span>
          {metaItems.map((item, i) => (
            <React.Fragment key={item.label}>
              {i > 0 && <span className="text-slate-600">•</span>}
              <span className="text-xs text-slate-500">
                <span className="text-slate-600">{item.label}</span> {item.value}
              </span>
            </React.Fragment>
          ))}
        </div>
        <div className="flex items-center gap-1">
          <VoteButtons
            reviewId={reviewId}
            sourceType="comment"
            commentContent={comment.content}
            filePath={filePath}
            severity={comment.severity}
          />
          <button
            type="button" onClick={() => setHidden(true)}
            title="Hide this issue"
            className="rounded border border-slate-700 px-1.5 py-0.5 text-[11px] text-slate-600 hover:text-slate-300"
          >
            —
          </button>
          <button
            type="button" onClick={handleCopy}
            title="Copy issue to clipboard"
            className="rounded border border-slate-700 px-1.5 py-0.5 text-[11px] text-slate-600 hover:text-slate-300"
          >
            {copyLabel || 'Copy'}
          </button>
        </div>
      </div>
      <p className="whitespace-pre-wrap text-sm text-slate-200">{comment.content}</p>
    </div>
  );
};

const CommentThread: React.FC<CommentThreadProps> = ({ reviewId, filePath, comments, hunkBlastDetail }) => {
  if (!comments.length) return null;
  return (
    <div className="space-y-2 border-l-2 border-slate-700 bg-slate-900/40 px-3 py-2">
      {comments.map(({ comment, idx }) => (
        <CommentCard
          key={idx}
          id={commentDomId(filePath, comment, idx)}
          reviewId={reviewId}
          filePath={filePath}
          comment={comment}
          hunkBlastDetail={hunkBlastDetail}
        />
      ))}
    </div>
  );
};

export default CommentThread;
