import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './HideWarnedFilterChip.scss';

/**
 * HideWarnedFilterChip — toggle chip in the browse/search filter row that
 * sets exclude_warned=true. State is controlled by the parent (persisted per
 * user + URL-synced).
 *
 * @param {Object} props
 * @param {boolean} props.active - whether the filter is on
 * @param {Function} props.onToggle - () => void
 */
const HideWarnedFilterChip = ({ active = false, onToggle }) => {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className={`hide-warned-chip ${active ? 'is-active' : ''}`}
      onClick={onToggle}
      aria-pressed={active}
      title={t('warnings.filter.hideWarned')}
    >
      <AlertIcon size={14} className="hide-warned-chip__icon" />
      <span>{t('warnings.filter.hideWarned')}</span>
    </button>
  );
};

export default HideWarnedFilterChip;