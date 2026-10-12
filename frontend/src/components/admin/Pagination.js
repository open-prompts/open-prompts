import React from 'react';
import { useTranslation } from 'react-i18next';
import './Pagination.scss';

/**
 * Page-based pagination controls (page X of N, prev/next).
 */
const Pagination = ({ page, total, perPage, onPageChange }) => {
  const { t } = useTranslation();
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const from = total === 0 ? 0 : (page - 1) * perPage + 1;
  const to = Math.min(page * perPage, total);

  return (
    <div className="admin-pagination">
      <span className="admin-pagination__info">
        {t('admin.common.total_count', { count: total })} · {from}–{to}
      </span>
      <div className="admin-pagination__controls">
        <button
          className="admin-btn admin-btn--ghost"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t('admin.common.prev')}
        </button>
        <span className="admin-pagination__page">
          {t('admin.common.page', { page, total: totalPages })}
        </span>
        <button
          className="admin-btn admin-btn--ghost"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t('admin.common.next')}
        </button>
      </div>
    </div>
  );
};

export default Pagination;
