import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useNotification } from '../context/NotificationContext';
import { followUser, unfollowUser } from '../services/api';
import './FollowButton.scss';

/**
 * FollowButton — optimistic follow/unfollow toggle with revert on error.
 * Following → Unfollow (danger) on hover/focus; no confirm modal.
 *
 * @param {Object} props
 * @param {string} props.userId - the creator to follow/unfollow
 * @param {boolean} props.initialFollowing - current follow state
 * @param {Function} [props.onChange] - (following) => void after a successful toggle
 * @param {string} [props.className] - extra class for sizing variants
 */
const FollowButton = ({ userId, initialFollowing = false, onChange, className = '' }) => {
  const { t } = useTranslation();
  const { isAuthenticated } = useSelector((state) => state.auth);
  const { addNotification } = useNotification();
  const navigate = useNavigate();
  const [following, setFollowing] = useState(!!initialFollowing);
  const [busy, setBusy] = useState(false);

  const handleClick = async () => {
    if (!isAuthenticated) {
      navigate('/login');
      return;
    }
    if (busy) return;
    const prev = following;
    setFollowing(!prev);
    setBusy(true);
    try {
      if (prev) {
        await unfollowUser(userId);
      } else {
        await followUser(userId);
      }
      if (onChange) onChange(!prev);
    } catch (err) {
      setFollowing(prev); // revert on error
      const status = err.response && err.response.status;
      if (status === 403) {
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('follow.error_banned') });
      } else {
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('follow.error') });
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <button
      type="button"
      className={`follow-button ${following ? 'is-following' : ''} ${className}`}
      onClick={handleClick}
      aria-pressed={following}
      disabled={busy}
      title={following ? t('follow.unfollow') : t('follow.follow')}
    >
      {following ? t('follow.following') : t('follow.follow')}
    </button>
  );
};

export default FollowButton;