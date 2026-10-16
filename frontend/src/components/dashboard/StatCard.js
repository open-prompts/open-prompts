import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

/**
 * StatCard — a whole-card link showing a workspace count.
 * @param {Object} props
 * @param {string} props.to - destination route
 * @param {string} props.label - i18n label key
 * @param {number} props.count - the count to display
 * @param {boolean} props.hidden - when true, render nothing (e.g. collections absent)
 */
const StatCard = ({ to, label, count, hidden = false }) => {
  const { t } = useTranslation();
  if (hidden) return null;
  return (
    <Link
      to={to}
      className="stat-card"
      aria-label={t('dashboard_v2.stats.aria', { label: t(label), count })}
    >
      <span className="stat-card__label">{t(label)}</span>
      <span className="stat-card__count">{count}</span>
    </Link>
  );
};

export default StatCard;