// Ported from git-lrc:internal/staticserve/static/app.js and Toolbar.js.
// Layout: sidebar (left, 260px) | main-content (flex-1)
// Hash fragment navigation: clicking file/hunk sets #file-<id> / #hunk-<id>
import React, { useEffect, useMemo, useState } from 'react';
import classNames from 'classnames';
import { Spinner } from '../../UIPrimitives';
import { getBlastRadiusReport, getDiffReview } from '../../../api/reviews';
import { BlastRadiusHunkReport, DiffReviewFile, DiffReviewStatusResponse } from '../../../types/reviews';
import { attachBlastData, buildBlastLookup, flattenFilesByRisk, hasBlastRadiusData, sortFilesByBlastRadius } from '../../../lib/blastRadius';
import { commentDomId, filePathToId, hunkDomId } from './diffUtils';
import {
  buildFilterFacets, commentMatchesFilters, createDefaultIssueFilters, IssueFilters,
  toggleCategoryFilter, toggleConfidenceFilter, toggleSeverityFilter, toggleSubcategoryFilter, toggleTypeFilter,
} from './issueFilters';
import FileBlock from './FileBlock';
import Sidebar from './Sidebar';
import IssueFilterBar from './IssueFilterBar';
import CommentNav, { NavComment } from './CommentNav';
import SummaryPanel from './SummaryPanel';

const SORT_OPTS = [
  { mode: 'risk-flat' as const, label: 'Score: Whole', title: 'All hunks globally ranked by risk score' },
  { mode: 'risk-file' as const, label: 'Score: Per file', title: 'Hunks ranked within each file by risk score' },
  { mode: 'diff' as const, label: 'Diff order', title: 'Original diff order' },
];
type SortMode = typeof SORT_OPTS[number]['mode'];

function navComments(files: DiffReviewFile[], f: IssueFilters): NavComment[] {
  const n: NavComment[] = [];
  files.forEach(file => (file.comments || []).forEach((c, i) => {
    if (commentMatchesFilters(c, f)) n.push({ id: commentDomId(file.file_path, c, i) });
  }));
  return n;
}

