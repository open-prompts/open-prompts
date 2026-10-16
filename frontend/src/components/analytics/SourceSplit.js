import React from 'react';
import { useTranslation } from 'react-i18next';
import './SourceSplit.scss';

/**
 * SourceSplit — a web/API use split bar.
 *
 * @param {Object} props
 * @param {number} props.web
 * @param {number} props.api
 */
const SourceSplit = ({ web = 0, api = 0 }) => {
  const { t } = useTranslation();
  const total = web + api;
  if (total === 0) {
    return (
      <div className="source-split">
        <h3>{t('analytics.source.title')}</h3>
        <p className="source-split__empty">{t('analytics.source.empty')}</p>
      </div>
    );
  }
  const webPct = (web / total) * 100;
  const apiPct = (api / total) * 100;
  return (
    <div className="source-split">
      <h3>{t('analytics.source.title')}</h3>
      <div className="source-split__bar" role="img" aria-label={`${t('analytics.source.web')} ${web}, ${t('analytics.source.api')} ${api}`}>
        <div className="source-split__web" style={{ width: `${webPct}%` }} />
        <div className="source-split__api" style={{ width: `${apiPct}%` }} />
      </div>
      <div className="source-split__legend">
        <span className="source-split__legend-item"><span className="source-split__swatch source-split__swatch--web" />{t('analytics.source.web')} {web}</span>
        <span className="source-split__legend-item"><span className="source-split__swatch source-split__swatch--api" />{t('analytics.source.api')} {api}</span>
      </div>
    </div>
  );
};

export default SourceSplit;