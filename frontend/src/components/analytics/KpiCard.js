import React from 'react';
import { useTranslation } from 'react-i18next';
import './KpiCard.scss';

/**
 * KpiCard — a single metric card with an icon, label, value, and a delta
 * badge comparing the current range to the previous one.
 *
 * @param {Object} props
 * @param {string} props.label - i18n key for the metric label
 * @param {React.ReactNode} props.icon
 * @param {number} props.value - current range total
 * @param {number} props.previous - previous range total
 * @param {string} [props.tooltip] - i18n key for the tooltip text
 */
const KpiCard = ({ label, icon, value, previous = 0, tooltip }) => {
  const { t } = useTranslation();

  let delta = null;
  if (previous === 0 && value > 0) {
    delta = { kind: 'up', text: t('analytics.delta.up', { value: '∞' }) };
  } else if (previous === value) {
    delta = { kind: 'flat', text: t('analytics.delta.flat') };
  } else if (previous > 0) {
    const pct = ((value - previous) / previous) * 100;
    const rounded = Math.round(pct * 10) / 10;
    delta = {
      kind: pct >= 0 ? 'up' : 'down',
      text: t(pct >= 0 ? 'analytics.delta.up' : 'analytics.delta.down', { value: Math.abs(rounded) }),
    };
  }

  return (
    <div className="kpi-card" title={tooltip ? t(tooltip) : undefined}>
      <div className="kpi-card__icon" aria-hidden="true">{icon}</div>
      <div className="kpi-card__body">
        <span className="kpi-card__label">{t(label)}</span>
        <span className="kpi-card__value">{value}</span>
        {delta && (
          <span className={`kpi-card__delta kpi-card__delta--${delta.kind}`}>{delta.text}</span>
        )}
      </div>
    </div>
  );
};

export default KpiCard;