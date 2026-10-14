import React from 'react';
import { useTranslation } from 'react-i18next';
import './EmptyState.scss';

/**
 * Empty state for search results. Echoes the keyword and offers to clear filters.
 *
 * @param {Object} props
 * @param {string} props.keyword - the active search keyword ("" for filter-only emptiness)
 * @param {Function} props.onClearAll
 */
const EmptyState = ({ keyword, onClearAll }) => {
  const { t } = useTranslation();

  return (
    <div className="empty-state">
      <h3 className="empty-state-title">
        {keyword
          ? t('search.empty.title', { keyword })
          : t('home.no_results')}
      </h3>
      <p className="empty-state-body">{t('search.empty.body')}</p>
      <button type="button" className="empty-state-clear" onClick={onClearAll}>
        {t('search.empty.clear_all')}
      </button>
    </div>
  );
};

export default EmptyState;
