// Ported from git-lrc:internal/staticserve/static/components/DiffTable.js (hunk
// header + line table structure) as of the git-lrc HEAD current when this port was
// written. git-lrc's server pre-splits hunk content into numbered lines; here that's
// done client-side by diffUtils.parseHunkLines since LiveReview only sends the raw
// unified-diff hunk body.
import React, { useState } from 'react';
import classNames from 'classnames';
import { DiffReviewComment, DiffReviewHunk } from '../../../types/reviews';
import { commentBelongsToLine, DiffLine, hunkDomId, parseHunkLines } from './diffUtils';
import { commentMatchesFilters, IssueFilters } from './issueFilters';
import CommentThread from './CommentThread';
import RiskBadge from './RiskBadge';
import BlastRadiusPanel from './BlastRadiusPanel';

interface HunkBlockProps {
  reviewId: number;
  filePath: string;
  // DOM-id identity for this file entry — see fileNavId's doc comment.
  // Distinct from filePath so synthetic per-hunk entries (flattenFilesByRisk,
  // the "Risk score: whole diff" sort mode) don't collide when several of
  // them share the same real filePath.
  navId: string;
  hunk: DiffReviewHunk;
  // Comments paired with their index in the file's original comments array
  // (not the per-line-filtered position) — CommentThread needs that original
  // index to build a DOM id that stays stable regardless of which line it's
  // grouped under, so CommentNav.tsx's flat list (built by iterating
  // file.comments directly) can find the exact same element.
  comments: { comment: DiffReviewComment; idx: number }[];
  hunkIndex: number;
  filters: IssueFilters;
}

const lineClass = (type: DiffLine['type']): string => {
  switch (type) {
    case 'add':
      return 'bg-emerald-950/40 text-emerald-200';
    case 'del':
      return 'bg-red-950/40 text-red-200';
    case 'meta':
      return 'text-slate-500 italic';
    default:
      return 'text-slate-300';
  }
};

const linePrefix = (type: DiffLine['type']): string => {
  if (type === 'add') return '+';
  if (type === 'del') return '-';
  if (type === 'meta') return '';
  return ' ';
};

const HunkBlock: React.FC<HunkBlockProps> = ({ reviewId, filePath, navId, hunk, comments, hunkIndex, filters }) => {
  const lines = parseHunkLines(hunk);
  const [panelOpen, setPanelOpen] = useState(false);
  const blastDetail = hunk.BlastDetail;

  return (
    <div id={hunkDomId(navId, hunkIndex)} className="scroll-mt-24" data-hunk-index={hunkIndex}>
      <div className="flex items-center gap-2 border-t border-slate-700 bg-slate-800/80 px-3 py-1.5 font-mono text-xs text-slate-400">
        {typeof hunk.BlastRadius === 'number' && (
          <RiskBadge score={hunk.BlastRadius} detail={blastDetail} size="large" onOpen={() => setPanelOpen((v) => !v)} />
        )}
        <span>{hunk.content ? `@@ -${hunk.old_start_line},${hunk.old_line_count} +${hunk.new_start_line},${hunk.new_line_count} @@` : 'No diff content available.'}</span>
      </div>
      {panelOpen && blastDetail && (
        <div className="border-t border-slate-700 p-3">
          <BlastRadiusPanel detail={blastDetail} />
        </div>
      )}
      {/* table-fixed forces columns to respect their declared widths instead of
          growing to fit the widest line — without it, whitespace-pre-wrap on the
          content column doesn't actually constrain anything, since auto table
          layout sizes columns from unwrapped content first. This is the actual
          fix for the page never needing to scroll horizontally. */}
      <table className="w-full table-fixed border-collapse font-mono text-xs">
        <tbody>
          {lines.map((line, idx) => {
            const lineComments = comments.filter(({ comment }) => commentBelongsToLine(comment, line) && commentMatchesFilters(comment, filters));
            return (
              <React.Fragment key={idx}>
                <tr className={classNames(lineClass(line.type))}>
                  <td className="w-10 select-none border-r border-slate-800 px-2 text-right text-slate-600">{line.oldNum ?? ''}</td>
                  <td className="w-10 select-none border-r border-slate-800 px-2 text-right text-slate-600">{line.newNum ?? ''}</td>
                  <td className="whitespace-pre-wrap break-all px-2">
                    <span className="select-none text-slate-500">{linePrefix(line.type)}</span>
                    {line.content}
                  </td>
                </tr>
                {lineComments.length > 0 && (
                  <tr>
                    <td colSpan={3} className="p-0">
                      <CommentThread reviewId={reviewId} filePath={filePath} comments={lineComments} hunkBlastDetail={blastDetail} />
                    </td>
                  </tr>
                )}
              </React.Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
};

export default HunkBlock;
