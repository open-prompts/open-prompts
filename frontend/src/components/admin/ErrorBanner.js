import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './ErrorBanner.scss';

/**
 * Inline error banner with an optional retry button.
 */
const ErrorBanner = ({ message, onRetry }) => {
  const { t } = useTranslation();
  return (
    <div className="admin-error-banner" role="alert">
      <AlertIcon size={18} />
      <span className="admin-error-banner__message">{message || t('admin.common.error')}</span>
      {onRetry && (
        <button className="admin-btn admin-btn--ghost" onClick={onRetry}>
          {t('admin.common.retry')}
        </button>
      )}
    </div>
  );
};

export default ErrorBanner;
