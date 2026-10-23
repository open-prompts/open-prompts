import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import './ActionReasonModal.scss';

const CONSEQUENCES = {
  unpublish: ['public', 'ownerKeeps', 'notified', 'reversible'],
  lock: ['frozen', 'notified', 'reversible'],
  delete: ['window', 'notified'],
  restore: ['reversible'],
};

/**
 * Modal for admin moderation actions that require a mandatory reason
 * (3–500 chars) plus a consequence checklist. Delete additionally requires
 * typing the template title (type-to-confirm, matching account-deletion).
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {'unpublish'|'lock'|'delete'|'restore'} props.action
 * @param {string} props.templateTitle
 * @param {Function} props.onConfirm - called with the trimmed reason
 * @param {Function} props.onClose
 * @param {boolean} [props.busy]
 */
const ActionReasonModal = ({ open, action, templateTitle, onConfirm, onClose, busy = false }) => {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');
  const [confirmTitle, setConfirmTitle] = useState('');

  useEffect(() => {
    if (open) {
      setReason('');
      setConfirmTitle('');
    }
  }, [open]);

  const trimmed = reason.trim();
  const reasonValid = trimmed.length >= 3 && trimmed.length <= 500;
  const needsConfirm = action === 'delete';
  const confirmValid = !needsConfirm || confirmTitle === templateTitle;
  const valid = reasonValid && confirmValid;

  const title = t(`mod.modal.${action}.title`);
  const confirmLabel = t(`mod.action.${action}`);

  return (
    <Modal
      open={open}
      title={title}
      onClose={onClose}
      primaryLabel={confirmLabel}
      primaryDisabled={!valid || busy}
      danger={action === 'delete'}
      onPrimary={() => onConfirm(trimmed)}
    >
      <div className="admin-action-reason-modal">
        <ul className="admin-action-reason-modal__consequences">
          {(CONSEQUENCES[action] || []).map((key) => (
            <li key={key}>{t(`mod.modal.consequence.${key}`)}</li>
          ))}
        </ul>
        <label className="admin-action-reason-modal__label" htmlFor="mod-reason-input">
          {t('mod.modal.reason')}
        </label>
        <textarea
          id="mod-reason-input"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          maxLength={500}
          rows={3}
          disabled={busy}
        />
        {needsConfirm && (
          <>
            <label className="admin-action-reason-modal__label" htmlFor="mod-confirm-input">
              {t('mod.modal.typeTitle')}
            </label>
            <input
              id="mod-confirm-input"
              type="text"
              value={confirmTitle}
              onChange={(e) => setConfirmTitle(e.target.value)}
              placeholder={templateTitle}
              disabled={busy}
            />
          </>
        )}
        <div className="admin-action-reason-modal__footer">
          {!reasonValid && trimmed.length > 0 && (
            <span className="admin-action-reason-modal__hint">{t('mod.modal.reason')}</span>
          )}
          <span className="admin-action-reason-modal__count">
            {t('admin.reasonModal.char_count', { count: reason.length })}
          </span>
        </div>
      </div>
    </Modal>
  );
};

export default ActionReasonModal;