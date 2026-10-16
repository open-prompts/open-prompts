import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import { useNotification } from '../context/NotificationContext';
import { toggleTemplateLike, toggleTemplateFavorite } from '../services/api';
import { HeartIcon, StarIcon } from './ui/Icons';
import BlurredPreview from './warnings/BlurredPreview';
import './PromptCard.scss';

/**
 * PromptCard component.
 * Displays a single prompt template in a card format.
 * @param {Object} props - Component props
 * @param {Object} props.template - The template data
 */
const PromptCard = ({ template, onAddToCollection }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();
  const navigate = useNavigate();
  const user = useSelector((state) => state.auth.user);
  const [liked, setLiked] = useState(template.is_liked);
  const [likeCount, setLikeCount] = useState(template.like_count || 0);
  const [favorited, setFavorited] = useState(template.is_favorited);
  const [favCount, setFavCount] = useState(template.favorite_count || 0);

  // Moderation badge only for the owner's own gated templates.
  const showModeration =
    user &&
    template.owner_id === user.id &&
    (template.moderation_status === 'in_review' || template.moderation_status === 'rejected');

  // Content warnings: blur the preview when the template has warnings and the
  // viewer has not opted out (show_warned_content) or revealed this session.
  const warnings = template.content_warnings || [];
  const showWarnedContent = user && user.show_warned_content;
  const hasWarnings = warnings.length > 0;
  const shouldGate = hasWarnings && !showWarnedContent;

  // "Add to collection" is hidden for private templates the viewer cannot
  // add (private-and-not-ownable).
  const showAddToCollection =
    onAddToCollection &&
    (template.visibility === 'VISIBILITY_PUBLIC' ||
      (user && template.owner_id === user.id));

  const handleLike = async (e) => {
    e.stopPropagation();
    try {
      const resp = await toggleTemplateLike(template.id);
      setLiked(resp.data.is_liked);
      setLikeCount(resp.data.like_count);
    } catch (err) {
      console.error('Failed to toggle like', err);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('prompt_card.error_like') });
    }
  };

  const handleFavorite = async (e) => {
    e.stopPropagation();
    try {
      const resp = await toggleTemplateFavorite(template.id);
      setFavorited(resp.data.is_favorited);
      setFavCount(resp.data.favorite_count);
    } catch (err) {
      console.error('Failed to toggle favorite', err);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('prompt_card.error_favorite') });
    }
  };

  return (
    <div className="prompt-card">
      <button
        type="button"
        className="prompt-card-overlay"
        aria-label={template.title}
        onClick={() => navigate(`/templates/${template.id}`)}
      />
      <div className="card-header">
        <h4 className="title">{template.title}</h4>
        <span className={`badge ${template.visibility === 'VISIBILITY_PUBLIC' ? 'badge-public' : 'badge-private'}`}>
          {template.visibility === 'VISIBILITY_PUBLIC' ? t('create_template.visibility_public') : t('create_template.visibility_private')}
        </span>
        {showModeration && (
          <span className={`badge badge-moderation-${template.moderation_status}`}>
            {template.moderation_status === 'in_review'
              ? t('review.status.inReview')
              : t('review.status.rejected')}
          </span>
        )}
      </div>
      <p className="card-description">{template.description}</p>
      {template.latest_version && (
        <div className="content-preview">
          {shouldGate ? (
            <BlurredPreview warnings={warnings}>
              <pre>{template.latest_version.content}</pre>
            </BlurredPreview>
          ) : (
            <pre>{template.latest_version.content}</pre>
          )}
        </div>
      )}
      <div className="card-footer">
        <div className="tags">
          {template.tags && template.tags.map((tag) => (
            <span key={tag} className="tag">#{tag}</span>
          ))}
        </div>
        <div className="stats">
          {template.owner_display_name && (
            <button
              type="button"
              className="stat-item owner-byline"
              onClick={(e) => {
                e.stopPropagation();
                navigate(`/creators/${template.owner_id}`);
              }}
              title={t('template_details.by', { owner: template.owner_display_name })}
            >
              {t('template_details.by', { owner: template.owner_display_name })}
            </button>
          )}
          <button
            type="button"
            className="stat-item"
            onClick={handleLike}
            aria-pressed={!!liked}
            aria-label={t('template_details.like')}
            title={t('template_details.like')}
          >
            <HeartIcon size={15} filled={liked} className="emoji" /> {likeCount}
          </button>
          <button
            type="button"
            className="stat-item"
            onClick={handleFavorite}
            aria-pressed={!!favorited}
            aria-label={t('template_details.favorite')}
            title={t('template_details.favorite')}
          >
            <StarIcon size={15} filled={favorited} className="emoji" /> {favCount}
          </button>
          {showAddToCollection && (
            <button
              type="button"
              className="stat-item"
              onClick={(e) => {
                e.stopPropagation();
                onAddToCollection(template);
              }}
              aria-label={t('collections.add_to')}
              title={t('collections.add_to')}
            >
              {t('collections.add_to')}
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

export default PromptCard;
