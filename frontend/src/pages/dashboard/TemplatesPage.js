import React, { useEffect, useRef, useState } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import FilterChips from '../../components/dashboard/FilterChips';
import TemplateManageRow from '../../components/dashboard/TemplateManageRow';
import { getTemplates, updateTemplate, deleteTemplate } from '../../services/api';
import { useNotification } from '../../context/NotificationContext';
import { usePageAnnouncement } from '../../utils/a11y';
import './TemplatesPage.scss';

const FILTERS = [
  { value: 'all', labelKey: 'dashboard_v2.templates.filter.all' },
  { value: 'public', labelKey: 'dashboard_v2.templates.filter.public' },
  { value: 'private', labelKey: 'dashboard_v2.templates.filter.private' },
  { value: 'system', labelKey: 'dashboard_v2.templates.filter.system' },
  { value: 'user', labelKey: 'dashboard_v2.templates.filter.user' },
];

/**
 * TemplatesPage — manage the caller's owned templates, sorted by updated_at
 * desc, merging the public + private streams exactly like Home.
 */
const TemplatesPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token, user } = useSelector((state) => state.auth);
  const { addNotification } = useNotification();
  const [filter, setFilter] = useState('all');
  const [templates, setTemplates] = useState([]);
  const [privateTemplates, setPrivateTemplates] = useState([]);
  const [nextToken, setNextToken] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const requestIdRef = useRef(0);

  usePageAnnouncement('dashboard_v2.nav.templates');

  const fetchPage = async (pageToken, append) => {
    const requestId = ++requestIdRef.current;
    if (append) setLoadingMore(true);
    else setLoading(true);
    setError(false);
    try {
      const params = { page_size: 20, sort: 'updated' };
      if (pageToken) params.page_token = pageToken;
      const resp = await getTemplates(params);
      if (requestId !== requestIdRef.current) return;
      const pub = resp.data.templates || [];
      const priv = resp.data.private_templates || [];
      if (append) {
        setTemplates((prev) => [...prev, ...pub]);
        setPrivateTemplates((prev) => [...prev, ...priv]);
      } else {
        setTemplates(pub);
        setPrivateTemplates(priv);
      }
      const next = resp.data.private_next_page_token !== undefined
        ? `${resp.data.next_page_token || ''}:${resp.data.private_next_page_token || ''}`
        : (resp.data.next_page_token || '');
      setNextToken(next);
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
      navigate('/login?next=/dashboard/templates');
      return undefined;
    }
    fetchPage('', false);
    return undefined;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, navigate]);

  const merged = [...templates, ...privateTemplates]
    .sort((a, b) => new Date(b.updated_at || b.updatedAt) - new Date(a.updated_at || a.updatedAt));

  const filtered = merged.filter((tpl) => {
    if (filter === 'all') return true;
    if (filter === 'public') return tpl.visibility === 'VISIBILITY_PUBLIC';
    if (filter === 'private') return tpl.visibility === 'VISIBILITY_PRIVATE';
    if (filter === 'system') return tpl.type === 'TEMPLATE_TYPE_SYSTEM';
    if (filter === 'user') return tpl.type === 'TEMPLATE_TYPE_USER';
    return true;
  });

  const hasMore = Boolean(nextToken) && nextToken !== ':';

  const handleToggleVisibility = async (tpl) => {
    const nextVisibility = tpl.visibility === 'VISIBILITY_PUBLIC' ? 'VISIBILITY_PRIVATE' : 'VISIBILITY_PUBLIC';
    const prev = tpl.visibility;
    // optimistic
    setTemplates((list) => list.map((x) => (x.id === tpl.id ? { ...x, visibility: nextVisibility } : x)));
    setPrivateTemplates((list) => list.map((x) => (x.id === tpl.id ? { ...x, visibility: nextVisibility } : x)));
    try {
      await updateTemplate(tpl.id, {
        template_id: tpl.id,
        owner_id: user?.id,
        title: tpl.title,
        description: tpl.description || '',
        visibility: nextVisibility,
        category: tpl.category || '',
        tags: tpl.tags || [],
        content: tpl.latest_version?.content || '',
      });
    } catch (err) {
      // rollback
      setTemplates((list) => list.map((x) => (x.id === tpl.id ? { ...x, visibility: prev } : x)));
      setPrivateTemplates((list) => list.map((x) => (x.id === tpl.id ? { ...x, visibility: prev } : x)));
      addNotification({ kind: 'error', title: t('common.error') });
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteTemplate(deleteTarget.id);
      setTemplates((list) => list.filter((x) => x.id !== deleteTarget.id));
      setPrivateTemplates((list) => list.filter((x) => x.id !== deleteTarget.id));
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
    <div className="templates-page">
      <h1 className="templates-page__title">{t('dashboard_v2.nav.templates')}</h1>
      <FilterChips options={FILTERS} value={filter} onChange={setFilter} />
      {filtered.length === 0 ? (
        <div className="templates-page__empty">
          <p>{t('dashboard_v2.templates.empty')}</p>
          <Link to="/dashboard" className="btn btn-primary">
            {t('dashboard_v2.templates.empty_cta')}
          </Link>
        </div>
      ) : (
        <ul className="templates-page__list">
          {filtered.map((tpl) => (
            <TemplateManageRow
              key={tpl.id}
              template={tpl}
              onToggleVisibility={() => handleToggleVisibility(tpl)}
              onDelete={() => setDeleteTarget(tpl)}
            />
          ))}
        </ul>
      )}
      {hasMore && (
        <button
          type="button"
          className="btn btn-ghost templates-page__more"
          disabled={loadingMore}
          onClick={() => fetchPage(nextToken, true)}
        >
          {loadingMore ? t('common.loading') : t('common.more')}
        </button>
      )}

      {deleteTarget && (
        <div className="modal-overlay" role="dialog" aria-modal="true" aria-label={t('dashboard_v2.templates.delete_confirm', { title: deleteTarget.title })}>
          <div className="modal">
            <p>{t('dashboard_v2.templates.delete_confirm', { title: deleteTarget.title })}</p>
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

export default TemplatesPage;