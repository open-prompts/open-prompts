/**
 * Shared formatting helpers for admin pages.
 */

const relativeTimeUnits = [
  { limit: 60, divisor: 1, en: 'second', zh: '秒' },
  { limit: 3600, divisor: 60, en: 'minute', zh: '分钟' },
  { limit: 86400, divisor: 3600, en: 'hour', zh: '小时' },
  { limit: 2592000, divisor: 86400, en: 'day', zh: '天' },
  { limit: 31536000, divisor: 2592000, en: 'month', zh: '个月' },
  { limit: Infinity, divisor: 31536000, en: 'year', zh: '年' },
];

/**
 * Format a relative time like "3 hours ago" / "3 小时前".
 */
export const formatRelativeTime = (dateStr, lang = 'en') => {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return dateStr;
  const diffSeconds = Math.max(0, (Date.now() - date.getTime()) / 1000);
  const isZh = lang === 'zh' || (lang || '').startsWith('zh');

  for (const unit of relativeTimeUnits) {
    if (diffSeconds < unit.limit) {
      const value = Math.max(1, Math.floor(diffSeconds / unit.divisor));
      if (isZh) return `${value} ${unit.zh}前`;
      return `${value} ${unit.en}${value === 1 ? '' : 's'} ago`;
    }
  }
  return date.toLocaleDateString(isZh ? 'zh-CN' : 'en-US');
};

/**
 * Locale-aware absolute date, e.g. "Jan 5, 2025, 14:30".
 */
export const formatDate = (dateStr, lang = 'en') => {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return dateStr;
  const isZh = lang === 'zh' || (lang || '').startsWith('zh');
  return date.toLocaleString(isZh ? 'zh-CN' : 'en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

/**
 * Locale number formatting, e.g. 12345 → "12,345".
 */
export const formatNumber = (value, lang = 'en') => {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '0';
  const isZh = lang === 'zh' || (lang || '').startsWith('zh');
  return Number(value).toLocaleString(isZh ? 'zh-CN' : 'en-US');
};
