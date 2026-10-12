import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Optional change-note input for template version saves.
 * maxlength 200 (server-enforced), live counter, warning color at 180+.
 */
const ChangeNoteInput = ({ value, onChange, disabled = false }) => {
  const { t } = useTranslation();
  const count = (value || '').length;
  const nearLimit = count >= 180;

  return (
    <div className="change-note-input">
      <label htmlFor="change-note">{t('versioning.note_label')}</label>
      <input
        id="change-note"
        type="text"
        maxLength={200}
        value={value || ''}
        disabled={disabled}
        placeholder={t('versioning.note_placeholder')}
        onChange={(e) => onChange(e.target.value)}
      />
      <span
        className={`change-note-counter ${nearLimit ? 'warning' : ''}`}
        aria-live="polite"
      >
        {t('versioning.note_counter', { count })}
      </span>
    </div>
  );
};

export default ChangeNoteInput;
