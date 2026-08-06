// Ported from git-lrc:internal/staticserve/static/components/Sidebar.js (as of the
// git-lrc HEAD current when this port was written). Structure, class names, and
// interaction model match git-lrc exactly:
//   .sidebar (width: 260px) > .sidebar-header > .sidebar-content
//   .sidebar-file (with .sidebar-file-collapsible, .sidebar-file-caret, .sidebar-file-name, .sidebar-file-badge)
//   .sidebar-hunk-list > .sidebar-hunk (with .sidebar-hunk-name, .sidebar-hunk-meta, .sidebar-hunk-score)
import React, { useState } from 'react';
import classNames from 'classnames';
import { DiffReviewFile } from '../../../types/reviews';
import { fileNavId, hunkDomId } from './diffUtils';
import { blastRadiusTier } from '../../../lib/blastRadius';
import { countFileVisibleComments, countHunkVisibleComments, IssueFilters } from './issueFilters';

interface SidebarProps {
  files: DiffReviewFile[];
  activeFileId: string | null;
  filters: IssueFilters;
  onFileClick: (fileId: string) => void;
  onHunkClick: (filePath: string, hunkId: string) => void;
}

const Sidebar: React.FC<SidebarProps> = ({ files, activeFileId, filters, onFileClick, onHunkClick }) => {
  const totalFiles = files.length;
  const totalComments = files.reduce((sum, f) => sum + countFileVisibleComments(f, filters), 0);
  const [expandedFiles, setExpandedFiles] = useState<Set<string>>(() => new Set());

  return (
    <div className="sidebar flex-shrink-0" style={{ width: 260, background: 'var(--bg-secondary, #1c1f26)', borderRight: '1px solid var(--border-subtle, #30363d)' }}>
      <div className="sidebar-header" style={{ padding: '10px 12px', borderBottom: '1px solid var(--border-subtle, #30363d)' }}>
        <h2 style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 11, fontWeight: 600, color: 'var(--text-muted, #8b949e)', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: 2 }}>
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"></path><polyline points="13 2 13 9 20 9"></polyline></svg>
          Files
        </h2>
        <div className="sidebar-stats" style={{ fontSize: 11, color: 'var(--text-dim, #6e7681)' }}>
          {totalFiles} file{totalFiles !== 1 ? 's' : ''} &middot; {totalComments} comment{totalComments !== 1 ? 's' : ''}
        </div>
      </div>
      <div className="sidebar-content" style={{ flex: 1, overflowY: 'auto', padding: '4px 0', maxHeight: 'calc(100vh - 200px)' }}>
        {files.map((file) => {
          const navId = fileNavId(file);
          const isActive = activeFileId === navId;
          const hunks = file.hunks || [];
          const hasExpandableHunks = hunks.length > 1;
          const isExpanded = expandedFiles.has(navId);
          const badgeCount = countFileVisibleComments(file, filters);

          return (
            <React.Fragment key={navId}>
              <div
                className={classNames(
                  'sidebar-file',
                  isActive && 'active',
                  hasExpandableHunks && 'sidebar-file-collapsible',
                  isExpanded && 'expanded'
                )}
                style={{
                  padding: '6px 12px', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 8,
                  fontSize: 13, color: isActive ? 'var(--text-primary, #e6edf3)' : 'var(--text-secondary, #8b949e)',
                  background: isActive ? 'var(--bg-active, rgba(56,139,253,0.1))' : undefined,
                  transition: 'background 0.1s ease',
                }}
                onClick={() => {
                  if (hasExpandableHunks) {
                    setExpandedFiles(prev => {
                      const next = new Set(prev);
                      next.has(navId) ? next.delete(navId) : next.add(navId);
                      return next;
                    });
                  }
                  onFileClick(navId);
                }}
                onMouseEnter={e => { if (!isActive) e.currentTarget.style.background = 'var(--bg-hover, rgba(177,186,196,0.08))'; }}
                onMouseLeave={e => { if (!isActive) e.currentTarget.style.background = ''; }}
              >
                {hasExpandableHunks && (
                  <span className="sidebar-file-caret" style={{ flexShrink: 0, width: 14, textAlign: 'center', fontSize: 13, color: isExpanded ? 'var(--accent-blue, #3794ff)' : 'var(--text-secondary, #8b949e)', opacity: 0.85 }}>
                    {isExpanded ? '▾' : '▸'}
                  </span>
                )}
                <span className="sidebar-file-name" title={file.file_path} style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontFamily: 'ui-monospace, monospace', fontSize: 12 }}>
                  {file.file_path}
                  {typeof file.sourceHunkNumber === 'number' && (
                    <span style={{ color: 'var(--text-dim, #6e7681)', marginLeft: 6 }}>Hunk #{file.sourceHunkNumber}</span>
                  )}
                </span>
                {badgeCount > 0 && (
                  <span className="sidebar-file-badge" style={{ flexShrink: 0, display: 'inline-flex', alignItems: 'center', justifyContent: 'center', minWidth: 20, height: 18, borderRadius: 999, background: 'rgba(56,139,253,0.15)', color: '#58a6ff', fontSize: 11, fontWeight: 500, padding: '0 6px' }}>
                    {badgeCount}
                  </span>
                )}
              </div>
              {hasExpandableHunks && isExpanded && (
                <div className="sidebar-hunk-list" style={{ paddingLeft: 24 }}>
                  {hunks.map((hunk, i) => {
                    const score = hunk.BlastRadius;
                    const tier = typeof score === 'number' ? blastRadiusTier(score) : 'blast-radius-none';
                    const hunkCommentCount = countHunkVisibleComments(hunk, file.comments || [], filters);
                    const tierColors: Record<string, string> = { 'blast-radius-high': '#f87171', 'blast-radius-medium': '#fbbf24', 'blast-radius-low': '#38bdf8', 'blast-radius-none': '#6b7280' };
                    return (
                      <div
                        key={i}
                        className="sidebar-hunk"
                        style={{ padding: '4px 12px 4px 16px', cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 12, color: 'var(--text-secondary, #8b949e)' }}
                        onClick={() => onHunkClick(file.file_path, hunkDomId(navId, i))}
                        onMouseEnter={e => { e.currentTarget.style.background = 'var(--bg-hover, rgba(177,186,196,0.08))'; }}
                        onMouseLeave={e => { e.currentTarget.style.background = ''; }}
                        title={`Hunk ${i + 1} of ${file.file_path} — risk ${typeof score === 'number' ? Math.round(score) : '-'}/100${hunkCommentCount ? `, ${hunkCommentCount} comments` : ''}`}
                      >
                        <span className="sidebar-hunk-name">Hunk {i + 1}</span>
                        <span className="sidebar-hunk-meta" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                          {hunkCommentCount > 0 && (
                            <span className="sidebar-hunk-comments" style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', minWidth: 16, height: 16, borderRadius: 999, background: 'rgba(177,186,196,0.12)', color: 'var(--text-dim, #6e7681)', fontSize: 10, padding: '0 4px' }}>
                              {hunkCommentCount}
                            </span>
                          )}
                          {typeof score === 'number' && (
                            <span className={`sidebar-hunk-score ${tier}`} style={{ fontSize: 11, fontFamily: 'ui-monospace, monospace', color: tierColors[tier] || '#6b7280', fontWeight: 'bold' }}>
                              {Math.round(score)}
                            </span>
                          )}
                        </span>
                      </div>
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
