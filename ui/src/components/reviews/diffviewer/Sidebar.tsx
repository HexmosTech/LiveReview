// Ported from git-lrc:internal/staticserve/static/components/Sidebar.js (as of the
// git-lrc HEAD current when this port was written). Structure and interaction model
// match git-lrc exactly: the sidebar always lists the REAL, unflattened files —
// never the sort-mode-dependent flattened stream the main content area may be
// showing. Only in "Score: Whole" mode does a file's hunk submenu appear at all: a
// file expands into "Hunk n" entries (via the `hunkNav` lookup, built in
// DiffViewerPanel.tsx from the flattened stream) that jump to that hunk's position
// in the ranked view. In "Score: Per file"/"Diff order" a file's hunks are already
// directly visible under it in the main content, so there is nothing to expand
// (git-lrc's own comment: "hunkNav ... null/empty outside that view"). Colors use
// LiveReview's own Tailwind slate/blue palette (matching every other card on this
// page) rather than git-lrc's literal VS-Code hex values.
import React, { useState } from 'react';
import classNames from 'classnames';
import { Icons } from '../../UIPrimitives';
import { DiffReviewFile } from '../../../types/reviews';
import { fileNavId } from './diffUtils';
import { blastRadiusTier, BlastRadiusTier } from '../../../lib/blastRadius';
import { countFileVisibleComments, IssueFilters } from './issueFilters';

export interface HunkNavEntry {
  // DOM id of this hunk's rendered position in the (possibly flattened) main
  // content stream — git-lrc's entry.targetId / entry.ID.
  targetId: string;
  // 1-based position of this hunk within its own file (not its rank),
  // matching git-lrc's SourceHunkNumber — submenu entries are always listed
  // in original file order, never risk order.
  hunkNum: number;
  score: number | null;
  commentCount: number;
}

// Real file_path -> that file's hunks' ranked-view positions. Only populated
// in "Score: Whole" mode; empty/absent otherwise (see file header comment).
export type HunkNav = Record<string, HunkNavEntry[]>;

interface SidebarProps {
  files: DiffReviewFile[];
  hunkNav: HunkNav;
  activeFileId: string | null;
  filters: IssueFilters;
  // (navId, filePath) — a file click always force-expands its own file too
  // (git-lrc's handleFileClick unconditionally adds fileId to expandedFiles),
  // matching the equivalent (targetId, expandKey) shape of onHunkClick below.
  onFileClick: (navId: string, filePath: string) => void;
  // (targetId, expandKey) — matches git-lrc's handleHunkClick(targetId, expandKey).
  onHunkClick: (targetId: string, expandKey: string) => void;
}

const TIER_TEXT: Record<BlastRadiusTier, string> = {
  'blast-radius-high': 'text-red-400',
  'blast-radius-medium': 'text-amber-400',
  'blast-radius-low': 'text-sky-400',
  'blast-radius-none': 'text-slate-500',
};

