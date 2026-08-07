// Ported from git-lrc:internal/staticserve/static/app.js (convertFilesToUIFormat's
// hunk-line parsing) and internal/staticserve/static/components/DiffTable.js (as of
// the git-lrc HEAD current when this port was written). git-lrc's server pre-splits
// hunks into { OldNum, NewNum, Content, Class } lines; LiveReview's
// GET /api/v1/diff-review/:review_id only sends the raw unified-diff hunk body
// (DiffReviewHunk.content), so this file does that same line/number derivation
// client-side instead.

import type { CSSProperties } from 'react';
import { DiffReviewComment, DiffReviewFile, DiffReviewHunk } from '../../../types/reviews';

export type DiffLineType = 'add' | 'del' | 'context' | 'meta';

export interface DiffLine {
  type: DiffLineType;
  oldNum: number | null;
  newNum: number | null;
  content: string;
}

/**
 * Splits a unified-diff hunk body into per-line records with derived old/new
 * line numbers, walking forward from the hunk's declared start lines. Lines
 * that don't start with '+'/'-'/' ' (e.g. a stray "\ No newline at end of
 * file" marker) are kept as 'meta' lines with no line number.
 */
export function parseHunkLines(hunk: DiffReviewHunk): DiffLine[] {
  const body = hunk.content || '';
  const rawLines = body.length > 0 ? body.split('\n') : [];
  // A trailing split-artifact empty line (content ending in '\n') isn't a
  // real diff line.
  if (rawLines.length > 0 && rawLines[rawLines.length - 1] === '') {
    rawLines.pop();
  }

  let oldNum = hunk.old_start_line;
  let newNum = hunk.new_start_line;
  const lines: DiffLine[] = [];

  for (const raw of rawLines) {
    const marker = raw.charAt(0);
    const text = raw.slice(1);
    if (marker === '+') {
      lines.push({ type: 'add', oldNum: null, newNum, content: text });
      newNum += 1;
    } else if (marker === '-') {
      lines.push({ type: 'del', oldNum, newNum: null, content: text });
      oldNum += 1;
    } else if (marker === ' ') {
      lines.push({ type: 'context', oldNum, newNum, content: text });
      oldNum += 1;
      newNum += 1;
    } else {
      lines.push({ type: 'meta', oldNum: null, newNum: null, content: raw });
    }
  }

  return lines;
}

/**
 * True when `comment` attaches to `line`. Backend comments are always
 * matched against a hunk's new-side line numbers (see lineWithinHunks in
 * internal/api/diff_review.go), so this only ever compares against newNum.
 */
export function commentBelongsToLine(comment: DiffReviewComment, line: DiffLine): boolean {
  return line.newNum !== null && comment.line === line.newNum;
}

/** Converts a file path into a DOM-safe id for scroll-to-file navigation. */
export function filePathToId(filePath: string): string {
  return `file-${filePath.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
}

/**
 * The DOM/React-key identity for one file entry. A real file's identity is
 * its path; a synthetic per-hunk entry from flattenFilesByRisk (see
 * blastRadius.ts) carries its own syntheticId so multiple entries that share
 * the same file_path (one real file split across several ranked positions)
 * don't collide.
 */
export function fileNavId(file: DiffReviewFile): string {
  return file.syntheticId ? filePathToId(file.syntheticId) : filePathToId(file.file_path);
}

/** Stable DOM id for one comment card, used by CommentNav to scroll to it. */
export function commentDomId(filePath: string, comment: DiffReviewComment, index: number): string {
  return `comment-${filePathToId(filePath)}-${comment.line}-${index}`;
}

/** Stable DOM id for one hunk header, used by the sidebar's hunk-level nav.
 * `navId` should be fileNavId(file), not the raw file path, so synthetic
 * per-hunk entries (flattenFilesByRisk) get distinct ids. */
export function hunkDomId(navId: string, hunkIndex: number): string {
  return `hunk-${navId}-${hunkIndex}`;
}

// Every programmatic scroll-to-element in the diff viewer (sidebar file/hunk
// clicks, comment nav, Open Breakdown) has to clear TWO stacked floating
// bars, not one: LiveReview's own top <nav> (always sticky) and
// IssueFilterBar (sticky once scrolled to it — see its `data-issue-filter-
// bar` attribute). Plain `scrollIntoView({block:'start'})` (or a static
// `scroll-mt-*` class) only ever accounts for a fixed guess, so targets
// consistently land underneath one or both bars once the filter bar has
// stuck. This measures both bars' actual current height — the filter bar's
// only counts if it's actually stuck at scroll time, so jumping to
// something above its natural position doesn't over-reserve space — and
// scrolls with that combined offset instead of trusting scrollIntoView's
// own alignment.
function computeStickyScrollOffset(): number {
  const nav = document.querySelector('nav');
  const navHeight = nav ? nav.getBoundingClientRect().height : 0;
  const filterBar = document.querySelector('[data-issue-filter-bar]');
  let filterBarHeight = 0;
  if (filterBar) {
    const rect = filterBar.getBoundingClientRect();
    // Stuck iff its top edge is pinned at (roughly) the navbar's bottom
    // edge, its sticky `top` offset — anything higher means it's still in
    // normal flow above that point and isn't actually occupying space at
    // the current scroll position.
    if (rect.top <= navHeight + 2) {
      filterBarHeight = rect.height;
    }
  }
  // Generous gap, not a tight minimum — landing a target flush against the
  // sticky bars still reads as "cut off" even when technically uncovered.
  return navHeight + filterBarHeight + 32;
}

export function scrollElementIntoViewBelowStickyBars(el: HTMLElement | null): void {
  if (!el) return;
  const top = el.getBoundingClientRect().top + window.scrollY - computeStickyScrollOffset();
  window.scrollTo({ top, behavior: 'smooth' });
}

// Badge's shared info/warning/danger variants use light-mode Tailwind colors
// (bg-blue-100 text-blue-800 etc) meant for a light surface elsewhere in the
// app — on the dark comment card they render as washed-out pastel chips with
// poor contrast. These inline styles mirror git-lrc's actual severity badge
// rgba values (Comment.js/styles.css .badge-info/.badge-warning/.badge-critical),
// which were designed for a dark surface from the start.
export function severityBadgeStyle(severity?: string): React.CSSProperties {
  switch (severity) {
    case 'critical':
      return { backgroundColor: 'rgba(241,76,76,0.25)', color: '#fecaca', border: '1px solid rgba(241,76,76,0.45)' };
    case 'warning':
      return { backgroundColor: 'rgba(204,167,0,0.2)', color: '#fef08a', border: '1px solid rgba(204,167,0,0.35)' };
    case 'info':
    default:
      return { backgroundColor: 'rgba(55,148,255,0.2)', color: '#93c5fd', border: '1px solid rgba(55,148,255,0.35)' };
  }
}
