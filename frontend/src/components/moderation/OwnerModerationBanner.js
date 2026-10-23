import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './OwnerModerationBanner.scss';

/**
 * Owner-view banner for a moderated template. Variants:
 *  - unpublished: reason + "Edit template" CTA (re-publish via the normal
 *    share flow).
 *  - locked: generic lock notice (reason is admin-only).
 *  - removed: soft-deleted notice with reason + appeal hint.
 */
const OwnerModerationBanner = ({ status, reason, onEdit }) => {
  const { t } = useTranslation();

  if (status === 'unpublished_by_admin') {
    return (
      <div className="mod-owner-banner mod-owner-banner--unpublished" role="alert">
        <AlertIcon size={16} />
        <span>{t('mod.banner.unpublished', { reason: reason || '' })}</span>
        {onEdit && (
          <button type="button" className="mod-owner-banner__cta" onClick={onEdit}>
            {t('mod.banner.editCta')}
          </button>
        )}
      </div>
    );
  }

  if (status === 'locked') {
    return (
      <div className="mod-owner-banner mod-owner-banner--locked" role="status">
        <AlertIcon size={16} />
        <span>{t('mod.banner.locked')}</span>
      </div>
    );
  }

  if (status === 'soft_deleted') {
    return (
      <div className="mod-owner-banner mod-owner-banner--removed" role="alert">
        <AlertIcon size={16} />
        <span>{t('mod.banner.removed', { reason: reason || '' })}</span>
      </div>
    );
  }

  return null;
};

export default OwnerModerationBanner;