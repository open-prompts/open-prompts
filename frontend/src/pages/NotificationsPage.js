import React, { useCallback, useEffect, useState } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Header from '../components/Header';
import Layout from '../components/Layout';
import Modal from '../components/ui/Modal';
import NotificationItem from '../components/notifications/NotificationItem';
import { useNotification } from '../context/NotificationContext';
import {
  fetchNotifications,
  markRead,
  markAllRead,
  removeNotification,
  setFilter,
} from '../store/notificationsSlice';
import './NotificationsPage.scss';

const PAGE_SIZE = 20;

/**
 * Notification history page: All/Unread tabs with counts, 20/page Load more,
 * per-item mark-read and delete (with confirm), and mark-all-read.
 */
const NotificationsPage = () => {
  const { t } = useTranslation();
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const { addNotification } = useNotification();
  const { token } = useSelector((state) => state.auth);
  const {
    items, unreadCount, filter, nextPageToken, hasMore, loading, error,
  } = useSelector((state) => state.notifications);
  const [pendingDelete, setPendingDelete] = useState(null);

  useEffect(() => {
    if (!token) {
      navigate('/login');
    }
  }, [token, navigate]);

  const load = useCallback((nextFilter, reset) => {
    dispatch(fetchNotifications({
      filter: nextFilter,
      reset,
      pageSize: PAGE_SIZE,
      pageToken: reset ? '' : nextPageToken,
    }));
  }, [dispatch, nextPageToken]);

  // Initial load + filter switches reset pagination.
  useEffect(() => {
    if (token) {
      load(filter, true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, filter]);

  const handleTabChange = (nextFilter) => {
    if (nextFilter !== filter) {
      dispatch(setFilter(nextFilter));
    }
  };

  const handleLoadMore = () => {
    load(filter, false);
  };

  const handleItemOpen = (notification) => {
    if (!notification.read_at) {
      dispatch(markRead(notification.id));
    }
    if (notification.type === 'appeal_resolved') {
      let appealId = '';
      try {
        appealId = (JSON.parse(notification.actor_display_name || '{}') || {}).appeal_id || '';
      } catch (e) {
        appealId = '';
      }
      if (appealId) {
        navigate(`/appeals/${appealId}`);
        return;
      }
    }
    navigate(`/templates/${notification.template_id}`);
  };

  const handleMarkRead = (notification) => {
    dispatch(markRead(notification.id))
      .unwrap()
      .then(() => addNotification({ kind: 'success', title: t('notifications.toast.marked_read') }))
      .catch(() => addNotification({ kind: 'error', title: t('notifications.error.mark') }));
  };

  const handleMarkAllRead = () => {
    dispatch(markAllRead())
      .unwrap()
      .then(() => addNotification({ kind: 'success', title: t('notifications.toast.all_marked_read') }))
      .catch(() => addNotification({ kind: 'error', title: t('notifications.error.mark') }));
  };

  const handleDeleteRequest = (notification) => {
    setPendingDelete(notification);
  };

  const handleDeleteConfirm = () => {
    const target = pendingDelete;
    setPendingDelete(null);
    if (!target) return;
    dispatch(removeNotification(target.id))
      .unwrap()
      .then(() => addNotification({ kind: 'success', title: t('notifications.toast.deleted') }))
      .catch(() => addNotification({ kind: 'error', title: t('notifications.error.delete') }));
  };

  const totalCount = items.length + (hasMore ? 1 : 0); // "at least N" display
  const emptyMessage = filter === 'unread'
    ? t('notifications.empty.unread')
    : t('notifications.empty.all');

  return (
    <React.Fragment>
      <Header />
      <Layout showSidebar={false} showCreateButton={false}>
        <div className="notifications-page">
          <div className="notifications-page-header">
            <h1>{t('notifications.title')}</h1>
            {unreadCount > 0 && (
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={handleMarkAllRead}
                disabled={loading}
              >
                {t('notifications.mark_all_read')}
              </button>
            )}
          </div>

          <nav className="notifications-filter-tabs" aria-label={t('notifications.title')}>
            <button
              type="button"
              className={`filter-tab ${filter === 'all' ? 'active' : ''}`}
              onClick={() => handleTabChange('all')}
              aria-current={filter === 'all' ? 'page' : undefined}
            >
              {t('notifications.filter.all', { count: filter === 'all' ? Math.max(items.length, 0) : totalCount })}
            </button>
            <button
              type="button"
              className={`filter-tab ${filter === 'unread' ? 'active' : ''}`}
              onClick={() => handleTabChange('unread')}
              aria-current={filter === 'unread' ? 'page' : undefined}
            >
              {t('notifications.filter.unread', { count: unreadCount })}
            </button>
          </nav>

          {error && (
            <div className="notifications-error" role="alert">
              <p>{t('notifications.error.fetch')}</p>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => load(filter, true)}
              >
                {t('notifications.error.retry')}
              </button>
            </div>
          )}

          {!error && loading && items.length === 0 && (
            <div className="notifications-list-skeleton" aria-label={t('common.loading')}>
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="notification-skeleton-row" />
              ))}
            </div>
          )}

          {!error && !loading && items.length === 0 && (
            <div className="notifications-empty">
              <p>{emptyMessage}</p>
            </div>
          )}

          {items.length > 0 && (
            <ul className="notifications-list">
              {items.map((n) => (
                <NotificationItem
                  key={n.id}
                  notification={n}
                  onOpen={handleItemOpen}
                  onMarkRead={handleMarkRead}
                  onDelete={handleDeleteRequest}
                />
              ))}
            </ul>
          )}

          {hasMore && !loading && (
            <div className="notifications-load-more">
              <button type="button" className="btn btn-ghost" onClick={handleLoadMore}>
                {t('notifications.load_more')}
              </button>
            </div>
          )}
        </div>
      </Layout>

      <Modal
        open={Boolean(pendingDelete)}
        title={t('notifications.delete')}
        danger
        primaryLabel={t('notifications.delete')}
        secondaryLabel={t('common.cancel', 'Cancel')}
        onPrimary={handleDeleteConfirm}
        onClose={() => setPendingDelete(null)}
      >
        <p>{t('notifications.confirm.delete')}</p>
      </Modal>
    </React.Fragment>
  );
};

export default NotificationsPage;
