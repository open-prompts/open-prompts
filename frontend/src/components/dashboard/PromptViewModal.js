import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';

/**
 * PromptViewModal — server-rendered prompt content viewer.
 * @param {Object} props
 * @param {Object} props.prompt - the Prompt to display
 * @param {Function} props.onClose - () => void
 * @param {Function} props.onDelete - () => void
 */
const PromptViewModal = ({ prompt, onClose, onDelete }) => {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(prompt.content || '');
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (err) {
      // clipboard unavailable
    }
  };

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-label={t('dashboard_v2.prompts.modal_title', { title: prompt.template_title || '' })}
    >
      <div className="modal prompt-view-modal">
        <h2 className="prompt-view-modal__title">
          {t('dashboard_v2.prompts.modal_title', { title: prompt.template_title || '' })}
        </h2>
        <pre className="prompt-content" role="region" aria-label={t('dashboard_v2.prompts.modal_title', { title: prompt.template_title || '' })}>
          {prompt.content}
        </pre>
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={handleCopy}>
            {copied ? t('common.copied') : t('dashboard_v2.prompts.copy')}
          </button>
          <button type="button" className="btn btn-danger" onClick={onDelete}>
            {t('common.delete')}
          </button>
          <button type="button" className="btn btn-primary" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>
    </div>
  );
};

export default PromptViewModal;