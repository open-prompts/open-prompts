import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import './CommentEditForm.scss';

const MAX_LEN = 2000;

/**
 * Inline edit form for a comment: prefilled textarea with Save/Cancel.
 * The 15-minute edit window is enforced server-side; a 403 exits edit mode
 * with a toast (handled by the parent).
 */
const CommentEditForm = ({ comment, onSave, onCancel }) => {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(comment.content || '');
  const [saving, setSaving] = useState(false);

  const trimmed = draft.trim();
  const valid = trimmed.length >= 1 && trimmed.length <= MAX_LEN;

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!valid || saving) return;
    setSaving(true);
    // Parent resets editingId on success/failure; keep local saving flag
    // only for the in-flight duration.
    Promise.resolve(onSave(draft)).finally(() => setSaving(false));
  };

  return (
    <form className="comment-edit-form" onSubmit={handleSubmit}>
      <label className="comment-edit-form__label" htmlFor={`edit-${comment.id}`}>
        {t('comments.edit')}
      </label>
      <textarea
        id={`edit-${comment.id}`}
        className="comment-edit-form__input"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        maxLength={MAX_LEN}
        rows={3}
        disabled={saving}
      />
      <div className="comment-edit-form__footer">
        <span className="comment-edit-form__count" aria-live="polite">
          {t('comments.char_count', { count: draft.length })}
        </span>
        <div className="comment-edit-form__buttons">
          <button type="button" className="comment-edit-form__cancel" onClick={onCancel} disabled={saving}>
            {t('comments.cancel')}
          </button>
          <button type="submit" className="comment-edit-form__save" disabled={!valid || saving}>
            {saving ? t('comments.saving') : t('comments.save')}
          </button>
        </div>
      </div>
    </form>
  );
};

export default CommentEditForm;
