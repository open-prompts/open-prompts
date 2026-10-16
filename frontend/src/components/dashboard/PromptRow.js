import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatRelativeTime } from '../../utils/time';

/**
 * PromptRow — one instantiated-prompt row with a source link and actions.
 * @param {Object} props
 * @param {Object} props.prompt - a Prompt object (with template_title)
 * @param {Function} props.onView - () => void
 * @param {Function} props.onDelete - () => void
 */
const PromptRow = ({ prompt, onView, onDelete }) => {
  const { t } = useTranslation();
  const variables = prompt.variables || {};
  const varKeys = Object.keys(variables);

  return (
    <li className="prompt-row">
      <div className="prompt-row__main">
        <div className="prompt-row__title">
          {prompt.template_title ? (
            <Link to={`/templates/${prompt.template_id}`} className="prompt-row__source">
              {t('dashboard_v2.prompts.from', { title: prompt.template_title })}
            </Link>
          ) : (
            <span>{t('dashboard_v2.prompts.from', { title: prompt.template_id })}</span>
          )}
        </div>
        <div className="prompt-row__meta">
          {varKeys.length > 0 ? (
            <span className="prompt-row__vars">
              {t('dashboard_v2.prompts.variables')}: {varKeys.join(', ')}
            </span>
          ) : null}
          <span>{t('dashboard_v2.prompts.created', { date: formatRelativeTime(prompt.created_at, t) })}</span>
        </div>
      </div>
      <div className="prompt-row__actions">
        <button type="button" className="btn btn-ghost" onClick={onView}>
          {t('dashboard_v2.prompts.view')}
        </button>
        <button type="button" className="btn btn-danger" onClick={onDelete}>
          {t('common.delete')}
        </button>
      </div>
    </li>
  );
};

export default PromptRow;