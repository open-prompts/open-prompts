import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { listAdminAppeals } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import { formatRelativeTime } from '../../utils/adminFormat';
import './AppealsPage.scss';

const STATE_TONE = {
  open: 'warning',
  under_review: 'accent',
  resolved_upheld: 'neutral',
  resolved_reversed: 'success',
};

/**
 * /admin/appeals — appeal queue with state filter tabs + counts,
 * oldest-first rows, statement excerpt.
 */
const AppealsPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const tab = searchParams.get('state') || 'open';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = 20;

  const [data, setData] = useState({ entries: [], total: 0 });
  const [counts, setCounts] = useState({ open_count: 0, under_review_count: 0, resolved_count: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const lang = i18n.language || 'en';

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
      if (tab !== 'all') params.state = tab;
      const resp = await listAdminAppeals(params);
      setData(resp.data);
      setCounts({
        open_count: resp.data.open_count || 0,
        under_review_count: resp.data.under_review_count || 0,
        resolved_count: resp.data.resolved_count || 0,
      });
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [tab, page]);

  useEffect(() => {
    load();
  }, [load]);

  const tabs = [
    { key: 'open', label: t('appeal.state.open'), count: counts.open_count },
    { key: 'under_review', label: t('appeal.state.under_review'), count: counts.under_review_count },
    { key: 'resolved', label: t('appeal.state.resolved', { defaultValue: 'Resolved' }), count: counts.resolved_count },
  ];

  const columns = [
    {
      key: 'age',
      header: t('appeal.admin.queue.col_age', { defaultValue: 'Age' }),
      render: (item) => <span className="appeals-queue__age">{formatRelativeTime(item.created_at, lang)}</span>,
    },
    {
      key: 'template',
      header: t('appeal.admin.queue.col_template', { defaultValue: 'Template' }),
      render: (item) => <span className="appeals-queue__title">{item.template_title}</span>,
    },
    {
      key: 'action',
      header: t('appeal.admin.queue.col_action', { defaultValue: 'Action' }),
      render: (item) => <span>{item.action_type}</span>,
    },
    {
      key: 'statement',
      header: t('appeal.admin.queue.col_statement', { defaultValue: 'Statement' }),
      render: (item) => <span className="appeals-queue__excerpt">{item.statement_excerpt}</span>,
    },
    {
      key: 'appellant',
      header: t('appeal.admin.queue.col_appellant', { defaultValue: 'Appellant' }),
      render: (item) => <span>{item.appellant_display_name}</span>,
    },
    {
      key: 'state',
      header: t('appeal.admin.queue.col_state', { defaultValue: 'State' }),
      render: (item) => <Badge tone={STATE_TONE[item.state] || 'neutral'}>{t(`appeal.state.${item.state}`, { defaultValue: item.state })}</Badge>,
    },
  ];

  return (
    <div className="appeals-queue">
      <h1 className="appeals-queue__title">{t('appeal.admin.queue.title')}</h1>
      <div className="appeals-queue__tabs" role="tablist" aria-label={t('appeal.admin.queue.title')}>
        {tabs.map((tb) => (
          <button
            key={tb.key}
            role="tab"
            aria-selected={tab === tb.key}
            className={`appeals-queue__tab${tab === tb.key ? ' appeals-queue__tab--active' : ''}`}
            onClick={() => setParam({ state: tb.key })}
          >
            {tb.label} ({tb.count})
          </button>
        ))}
      </div>
      {error && <ErrorBanner message={error} onRetry={load} />}
      <AdminTable
        columns={columns}
        rows={data.entries || []}
        loading={loading}
        onRowClick={(row) => navigate(`/admin/appeals/${row.id}`)}
        rowKey={(row) => row.id}
        emptyState={<EmptyState title={t('appeal.admin.queue.empty')} />}
      />
      <Pagination page={page} total={data.total || 0} perPage={perPage} onPageChange={(p) => setParam({ page: p }, false)} />
    </div>
  );
};

export default AppealsPage;