// Ported from git-lrc:internal/staticserve/static/components/issue_filter_model.mjs
// and issue_filter_state.mjs (as of the git-lrc HEAD current when this port was
// written) — full facet set: severity, confidence, type, and a category ->
// subcategory tree (toggling a category cascades to its subcategories, matching
// toggleIssueFilterValue's category branch in issue_filter_model.mjs).
import { DiffReviewComment, DiffReviewFile, DiffReviewHunk } from '../../../types/reviews';

export const DEFAULT_SEVERITIES = ['critical', 'warning', 'info'] as const;
export const CONFIDENCE_ORDER = ['high', 'medium', 'low'];

export interface IssueFilters {
  disabledSeverities: Set<string>;
  disabledConfidences: Set<string>;
  disabledTypes: Set<string>;
  disabledCategories: Set<string>;
  disabledSubcategories: Set<string>;
}

export function createDefaultIssueFilters(): IssueFilters {
  return {
    disabledSeverities: new Set(),
    disabledConfidences: new Set(),
    disabledTypes: new Set(),
    disabledCategories: new Set(),
    disabledSubcategories: new Set(),
  };
}

export function cloneIssueFilters(filters: IssueFilters): IssueFilters {
  return {
    disabledSeverities: new Set(filters.disabledSeverities),
    disabledConfidences: new Set(filters.disabledConfidences),
    disabledTypes: new Set(filters.disabledTypes),
    disabledCategories: new Set(filters.disabledCategories),
    disabledSubcategories: new Set(filters.disabledSubcategories),
  };
}

export function hasActiveIssueFilters(filters: IssueFilters): boolean {
  return (
    filters.disabledSeverities.size > 0 ||
    filters.disabledConfidences.size > 0 ||
    filters.disabledTypes.size > 0 ||
    filters.disabledCategories.size > 0 ||
    filters.disabledSubcategories.size > 0
  );
}

function toggle(set: Set<string>, value: string) {
  if (set.has(value)) set.delete(value);
  else set.add(value);
}

export function toggleSeverityFilter(filters: IssueFilters, severity: string): IssueFilters {
  const next = cloneIssueFilters(filters);
  toggle(next.disabledSeverities, severity);
  return next;
}

export function toggleConfidenceFilter(filters: IssueFilters, confidence: string): IssueFilters {
  const next = cloneIssueFilters(filters);
  toggle(next.disabledConfidences, confidence.toLowerCase());
  return next;
}

export function toggleTypeFilter(filters: IssueFilters, type: string): IssueFilters {
  const next = cloneIssueFilters(filters);
  toggle(next.disabledTypes, type.toLowerCase());
  return next;
}

/** Toggling a category cascades to every subcategory under it, mirroring
 * git-lrc's toggleIssueFilterValue category branch. */
export function toggleCategoryFilter(filters: IssueFilters, category: string, childSubcategories: string[] = []): IssueFilters {
  const next = cloneIssueFilters(filters);
  const value = category.toLowerCase();
  if (next.disabledCategories.has(value)) {
    next.disabledCategories.delete(value);
    childSubcategories.forEach((s) => next.disabledSubcategories.delete(s.toLowerCase()));
  } else {
    next.disabledCategories.add(value);
    childSubcategories.forEach((s) => next.disabledSubcategories.add(s.toLowerCase()));
  }
  return next;
}

export function toggleSubcategoryFilter(filters: IssueFilters, subcategory: string): IssueFilters {
  const next = cloneIssueFilters(filters);
  toggle(next.disabledSubcategories, subcategory.toLowerCase());
  return next;
}

function normalizeSeverity(severity?: string): string {
  const s = (severity || '').toLowerCase();
  return (DEFAULT_SEVERITIES as readonly string[]).includes(s) ? s : 'info';
}

export function commentMatchesFilters(comment: DiffReviewComment, filters: IssueFilters): boolean {
  if (filters.disabledSeverities.has(normalizeSeverity(comment.severity))) return false;
  const confidence = (comment.confidence || '').toLowerCase();
  if (confidence && filters.disabledConfidences.has(confidence)) return false;
  const type = (comment.type || '').toLowerCase();
  if (type && filters.disabledTypes.has(type)) return false;
  const category = (comment.category || '').toLowerCase();
  if (category && filters.disabledCategories.has(category)) return false;
  const subcategory = (comment.subcategory || '').toLowerCase();
  if (subcategory && filters.disabledSubcategories.has(subcategory)) return false;
  return true;
}

