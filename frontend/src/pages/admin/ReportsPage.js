import React, { useEffect, useState, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { listAdminReports } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import AdminSelect from '../../components/admin/AdminSelect';
import ReviewDrawer from '../../components/admin/ReviewDrawer';
import { formatRelativeTime } from '../../utils/adminFormat';
import './ReportsPage.scss';

const STATUS_TONE = { open: 'warning', resolved: 'neutral' };
const TYPE_TONE = { template: 'accent', comment: 'neutral' };

/**
 * /admin/reports — grouped report queue with status/type/reason filters,
 * open-first by count then recency (FR-8/FR-9). Review opens the drawer.
 */
const ReportsPage = () => {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();

  const status = searchParams.get('status') || 'open';
  const itemType = searchParams.get('item_type') || '';
  const reason = searchParams.get('reason') || '';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = 50;

  const [data, setData] = useState({ groups: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [drawer, setDrawer] = useState(null); // { item_type, item_id }

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
      if (status !== 'all') params.status = status;
      if (itemType) params.item_type = itemType;
      if (reason) params.reason = reason;
      const resp = await listAdminReports(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [status, itemType, reason, page]);

  useEffect(() => {
    load();
  }, [load]);

  const columns = [
    {
      key: 'type',
      header: t('admin.reports.type'),
      render: (item) => <Badge tone={TYPE_TONE[item.item_type] || 'neutral'}>{t(`admin.reports.type${item.item_type === 'template' ? 'Template' : 'Comment'}`)}</Badge>,
    },
    {
      key: 'item',
      header: t('admin.reports.item'),
      render: (item) => <span className="reports-queue__item">{item.item_title}</span>,
    },
    {
      key: 'author',
      header: t('admin.reports.author'),
      render: (item) => <span>{item.author_display_name || '—'}</span>,
    },
    {
      key: 'open_count',
      header: t('admin.reports.openCount'),
      render: (item) => <span className="reports-queue__count">{item.open_count}</span>,
    },
    {
      key: 'reasons',
      header: t('admin.reports.reasons'),
      render: (item) => (
        <span className="reports-queue__reasons">
          {(item.reasons || []).map((r) => (
            <span key={r} className="reports-queue__chip">{t(`reports.reason.${r}`, { defaultValue: r })}</span>
          ))}
        </span>
      ),
    },
    {
      key: 'latest',
      header: t('admin.reports.latest'),
      render: (item) => <span>{formatRelativeTime(item.latest_at, lang)}</span>,
    },
    {
      key: 'status',
      header: t('admin.reports.status'),
      render: (item) => <Badge tone={STATUS_TONE[item.status] || 'neutral'}>{t(`admin.reports.${item.status}`, { defaultValue: item.status })}</Badge>,
    },
    {
      key: 'actions',
      header: t('admin.reports.actions'),
      render: (item) => (
        <button
          type="button"
          className="admin-btn admin-btn--small"
          onClick={(e) => {
            e.stopPropagation();
            setDrawer({ item_type: item.item_type, item_id: item.item_id });
          }}
        >
          {item.status === 'open' ? t('admin.reports.review') : t('admin.reports.view')}
        </button>
      ),
    },
  ];

  return (
    <div className="reports-queue">
      <h1 className="admin-page-title">{t('admin.reports.title')}</h1>

      <div className="reports-queue__filters">
        <AdminSelect
          value={status}
          onChange={(e) => setParam({ status: e.target.value })}
          options={[
            { value: 'open', label: t('admin.reports.open') },
            { value: 'resolved', label: t('admin.reports.resolved') },
            { value: 'all', label: t('admin.reports.all') },
          ]}
          label={t('admin.reports.status')}
        />
        <AdminSelect
          value={itemType}
          onChange={(e) => setParam({ item_type: e.target.value })}
          options={[
            { value: '', label: t('admin.reports.allTypes') },
            { value: 'template', label: t('admin.reports.typeTemplate') },
            { value: 'comment', label: t('admin.reports.typeComment') },
          ]}
          label={t('admin.reports.type')}
        />
        <AdminSelect
          value={reason}
          onChange={(e) => setParam({ reason: e.target.value })}
          options={[
            { value: '', label: t('admin.reports.allReasons') },
            { value: 'spam', label: t('reports.reason.spam') },
            { value: 'offensive', label: t('reports.reason.offensive') },
            { value: 'misleading', label: t('reports.reason.misleading') },
            { value: 'privacy', label: t('reports.reason.privacy') },
            { value: 'other', label: t('reports.reason.other') },
          ]}
          label={t('admin.reports.reasons')}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}
      <AdminTable
        columns={columns}
        rows={data.groups || []}
        loading={loading}
        rowKey={(row) => `${row.item_type}:${row.item_id}`}
        emptyState={<EmptyState title={t('admin.reports.empty')} />}
      />
      <Pagination page={page} total={data.total || 0} perPage={perPage} onPageChange={(p) => setParam({ page: p }, false)} />

      {drawer && (
        <ReviewDrawer
          open
          itemType={drawer.item_type}
          itemId={drawer.item_id}
          onClose={() => setDrawer(null)}
          onResolved={load}
        />
      )}
    </div>
  );
};

export default ReportsPage;