import React from 'react';
import { useTranslation } from 'react-i18next';
import { formatCount } from '../../utils/formatCount';
import './PublicCounters.scss';

/**
 * PublicCounters — view/use counters shown to all visitors on the template
 * detail page.
 *
 * @param {Object} props
 * @param {number} props.views
 * @param {number} props.uses
 * @param {string} [props.locale]
 */
const PublicCounters = ({ views = 0, uses = 0, locale }) => {
  const { t } = useTranslation();
  return (
    <div className="public-counters">
      <span className="public-counters__item">
        <span className="public-counters__icon" aria-hidden="true">👁</span>
        <span className="public-counters__count">{formatCount(views, locale)}</span>
        <span className="public-counters__label">{t('analytics.public.views')}</span>
      </span>
      <span className="public-counters__item">
        <span className="public-counters__icon" aria-hidden="true">⚡</span>
        <span className="public-counters__count">{formatCount(uses, locale)}</span>
        <span className="public-counters__label">{t('analytics.public.uses')}</span>
      </span>
    </div>
  );
};

export default PublicCounters;