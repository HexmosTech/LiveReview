// Ported from git-lrc:internal/staticserve/static/components/IssueFilterBar.js
// Position: sticky, top:0, z-index:120 — pins below the toolbar on scroll.
// Click-to-expand details panel (not hover-reveal).
import React, { useState } from 'react';
import classNames from 'classnames';
import { CategoryGroup, FacetOption, FilterFacets, hasActiveIssueFilters, IssueFilters } from './issueFilters';

const SEVERITY_CLASSES: Record<string, string> = {
  critical: 'border-red-800 bg-red-950/30 text-red-300',
  warning: 'border-amber-800 bg-amber-950/20 text-amber-300',
  info: 'border-blue-800 bg-blue-950/20 text-blue-300',
};

interface IssueFilterBarProps {
  filters: IssueFilters;
  facets: FilterFacets;
  onToggleSeverity: (v: string) => void;
  onToggleConfidence: (v: string) => void;
  onToggleType: (v: string) => void;
  onToggleCategory: (v: string, c: string[]) => void;
  onToggleSubcategory: (v: string) => void;
  onReset: () => void;
}

const Chip: React.FC<{ opt: FacetOption; onClick: () => void; severity?: string; compact?: boolean }> = ({ opt, onClick, severity, compact }) => (
  <button type="button" onClick={onClick} className={classNames(
    'issue-filter-chip inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors',
    opt.active
      ? (severity || 'border-slate-600 bg-slate-700 text-slate-200')
      : 'border-slate-700 text-slate-600 opacity-60 hover:opacity-80'
  )}><span>{opt.label}</span><span className="opacity-60">{opt.count}</span></button>
);

const IssueFilterBar: React.FC<IssueFilterBarProps> = ({
  filters, facets, onToggleSeverity, onToggleConfidence, onToggleType,
  onToggleCategory, onToggleSubcategory, onReset,
}) => {
  if (facets.total === 0) return null;

  const [open, setOpen] = useState(false);
  const hasActive = hasActiveIssueFilters(filters);
  const label = facets.visible === facets.total ? `${facets.total} issues visible` : `${facets.visible} of ${facets.total} visible`;

  return (
    <div className={classNames('issue-filter-bar', open && 'expanded')} style={{
      display: 'flex', flexDirection: 'column', gap: 8, margin: 0, padding: '8px 12px',
      background: '#1e2127', border: '1px solid rgba(110,118,129,0.25)', borderRadius: 6,
      position: 'sticky', top: 0, zIndex: 120, boxShadow: '0 10px 24px rgba(0,0,0,0.22)',
    }}>
      <div className="issue-filter-main-row" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, minWidth: 0 }}>
        <div className="issue-filter-summary-block issue-filter-summary-block-collapsed" style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
          <span className="issue-filter-title" style={{ fontSize: 12, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#e6edf3' }}>Issue Filters</span>
          <span className="issue-filter-summary-text" style={{ fontSize: 13, color: '#8b949e' }}>{label}</span>
        </div>
        <div className="issue-filter-toolbar-actions" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            className={classNames('issue-filter-expand-btn', open && 'active')}
            style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 10px', borderRadius: 6, border: '1px solid rgba(110,118,129,0.2)', background: open ? 'rgba(56,139,253,0.15)' : 'transparent', color: open ? '#58a6ff' : '#8b949e', fontSize: 12, cursor: 'pointer' }}
            onClick={() => setOpen(v => !v)}
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><path d="M22 3H2l8 9.46V19l4 2v-8.54L22 3z"></path></svg>
            {open ? 'Hide Filters' : 'Open Filters'}
          </button>
          {hasActive && (
            <button style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 10px', borderRadius: 6, border: '1px solid rgba(110,118,129,0.2)', background: 'transparent', color: '#8b949e', fontSize: 12, cursor: 'pointer' }} onClick={onReset}>
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
              Reset Filters
            </button>
          )}
        </div>
      </div>

      {/* Quick severity row */}
      {facets.severities.length > 0 && (
        <div className="issue-filter-quick-row" style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '4px 0 2px' }}>
          <span style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6e7681', whiteSpace: 'nowrap' }}>Severity</span>
          <div className="issue-filter-chip-row-compact" style={{ display: 'flex', flexWrap: 'wrap', gap: 6, overflowX: 'auto' }}>
            {facets.severities.map(opt => <Chip key={opt.value} opt={opt} onClick={() => onToggleSeverity(opt.value)} severity={SEVERITY_CLASSES[opt.value]} compact />)}
          </div>
        </div>
      )}

      {/* Expandable details */}
      {open && (
        <div className="issue-filter-details" style={{ display: 'flex', flexDirection: 'column', gap: 12, padding: '8px 0 4px', borderTop: '1px solid rgba(110,118,129,0.12)' }}>
          {facets.confidences.length > 0 && (
            <div className="issue-filter-group">
              <span style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6e7681', marginBottom: 6, display: 'block' }}>Confidence</span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>{facets.confidences.map(opt => <Chip key={opt.value} opt={opt} onClick={() => onToggleConfidence(opt.value)} />)}</div>
            </div>
          )}
          {facets.types.length > 0 && (
            <div className="issue-filter-group">
              <span style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6e7681', marginBottom: 6, display: 'block' }}>Type</span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>{facets.types.map(opt => <Chip key={opt.value} opt={opt} onClick={() => onToggleType(opt.value)} />)}</div>
            </div>
          )}
          {facets.categoryGroups.length > 0 && (
            <div className="issue-filter-group">
              <span style={{ fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6e7681', marginBottom: 6, display: 'block' }}>Classification</span>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {facets.categoryGroups.map(group => (
                  <div key={group.value} className={classNames(group.active && 'active')}>
                    <Chip opt={group} onClick={() => onToggleCategory(group.value, group.subcategories.map(s => s.value))} />
                    {group.subcategories.length > 0 && (
                      <div style={{ marginLeft: 16, marginTop: 4, display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        {group.subcategories.map(sub => (
                          <button key={sub.value} type="button" onClick={() => onToggleSubcategory(sub.value)}
                            className={classNames('rounded-full border px-2 py-0.5 text-[11px]', sub.active ? 'border-slate-600 bg-slate-700 text-slate-300' : 'border-slate-700 text-slate-600 opacity-60')}>
                            {sub.label} <span className="opacity-60">{sub.count}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default IssueFilterBar;
