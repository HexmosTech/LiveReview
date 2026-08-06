// Ported from git-lrc:internal/staticserve/static/components/blast_radius_sort_state.mjs
// and callgraph_model.mjs (as of the git-lrc HEAD current when this port was written),
// including the whole-diff SORT_MODE_RISK_FLAT mode (flattenFilesByRisk) that dissolves
// file boundaries into one globally-ranked stream of synthetic single-hunk "files" —
// see flattenFilesByRisk below and diffUtils.ts's fileNavId for how the resulting
// duplicate file_paths get distinct DOM/React identities.

import {
  BlastRadiusCallerRef,
  BlastRadiusHunkReport,
  BlastRadiusReport,
  BlastRadiusSignal,
  DiffReviewFile,
  DiffReviewHunk,
} from '../types/reviews';

export type BlastRadiusTier = 'blast-radius-high' | 'blast-radius-medium' | 'blast-radius-low' | 'blast-radius-none';

/** Maps a 0-100 score to a discrete severity tier (badge color scheme). */
export function blastRadiusTier(score: number): BlastRadiusTier {
  if (score >= 66) return 'blast-radius-high';
  if (score >= 33) return 'blast-radius-medium';
  if (score > 0) return 'blast-radius-low';
  return 'blast-radius-none';
}

export function blastRadiusTierLabel(score: number): string {
  if (score >= 66) return 'High risk';
  if (score >= 33) return 'Moderate risk';
  if (score > 0) return 'Low risk';
  return 'Minimal risk';
}

/**
 * Flattens a report hunk into every Signal that contributed to it — the
 * hunk's own (file coupling, arch role) plus every touched symbol's —
 * ranked by absolute contribution. BlastRadiusRaw/ReviewPriorityRaw are
 * literally the sum of this list (see blastradius.go's sumSignalPoints);
 * detail.Signals alone is only a small fraction of it.
 */
export function allSignals(detail: BlastRadiusHunkReport | null | undefined): BlastRadiusSignal[] {
  if (!detail) return [];
  const all: BlastRadiusSignal[] = [...(detail.Signals || [])];
  (detail.Symbols || []).forEach((sym) => {
    (sym.Signals || []).forEach((s) => all.push({ ...s, _symbolName: sym.Name || sym.QualifiedName }));
  });
  all.sort((a, b) => Math.abs(b.Points || 0) - Math.abs(a.Points || 0));
  return all;
}

/** Join key between UI hunks and a blast-radius report's hunks. */
export function hunkBlastKey(filePath: string, newStart: number, newLines: number): string {
  return `${filePath}:${newStart}:${newLines}`;
}

export function buildBlastLookup(report: BlastRadiusReport | null | undefined): Map<string, BlastRadiusHunkReport> {
  const lookup = new Map<string, BlastRadiusHunkReport>();
  (report?.Files || []).forEach((file) => {
    (file.Hunks || []).forEach((hunk) => {
      lookup.set(hunkBlastKey(file.Path, hunk.NewStart, hunk.NewLines), hunk);
    });
  });
  return lookup;
}

/** Looks up the report hunk (if any) matching a diff-review hunk. */
export function lookupBlastDetail(
  lookup: Map<string, BlastRadiusHunkReport>,
  filePath: string,
  hunk: DiffReviewHunk
): BlastRadiusHunkReport | undefined {
  return lookup.get(hunkBlastKey(filePath, hunk.new_start_line, hunk.new_line_count));
}

/**
 * Returns new file objects whose hunks carry BlastRadius (the Combined 0-100
 * score) and BlastDetail (the full report hunk) joined from the lookup.
 * Hunks with no lookup entry are returned unchanged. Inputs are never
 * mutated. Call this once per fetch (not per render) — sortFilesByBlastRadius
 * and hasBlastRadiusData both read hunk.BlastRadius, so it needs to already
 * be attached before either runs.
 */
export function attachBlastData(files: DiffReviewFile[], lookup: Map<string, BlastRadiusHunkReport>): DiffReviewFile[] {
  if (!lookup || lookup.size === 0) return files;
  return files.map((file) => ({
    ...file,
    hunks: (file.hunks || []).map((hunk) => {
      const detail = lookupBlastDetail(lookup, file.file_path, hunk);
      if (!detail) return hunk;
      return { ...hunk, BlastRadius: detail.Combined, BlastDetail: detail };
    }),
  }));
}

