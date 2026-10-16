import React, { useEffect, useRef, useState } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import FilterChips from '../../components/dashboard/FilterChips';
import PromptRow from '../../components/dashboard/PromptRow';
import PromptViewModal from '../../components/dashboard/PromptViewModal';
import { listPrompts, deletePrompt } from '../../services/api';
import { useNotification } from '../../context/NotificationContext';
import { usePageAnnouncement } from '../../utils/a11y';
import './PromptsPage.scss';

/**
 * PromptsPage — manage the caller's instantiated prompts, newest first.
 */
const PromptsPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token, user } = useSelector((state) => state.auth);
  const { addNotification } = useNotification();
  const [prompts, setPrompts] = useState([]);
  const [nextToken, setNextToken] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(false);
  const [filter, setFilter] = useState('all');
  const [viewing, setViewing] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const requestIdRef = useRef(0);

  usePageAnnouncement('dashboard_v2.nav.prompts');

  const fetchPage = async (pageToken, append) => {
    const requestId = ++requestIdRef.current;
    if (append) setLoadingMore(true);
    else setLoading(true);
    setError(false);
    try {
      const params = { page_size: 20, owner_id: user?.id };
      if (pageToken) params.page_token = pageToken;
      const resp = await listPrompts(params);
      if (requestId !== requestIdRef.current) return;
      const items = resp.data.prompts || [];
      if (append) setPrompts((prev) => [...prev, ...items]);
      else setPrompts(items);
      setNextToken(resp.data.next_page_token || '');
    } catch (err) {
      if (requestId !== requestIdRef.current) return;
      setError(true);
    } finally {
      if (requestId === requestIdRef.current) {
        setLoading(false);
        setLoadingMore(false);
      }
    }
  };

  useEffect(() => {
    if (!token) {
      navigate('/login?next=/dashboard/prompts');
      return undefined;
    }
    fetchPage('', false);
    return undefined;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, navigate, user?.id]);

  const sourceTemplates = [...new Set(prompts.map((p) => p.template_id))];
  const filterOptions = [
    { value: 'all', labelKey: 'dashboard_v2.prompts.filter' },
    ...sourceTemplates.map((tid) => {
      const p = prompts.find((x) => x.template_id === tid);
      return { value: tid, labelKey: p?.template_title || tid };
    }),
  ];

  const filtered = filter === 'all'
    ? prompts
    : prompts.filter((p) => p.template_id === filter);

  const hasMore = Boolean(nextToken);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deletePrompt(deleteTarget.id);
      setPrompts((list) => list.filter((x) => x.id !== deleteTarget.id));
      if (viewing && viewing.id === deleteTarget.id) setViewing(null);
      setDeleteTarget(null);
    } catch (err) {
      addNotification({ kind: 'error', title: t('common.error') });
    }
  };

  if (loading) {
    return <div className="dashboard-loading" role="status">{t('common.loading')}</div>;
  }

  if (error) {
    return (
      <div className="dashboard-error" role="alert">
        <p>{t('dashboard_v2.error.fetch')}</p>
        <button type="button" className="btn btn-primary" onClick={() => fetchPage('', false)}>
          {t('dashboard_v2.error.retry')}
        </button>
      </div>
    );
  }

  return (
    <div className="prompts-page">
      <h1 className="prompts-page__title">{t('dashboard_v2.nav.prompts')}</h1>
      <FilterChips options={filterOptions} value={filter} onChange={setFilter} />
      {filtered.length === 0 ? (
        <div className="prompts-page__empty">
          <p>{t('dashboard_v2.prompts.empty')}</p>
          <Link to="/" className="btn btn-primary">
            {t('dashboard_v2.prompts.empty_cta')}
          </Link>
        </div>
      ) : (
        <ul className="prompts-page__list">
          {filtered.map((p) => (
            <PromptRow
              key={p.id}
              prompt={p}
              onView={() => setViewing(p)}
              onDelete={() => setDeleteTarget(p)}
            />
          ))}
        </ul>
      )}
      {hasMore && (
        <button
          type="button"
          className="btn btn-ghost prompts-page__more"
          disabled={loadingMore}
          onClick={() => fetchPage(nextToken, true)}
        >
          {loadingMore ? t('common.loading') : t('common.more')}
        </button>
      )}

      {viewing && (
        <PromptViewModal
          prompt={viewing}
          onClose={() => setViewing(null)}
          onDelete={() => setDeleteTarget(viewing)}
        />
      )}

      {deleteTarget && (
        <div className="modal-overlay" role="dialog" aria-modal="true" aria-label={t('dashboard_v2.prompts.delete_confirm')}>
          <div className="modal">
            <p>{t('dashboard_v2.prompts.delete_confirm')}</p>
            <div className="modal-actions">
              <button type="button" className="btn btn-ghost" onClick={() => setDeleteTarget(null)}>
                {t('common.cancel')}
              </button>
              <button type="button" className="btn btn-danger" onClick={handleDelete}>
                {t('common.delete')}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default PromptsPage;