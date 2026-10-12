import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import { getAdminUsers, banAdminUser, unbanAdminUser, changeAdminUserRole } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import AdminSearchInput from '../../components/admin/AdminSearchInput';
import AdminSelect from '../../components/admin/AdminSelect';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import ReasonModal from '../../components/admin/ReasonModal';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import { useNotification } from '../../context/NotificationContext';
import { formatRelativeTime, formatNumber } from '../../utils/adminFormat';
import './UsersPage.scss';

/**
 * Admin user list: URL-synced search/filters/sort/pagination plus row
 * actions (ban/unban/role change) behind a mandatory reason modal.
 */
const UsersPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { addNotification } = useNotification();
  const adminUser = useSelector((state) => state.adminAuth.user);
  const [searchParams, setSearchParams] = useSearchParams();

  const q = searchParams.get('q') || '';
  const status = searchParams.get('status') || '';
  const role = searchParams.get('role') || '';
  const sort = searchParams.get('sort') || 'newest';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = Math.max(1, parseInt(searchParams.get('per_page') || '20', 10) || 20);

  const [data, setData] = useState({ users: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [modal, setModal] = useState(null); // { action, user }
  const [busy, setBusy] = useState(false);
  const [menuFor, setMenuFor] = useState(null); // user id with open ⋯ menu
  const menuRef = useRef(null);

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
      const params = { page, per_page: perPage, sort };
      if (q) params.search = q;
      if (status) params.status = status;
      if (role) params.role = role;
      const resp = await getAdminUsers(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [q, status, role, sort, page, perPage]);

  useEffect(() => {
    load();
  }, [load]);

  // Close the ⋯ menu on outside click.
  useEffect(() => {
    const handleClick = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) setMenuFor(null);
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  const handleAction = async (reason) => {
    if (!modal) return;
    setBusy(true);
    const { action, user } = modal;
    try {
      if (action === 'ban') await banAdminUser(user.id, reason);
      else if (action === 'unban') await unbanAdminUser(user.id, reason);
      else if (action === 'promote') await changeAdminUserRole(user.id, 'admin', reason);
      else if (action === 'demote') await changeAdminUserRole(user.id, 'user', reason);
      addNotification({ kind: 'success', title: t('admin.users.action_done') });
      setModal(null);
      await load();
    } catch (err) {
      const msg = (err.response && err.response.data && err.response.data.error) || '';
      addNotification({ kind: 'error', title: msg || t('admin.common.error') });
      // Keep the modal open so the admin can retry or cancel.
    } finally {
      setBusy(false);
    }
  };

  const lang = i18n.language || 'en';
  const hasFilters = q || status || role;

  const columns = [
    {
      key: 'email',
      header: t('admin.users.email'),
      render: (u) => (
        <div className="admin-users__identity">
          <span className="admin-users__name">{u.display_name || '—'}</span>
          <span className="admin-users__email">{u.email}</span>
        </div>
      ),
    },
    {
      key: 'role',
      header: t('admin.users.role'),
      render: (u) => (
        <Badge tone={u.role === 'admin' ? 'accent' : 'neutral'}>
          {u.role === 'admin' ? t('admin.users.admin') : t('admin.users.user')}
        </Badge>
      ),
    },
    {
      key: 'status',
      header: t('admin.users.status'),
      render: (u) => (
        <Badge tone={u.status === 'banned' ? 'danger' : 'success'} dot>
          {u.status === 'banned' ? t('admin.users.banned') : t('admin.users.active')}
        </Badge>
      ),
    },
    {
      key: 'template_count',
      header: t('admin.users.templates'),
      render: (u) => formatNumber(u.template_count, lang),
    },
    {
      key: 'created_at',
      header: t('admin.users.created'),
      render: (u) => (
        <span title={u.created_at}>{formatRelativeTime(u.created_at, lang)}</span>
      ),
    },
    {
      key: 'actions',
      header: t('admin.users.actions'),
      render: (u) => (
        <div className="admin-users__actions" ref={menuFor === u.id ? menuRef : undefined}>
          <button
            className="admin-btn admin-btn--ghost admin-users__view-btn"
            onClick={(e) => {
              e.stopPropagation();
              navigate(`/admin/users/${u.id}`);
            }}
          >
            {t('admin.users.view')}
          </button>
          <button
            className="admin-users__menu-btn"
            aria-label={t('admin.users.actions')}
            onClick={(e) => {
              e.stopPropagation();
              setMenuFor(menuFor === u.id ? null : u.id);
            }}
          >
            ⋯
          </button>
          {menuFor === u.id && (
            <div className="admin-users__menu" role="menu">
              <button
                role="menuitem"
                onClick={(e) => {
                  e.stopPropagation();
                  setMenuFor(null);
                  navigate(`/admin/users/${u.id}`);
                }}
              >
                {t('admin.users.view')}
              </button>
              {u.status !== 'banned' && adminUser && u.id !== adminUser.id && (
                <button
                  role="menuitem"
                  className="admin-users__menu-danger"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuFor(null);
                    setModal({ action: 'ban', user: u });
                  }}
                >
                  {t('admin.users.ban')}…
                </button>
              )}
              {u.status === 'banned' && (
                <button
                  role="menuitem"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuFor(null);
                    setModal({ action: 'unban', user: u });
                  }}
                >
                  {t('admin.users.unban')}…
                </button>
              )}
              {u.role !== 'admin' && (
                <button
                  role="menuitem"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuFor(null);
                    setModal({ action: 'promote', user: u });
                  }}
                >
                  {t('admin.users.make_admin')}…
                </button>
              )}
              {u.role === 'admin' && adminUser && u.id !== adminUser.id && (
                <button
                  role="menuitem"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuFor(null);
                    setModal({ action: 'demote', user: u });
                  }}
                >
                  {t('admin.users.demote')}…
                </button>
              )}
            </div>
          )}
        </div>
      ),
    },
  ];

  return (
    <div>
      <div className="admin-users__header">
        <div>
          <h1 className="admin-page-title">{t('admin.users.title')}</h1>
        </div>
        <button className="admin-btn admin-btn--ghost" onClick={load} disabled={loading}>
          {t('admin.users.refresh')}
        </button>
      </div>

      <div className="admin-users__filters">
        <AdminSearchInput
          value={q}
          onChange={(v) => setParam({ q: v })}
          placeholder={t('admin.users.search_placeholder')}
        />
        <AdminSelect
          label={t('admin.users.status')}
          value={status}
          onChange={(e) => setParam({ status: e.target.value })}
          options={[
            { value: '', label: t('admin.users.all') },
            { value: 'active', label: t('admin.users.active') },
            { value: 'banned', label: t('admin.users.banned') },
          ]}
        />
        <AdminSelect
          label={t('admin.users.role')}
          value={role}
          onChange={(e) => setParam({ role: e.target.value })}
          options={[
            { value: '', label: t('admin.users.all') },
            { value: 'user', label: t('admin.users.user') },
            { value: 'admin', label: t('admin.users.admin') },
          ]}
        />
        <AdminSelect
          label={t('admin.users.sort')}
          value={sort}
          onChange={(e) => setParam({ sort: e.target.value })}
          options={[
            { value: 'newest', label: t('admin.users.newest') },
            { value: 'oldest', label: t('admin.users.oldest') },
            { value: 'recently_updated', label: t('admin.users.recently_updated') },
          ]}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      <AdminTable
        columns={columns}
        rows={data.users || []}
        loading={loading}
        rowKey={(u) => u.id}
        onRowClick={(u) => navigate(`/admin/users/${u.id}`)}
        emptyState={
          hasFilters ? (
            <EmptyState
              title={t('admin.users.no_match')}
              action={
                <button
                  className="admin-btn admin-btn--ghost"
                  onClick={() => setParam({ q: '', status: '', role: '' })}
                >
                  {t('admin.users.clear_filters')}
                </button>
              }
            />
          ) : (
            <EmptyState title={t('admin.users.no_match')} />
          )
        }
      />

      <Pagination
        page={page}
        total={data.total || 0}
        perPage={perPage}
        onPageChange={(p) => setParam({ page: p }, false)}
      />

      <ReasonModal
        open={!!modal}
        action={modal ? modal.action : null}
        name={modal ? modal.user.display_name || modal.user.email : ''}
        onConfirm={handleAction}
        onClose={() => setModal(null)}
        busy={busy}
      />
    </div>
  );
};

export default UsersPage;
