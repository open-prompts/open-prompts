import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * FilterChips — an accessible radiogroup of client-side filter options.
 * @param {Object} props
 * @param {Array} props.options - [{value, labelKey}]
 * @param {string} props.value - active option value
 * @param {Function} props.onChange - (value) => void
 */
const FilterChips = ({ options, value, onChange }) => {
  const { t } = useTranslation();
  return (
    <div className="filter-chips" role="radiogroup" aria-label={t('dashboard_v2.templates.filter.all')}>
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="radio"
          aria-checked={value === opt.value}
          className={`filter-chip ${value === opt.value ? 'active' : ''}`}
          onClick={() => onChange(opt.value)}
        >
          {t(opt.labelKey)}
        </button>
      ))}
    </div>
  );
};

export default FilterChips;