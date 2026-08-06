// Ported 1:1 from git-lrc:internal/staticserve/static/components/IssueFilterBar.js
// and its CSS (styles.css:4960-5460), as of the git-lrc HEAD current when this port
// was written. Structure, copy text, and hover-delay behavior match exactly:
//   .issue-filter-bar (sticky, own positioning)
//     .issue-filter-main-row: "Issue Filters" title + summary text  |  Open/Hide
//       Filters btn, Reset Filters btn, vote thumbs, Copy Visible Issues btn
//       (git-lrc keeps these inside IssueFilterBar itself, not the separate
//       Toolbar — see DiffViewerPanel.tsx's render order comment)
//     .issue-filter-secondary-row: always-visible "Severity" quick chips
//     .issue-filter-details: hover/focus-within/click-pinned expandable panel,
//       with its OWN header ("Issue Filters" + summary + hint text), opening on
//       hover with a 0.3s delay and closing with a 0.5s delay (not click-only).
// Colors use LiveReview's own Tailwind slate/blue palette rather than git-lrc's
// literal VS-Code rgba values (an agreed exception — everything else here,
// including the exact copy, spacing rhythm, and transition timing, is a direct port).
import React, { useState } from 'react';
import classNames from 'classnames';
import { Button } from '../../UIPrimitives';
import { CategoryGroup, FacetOption, FilterFacets, hasActiveIssueFilters, IssueFilters } from './issueFilters';
import VoteButtons from './VoteButtons';

const SEVERITY_ACTIVE_CLASSES: Record<string, string> = {
  critical: 'border-red-500/50 bg-red-500/[0.14] text-red-200',
  warning: 'border-amber-500/50 bg-amber-500/[0.14] text-amber-200',
  info: 'border-sky-500/50 bg-sky-500/[0.14] text-sky-200',
};

const FilterIcon: React.FC<{ size?: number }> = ({ size = 13 }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    <path d="M22 3H2l8 9.46V19l4 2v-8.54L22 3z" />
  </svg>
);

const XIcon: React.FC<{ size?: number }> = ({ size = 13 }) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
    <path d="M6 6l12 12M18 6L6 18" />
  </svg>
);

const Chip: React.FC<{ option: FacetOption; onClick: () => void; severityClass?: string }> = ({ option, onClick, severityClass }) => (
  <button
    type="button"
    onClick={onClick}
    aria-pressed={option.active}
    title={`Toggle ${option.label}`}
    className={classNames(
      'inline-flex min-h-[32px] items-center gap-2 rounded-md border px-2.5 text-xs font-semibold transition-colors',
      option.active ? severityClass || 'border-slate-600 bg-slate-700/50 text-slate-200' : 'border-slate-700 bg-slate-800/40 text-slate-500 hover:border-slate-600 hover:bg-slate-700/40 hover:text-slate-300'
    )}
  >
    <span className="max-w-[240px] truncate">{option.label}</span>
    <span className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-full bg-white/10 px-1.5 text-[11px] font-bold">{option.count}</span>
  </button>
);

const FacetSection: React.FC<{ title: string; options: FacetOption[]; onToggle: (value: string) => void }> = ({ title, options, onToggle }) => {
  if (options.length === 0) return null;
  return (
    <div className="grid grid-cols-[minmax(120px,156px)_1fr] items-start gap-2.5">
      <span className="pt-1.5 text-[11px] font-bold uppercase tracking-wide text-slate-500">{title}</span>
      <div className="flex flex-wrap gap-2">
        {options.map((opt) => <Chip key={opt.value} option={opt} onClick={() => onToggle(opt.value)} />)}
      </div>
    </div>
  );
};

