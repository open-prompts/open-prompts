import React, { useState, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { createTemplateComment } from '../../services/api';
import { useNotification } from '../../context/NotificationContext';
import './ReplyComposer.scss';

const MAX_LEN = 2000;

/**
 * Compact reply composer shown inline under a top-level comment. Posts with
 * parent_comment_id; on success auto-expands the bucket and appends the reply.
 * On 400 thread-full shows the inline localized message and preserves the
 * draft; on 429 surfaces the existing rate-limit error.
 */
const ReplyComposer = ({ templateId, parentCommentId, parentName, onPosted, onCancel }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();

  const [draft, setDraft] = useState('');
  const [posting, setPosting] = useState(false);
  const [fieldError, setFieldError] = useState('');
  const textareaRef = useRef(null);

  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.focus();
    }
  }, []);

  const trimmed = draft.trim();
  const valid = trimmed.length >= 1 && trimmed.length <= MAX_LEN;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!valid || posting) return;
    setPosting(true);
    setFieldError('');
    try {
      const resp = await createTemplateComment(templateId, draft, parentCommentId);
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
        setFieldError(t('comments.threadFull'));
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
    <form className="reply-composer" onSubmit={handleSubmit}>
      <label className="reply-composer__label" htmlFor={`reply-composer-${parentCommentId}`}>
        {t('comments.replyTo', { name: parentName })}
      </label>
      <textarea
        id={`reply-composer-${parentCommentId}`}
        ref={textareaRef}
        className="reply-composer__input"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder={t('comments.replyTo', { name: parentName })}
        maxLength={MAX_LEN}
        rows={2}
        disabled={posting}
        aria-invalid={!!fieldError}
      />
      {fieldError && (
        <p className="reply-composer__error" role="alert">
          {fieldError}
        </p>
      )}
      <div className="reply-composer__footer">
        <span className="reply-composer__count" aria-live="polite">
          {t('comments.char_count', { count: draft.length })}
        </span>
        <div className="reply-composer__actions">
          <button
            type="button"
            className="reply-composer__cancel"
            onClick={onCancel}
            disabled={posting}
          >
            {t('comments.cancel')}
          </button>
          <button
            type="submit"
            className="reply-composer__submit"
            disabled={!valid || posting}
          >
            {posting ? t('comments.posting') : t('comments.reply')}
          </button>
        </div>
      </div>
    </form>
  );
};

export default ReplyComposer;