export interface FacetOption {
  value: string;
  label: string;
  count: number;
  active: boolean;
}

export interface CategoryGroup extends FacetOption {
  subcategories: FacetOption[];
}

export interface FilterFacets {
  severities: FacetOption[];
  confidences: FacetOption[];
  types: FacetOption[];
  categoryGroups: CategoryGroup[];
  total: number;
  visible: number;
}

function formatLabel(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function sortByOrder(values: string[], preferredOrder: string[]): string[] {
  return [...values].sort((a, b) => {
    const ai = preferredOrder.indexOf(a);
    const bi = preferredOrder.indexOf(b);
    if (ai !== -1 || bi !== -1) {
      if (ai === -1) return 1;
      if (bi === -1) return -1;
      return ai - bi;
    }
    return a.localeCompare(b);
  });
}

export function buildFilterFacets(files: DiffReviewFile[], filters: IssueFilters): FilterFacets {
  const severityCounts = new Map<string, number>();
  const confidenceCounts = new Map<string, number>();
  const typeCounts = new Map<string, { label: string; count: number }>();
  // categoryKey -> { label, count, subcategories: Map<key, {label, count}> }
  const categoryMap = new Map<string, { label: string; count: number; subcategories: Map<string, { label: string; count: number }> }>();
  let total = 0;
  let visible = 0;

  files.forEach((file) => {
    (file.comments || []).forEach((comment) => {
      total++;
      const severity = normalizeSeverity(comment.severity);
      severityCounts.set(severity, (severityCounts.get(severity) || 0) + 1);

      if (comment.confidence) {
        const key = comment.confidence.toLowerCase();
        confidenceCounts.set(key, (confidenceCounts.get(key) || 0) + 1);
      }
      if (comment.type) {
        const key = comment.type.toLowerCase();
        const existing = typeCounts.get(key);
        typeCounts.set(key, { label: comment.type, count: (existing?.count || 0) + 1 });
      }
      if (comment.category) {
        const catKey = comment.category.toLowerCase();
        const catEntry = categoryMap.get(catKey) || { label: comment.category, count: 0, subcategories: new Map() };
        catEntry.count += 1;
        if (comment.subcategory) {
          const subKey = comment.subcategory.toLowerCase();
          const subEntry = catEntry.subcategories.get(subKey) || { label: comment.subcategory, count: 0 };
          subEntry.count += 1;
          catEntry.subcategories.set(subKey, subEntry);
        }
        categoryMap.set(catKey, catEntry);
      }

      if (commentMatchesFilters(comment, filters)) visible++;
    });
  });

  const categoryGroups: CategoryGroup[] = [...categoryMap.entries()]
    .sort((a, b) => a[1].label.localeCompare(b[1].label))
    .map(([catKey, entry]) => ({
      value: catKey,
      label: entry.label,
      count: entry.count,
      active: !filters.disabledCategories.has(catKey),
      subcategories: [...entry.subcategories.entries()]
        .sort((a, b) => a[1].label.localeCompare(b[1].label))
        .map(([subKey, sub]) => ({
          value: subKey,
          label: sub.label,
          count: sub.count,
          active: !filters.disabledCategories.has(catKey) && !filters.disabledSubcategories.has(subKey),
        })),
    }));

  return {
    severities: DEFAULT_SEVERITIES.map((s) => ({
      value: s,
      label: formatLabel(s),
      count: severityCounts.get(s) || 0,
      active: !filters.disabledSeverities.has(s),
    })),
    confidences: sortByOrder([...confidenceCounts.keys()], CONFIDENCE_ORDER).map((v) => ({
      value: v,
      label: formatLabel(v),
      count: confidenceCounts.get(v) || 0,
      active: !filters.disabledConfidences.has(v),
    })),
    types: [...typeCounts.entries()]
      .sort((a, b) => a[1].label.localeCompare(b[1].label))
      .map(([value, { label, count }]) => ({ value, label, count, active: !filters.disabledTypes.has(value) })),
    categoryGroups,
    total,
    visible,
  };
}

export function countFileVisibleComments(file: DiffReviewFile, filters: IssueFilters): number {
  return (file.comments || []).filter((c) => commentMatchesFilters(c, filters)).length;
}

export function countHunkVisibleComments(hunk: DiffReviewHunk, fileComments: DiffReviewComment[], filters: IssueFilters): number {
  const startLine = hunk.new_start_line;
  const endLine = startLine + hunk.new_line_count;
  return fileComments.filter((c) => c.line >= startLine && c.line < endLine && commentMatchesFilters(c, filters)).length;
}
