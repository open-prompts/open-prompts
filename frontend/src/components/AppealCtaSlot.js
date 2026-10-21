import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { getAppeal } from '../services/api';
import AppealStateChip from './AppealStateChip';
import AppealFormModal from './AppealFormModal';
import './AppealCtaSlot.scss';

/**
 * Owner banner / removed-notice slot: shows the appeal CTA when there is no
 * appeal, a status chip when open/under_review, or an outcome chip + note
 * excerpt when resolved. Hidden when the action is superseded (FR-14).
 *
 * @param {Object} props
 * @param {Object} props.action - { id, action_type, reason, actor_display_name, created_at, template_title }
 * @param {string} props.templateId
 */
const AppealCtaSlot = ({ action, templateId }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [appeal, setAppeal] = useState(null);
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);

  const load = useCallback(async () => {
    if (!action || !action.id) {
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const resp = await getAppeal(action.id);
      setAppeal(resp.data);
    } catch (err) {
      // 404 means no appeal yet — show the CTA.
      setAppeal(null);
    } finally {
      setLoading(false);
    }
  }, [action]);

  useEffect(() => {
    load();
  }, [load]);

  if (!action || action.superseded) return null;
  if (loading) return null;

  const resolved = appeal && (appeal.state === 'resolved_upheld' || appeal.state === 'resolved_reversed');

  return (
    <div className="appeal-cta" role="status">
      {!appeal && (
        <button type="button" className="appeal-cta__button" onClick={() => setFormOpen(true)}>
          {t('appeal.cta.appeal')}
        </button>
      )}
      {appeal && !resolved && (
        <button
          type="button"
          className="appeal-cta__status"
          onClick={() => navigate(`/appeals/${appeal.id}`)}
        >
          <AppealStateChip state={appeal.state} />
          <span className="appeal-cta__hint">{t('appeal.cta.onePerAction')}</span>
        </button>
      )}
      {appeal && resolved && (
        <button
          type="button"
          className="appeal-cta__status"
          onClick={() => navigate(`/appeals/${appeal.id}`)}
        >
          <AppealStateChip state={appeal.state} />
          {appeal.resolution_note && (
            <span className="appeal-cta__note">{appeal.resolution_note}</span>
          )}
        </button>
      )}
      <AppealFormModal
        open={formOpen}
        action={action}
        onClose={() => setFormOpen(false)}
        onSuccess={(a) => setAppeal(a)}
      />
    </div>
  );
};

export default AppealCtaSlot;