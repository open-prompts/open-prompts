import React, { useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { updateTemplateWarnings, removeTemplateWarnings, getWarningHistory } from '../../services/adminApi';
import { AddIcon, CloseIcon } from '../ui/Icons';
import './WarningActionsPanel.scss';

const DEFAULT_TAXONOMY = ['mature', 'violence', 'profanity', 'sensitive'];

/**
 * WarningActionsPanel — admin panel to view/add/remove content warnings on a
 * template, with a mandatory reason and a change-history list.
 *
 * @param {Object} props
 * @param {string} props.templateId
 * @param {string[]} props.contentWarnings - current active warnings
 * @param {string[]} props.lockedWarnings - moderator-applied subset
 * @param {Function} props.onChanged - (contentWarnings, lockedWarnings) => void
 */
const WarningActionsPanel = ({ templateId, contentWarnings = [], lockedWarnings = [], onChanged }) => {
  const { t, i18n } = useTranslation();
  const [addKey, setAddKey] = useState('');
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const lang = i18n.language || 'en';

  const loadHistory = useCallback(async () => {
    setHistoryLoading(true);
    try {
      const resp = await getWarningHistory(templateId, { per_page: 50 });
      setHistory(resp.data.entries || []);
    } catch (e) {
      setHistory([]);
    } finally {
      setHistoryLoading(false);
    }
  }, [templateId]);

  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  const reasonValid = reason.trim().length >= 3;
  const addDisabled = !addKey || !reasonValid || submitting;
  const removeDisabled = (key) => !reasonValid || submitting;

  const applyChange = async (add, remove) => {
    setSubmitting(true);
    setError('');
    try {
      const resp = add.length > 0
        ? await updateTemplateWarnings(templateId, { add, remove: [], reason })
        : await removeTemplateWarnings(templateId, { add: [], remove, reason });
      if (onChanged) onChanged(resp.data.content_warnings || [], resp.data.locked_warnings || []);
      setReason('');
      setAddKey('');
      await loadHistory();
    } catch (e) {
      setError((e.response && e.response.data && e.response.data.error) || t('common.error'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleAdd = () => {
    if (!addKey) return;
    applyChange([addKey], []);
  };

  const handleRemove = (key) => {
    applyChange([], [key]);
  };

  const availableToAdd = DEFAULT_TAXONOMY.filter((k) => !contentWarnings.includes(k));

  return (
    <section className="warning-actions-panel">
      <h2>{t('warnings.admin.panel.title')}</h2>

      <div className="warning-actions-panel__current">
        <span className="warning-actions-panel__current-label">{t('warnings.admin.panel.current')}:</span>
        {contentWarnings.length === 0 && <span className="warning-actions-panel__none">—</span>}
        {contentWarnings.map((key) => {
          const locked = lockedWarnings.includes(key);
          return (
            <span key={key} className="warning-actions-panel__chip">
              <span className="warning-actions-panel__chip-name">
                {t(`warnings.type.${key}`, { defaultValue: key })}
              </span>
              <span className="warning-actions-panel__chip-origin">
                ({locked
                  ? t('warnings.admin.panel.origin.moderation')
                  : t('warnings.admin.panel.origin.creator')})
              </span>
              <button
                type="button"
                className="warning-actions-panel__remove"
                onClick={() => handleRemove(key)}
                disabled={removeDisabled(key)}
                aria-label={`${t('warnings.admin.panel.remove')} ${key}`}
                title={t('warnings.admin.panel.remove')}
              >
                <CloseIcon size={12} />
              </button>
            </span>
          );
        })}
      </div>

      <div className="warning-actions-panel__controls">
        <select
          className="warning-actions-panel__select"
          value={addKey}
          onChange={(e) => setAddKey(e.target.value)}
          aria-label={t('warnings.admin.panel.add')}
        >
          <option value="">{t('warnings.admin.panel.add')}…</option>
          {availableToAdd.map((k) => (
            <option key={k} value={k}>
              {t(`warnings.type.${k}`, { defaultValue: k })}
            </option>
          ))}
        </select>
        <input
          type="text"
          className="warning-actions-panel__reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder={t('warnings.admin.panel.reason')}
          aria-label={t('warnings.admin.panel.reason')}
        />
        <button
          type="button"
          className="admin-btn admin-btn--primary"
          onClick={handleAdd}
          disabled={addDisabled}
        >
          <AddIcon size={14} />
          {t('warnings.admin.panel.add')}
        </button>
      </div>

      {error && <p className="warning-actions-panel__error" role="alert">{error}</p>}

      <div className="warning-actions-panel__history">
        <h3>{t('warnings.admin.panel.history')}</h3>
        {historyLoading && <p className="warning-actions-panel__muted">{t('admin.common.loading')}</p>}
        {!historyLoading && history.length === 0 && (
          <p className="warning-actions-panel__muted">{t('warnings.admin.panel.historyEmpty')}</p>
        )}
        <ul className="warning-actions-panel__history-list">
          {history.map((entry) => {
            const names = (entry.warnings || []).map((w) => t(`warnings.type.${w}`, { defaultValue: w }));
            const actionLabel =
              entry.action === 'template.warning_add'
                ? t('warnings.notify.added', { warning: names.join(', ') })
                : t('warnings.notify.removed', { warning: names.join(', ') });
            return (
              <li key={entry.id} className="warning-actions-panel__history-item">
                <span className="warning-actions-panel__history-action">{actionLabel}</span>
                <span className="warning-actions-panel__history-actor">{entry.actor_email}</span>
                <span className="warning-actions-panel__history-time">
                  {new Date(entry.created_at).toLocaleString(lang)}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
};

export default WarningActionsPanel;