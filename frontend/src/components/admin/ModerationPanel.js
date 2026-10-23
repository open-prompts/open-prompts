import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  unpublishTemplate,
  lockTemplate,
  softDeleteTemplate,
  restoreTemplate,
  getAdminAuditLogs,
} from '../../services/adminApi';
import StatusChip from './StatusChip';
import ActionReasonModal from './ActionReasonModal';
import { formatDate } from '../../utils/adminFormat';
import './ModerationPanel.scss';

/**
 * ModerationPanel on the admin template detail page: current status chip,
 * the three moderation actions with consequence summaries, restore when
 * applicable, and the moderation history (from audit logs filtered by
 * target).
 *
 * @param {Object} props
 * @param {Object} props.template - AdminTemplateSummary
 * @param {Function} props.onChanged - called after a successful action
 */
const ModerationPanel = ({ template, onChanged }) => {
  const { t, i18n } = useTranslation();
  const [modal, setModal] = useState(null); // 'unpublish' | 'lock' | 'delete' | 'restore'
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [history, setHistory] = useState([]);
  const lang = i18n.language || 'en';

  const loadHistory = useCallback(async () => {
    try {
      const resp = await getAdminAuditLogs({ per_page: 100 });
      const entries = (resp.data.entries || []).filter((e) =>
        e.target_id === template.id &&
        ['template.unpublish', 'template.lock', 'template.soft_delete', 'template.restore'].includes(e.action)
      );
      setHistory(entries);
    } catch (e) {
      // History is best-effort; the panel still renders.
    }
  }, [template.id]);

  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  const status = template.moderation_status || 'normal';
  const canRestore = status !== 'normal' && status !== 'in_review' && status !== 'rejected';

  const handleConfirm = async (reason) => {
    setBusy(true);
    setError('');
    try {
      if (modal === 'unpublish') await unpublishTemplate(template.id, { reason });
      else if (modal === 'lock') await lockTemplate(template.id, { reason });
      else if (modal === 'delete') await softDeleteTemplate(template.id, { reason });
      else if (modal === 'restore') await restoreTemplate(template.id, { reason });
      setModal(null);
      onChanged && onChanged();
      loadHistory();
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="admin-moderation-panel">
      <div className="admin-moderation-panel__header">
        <h2>{t('mod.panel.title')}</h2>
        <div className="admin-moderation-panel__status">
          <span className="admin-moderation-panel__status-label">{t('mod.panel.status')}:</span>
          <StatusChip status={status} />
        </div>
      </div>

      {error && <div className="admin-moderation-panel__error" role="alert">{error}</div>}

      <div className="admin-moderation-panel__actions">
        <button
          type="button"
          className="admin-btn"
          onClick={() => setModal('unpublish')}
          disabled={status === 'soft_deleted'}
        >
          {t('mod.action.unpublish')}
        </button>
        <button
          type="button"
          className="admin-btn"
          onClick={() => setModal('lock')}
          disabled={status === 'soft_deleted'}
        >
          {t('mod.action.lock')}
        </button>
        <button
          type="button"
          className="admin-btn admin-btn--danger"
          onClick={() => setModal('delete')}
          disabled={status === 'soft_deleted'}
        >
          {t('mod.action.delete')}
        </button>
        {canRestore && (
          <button
            type="button"
            className="admin-btn admin-btn--ghost"
            onClick={() => setModal('restore')}
          >
            {t('mod.action.restore')}
          </button>
        )}
      </div>

      <div className="admin-moderation-panel__history">
        <h3>{t('mod.history.title')}</h3>
        {history.length === 0 ? (
          <p className="admin-moderation-panel__empty">{t('mod.history.empty')}</p>
        ) : (
          <ul className="admin-moderation-panel__history-list">
            {history.map((e) => (
              <li key={e.id} className="admin-moderation-panel__history-item">
                <span className="admin-moderation-panel__history-action">{e.action}</span>
                <span className="admin-moderation-panel__history-actor">{e.actor_email || e.actor_id}</span>
                <span className="admin-moderation-panel__history-time">{formatDate(e.created_at, lang)}</span>
                {e.reason && <span className="admin-moderation-panel__history-reason">{e.reason}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>

      <ActionReasonModal
        open={!!modal}
        action={modal}
        templateTitle={template.title}
        onConfirm={handleConfirm}
        onClose={() => setModal(null)}
        busy={busy}
      />
    </section>
  );
};

export default ModerationPanel;