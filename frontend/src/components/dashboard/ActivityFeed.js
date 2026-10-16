import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatRelativeTime } from '../../utils/time';

const TYPE_ICON = {
  TEMPLATE: '📄',
  PROMPT: '💬',
  LIKE: '❤️',
  FAVORITE: '⭐',
};

/**
 * ActivityFeed — the recent-activity list on the dashboard overview.
 * @param {Object} props
 * @param {Array} props.items - DashboardActivityItem[] from the API
 */
const ActivityFeed = ({ items }) => {
  const { t } = useTranslation();

  if (!items || items.length === 0) {
    return <p className="activity-feed__empty">{t('dashboard_v2.activity.empty')}</p>;
  }

  return (
    <ul className="activity-feed">
      {items.map((item, idx) => {
        const key = `${item.type}-${item.entityId}-${idx}`;
        const isPrompt = item.type === 'PROMPT';
        const title = isPrompt
          ? t('dashboard_v2.activity.prompt_used', { title: item.entityTitle })
          : item.type === 'LIKE'
            ? t('dashboard_v2.activity.liked', { title: item.entityTitle })
            : item.type === 'FAVORITE'
              ? t('dashboard_v2.activity.favorited', { title: item.entityTitle })
              : t('dashboard_v2.activity.template_created', { title: item.entityTitle });
        const href = isPrompt && item.templateId
          ? `/templates/${item.templateId}`
          : `/templates/${item.entityId}`;
        return (
          <li key={key} className="activity-feed__item">
            <span className="activity-feed__icon" aria-hidden="true">
              {TYPE_ICON[item.type] || '•'}
            </span>
            <Link to={href} className="activity-feed__text">
              {title}
            </Link>
            <time
              className="activity-feed__time"
              dateTime={item.timestamp}
              title={new Date(item.timestamp).toLocaleString()}
            >
              {formatRelativeTime(item.timestamp, t)}
            </time>
          </li>
        );
      })}
    </ul>
  );
};

export default ActivityFeed;