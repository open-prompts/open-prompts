import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { CloseIcon } from '../ui/Icons';
import { getAdminReportDetail, resolveReportGroup, unpublishTemplate } from '../../services/adminApi';
import ReasonPopover from './ReasonPopover';
import { formatRelativeTime } from '../../utils/adminFormat';
import './ReviewDrawer.scss';

/**
 * ReviewDrawer is the right-side drawer for a reported item: item context
 * card, every report (reporter, reason, note, time), and resolution actions
 * — Dismiss reports / Act (unpublish template or delete comment), each
 * requiring a reason (FR-10). "Ban author" links to the admin user detail.
 *
 * @param {Object} props
 * @param {boolean} props.open
 * @param {string} props.itemType - "template" | "comment"
 * @param {string} props.itemId
 * @param {Function} props.onClose
 * @param {Function} props.onResolved - called after a successful resolve
 */
const ReviewDrawer = ({ open, itemType, itemId, onClose, onResolved }) => {
  const { t, i18n } = useTranslation();
  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [popover, setPopover] = useState(null); // 'dismiss' | 'act'
  const [busy, setBusy] = useState(false);
  const drawerRef = useRef(null);
  const lang = i18n.language || 'en';

  useEffect(() => {
    if (!open) return undefined;
    setDetail(null);
    setError('');
    setPopover(null);
    setLoading(true);
    let cancelled = false;
    getAdminReportDetail(itemType, itemId)
      .then((resp) => {
        if (!cancelled) setDetail(resp.data);
      })
      .catch((err) => {
        if (!cancelled) setError((err.response && err.response.data && err.response.data.error) || '');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, itemType, itemId]);

  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (e) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const handleResolve = async (resolution, reason) => {
    setBusy(true);
    setError('');
    try {
      // "Act → unpublish template" (FR-10): call the real moderation
      // unpublish endpoint, which auto-resolves the report group as
      // actioned in the same transaction.
      if (isTemplate && resolution === 'actioned') {
        await unpublishTemplate(itemId, { reason });
      } else {
        await resolveReportGroup(itemType, itemId, { resolution, reason });
      }
      setPopover(null);
      onResolved && onResolved();
      onClose();
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
      setBusy(false);
    }
  };

  const isTemplate = itemType === 'template';

  return createPortal(
    <div className="review-drawer-overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="review-drawer" ref={drawerRef} role="dialog" aria-modal="true" aria-label={t('admin.reports.review')}>
        <div className="review-drawer__header">
          <h3 className="review-drawer__title">
            {t('admin.reports.review')} · {isTemplate ? t('admin.reports.typeTemplate') : t('admin.reports.typeComment')}
          </h3>
          <button type="button" className="review-drawer__close" onClick={onClose} aria-label={t('common.close')}>
            <CloseIcon size={18} />
          </button>
        </div>

        <div className="review-drawer__body">
          {loading && <div className="review-drawer__loading">{t('admin.common.loading')}</div>}
          {error && <div className="review-drawer__error" role="alert">{error}</div>}

          {detail && (
            <>
              <div className="review-drawer__item">
                <div className="review-drawer__item-title">{detail.item_title || detail.item_content}</div>
                {detail.author_display_name && (
                  <div className="review-drawer__item-author">
                    {t('admin.reports.author')}: {detail.author_display_name}
                  </div>
                )}
                {detail.item_content && (
                  <p className="review-drawer__item-content">{detail.item_content}</p>
                )}
              </div>

              <div className="review-drawer__reports">
                <h4 className="review-drawer__reports-title">
                  {t('admin.reports.reports', { count: (detail.reports || []).length })}
                </h4>
                {(detail.reports || []).map((r) => (
                  <div key={r.id} className="review-drawer__report">
                    <div className="review-drawer__report-meta">
                      <span className="review-drawer__reporter">{r.reporter_display_name || r.reporter_id}</span>
                      <span className="review-drawer__reason">{t(`reports.reason.${r.reason}`, { defaultValue: r.reason })}</span>
                      <span className="review-drawer__time">{formatRelativeTime(r.created_at, lang)}</span>
                    </div>
                    {r.note && <p className="review-drawer__note">{r.note}</p>}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>

        {detail && (
          <div className="review-drawer__footer">
            <div className="review-drawer__actions">
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setPopover(popover === 'dismiss' ? null : 'dismiss')}
              >
                {t('admin.reports.dismiss')}
              </button>
              <button
                type="button"
                className="btn btn-danger"
                onClick={() => setPopover(popover === 'act' ? null : 'act')}
              >
                {isTemplate ? t('admin.reports.unpublish') : t('admin.reports.deleteComment')}
              </button>
            </div>
            {detail.author_id && (
              <a className="review-drawer__author-link" href={`/admin/users/${detail.author_id}`}>
                {t('admin.reports.banAuthor')}
              </a>
            )}
            {popover && (
              <ReasonPopover
                open
                title={popover === 'dismiss' ? t('admin.reports.dismiss') : (isTemplate ? t('admin.reports.unpublish') : t('admin.reports.deleteComment'))}
                confirmLabel={popover === 'dismiss' ? t('admin.reports.dismiss') : t('admin.reports.act')}
                danger={popover === 'act'}
                busy={busy}
                onConfirm={(reason) => handleResolve(popover === 'dismiss' ? 'dismissed' : 'actioned', reason)}
                onClose={() => setPopover(null)}
              />
            )}
          </div>
        )}
      </div>
    </div>,
    document.body
  );
};

export default ReviewDrawer;