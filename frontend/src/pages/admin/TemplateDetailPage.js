import React, { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminTemplate } from '../../services/adminApi';
import Badge from '../../components/admin/Badge';
import ErrorBanner from '../../components/admin/ErrorBanner';
import EmptyState from '../../components/admin/EmptyState';
import WarningActionsPanel from '../../components/warnings/WarningActionsPanel';
import ExportButton from '../../components/ExportButton';
import { formatDate } from '../../utils/adminFormat';
import './TemplateDetailPage.scss';

/**
 * Read-only admin template detail: metadata card plus a versions accordion.
 */
const TemplateDetailPage = () => {
  const { id } = useParams();
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();

  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState('');
  const [openVersion, setOpenVersion] = useState(null);
  const [warnings, setWarnings] = useState([]);
  const [lockedWarnings, setLockedWarnings] = useState([]);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    setNotFound(false);
    try {
      const resp = await getAdminTemplate(id);
      setData(resp.data);
      setWarnings(resp.data.template.content_warnings || []);
      setLockedWarnings(resp.data.template.locked_warnings || []);
      // Open the newest version by default.
      if (resp.data && resp.data.versions && resp.data.versions.length > 0) {
        setOpenVersion(resp.data.versions[0].id);
      }
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 404) setNotFound(true);
      else setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) {
    return (
      <div>
        <h1 className="admin-page-title">{t('admin.templates.title')}</h1>
        <p className="admin-page-subtitle">{t('admin.common.loading')}</p>
      </div>
    );
  }

  if (notFound) {
    return (
      <div>
        <Link to="/admin/templates" className="admin-detail__back">
          ← {t('admin.templates.back')}
        </Link>
        <EmptyState title={t('admin.templates.not_found')} />
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <Link to="/admin/templates" className="admin-detail__back">
          ← {t('admin.templates.back')}
        </Link>
        <ErrorBanner message={error} onRetry={load} />
      </div>
    );
  }

  const { template, description, versions } = data;
  const lang = i18n.language || 'en';

  return (
    <div>
      <Link to="/admin/templates" className="admin-detail__back">
        ← {t('admin.templates.back')}
      </Link>

      <div className="admin-template-detail__header">
        <div>
          <h1>{template.title}</h1>
          {description && <p className="admin-template-detail__desc">{description}</p>}
          <div className="admin-template-detail__meta">
            <Badge tone={template.visibility === 'public' ? 'success' : 'neutral'}>
              {template.visibility === 'public'
                ? t('admin.templates.public')
                : t('admin.templates.private')}
            </Badge>
            <Badge tone="accent">{template.language}</Badge>
            <span>
              {t('admin.templates.owner')}:{' '}
              <button
                className="admin-template-detail__owner-link"
                onClick={() => navigate(`/admin/users/${template.owner_id}`)}
              >
                {template.owner_display_name || template.owner_id}
              </button>
            </span>
            <span>
              {t('admin.templates.likes')}: {template.like_count}
            </span>
            <span>
              {t('admin.templates.favorites')}: {template.favorite_count}
            </span>
            <span>
              {t('admin.templates.rating')}: {template.rating_count > 0 ? `${template.rating_avg.toFixed(2)} (${template.rating_count})` : t('admin.templates.no_ratings')}
            </span>
          </div>
        </div>
        <ExportButton template={template} dataSource="admin" />
      </div>

      <section className="admin-template-detail__panel">
        <h2>{t('admin.templates.versions')}</h2>
        {(!versions || versions.length === 0) && (
          <p className="admin-template-detail__muted">{t('admin.templates.no_versions')}</p>
        )}
        <div className="admin-template-detail__versions">
          {(versions || []).map((v) => {
            const open = openVersion === v.id;
            return (
              <div key={v.id} className="admin-template-detail__version">
                <button
                  className="admin-template-detail__version-header"
                  onClick={() => setOpenVersion(open ? null : v.id)}
                  aria-expanded={open}
                >
                  <span>
                    v{v.version}
                    {v.alias_name && (
                      <em className="admin-template-detail__alias">
                        {t('admin.templates.alias')}: {v.alias_name}
                      </em>
                    )}
                  </span>
                  <span className="admin-template-detail__version-date">
                    {formatDate(v.created_at, lang)}
                  </span>
                </button>
                {open && (
                  <pre className="admin-template-detail__content">{v.content}</pre>
                )}
              </div>
            );
          })}
        </div>
      </section>

      <WarningActionsPanel
        templateId={template.id}
        contentWarnings={warnings}
        lockedWarnings={lockedWarnings}
        onChanged={(cw, lw) => {
          setWarnings(cw);
          setLockedWarnings(lw);
        }}
      />
    </div>
  );
};

export default TemplateDetailPage;
