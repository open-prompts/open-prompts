import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import './PublishToggle.scss';

/**
 * PublishToggle — a Publish/Unpublish button with a confirm modal.
 *
 * @param {Object} props
 * @param {boolean} props.isPublic - Current visibility
 * @param {Function} props.onToggle - Called with the new visibility on confirm
 * @param {boolean} [props.disabled] - Disable the button
 */
const PublishToggle = ({ isPublic, onToggle, disabled = false }) => {
  const { t } = useTranslation();
  const [confirmOpen, setConfirmOpen] = useState(false);

  const label = isPublic ? t('collections.unpublish') : t('collections.publish');
  const confirmText = isPublic ? t('collections.unpublish_confirm') : t('collections.publish_confirm');

  const handleConfirm = () => {
    setConfirmOpen(false);
    onToggle(isPublic ? 'private' : 'public');
  };

  return (
    <>
      <button
        type="button"
        className="publish-toggle"
        onClick={() => setConfirmOpen(true)}
        disabled={disabled}
      >
        {label}
      </button>
      <Modal
        open={confirmOpen}
        title={label}
        primaryLabel={t('common.confirmation')}
        secondaryLabel={t('common.cancel')}
        onPrimary={handleConfirm}
        onClose={() => setConfirmOpen(false)}
      >
        <p>{confirmText}</p>
      </Modal>
    </>
  );
};

export default PublishToggle;