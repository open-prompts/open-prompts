import React, { useEffect, useRef, useState } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import { useNotification } from '../../context/NotificationContext';
import {
  fetchNotificationPreferences,
  updateNotificationPreference,
  optimisticToggle,
  rollbackToggle,
} from '../../store/notificationPreferencesSlice';
import './NotificationPreferencesPanel.scss';

const TYPE_FIELDS = [
  { field: 'like_enabled', typeKey: 'like' },
  { field: 'favorite_enabled', typeKey: 'favorite' },
  { field: 'fork_enabled', typeKey: 'fork' },
  { field: 'comment_enabled', typeKey: 'comment' },
  { field: 'follow_enabled', typeKey: 'follow' },
];

/**
 * Notification preferences modal: a master "Pause all" switch plus five
 * per-type toggles. Each toggle auto-saves (PATCH) with an optimistic update
 * and rollback-on-error. While paused, type toggles are disabled.
 */
const NotificationPreferencesPanel = ({ open, onClose }) => {
  const { t } = useTranslation();
  const dispatch = useDispatch();
  const { addNotification } = useNotification();
  const { prefs, loaded, saving, error } = useSelector((state) => state.notificationPreferences);
  const [savedField, setSavedField] = useState(null);
  const savedTimerRef = useRef(null);

  // Fetch preferences on open (absent row renders all-on defaults).
  useEffect(() => {
    if (open) {
      dispatch(fetchNotificationPreferences());
    }
  }, [open, dispatch]);

  // Clear the transient "Saved" indicator after 1.5s.
  useEffect(() => {
    if (savedField) {
      savedTimerRef.current = setTimeout(() => setSavedField(null), 1500);
      return () => clearTimeout(savedTimerRef.current);
    }
    return undefined;
  }, [savedField]);

  const handleToggle = (field, value) => {
    const previous = prefs[field];
    dispatch(optimisticToggle({ field, value }));
    dispatch(updateNotificationPreference({ field, value }))
      .unwrap()
      .then(() => {
        setSavedField(field);
      })
      .catch(() => {
        dispatch(rollbackToggle({ field, value: previous }));
        addNotification({ kind: 'error', title: t('notifPrefs.error') });
      });
  };

  const paused = Boolean(prefs.paused_all);

  return (
    <Modal open={open} title={t('notifPrefs.title')} onClose={onClose} className="notif-prefs-modal">
      {error && !loaded && (
        <div className="notif-prefs-error" role="alert">
          <p>{t('notifPrefs.error')}</p>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => dispatch(fetchNotificationPreferences())}
          >
            {t('notifications.error.retry')}
          </button>
        </div>
      )}

      {!loaded && !error && (
        <div className="notif-prefs-skeleton" aria-label={t('common.loading')}>
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="notif-prefs-skeleton-row" />
          ))}
        </div>
      )}

      {loaded && (
        <div className="notif-prefs-body">
          <fieldset className="notif-prefs-fieldset">
            <legend className="sr-only">{t('notifPrefs.title')}</legend>

            <div className="notif-prefs-row notif-prefs-pause">
              <div className="notif-prefs-label">
                <span className="notif-prefs-name">{t('notifPrefs.pauseAll')}</span>
                <span className="notif-prefs-desc">{t('notifPrefs.pauseAllDesc')}</span>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={paused}
                aria-label={t('notifPrefs.pauseAll')}
                className={`switch ${paused ? 'switch-on' : ''}`}
                disabled={saving.paused_all}
                onClick={() => handleToggle('paused_all', !paused)}
              >
                <span className="switch-thumb" />
              </button>
            </div>

            {paused && (
              <p className="notif-prefs-paused-note" role="note">
                {t('notifPrefs.pausedNote')}
              </p>
            )}

            {TYPE_FIELDS.map(({ field, typeKey }) => {
              const isOn = Boolean(prefs[field]);
              const isSaving = Boolean(saving[field]);
              return (
                <div
                  key={field}
                  className={`notif-prefs-row ${paused ? 'notif-prefs-row-disabled' : ''}`}
                >
                  <div className="notif-prefs-label">
                    <span className="notif-prefs-name">{t(`notifPrefs.type.${typeKey}`)}</span>
                    <span className="notif-prefs-desc">{t(`notifPrefs.type.${typeKey}.desc`)}</span>
                  </div>
                  <div className="notif-prefs-control">
                    {savedField === field && (
                      <span className="notif-prefs-saved" role="status" aria-live="polite">
                        {t('notifPrefs.saved')} ✓
                      </span>
                    )}
                    <button
                      type="button"
                      role="switch"
                      aria-checked={isOn}
                      aria-disabled={paused}
                      aria-labelledby={`notif-prefs-${field}`}
                      className={`switch ${isOn ? 'switch-on' : ''}`}
                      disabled={paused || isSaving}
                      onClick={() => handleToggle(field, !isOn)}
                    >
                      <span className="switch-thumb" />
                    </button>
                  </div>
                </div>
              );
            })}
          </fieldset>

          <p className="notif-prefs-autosave">{t('notifPrefs.autoSave')}</p>
        </div>
      )}
    </Modal>
  );
};

export default NotificationPreferencesPanel;