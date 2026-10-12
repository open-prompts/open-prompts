import React, { useEffect, useState, useCallback } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminAuditLogs } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import AdminSearchInput from '../../components/admin/AdminSearchInput';
import AdminSelect from '../../components/admin/AdminSelect';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import { formatDate } from '../../utils/adminFormat';
import './AuditLogPage.scss';

/**
 * Append-only admin audit trail with action/actor filters.
 */
const AuditLogPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const action = searchParams.get('action') || '';
  const actor = searchParams.get('actor') || '';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = Math.max(1, parseInt(searchParams.get('per_page') || '20', 10) || 20);

  const [data, setData] = useState({ entries: [], total: 0 });
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
      if (action) params.action = action;
      if (actor) params.actor = actor;
      const resp = await getAdminAuditLogs(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [action, actor, page, perPage]);

  useEffect(() => {
    load();
  }, [load]);

  const lang = i18n.language || 'en';

  const columns = [
    {
      key: 'created_at',
      header: t('admin.audit.timestamp'),
      render: (e) => <span title={e.created_at}>{formatDate(e.created_at, lang)}</span>,
    },
    {
      key: 'action',
      header: t('admin.audit.action'),
      render: (e) => (
        <Badge
          tone={e.action === 'ban' ? 'danger' : e.action === 'unban' ? 'success' : 'warning'}
        >
          {t(`admin.audit.${e.action}`)}
        </Badge>
      ),
    },
    {
      key: 'actor_email',
      header: t('admin.audit.actor'),
      render: (e) => (
        <span className="admin-audit__actor" title={e.actor_id}>
          {e.actor_email}
        </span>
      ),
    },
    {
      key: 'target_id',
      header: t('admin.audit.target'),
      render: (e) => (
        <button
          className="admin-audit__target-link"
          onClick={(ev) => {
            ev.stopPropagation();
            navigate(`/admin/users/${e.target_id}`);
          }}
          title={e.target_id}
        >
          {e.target_id}
        </button>
      ),
    },
    {
      key: 'reason',
      header: t('admin.audit.reason'),
      render: (e) => (
        <span className="admin-audit__reason" title={e.reason}>
          {e.reason}
        </span>
      ),
    },
  ];

  return (
    <div>
      <h1 className="admin-page-title">{t('admin.audit.title')}</h1>

      <div className="admin-audit__filters">
        <AdminSelect
          label={t('admin.audit.action')}
          value={action}
          onChange={(e) => setParam({ action: e.target.value })}
          options={[
            { value: '', label: t('admin.audit.all') },
            { value: 'ban', label: t('admin.audit.ban') },
            { value: 'unban', label: t('admin.audit.unban') },
            { value: 'role_change', label: t('admin.audit.role_change') },
          ]}
        />
        <AdminSearchInput
          value={actor}
          onChange={(v) => setParam({ actor: v })}
          placeholder={t('admin.audit.search_actor')}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      <AdminTable
        columns={columns}
        rows={data.entries || []}
        loading={loading}
        rowKey={(e) => e.id}
        emptyState={<EmptyState title={t('admin.audit.no_entries')} />}
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

export default AuditLogPage;
