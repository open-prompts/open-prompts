import React, { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { listCommentReplies } from '../../services/api';
import CommentItem from './CommentItem';
import './RepliesBucket.scss';

/**
 * Collapsed "View N replies" bucket under a top-level comment. On first
 * expansion it lazily fetches replies (oldest-first); subsequent expansions
 * render from the cached list passed down by the section. Inline error +
 * Retry is contained to the bucket only.
 */
const RepliesBucket = ({
  commentId,
  replyCount,
  templateOwnerId,
  user,
  expanded,
  cachedReplies,
  onToggle,
  onCacheReplies,
  onEdit,
  onDelete,
}) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const resp = await listCommentReplies(commentId);
      onCacheReplies(commentId, resp.data.replies || []);
    } catch (err) {
      setError(true);
    } finally {
      setLoading(false);
    }
  }, [commentId, onCacheReplies]);

  const handleToggle = () => {
    if (!expanded) {
      // Expand: fetch if not cached.
      if (!cachedReplies) {
        load();
      }
    }
    onToggle(commentId);
  };

  const replies = cachedReplies || [];

  return (
    <div className="replies-bucket">
      <button
        type="button"
        className="replies-bucket__toggle"
        onClick={handleToggle}
        aria-expanded={expanded}
        aria-controls={`replies-${commentId}`}
      >
        {expanded
          ? t('comments.hideReplies')
          : replyCount === 1
            ? t('comments.viewOneReply')
            : t('comments.viewReplies', { count: replyCount })}
        <span className="replies-bucket__chevron" aria-hidden="true">
          {expanded ? '▴' : '▾'}
        </span>
      </button>

      {expanded && (
        <div className="replies-bucket__body" id={`replies-${commentId}`}>
          {loading && (
            <div className="replies-bucket__loading" role="status">
              <span className="replies-bucket__shimmer" />
              <span className="replies-bucket__shimmer" />
            </div>
          )}
          {error && (
            <div className="replies-bucket__error" role="alert">
              <span>{t('comments.repliesError')}</span>
              <button type="button" className="replies-bucket__retry" onClick={load}>
                {t('admin.common.retry')}
              </button>
            </div>
          )}
          {!loading && !error && (
            <ul className="replies-bucket__list">
              {replies.map((reply) => (
                <li key={reply.id}>
                  <CommentItem
                    comment={reply}
                    canEdit={!!user && reply.is_own}
                    canDelete={!!user && (reply.is_own || user.id === templateOwnerId)}
                    onEdit={() => onEdit(reply)}
                    onDelete={() => onDelete(reply)}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
};

export default RepliesBucket;