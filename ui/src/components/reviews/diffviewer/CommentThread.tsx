// Ported from git-lrc:internal/staticserve/static/components/Comment.js (header/
// meta-line/body structure + per-comment vote buttons + hide/show + copy) as of the
// git-lrc HEAD current when this port was written. Styles mirror git-lrc's actual
// .comment-container, .badge-info/.badge-warning/.badge-critical, .comment-tool-badge,
// .comment-meta-line, and .comment-body CSS rules (Comment.js/styles.css) 1:1.
import React, { useCallback, useEffect, useState } from 'react';
import { DiffReviewComment } from '../../../types/reviews';
import { BlastRadiusHunkReport } from '../../../types/reviews';
import { commentDomId, severityBadgeStyle } from './diffUtils';
import { getCommentRenderLabel } from './reviewPerformance';
import { renderInline } from '../../../lib/markdown';
import VoteButtons from './VoteButtons';
import RiskBadge from './RiskBadge';

interface CommentThreadProps {
  reviewId: number;
  filePath: string;
  comments: { comment: DiffReviewComment; idx: number }[];
  hunkBlastDetail?: BlastRadiusHunkReport;
  // The hunk header badge's score (structural blended with findings), as git-lrc's DiffTable.js passes.
  hunkRiskScore?: number;
  codeExcerpt?: string;
  // Opens the same BlastRadiusPanel the hunk header's own RiskBadge opens —
  // git-lrc repeats the hunk's risk pill on every comment's action row "so
  // score and comment are always assessed together" (RiskBadge.js), and both
  // copies drive the one shared panel, not independent ones.
  onOpenBreakdown?: () => void;
  // Time-to-first-comment tracking (ported from git-lrc - see reviewPerformance.ts /
  // DiffViewerPanel.tsx). baselineMs is the review's start time; onCommentRendered fires
  // once per comment id the first time it mounts, recording when it actually appeared.
  baselineMs?: number;
  commentRenderTimes?: Record<string, number>;
  onCommentRendered?: (commentKey: string) => void;
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
  hunkRiskScore?: number;
  codeExcerpt?: string;
  onOpenBreakdown?: () => void;
  renderLabel?: string;
  onRendered?: () => void;
}> = ({ id, reviewId, filePath, comment, hunkBlastDetail, hunkRiskScore, codeExcerpt, onOpenBreakdown, renderLabel, onRendered }) => {
  const [hidden, setHidden] = useState(false);
  const [copyLabel, setCopyLabel] = useState<string | null>(null);

  // Mount-only: records the first moment this specific comment actually
  // rendered, for the "Appeared in Xs" meta item below (ported from
  // git-lrc's handleCommentRendered - app.js).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { onRendered?.(); }, []);

  const metaItems = buildMetaItems(comment);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(buildCopyText(filePath, comment, codeExcerpt)).then(() => {
      setCopyLabel('Copied!');
      window.setTimeout(() => setCopyLabel(null), 2000);
    });
  }, [filePath, comment, codeExcerpt]);

  if (hidden) {
    return (
      <div className="relative my-2.5 rounded-[6px] border border-dashed border-[rgba(248,113,113,0.4)] bg-[rgba(82,27,27,0.35)] shadow-[inset_0_0_0_1px_rgba(248,113,113,0.08)] p-3.5 flex flex-col gap-1.5 font-sans">
        <div className="absolute right-3 top-3 flex items-center gap-2">
          <button
            type="button"
            onClick={() => setHidden(false)}
            title="Show this comment to the AI Agent"
            className="inline-flex items-center gap-1.5 rounded-[6px] border border-[rgba(110,118,129,0.3)] bg-[rgba(110,118,129,0.14)] px-2.5 py-1 text-[11px] font-semibold text-[#c9d1d9] hover:bg-[rgba(110,118,129,0.22)] hover:border-[rgba(110,118,129,0.44)] hover:text-white transition-colors"
          >
            <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
              <path d="M1 12s4-7 11-7 11 7 11 7-4 7-11 7S1 12 1 12z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
            Show
          </button>
        </div>
        <span className="font-semibold text-xs text-slate-200">Comment hidden</span>
        <span className="text-xs text-slate-400 font-mono">{filePath}{comment.line ? `:${comment.line}` : ''}</span>
        <span className="text-[11px] text-slate-400">Hidden comments are excluded from Copy Visible Issues and the Claude Agent.</span>
      </div>
    );
  }

  return (
    <div
      id={id}
      className="scroll-mt-24 relative my-2.5 rounded-[10px] border border-slate-700/80 bg-slate-900 p-4 md:px-[18px] md:pt-4 md:pb-[18px] shadow-lg shadow-black/40 font-sans target:border-blue-500 transition-colors"
    >
      {/* Header row — git-lrc .comment-header: pr-28 reserves space for action buttons */}
      <div className="flex flex-wrap items-center justify-between gap-3 mb-2 pr-28 text-slate-300">
        <div className="flex flex-wrap items-center gap-2.5">
          <span
            className="inline-flex items-center uppercase px-2.5 py-[3px] rounded-full text-[11px] font-extrabold tracking-[0.02em]"
            style={severityBadgeStyle(comment.severity)}
          >
            {(comment.severity || 'info').toUpperCase()}
          </span>
          {comment.tool_name && (
            <span className="comment-tool-badge inline-flex items-center rounded-full border border-[rgba(56,189,248,0.35)] bg-[rgba(56,189,248,0.15)] px-2 py-0.5 text-[11px] font-semibold text-[#38bdf8]">
              {comment.tool_name}
            </span>
          )}
          <span className="font-mono text-xs text-slate-400">
            {filePath}:{comment.line}
          </span>
          {renderLabel && (
            <span className="text-xs font-medium tracking-[0.01em] text-slate-400">
              {renderLabel}
            </span>
          )}
        </div>
        <div className="absolute right-3 top-3 flex items-center gap-1.5">
          {hunkBlastDetail && typeof hunkRiskScore === 'number' && (
            <RiskBadge score={hunkRiskScore} detail={hunkBlastDetail} size="large" onOpen={onOpenBreakdown} />
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
            type="button"
            onClick={() => setHidden(true)}
            title="Hide this comment"
            className="inline-flex h-[30px] w-[30px] items-center justify-center rounded-[6px] border border-[rgba(110,118,129,0.3)] bg-[rgba(110,118,129,0.14)] text-[#c9d1d9] hover:bg-[rgba(110,118,129,0.22)] hover:border-[rgba(110,118,129,0.44)] hover:text-white transition-colors"
          >
            <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
              <path d="M10.58 10.58a2 2 0 0 0 2.84 2.84" />
              <path d="M9.36 5.37A10.94 10.94 0 0 1 12 5c7 0 10 7 10 7a13.03 13.03 0 0 1-3.08 4.25" />
              <path d="M6.61 6.61C3.61 8.13 2 12 2 12s3 7 10 7a9.76 9.76 0 0 0 4.39-1.02" />
              <line x1="2" y1="2" x2="22" y2="22" />
            </svg>
          </button>
          <button
            type="button"
            onClick={handleCopy}
            title="Copy issue details"
            className={copyLabel === 'Copied!'
              ? 'inline-flex h-[30px] w-[30px] items-center justify-center rounded-[6px] border border-[rgba(34,197,94,0.3)] bg-[rgba(34,197,94,0.15)] text-[#bbf7d0]'
              : 'inline-flex h-[30px] w-[30px] items-center justify-center rounded-[6px] border border-[rgba(56,139,253,0.28)] bg-[rgba(56,139,253,0.12)] text-[#9ecbff] hover:bg-[rgba(56,139,253,0.2)] hover:border-[rgba(56,139,253,0.42)] transition-colors'}
          >
            {copyLabel === 'Copied!' ? (
              <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 13l4 4L19 7" />
              </svg>
            ) : (
              <svg width={14} height={14} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round">
                <path d="M8 16H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v2" />
                <path d="M10 10h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-8a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2z" />
              </svg>
            )}
          </button>
        </div>
      </div>

      {/* Meta row — git-lrc .comment-meta-line */}
      {metaItems.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 mb-3">
          {metaItems.map((item, i) => (
            <React.Fragment key={item.label}>
              {i > 0 && (
                <span className="select-none text-[11px] text-[rgba(148,163,184,0.55)]">
                  •
                </span>
              )}
              <span className="inline-flex items-center gap-1.5">
                <span className="text-[11px] font-bold uppercase tracking-[0.04em] text-[#8b949e]">
                  {item.label}
                </span>
                <span className={`text-xs font-medium break-words ${item.label === 'Classification' ? 'text-white' : 'text-[#d4d4d4]'}`}>
                  {item.value}
                </span>
              </span>
            </React.Fragment>
          ))}
        </div>
      )}

      {/* Body — git-lrc .comment-body with subtle 12% border */}
      <div className="whitespace-pre-wrap pt-3.5 px-0.5 text-sm font-medium leading-[1.72] tracking-[0.01em] text-[#cccccc] border-t border-[rgba(110,118,129,0.12)] font-sans">
        {renderInline(comment.content || '', `comment-text-${id}`)}
      </div>
    </div>
  );
};

const CommentThread: React.FC<CommentThreadProps> = ({ reviewId, filePath, comments, hunkBlastDetail, hunkRiskScore, codeExcerpt, onOpenBreakdown, baselineMs, commentRenderTimes, onCommentRendered }) => {
  if (!comments.length) return null;
  return (
    <div className="space-y-3 px-3 py-3">
      {comments.map(({ comment, idx }) => {
        const domId = commentDomId(filePath, comment, idx);
        return (
          <CommentCard
            key={idx}
            id={domId}
            reviewId={reviewId}
            filePath={filePath}
            comment={comment}
            hunkBlastDetail={hunkBlastDetail}
            hunkRiskScore={hunkRiskScore}
            codeExcerpt={codeExcerpt}
            onOpenBreakdown={onOpenBreakdown}
            renderLabel={getCommentRenderLabel(baselineMs, commentRenderTimes?.[domId])}
            onRendered={onCommentRendered ? () => onCommentRendered(domId) : undefined}
          />
        );
      })}
    </div>
  );
};

export default CommentThread;