const Sidebar: React.FC<SidebarProps> = ({ files, hunkNav, activeFileId, filters, onFileClick, onHunkClick }) => {
  const totalFiles = files.length;
  const totalComments = files.reduce((sum, f) => sum + countFileVisibleComments(f, filters), 0);
  // Expand state is keyed by the real file_path — matches git-lrc's
  // expandedFiles Set keyed by the real file ID (Sidebar.js:24-27).
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(() => new Set());

  return (
    // Sticky so the file list stays reachable while the (much longer) diff
    // content scrolls past it — git-lrc's sidebar is a fixed side panel for
    // the same reason. LiveReview's own <Navbar> is itself "sticky top-0
    // z-50" (65px tall) — sticking this to top-4 (16px) put it underneath
    // the navbar once scrolled, clipping its top edge. top-16 seats it
    // right below the navbar instead, matching the same fix already applied
    // to IssueFilterBar's sticky offset; max-height is adjusted to match so
    // it doesn't run off the bottom of the viewport.
    <div className="sticky top-16 flex max-h-[calc(100vh-5rem)] w-64 shrink-0 flex-col overflow-hidden rounded-lg border border-slate-700 bg-slate-800/60">
      <div className="border-b border-slate-700 px-3 py-2.5">
        <h2 className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Icons.Folder />
          Files
        </h2>
        <div className="mt-0.5 text-xs text-slate-600">
          {totalFiles} file{totalFiles !== 1 ? 's' : ''} &bull; {totalComments} comment{totalComments !== 1 ? 's' : ''}
        </div>
      </div>
      <div className="flex-1 overflow-y-auto py-1">
        {files.map((file) => {
          const navId = fileNavId(file);
          const isActive = activeFileId === navId;
          const hunkEntries = hunkNav[file.file_path] || [];
          const hasHunkSubmenu = hunkEntries.length > 0;
          const isExpanded = expandedFiles.has(file.file_path);
          const badgeCount = countFileVisibleComments(file, filters);

          return (
            <React.Fragment key={navId}>
              <button
                type="button"
                onClick={() => {
                  if (hasHunkSubmenu) {
                    setExpandedFiles((prev) => {
                      const next = new Set(prev);
                      if (next.has(file.file_path)) next.delete(file.file_path);
                      else next.add(file.file_path);
                      return next;
                    });
                  }
                  onFileClick(navId, file.file_path);
                }}
                className={classNames(
                  'flex w-full items-center gap-2 px-3 py-1.5 text-left text-[13px] transition-colors',
                  isActive ? 'bg-blue-500/10 text-white' : 'text-slate-400 hover:bg-slate-700/40'
                )}
              >
                {hasHunkSubmenu && (
                  <span className={classNames('w-3.5 shrink-0 text-center text-xs', isExpanded ? 'text-sky-400' : 'text-slate-500')}>
                    {isExpanded ? '▾' : '▸'}
                  </span>
                )}
                <span className="min-w-0 flex-1 truncate" title={file.file_path}>
                  {file.file_path}
                </span>
                {badgeCount > 0 && (
                  <span className="shrink-0 rounded-full bg-sky-600 px-1.5 py-0.5 text-[10px] font-medium text-white">
                    {badgeCount}
                  </span>
                )}
              </button>
              {hasHunkSubmenu && isExpanded && (
                <div className="pl-6">
                  {hunkEntries.map((entry) => {
                    const tier = typeof entry.score === 'number' ? blastRadiusTier(entry.score) : 'blast-radius-none';
                    return (
                      <button
                        type="button"
                        key={entry.targetId}
                        onClick={() => onHunkClick(entry.targetId, file.file_path)}
                        title={`Jump to hunk ${entry.hunkNum} of ${file.file_path} — risk ${typeof entry.score === 'number' ? Math.round(entry.score) : '–'}/100${entry.commentCount ? `, ${entry.commentCount} comment${entry.commentCount !== 1 ? 's' : ''}` : ''}`}
                        className="flex w-full items-center justify-between gap-2 border-l-2 border-transparent px-3 py-1 text-left text-xs text-slate-500 hover:border-sky-500 hover:bg-slate-700/40 hover:text-slate-200"
                      >
                        <span>Hunk {entry.hunkNum}</span>
                        <span className="flex items-center gap-1.5">
                          {entry.commentCount > 0 && (
                            <span className="rounded-full bg-slate-700 px-1 text-[10px] text-slate-300">{entry.commentCount}</span>
                          )}
                          {typeof entry.score === 'number' && (
                            <span className={classNames('rounded-full border px-1.5 font-mono text-[11px]', TIER_TEXT[tier], 'border-current/40')}>
                              {Math.round(entry.score)}
                            </span>
                          )}
                        </span>
                      </button>
                    );
                  })}
                </div>
              )}
            </React.Fragment>
          );
        })}
      </div>
    </div>
  );
};

export default Sidebar;
