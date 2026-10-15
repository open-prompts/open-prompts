/**
 * Locale-aware count abbreviation. Uses the Intl compact notation so it
 * renders "1.2K" for en and "1.2万" for zh without a hardcoded table.
 * Falls back to the raw number when Intl is unavailable.
 *
 * @param {number} count
 * @param {string} [locale] - BCP-47 tag (defaults to the browser locale)
 * @returns {string}
 */
export const formatCount = (count, locale) => {
  const n = Number(count) || 0;
  if (typeof Intl !== 'undefined' && Intl.NumberFormat) {
    try {
      return new Intl.NumberFormat(locale || undefined, {
        notation: 'compact',
        maximumFractionDigits: 1,
      }).format(n);
    } catch (e) {
      // fall through to raw number
    }
  }
  return String(n);
};

export default formatCount;