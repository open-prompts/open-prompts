import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './RemovedNotice.scss';

/**
 * Owner soft-deleted notice with reason + appeal hint. Shown on the owner's
 * direct visit to a soft-deleted template.
 */
const RemovedNotice = ({ reason }) => {
  const { t } = useTranslation();
  return (
    <div className="mod-removed-notice" role="alert">
      <AlertIcon size={16} />
      <span>{t('mod.banner.removed', { reason: reason || '' })}</span>
    </div>
  );
};

export default RemovedNotice;