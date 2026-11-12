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
  critical: 'border-red-500/50 bg-red-500/[0.14] text-[#fecaca]',
  warning: 'border-amber-500/50 bg-amber-500/[0.14] text-[#fef08a]',
  info: 'border-sky-500/50 bg-sky-500/[0.14] text-[#93c5fd]',
};

const SEVERITY_INACTIVE_CLASSES: Record<string, string> = {
  critical: 'border-red-500/30 bg-red-500/[0.06] text-red-300/85 hover:bg-red-500/[0.12] hover:border-red-500/50',
  warning: 'border-amber-500/30 bg-amber-500/[0.06] text-amber-300/85 hover:bg-amber-500/[0.12] hover:border-amber-500/50',
  info: 'border-sky-500/30 bg-sky-500/[0.06] text-sky-300/85 hover:bg-sky-500/[0.12] hover:border-sky-500/50',
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
      'inline-flex min-h-[32px] items-center gap-2 rounded-md border px-2.5 text-xs font-semibold transition-colors cursor-pointer',
      option.active
        ? severityClass || 'border-blue-500/40 bg-blue-500/20 text-[#eef6ff]'
        : severityClass || 'border-[rgba(110,118,129,0.22)] bg-[rgba(69,78,89,0.3)] text-slate-300 hover:bg-[rgba(84,94,106,0.36)] hover:border-[rgba(132,141,153,0.36)] hover:text-white'
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
              'grid gap-2 rounded-lg border p-2.5 transition-colors',
              group.active ? 'border-blue-700/50 bg-slate-900/70' : 'border-slate-700/40 bg-slate-900/40'
            )}
          >
            <button
              type="button"
              onClick={() => onToggleCategory(group.value, group.subcategories.map((s) => s.value))}
              aria-pressed={group.active}
              title={`Toggle main category ${group.label}`}
              className={classNames(
                'inline-flex w-fit min-h-[34px] items-center gap-2 rounded-md border px-3 text-[13px] font-bold transition-colors cursor-pointer',
                group.active ? 'border-blue-500/50 bg-blue-500/20 text-[#eef6ff]' : 'border-[rgba(110,118,129,0.22)] bg-[rgba(69,78,89,0.3)] text-slate-300 hover:bg-[rgba(84,94,106,0.36)] hover:text-white'
              )}
            >
              <span>{group.label}</span>
              <span className="inline-flex h-5 min-w-[20px] items-center justify-center rounded-full bg-white/10 px-1.5 text-[11px] font-bold">{group.count}</span>
            </button>
            {group.subcategories.length > 0 && (
              <div className="relative flex flex-wrap gap-2 border-l border-slate-700/60 pl-[18px]">
                {group.subcategories.map((sub) => (
                  <button
                    key={sub.value}
                    type="button"
                    onClick={() => onToggleSubcategory(sub.value)}
                    aria-pressed={sub.active}
                    title={`Toggle subcategory ${sub.label}`}
                    className={classNames(
                      'inline-flex min-h-[26px] items-center gap-2 rounded-full border px-2.5 text-[11px] font-semibold transition-colors cursor-pointer',
                      sub.active ? 'border-blue-700/60 bg-blue-500/[0.18] text-blue-100' : 'border-[rgba(110,118,129,0.22)] bg-[rgba(69,78,89,0.28)] text-slate-300 hover:bg-slate-700/40 hover:text-white'
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
  onToggleTool: (value: string) => void;
  onReset: () => void;
  onCopyVisibleIssues: () => void;
  copyStatus: 'idle' | 'copied';
}

const IssueFilterBar: React.FC<IssueFilterBarProps> = ({
  reviewId, filters, facets, onToggleSeverity, onToggleConfidence, onToggleType,
  onToggleCategory, onToggleSubcategory, onToggleTool, onReset, onCopyVisibleIssues, copyStatus,
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
    <div
      data-issue-filter-bar
      className={classNames(
        'issue-filter-bar sticky top-16 z-40 flex flex-col gap-2 rounded-md border border-slate-700 bg-slate-800/95 px-3 py-2 shadow-lg backdrop-blur',
        isPinnedOpen && 'expanded'
      )}
    >
      <style>{`
        .issue-filter-details {
          max-height: 0;
          opacity: 0;
          overflow: hidden;
          pointer-events: none;
          transform: translateY(-6px);
          scrollbar-gutter: stable;
          transition:
            max-height 0.18s ease,
            opacity 0.18s ease,
            transform 0.18s ease,
            padding-top 0.18s ease;
          transition-delay: 0.5s;
          padding-top: 0;
        }

        .issue-filter-bar:hover .issue-filter-details,
        .issue-filter-bar:focus-within .issue-filter-details,
        .issue-filter-bar.expanded .issue-filter-details {
          max-height: min(720px, calc(100vh - 170px));
          opacity: 1;
          pointer-events: auto;
          transform: translateY(0);
          padding-top: 8px;
          padding-right: 4px;
          overflow-y: auto;
          overscroll-behavior: contain;
          scrollbar-width: thin;
          scrollbar-color: rgba(148, 163, 184, 0.4) rgba(255, 255, 255, 0.04);
          transition-delay: 0.3s;
        }

        .issue-filter-bar.expanded .issue-filter-details {
          transition-delay: 0s;
        }

        .issue-filter-bar:hover,
        .issue-filter-bar:focus-within,
        .issue-filter-bar.expanded {
          box-shadow: 0 14px 28px rgba(0, 0, 0, 0.28);
          outline: 1px solid rgba(148, 163, 184, 0.18);
        }
      `}</style>
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
            title={isPinnedOpen ? 'Hide filter options' : 'Open filter options'}
            className={classNames(
              'inline-flex min-h-[32px] items-center gap-1.5 rounded-md border px-3 text-xs font-semibold cursor-pointer transition-colors',
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
              className="inline-flex min-h-[32px] items-center gap-1.5 rounded-md border border-slate-600/50 bg-slate-700/40 px-3 text-xs font-semibold text-slate-300 hover:bg-slate-700/70 cursor-pointer"
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
              <Chip
                key={opt.value}
                option={opt}
                onClick={() => onToggleSeverity(opt.value)}
                severityClass={opt.active ? SEVERITY_ACTIVE_CLASSES[opt.value] : SEVERITY_INACTIVE_CLASSES[opt.value]}
              />
            ))}
          </div>
        </div>
      </div>

      <div className="issue-filter-details">
        <div className="issue-filter-details-header flex items-start justify-between gap-3 mb-2.5">
          <div className="flex flex-col gap-0.5">
            <span className="text-xs font-bold uppercase tracking-wider text-slate-200">Issue Filters</span>
            <span className="text-[13px] text-slate-400">{filterLabel}</span>
            <span className="text-[11px] text-slate-500">Hover or open to browse all filter options</span>
          </div>
        </div>
        <div className="grid gap-3">
          {facets.tools.length > 0 && (
            <FacetSection title="Tool" options={facets.tools} onToggle={onToggleTool} />
          )}
          <FacetSection title="Confidence" options={facets.confidences} onToggle={onToggleConfidence} />
          <FacetSection title="Type" options={facets.types} onToggle={onToggleType} />
          <CategoryTree groups={facets.categoryGroups} onToggleCategory={onToggleCategory} onToggleSubcategory={onToggleSubcategory} />
        </div>
      </div>
    </div>
  );
};

export default IssueFilterBar;
