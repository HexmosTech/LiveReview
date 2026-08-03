import React from 'react';
import { flexRender, Table } from '@tanstack/react-table';
import { motion, AnimatePresence } from 'framer-motion';
import { Button, Icons, Spinner } from '../UIPrimitives';
import { DataTablePaginationControls, DataTableEmptyState } from './DataTable';

export interface ClientTableProps<TData> {
  /** An already-configured TanStack table instance (client-side row model,
   * with whatever filter/sort/pagination row models the page needs). This
   * component only renders it - all data/column/state logic stays in the
   * page, same division of responsibility as the plain <DataTable>. */
  table: Table<TData>;
  /** Column widths (colgroup), one per column in table order, e.g. ['32%', '17%', ...].
   * Needed because table-fixed layout (used so columns stay a fixed,
   * consistent width instead of auto-sizing from content) requires an
   * explicit width per column. */
  columnWidths: string[];
  loading: boolean;
  loadingLabel?: string;
  error: string | null;
  onRetry: () => void;
  /** Whether to show the big empty-state graphic. Deliberately a separate
   * flag rather than `table.getRowModel().rows.length === 0` - the caller
   * decides based on the raw/unfiltered data array, so "filters excluded
   * everything" still renders the table (with an empty body) instead of the
   * "no repositories yet, go sync a connector"-style empty state. */
  isEmpty: boolean;
  empty: DataTableEmptyState;
  pageSizeOptions: number[];
  /** Optional whole-row click (e.g. Reviews rows have always navigated to
   * the review's detail page on click, in addition to any explicit action
   * button in a cell). Adds a pointer cursor + keyboard (Enter/Space)
   * activation to each row when set; omit for pages that only expose
   * actions via explicit buttons. */
  onRowClick?: (row: TData) => void;
  /** Override for the pagination footer's "total rows" count. Needed for
   * server-side (manualPagination/manualFiltering) tables, where the
   * table instance only ever holds one page's worth of data -
   * table.getFilteredRowModel().rows.length would just be that page's row
   * count, not the true across-all-pages total the server already
   * computed. Omit for client-side tables (derives it from the table's own
   * row model, same as before). */
  manualTotal?: number;
}

/**
 * Shared rendering shell for the client-side (TanStack-table-driven) pages -
 * Explore > Repositories and Explore > Merge Requests. Handles loading/error/
 * empty states, the fixed-width table + header + animated row body, and the
 * pagination footer; the page itself only builds columns, fetches data, and
 * constructs the `table` instance via useReactTable.
 */
export function ClientTable<TData>({
  table,
  columnWidths,
  loading,
  loadingLabel = 'Loading...',
  error,
  onRetry,
  isEmpty,
  empty,
  pageSizeOptions,
  onRowClick,
  manualTotal,
}: ClientTableProps<TData>) {
  if (loading && isEmpty) {
    return (
      <div className="flex items-center justify-center py-24">
        <div className="text-center">
          <Spinner size="lg" className="mb-4" />
          <p className="text-slate-300">{loadingLabel}</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-4 bg-red-900/50 border border-red-600 rounded-lg">
        <div className="flex items-center">
          <Icons.Error />
          <span className="ml-2 text-red-200">{error}</span>
          <Button variant="ghost" onClick={onRetry} className="ml-auto text-red-200 hover:text-white">
            Retry
          </Button>
        </div>
      </div>
    );
  }

  if (isEmpty) {
    return (
      <div className="text-center py-16">
        <Icons.EmptyState />
        <h3 className="text-xl font-medium text-slate-300 mt-4">{empty.title}</h3>
        {empty.description && <p className="text-slate-400 mt-2 mb-6">{empty.description}</p>}
        {empty.action}
      </div>
    );
  }

  const { pageIndex, pageSize } = table.getState().pagination;
  if (process.env.NODE_ENV !== 'production' && table.options.manualPagination && manualTotal === undefined) {
    // table.getFilteredRowModel().rows.length would silently fall back to
    // just the current page's row count (the only rows the table instance
    // ever holds in manual mode), not the true across-all-pages total the
    // server computed - loud in dev instead of a quietly wrong "Showing X
    // of Y" footer in production.
    console.warn('ClientTable: manualPagination is enabled but manualTotal was not provided - the pagination footer\'s total will be wrong.');
  }
  const filteredCount = manualTotal ?? table.getFilteredRowModel().rows.length;
  const totalPages = table.getPageCount();

  return (
    <div className="bg-[#1F2836] rounded-lg border border-slate-700">
      <table className="w-full table-fixed text-sm">
        <colgroup>
          {columnWidths.map((width, i) => (
            <col key={i} style={{ width }} />
          ))}
        </colgroup>
        <thead className="bg-[#2A3340]">
          {table.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id} className="divide-x divide-slate-600/60">
              {headerGroup.headers.map((header) => (
                <th key={header.id} className="align-top px-6 py-4 text-left">
                  {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody className="divide-y divide-slate-700">
          <AnimatePresence initial={false}>
            {table.getRowModel().rows.map((row) => (
              <motion.tr
                key={row.id}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                transition={{ duration: 0.15 }}
                className={`hover:bg-slate-700/30 transition-colors ${onRowClick ? 'cursor-pointer' : ''}`}
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
                onKeyDown={
                  onRowClick
                    ? (e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          onRowClick(row.original);
                        }
                      }
                    : undefined
                }
                tabIndex={onRowClick ? 0 : undefined}
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="px-6 py-2.5 align-middle min-h-[52px]">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </motion.tr>
            ))}
          </AnimatePresence>
        </tbody>
      </table>

      {totalPages > 1 && (
        <DataTablePaginationControls
          pagination={{
            page: pageIndex + 1,
            perPage: pageSize,
            total: filteredCount,
            totalPages,
            onPageChange: (page) => table.setPageIndex(page - 1),
            onPerPageChange: (perPage) => table.setPagination({ pageIndex: 0, pageSize: perPage }),
            perPageOptions: pageSizeOptions,
          }}
        />
      )}
    </div>
  );
}
