import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './WarningChip.scss';

/**
 * WarningChip — a small chip with a ⚠ icon and the localized reason name.
 * When multiple warnings are present, it shows the first + a "+N more" suffix
 * and lists all reasons in a tooltip.
 *
 * @param {Object} props
 * @param {string[]} props.warnings - taxonomy keys (mature|violence|profanity|sensitive)
 * @param {string} [props.className] - extra class for the chip
 */
const WarningChip = ({ warnings = [], className = '' }) => {
  const { t } = useTranslation();
  if (!warnings || warnings.length === 0) return null;

  const names = warnings.map((w) => t(`warnings.type.${w}`, { defaultValue: w }));
  const first = names[0];
  const label =
    names.length === 1
      ? first
      : t('warnings.chip.multiple', { first, count: names.length - 1 });

  return (
    <span
      className={`warning-chip ${className}`}
      title={names.join(', ')}
      aria-label={`${t('warnings.interstitial.title')}: ${names.join(', ')}`}
    >
      <AlertIcon size={12} className="warning-chip__icon" />
      <span className="warning-chip__label">{label}</span>
    </span>
  );
};

export default WarningChip;