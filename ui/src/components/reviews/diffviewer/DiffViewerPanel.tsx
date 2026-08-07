// Ported from git-lrc:internal/staticserve/static/app.js and Toolbar.js.
// Layout: sidebar (left, sticky) | main-content (flex-1) | comment nav (floating).
//
// Navigation (file/hunk/comment jumps) uses react-router's own useSearchParams
// rather than writing to location.hash directly — LiveReview's HashRouter
// already owns location.hash for page routing (#/reviews/82), so a second,
// competing hash write there would corrupt route navigation. Search params
// live inside the router's own owned URL space (#/reviews/82?to=...), so this
// is entirely router-native: it shows up in the URL bar, survives reload
// (real deep-linking — a step beyond git-lrc's own behavior, which doesn't
// read location.hash on initial mount either), and works with the browser
// Back/Forward buttons via the router's normal history subscription.
import React, { useEffect, useMemo, useState } from 'react';
import classNames from 'classnames';
import { useSearchParams } from 'react-router-dom';
import { Button, EmptyState, Icons, Spinner } from '../../UIPrimitives';
import { getBlastRadiusReport, getDiffReview } from '../../../api/reviews';
import { BlastRadiusHunkReport, DiffReviewFile, DiffReviewStatusResponse } from '../../../types/reviews';
import { attachBlastData, buildBlastLookup, flattenFilesByRisk, hasBlastRadiusData, sortFilesByBlastRadius } from '../../../lib/blastRadius';
import { commentDomId, fileNavId, scrollElementIntoViewBelowStickyBars } from './diffUtils';
import {
  buildFilterFacets,
  commentMatchesFilters,
  countFileVisibleComments,
  createDefaultIssueFilters,
  IssueFilters,
  toggleCategoryFilter,
  toggleConfidenceFilter,
  toggleSeverityFilter,
  toggleSubcategoryFilter,
  toggleTypeFilter,
} from './issueFilters';
import FileBlock from './FileBlock';
import Sidebar, { HunkNav } from './Sidebar';
import IssueFilterBar from './IssueFilterBar';
import CommentNav, { NavComment } from './CommentNav';
import SummaryPanel from './SummaryPanel';

interface DiffViewerPanelProps {
  reviewId: number;
}

// Mirrors git-lrc's SORT_MODE_RISK_FLAT / SORT_MODE_RISK_FILE / SORT_MODE_DIFF
// exactly (Toolbar.js's SORT_MODE_OPTIONS), including labels/titles.
type SortMode = 'risk-flat' | 'risk-file' | 'diff';
const SORT_MODE_OPTIONS: { mode: SortMode; label: string; title: string }[] = [
  { mode: 'risk-flat', label: 'Score: Whole', title: 'One ranked stream: every hunk across the whole diff ordered by risk score, highest first' },
  { mode: 'risk-file', label: 'Score: Per file', title: 'Keep files together; order hunks inside each file by risk score' },
  { mode: 'diff', label: 'Diff order', title: 'Original diff order: files and hunks as they appear in the diff' },
];

function buildVisibleCommentNav(files: DiffReviewFile[], filters: IssueFilters): NavComment[] {
  const nav: NavComment[] = [];
  files.forEach((file) => {
    (file.comments || []).forEach((comment, idx) => {
      if (commentMatchesFilters(comment, filters)) {
        nav.push({ id: commentDomId(file.file_path, comment, idx), filePath: file.file_path });
      }
    });
  });
  return nav;
}

