import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import { getAdminUser, banAdminUser, unbanAdminUser, changeAdminUserRole } from '../../services/adminApi';
import Badge from '../../components/admin/Badge';
import ReasonModal from '../../components/admin/ReasonModal';
import ErrorBanner from '../../components/admin/ErrorBanner';
import EmptyState from '../../components/admin/EmptyState';
import { useNotification } from '../../context/NotificationContext';
import { formatDate, formatNumber, formatRelativeTime } from '../../utils/adminFormat';
import './UserDetailPage.scss';

/**
 * Admin user detail: profile header, stats, identities, API keys, latest
 * templates and the admin action history for this user.
 */
const UserDetailPage = () => {
  const { id } = useParams();
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { addNotification } = useNotification();
  const adminUser = useSelector((state) => state.adminAuth.user);

  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState('');
  const [modal, setModal] = useState(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotFound(false);
    try {
      const resp = await getAdminUser(id);
      setData(resp.data);
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 404) setNotFound(true);
      else setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const handleAction = async (reason) => {
    if (!modal) return;
    setBusy(true);
    const { action } = modal;
    try {
      if (action === 'ban') await banAdminUser(id, reason);
      else if (action === 'unban') await unbanAdminUser(id, reason);
      else if (action === 'promote') await changeAdminUserRole(id, 'admin', reason);
      else if (action === 'demote') await changeAdminUserRole(id, 'user', reason);
      addNotification({ kind: 'success', title: t('admin.users.action_done') });
      setModal(null);
      await load();
    } catch (err) {
      const msg = (err.response && err.response.data && err.response.data.error) || '';
      addNotification({ kind: 'error', title: msg || t('admin.common.error') });
    } finally {
      setBusy(false);
    }
  };

  if (loading) {
    return (
      <div>
        <h1 className="admin-page-title">{t('admin.users.title')}</h1>
        <p className="admin-page-subtitle">{t('admin.common.loading')}</p>
      </div>
    );
  }

  if (notFound) {
    return (
      <div>
        <Link to="/admin/users" className="admin-detail__back">
          ← {t('admin.users.back')}
        </Link>
        <EmptyState title={t('admin.users.not_found')} />
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <Link to="/admin/users" className="admin-detail__back">
          ← {t('admin.users.back')}
        </Link>
        <ErrorBanner message={error} onRetry={load} />
      </div>
    );
  }

  const { user, stats, identities, api_keys, templates, action_history } = data;
  const lang = i18n.language || 'en';
  const isSelf = adminUser && adminUser.id === user.id;

  const statCards = [
    { label: t('admin.users.templates'), value: stats ? stats.templates : 0 },
    { label: t('admin.users.likes_received'), value: stats ? stats.likes_received : 0 },
    { label: t('admin.users.favorites_received'), value: stats ? stats.favorites_received : 0 },
    { label: t('admin.users.prompts_created'), value: stats ? stats.prompts_created : 0 },
  ];

  return (
    <div>
      <Link to="/admin/users" className="admin-detail__back">
        ← {t('admin.users.back')}
      </Link>

      <div className="admin-user-detail__header">
        <div className="admin-user-detail__identity">
          <h1>{user.display_name || user.email}</h1>
          <p>{user.email}</p>
          <p className="admin-user-detail__id">ID: {user.id}</p>
        </div>
        <div className="admin-user-detail__badges">
          <Badge tone={user.role === 'admin' ? 'accent' : 'neutral'}>
            {user.role === 'admin' ? t('admin.users.admin') : t('admin.users.user')}
          </Badge>
          <Badge tone={user.status === 'banned' ? 'danger' : 'success'} dot>
            {user.status === 'banned' ? t('admin.users.banned') : t('admin.users.active')}
          </Badge>
        </div>
        <div className="admin-user-detail__actions">
          {user.status !== 'banned' && !isSelf && (
            <button
              className="admin-btn admin-btn--danger"
              onClick={() => setModal({ action: 'ban' })}
            >
              {t('admin.users.ban')}…
            </button>
          )}
          {user.status === 'banned' && (
            <button className="admin-btn admin-btn--primary" onClick={() => setModal({ action: 'unban' })}>
              {t('admin.users.unban')}…
            </button>
          )}
          {user.role !== 'admin' && (
            <button className="admin-btn admin-btn--ghost" onClick={() => setModal({ action: 'promote' })}>
              {t('admin.users.make_admin')}…
            </button>
          )}
          {user.role === 'admin' && !isSelf && (
            <button className="admin-btn admin-btn--ghost" onClick={() => setModal({ action: 'demote' })}>
              {t('admin.users.demote')}…
            </button>
          )}
        </div>
      </div>

      <div className="admin-user-detail__stats">
        {statCards.map((card) => (
          <div key={card.label} className="admin-user-detail__stat">
            <span className="admin-user-detail__stat-label">{card.label}</span>
            <span className="admin-user-detail__stat-value">{formatNumber(card.value, lang)}</span>
          </div>
        ))}
      </div>

      <div className="admin-user-detail__grid">
        <section className="admin-user-detail__panel">
          <h2>{t('admin.users.identities')}</h2>
          {(!identities || identities.length === 0) && (
            <p className="admin-user-detail__muted">{t('admin.users.no_identities')}</p>
          )}
          <ul className="admin-user-detail__plain-list">
            {(identities || []).map((identity) => (
              <li key={identity.provider}>
                <Badge tone="accent">{identity.provider}</Badge>
                <span>{formatDate(identity.created_at, lang)}</span>
              </li>
            ))}
          </ul>

          <h2>{t('admin.users.api_keys')}</h2>
          {(!api_keys || api_keys.length === 0) && (
            <p className="admin-user-detail__muted">{t('admin.users.no_api_keys')}</p>
          )}
          <ul className="admin-user-detail__plain-list">
            {(api_keys || []).map((key) => (
              <li key={key.id}>
                <span className="admin-user-detail__key-name">
                  {key.name} <code>{key.prefix}</code>
                </span>
                <Badge tone={key.is_active ? 'success' : 'neutral'}>
                  {key.is_active ? t('admin.users.active') : t('admin.users.inactive')}
                </Badge>
              </li>
            ))}
          </ul>
        </section>

        <section className="admin-user-detail__panel">
          <div className="admin-user-detail__panel-header">
            <h2>{t('admin.users.templates')}</h2>
            <Link to={`/admin/templates?owner_id=${user.id}`}>{t('admin.users.view_all')}</Link>
          </div>
          {(!templates || templates.length === 0) && (
            <p className="admin-user-detail__muted">{t('admin.users.no_templates')}</p>
          )}
          <ul className="admin-user-detail__plain-list">
            {(templates || []).map((tpl) => (
              <li key={tpl.id}>
                <button
                  className="admin-user-detail__link"
                  onClick={() => navigate(`/admin/templates/${tpl.id}`)}
                >
                  {tpl.title}
                </button>
                <Badge tone={tpl.visibility === 'public' ? 'success' : 'neutral'}>
                  {tpl.visibility === 'public'
                    ? t('admin.templates.public')
                    : t('admin.templates.private')}
                </Badge>
              </li>
            ))}
          </ul>
        </section>
      </div>

      <section className="admin-user-detail__panel">
        <h2>{t('admin.users.action_history')}</h2>
        {(!action_history || action_history.length === 0) && (
          <p className="admin-user-detail__muted">{t('admin.users.no_history')}</p>
        )}
        <ul className="admin-user-detail__history">
          {(action_history || []).map((entry) => (
            <li key={entry.id}>
              <Badge
                tone={
                  entry.action === 'ban'
                    ? 'danger'
                    : entry.action === 'unban'
                      ? 'success'
                      : 'warning'
                }
              >
                {t(`admin.audit.${entry.action}`)}
              </Badge>
              <span className="admin-user-detail__history-reason" title={entry.reason}>
                {entry.reason}
              </span>
              <span className="admin-user-detail__history-meta">
                {entry.actor_email} · {formatRelativeTime(entry.created_at, lang)}
              </span>
            </li>
          ))}
        </ul>
      </section>

      <ReasonModal
        open={!!modal}
        action={modal ? modal.action : null}
        name={user.display_name || user.email}
        onConfirm={handleAction}
        onClose={() => setModal(null)}
        busy={busy}
      />
    </div>
  );
};

export default UserDetailPage;
