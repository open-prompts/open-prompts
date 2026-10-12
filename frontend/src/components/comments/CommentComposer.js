import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { createTemplateComment } from '../../services/api';
import { useNotification } from '../../context/NotificationContext';
import './CommentComposer.scss';

const MAX_LEN = 2000;

/**
 * Comment composer: labeled textarea with live character counter. The draft
 * is preserved on failure (NFR-8); 429 surfaces the retry window.
 */
const CommentComposer = ({ templateId, onPosted }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();

  const [draft, setDraft] = useState('');
  const [posting, setPosting] = useState(false);
  const [fieldError, setFieldError] = useState('');

  const trimmed = draft.trim();
  const valid = trimmed.length >= 1 && trimmed.length <= MAX_LEN;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!valid || posting) return;
    setPosting(true);
    setFieldError('');
    try {
      const resp = await createTemplateComment(templateId, draft);
      onPosted(resp.data);
      setDraft('');
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 429) {
        const retryAfter = err.response && err.response.headers && err.response.headers['retry-after'];
        const seconds = parseInt(retryAfter, 10);
        addNotification({
          kind: 'error',
          title: t('comments.error_rate_limited', {
            seconds: Number.isFinite(seconds) ? seconds : 60,
          }),
        });
      } else if (status === 400) {
        setFieldError(t('comments.error_content_length'));
      } else if (status === 403) {
        addNotification({ kind: 'error', title: t('comments.error_banned') });
      } else {
        addNotification({ kind: 'error', title: t('comments.error_post') });
      }
    } finally {
      setPosting(false);
    }
  };

  return (
    <form className="comment-composer" onSubmit={handleSubmit}>
      <label className="comment-composer__label" htmlFor="comment-composer-input">
        {t('comments.composer_label')}
      </label>
      <textarea
        id="comment-composer-input"
        className="comment-composer__input"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder={t('comments.placeholder')}
        maxLength={MAX_LEN}
        rows={3}
        disabled={posting}
        aria-invalid={!!fieldError}
      />
      {fieldError && (
        <p className="comment-composer__error" role="alert">
          {fieldError}
        </p>
      )}
      <div className="comment-composer__footer">
        <span className="comment-composer__count" aria-live="polite">
          {t('comments.char_count', { count: draft.length })}
        </span>
        <button
          type="submit"
          className="comment-composer__submit"
          disabled={!valid || posting}
        >
          {posting ? t('comments.posting') : t('comments.post')}
        </button>
      </div>
    </form>
  );
};

export default CommentComposer;