// Ported from git-lrc:app.js's hunkNav construction. In the whole-diff risk
// view a file's hunks are scattered through the ranked stream, so the
// sidebar (which always shows the real, unflattened file list — see
// Sidebar.js's own comment) needs a lookup from real file_path to each of
// that file's hunks' position in the flattened stream, to render a "Hunk n"
// submenu. Outside that view this stays empty — hunks are already visible
// directly under their file, no submenu needed (Sidebar.js:11-20).
function buildHunkNav(flatFiles: DiffReviewFile[], filters: IssueFilters): HunkNav {
  const nav: HunkNav = {};
  flatFiles.forEach((entry) => {
    if (typeof entry.sourceHunkNumber !== 'number') return;
    const list = nav[entry.file_path] || (nav[entry.file_path] = []);
    list.push({
      targetId: fileNavId(entry),
      hunkNum: entry.sourceHunkNumber,
      score: typeof entry.hunks[0]?.BlastRadius === 'number' ? entry.hunks[0].BlastRadius : null,
      commentCount: countFileVisibleComments(entry, filters),
    });
  });
  Object.values(nav).forEach((list) => list.sort((a, b) => a.hunkNum - b.hunkNum));
  return nav;
}

function buildCopyText(files: DiffReviewFile[], filters: IssueFilters): string {
  const lines: string[] = [];
  files.forEach((file) => {
    (file.comments || []).forEach((comment) => {
      if (!commentMatchesFilters(comment, filters)) return;
      lines.push(`${file.file_path}:${comment.line} [${(comment.severity || 'info').toUpperCase()}] ${comment.content}`);
    });
  });
  return lines.join('\n\n');
}

// The DOM id to scroll to, plus (when jumping into a hunk/comment) the
// file_path that must be expanded first for that element to exist at all.
interface DiffNavTarget {
  to: string;
  file?: string;
}

