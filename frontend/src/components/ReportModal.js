import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from './ui/Modal';
import { createReport } from '../services/api';
import './ReportModal.scss';

const REASONS = ['spam', 'offensive', 'misleading', 'privacy', 'other'];

/**
 * Report modal: five radio reasons with descriptors + an optional note
 * (<= 500 chars). Submit is disabled until a reason is chosen (FR-5).
 * On success the parent flips the menu to the reported state (FR-6/FR-7).
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {string} props.itemType - "template" | "comment"
 * @param {string} props.itemId
 * @param {string} props.initialReason - prefill when updating an existing report
 * @param {string} props.initialNote
 * @param {Function} props.onClose
 * @param {Function} props.onSuccess - called with the created report
 */
const ReportModal = ({ open, itemType, itemId, initialReason = '', initialNote = '', onClose, onSuccess }) => {
  const { t } = useTranslation();
  const [reason, setReason] = useState(initialReason);
  const [note, setNote] = useState(initialNote);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const titleKey = itemType === 'comment' ? 'reports.modal.title.comment' : 'reports.modal.title.template';
  const valid = reason !== '';

  const handleSubmit = async () => {
    if (!valid) return;
    setSubmitting(true);
    setError('');
    try {
      const resp = await createReport({
        item_type: itemType,
        item_id: itemId,
        reason,
        note: note.trim(),
      });
      onSuccess && onSuccess(resp.data);
      onClose && onClose();
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 429) {
        setError(t('reports.rateLimited'));
      } else {
        setError((err.response && err.response.data && err.response.data.error) || '');
      }
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      title={t(titleKey)}
      primaryLabel={t('reports.modal.submit')}
      secondaryLabel={t('reports.modal.cancel')}
      primaryDisabled={!valid || submitting}
      onPrimary={handleSubmit}
      onClose={onClose}
    >
      <div className="report-modal">
        <p className="report-modal__question">{t('reports.modal.question')}</p>
        <div className="report-modal__reasons" role="radiogroup" aria-label={t('reports.modal.question')}>
          {REASONS.map((r) => (
            <label key={r} className="report-modal__reason">
              <input
                type="radio"
                name="report-reason"
                value={r}
                checked={reason === r}
                onChange={() => setReason(r)}
              />
              <span className="report-modal__reason-label">{t(`reports.reason.${r}`)}</span>
            </label>
          ))}
        </div>
        <label className="report-modal__note-label" htmlFor="report-note">
          {t('reports.modal.noteLabel')}
        </label>
        <textarea
          id="report-note"
          className="report-modal__note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={2}
          maxLength={500}
          placeholder={t('reports.modal.noteLabel')}
        />
        <div className="report-modal__counter">{note.length} / 500</div>
        {error && <div className="report-modal__error" role="alert">{error}</div>}
      </div>
    </Modal>
  );
};

export default ReportModal;