const DiffViewerPanel: React.FC<{ reviewId: number }> = ({ reviewId }) => {
  const [data, setData] = useState<DiffReviewStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [blastLookup, setBlastLookup] = useState<Map<string, BlastRadiusHunkReport> | undefined>();
  const [sortMode, setSortMode] = useState<SortMode>('risk-flat');
  const [expandedFiles, setExpandedFiles] = useState<Record<string, boolean>>({});
  const [activeFileId, setActiveFileId] = useState<string | null>(null);
  const [filters, setFilters] = useState<IssueFilters>(createDefaultIssueFilters());

  useEffect(() => {
    let c = false; setLoading(true); setError(null);
    getDiffReview(reviewId).then(r => {
      if (c) return; setData(r);
      const e: Record<string, boolean> = {};
      (r.files || []).forEach(f => { e[f.file_path] = true; });
      setExpandedFiles(e);
    }).catch(err => {
      if (c) return;
      if ((err as any)?.status === 404) setData(null);
      else setError(err instanceof Error ? err.message : 'Failed');
    }).finally(() => { if (!c) setLoading(false); });
    return () => { c = true; };
  }, [reviewId]);

  useEffect(() => {
    let c = false;
    getBlastRadiusReport(reviewId).then(r => { if (!c) setBlastLookup(buildBlastLookup(r)); }).catch(() => { if (!c) setBlastLookup(undefined); });
    return () => { c = true; };
  }, [reviewId]);

  // Hash fragment navigation
  useEffect(() => {
    const onHash = () => {
      const h = window.location.hash;
      const parts = h.split('#');
      const frag = parts.length > 2 ? parts[parts.length - 1] : '';
      if (!frag) return;
      requestAnimationFrame(() => {
        document.getElementById(frag)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      });
    };
    window.addEventListener('hashchange', onHash);
    onHash();
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const rawFiles = data?.files || [];
  const enrichedFiles = useMemo(() => attachBlastData(rawFiles, blastLookup || new Map()), [rawFiles, blastLookup]);
  const canSortByRisk = useMemo(() => hasBlastRadiusData(enrichedFiles), [enrichedFiles]);
  const files = useMemo(() => {
    if (!canSortByRisk) return enrichedFiles;
    if (sortMode === 'risk-flat') return flattenFilesByRisk(enrichedFiles);
    if (sortMode === 'risk-file') return sortFilesByBlastRadius(enrichedFiles);
    return enrichedFiles;
  }, [enrichedFiles, sortMode, canSortByRisk]);

  const quiz = data?.quiz || [];
  const facets = useMemo(() => buildFilterFacets(files, filters), [files, filters]);
  const commentNav = useMemo(() => navComments(files, filters), [files, filters]);

  const pushHash = (h: string) => { window.location.hash = h; };

  const jumpToFile = (navId: string) => {
    pushHash(`/reviews/${reviewId}#${navId}`);
    setActiveFileId(navId);
  };

  const jumpToHunk = (filePath: string, hunkId: string) => {
    pushHash(`/reviews/${reviewId}#${hunkId}`);
    setExpandedFiles(prev => ({ ...prev, [filePath]: true }));
  };

  if (loading) return <div className="flex items-center justify-center py-10"><Spinner /></div>;
  if (error) return <div className="rounded border border-red-700 bg-red-900/30 p-4 text-sm text-red-300">{error}</div>;
  if (!data || data.status !== 'completed' || !data.files) return null;
  if (files.length === 0) return null;

  return (
    <div className="flex items-start gap-0">
      <Sidebar files={files} activeFileId={activeFileId} filters={filters} onFileClick={jumpToFile} onHunkClick={jumpToHunk} />
      <div className="main-content min-w-0 flex-1 px-4">
        <SummaryPanel summary={data.summary} files={files} quiz={quiz} />

        {/* Toolbar: view tabs + sort + expand/collapse */}
        <div className="toolbar-row flex flex-wrap items-center justify-between gap-3 py-2">
          <div className="view-tabs flex items-center gap-0.5 rounded-full border border-slate-700 bg-slate-900/80 p-0.5 text-xs">
            <span className="rounded-full px-3 py-1 bg-slate-700 text-white flex items-center gap-1.5">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>
              Files &amp; Comments
            </span>
          </div>
          {canSortByRisk && (
            <div className="flex items-center gap-0.5 rounded-full border border-slate-700 bg-slate-900/80 p-0.5 text-xs">
              <span className="px-2 text-slate-500">Order By</span>
              {SORT_OPTS.map(o => (
                <button key={o.mode} onClick={() => setSortMode(o.mode)} title={o.title}
                  className={classNames('rounded-full px-3 py-1', sortMode === o.mode ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200')}>{o.label}</button>
              ))}
            </div>
          )}
          <button onClick={() => {
            const all = files.every(f => expandedFiles[f.file_path]);
            const n: Record<string, boolean> = {};
            files.forEach(f => { n[f.file_path] = !all; });
            setExpandedFiles(n);
          }} className="rounded border border-slate-700 px-2.5 py-1 text-xs text-slate-400 hover:text-slate-200">
            {files.every(f => expandedFiles[f.file_path]) ? 'Collapse All' : 'Expand All'}
          </button>
        </div>

        {/* Filter bar */}
        <IssueFilterBar
          filters={filters} facets={facets}
          onToggleSeverity={v => setFilters(f => toggleSeverityFilter(f, v))}
          onToggleConfidence={v => setFilters(f => toggleConfidenceFilter(f, v))}
          onToggleType={v => setFilters(f => toggleTypeFilter(f, v))}
          onToggleCategory={(v, c) => setFilters(f => toggleCategoryFilter(f, v, c))}
          onToggleSubcategory={v => setFilters(f => toggleSubcategoryFilter(f, v))}
          onReset={() => setFilters(createDefaultIssueFilters())}
        />

        {!!data.excluded_files?.length && <p className="text-xs text-slate-500 mt-2">{data.excluded_files.length} file(s) excluded.</p>}

        {/* Files */}
        <div className="space-y-3 mt-3">
          {files.map(file => (
            <FileBlock key={file.syntheticId || filePathToId(file.file_path)} reviewId={reviewId} file={file}
              expanded={!!expandedFiles[file.file_path]} onToggle={() => setExpandedFiles(p => ({ ...p, [file.file_path]: !p[file.file_path] }))} filters={filters} />
          ))}
        </div>
      </div>
      <CommentNav comments={commentNav} active />
    </div>
  );
};

export default DiffViewerPanel;
