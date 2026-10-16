import React, { useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import { useNotification } from '../context/NotificationContext';
import { rateTemplate, deleteRating } from '../services/api';
import { StarIcon } from './ui/Icons';
import './RatingWidget.scss';

const STAR_HINTS = ['hint_1', 'hint_2', 'hint_3', 'hint_4', 'hint_5'];

/**
 * RatingWidget — interactive 5-star rating control.
 *
 * Keyboard: single tab stop, ←/→ adjust, Enter/Space commit, Esc cancels.
 * Optimistic fill with revert on failure. Owner view shows a non-interactive
 * hint; anonymous clicks route to /login.
 *
 * @param {Object} props
 * @param {string} props.templateId
 * @param {number} props.ratingAvg - current average (0 when unrated)
 * @param {number} props.ratingCount - number of ratings
 * @param {number} props.myRating - caller's rating (0 = unrated)
 * @param {boolean} props.isOwner - whether the viewer owns the template
 * @param {Function} [props.onChange] - called with {ratingAvg, ratingCount, myRating}
 */
const RatingWidget = ({
  templateId,
  ratingAvg = 0,
  ratingCount = 0,
  myRating = 0,
  isOwner = false,
  onChange,
}) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();
  const navigate = useNavigate();
  const user = useSelector((state) => state.auth.user);

  const [hover, setHover] = useState(0);
  const [value, setValue] = useState(myRating);
  const [saving, setSaving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [focused, setFocused] = useState(false);
  const containerRef = useRef(null);

  const display = hover || value || 0;

  const commit = async (rating) => {
    if (!user) {
      navigate('/login');
      return;
    }
    if (isOwner) return;
    if (rating === myRating) return; // no-op: already rated this value
    setSaving(true);
    const prev = value;
    setValue(rating); // optimistic
    try {
      const resp = await rateTemplate(templateId, rating);
      setValue(resp.data.my_rating);
      if (onChange) onChange(resp.data);
      addNotification({ kind: 'success', title: t('ratings.submitted', { count: rating }) });
    } catch (err) {
      setValue(prev); // revert
      if (err.response && err.response.status === 429) {
        addNotification({ kind: 'error', title: t('ratings.error'), subtitle: t('ratings.rate_limited') });
      } else {
        addNotification({ kind: 'error', title: t('ratings.error') });
      }
    } finally {
      setSaving(false);
    }
  };

  const handleClear = async () => {
    if (!user) return;
    setSaving(true);
    const prev = value;
    setValue(0); // optimistic
    setConfirmOpen(false);
    try {
      const resp = await deleteRating(templateId);
      setValue(resp.data.my_rating);
      if (onChange) onChange(resp.data);
    } catch (err) {
      setValue(prev); // revert
      addNotification({ kind: 'error', title: t('ratings.error') });
    } finally {
      setSaving(false);
    }
  };

  const handleKeyDown = (e) => {
    switch (e.key) {
      case 'ArrowLeft':
        e.preventDefault();
        setValue((v) => Math.max(0, v - 1));
        break;
      case 'ArrowRight':
        e.preventDefault();
        setValue((v) => Math.min(5, v + 1));
        break;
      case 'Enter':
      case ' ':
        e.preventDefault();
        if (value > 0) commit(value);
        break;
      case 'Escape':
        e.preventDefault();
        setValue(myRating);
        setHover(0);
        break;
      default:
        break;
    }
  };

  if (isOwner) {
    return (
      <div className="rating-widget rating-widget--owner">
        <span className="rating-widget__hint">{t('ratings.owner_hint')}</span>
      </div>
    );
  }

  return (
    <div
      className="rating-widget"
      ref={containerRef}
      onMouseLeave={() => setHover(0)}
    >
      <div
        className="rating-widget__stars"
        role="slider"
        aria-label={t('ratings.rate')}
        aria-valuemin={0}
        aria-valuemax={5}
        aria-valuenow={value}
        aria-valuetext={value ? t('ratings.you_rated', { count: value }) : t('ratings.no_ratings')}
        tabIndex={0}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onKeyDown={handleKeyDown}
      >
        {[1, 2, 3, 4, 5].map((i) => (
          <button
            key={i}
            type="button"
            className={`rating-widget__star ${i <= display ? 'active' : ''}`}
            aria-label={t(`ratings.${STAR_HINTS[i - 1]}`)}
            aria-hidden={focused}
            tabIndex={-1}
            onMouseEnter={() => setHover(i)}
            onClick={() => commit(i)}
            disabled={saving}
          >
            <StarIcon size={22} filled={i <= display} />
          </button>
        ))}
      </div>
      <div className="rating-widget__meta">
        <span className="rating-widget__avg">
          {ratingCount > 0 ? ratingAvg.toFixed(2) : t('ratings.be_first')}
        </span>
        {ratingCount > 0 && (
          <span className="rating-widget__count">
            {t('ratings.ratings_count', { count: ratingCount })}
          </span>
        )}
        {value > 0 && (
          <button
            type="button"
            className="rating-widget__clear"
            onClick={() => setConfirmOpen((o) => !o)}
          >
            {t('ratings.clear')}
          </button>
        )}
      </div>
      {confirmOpen && (
        <div className="rating-widget__confirm" role="alertdialog" aria-label={t('ratings.clear_confirm')}>
          <span>{t('ratings.clear_confirm')}</span>
          <button type="button" className="rating-widget__confirm-yes" onClick={handleClear}>
            {t('common.confirmation')}
          </button>
          <button type="button" className="rating-widget__confirm-no" onClick={() => setConfirmOpen(false)}>
            {t('common.cancel')}
          </button>
        </div>
      )}
    </div>
  );
};

export default RatingWidget;