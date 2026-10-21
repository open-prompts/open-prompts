import React, { useEffect, useRef, useState } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import StatCard from '../../components/dashboard/StatCard';
import ActivityFeed from '../../components/dashboard/ActivityFeed';
import QuickActions from '../../components/dashboard/QuickActions';
import CreateTemplateModal from '../../components/CreateTemplateModal';
import { getDashboardOverview } from '../../services/api';
import { hasCollections } from '../../utils/collections';
import { usePageAnnouncement } from '../../utils/a11y';
import './OverviewPage.scss';

/**
 * OverviewPage — the dashboard landing: stat cards, recent activity,
 * and quick actions.
 */
const OverviewPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token } = useSelector((state) => state.auth);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [collectionsAvailable, setCollectionsAvailable] = useState(true);
  const [createOpen, setCreateOpen] = useState(false);
  const requestIdRef = useRef(0);

  usePageAnnouncement('dashboard_v2.title');

  useEffect(() => {
    if (!token) {
      navigate('/login?next=/dashboard');
      return undefined;
    }
    const requestId = ++requestIdRef.current;
    setLoading(true);
    setError(false);
    getDashboardOverview()
      .then((res) => {
        if (requestId !== requestIdRef.current) return;
        setData(res.data);
        setLoading(false);
      })
      .catch(() => {
        if (requestId !== requestIdRef.current) return;
        setError(true);
        setLoading(false);
      });
    return undefined;
  }, [token, navigate]);

  useEffect(() => {
    let mounted = true;
    hasCollections().then((ok) => {
      if (mounted) setCollectionsAvailable(ok);
    });
    return () => {
      mounted = false;
    };
  }, []);

  if (loading) {
    return <div className="dashboard-loading" role="status">{t('common.loading')}</div>;
  }

  if (error) {
    return (
      <div className="dashboard-error" role="alert">
        <p>{t('dashboard_v2.error.fetch')}</p>
        <button type="button" className="btn btn-primary" onClick={() => window.location.reload()}>
          {t('dashboard_v2.error.retry')}
        </button>
      </div>
    );
  }

  return (
    <div className="overview-page">
      <h1 className="overview-page__title">{t('dashboard_v2.title')}</h1>
      <div className="overview-page__stats">
        <StatCard to="/dashboard/templates" label="dashboard_v2.stats.templates" count={data?.template_count ?? 0} />
        <StatCard to="/dashboard/prompts" label="dashboard_v2.stats.prompts" count={data?.prompt_count ?? 0} />
        <StatCard
          to="/collections/mine"
          label="dashboard_v2.stats.collections"
          count={data?.collection_count ?? 0}
          hidden={!collectionsAvailable}
        />
        <StatCard to="/api-keys" label="dashboard_v2.stats.api_keys" count={data?.api_key_count ?? 0} />
      </div>
      <section className="overview-page__activity" aria-labelledby="activity-title">
        <h2 id="activity-title">{t('dashboard_v2.activity.title')}</h2>
        <ActivityFeed items={data?.recent_activity} />
      </section>
      <QuickActions onNewTemplate={() => setCreateOpen(true)} />
      <CreateTemplateModal
        open={createOpen}
        onRequestClose={() => setCreateOpen(false)}
        onSuccess={() => {
          setCreateOpen(false);
          window.location.reload();
        }}
      />
    </div>
  );
};

export default OverviewPage;