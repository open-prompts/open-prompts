import React from 'react';
import { useTranslation } from 'react-i18next';
import './RestoreCountdown.scss';

const RESTORE_WINDOW_DAYS = 30;

/**
 * "N days left" countdown for a soft-deleted template's restore window.
 * Computed from deleted_at; falls back to the full window when the timestamp
 * is missing.
 */
const RestoreCountdown = ({ deletedAt }) => {
  const { t } = useTranslation();
  let daysLeft = RESTORE_WINDOW_DAYS;
  if (deletedAt) {
    const deleted = new Date(deletedAt).getTime();
    const elapsed = (Date.now() - deleted) / (1000 * 60 * 60 * 24);
    daysLeft = Math.max(0, Math.ceil(RESTORE_WINDOW_DAYS - elapsed));
  }
  return (
    <span className="admin-restore-countdown">
      {t('mod.modal.daysLeft', { count: daysLeft })}
    </span>
  );
};

export default RestoreCountdown;