import React, { useEffect, useRef, useState } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Layout from '../components/Layout';
import KpiCard from '../components/analytics/KpiCard';
import DailyChart from '../components/analytics/DailyChart';
import SourceSplit from '../components/analytics/SourceSplit';
import TopTemplatesTable from '../components/analytics/TopTemplatesTable';
import RangeSelector from '../components/analytics/RangeSelector';
import { getAnalyticsOverview } from '../services/api';
import { usePageAnnouncement } from '../utils/a11y';
import './AnalyticsPage.scss';

const VALID_RANGES = [7, 30, 90];

/**
 * AnalyticsPage — the creator analytics dashboard at /analytics.
 * Auth-guarded; range mirrored in ?range=7|30|90.
 */
const AnalyticsPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token } = useSelector((state) => state.auth);
  const [searchParams, setSearchParams] = useSearchParams();

  usePageAnnouncement('analytics.title');

  const initialRange = Number(searchParams.get('range')) || 30;
  const [range, setRange] = useState(VALID_RANGES.includes(initialRange) ? initialRange : 30);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const requestIdRef = useRef(0);

  useEffect(() => {
    if (!token) {
      navigate('/login');
      return undefined;
    }
    const rid = ++requestIdRef.current;
    setLoading(true);
    setError(false);
    getAnalyticsOverview(range)
      .then((res) => {
        if (rid === requestIdRef.current) {
          setData(res.data);
          setLoading(false);
        }
      })
      .catch(() => {
        if (rid === requestIdRef.current) {
          setError(true);
          setLoading(false);
        }
      });
    return undefined;
  }, [range, token, navigate]);

  const handleRangeChange = (days) => {
    setRange(days);
    setSearchParams({ range: String(days) }, { replace: true });
  };

  const handleRetry = () => {
    setError(false);
    setLoading(true);
    const rid = ++requestIdRef.current;
    getAnalyticsOverview(range)
      .then((res) => {
        if (rid === requestIdRef.current) {
          setData(res.data);
          setLoading(false);
        }
      })
      .catch(() => {
        if (rid === requestIdRef.current) {
          setError(true);
          setLoading(false);
        }
      });
  };

  const empty = data && data.template_count === 0;

  return (
    <Layout showCreateButton={false}>
      <div className="analytics-page">
        <div className="analytics-page__header">
          <h1>{t('analytics.title')}</h1>
          <RangeSelector value={range} onChange={handleRangeChange} />
        </div>

        {error && (
          <div className="analytics-page__error" role="alert">
            <span>{t('analytics.error.fetch')}</span>
            <button type="button" onClick={handleRetry}>{t('analytics.error.retry')}</button>
          </div>
        )}

        {empty && (
          <div className="analytics-page__empty">
            <h2>{t('analytics.empty.title')}</h2>
            <p>{t('analytics.empty.body')}</p>
            <button type="button" onClick={() => navigate('/')}>{t('analytics.empty.cta')}</button>
          </div>
        )}

        {!empty && (
          <div className="analytics-page__kpis" aria-live="polite">
            {loading ? (
              <>
                <div className="kpi-skeleton" />
                <div className="kpi-skeleton" />
                <div className="kpi-skeleton" />
                <div className="kpi-skeleton" />
              </>
            ) : (
              <>
                <KpiCard label="analytics.kpi.views" icon="👁" value={data?.current?.views || 0} previous={data?.previous?.views || 0} tooltip="analytics.kpi_tooltip.views" />
                <KpiCard label="analytics.kpi.uses" icon="⚡" value={data?.current?.uses || 0} previous={data?.previous?.uses || 0} tooltip="analytics.kpi_tooltip.uses" />
                <KpiCard label="analytics.kpi.likes" icon="❤" value={data?.current?.likes || 0} previous={data?.previous?.likes || 0} tooltip="analytics.kpi_tooltip.likes" />
                <KpiCard label="analytics.kpi.favorites" icon="★" value={data?.current?.favorites || 0} previous={data?.previous?.favorites || 0} tooltip="analytics.kpi_tooltip.favorites" />
              </>
            )}
          </div>
        )}

        {!empty && !loading && !error && (
          <>
            <DailyChart series={data?.series || []} />
            <div className="analytics-page__grid">
              <SourceSplit web={data?.source_split?.web || 0} api={data?.source_split?.api || 0} />
              <TopTemplatesTable rows={data?.top_templates || []} />
            </div>
          </>
        )}
      </div>
    </Layout>
  );
};

export default AnalyticsPage;