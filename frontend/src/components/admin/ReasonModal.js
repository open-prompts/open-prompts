import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import './ReasonModal.scss';

/**
 * Modal for admin mutations that require a mandatory reason (3–500 chars).
 * action: 'ban' | 'unban' | 'promote' | 'demote'
 */
const ReasonModal = ({ open, action, name, onConfirm, onClose, busy = false }) => {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');

  useEffect(() => {
    if (open) setReason('');
  }, [open]);

  const trimmed = reason.trim();
  const valid = trimmed.length >= 3 && trimmed.length <= 500;

  const titles = {
    ban: t('admin.reasonModal.ban_title', { name }),
    unban: t('admin.reasonModal.unban_title', { name }),
    promote: t('admin.reasonModal.promote_title', { name }),
    demote: t('admin.reasonModal.demote_title', { name }),
  };

  const confirms = {
    ban: t('admin.reasonModal.confirm_ban'),
    unban: t('admin.reasonModal.confirm_unban'),
    promote: t('admin.reasonModal.confirm_promote'),
    demote: t('admin.reasonModal.confirm_demote'),
  };

  const consequences = {
    ban: ['consequence_ban_1', 'consequence_ban_2', 'consequence_ban_3'],
    unban: ['consequence_unban_1', 'consequence_unban_2'],
    promote: ['consequence_role_1'],
    demote: ['consequence_role_1'],
  };

  return (
    <Modal
      open={open}
      title={titles[action] || ''}
      onClose={onClose}
      primaryLabel={confirms[action] || ''}
      primaryDisabled={!valid || busy}
      danger={action === 'ban'}
      onPrimary={() => onConfirm(trimmed)}
    >
      <div className="admin-reason-modal">
        <ul className="admin-reason-modal__consequences">
          {(consequences[action] || []).map((key) => (
            <li key={key}>{t(`admin.reasonModal.${key}`)}</li>
          ))}
        </ul>
        <label className="admin-reason-modal__label" htmlFor="admin-reason-input">
          {t('admin.reasonModal.reason_label')}
        </label>
        <textarea
          id="admin-reason-input"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          maxLength={500}
          rows={3}
          placeholder={t('admin.reasonModal.reason_required')}
          disabled={busy}
        />
        <div className="admin-reason-modal__footer">
          {!valid && trimmed.length > 0 && (
            <span className="admin-reason-modal__hint">
              {t('admin.reasonModal.reason_required')}
            </span>
          )}
          <span className="admin-reason-modal__count">
            {t('admin.reasonModal.char_count', { count: reason.length })}
          </span>
        </div>
      </div>
    </Modal>
  );
};

export default ReasonModal;
