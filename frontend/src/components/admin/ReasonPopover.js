import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import './ReasonPopover.scss';

/**
 * ReasonPopover is a small reason-required confirm popover for the report
 * review drawer's Dismiss / Act actions (FR-10). Requires a 3-500 char
 * reason before the action proceeds.
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {string} props.title - e.g. "Dismiss reports"
 * @param {string} props.confirmLabel
 * @param {boolean} props.danger
 * @param {Function} props.onConfirm - called with the trimmed reason
 * @param {Function} props.onClose
 * @param {boolean} props.busy
 */
const ReasonPopover = ({ open, title, confirmLabel, danger = false, onConfirm, onClose, busy = false }) => {
  const { t } = useTranslation();
  const [reason, setReason] = useState('');

  useEffect(() => {
    if (open) setReason('');
  }, [open]);

  if (!open) return null;

  const trimmed = reason.trim();
  const valid = trimmed.length >= 3 && trimmed.length <= 500;

  return (
    <div className="reason-popover" role="dialog" aria-label={title}>
      <p className="reason-popover__title">{title}</p>
      <textarea
        className="reason-popover__textarea"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        rows={3}
        maxLength={500}
        placeholder={t('admin.reports.reasonRequired')}
        aria-label={t('admin.reports.reasonRequired')}
        disabled={busy}
      />
      <div className="reason-popover__footer">
        {!valid && trimmed.length > 0 && (
          <span className="reason-popover__hint" aria-live="polite">
            {t('admin.reports.reasonRequired')}
          </span>
        )}
        <div className="reason-popover__actions">
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className={`btn ${danger ? 'btn-danger' : 'btn-primary'}`}
            onClick={() => onConfirm(trimmed)}
            disabled={!valid || busy}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

export default ReasonPopover;