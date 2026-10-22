import React, { useEffect, useState, useCallback } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminComments, deleteAdminComment } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import AdminSearchInput from '../../components/admin/AdminSearchInput';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import Modal from '../../components/ui/Modal';
import { formatRelativeTime } from '../../utils/adminFormat';
import './CommentsPage.scss';

/**
 * /admin/comments — platform-wide comment moderation. Shows a parent column
 * ("—" for top-level, "in reply to {author}" for replies) and supports
 * delete-with-reason (audited server-side).
 */
const CommentsPage = () => {
  const { t, i18n } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();

  const q = searchParams.get('q') || '';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = 20;

  const [data, setData] = useState({ entries: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [reason, setReason] = useState('');
  const [deleting, setDeleting] = useState(false);

  const lang = i18n.language || 'en';

  const setParam = (updates, resetPage = true) => {
    const next = new URLSearchParams(searchParams);
    Object.entries(updates).forEach(([k, v]) => {
      if (v === '' || v === null || v === undefined) next.delete(k);
      else next.set(k, String(v));
    });
    if (resetPage) next.delete('page');
    setSearchParams(next);
  };

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const params = { page, per_page: perPage };
      if (q) params.q = q;
      const resp = await getAdminComments(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [q, page]);

  useEffect(() => {
    load();
  }, [load]);

  const openDelete = (entry) => {
    setDeleteTarget(entry);
    setReason('');
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteAdminComment(deleteTarget.id, reason.trim());
      setDeleteTarget(null);
      load();
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setDeleting(false);
    }
  };

  const reasonValid = reason.trim().length >= 3 && reason.trim().length <= 500;

  const columns = [
    {
      key: 'content',
      header: t('admin.comments.content', { defaultValue: 'Content' }),
      render: (item) => <span className="admin-comments__content">{item.content}</span>,
    },
    {
      key: 'parent',
      header: t('admin.comments.parent'),
      render: (item) =>
        item.parent_author_display_name
          ? t('admin.comments.inReplyTo', { author: item.parent_author_display_name })
          : t('admin.comments.topLevel'),
    },
    {
      key: 'author',
      header: t('admin.comments.author', { defaultValue: 'Author' }),
      render: (item) => <span>{item.user_display_name || item.user_id}</span>,
    },
    {
      key: 'template',
      header: t('admin.comments.template', { defaultValue: 'Template' }),
      render: (item) => <span className="admin-comments__template">{item.template_title}</span>,
    },
    {
      key: 'posted',
      header: t('admin.comments.posted', { defaultValue: 'Posted' }),
      render: (item) => <span>{formatRelativeTime(item.created_at, lang)}</span>,
    },
    {
      key: 'actions',
      header: '',
      render: (item) => (
        <button
          type="button"
          className="admin-comments__delete"
          onClick={(e) => {
            e.stopPropagation();
            openDelete(item);
          }}
        >
          {t('admin.comments.delete', { defaultValue: 'Delete' })}
        </button>
      ),
    },
  ];

  return (
    <div className="admin-comments">
      <h1 className="admin-comments__title">{t('admin.comments.title', { defaultValue: 'Comments' })}</h1>
      <div className="admin-comments__toolbar">
        <AdminSearchInput
          value={q}
          onChange={(v) => setParam({ q: v })}
          placeholder={t('admin.comments.search_placeholder', { defaultValue: 'Search comments…' })}
        />
      </div>
      {error && <ErrorBanner message={error} onRetry={load} />}
      <AdminTable
        columns={columns}
        rows={data.entries || []}
        loading={loading}
        rowKey={(row) => row.id}
        emptyState={<EmptyState title={t('admin.comments.empty', { defaultValue: 'No comments found' })} />}
      />
      <Pagination page={page} total={data.total || 0} perPage={perPage} onPageChange={(p) => setParam({ page: p }, false)} />

      <Modal
        open={!!deleteTarget}
        title={t('admin.comments.delete_title', { defaultValue: 'Delete comment' })}
        primaryLabel={t('admin.comments.confirm_delete', { defaultValue: 'Delete' })}
        secondaryLabel={t('admin.common.cancel', { defaultValue: 'Cancel' })}
        primaryDisabled={!reasonValid || deleting}
        danger
        onPrimary={confirmDelete}
        onClose={() => setDeleteTarget(null)}
      >
        <div className="admin-comments__delete-modal">
          <p className="admin-comments__delete-preview">
            {deleteTarget ? deleteTarget.content : ''}
          </p>
          <label className="admin-comments__reason-label" htmlFor="admin-comment-reason">
            {t('admin.comments.reason_label', { defaultValue: 'Reason (required)' })}
          </label>
          <textarea
            id="admin-comment-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={500}
            rows={3}
            placeholder={t('admin.comments.reason_required', { defaultValue: 'Reason must be 3-500 characters' })}
            disabled={deleting}
          />
        </div>
      </Modal>
    </div>
  );
};

export default CommentsPage;