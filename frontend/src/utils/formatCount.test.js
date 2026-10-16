import { formatCount } from './formatCount';

describe('formatCount', () => {
  test('returns 0 for falsy input', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(null)).toBe('0');
    expect(formatCount(undefined)).toBe('0');
  });

  test('formats small numbers with locale separators (en)', () => {
    expect(formatCount(999, 'en')).toBe('999');
    expect(formatCount(1000, 'en')).toBe('1K');
  });

  test('formats compact numbers (en)', () => {
    expect(formatCount(9999, 'en')).toBe('10K');
    expect(formatCount(10000, 'en')).toBe('10K');
    expect(formatCount(1000000, 'en')).toBe('1M');
  });

  test('formats compact numbers (zh)', () => {
    expect(formatCount(9999, 'zh-CN')).toBe('9999');
    expect(formatCount(10000, 'zh-CN')).toBe('1万');
    expect(formatCount(1000000, 'zh-CN')).toBe('100万');
  });
});