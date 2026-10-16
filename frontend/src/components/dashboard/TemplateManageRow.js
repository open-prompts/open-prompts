import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatRelativeTime } from '../../utils/time';
import { formatCount } from '../../utils/formatCount';

/**
 * TemplateManageRow — one owned-template row with edit/delete/toggle actions.
 * @param {Object} props
 * @param {Object} props.template - a Template object
 * @param {Function} props.onToggleVisibility - () => void
 * @param {Function} props.onDelete - () => void
 */
const TemplateManageRow = ({ template, onToggleVisibility, onDelete }) => {
  const { t } = useTranslation();
  const isPublic = template.visibility === 'VISIBILITY_PUBLIC';
  const updated = template.updated_at || template.updatedAt;

  return (
    <li className="template-manage-row">
      <div className="template-manage-row__main">
        <Link to={`/templates/${template.id}`} className="template-manage-row__title">
          {template.title}
        </Link>
        <div className="template-manage-row__meta">
          <span className="template-manage-row__badge">
            {isPublic ? t('create_template.visibility_public') : t('create_template.visibility_private')}
          </span>
          {template.category ? <span>{template.category}</span> : null}
          {template.tags && template.tags.length > 0 ? (
            <span>{template.tags.slice(0, 3).join(', ')}</span>
          ) : null}
          <span>{t('dashboard_v2.templates.updated', { date: formatRelativeTime(updated, t) })}</span>
          <span>❤️ {formatCount(template.like_count || 0)}</span>
        </div>
      </div>
      <div className="template-manage-row__actions">
        <Link to={`/templates/${template.id}`} className="btn btn-ghost">
          {t('common.edit')}
        </Link>
        <button type="button" className="btn btn-ghost" onClick={onToggleVisibility}>
          {isPublic ? t('create_template.visibility_private') : t('create_template.visibility_public')}
        </button>
        <button type="button" className="btn btn-danger" onClick={onDelete}>
          {t('common.delete')}
        </button>
      </div>
    </li>
  );
};

export default TemplateManageRow;