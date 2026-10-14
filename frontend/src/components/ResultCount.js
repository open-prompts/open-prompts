import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Accessible result count announcement. Shows "—" while unavailable.
 *
 * @param {Object} props
 * @param {number|null} props.count - total matching templates, null while loading/unavailable
 */
const ResultCount = ({ count }) => {
  const { t } = useTranslation();

  return (
    <div className="result-count" aria-live="polite">
      {count === null ? '—' : t('search.results_count', { count })}
    </div>
  );
};

export default ResultCount;
