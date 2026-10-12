import React from 'react';
import './Badge.scss';

/**
 * Small status badge. Tone maps to a color; an optional icon/dot keeps the
 * signal accessible (color is never the only cue).
 */
const Badge = ({ tone = 'neutral', dot = false, children }) => {
  const tones = ['accent', 'success', 'danger', 'warning', 'neutral'];
  const safeTone = tones.includes(tone) ? tone : 'neutral';
  return (
    <span className={`admin-badge admin-badge--${safeTone}`}>
      {dot && <span className="admin-badge__dot" aria-hidden="true" />}
      {children}
    </span>
  );
};

export default Badge;
