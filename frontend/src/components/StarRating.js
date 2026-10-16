import React from 'react';
import { useTranslation } from 'react-i18next';
import { StarIcon } from './ui/Icons';
import './StarRating.scss';

/**
 * StarRating — read-only star display with value + count.
 *
 * @param {Object} props
 * @param {number} props.ratingAvg - average rating (0 when unrated)
 * @param {number} props.ratingCount - number of ratings
 * @param {string} [props.size] - "sm" (cards) or "lg" (detail/admin)
 * @param {string} [props.className] - extra class
 */
const StarRating = ({ ratingAvg = 0, ratingCount = 0, size = 'sm', className = '' }) => {
  const { t } = useTranslation();
  if (!ratingCount) return null;

  const rounded = Math.round(ratingAvg * 10) / 10;
  const label = t('ratings.aria_label', { avg: rounded, count: ratingCount });

  return (
    <div
      className={`star-rating star-rating--${size} ${className}`}
      role="img"
      aria-label={label}
    >
      <span className="star-rating__stars" aria-hidden="true">
        {[1, 2, 3, 4, 5].map((i) => (
          <StarIcon
            key={i}
            size={size === 'lg' ? 16 : 13}
            filled={i <= Math.round(ratingAvg)}
            className="star-rating__star"
          />
        ))}
      </span>
      <span className="star-rating__value">{rounded.toFixed(1)}</span>
      <span className="star-rating__count">
        {t('ratings.ratings_count', { count: ratingCount })}
      </span>
    </div>
  );
};

export default StarRating;