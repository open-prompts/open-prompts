import { useTranslation } from 'react-i18next';

/**
 * Format a date/time as a short relative time string.
 * <60s "just now", <60m "Xm ago", <24h "Xh ago", <7d "Xd ago",
 * else locale date string. Shared with future features (user-dashboard).
 *
 * @param {string|Date} date - the timestamp to format
 * @param {Function} t - i18next translate function (time.* namespace)
 * @returns {string} localized relative time
 */
export const formatRelativeTime = (date, t) => {
  if (!date) return '';
  const then = date instanceof Date ? date : new Date(date);
  if (Number.isNaN(then.getTime())) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - then.getTime()) / 1000));
  if (seconds < 60) return t('time.justNow');
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return t('time.minutesAgo', { count: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t('time.hoursAgo', { count: hours });
  const days = Math.floor(hours / 24);
  if (days < 7) return t('time.daysAgo', { count: days });
  return then.toLocaleDateString();
};

/**
 * React hook wrapper for formatRelativeTime.
 */
export const useRelativeTime = (date) => {
  const { t } = useTranslation();
  return formatRelativeTime(date, t);
};

export default formatRelativeTime;
