import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { ClockIcon, AlertIcon } from '../ui/Icons';
import { formatRelativeTime } from '../../utils/time';
import './ReviewStatusCard.scss';

/**
 * Creator dashboard card for a template in pre-publication review.
 * Shows honest status (in_review / rejected), the submitted time, the
 * expected decision window, and quick actions.
 *
 * @param {Object} props.template - template with moderation_status
 */
const ReviewStatusCard = ({ template }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const rejected = template.moderation_status === 'rejected';

  const handleEdit = () => navigate(`/templates/${template.id}`);

  return (
    <div className={`review-status-card ${rejected ? 'review-status-card--rejected' : ''}`}>
      <div className="review-status-card__icon" aria-hidden="true">
        {rejected ? <AlertIcon size={18} /> : <ClockIcon size={18} />}
      </div>
      <div className="review-status-card__body">
        <span className="review-status-card__status">
          {rejected ? t('review.status.rejected') : t('review.status.inReview')}
        </span>
        <span className="review-status-card__title">{template.title}</span>
        {rejected ? (
          <span className="review-status-card__meta">
            {t('review.card.rejected_hint')}
          </span>
        ) : (
          <span className="review-status-card__meta">
            {t('review.card.submitted', {
              time: formatRelativeTime(template.updated_at, t),
            })}
            {' · '}
            {t('review.card.expected')}
          </span>
        )}
      </div>
      <div className="review-status-card__actions">
        {rejected ? (
          <button type="button" className="review-status-card__cta" onClick={handleEdit}>
            {t('review.cta.resubmit')}
          </button>
        ) : (
          <>
            <button type="button" className="review-status-card__link" onClick={handleEdit}>
              {t('review.card.view')}
            </button>
            <button type="button" className="review-status-card__link" onClick={handleEdit}>
              {t('review.card.edit')}
            </button>
          </>
        )}
      </div>
    </div>
  );
};

export default ReviewStatusCard;
