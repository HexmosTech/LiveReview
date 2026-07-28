import React, { ReactNode } from 'react';
import { ColumnDef, flexRender, getCoreRowModel, useReactTable } from '@tanstack/react-table';
import { Button, Icons, Spinner } from '../UIPrimitives';

export interface DataTablePagination {
  page: number;
  perPage: number;
  total: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  onPerPageChange?: (perPage: number) => void;
  perPageOptions?: number[];
}

export interface DataTableEmptyState {
  title: string;
  description?: string;
  action?: ReactNode;
}

export interface DataTableProps<T> {
  data: T[];
  columns: ColumnDef<T, any>[];
  getRowId: (row: T) => string | number;
  loading?: boolean;
  loadingLabel?: string;
  error?: string | null;
  onRetry?: () => void;
  empty?: DataTableEmptyState;
  onRowClick?: (row: T) => void;
  pagination?: DataTablePagination;
  /** Column resizing (drag column edges). Off by default. */
  enableColumnResizing?: boolean;
}

/**
 * A reusable, server-driven data table: pagination, filtering, and sorting
 * are all expected to be handled by the caller (via query params / API
 * calls) and passed in as already-fetched `data` plus a `pagination`
 * descriptor - this component only renders. Built on @tanstack/react-table
 * (already used by TaxonomyReports.tsx) so column definitions, cell
 * renderers, and optional resizing all come from the table library rather
 * than being hand-rolled per page.
 */
export function DataTable<T>({
  data,
  columns,
  getRowId,
  loading = false,
  loadingLabel = 'Loading...',
  error = null,
  onRetry,
  empty,
  onRowClick,
  pagination,
  enableColumnResizing = false,
}: DataTableProps<T>) {
  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
    columnResizeMode: enableColumnResizing ? 'onChange' : undefined,
    enableColumnResizing,
  });

  if (loading && data.length === 0) {
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
          {onRetry && (
            <Button variant="ghost" onClick={onRetry} className="ml-auto text-red-200 hover:text-white">
              Retry
            </Button>
          )}
        </div>
      </div>
    );
  }

  if (data.length === 0 && empty) {
    return (
      <div className="text-center py-16">
        <Icons.EmptyState />
        <h3 className="text-xl font-medium text-slate-300 mt-4">{empty.title}</h3>
        {empty.description && <p className="text-slate-400 mt-2 mb-6">{empty.description}</p>}
        {empty.action}
      </div>
    );
  }

  return (
    <div className="bg-slate-800 rounded-lg overflow-hidden border border-slate-700">
      <div className="overflow-x-auto">
        <table className="w-full" style={enableColumnResizing ? { width: table.getTotalSize() } : undefined}>
          <thead className="bg-slate-700">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    className="relative px-6 py-4 text-left text-sm font-semibold text-slate-200 uppercase tracking-wider whitespace-nowrap"
                    style={enableColumnResizing ? { width: header.getSize() } : undefined}
                  >
                    {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                    {enableColumnResizing && header.column.getCanResize() && (
                      <div
                        onMouseDown={header.getResizeHandler()}
                        onTouchStart={header.getResizeHandler()}
                        className="absolute right-0 top-0 h-full w-1 cursor-col-resize select-none touch-none bg-slate-600/0 hover:bg-blue-500/60"
                      />
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody className="divide-y divide-slate-700">
            {table.getRowModel().rows.map((row) => (
              <tr
                key={getRowId(row.original)}
                className={
                  onRowClick
                    ? 'group hover:bg-slate-700/50 transition-colors cursor-pointer focus-within:bg-slate-700/50'
                    : 'hover:bg-slate-700/30 transition-colors'
                }
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
                  <td key={cell.id} className="px-6 py-4 align-top">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {pagination && pagination.totalPages > 1 && (
        <DataTablePaginationControls pagination={pagination} />
      )}
    </div>
  );
}

const DataTablePaginationControls: React.FC<{ pagination: DataTablePagination }> = ({ pagination }) => {
  const { page, perPage, total, totalPages, onPageChange, onPerPageChange, perPageOptions } = pagination;

  const pageButtons = Array.from({ length: Math.min(totalPages, 5) }, (_, i) => {
    let pageNum: number;
    if (totalPages <= 5) {
      pageNum = i + 1;
    } else if (page <= 3) {
      pageNum = i + 1;
    } else if (page >= totalPages - 2) {
      pageNum = totalPages - 4 + i;
    } else {
      pageNum = page - 2 + i;
    }
    return pageNum;
  });

  return (
    <div className="px-6 py-4 border-t border-slate-700">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div className="text-sm text-slate-300">
          Showing {total === 0 ? 0 : (page - 1) * perPage + 1} to {Math.min(page * perPage, total)} of {total}
        </div>
        <div className="flex items-center gap-3">
          {onPerPageChange && perPageOptions && (
            <select
              value={perPage}
              onChange={(e) => onPerPageChange(parseInt(e.target.value, 10))}
              className="px-2 py-1.5 bg-slate-700 border border-slate-600 rounded-md text-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            >
              {perPageOptions.map((option) => (
                <option key={option} value={option}>
                  {option} / page
                </option>
              ))}
            </select>
          )}
          <div className="flex items-center space-x-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onPageChange(page - 1)}
              disabled={page <= 1}
              className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Previous
            </Button>
            <div className="flex items-center space-x-1">
              {pageButtons.map((pageNum) => (
                <Button
                  key={pageNum}
                  variant={pageNum === page ? 'primary' : 'outline'}
                  size="sm"
                  onClick={() => onPageChange(pageNum)}
                  className={
                    pageNum === page
                      ? 'bg-blue-600 text-white'
                      : 'border-slate-600 text-slate-300 hover:text-white hover:border-slate-500'
                  }
                >
                  {pageNum}
                </Button>
              ))}
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onPageChange(page + 1)}
              disabled={page >= totalPages}
              className="border-slate-600 text-slate-300 hover:text-white hover:border-slate-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Next
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default DataTable;
