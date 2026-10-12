import { useEffect, useRef } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import { fetchUnreadCount } from '../store/notificationsSlice';

// All polling timing constants in one place (FR-10 / UI/UX §6.1).
export const POLL_INTERVAL_MS = 60 * 1000;
export const POLL_BACKOFF_STEPS_MS = [60 * 1000, 120 * 1000, 300 * 1000];
export const FOCUS_REFRESH_MIN_INTERVAL_MS = 30 * 1000;

/**
 * Polls the unread notification count while the tab is visible and the user
 * is authenticated. Pauses on hidden, refreshes immediately on visibility
 * regain / window focus (throttled), backs off on consecutive errors
 * (60s → 120s → 300s cap, reset on success), and stops on logout/unmount.
 *
 * @param {boolean} enabled - mount behind an auth check
 */
const useNotificationPolling = (enabled) => {
  const dispatch = useDispatch();
  const isAuthenticated = useSelector((state) => state.auth.isAuthenticated);
  const lastFetchedAt = useSelector((state) => state.notifications.lastFetchedAt);
  const lastFetchedAtRef = useRef(lastFetchedAt);
  lastFetchedAtRef.current = lastFetchedAt;

  const timerRef = useRef(null);
  const errorCountRef = useRef(0);
  const inFlightRef = useRef(false);
  const activeRef = useRef(false);

  useEffect(() => {
    if (!enabled || !isAuthenticated) return undefined;

    const clearTimer = () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    const poll = async () => {
      if (inFlightRef.current) return;
      inFlightRef.current = true;
      try {
        await dispatch(fetchUnreadCount()).unwrap();
        errorCountRef.current = 0;
      } catch (e) {
        errorCountRef.current += 1;
      } finally {
        inFlightRef.current = false;
      }
      if (activeRef.current) {
        const idx = Math.min(errorCountRef.current, POLL_BACKOFF_STEPS_MS.length - 1);
        const delay = errorCountRef.current === 0
          ? POLL_INTERVAL_MS
          : POLL_BACKOFF_STEPS_MS[idx];
        clearTimer();
        timerRef.current = setTimeout(poll, delay);
      }
    };

    const start = () => {
      activeRef.current = true;
      clearTimer();
      timerRef.current = setTimeout(poll, POLL_INTERVAL_MS);
    };

    const stop = () => {
      activeRef.current = false;
      clearTimer();
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        // Immediate refresh (throttled) then restart the interval.
        const since = Date.now() - lastFetchedAtRef.current;
        if (lastFetchedAtRef.current === 0 || since >= FOCUS_REFRESH_MIN_INTERVAL_MS) {
          poll();
        }
        start();
      } else {
        stop();
      }
    };

    const handleFocus = () => {
      if (document.visibilityState !== 'visible') return;
      const since = Date.now() - lastFetchedAtRef.current;
      if (lastFetchedAtRef.current === 0 || since >= FOCUS_REFRESH_MIN_INTERVAL_MS) {
        poll();
      }
    };

    start();
    document.addEventListener('visibilitychange', handleVisibilityChange);
    window.addEventListener('focus', handleFocus);

    return () => {
      stop();
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('focus', handleFocus);
    };
  }, [enabled, isAuthenticated, dispatch]);
};

export default useNotificationPolling;
