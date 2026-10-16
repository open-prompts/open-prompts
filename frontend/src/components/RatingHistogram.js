import React from 'react';
import { useTranslation } from 'react-i18next';
import { StarIcon } from './ui/Icons';
import './RatingHistogram.scss';

/**
 * RatingHistogram — 5-row distribution (star label, bar, percentage).
 * Semantic <ul>; bars are decorative (aria-hidden). Hidden at zero ratings.
 *
 * @param {Object} props
 * @param {number[]} props.distribution - 5 counts, index 0 -> 1★ ... 4 -> 5★
 */
const RatingHistogram = ({ distribution = [] }) => {
  const { t } = useTranslation();
  const counts = Array.isArray(distribution) && distribution.length === 5 ? distribution : [0, 0, 0, 0, 0];
  const total = counts.reduce((a, b) => a + b, 0);
  if (!total) return null;

  return (
    <ul className="rating-histogram" aria-label={t('ratings.histogram')}>
      {counts.map((count, i) => {
        const stars = i + 1;
        const pct = Math.round((count / total) * 100);
        return (
          <li key={stars} className="rating-histogram__row">
            <span className="rating-histogram__label">
              <StarIcon size={13} filled className="rating-histogram__star" />
              {stars}
            </span>
            <span className="rating-histogram__bar-track" aria-hidden="true">
              <span
                className="rating-histogram__bar"
                style={{ width: `${pct}%` }}
              />
            </span>
            <span className="rating-histogram__pct">{pct}%</span>
          </li>
        );
      })}
    </ul>
  );
};

export default RatingHistogram;