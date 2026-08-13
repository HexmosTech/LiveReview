import React, { ReactNode, ThHTMLAttributes, TdHTMLAttributes, HTMLAttributes } from 'react';
import classNames from 'classnames';

// Lightweight, non-sortable table shell matching the header/row styling used by the
// TanStack-based ClientTable (components/DataTable/ClientTable.tsx, used on Reviews
// and Explore > Repositories). Use this for simple tables that don't need sorting or
// column filters — e.g. UserManagement/UserList.tsx and the bulk-invite review table
// in UserManagement/UserForm.tsx. If a table later needs sorting/filtering, migrate
// it to ClientTable instead of adding that machinery here.

export const Table: React.FC<{ children: ReactNode; className?: string; style?: React.CSSProperties }> = ({
  children,
  className,
  style,
}) => (
  <table className={classNames('w-full text-sm', className)} style={style}>
    {children}
  </table>
);

export const TableHead: React.FC<{ children: ReactNode }> = ({ children }) => (
  <thead className="bg-[#2A3340]">
    <tr className="divide-x divide-slate-600/60">{children}</tr>
  </thead>
);

export const TableHeaderCell: React.FC<
  ThHTMLAttributes<HTMLTableCellElement> & { children?: ReactNode }
> = ({ children, className, ...props }) => (
  <th
    className={classNames(
      'px-6 py-4 align-top text-left text-xs font-semibold uppercase tracking-wide text-slate-300 whitespace-nowrap',
      className
    )}
    {...props}
  >
    {children}
  </th>
);

export const TableBody: React.FC<{ children: ReactNode }> = ({ children }) => (
  <tbody className="divide-y divide-slate-700">{children}</tbody>
);

export const TableRow: React.FC<HTMLAttributes<HTMLTableRowElement>> = ({ children, className, ...props }) => (
  <tr className={classNames('hover:bg-slate-700/30 transition-colors', className)} {...props}>
    {children}
  </tr>
);

export const TableCell: React.FC<
  TdHTMLAttributes<HTMLTableCellElement> & { children?: ReactNode }
> = ({ children, className, ...props }) => (
  <td className={classNames('px-6 py-2.5 align-middle', className)} {...props}>
    {children}
  </td>
);
