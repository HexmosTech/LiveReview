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

const CHIP_FOCUS_CLASS =
  'focus:outline-none focus-visible:ring-1 focus-visible:ring-slate-400/50';
const CHIP_BASE_CLASS =
  `inline-flex min-h-[32px] items-center gap-2 rounded-md border px-2.5 text-xs font-semibold transition-colors cursor-pointer ${CHIP_FOCUS_CLASS}`;
const CHIP_ACTIVE_CLASS =
  'border-blue-400/50 bg-blue-500/[0.16] text-blue-200 hover:bg-blue-500/20 hover:border-blue-400/60';
const CHIP_PARTIAL_CLASS =
  'border-blue-500/40 bg-blue-500/[0.10] text-blue-200/90 hover:bg-blue-500/[0.14]';
const CHIP_SUB_ACTIVE_CLASS =
  'border-blue-400/45 bg-blue-500/[0.15] text-blue-200 hover:bg-blue-500/20';
const CHIP_INACTIVE_CLASS =
  'border-slate-700/70 bg-slate-900/50 text-slate-400 hover:border-slate-600 hover:bg-slate-800/60 hover:text-slate-200';
const CHIP_PILL_ACTIVE_CLASS = 'bg-white/10 text-current';
const CHIP_PILL_INACTIVE_CLASS = 'bg-slate-800/80 text-slate-400';

const SEVERITY_ACTIVE_CLASSES: Record<string, string> = {
  critical: 'border-red-400/50 bg-red-500/[0.15] text-red-200 hover:bg-red-500/20',
  warning: 'border-amber-400/50 bg-amber-500/[0.15] text-amber-200 hover:bg-amber-500/20',
  info: 'border-sky-400/50 bg-sky-500/[0.15] text-sky-200 hover:bg-sky-500/20',
};

const SEVERITY_INACTIVE_CLASSES: Record<string, string> = {
  critical: CHIP_INACTIVE_CLASS,
  warning: CHIP_INACTIVE_CLASS,
  info: CHIP_INACTIVE_CLASS,
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
      CHIP_BASE_CLASS,
      option.active
        ? severityClass || CHIP_ACTIVE_CLASS
        : severityClass || CHIP_INACTIVE_CLASS
    )}
  >
    <span className="max-w-[240px] truncate">{option.label}</span>
    <span
      className={classNames(
        'inline-flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[11px] font-bold',
        option.active ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
      )}
    >
      {option.count}
    </span>
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

interface ToolCategoryGroup {
  category: string;
  count: number;
  tools: FacetOption[];
}

