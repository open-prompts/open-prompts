import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { MenuIcon } from './ui/Icons';
import ReportModal from './ReportModal';
import ReportedMenuItem from './ReportedMenuItem';
import { getMyReport, withdrawReport } from '../services/api';
import { useNotification } from '../context/NotificationContext';
import './ReportMenu.scss';

/**
 * ReportMenu is the ⋯ menu entry on templates and comments. It shows
 * "Report" when the caller has no open report, or a checkmark state with
 * Update/Withdraw when they do (FR-7). Hidden for the user's own content
 * (FR-4) — the parent decides whether to render it.
 *
 * @param {Object} props
 * @param {string} props.itemType - "template" | "comment"
 * @param {string} props.itemId
 * @param {boolean} props.isOwner - hide the menu for the user's own content
 */
const ReportMenu = ({ itemType, itemId, isOwner = false }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();
  const [open, setOpen] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [reported, setReported] = useState(null); // the caller's open report or null
  const [confirmWithdraw, setConfirmWithdraw] = useState(false);
  const menuRef = useRef(null);

  useEffect(() => {
    if (isOwner) return;
    let cancelled = false;
    const load = async () => {
      try {
        const resp = await getMyReport(itemType, itemId);
        if (!cancelled) setReported(resp.data.report || null);
      } catch (e) {
        // ignore; treat as not reported
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [itemType, itemId, isOwner]);

  useEffect(() => {
    const onClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        setOpen(false);
        setConfirmWithdraw(false);
      }
    };
    document.addEventListener('mousedown', onClickOutside);
    return () => document.removeEventListener('mousedown', onClickOutside);
  }, []);

  if (isOwner) return null;

  const handleReported = (report) => {
    setReported(report);
    addNotification({
      kind: 'success',
      title: report && report.created_at ? t('reports.toast.updated') : t('reports.toast.submitted'),
    });
  };

  const handleWithdraw = async () => {
    try {
      await withdrawReport(itemType, itemId);
      setReported(null);
      setOpen(false);
      setConfirmWithdraw(false);
    } catch (e) {
      addNotification({ kind: 'error', title: t('reports.withdraw') });
    }
  };

  return (
    <div className="report-menu" ref={menuRef}>
      <button
        type="button"
        className="report-menu__trigger"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t('reports.action')}
        title={t('reports.action')}
      >
        <MenuIcon size={17} />
      </button>

      {open && (
        <div className="report-menu__dropdown" role="menu">
          {reported ? (
            <ReportedMenuItem
              onUpdate={() => {
                setOpen(false);
                setModalOpen(true);
              }}
              onWithdraw={() => setConfirmWithdraw(true)}
            />
          ) : (
            <button
              type="button"
              className="report-menu__item"
              role="menuitem"
              onClick={() => {
                setOpen(false);
                setModalOpen(true);
              }}
            >
              {t('reports.action')}
            </button>
          )}
        </div>
      )}

      {confirmWithdraw && (
        <div className="report-menu__confirm" role="alertdialog" aria-label={t('reports.withdrawConfirm')}>
          <p>{t('reports.withdrawConfirm')}</p>
          <div className="report-menu__confirm-actions">
            <button type="button" className="btn btn-ghost" onClick={() => setConfirmWithdraw(false)}>
              {t('common.cancel')}
            </button>
            <button type="button" className="btn btn-danger" onClick={handleWithdraw}>
              {t('reports.withdraw')}
            </button>
          </div>
        </div>
      )}

      <ReportModal
        open={modalOpen}
        itemType={itemType}
        itemId={itemId}
        initialReason={reported ? reported.reason : ''}
        initialNote={reported ? reported.note : ''}
        onClose={() => setModalOpen(false)}
        onSuccess={handleReported}
      />
    </div>
  );
};

export default ReportMenu;