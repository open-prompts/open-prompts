import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminStats } from '../../services/adminApi';
import KpiCard from '../../components/admin/KpiCard';
import Badge from '../../components/admin/Badge';
import ErrorBanner from '../../components/admin/ErrorBanner';
import EmptyState from '../../components/admin/EmptyState';
import { formatRelativeTime, formatNumber } from '../../utils/adminFormat';
import './DashboardPage.scss';

/**
 * Admin dashboard: KPI cards plus recent signups and recent public templates.
 */
const DashboardPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [stats, setStats] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const resp = await getAdminStats();
      setStats(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const lang = i18n.language || 'en';

  if (loading) {
    return (
      <div>
        <h1 className="admin-page-title">{t('admin.dashboard.title')}</h1>
        <div className="admin-dashboard__grid">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="admin-dashboard__kpi-skeleton" />
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <h1 className="admin-page-title">{t('admin.dashboard.title')}</h1>
        <ErrorBanner message={error} onRetry={load} />
      </div>
    );
  }

  const kpis = [
    { label: t('admin.dashboard.total_users'), value: formatNumber(stats.total_users, lang), to: '/admin/users' },
    { label: t('admin.dashboard.active_users'), value: formatNumber(stats.active_users, lang), to: '/admin/users?status=active' },
    { label: t('admin.dashboard.banned_users'), value: formatNumber(stats.banned_users, lang), to: '/admin/users?status=banned' },
    { label: t('admin.dashboard.total_templates'), value: formatNumber(stats.total_templates, lang), to: '/admin/templates' },
    { label: t('admin.dashboard.public_templates'), value: formatNumber(stats.public_templates, lang), to: '/admin/templates?visibility=public' },
    { label: t('admin.dashboard.prompts_created'), value: formatNumber(stats.prompts_created, lang) },
  ];

  return (
    <div>
      <h1 className="admin-page-title">{t('admin.dashboard.title')}</h1>
      <div className="admin-dashboard__grid">
        {kpis.map((kpi) => (
          <KpiCard
            key={kpi.label}
            label={kpi.label}
            value={kpi.value}
            onClick={kpi.to ? () => navigate(kpi.to) : undefined}
          />
        ))}
      </div>

      <div className="admin-dashboard__lists">
        <section className="admin-dashboard__panel">
          <h2>{t('admin.dashboard.recent_signups')}</h2>
          {(!stats.recent_signups || stats.recent_signups.length === 0) && (
            <EmptyState title={t('admin.dashboard.no_signups')} />
          )}
          <ul className="admin-dashboard__list">
            {(stats.recent_signups || []).map((u) => (
              <li key={u.id}>
                <button
                  className="admin-dashboard__list-item"
                  onClick={() => navigate(`/admin/users/${u.id}`)}
                >
                  <span className="admin-dashboard__list-name">
                    {u.display_name || u.email}
                    <small>{u.email}</small>
                  </span>
                  <span className="admin-dashboard__list-meta">
                    <Badge tone={u.status === 'banned' ? 'danger' : 'success'} dot>
                      {u.status === 'banned'
                        ? t('admin.users.banned')
                        : t('admin.users.active')}
                    </Badge>
                    <span className="admin-dashboard__time">
                      {formatRelativeTime(u.created_at, lang)}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>

        <section className="admin-dashboard__panel">
          <h2>{t('admin.dashboard.recent_templates')}</h2>
          {(!stats.recent_templates || stats.recent_templates.length === 0) && (
            <EmptyState title={t('admin.dashboard.no_templates')} />
          )}
          <ul className="admin-dashboard__list">
            {(stats.recent_templates || []).map((tpl) => (
              <li key={tpl.id}>
                <button
                  className="admin-dashboard__list-item"
                  onClick={() => navigate(`/admin/templates/${tpl.id}`)}
                >
                  <span className="admin-dashboard__list-name">
                    {tpl.title}
                    <small>{tpl.owner_display_name || tpl.owner_id}</small>
                  </span>
                  <span className="admin-dashboard__time">
                    {formatRelativeTime(tpl.updated_at, lang)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
};

export default DashboardPage;
