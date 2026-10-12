import React from 'react';
import { useTranslation } from 'react-i18next';
import Modal from './ui/Modal';

/**
 * Confirmation modal for non-destructive version restore.
 * Restoring appends a new version with the canonical note
 * "Restored from v{n}"; nothing is deleted.
 */
const RestoreConfirmModal = ({ open, version, nextVersion, inFlight, onCancel, onConfirm }) => {
  const { t } = useTranslation();
  if (!open || !version) return null;

  return (
    <Modal
      open={open}
      title={t('versioning.restore_title')}
      subtitle={t('common.confirmation')}
      primaryLabel={t('versioning.restore_confirm', { version: version.version })}
      primaryDisabled={inFlight}
      secondaryLabel={t('common.cancel')}
      onClose={inFlight ? undefined : onCancel}
      onPrimary={onConfirm}
    >
      <p className="modal-confirm-text">
        {t('versioning.restore_body', { version: version.version, next: nextVersion })}
      </p>
    </Modal>
  );
};

export default RestoreConfirmModal;
