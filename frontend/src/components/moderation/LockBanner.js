import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './LockBanner.scss';

/**
 * Public generic lock notice shown on a locked template's detail page. The
 * reason is admin-only (NFR-3); the public banner shows a generic message.
 */
const LockBanner = () => {
  const { t } = useTranslation();
  return (
    <div className="mod-lock-banner" role="status">
      <AlertIcon size={16} />
      <span>{t('mod.lock.public')}</span>
    </div>
  );
};

export default LockBanner;