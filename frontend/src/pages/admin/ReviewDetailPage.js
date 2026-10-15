import React, { useEffect, useState, useCallback } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  getReviewSubmission,
  getReviewQueue,
  approveReviewSubmission,
  rejectReviewSubmission,
} from '../../services/adminApi';
import Modal from '../../components/ui/Modal';
import ErrorBanner from '../../components/admin/ErrorBanner';
import Badge from '../../components/admin/Badge';
import { formatRelativeTime } from '../../utils/adminFormat';
import './ReviewDetailPage.scss';

const REJECT_REASONS = ['spam', 'plagiarism', 'harmful', 'lowQuality', 'other'];

/**
 * Admin review submission detail: full content, author context,
 * resubmission chain, approve/reject actions with keyboard shortcuts.
 */
const ReviewDetailPage = () => {
  const { t, i18n } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();

  const [submission, setSubmission] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [deciding, setDeciding] = useState(false);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [rejectReason, setRejectReason] = useState('');
  const [rejectNote, setRejectNote] = useState('');
  const [actionError, setActionError] = useState('');
  const [nextId, setNextId] = useState(null);

  const lang = i18n.language || 'en';

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const resp = await getReviewSubmission(id);
      setSubmission(resp.data.submission);
    } catch (err) {
      const status = err.response && err.response.status;
      setError(
        status === 404
          ? t('review.notFound')
          : (err.response && err.response.data && err.response.data.error) || ''
      );
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
  }, [load]);

  // Preload the next queue item so a decision can auto-advance.
  useEffect(() => {
    let cancelled = false;
    const loadNext = async () => {
      try {
        const resp = await getReviewQueue({ per_page: 100 });
        const items = resp.data.items || [];
        const idx = items.findIndex((i) => i.id === id);
        if (!cancelled && idx >= 0 && idx + 1 < items.length) {
          setNextId(items[idx + 1].id);
        } else if (!cancelled) {
          setNextId(null);
        }
      } catch (err) {
        // Non-critical.
      }
    };
    loadNext();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const advance = () => {
    if (nextId) navigate(`/admin/review/${nextId}`);
    else navigate('/admin/review');
  };

  const handleApprove = async () => {
    setDeciding(true);
    setActionError('');
    try {
      await approveReviewSubmission(id);
      advance();
    } catch (err) {
      setActionError((err.response && err.response.data && err.response.data.error) || '');
      setDeciding(false);
    }
  };

  const handleRejectConfirm = async () => {
    setDeciding(true);
    setActionError('');
    try {
      await rejectReviewSubmission(id, rejectReason, rejectNote.trim());
      setRejectOpen(false);
      advance();
    } catch (err) {
      setActionError((err.response && err.response.data && err.response.data.error) || '');
      setDeciding(false);
    }
  };

  // Keyboard shortcuts: A approve, R reject, → next. Never fire while a
  // dialog is open or focus is in an input.
  useEffect(() => {
    const onKey = (e) => {
      if (rejectOpen || deciding || loading) return;
      const tag = (e.target && e.target.tagName) || '';
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      if (e.key === 'a' || e.key === 'A') {
        e.preventDefault();
        handleApprove();
      } else if (e.key === 'r' || e.key === 'R') {
        e.preventDefault();
        setRejectOpen(true);
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        advance();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rejectOpen, deciding, loading, nextId, id]);

  if (loading) {
    return <div className="admin-page-loading" role="status">{t('admin.common.loading')}</div>;
  }

  if (error) {
    return (
      <div>
        <button className="admin-btn admin-btn--ghost" onClick={() => navigate('/admin/review')}>
          ← {t('review.detail.back')}
        </button>
        <ErrorBanner message={error} onRetry={load} />
      </div>
    );
  }

  if (!submission) return null;

  const s = submission;

  return (
    <div className="review-detail">
      <button className="admin-btn admin-btn--ghost" onClick={() => navigate('/admin/review')}>
        ← {t('review.detail.back')}
      </button>

      <div className="review-detail__heading">
        <h1 className="admin-page-title">{s.template_title}</h1>
        <div className="review-detail__subtitle">
          <button
            className="review-queue__creator-link"
            onClick={() => navigate(`/admin/users/${s.creator_id}`)}
          >
            {s.creator_display_name || s.creator_id}
          </button>
          <span>·</span>
          <span>{t('review.detail.submitted', { time: formatRelativeTime(s.submitted_at, lang) })}</span>
          {s.attempt_number > 1 && (
            <span className="review-queue__attempt-chip">
              {t('review.row.resubmission', { n: s.attempt_number })}
            </span>
          )}
        </div>
      </div>

      {actionError && <ErrorBanner message={actionError} />}

      <section className="review-detail__section">
        <h2>{t('review.detail.template_section')}</h2>
        {s.template_description && <p className="review-detail__desc">{s.template_description}</p>}
        <pre className="review-detail__content">{s.template_content}</pre>
        <div className="review-detail__meta">
          {s.category && <Badge tone="neutral">{s.category}</Badge>}
          {(s.tags || []).map((tag) => (
            <span key={tag} className="review-detail__tag">#{tag}</span>
          ))}
        </div>
      </section>

      <section className="review-detail__section">
        <h2>{t('review.detail.author_section')}</h2>
        <p className="review-detail__author">
          {t('review.detail.author', {
            date: new Date(s.creator_joined_at).toLocaleDateString(lang),
            published: s.creator_published_count,
            rejections: s.creator_prior_rejections,
          })}
        </p>
        {s.creator_banned && <Badge tone="danger">{t('review.detail.banned')}</Badge>}
      </section>

      <section className="review-detail__section">
        <h2>{t('review.detail.chain')}</h2>
        {(s.chain || []).length === 0 ? (
          <p className="review-detail__chain-empty">{t('review.detail.chain_none')}</p>
        ) : (
          <ul className="review-detail__chain">
            {s.chain.map((c) => (
              <li key={c.attempt_number} className="review-detail__chain-item">
                <Badge tone={c.status === 'approved' ? 'success' : 'danger'}>
                  {t('review.row.resubmission', { n: c.attempt_number })}
                </Badge>
                <span>
                  {c.status === 'rejected'
                    ? t('review.detail.chain_rejected', {
                        reason: t(`review.reject.reason.${c.reject_reason_code}`),
                      })
                    : t('review.status.approved')}
                </span>
                {c.reject_note && <span className="review-detail__chain-note">“{c.reject_note}”</span>}
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="review-detail__actions">
        <button
          className="admin-btn admin-btn--primary"
          onClick={handleApprove}
          disabled={deciding}
        >
          {t('review.action.approve')}
        </button>
        <button
          className="admin-btn admin-btn--danger"
          onClick={() => setRejectOpen(true)}
          disabled={deciding}
        >
          {t('review.action.reject')}
        </button>
        <span className="review-detail__kbd-hint">{t('review.detail.kbd_hint')}</span>
      </div>

      <Modal
        open={rejectOpen}
        title={t('review.reject.title')}
        primaryLabel={deciding ? t('admin.common.loading') : t('review.reject.confirm')}
        primaryDisabled={!rejectReason || deciding}
        secondaryLabel={t('common.cancel')}
        danger
        onPrimary={handleRejectConfirm}
        onClose={() => setRejectOpen(false)}
      >
        <div className="review-reject-dialog">
          <fieldset className="review-reject-dialog__reasons">
            <legend>{t('review.reject.reason_label')}</legend>
            {REJECT_REASONS.map((code) => (
              <label key={code} className="review-reject-dialog__reason">
                <input
                  type="radio"
                  name="reject-reason"
                  value={code}
                  checked={rejectReason === code}
                  onChange={() => setRejectReason(code)}
                  disabled={deciding}
                />
                {t(`review.reject.reason.${code}`)}
              </label>
            ))}
          </fieldset>
          {!rejectReason && (
            <p className="review-reject-dialog__hint" role="status">
              {t('review.reject.reasonRequired')}
            </p>
          )}
          <label className="review-reject-dialog__note-label" htmlFor="reject-note">
            {t('review.reject.note')}
          </label>
          <textarea
            id="reject-note"
            className="review-reject-dialog__note"
            value={rejectNote}
            onChange={(e) => setRejectNote(e.target.value)}
            maxLength={500}
            rows={3}
            disabled={deciding}
          />
        </div>
      </Modal>
    </div>
  );
};

export default ReviewDetailPage;
