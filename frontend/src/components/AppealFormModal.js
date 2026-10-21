import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from './ui/Modal';
import Field from './ui/Field';
import { createAppeal } from '../services/api';
import './AppealFormModal.scss';

/**
 * Appeal form modal: read-only action context + statement textarea
 * (50-2000 chars) with a live counter. On a 409 conflict the form is
 * replaced by the existing appeal's status card (FR-3).
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {Object} props.action - { id, action_type, reason, actor_display_name, created_at, template_title }
 * @param {Function} props.onClose
 * @param {Function} props.onSuccess - called with the created appeal
 */
const AppealFormModal = ({ open, action, onClose, onSuccess }) => {
  const { t } = useTranslation();
  const [statement, setStatement] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [conflict, setConflict] = useState(null);

  const len = statement.length;
  const tooShort = len < 50;
  const tooLong = len > 2000;
  const invalid = tooShort || tooLong;

  const handleSubmit = async () => {
    if (invalid || !action) return;
    setSubmitting(true);
    setError('');
    try {
      const resp = await createAppeal({
        moderation_action_id: action.id,
        statement: statement.trim(),
      });
      onSuccess && onSuccess(resp.data);
      onClose && onClose();
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 409) {
        setConflict(true);
      } else {
        setError((err.response && err.response.data && err.response.data.error) || '');
      }
      setSubmitting(false);
    }
  };

  const fieldError = tooShort
    ? t('appeal.form.tooShort')
    : tooLong
    ? t('appeal.form.tooLong')
    : '';

  return (
    <Modal
      open={open}
      title={t('appeal.form.title')}
      primaryLabel={t('appeal.form.submit')}
      secondaryLabel={t('common.cancel')}
      primaryDisabled={invalid || submitting}
      onPrimary={handleSubmit}
      onClose={onClose}
    >
      {conflict ? (
        <div className="appeal-form__conflict" role="status">
          <p>{t('appeal.form.conflict')}</p>
        </div>
      ) : (
        <div className="appeal-form">
          <div className="appeal-form__context">
            <span className="appeal-form__context-label">{t('appeal.form.actionContext')}</span>
            <span className="appeal-form__context-value">
              {action && action.action_type} · {action && action.reason}
            </span>
            <span className="appeal-form__context-meta">
              {action && action.actor_display_name} · {action && action.template_title}
            </span>
          </div>
          <Field
            id="appeal-statement"
            label={t('appeal.form.statement')}
            hint={t('appeal.form.statementHint')}
            error={fieldError}
          >
            <textarea
              className="appeal-form__textarea"
              value={statement}
              onChange={(e) => setStatement(e.target.value)}
              rows={6}
              maxLength={2000}
              aria-describedby="appeal-statement-count"
            />
          </Field>
          <div className="appeal-form__counter" id="appeal-statement-count">
            {len} / 2000
          </div>
          {error && <div className="appeal-form__error" role="alert">{error}</div>}
        </div>
      )}
    </Modal>
  );
};

export default AppealFormModal;