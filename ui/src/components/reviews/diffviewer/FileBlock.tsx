// Ported from git-lrc:internal/staticserve/static/components/FileBlock.js
// (collapsible file wrapper) as of the git-lrc HEAD current when this port was
// written. Expand/collapse state is lifted to DiffViewerPanel (controlled), unlike
// git-lrc's per-component state, so a top-level "Expand All"/"Collapse All" toggle
// can drive every file block at once.
import React from 'react';
import { Icons } from '../../UIPrimitives';
import { DiffReviewFile } from '../../../types/reviews';
import { fileNavId } from './diffUtils';
import { countFileVisibleComments, IssueFilters } from './issueFilters';
import HunkBlock from './HunkBlock';

interface FileBlockProps {
  reviewId: number;
  file: DiffReviewFile;
  expanded: boolean;
  onToggle: () => void;
  filters: IssueFilters;
}

const FileBlock: React.FC<FileBlockProps> = ({ reviewId, file, expanded, onToggle, filters }) => {
  const visibleCount = countFileVisibleComments(file, filters);
  const indexedComments = (file.comments || []).map((comment, idx) => ({ comment, idx }));
  const navId = fileNavId(file);

  return (
    <div id={navId} className="scroll-mt-24 overflow-hidden rounded-lg border border-slate-700 bg-slate-800">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center justify-between gap-2 px-4 py-2.5 text-left hover:bg-slate-750"
      >
        <div className="flex min-w-0 items-center gap-2">
          <span className="text-slate-400">{expanded ? <Icons.FolderOpen /> : <Icons.Folder />}</span>
          <span className="truncate font-mono text-sm text-slate-200">
            {file.file_path}
            {typeof file.sourceHunkNumber === 'number' && (
              <span className="ml-2 text-xs font-normal text-slate-500">— hunk {file.sourceHunkNumber}</span>
            )}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {visibleCount > 0 && (
            <span className="rounded-full bg-slate-700 px-2 py-0.5 text-xs text-slate-300">{visibleCount}</span>
          )}
          <span className="text-slate-500">{expanded ? <Icons.ChevronDown /> : <Icons.ChevronRight />}</span>
        </div>
      </button>
      {expanded && (
        <div className="border-t border-slate-700">
          {(file.hunks || []).length === 0 ? (
            <p className="px-4 py-3 text-sm text-slate-500">No diff content available.</p>
          ) : (
            file.hunks.map((hunk, idx) => (
              <HunkBlock
                key={idx}
                reviewId={reviewId}
                filePath={file.file_path}
                navId={navId}
                hunk={hunk}
                comments={indexedComments}
                hunkIndex={idx}
                filters={filters}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
};

export default FileBlock;
