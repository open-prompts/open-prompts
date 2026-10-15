import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getReviewQueue, getReviewMetrics } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import AdminSelect from '../../components/admin/AdminSelect';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import { formatRelativeTime } from '../../utils/adminFormat';
import './ReviewPage.scss';

/**
 * Admin pre-publication review queue: metrics bar, aging/resubmission
 * filter tabs, category filter, oldest-first table.
 */
const ReviewPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const tab = searchParams.get('tab') || 'all'; // all | aging | resubmissions
  const category = searchParams.get('category') || '';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = 20;

  const [data, setData] = useState({ items: [], total: 0 });
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const setParam = (updates, resetPage = true) => {
    const next = new URLSearchParams(searchParams);
    Object.entries(updates).forEach(([k, v]) => {
      if (v === '' || v === null || v === undefined) next.delete(k);
      else next.set(k, String(v));
    });
    if (resetPage) next.delete('page');
    setSearchParams(next);
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const params = { page, per_page: perPage };
      if (tab === 'aging') params.aging = 'true';
      if (tab === 'resubmissions') params.resubmission_only = 'true';
      if (category) params.category = category;
      const resp = await getReviewQueue(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [tab, category, page]);

  const loadMetrics = useCallback(async () => {
    try {
      const resp = await getReviewMetrics();
      setMetrics(resp.data);
    } catch (err) {
      // Metrics are non-critical; the queue still renders.
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    loadMetrics();
  }, [loadMetrics]);

  const lang = i18n.language || 'en';

  const formatHours = (h) => {
    const hours = Number(h) || 0;
    if (hours < 1) return t('review.queue.minutes', { count: Math.max(1, Math.round(hours * 60)) });
    if (hours < 48) return t('review.queue.hours', { count: Math.round(hours) });
    return t('review.queue.days', { count: Math.round(hours / 24) });
  };

  const columns = [
    {
      key: 'age',
      header: t('review.queue.col_age'),
      render: (item) =>
        item.aging ? (
          <Badge tone="warning" title={t('review.queue.aging')}>
            ⏳ {formatRelativeTime(item.submitted_at, lang)}
          </Badge>
        ) : (
          <span className="review-queue__age">{formatRelativeTime(item.submitted_at, lang)}</span>
        ),
    },
    {
      key: 'title',
      header: t('review.queue.col_template'),
      render: (item) => (
        <span className="review-queue__title">
          {item.template_title}
          {item.is_resubmission && (
            <span className="review-queue__attempt-chip">
              {t('review.row.resubmission', { n: item.attempt_number })}
            </span>
          )}
        </span>
      ),
    },
    {
      key: 'creator',
      header: t('review.queue.col_creator'),
      render: (item) => (
        <button
          className="review-queue__creator-link"
          onClick={(e) => {
            e.stopPropagation();
            navigate(`/admin/users/${item.creator_id}`);
          }}
          title={item.creator_id}
        >
          {item.creator_display_name || item.creator_id}
        </button>
      ),
    },
    {
      key: 'context',
      header: t('review.queue.col_context'),
      render: (item) =>
        item.creator_prior_rejections > 0 ? (
          <span className="review-queue__context">
            {t('review.queue.context_rejections', { count: item.creator_prior_rejections })}
          </span>
        ) : (
          <span className="review-queue__context">
            {t('review.queue.context_approved', { count: item.creator_approved_count })}
          </span>
        ),
    },
    {
      key: 'actions',
      header: '',
      render: (item) => (
        <button
          className="admin-btn admin-btn--primary"
          onClick={(e) => {
            e.stopPropagation();
            navigate(`/admin/review/${item.id}`);
          }}
          aria-label={t('review.queue.review_aria', { title: item.template_title })}
        >
          {t('review.queue.review')}
        </button>
      ),
    },
  ];

  const tabs = [
    { id: 'all', label: t('review.queue.tab_all') },
    { id: 'aging', label: t('review.queue.aging') },
    { id: 'resubmissions', label: t('review.queue.resubmissions') },
  ];

  return (
    <div>
      <div className="review-queue__header">
        <h1 className="admin-page-title">{t('review.queue.title')}</h1>
        <button
          className="admin-btn admin-btn--ghost"
          onClick={() => {
            load();
            loadMetrics();
          }}
          disabled={loading}
        >
          {t('admin.users.refresh')}
        </button>
      </div>

      {metrics && (
        <div className="review-queue__metrics">
          <span>{t('review.queue.open', { count: Number(metrics.open_count) || 0 })}</span>
          <span>
            {t('review.queue.median', {
              duration: formatHours(metrics.median_time_in_review_hours),
            })}
          </span>
          <span>
            {t('review.queue.today', { count: Number(metrics.decisions_today) || 0 })}
          </span>
        </div>
      )}

      <div className="review-queue__filters">
        <div className="review-queue__tabs" role="tablist">
          {tabs.map((tb) => (
            <button
              key={tb.id}
              role="tab"
              aria-selected={tab === tb.id}
              className={`review-queue__tab${tab === tb.id ? ' review-queue__tab--active' : ''}`}
              onClick={() => setParam({ tab: tb.id === 'all' ? '' : tb.id })}
            >
              {tb.label}
            </button>
          ))}
        </div>
        <AdminSelect
          label={t('review.queue.category')}
          value={category}
          onChange={(e) => setParam({ category: e.target.value })}
          options={[
            { value: '', label: t('admin.users.all') },
            { value: 'general', label: 'General' },
            { value: 'writing', label: 'Writing' },
            { value: 'coding', label: 'Coding' },
            { value: 'business', label: 'Business' },
          ]}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      <AdminTable
        columns={columns}
        rows={data.items || []}
        loading={loading}
        rowKey={(item) => item.id}
        onRowClick={(item) => navigate(`/admin/review/${item.id}`)}
        emptyState={<EmptyState title={t('review.queue.empty')} />}
      />

      <Pagination
        page={page}
        total={data.total || 0}
        perPage={perPage}
        onPageChange={(p) => setParam({ page: p }, false)}
      />
    </div>
  );
};

export default ReviewPage;