const DiffViewerPanel: React.FC<DiffViewerPanelProps> = ({ reviewId }) => {
  const [data, setData] = useState<DiffReviewStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [blastLookup, setBlastLookup] = useState<Map<string, BlastRadiusHunkReport> | undefined>(undefined);
  const [sortMode, setSortMode] = useState<SortMode>('risk-flat');
  const [expandedFiles, setExpandedFiles] = useState<Record<string, boolean>>({});
  const [activeFileId, setActiveFileId] = useState<string | null>(null);
  const [filters, setFilters] = useState<IssueFilters>(createDefaultIssueFilters());
  const [copyStatus, setCopyStatus] = useState<'idle' | 'copied'>('idle');
  const [searchParams, setSearchParams] = useSearchParams();

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    getDiffReview(reviewId)
      .then((res) => {
        if (cancelled) return;
        setData(res);
        const expanded: Record<string, boolean> = {};
        (res.files || []).forEach((f) => { expanded[f.file_path] = true; });
        setExpandedFiles(expanded);
      })
      .catch((err) => {
        if (cancelled) return;
        // A review triggered outside the CLI/diff-review flow (e.g. a plain
        // git-host webhook) never had structured diff data to begin with —
        // treat 404 as "nothing to show", not an error banner.
        if ((err as any)?.status === 404) {
          setData(null);
        } else {
          setError(err instanceof Error ? err.message : 'Failed to load diff');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [reviewId]);

  useEffect(() => {
    // Blast radius is opportunistic — only reviews run via `git lrc review`
    // ever have one. A 404 here is the common case, not an error; leave
    // blastLookup undefined so hunks simply render without a RiskBadge.
    let cancelled = false;
    getBlastRadiusReport(reviewId)
      .then((report) => {
        if (!cancelled) setBlastLookup(buildBlastLookup(report));
      })
      .catch(() => {
        if (!cancelled) setBlastLookup(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, [reviewId]);

  const rawFiles = data?.files || [];
  // Attached once here (not per-render inside FileBlock/HunkBlock) so
  // sortFilesByBlastRadius/flattenFilesByRisk/hasBlastRadiusData all see
  // hunk.BlastRadius already in place — see attachBlastData's doc comment.
  const enrichedFiles = useMemo(() => attachBlastData(rawFiles, blastLookup || new Map()), [rawFiles, blastLookup]);
  const canSortByRisk = useMemo(() => hasBlastRadiusData(enrichedFiles), [enrichedFiles]);
  const files = useMemo(() => {
    if (!canSortByRisk) return enrichedFiles;
    if (sortMode === 'risk-flat') return flattenFilesByRisk(enrichedFiles);
    if (sortMode === 'risk-file') return sortFilesByBlastRadius(enrichedFiles);
    return enrichedFiles;
  }, [enrichedFiles, sortMode, canSortByRisk]);

  const facets = useMemo(() => buildFilterFacets(files, filters), [files, filters]);
  const navComments = useMemo(() => buildVisibleCommentNav(files, filters), [files, filters]);
  const allExpanded = files.length > 0 && files.every((f) => expandedFiles[f.file_path]);
  // Sidebar always shows the real, unflattened file list (git-lrc's
  // Sidebar.js receives `filesInDiffOrder`, never the sort-mode-dependent
  // `files`) — only the main content area's ordering changes with sortMode.
  const hunkNav = useMemo(
    () => (sortMode === 'risk-flat' ? buildHunkNav(files, filters) : {}),
    [files, sortMode, filters]
  );

  const toggleFile = (filePath: string) => {
    setExpandedFiles((prev) => ({ ...prev, [filePath]: !prev[filePath] }));
  };

  const toggleAll = () => {
    const next: Record<string, boolean> = {};
    files.forEach((f) => { next[f.file_path] = !allExpanded; });
    setExpandedFiles(next);
  };

  const copyVisibleIssues = () => {
    navigator.clipboard.writeText(buildCopyText(files, filters)).then(() => {
      setCopyStatus('copied');
      window.setTimeout(() => setCopyStatus('idle'), 2000);
    });
  };

  const navigateTo = (target: DiffNavTarget) => {
    const next = new URLSearchParams();
    next.set('to', target.to);
    if (target.file) next.set('file', target.file);
    setSearchParams(next);
  };

  // Single effect drives every scroll/expand from the URL's own state — it
  // fires identically whether `to`/`file` changed because the user clicked a
  // sidebar row (navigateTo -> setSearchParams push), because they hit
  // Back/Forward (the router's own popstate handling updates searchParams),
  // or because the page loaded with a link that already had them (real
  // deep-linking).
  useEffect(() => {
    const to = searchParams.get('to');
    if (!to) return;
    const file = searchParams.get('file');
    setActiveFileId(to);
    if (file) {
      setExpandedFiles((prev) => (prev[file] ? prev : { ...prev, [file]: true }));
    }
    requestAnimationFrame(() => {
      // In "Score: Whole" mode a real file's plain navId never appears in
      // the DOM — the main content renders each of its hunks as its own
      // separately-ided block (`${navId}--hunk-N-M`). Falls back to the
      // first (highest-ranked) one, matching git-lrc's handleFileClick
      // fallback exactly (app.js:632-634).
      const el = document.getElementById(to) || document.querySelector(`[id^="${to}--hunk-"]`);
      scrollElementIntoViewBelowStickyBars(el as HTMLElement | null);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams]);

  // (navId, filePath) — always force-expands the target file too, matching
  // git-lrc's handleFileClick (app.js:614-626), which unconditionally adds
  // fileId to expandedFiles regardless of whether it came from a plain file
  // row or (via handleHunkClick) a hunk submenu entry.
  const jumpToFile = (navId: string, filePath: string) => navigateTo({ to: navId, file: filePath });
  // Matches git-lrc's handleHunkClick(targetId, expandKey) param order —
  // expandKey is the real file's file_path (expand/collapse state is keyed
  // by the real file, never by a synthetic per-hunk entry).
  const jumpToHunk = (targetId: string, expandKey: string) => navigateTo({ to: targetId, file: expandKey });
  const jumpToComment = (comment: NavComment) => navigateTo({ to: comment.id, file: comment.filePath });

  // git-lrc's enhanceTextWithFileChips resolves a `path:line` token from
  // summary markdown to a diff-viewer jump (Summary.js's
  // onOpenFileFromSlide) — this is the file-level equivalent (line-precise
  // scrolling would need a line->hunk lookup this port doesn't have yet).
  const jumpToFileByPath = (filePath: string) => {
    const file = files.find((f) => f.file_path === filePath);
    if (file) jumpToFile(fileNavId(file), file.file_path);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-10">
        <Spinner />
      </div>
    );
  }

  if (error) {
    return (
      <EmptyState
        icon={<Icons.Error />}
        title="Couldn't load the diff"
        description={error}
      />
    );
  }

  if (!data || data.status !== 'completed' || !data.files) {
    return (
      <EmptyState
        icon={<Icons.Reviews />}
        title={data && data.status !== 'completed' ? 'Diff not ready yet' : 'No diff data for this review'}
        description={
          data && data.status !== 'completed'
            ? 'This review is still in progress. Findings will appear here once it completes.'
            : 'This review has no structured diff/findings data — most likely it wasn’t triggered via the diff-review flow.'
        }
      />
    );
  }

  const quiz = data.quiz || [];

  if (files.length === 0) {
    return <EmptyState icon={<Icons.Reviews />} title="No files changed" />;
  }

  return (
    <div className="flex items-start gap-4">
      <Sidebar
        files={enrichedFiles}
        hunkNav={hunkNav}
        activeFileId={activeFileId}
        filters={filters}
        onFileClick={jumpToFile}
        onHunkClick={jumpToHunk}
      />
      <div className="min-w-0 flex-1 space-y-4">
        <SummaryPanel reviewId={reviewId} summary={data.summary} files={files} quiz={quiz} onOpenFile={jumpToFileByPath} />

        {/* Toolbar row (sort mode + Expand All) — NOT sticky, matching
            git-lrc: only IssueFilterBar itself is sticky (styles.css:4969),
            the Toolbar above it just scrolls away normally. */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          {canSortByRisk && (
            <div className="flex items-center gap-0.5 rounded-full border border-slate-700 bg-slate-900/80 p-0.5 text-xs">
              <span className="px-2 text-slate-500">Order By</span>
              {SORT_MODE_OPTIONS.map((opt) => (
                <button
                  key={opt.mode}
                  type="button"
                  onClick={() => setSortMode(opt.mode)}
                  title={opt.title}
                  className={classNames('rounded-full px-3 py-1', sortMode === opt.mode ? 'bg-emerald-500/20 font-semibold text-emerald-300' : 'text-slate-400 hover:text-slate-200')}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          )}
          <Button variant="outline" size="sm" onClick={toggleAll} className="ml-auto">
            {allExpanded ? 'Collapse All' : 'Expand All'}
          </Button>
        </div>

        {/* git-lrc keeps Copy Visible Issues + the vote thumbs inside
            IssueFilterBar itself (IssueFilterBar.js's toolbar-actions row),
            not the Toolbar above — see IssueFilterBar.tsx's header comment. */}
        <IssueFilterBar
          reviewId={reviewId}
          filters={filters}
          facets={facets}
          onToggleSeverity={(v) => setFilters((f) => toggleSeverityFilter(f, v))}
          onToggleConfidence={(v) => setFilters((f) => toggleConfidenceFilter(f, v))}
          onToggleType={(v) => setFilters((f) => toggleTypeFilter(f, v))}
          onToggleCategory={(v, children) => setFilters((f) => toggleCategoryFilter(f, v, children))}
          onToggleSubcategory={(v) => setFilters((f) => toggleSubcategoryFilter(f, v))}
          onReset={() => setFilters(createDefaultIssueFilters())}
          onCopyVisibleIssues={copyVisibleIssues}
          copyStatus={copyStatus}
        />
        {!!data.excluded_files?.length && (
          <p className="text-xs text-slate-500">{data.excluded_files.length} file(s) excluded from review.</p>
        )}
        <div className="space-y-3">
          {files.map((file) => (
            <FileBlock
              key={fileNavId(file)}
              reviewId={reviewId}
              file={file}
              expanded={!!expandedFiles[file.file_path]}
              onToggle={() => toggleFile(file.file_path)}
              filters={filters}
            />
          ))}
        </div>
      </div>
      <CommentNav comments={navComments} active onNavigate={jumpToComment} />
    </div>
  );
};

export default DiffViewerPanel;
