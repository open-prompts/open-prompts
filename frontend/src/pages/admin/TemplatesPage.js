import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminTemplates } from '../../services/adminApi';
import AdminTable from '../../components/admin/AdminTable';
import AdminSearchInput from '../../components/admin/AdminSearchInput';
import AdminSelect from '../../components/admin/AdminSelect';
import Badge from '../../components/admin/Badge';
import Pagination from '../../components/admin/Pagination';
import EmptyState from '../../components/admin/EmptyState';
import ErrorBanner from '../../components/admin/ErrorBanner';
import StatusChip from '../../components/admin/StatusChip';
import RestoreCountdown from '../../components/admin/RestoreCountdown';
import { formatRelativeTime, formatNumber } from '../../utils/adminFormat';
import './TemplatesPage.scss';

/**
 * Admin template moderation list: includes private templates, supports
 * owner deep-link filter from the user detail page.
 */
const TemplatesPage = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const q = searchParams.get('q') || '';
  const visibility = searchParams.get('visibility') || '';
  const ownerId = searchParams.get('owner_id') || '';
  const sort = searchParams.get('sort') || 'recently_updated';
  const warning = searchParams.get('warning') || '';
  const moderationStatus = searchParams.get('moderation_status') || '';
  const page = Math.max(1, parseInt(searchParams.get('page') || '1', 10) || 1);
  const perPage = Math.max(1, parseInt(searchParams.get('per_page') || '20', 10) || 20);

  const [data, setData] = useState({ templates: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

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
      const params = { page, per_page: perPage, sort };
      if (q) params.search = q;
      if (visibility) params.visibility = visibility;
      if (ownerId) params.owner_id = ownerId;
      if (warning) params.warning = warning;
      if (moderationStatus) params.moderation_status = moderationStatus;
      const resp = await getAdminTemplates(params);
      setData(resp.data);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [q, visibility, ownerId, sort, warning, moderationStatus, page, perPage]);

  useEffect(() => {
    load();
  }, [load]);

  const lang = i18n.language || 'en';
  const hasFilters = q || visibility || ownerId || warning || moderationStatus;

  const columns = [
    {
      key: 'title',
      header: t('admin.templates.title'),
      render: (tpl) => (
        <div className="admin-templates__title-cell">
          <span className="admin-templates__title">{tpl.title}</span>
          <span className="admin-templates__tags">
            {(tpl.tags || []).slice(0, 3).map((tag) => (
              <span key={tag} className="admin-templates__tag">
                {tag}
              </span>
            ))}
          </span>
        </div>
      ),
    },
    {
      key: 'visibility',
      header: t('admin.templates.visibility'),
      render: (tpl) => (
        <Badge tone={tpl.visibility === 'public' ? 'success' : 'neutral'}>
          {tpl.visibility === 'public' ? t('admin.templates.public') : t('admin.templates.private')}
        </Badge>
      ),
    },
    {
      key: 'moderation_status',
      header: t('mod.filter.status'),
      render: (tpl) => (
        <div className="admin-templates__mod-cell">
          <StatusChip status={tpl.moderation_status || 'normal'} />
          {tpl.moderation_status === 'soft_deleted' && (
            <RestoreCountdown deletedAt={tpl.deleted_at} />
          )}
        </div>
      ),
    },
    {
      key: 'owner',
      header: t('admin.templates.owner'),
      render: (tpl) => (
        <button
          className="admin-templates__owner-link"
          onClick={(e) => {
            e.stopPropagation();
            navigate(`/admin/users/${tpl.owner_id}`);
          }}
          title={tpl.owner_id}
        >
          {tpl.owner_display_name || tpl.owner_id}
        </button>
      ),
    },
    {
      key: 'like_count',
      header: t('admin.templates.likes'),
      render: (tpl) => formatNumber(tpl.like_count, lang),
    },
    {
      key: 'favorite_count',
      header: t('admin.templates.favorites'),
      render: (tpl) => formatNumber(tpl.favorite_count, lang),
    },
    {
      key: 'warnings',
      header: t('warnings.admin.filter.label'),
      render: (tpl) => {
        const warnings = tpl.content_warnings || [];
        if (warnings.length === 0) return <span className="admin-templates__no-warnings">—</span>;
        return (
          <span className="admin-templates__warnings">
            {warnings.map((w) => (
              <span key={w} className="admin-templates__warning-chip">
                {t(`warnings.type.${w}`, { defaultValue: w })}
              </span>
            ))}
          </span>
        );
      },
    },
    {
      key: 'updated_at',
      header: t('admin.templates.updated'),
      render: (tpl) => (
        <span title={tpl.updated_at}>{formatRelativeTime(tpl.updated_at, lang)}</span>
      ),
    },
  ];

  return (
    <div>
      <div className="admin-templates__header">
        <h1 className="admin-page-title">{t('admin.templates.title')}</h1>
        <button className="admin-btn admin-btn--ghost" onClick={load} disabled={loading}>
          {t('admin.users.refresh')}
        </button>
      </div>

      {ownerId && (
        <div className="admin-templates__owner-filter">
          <span>
            {t('admin.templates.owner')}: <code>{ownerId}</code>
          </span>
          <button className="admin-btn admin-btn--ghost" onClick={() => setParam({ owner_id: '' })}>
            {t('admin.users.clear_filters')}
          </button>
        </div>
      )}

      <div className="admin-templates__filters">
        <AdminSearchInput
          value={q}
          onChange={(v) => setParam({ q: v })}
          placeholder={t('admin.templates.search_placeholder')}
        />
        <AdminSelect
          label={t('admin.templates.visibility')}
          value={visibility}
          onChange={(e) => setParam({ visibility: e.target.value })}
          options={[
            { value: '', label: t('admin.users.all') },
            { value: 'public', label: t('admin.templates.public') },
            { value: 'private', label: t('admin.templates.private') },
          ]}
        />
        <AdminSelect
          label={t('admin.users.sort')}
          value={sort}
          onChange={(e) => setParam({ sort: e.target.value })}
          options={[
            { value: 'recently_updated', label: t('admin.templates.sort_recently_updated') },
            { value: 'newest', label: t('admin.users.newest') },
            { value: 'oldest', label: t('admin.users.oldest') },
            { value: 'most_likes', label: t('admin.templates.sort_most_likes') },
          ]}
        />
        <AdminSelect
          label={t('warnings.admin.filter.label')}
          value={warning}
          onChange={(e) => setParam({ warning: e.target.value })}
          options={[
            { value: '', label: t('warnings.admin.filter.any') },
            { value: 'none', label: t('warnings.admin.filter.none') },
            { value: 'mature', label: t('warnings.type.mature') },
            { value: 'violence', label: t('warnings.type.violence') },
            { value: 'profanity', label: t('warnings.type.profanity') },
            { value: 'sensitive', label: t('warnings.type.sensitive') },
          ]}
        />
        <AdminSelect
          label={t('mod.filter.status')}
          value={moderationStatus}
          onChange={(e) => setParam({ moderation_status: e.target.value })}
          options={[
            { value: '', label: t('admin.users.all') },
            { value: 'normal', label: t('mod.status.normal') },
            { value: 'locked', label: t('mod.status.locked') },
            { value: 'unpublished_by_admin', label: t('mod.status.unpublished') },
            { value: 'soft_deleted', label: t('mod.status.soft_deleted') },
          ]}
        />
      </div>

      {error && <ErrorBanner message={error} onRetry={load} />}

      <AdminTable
        columns={columns}
        rows={data.templates || []}
        loading={loading}
        rowKey={(tpl) => tpl.id}
        onRowClick={(tpl) => navigate(`/admin/templates/${tpl.id}`)}
        emptyState={
          hasFilters ? (
            <EmptyState
              title={t('admin.templates.no_match')}
              action={
                <button
                  className="admin-btn admin-btn--ghost"
                  onClick={() => setParam({ q: '', visibility: '', owner_id: '', moderation_status: '' })}
                >
                  {t('admin.templates.clear_filters')}
                </button>
              }
            />
          ) : (
            <EmptyState title={t('admin.templates.no_match')} />
          )
        }
      />

      <Pagination
        page={page}
        total={data.total || 0}
        perPage={perPage}
        onPageChange={(p) => setParam({ page: p }, false)}
      />
    </div>
  );
};

export default TemplatesPage;
