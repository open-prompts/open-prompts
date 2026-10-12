import React from 'react';
import './AdminTable.scss';

/**
 * Generic admin table with loading skeleton rows, sticky header and
 * optional row click navigation.
 */
const AdminTable = ({ columns, rows, loading = false, onRowClick, rowKey, emptyState }) => {
  if (!loading && rows.length === 0 && emptyState) {
    return <>{emptyState}</>;
  }

  return (
    <div className="admin-table-wrap">
      <table className="admin-table">
        <thead>
          <tr>
            {columns.map((col) => (
              <th key={col.key} scope="col" style={col.width ? { width: col.width } : undefined}>
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading &&
            Array.from({ length: 6 }).map((_, i) => (
              <tr key={`skeleton-${i}`} className="admin-table__skeleton-row">
                {columns.map((col) => (
                  <td key={col.key}>
                    <span className="admin-table__skeleton" />
                  </td>
                ))}
              </tr>
            ))}
          {!loading &&
            rows.map((row) => (
              <tr
                key={rowKey(row)}
                className={onRowClick ? 'admin-table__clickable-row' : undefined}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
              >
                {columns.map((col) => (
                  <td key={col.key}>{col.render ? col.render(row) : row[col.key]}</td>
                ))}
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  );
};

export default AdminTable;