const SourceSection: React.FC<{
  tools: FacetOption[];
  onToggleTool: (value: string) => void;
}> = ({ tools, onToggleTool }) => {
  if (tools.length === 0) return null;

  const aiOption = tools.find((t) => t.value === 'ai');
  const staticTools = tools.filter((t) => t.value !== 'ai');

  const groupsMap = new Map<string, ToolCategoryGroup>();
  staticTools.forEach((tool) => {
    const cat = tool.category?.trim() || 'SAST Tools';
    const group = groupsMap.get(cat) || { category: cat, count: 0, tools: [] };
    group.count += tool.count;
    group.tools.push(tool);
    groupsMap.set(cat, group);
  });

  const orderedCategories = [...groupsMap.values()].sort((a, b) => a.category.localeCompare(b.category));

  const totalStaticCount = staticTools.reduce((acc, t) => acc + t.count, 0);
  const isAllStaticActive = staticTools.length > 0 && staticTools.every((t) => t.active);
  const isAnyStaticActive = staticTools.some((t) => t.active);

  const toggleAllStatic = () => {
    const targetState = !isAllStaticActive;
    staticTools.forEach((t) => {
      if (t.active !== targetState) {
        onToggleTool(t.value);
      }
    });
  };

  const toggleCategoryGroup = (groupTools: FacetOption[]) => {
    const allActive = groupTools.every((t) => t.active);
    const targetState = !allActive;
    groupTools.forEach((t) => {
      if (t.active !== targetState) {
        onToggleTool(t.value);
      }
    });
  };

  return (
    <div className="grid grid-cols-[minmax(120px,156px)_1fr] items-start gap-2.5">
      <span className="pt-1.5 text-[11px] font-bold uppercase tracking-wide text-slate-500">Source</span>
      <div className="grid gap-3">
        {/* Row 1: AI Review */}
        {aiOption && (
          <div className="flex items-center">
            <button
              type="button"
              onClick={() => onToggleTool('ai')}
              aria-pressed={aiOption.active}
              title={aiOption.active ? 'Disable AI Review comments' : 'Enable AI Review comments'}
              className={classNames(
                'inline-flex min-h-[34px] items-center gap-2 rounded-md border px-3 text-[13px] font-bold transition-colors cursor-pointer',
                CHIP_FOCUS_CLASS,
                aiOption.active ? CHIP_ACTIVE_CLASS : CHIP_INACTIVE_CLASS
              )}
            >
              <span>AI Review</span>
              <span
                className={classNames(
                  'inline-flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[11px] font-bold',
                  aiOption.active ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                )}
              >
                {aiOption.count}
              </span>
            </button>
          </div>
        )}

        {/* Row 2: SAST Tools */}
        {staticTools.length > 0 && (
          <div className="grid gap-2.5">
            <div className="flex items-center">
              <button
                type="button"
                onClick={toggleAllStatic}
                aria-pressed={isAllStaticActive}
                title={isAllStaticActive ? 'Disable all SAST tools' : 'Enable all SAST tools'}
                className={classNames(
                  'inline-flex min-h-[34px] items-center gap-2 rounded-md border px-3 text-[13px] font-bold transition-colors cursor-pointer',
                  CHIP_FOCUS_CLASS,
                  isAllStaticActive
                    ? CHIP_ACTIVE_CLASS
                    : isAnyStaticActive
                    ? CHIP_PARTIAL_CLASS
                    : CHIP_INACTIVE_CLASS
                )}
              >
                <span>SAST Tools</span>
                <span
                  className={classNames(
                    'inline-flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[11px] font-bold',
                    isAnyStaticActive ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                  )}
                >
                  {totalStaticCount}
                </span>
              </button>
            </div>

            {/* Static Tools Breakdown */}
            <div
              className={classNames(
                'grid gap-2.5 rounded-lg border p-2.5 transition-colors',
                isAnyStaticActive ? 'border-slate-700/60 bg-slate-900/60' : 'border-slate-800/70 bg-slate-950/35'
              )}
            >
            {/* Directly visible categories with boundary, with tools placed underneath */}
            <div className="grid gap-2.5">
              {orderedCategories.map((catGroup) => {
                const isCatAllActive = catGroup.tools.every((t) => t.active);
                const isCatAnyActive = catGroup.tools.some((t) => t.active);

                return (
                  <div
                    key={catGroup.category}
                    className={classNames(
                      'grid gap-2 rounded-lg border p-2.5 transition-colors',
                      isCatAllActive
                        ? 'border-slate-700/60 bg-slate-900/60'
                        : isCatAnyActive
                        ? 'border-slate-800/80 bg-slate-900/45'
                        : 'border-slate-800/60 bg-slate-950/30'
                    )}
                  >
                    {/* Tool Category header/toggle */}
                    <button
                      type="button"
                      onClick={() => toggleCategoryGroup(catGroup.tools)}
                      aria-pressed={isCatAllActive}
                      title={`Toggle all ${catGroup.category} tools`}
                      className={classNames(
                        'inline-flex w-fit min-h-[30px] items-center gap-2 rounded-md border px-2.5 text-xs font-bold transition-colors cursor-pointer',
                        CHIP_FOCUS_CLASS,
                        isCatAllActive
                          ? CHIP_ACTIVE_CLASS
                          : isCatAnyActive
                          ? CHIP_PARTIAL_CLASS
                          : CHIP_INACTIVE_CLASS
                      )}
                    >
                      <span>{catGroup.category}</span>
                      <span
                        className={classNames(
                          'inline-flex h-4 min-w-[18px] items-center justify-center rounded-full px-1 text-[10px] font-bold',
                          isCatAnyActive ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                        )}
                      >
                        {catGroup.count}
                      </span>
                    </button>

                    {/* Tools under this category: placed underneath with visual hierarchy border */}
                    <div className="relative flex flex-wrap gap-2 border-l border-slate-700/60 pl-[18px]">
                      {catGroup.tools.map((tool) => (
                        <button
                          key={tool.value}
                          type="button"
                          onClick={() => onToggleTool(tool.value)}
                          aria-pressed={tool.active}
                          title={`Toggle ${tool.label}`}
                          className={classNames(
                            'inline-flex min-h-[26px] items-center gap-2 rounded-full border px-2.5 text-[11px] font-semibold transition-colors cursor-pointer',
                            CHIP_FOCUS_CLASS,
                            tool.active ? CHIP_SUB_ACTIVE_CLASS : CHIP_INACTIVE_CLASS
                          )}
                        >
                          <span>{tool.label}</span>
                          <span
                            className={classNames(
                              'inline-flex h-4 min-w-[16px] items-center justify-center rounded-full px-1 text-[10px] font-bold',
                              tool.active ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                            )}
                          >
                            {tool.count}
                          </span>
                        </button>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}
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
              group.active ? 'border-slate-700/60 bg-slate-900/60' : 'border-slate-800/60 bg-slate-950/30'
            )}
          >
            <button
              type="button"
              onClick={() => onToggleCategory(group.value, group.subcategories.map((s) => s.value))}
              aria-pressed={group.active}
              title={`Toggle main category ${group.label}`}
              className={classNames(
                'inline-flex w-fit min-h-[34px] items-center gap-2 rounded-md border px-3 text-[13px] font-bold transition-colors cursor-pointer',
                CHIP_FOCUS_CLASS,
                group.active ? CHIP_ACTIVE_CLASS : CHIP_INACTIVE_CLASS
              )}
            >
              <span>{group.label}</span>
              <span
                className={classNames(
                  'inline-flex h-5 min-w-[20px] items-center justify-center rounded-full px-1.5 text-[11px] font-bold',
                  group.active ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                )}
              >
                {group.count}
              </span>
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
                      CHIP_FOCUS_CLASS,
                      sub.active ? CHIP_SUB_ACTIVE_CLASS : CHIP_INACTIVE_CLASS
                    )}
                  >
                    <span>{sub.label}</span>
                    <span
                      className={classNames(
                        'inline-flex h-4 min-w-[16px] items-center justify-center rounded-full px-1 text-[10px] font-bold',
                        sub.active ? CHIP_PILL_ACTIVE_CLASS : CHIP_PILL_INACTIVE_CLASS
                      )}
                    >
                      {sub.count}
                    </span>
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
              CHIP_FOCUS_CLASS,
              isPinnedOpen
                ? CHIP_ACTIVE_CLASS
                : 'border-slate-700/80 bg-slate-800/50 text-slate-300 hover:border-slate-600 hover:bg-slate-800/80 hover:text-slate-100'
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
              className={classNames(
                'inline-flex min-h-[32px] items-center gap-1.5 rounded-md border border-slate-700/80 bg-slate-800/50 px-3 text-xs font-semibold text-slate-300 hover:bg-slate-800 hover:text-slate-100 cursor-pointer transition-colors',
                CHIP_FOCUS_CLASS
              )}
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
          <FacetSection title="Confidence" options={facets.confidences} onToggle={onToggleConfidence} />
          <FacetSection title="Type" options={facets.types} onToggle={onToggleType} />
          <CategoryTree groups={facets.categoryGroups} onToggleCategory={onToggleCategory} onToggleSubcategory={onToggleSubcategory} />
          {facets.tools.length > 0 && (
            <SourceSection tools={facets.tools} onToggleTool={onToggleTool} />
          )}
        </div>
      </div>
    </div>
  );
};

export default IssueFilterBar;
