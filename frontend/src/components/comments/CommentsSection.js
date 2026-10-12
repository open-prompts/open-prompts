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
import './CommentsSection.scss';

const PAGE_SIZE = 20;

/**
 * Template comment thread: paginated list, composer for signed-in users,
 * inline edit (15-min window enforced server-side) and delete for authors
 * or the template owner. Page-scoped local state, no Redux slice.
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
