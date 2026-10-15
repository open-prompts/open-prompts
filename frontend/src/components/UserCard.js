import React from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import FollowButton from './FollowButton';
import { formatCount } from '../utils/formatCount';
import './UserCard.scss';

/**
 * UserCard — avatar + display name + follower count + FollowButton, linking
 * to the creator profile page.
 *
 * @param {Object} props
 * @param {Object} props.user - { id, display_name, avatar, follower_count, is_following }
 * @param {string} [props.locale] - for count abbreviation
 */
const UserCard = ({ user, locale }) => {
  const { t } = useTranslation();
  if (!user) return null;
  return (
    <div className="user-card">
      <Link to={`/creators/${user.id}`} className="user-card__link">
        <span className="user-card__avatar" aria-hidden="true">
          {user.avatar ? (
            <img src={user.avatar} alt="" />
          ) : (
            <span className="user-card__avatar-fallback">{(user.display_name || user.id || '?').charAt(0).toUpperCase()}</span>
          )}
        </span>
        <span className="user-card__info">
          <span className="user-card__name">{user.display_name || user.id}</span>
          <span className="user-card__count">
            {t('creators.followers', { count: formatCount(user.follower_count || 0, locale) })}
          </span>
        </span>
      </Link>
      <FollowButton userId={user.id} initialFollowing={!!user.is_following} />
    </div>
  );
};

export default UserCard;