function normalizedScore(hunk: DiffReviewHunk): number | null {
  const value = hunk.BlastRadius;
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

/** True when any hunk across files carries a computed score. */
export function hasBlastRadiusData(files: DiffReviewFile[]): boolean {
  return files.some((file) => (file.hunks || []).some((hunk) => normalizedScore(hunk) !== null));
}

/**
 * Returns a new array of hunks ordered by descending score; hunks with no
 * score keep their original relative order and sort after every scored hunk.
 * The input array is never mutated.
 */
export function sortHunksByBlastRadius(hunks: DiffReviewHunk[]): DiffReviewHunk[] {
  return hunks
    .map((hunk, index) => ({ hunk, index, score: normalizedScore(hunk) }))
    .sort((a, b) => {
      if ((a.score === null) !== (b.score === null)) return a.score === null ? 1 : -1;
      if (a.score === null) return a.index - b.index;
      return (b.score as number) - (a.score as number);
    })
    .map((entry) => entry.hunk);
}

/**
 * Returns new file objects with hunks reordered by sortHunksByBlastRadius,
 * and the files themselves reordered by their own highest-scoring hunk
 * (descending) — so both risky hunks within a file and risky files across
 * the diff bubble to the top. Files with no scored hunks keep their original
 * relative order, after every scored file.
 */
export function sortFilesByBlastRadius(files: DiffReviewFile[]): DiffReviewFile[] {
  return files
    .map((file, index) => {
      const sortedHunks = sortHunksByBlastRadius(file.hunks || []);
      const topScore = sortedHunks.length > 0 ? normalizedScore(sortedHunks[0]) : null;
      return { file: { ...file, hunks: sortedHunks }, index, topScore };
    })
    .sort((a, b) => {
      if ((a.topScore === null) !== (b.topScore === null)) return a.topScore === null ? 1 : -1;
      if (a.topScore === null) return a.index - b.index;
      return (b.topScore as number) - (a.topScore as number);
    })
    .map((entry) => entry.file);
}

/**
 * Dissolves file boundaries into one globally ranked hunk list: each entry
 * is a synthetic single-hunk "file" (so FileBlock renders it unchanged) in
 * descending BlastRadius order. Unscored hunks keep their diff order after
 * every scored hunk. Mirrors git-lrc's flattenFilesByRisk exactly, except
 * the synthetic identity lives in `syntheticId`/`sourceHunkNumber` (see
 * diffUtils.ts's fileNavId) instead of git-lrc's ID/ExpandKey/RiskRank
 * fields, since LiveReview's expand-state is already keyed by file_path
 * (equivalent to git-lrc's ExpandKey) rather than needing a separate field.
 */
export function flattenFilesByRisk(files: DiffReviewFile[]): DiffReviewFile[] {
  interface Entry { file: DiffReviewFile; fileIdx: number; hunk: DiffReviewHunk; hunkIdx: number; score: number | null }
  const entries: Entry[] = [];
  files.forEach((file, fileIdx) => {
    (file.hunks || []).forEach((hunk, hunkIdx) => {
      entries.push({ file, fileIdx, hunk, hunkIdx, score: normalizedScore(hunk) });
    });
  });

  entries.sort((a, b) => {
    if ((a.score === null) !== (b.score === null)) return a.score === null ? 1 : -1;
    if (a.score === null || a.score === b.score) return a.fileIdx - b.fileIdx || a.hunkIdx - b.hunkIdx;
    return (b.score as number) - (a.score as number);
  });

  return entries.map(({ file, fileIdx, hunk, hunkIdx }) => ({
    ...file,
    hunks: [hunk],
    syntheticId: `${file.file_path}--hunk-${fileIdx}-${hunkIdx}`,
    sourceHunkNumber: hunkIdx + 1,
  }));
}

// ===== Call-graph presentation (from callgraph_model.mjs) =====

export function shortName(qualifiedName: string | undefined): string {
  const parts = (qualifiedName || '').split('.');
  return parts[parts.length - 1] || qualifiedName || '';
}

export interface CallerGroup {
  key: string;
  depth: number;
  preRename: boolean;
  callers: BlastRadiusCallerRef[];
}

/**
 * Buckets a caller list for display. Pre-rename callers get their own
 * bucket ahead of the depth buckets rather than being folded into "Direct
 * callers": they were found under a different name and are worth looking
 * at first.
 */
export function groupCallers(callers: BlastRadiusCallerRef[] | undefined): CallerGroup[] {
  const preRename: BlastRadiusCallerRef[] = [];
  const byDepth = new Map<number, BlastRadiusCallerRef[]>();
  (callers || []).forEach((c) => {
    if (c.PreRename) {
      preRename.push(c);
      return;
    }
    const depth = c.Depth || 1;
    if (!byDepth.has(depth)) byDepth.set(depth, []);
    byDepth.get(depth)!.push(c);
  });

  const groups: CallerGroup[] = [...byDepth.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([depth, list]) => ({ key: `d${depth}`, depth, preRename: false, callers: list }));

  if (preRename.length > 0) {
    groups.unshift({ key: 'pre-rename', depth: 1, preRename: true, callers: preRename });
  }
  return groups;
}

/**
 * Names a caller-group bucket. The pre-rename label states only what was
 * measured — a textual occurrence of the old name — not what it implies (a
 * grep hit isn't necessarily a broken call).
 */
export function callerGroupLabel(group: CallerGroup, oldName?: string): string {
  if (group.preRename) {
    return oldName ? `Still uses the old name "${oldName}"` : 'Still uses the old name';
  }
  if (group.depth === 1) return 'Direct callers';
  return `${group.depth} calls away`;
}

export interface CallHierarchyNode {
  name: string;
  qualifiedName: string;
  children: CallHierarchyNode[];
  isIntermediate?: boolean;
  isLeaf?: boolean;
  depth?: number;
  weight?: number;
}

/**
 * Turns a symbol's flat caller list into the nested shape both charts
 * render. Callers arrive with a Path of intermediate nodes between the
 * symbol and themselves (ordered outward), so a depth-3 caller contributes
 * two intermediate levels plus its own leaf.
 *
 * Must only ever see genuine CALLS-graph callers — a symbol's pre-rename
 * callers (found by text search, not a real graph edge) should be filtered
 * out by the caller before this runs.
 */
export function buildHierarchy(symbol: {
  Name?: string;
  QualifiedName: string;
  Callers?: BlastRadiusCallerRef[];
}): CallHierarchyNode {
  const callers = symbol.Callers || [];
  const nodeMap = new Map<string, CallHierarchyNode & { _names: Set<string> }>();
  const children: CallHierarchyNode[] = [];
  const childrenNames = new Set<string>();

  function ensureNode(qualifiedName: string, isIntermediate: boolean) {
    let node = nodeMap.get(qualifiedName);
    if (!node) {
      node = {
        name: shortName(qualifiedName),
        qualifiedName,
        children: [],
        _names: new Set<string>(),
      };
      if (isIntermediate) node.isIntermediate = true;
      nodeMap.set(qualifiedName, node);
    }
    return node;
  }

  for (const caller of callers) {
    const path = caller.Path || [];

    if (path.length === 0) {
      const node = ensureNode(caller.QualifiedName, false);
      node.depth = caller.Depth;
      node.weight = caller.Weight;
      node.isLeaf = true;
      children.push(node);
      childrenNames.add(caller.QualifiedName);
      continue;
    }

    const firstVia = path[0];
    const viaNode = ensureNode(firstVia, true);
    if (!childrenNames.has(firstVia)) {
      children.push(viaNode);
      childrenNames.add(firstVia);
    }
    if (!viaNode.depth) viaNode.depth = 1;

    let parent = viaNode;
    for (let i = 1; i < path.length; i++) {
      const childNode = ensureNode(path[i], true);
      if (!parent._names.has(path[i])) {
        parent.children.push(childNode);
        parent._names.add(path[i]);
      }
      if (!childNode.depth) childNode.depth = i + 1;
      parent = childNode;
    }

    const leafNode = ensureNode(caller.QualifiedName, false);
    leafNode.depth = caller.Depth;
    leafNode.weight = caller.Weight;
    leafNode.isLeaf = true;
    if (!parent._names.has(caller.QualifiedName)) {
      parent.children.push(leafNode);
      parent._names.add(caller.QualifiedName);
    }
  }

  return {
    name: symbol.Name || shortName(symbol.QualifiedName),
    qualifiedName: symbol.QualifiedName,
    children,
  };
}

export function emptyCallGraphMessage(symbol: { Method?: string } | null | undefined): string {
  if (symbol && symbol.Method === 'text-references') {
    return 'Text reference method — no call graph available';
  }
  return 'No callers in the dependency graph';
}
