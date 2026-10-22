import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  listTemplateComments,
  updateComment,
  deleteComment,
} from '../../services/api';
import { useNotification } from '../../context/NotificationContext';
import CommentComposer from './CommentComposer';
import CommentItem from './CommentItem';
import CommentEditForm from './CommentEditForm';
import ReplyComposer from './ReplyComposer';
import RepliesBucket from './RepliesBucket';
import ReportMenu from '../ReportMenu';
import './CommentsSection.scss';

const PAGE_SIZE = 20;

/**
 * Template comment thread: paginated top-level list, composer for signed-in
 * users, inline edit (15-min window enforced server-side) and delete for
 * authors or the template owner. Thread-aware: one reply composer open at a
 * time, per-thread reply buckets with lazy fetch + cache. Page-scoped local
 * state, no Redux slice.
 */
const CommentsSection = ({ templateId, templateOwnerId, user }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();

  const [comments, setComments] = useState([]);
  const [page, setPage] = useState(1);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [editingId, setEditingId] = useState(null);
  const [openComposerId, setOpenComposerId] = useState(null);
  const [expandedThreads, setExpandedThreads] = useState({});
  const sectionRef = useRef(null);

  const load = useCallback(
    async (targetPage, append) => {
      setLoading(true);
      setError(null);
      try {
        const resp = await listTemplateComments(templateId, {
          page: targetPage,
          page_size: PAGE_SIZE,
        });
        const data = resp.data || {};
        setComments((prev) => (append ? [...prev, ...(data.comments || [])] : data.comments || []));
        setTotalCount(data.total_count || 0);
        setPage(targetPage);
      } catch (err) {
        setError('load');
      } finally {
        setLoading(false);
      }
    },
    [templateId]
  );

  useEffect(() => {
    load(1, false);
  }, [load]);

  const handlePosted = (comment) => {
    setComments((prev) => [comment, ...prev]);
    setTotalCount((prev) => prev + 1);
  };

  const handleReplyPosted = (commentId, reply) => {
    // Auto-expand the bucket and append the reply; bump reply_count locally.
    setExpandedThreads((prev) => ({
      ...prev,
      [commentId]: [...(prev[commentId] || []), reply],
    }));
    setComments((prev) =>
      prev.map((c) => (c.id === commentId ? { ...c, reply_count: (c.reply_count || 0) + 1 } : c))
    );
    setOpenComposerId(null);
    addNotification({ kind: 'success', title: t('comments.replyPosted') });
  };

  const handleEdit = async (commentId, content) => {
    try {
      const resp = await updateComment(commentId, content);
      const updated = resp.data;
      setComments((prev) => prev.map((c) => (c.id === commentId ? updated : c)));
      setEditingId(null);
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 403) {
        addNotification({
          kind: 'error',
          title: t('comments.error_window_expired'),
        });
        setEditingId(null);
      } else {
        addNotification({ kind: 'error', title: t('comments.error_edit') });
      }
    }
  };

  const handleDelete = async (comment) => {
    const isTemplateOwner = user && user.id === templateOwnerId;
    const confirmed = window.confirm(
      isTemplateOwner ? t('comments.confirm_delete_owner') : t('comments.confirm_delete')
    );
    if (!confirmed) return;
    try {
      await deleteComment(comment.id);
      setComments((prev) => prev.filter((c) => c.id !== comment.id));
      setTotalCount((prev) => Math.max(0, prev - 1));
    } catch (err) {
      addNotification({ kind: 'error', title: t('comments.error_delete') });
    }
  };

  const handleReplyDelete = async (parentId, reply) => {
    const isTemplateOwner = user && user.id === templateOwnerId;
    const confirmed = window.confirm(
      isTemplateOwner ? t('comments.confirm_delete_owner') : t('comments.confirm_delete')
    );
    if (!confirmed) return;
    try {
      await deleteComment(reply.id);
      setExpandedThreads((prev) => ({
        ...prev,
        [parentId]: (prev[parentId] || []).filter((r) => r.id !== reply.id),
      }));
      setComments((prev) =>
        prev.map((c) =>
          c.id === parentId ? { ...c, reply_count: Math.max(0, (c.reply_count || 0) - 1) } : c
        )
      );
    } catch (err) {
      addNotification({ kind: 'error', title: t('comments.error_delete') });
    }
  };

  const handleCacheReplies = useCallback((commentId, replies) => {
    setExpandedThreads((prev) => ({ ...prev, [commentId]: replies }));
  }, []);

  const handleToggleThread = useCallback((commentId) => {
    setExpandedThreads((prev) => ({ ...prev, [commentId]: !prev[commentId] }));
  }, []);

  const hasMore = comments.length < totalCount;

  return (
    <section className="comments-section" ref={sectionRef} aria-label={t('comments.title')}>
      <div className="comments-section__header">
        <h3>{t('comments.title')}</h3>
        <span className="comments-section__count">
          {t('comments.count', { count: totalCount })}
        </span>
      </div>

      {user ? (
        <CommentComposer templateId={templateId} onPosted={handlePosted} />
      ) : (
        <p className="comments-section__signin">
          <Link to="/login">{t('comments.sign_in_to_comment')}</Link>
        </p>
      )}

      {error === 'load' && (
        <div className="comments-section__error" role="alert">
          <span>{t('comments.error_load')}</span>
          <button className="comments-section__retry" onClick={() => load(1, false)}>
            {t('admin.common.retry')}
          </button>
        </div>
      )}

      {comments.length === 0 && !loading && error !== 'load' && (
        <p className="comments-section__empty">{t('comments.empty')}</p>
      )}

      <ul className="comments-section__list">
        {comments.map((comment) =>
          editingId === comment.id ? (
            <li key={comment.id}>
              <CommentEditForm
                comment={comment}
                onSave={(content) => handleEdit(comment.id, content)}
                onCancel={() => setEditingId(null)}
              />
            </li>
          ) : (
            <li key={comment.id}>
              <CommentItem
                comment={comment}
                canEdit={!!user && comment.is_own}
                canDelete={!!user && (comment.is_own || user.id === templateOwnerId)}
                onEdit={() => setEditingId(comment.id)}
                onDelete={() => handleDelete(comment)}
                onReply={user ? () => setOpenComposerId(comment.id) : null}
                isTopLevel
                templateOwnerId={templateOwnerId}
                user={user}
                reportMenu={
                  user && (
                    <ReportMenu
                      itemType="comment"
                      itemId={comment.id}
                      isOwner={!!user && comment.is_own}
                    />
                  )
                }
                repliesBucket={
                  <>
                    {openComposerId === comment.id && (
                      <ReplyComposer
                        templateId={templateId}
                        parentCommentId={comment.id}
                        parentName={(comment.author && comment.author.display_name) || ''}
                        onPosted={(reply) => handleReplyPosted(comment.id, reply)}
                        onCancel={() => setOpenComposerId(null)}
                      />
                    )}
                    {comment.reply_count > 0 && (
                      <RepliesBucket
                        commentId={comment.id}
                        replyCount={comment.reply_count}
                        templateOwnerId={templateOwnerId}
                        user={user}
                        expanded={!!expandedThreads[comment.id]}
                        cachedReplies={expandedThreads[comment.id]}
                        onToggle={handleToggleThread}
                        onCacheReplies={handleCacheReplies}
                        onEdit={(reply) => setEditingId(reply.id)}
                        onDelete={(reply) => handleReplyDelete(comment.id, reply)}
                      />
                    )}
                  </>
                }
              />
            </li>
          )
        )}
      </ul>

      {hasMore && (
        <button
          className="comments-section__load-more"
          onClick={() => load(page + 1, true)}
          disabled={loading}
        >
          {loading ? t('common.loading') : t('comments.load_more')}
        </button>
      )}
    </section>
  );
};

export default CommentsSection;
