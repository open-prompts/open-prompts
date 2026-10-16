import React from 'react';
import { useTranslation } from 'react-i18next';
import './RangeSelector.scss';

/**
 * RangeSelector — a segmented 7/30/90-day control (radiogroup pattern).
 *
 * @param {Object} props
 * @param {number} props.value - current range in days
 * @param {Function} props.onChange - (days) => void
 */
const RangeSelector = ({ value, onChange }) => {
  const { t } = useTranslation();
  const options = [7, 30, 90];

  return (
    <div className="range-selector" role="radiogroup" aria-label={t('analytics.title')}>
      {options.map((days) => (
        <button
          key={days}
          type="button"
          role="radio"
          aria-checked={value === days}
          className={`range-selector__option ${value === days ? 'active' : ''}`}
          onClick={() => onChange(days)}
        >
          {t(`analytics.range.${days}`)}
        </button>
      ))}
    </div>
  );
};

export default RangeSelector;