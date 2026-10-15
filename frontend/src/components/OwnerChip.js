import React from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import FollowButton from './FollowButton';
import { formatCount } from '../utils/formatCount';
import './OwnerChip.scss';

/**
 * OwnerChip — avatar + name + follower count on TemplateDetails, linking to
 * the creator page. The FollowButton is a separate stop target.
 *
 * @param {Object} props
 * @param {Object} props.owner - { id, display_name, avatar, follower_count, is_following }
 * @param {string} [props.locale]
 */
const OwnerChip = ({ owner, locale }) => {
  const { t } = useTranslation();
  if (!owner) return null;
  return (
    <div className="owner-chip">
      <Link to={`/creators/${owner.id}`} className="owner-chip__link">
        <span className="owner-chip__avatar" aria-hidden="true">
          {owner.avatar ? (
            <img src={owner.avatar} alt="" />
          ) : (
            <span className="owner-chip__avatar-fallback">{(owner.display_name || owner.id || '?').charAt(0).toUpperCase()}</span>
          )}
        </span>
        <span className="owner-chip__info">
          <span className="owner-chip__name">{owner.display_name || owner.id}</span>
          <span className="owner-chip__count">
            {t('creators.followers', { count: formatCount(owner.follower_count || 0, locale) })}
          </span>
        </span>
      </Link>
      <FollowButton userId={owner.id} initialFollowing={!!owner.is_following} />
    </div>
  );
};

export default OwnerChip;