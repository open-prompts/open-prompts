import React from 'react';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import './PublishReviewModal.scss';

/**
 * Confirmation modal shown to gated creators right after a publish that
 * entered pre-publication review (instead of the "live" feedback).
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {Function} props.onViewStatus - "View status" action
 * @param {Function} props.onDone - "Done" / close action
 */
const PublishReviewModal = ({ open, onViewStatus, onDone }) => {
  const { t } = useTranslation();

  return (
    <Modal
      open={open}
      title={t('review.modal.title')}
      primaryLabel={t('review.modal.view_status')}
      secondaryLabel={t('review.modal.done')}
      onPrimary={onViewStatus}
      onClose={onDone}
      className="publish-review-modal"
    >
      <div className="publish-review-modal__body">
        <p>{t('review.modal.body')}</p>
      </div>
    </Modal>
  );
};

export default PublishReviewModal;
