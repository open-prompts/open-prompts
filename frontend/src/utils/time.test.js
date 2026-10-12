import { formatRelativeTime } from './time';

const t = (key, opts) => (opts ? `${key}:${opts.count}` : key);

describe('formatRelativeTime', () => {
  test('returns empty string for falsy input', () => {
    expect(formatRelativeTime(null, t)).toBe('');
    expect(formatRelativeTime(undefined, t)).toBe('');
    expect(formatRelativeTime('', t)).toBe('');
  });

  test('returns empty string for invalid dates', () => {
    expect(formatRelativeTime('not-a-date', t)).toBe('');
    expect(formatRelativeTime('2025-13-45T99:99:99Z', t)).toBe('');
  });

  test('just now for < 60 seconds', () => {
    expect(formatRelativeTime(new Date(Date.now() - 10 * 1000), t)).toBe('time.justNow');
  });

  test('minutes ago for < 60 minutes', () => {
    expect(formatRelativeTime(new Date(Date.now() - 5 * 60 * 1000), t)).toBe('time.minutesAgo:5');
  });

  test('hours ago for < 24 hours', () => {
    expect(formatRelativeTime(new Date(Date.now() - 3 * 3600 * 1000), t)).toBe('time.hoursAgo:3');
  });

  test('days ago for < 7 days', () => {
    expect(formatRelativeTime(new Date(Date.now() - 2 * 86400 * 1000), t)).toBe('time.daysAgo:2');
  });

  test('locale date string for >= 7 days', () => {
    const d = new Date(Date.now() - 10 * 86400 * 1000);
    expect(formatRelativeTime(d, t)).toBe(d.toLocaleDateString());
  });

  test('accepts ISO strings', () => {
    expect(formatRelativeTime(new Date().toISOString(), t)).toBe('time.justNow');
  });

  test('future dates clamp to just now', () => {
    expect(formatRelativeTime(new Date(Date.now() + 60 * 1000), t)).toBe('time.justNow');
  });
});
