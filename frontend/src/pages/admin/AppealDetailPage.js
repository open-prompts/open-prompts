import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { getAdminAppeal, reviewAppeal, resolveAppeal } from '../../services/adminApi';
import Modal from '../../components/ui/Modal';
import ErrorBanner from '../../components/admin/ErrorBanner';
import Badge from '../../components/admin/Badge';
import { formatRelativeTime } from '../../utils/adminFormat';
import './AppealDetailPage.scss';

const STATE_TONE = {
  open: 'warning',
  under_review: 'accent',
  resolved_upheld: 'neutral',
  resolved_reversed: 'success',
};

/**
 * /admin/appeals/:id — two-pane appeal + original action, Review button,
 * Resolve form (outcome radio + required note) with self-guard disabled
 * state.
 */
const AppealDetailPage = () => {
  const { t, i18n } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();

  const [appeal, setAppeal] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');
  const [reviewing, setReviewing] = useState(false);
  const [resolveOpen, setResolveOpen] = useState(false);
  const [outcome, setOutcome] = useState('upheld');
  const [note, setNote] = useState('');
  const [resolving, setResolving] = useState(false);

  const lang = i18n.language || 'en';

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const resp = await getAdminAppeal(id);
      setAppeal(resp.data);
    } catch (err) {
      const status = err.response && err.response.status;
      setError(
        status === 404
          ? t('common.notFound')
          : (err.response && err.response.data && err.response.data.error) || ''
      );
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
  }, [load]);

  const handleReview = async () => {
    setReviewing(true);
    setActionError('');
    try {
      const resp = await reviewAppeal(id);
      setAppeal(resp.data);
    } catch (err) {
      setActionError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setReviewing(false);
    }
  };

  const handleResolve = async () => {
    setResolving(true);
    setActionError('');
    try {
      const resp = await resolveAppeal(id, { outcome, note: note.trim() });
      setAppeal(resp.data);
      setResolveOpen(false);
    } catch (err) {
      setActionError((err.response && err.response.data && err.response.data.error) || '');
      setResolving(false);
    }
  };

  if (loading) {
    return <div className="admin-page-loading" role="status">{t('admin.common.loading')}</div>;
  }
  if (error) {
    return (
      <div>
        <button className="admin-btn admin-btn--ghost" onClick={() => navigate('/admin/appeals')}>
          ← {t('appeal.admin.queue.title')}
        </button>
        <ErrorBanner message={error} onRetry={load} />
      </div>
    );
  }
  if (!appeal) return null;

  const a = appeal;
  const resolved = a.state === 'resolved_upheld' || a.state === 'resolved_reversed';
  const noteLen = note.length;
  const noteInvalid = noteLen < 20 || noteLen > 1000;

  return (
    <div className="appeal-admin-detail">
      <button className="admin-btn admin-btn--ghost" onClick={() => navigate('/admin/appeals')}>
        ← {t('appeal.admin.queue.title')}
      </button>

      <div className="appeal-admin-detail__heading">
        <h1 className="admin-page-title">{a.template_title}</h1>
        <Badge tone={STATE_TONE[a.state] || 'neutral'}>{t(`appeal.state.${a.state}`, { defaultValue: a.state })}</Badge>
      </div>

      {actionError && <ErrorBanner message={actionError} />}

      <div className="appeal-admin-detail__panes">
        <section className="appeal-admin-detail__pane">
          <h2>{t('appeal.admin.detail.appeal', { defaultValue: 'Appeal' })}</h2>
          <p className="appeal-admin-detail__statement">{a.statement}</p>
          <p className="appeal-admin-detail__meta">
            {t('appeal.admin.detail.filed', { defaultValue: 'Filed' })} {formatRelativeTime(a.created_at, lang)} · {a.appellant_display_name}
          </p>
        </section>
        <section className="appeal-admin-detail__pane">
          <h2>{t('appeal.admin.detail.originalAction')}</h2>
          <div className="appeal-admin-detail__action">
            <span className="appeal-admin-detail__action-type">{a.action_type}</span>
            <span className="appeal-admin-detail__action-reason">"{a.action_reason}"</span>
            <span className="appeal-admin-detail__action-meta">
              {a.action_actor_email} · {formatRelativeTime(a.action_created_at, lang)}
            </span>
            {a.audit_entry_id && (
              <span className="appeal-admin-detail__audit">Audit #{a.audit_entry_id}</span>
            )}
          </div>
        </section>
      </div>

      <section className="appeal-admin-detail__resolution">
        <h2>{t('appeal.detail.resolution')}</h2>
        {resolved ? (
          <p className="appeal-admin-detail__resolution-note">{a.resolution_note}</p>
        ) : (
          <div className="appeal-admin-detail__actions">
            {a.state === 'open' && (
              <button className="admin-btn admin-btn--primary" onClick={handleReview} disabled={reviewing}>
                {reviewing ? t('admin.common.loading') : t('appeal.admin.detail.review')}
              </button>
            )}
            <button
              className="admin-btn admin-btn--primary"
              onClick={() => setResolveOpen(true)}
              disabled={a.action_actor_id === a.resolver_id || a.superseded}
              title={a.action_actor_id === a.resolver_id ? t('appeal.admin.detail.selfGuard') : undefined}
            >
              {t('appeal.admin.detail.resolve')}
            </button>
            {a.action_actor_id === a.resolver_id && (
              <span className="appeal-admin-detail__self-guard">{t('appeal.admin.detail.selfGuard')}</span>
            )}
          </div>
        )}
      </section>

      <Modal
        open={resolveOpen}
        title={t('appeal.admin.detail.resolve')}
        primaryLabel={resolving ? t('admin.common.loading') : t('appeal.admin.detail.resolve')}
        primaryDisabled={noteInvalid || resolving}
        secondaryLabel={t('common.cancel')}
        onPrimary={handleResolve}
        onClose={() => setResolveOpen(false)}
      >
        <div className="appeal-admin-detail__resolve-form">
          <fieldset className="appeal-admin-detail__outcome">
            <legend>{t('appeal.admin.detail.outcome')}</legend>
            <label>
              <input type="radio" name="outcome" value="upheld" checked={outcome === 'upheld'} onChange={() => setOutcome('upheld')} />
              {t('appeal.admin.detail.upheld')}
            </label>
            <label>
              <input type="radio" name="outcome" value="reversed" checked={outcome === 'reversed'} onChange={() => setOutcome('reversed')} />
              {t('appeal.admin.detail.reversed')}
            </label>
          </fieldset>
          {outcome === 'reversed' && (
            <p className="appeal-admin-detail__confirm">{t('appeal.admin.confirm.reversed')}</p>
          )}
          <label className="appeal-admin-detail__note-label" htmlFor="resolve-note">
            {t('appeal.admin.detail.note')}
          </label>
          <textarea
            id="resolve-note"
            className="appeal-admin-detail__note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={4}
            maxLength={1000}
          />
          <span className="appeal-admin-detail__note-rule">{t('appeal.admin.detail.noteRule')}</span>
        </div>
      </Modal>
    </div>
  );
};

export default AppealDetailPage;