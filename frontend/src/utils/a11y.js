import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

/**
 * Shared selector for focusable elements inside a dialog/trap.
 * Used by the Modal focus trap and any future dialogs.
 */
export const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * On pathname change: set document.title (i18n'd) and move focus to the page
 * h1. Query-string-only changes (filters, pagination) are ignored.
 *
 * @param {string} pageTitleKey - i18n key for the page title
 */
export function usePageAnnouncement(pageTitleKey) {
  const { pathname } = useLocation();
  const { t } = useTranslation();
  useEffect(() => {
    document.title = t(pageTitleKey);
    const h1 = document.querySelector('main h1');
    if (h1) {
      h1.setAttribute('tabindex', '-1');
      h1.focus({ preventScroll: false });
    }
  }, [pathname, t, pageTitleKey]);
}