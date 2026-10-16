import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getTemplateStats } from '../../services/api';
import { formatCount } from '../../utils/formatCount';
import Sparkline from './Sparkline';
import './OwnerStatsSection.scss';

/**
 * OwnerStatsSection — owner-only analytics block on the template detail page.
 *
 * @param {Object} props
 * @param {string} props.templateId
 * @param {string} [props.locale]
 */
const OwnerStatsSection = ({ templateId, locale }) => {
  const { t } = useTranslation();
  const [data, setData] = useState(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setError(false);
    getTemplateStats(templateId, 30)
      .then((res) => {
        if (!cancelled) setData(res.data);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });
    return () => {
      cancelled = true;
    };
  }, [templateId]);

  if (error) {
    return <p className="owner-stats__error">{t('analytics.error.fetch')}</p>;
  }
  if (!data) {
    return <div className="owner-stats owner-stats--loading">{t('common.loading')}</div>;
  }

  const rate = data.view_to_use_rate != null ? `${(data.view_to_use_rate * 100).toFixed(1)}%` : '—';

  return (
    <div className="owner-stats">
      <h4>{t('analytics.owner_stats.title')}</h4>
      <div className="owner-stats__row">
        <span className="owner-stats__metric">
          <span className="owner-stats__value">{formatCount(data.current?.views || 0, locale)}</span>
          <span className="owner-stats__label">{t('analytics.public.views')}</span>
        </span>
        <span className="owner-stats__metric">
          <span className="owner-stats__value">{formatCount(data.current?.uses || 0, locale)}</span>
          <span className="owner-stats__label">{t('analytics.public.uses')}</span>
        </span>
        <span className="owner-stats__metric">
          <span className="owner-stats__value">{rate}</span>
          <span className="owner-stats__label">{t('analytics.owner_stats.rate')}</span>
        </span>
      </div>
      <Sparkline series={data.series || []} />
    </div>
  );
};

export default OwnerStatsSection;