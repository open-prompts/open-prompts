import React from 'react';
import { useTranslation } from 'react-i18next';
import { useRelativeTime } from '../../utils/time';
import './CommentItem.scss';

/**
 * A single comment row: author identity, [CREATOR] chip for the template
 * owner, relative time, edited marker, plain-text content (pre-wrap), and
 * hover/focus-revealed Edit/Delete actions. Top-level comments additionally
 * render a Reply action, an "Answered" chip when the creator has replied, and
 * a RepliesBucket when reply_count > 0.
 */
const CommentItem = ({
  comment,
  canEdit,
  canDelete,
  onEdit,
  onDelete,
  onReply,
  isTopLevel = false,
  templateOwnerId,
  user,
  repliesBucket,
  reportMenu,
}) => {
  const { t } = useTranslation();
  const author = comment.author || {};
  const time = useRelativeTime(comment.created_at);

  return (
    <article className="comment-item">
      {author.avatar ? (
        <img className="comment-item__avatar" src={author.avatar} alt={t('common.avatar_alt')} />
      ) : (
        <span className="comment-item__avatar comment-item__avatar--placeholder" aria-hidden="true">
          {(author.display_name || '?').charAt(0).toUpperCase()}
        </span>
      )}
      <div className="comment-item__body">
        <div className="comment-item__meta">
          <span className="comment-item__author">{author.display_name || t('common.user_fallback')}</span>
          {comment.is_owner && <span className="comment-item__badge">{t('comments.creator_badge')}</span>}
          <time className="comment-item__time" dateTime={comment.created_at}>
            {time}
          </time>
          {comment.is_edited && <span className="comment-item__edited">({t('comments.edited')})</span>}
          {isTopLevel && comment.has_creator_reply && (
            <span className="comment-item__answered">{t('comments.answered')}</span>
          )}
        </div>
        <p className="comment-item__content">{comment.content}</p>
        <div className="comment-item__actions">
          {isTopLevel && onReply && (
            <button type="button" className="comment-item__action" onClick={onReply}>
              {t('comments.reply')}
            </button>
          )}
          {canEdit && (
            <button type="button" className="comment-item__action" onClick={onEdit}>
              {t('comments.edit')}
            </button>
          )}
          {canDelete && (
            <button type="button" className="comment-item__action comment-item__action--danger" onClick={onDelete}>
              {t('comments.delete')}
            </button>
          )}
          {reportMenu}
        </div>
        {isTopLevel && repliesBucket}
      </div>
    </article>
  );
};

export default CommentItem;
