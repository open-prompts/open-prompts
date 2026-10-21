import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { BellIcon } from '../ui/Icons';
import useDismissable from '../../hooks/useDismissable';
import useNotificationPolling from '../../hooks/useNotificationPolling';
import {
  fetchNotifications,
  markRead,
  markAllRead,
} from '../../store/notificationsSlice';
import NotificationItem from './NotificationItem';
import './NotificationBell.scss';

/**
 * Header bell with unread badge and a 10-item dropdown.
 * Rendered only for authenticated users; mounts the polling hook.
 */
const NotificationBell = () => {
  const { t } = useTranslation();
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const { items, unreadCount, loading, error } = useSelector((state) => state.notifications);
  const [isOpen, setIsOpen] = React.useState(false);
  const wrapperRef = useDismissable(isOpen, () => setIsOpen(false));
  const bellRef = React.useRef(null);

  useNotificationPolling(true);

  // Fetch the 10 most recent on open (mixed read/unread).
  useEffect(() => {
    if (isOpen) {
      dispatch(fetchNotifications({ filter: 'all', reset: true, pageSize: 10 }));
    }
  }, [isOpen, dispatch]);

  const handleBellClick = () => {
    setIsOpen((open) => !open);
  };

  const handleItemOpen = (notification) => {
    if (!notification.read_at) {
      dispatch(markRead(notification.id));
    }
    setIsOpen(false);
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

  const handleMarkAllRead = () => {
    dispatch(markAllRead());
  };

  const handleRetry = () => {
    dispatch(fetchNotifications({ filter: 'all', reset: true, pageSize: 10 }));
  };

  const badgeText = unreadCount > 99 ? '99+' : String(unreadCount);
  const ariaLabel = unreadCount > 0
    ? t('notifications.bell_aria', { count: unreadCount })
    : t('notifications.bell_aria_zero');

  return (
    <div className="notification-bell" ref={wrapperRef}>
      <button
        ref={bellRef}
        type="button"
        className="icon-btn header-icon-btn notification-bell-btn"
        onClick={handleBellClick}
        aria-label={ariaLabel}
        aria-haspopup="true"
        aria-expanded={isOpen}
      >
        <BellIcon size={19} />
        {unreadCount > 0 && (
          <span className="notification-badge" aria-hidden="true">{badgeText}</span>
        )}
      </button>
      <span className="sr-only" role="status" aria-live="polite">
        {unreadCount > 0 ? t('notifications.bell_aria', { count: unreadCount }) : ''}
      </span>

      {isOpen && (
        <div className="notification-dropdown" role="dialog" aria-label={t('notifications.title')}>
          <div className="notification-dropdown-header">
            <span className="notification-dropdown-title">{t('notifications.title')}</span>
            {unreadCount > 0 && (
              <button
                type="button"
                className="notification-dropdown-mark-all"
                onClick={handleMarkAllRead}
              >
                {t('notifications.mark_all_read')}
              </button>
            )}
          </div>
          <div className="notification-dropdown-body">
            {loading && items.length === 0 && (
              <div className="notification-dropdown-loading" aria-label={t('common.loading')}>
                <div className="notification-skeleton-row" />
                <div className="notification-skeleton-row" />
                <div className="notification-skeleton-row" />
              </div>
            )}
            {!loading && error && (
              <div className="notification-dropdown-error">
                <p>{t('notifications.error.fetch')}</p>
                <button type="button" className="btn btn-ghost btn-sm" onClick={handleRetry}>
                  {t('notifications.error.retry')}
                </button>
              </div>
            )}
            {!loading && !error && items.length === 0 && (
              <div className="notification-dropdown-empty">
                <BellIcon size={24} />
                <p className="notification-empty-title">{t('notifications.empty.dropdown')}</p>
              </div>
            )}
            {!error && items.length > 0 && (
              <ul className="notification-dropdown-list">
                {items.slice(0, 10).map((n) => (
                  <NotificationItem
                    key={n.id}
                    notification={n}
                    onOpen={handleItemOpen}
                    compact
                  />
                ))}
              </ul>
            )}
          </div>
          <button
            type="button"
            className="notification-dropdown-footer"
            onClick={() => {
              setIsOpen(false);
              navigate('/notifications');
            }}
          >
            {t('notifications.view_all')}
          </button>
        </div>
      )}
    </div>
  );
};

export default NotificationBell;
