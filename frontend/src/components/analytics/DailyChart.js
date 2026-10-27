import React, { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import './DailyChart.scss';

const METRICS = ['views', 'uses', 'likes', 'favorites'];

/**
 * DailyChart — a custom SVG line chart of the daily series with client-side
 * metric toggles and an accessible table view.
 *
 * @param {Object} props
 * @param {Array} props.series - [{ date, views, uses, likes, favorites }]
 */
const DailyChart = ({ series = [] }) => {
  const { t } = useTranslation();
  const [metric, setMetric] = useState('views');
  const [asTable, setAsTable] = useState(false);

  const { points, width, height } = useMemo(() => {
    const w = 640;
    const h = 220;
    const pad = 8;
    const values = series.map((p) => p[metric] || 0);
    const m = Math.max(1, ...values);
    const pts = series.map((p, i) => {
      const x = series.length > 1 ? pad + (i / (series.length - 1)) * (w - pad * 2) : w / 2;
      const y = h - pad - (p[metric] || 0) / m * (h - pad * 2);
      return { x, y, date: p.date, value: p[metric] || 0 };
    });
    return { points: pts, width: w, height: h };
  }, [series, metric]);

  const path = useMemo(() => {
    if (points.length === 0) return '';
    return points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  }, [points]);

  if (series.length === 0) {
    return <p className="daily-chart__empty">{t('analytics.chart.empty')}</p>;
  }

  return (
    <div className="daily-chart">
      <div className="daily-chart__header">
        <h3>{t('analytics.chart.title')}</h3>
        <div className="daily-chart__toggles" role="group" aria-label={t('analytics.chart.title')}>
          {METRICS.map((m) => (
            <button
              key={m}
              type="button"
              className={`daily-chart__toggle ${metric === m ? 'active' : ''}`}
              aria-pressed={metric === m}
              onClick={() => setMetric(m)}
            >
              {t(`analytics.chart.${m}`)}
            </button>
          ))}
        </div>
        <button type="button" className="daily-chart__table-toggle" onClick={() => setAsTable((s) => !s)}>
          {t('analytics.chart.view_as_table')}
        </button>
      </div>

      {asTable ? (
        <table className="daily-chart__table">
          <thead>
            <tr>
              <th scope="col">{t('analytics.chart.date')}</th>
              {METRICS.map((m) => (
                <th key={m} scope="col">{t(`analytics.chart.${m}`)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {series.map((p) => (
              <tr key={p.date}>
                <td>{p.date}</td>
                <td>{p.views}</td>
                <td>{p.uses}</td>
                <td>{p.likes}</td>
                <td>{p.favorites}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <svg
          className="daily-chart__svg"
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={`${t('analytics.chart.title')} — ${t(`analytics.chart.${metric}`)}`}
        >
          <line x1="8" y1={height - 8} x2={width - 8} y2={height - 8} className="daily-chart__axis" />
          <path d={path} fill="none" className="daily-chart__line" />
          {points.map((p) => (
            <circle key={p.date} cx={p.x} cy={p.y} r="2.5" className="daily-chart__dot">
              <title>{`${p.date}: ${p.value}`}</title>
            </circle>
          ))}
        </svg>
      )}
    </div>
  );
};

export default DailyChart;