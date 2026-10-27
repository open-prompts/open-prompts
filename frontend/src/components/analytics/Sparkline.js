import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Sparkline — a small 30-day SVG line for the owner stats section.
 *
 * @param {Object} props
 * @param {Array} props.series - [{ date, views, uses }]
 */
const Sparkline = ({ series = [] }) => {
  const { t } = useTranslation();
  const { path, width, height, pts } = useMemo(() => {
    const w = 120;
    const h = 32;
    const pad = 2;
    const values = series.map((p) => (p.views || 0) + (p.uses || 0));
    const m = Math.max(1, ...values);
    const pts = series.map((p, i) => {
      const x = series.length > 1 ? pad + (i / (series.length - 1)) * (w - pad * 2) : w / 2;
      const y = h - pad - ((p.views || 0) + (p.uses || 0)) / m * (h - pad * 2);
      return { x, y };
    });
    const p = pts.map((pt, i) => `${i === 0 ? 'M' : 'L'}${pt.x.toFixed(1)},${pt.y.toFixed(1)}`).join(' ');
    return { path: p, width: w, height: h, pts };
  }, [series]);

  const peak = useMemo(() => {
    if (series.length === 0) return 0;
    return Math.max(...series.map((p) => (p.views || 0) + (p.uses || 0)));
  }, [series]);

  return (
    <svg
      className="sparkline"
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={t('analytics.owner_stats.sparkline', { peak })}
    >
      {pts.length > 1 && <path d={path} fill="none" className="sparkline__line" />}
    </svg>
  );
};

export default Sparkline;