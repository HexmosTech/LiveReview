import React, { useEffect, useRef, useState } from 'react';
import { LuListFilter } from 'react-icons/lu';
import { FaArrowUpLong, FaArrowDownLong } from 'react-icons/fa6';
import { Tooltip, parseMultiFilterValue } from '../UIPrimitives';

// Shared header/cell building blocks for pages that build their own
// client-side TanStack table (Explore > Repositories, Explore > Merge
// Requests) - sort icon + label, a click-to-open filter/search popover, the
// checkbox-multiselect columnFilter matcher, and a tooltip-on-truncate
// wrapper. Kept separate from the app's general UIPrimitives.tsx since these
// are TanStack-column-shaped (expect `column`/`onToggle`/filterFn signatures)
// rather than generic UI.

/** Wraps `children` in the app's Tooltip only when `text` actually got cut
 * down to `max` chars - otherwise the tooltip would just repeat what's
 * already visible. */
export const TruncatedWithTooltip: React.FC<{ text: string; max: number; children: React.ReactNode }> = ({ text, max, children }) => {
  if (text.length <= max) return <>{children}</>;
  return <Tooltip content={text}>{children}</Tooltip>;
};

/** Sort indicator icon - distinct up/down arrows (FaArrowUpLong/FaArrowDownLong)
 * rather than one icon rotated. Solid white/slate to match header text,
 * sized to match a 14px (text-sm) header label's height. It's its own
 * button (not just a decorative glyph next to the label) so it's
 * independently clickable and gets a pointer cursor, since it's meant to sit
 * at the trailing end of the column rather than glued to the label text. */
export const SortIcon: React.FC<{ sorted: false | 'asc' | 'desc'; onToggle: (e: unknown) => void }> = ({ sorted, onToggle }) => {
  const ArrowIcon = sorted === 'desc' ? FaArrowDownLong : FaArrowUpLong;
  return (
    <button type="button" onClick={onToggle} aria-label="Toggle sort" className="inline-flex items-center cursor-pointer">
      <ArrowIcon size={12} className="text-slate-300" />
    </button>
  );
};

/** Click-to-sort label, shared by every sortable column header. Cycles
 * asc -> desc -> none on each click via TanStack's own
 * column.getToggleSortingHandler(). Just the label text/button - the sort
 * icon is meant to live in a trailing icon group placed separately in the
 * header (see SortIcon) so it can line up with a filter icon at the end of
 * the column instead of being glued to the label text. */
export const SortableHeaderLabel: React.FC<{ label: string; onToggle: (e: unknown) => void }> = ({ label, onToggle }) => (
  <button
    type="button"
    onClick={onToggle}
    className="text-left font-semibold text-slate-300 uppercase tracking-wide text-xs hover:text-slate-200 cursor-pointer"
  >
    {label}
  </button>
);

/** Icon button that opens/closes a small popover on click (instead of a
 * permanently-visible filter/search row under the header). Defaults to the
 * filter (funnel) icon for checkbox filters; pass a different `icon` (e.g. a
 * search glyph) for a free-text search column. */
export const HeaderFilterPopover: React.FC<{ children: React.ReactNode; icon?: React.FC<{ size?: number }>; label?: string }> = ({ children, icon: TriggerIcon = LuListFilter, label = 'Open filter' }) => {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onClickOutside = (ev: MouseEvent) => {
      if (!rootRef.current) return;
      if (!rootRef.current.contains(ev.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, [open]);

  return (
    <div ref={rootRef} className="relative inline-flex">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={label}
        className="inline-flex items-center text-slate-300 hover:text-slate-200 cursor-pointer"
      >
        <TriggerIcon size={12} />
      </button>
      {open && (
        <div className="absolute z-30 top-full left-0 mt-2 w-56 rounded-lg border border-slate-600 bg-slate-800 shadow-xl p-2 font-normal normal-case tracking-normal">
          {children}
        </div>
      )}
    </div>
  );
};

/** Checkbox-multiselect columnFilter: value is a comma-joined string of
 * selected raw values (same convention as MultiSelectField/MultiSelectPanel),
 * empty meaning "no filter applied" (matches all rows). Column should
 * accessor/accessorFn a value drawn from the same short set of values the
 * filter's checkbox options use (e.g. a normalized key), not a raw
 * free-form string, or the exact-match check here will never match. */
export const multiSelectFilterFn = (row: any, columnId: string, filterValue: string) => {
  const selected = parseMultiFilterValue(filterValue);
  if (selected.length === 0) return true;
  return selected.includes(String(row.getValue(columnId)));
};