const CategoryTree: React.FC<{
  groups: CategoryGroup[];
  onToggleCategory: (value: string, childSubcategories: string[]) => void;
  onToggleSubcategory: (value: string) => void;
}> = ({ groups, onToggleCategory, onToggleSubcategory }) => {
  if (groups.length === 0) return null;
  return (
    <div className="grid grid-cols-[minmax(120px,156px)_1fr] items-start gap-2.5">
      <span className="pt-1.5 text-[11px] font-bold uppercase tracking-wide text-slate-500">Classification</span>
      <div className="grid gap-2.5">
        {groups.map((group) => (
          <div
            key={group.value}
            className={classNames(
              'grid gap-2 rounded-lg border p-2.5',
              group.active ? 'border-blue-800/50 bg-slate-900/70' : 'border-slate-800 bg-slate-900/40'
            )}
          >
            <button
              type="button"
              onClick={() => onToggleCategory(group.value, group.subcategories.map((s) => s.value))}
              aria-pressed={group.active}
              title={`Toggle main category ${group.label}`}
              className={classNames(
                'inline-flex w-fit min-h-[34px] items-center gap-2 rounded-md border px-3 text-[13px] font-bold',
                group.active ? 'border-slate-600 bg-slate-700/50 text-slate-200' : 'border-slate-700 bg-slate-800/40 text-slate-500'
              )}
            >
              <span>{group.label}</span>
              <span className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-full bg-white/10 px-1.5 text-[11px] font-bold">{group.count}</span>
            </button>
            {group.subcategories.length > 0 && (
              <div className="relative flex flex-wrap gap-2 border-l border-slate-700 pl-[18px]">
                {group.subcategories.map((sub) => (
                  <button
                    key={sub.value}
                    type="button"
                    onClick={() => onToggleSubcategory(sub.value)}
                    aria-pressed={sub.active}
                    title={`Toggle subcategory ${sub.label}`}
                    className={classNames(
                      'inline-flex min-h-[26px] items-center gap-2 rounded-full border px-2.5 text-[11px] font-semibold',
                      sub.active ? 'border-blue-700/60 bg-blue-500/[0.18] text-blue-100' : 'border-slate-700 bg-slate-800/30 text-slate-500 hover:bg-slate-700/40 hover:text-slate-300'
                    )}
                  >
                    <span>{sub.label}</span>
                    <span className="inline-flex h-4 min-w-[16px] items-center justify-center rounded-full bg-white/10 px-1 text-[10px] font-bold">{sub.count}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

interface IssueFilterBarProps {
  reviewId: number;
  filters: IssueFilters;
  facets: FilterFacets;
  onToggleSeverity: (value: string) => void;
  onToggleConfidence: (value: string) => void;
  onToggleType: (value: string) => void;
  onToggleCategory: (value: string, childSubcategories: string[]) => void;
  onToggleSubcategory: (value: string) => void;
  onReset: () => void;
  onCopyVisibleIssues: () => void;
  copyStatus: 'idle' | 'copied';
}

const IssueFilterBar: React.FC<IssueFilterBarProps> = ({
  reviewId, filters, facets, onToggleSeverity, onToggleConfidence, onToggleType,
  onToggleCategory, onToggleSubcategory, onReset, onCopyVisibleIssues, copyStatus,
}) => {
  const [isPinnedOpen, setIsPinnedOpen] = useState(false);
  if (facets.total === 0) return null;
  const hasActiveFilters = hasActiveIssueFilters(filters);
  const filterLabel = facets.visible === facets.total ? `${facets.total} issues visible` : `${facets.visible} of ${facets.total} visible`;

  return (
    // "group" backs the hover-reveal .issue-filter-details below — git-lrc's
    // filter panel opens on hover/focus-within (0.3s delay), in addition to
    // being pinnable open via the "Open Filters" click toggle, and closes
    // 0.5s after the pointer leaves rather than instantly.
    <div className={classNames('group sticky top-16 z-40 flex flex-col gap-2 rounded-md border border-slate-700 bg-slate-800/95 px-3 py-2 shadow-lg backdrop-blur', isPinnedOpen && 'expanded')}>
      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="flex shrink-0 flex-wrap items-center gap-2.5">
          <span className="text-xs font-bold uppercase tracking-wide text-slate-200">Issue Filters</span>
          <span className="text-[13px] text-slate-400">{filterLabel}</span>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <button
            type="button"
            onClick={() => setIsPinnedOpen((v) => !v)}
            aria-expanded={isPinnedOpen}
            title="Toggle expanded issue filters"
            className={classNames(
              'inline-flex min-h-[32px] items-center gap-1.5 rounded-md border px-3 text-xs font-semibold',
              isPinnedOpen ? 'border-blue-700/60 bg-blue-500/[0.18] text-sky-200' : 'border-blue-800/40 bg-blue-500/10 text-sky-300 hover:bg-blue-500/[0.18]'
            )}
          >
            <FilterIcon />
            <span>{isPinnedOpen ? 'Hide Filters' : 'Open Filters'}</span>
          </button>
          {hasActiveFilters && (
            <button
              type="button"
              onClick={onReset}
              title="Reset all issue filters"
              className="inline-flex min-h-[32px] items-center gap-1.5 rounded-md border border-slate-600/50 bg-slate-700/40 px-3 text-xs font-semibold text-slate-300 hover:bg-slate-700/70"
            >
              <XIcon />
              <span>Reset Filters</span>
            </button>
          )}
          <div className="flex items-center gap-1">
            <VoteButtons reviewId={reviewId} sourceType="pr_level" size="sm" />
          </div>
          <Button variant="outline" size="sm" onClick={onCopyVisibleIssues} title="Copy all visible issues to clipboard">
            {copyStatus === 'copied' ? 'Copied!' : 'Copy Visible Issues'}
          </Button>
        </div>
      </div>

      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className="shrink-0 text-[11px] font-bold uppercase tracking-wide text-slate-500">Severity</span>
          <div className="flex min-w-0 flex-nowrap gap-1.5 overflow-x-auto">
            {facets.severities.map((opt) => (
              <Chip key={opt.value} option={opt} onClick={() => onToggleSeverity(opt.value)} severityClass={SEVERITY_ACTIVE_CLASSES[opt.value]} />
            ))}
          </div>
        </div>
      </div>

      <div
        className={classNames(
          'grid gap-3 overflow-hidden border-slate-700 transition-all duration-[180ms] ease-in-out',
          'max-h-0 -translate-y-1.5 border-t-0 pt-0 opacity-0 delay-500',
          'group-hover:max-h-[720px] group-hover:translate-y-0 group-hover:border-t group-hover:pt-1.5 group-hover:opacity-100 group-hover:delay-300',
          'group-focus-within:max-h-[720px] group-focus-within:translate-y-0 group-focus-within:border-t group-focus-within:pt-1.5 group-focus-within:opacity-100 group-focus-within:delay-300',
          isPinnedOpen && 'max-h-[720px] translate-y-0 border-t pt-1.5 opacity-100 delay-300'
        )}
      >
        <div className="flex items-start justify-between gap-3">
          <div className="flex flex-col gap-1.5">
            <div className="flex flex-wrap items-center gap-2.5">
              <span className="text-xs font-bold uppercase tracking-wide text-slate-200">Issue Filters</span>
              <span className="text-[13px] text-slate-400">{filterLabel}</span>
            </div>
            <span className="text-[11px] text-slate-600">Hover or open to browse all filter options</span>
          </div>
        </div>
        <div className="grid gap-3">
          <FacetSection title="Confidence" options={facets.confidences} onToggle={onToggleConfidence} />
          <FacetSection title="Type" options={facets.types} onToggle={onToggleType} />
          <CategoryTree groups={facets.categoryGroups} onToggleCategory={onToggleCategory} onToggleSubcategory={onToggleSubcategory} />
        </div>
      </div>
    </div>
  );
};

export default IssueFilterBar;
