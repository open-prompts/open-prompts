import React from 'react';
import './SkeletonCards.scss';

/**
 * Shimmer placeholder cards shown during the first page load.
 * @param {Object} props
 * @param {number} props.count - number of skeleton cards (default 8)
 */
const SkeletonCards = ({ count = 8 }) => (
  <div className="skeleton-cards" aria-hidden="true">
    {Array.from({ length: count }, (_, i) => (
      <div className="skeleton-card" key={i}>
        <div className="skeleton-line title" />
        <div className="skeleton-line" />
        <div className="skeleton-line short" />
        <div className="skeleton-tags">
          <div className="skeleton-tag" />
          <div className="skeleton-tag" />
        </div>
      </div>
    ))}
  </div>
);

export default SkeletonCards;
