import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * SkipLink component.
 * First focusable element in the layout; lets keyboard users jump straight
 * to the main content, skipping the header and sidebar.
 */
export default function SkipLink() {
  const { t } = useTranslation();
  return <a href="#main-content" className="skip-link">{t('a11y.skip_to_content')}</a>;
}