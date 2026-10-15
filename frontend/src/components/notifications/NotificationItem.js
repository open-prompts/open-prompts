import React from 'react';
import { useTranslation } from 'react-i18next';
import { HeartIcon, StarIcon, ForkIcon, CheckIcon, AlertIcon } from '../ui/Icons';
import { formatRelativeTime } from '../../utils/time';

const EVENT_ICONS = {
  like: { Icon: HeartIcon, className: 'event-icon-like' },
  favorite: { Icon: StarIcon, className: 'event-icon-favorite' },
  fork: { Icon: ForkIcon, className: 'event-icon-fork' },
  template_approved: { Icon: CheckIcon, className: 'event-icon-approved' },
  template_rejected: { Icon: AlertIcon, className: 'event-icon-rejected' },
};

/**
 * Shared notification row used by the bell dropdown and the history page.
 * The sentence is composed client-side from typed data so it stays fully
 * translatable; actor name and template title are interpolated values.
 *
 * @param {Object} props
 * @param {Object} props.notification - API notification object
 * @param {Function} props.onOpen - row click (mark read + navigate)
 * @param {Function} [props.onMarkRead] - explicit mark-read action (page only)
 * @param {Function} [props.onDelete] - delete action (page only)
 * @param {boolean} [props.compact] - compact rendering for the dropdown
 */
const NotificationItem = ({ notification, onOpen, onMarkRead, onDelete, compact = false }) => {
  const { t } = useTranslation();
  const n = notification;
  const unread = !n.read_at;
  const event = EVENT_ICONS[n.type] || EVENT_ICONS.like;
  const EventIcon = event.Icon;

  // Review decisions have no acting user in the sentence sense: the
  // actor_display_name slot carries the rejection reason code instead.
  const isReviewDecision = n.type === 'template_approved' || n.type === 'template_rejected';
  const sentence = isReviewDecision
    ? t(`notifications.event.${n.type}`, {
        title: n.template_title,
        reason: n.actor_display_name,
        defaultValue: `Review decision on "${n.template_title}"`,
      })
    : t(`notifications.event.${n.type}`, {
        actor: n.actor_display_name,
        title: n.template_title,
        defaultValue: `${n.actor_display_name} ${n.type} "${n.template_title}"`,
      });
  const relativeTime = formatRelativeTime(n.created_at, t);
  const absoluteTime = n.created_at ? new Date(n.created_at).toLocaleString() : '';

  return (
    <li
      className={`notification-item ${unread ? 'unread' : ''} ${compact ? 'compact' : ''}`}
    >
      <button
        type="button"
        className="notification-item-main"
        onClick={() => onOpen && onOpen(n)}
        title={n.template_title}
      >
        <span className="notification-unread-dot" aria-hidden="true" />
        <span className={`notification-event-icon ${event.className}`} aria-hidden="true">
          <EventIcon size={16} />
        </span>
        <span className="notification-body">
          <span className="notification-sentence">{sentence}</span>
          <time className="notification-time" dateTime={n.created_at} title={absoluteTime}>
            {relativeTime}
          </time>
        </span>
      </button>
      {!compact && (
        <span className="notification-actions">
          {unread && onMarkRead && (
            <button
              type="button"
              className="notification-action mark-read"
              onClick={() => onMarkRead(n)}
              aria-label={t('notifications.mark_read')}
              title={t('notifications.mark_read')}
            >
              {t('notifications.mark_read')}
            </button>
          )}
          {onDelete && (
            <button
              type="button"
              className="notification-action delete"
              onClick={() => onDelete(n)}
              aria-label={t('notifications.delete')}
              title={t('notifications.delete')}
            >
              {t('notifications.delete')}
            </button>
          )}
        </span>
      )}
    </li>
  );
};

export default NotificationItem;
