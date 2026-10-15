import React, { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertIcon } from '../ui/Icons';
import './ContentInterstitial.scss';

// sessionStorage key prefix for per-template reveal state (OQ-3).
const revealKey = (templateId) => `warned_revealed_${templateId}`;

/**
 * ContentInterstitial — full-width panel shown before a warned template's
 * content. Lists the reasons, offers "Show content" / "Go back". Reveal is
 * session-scoped (sessionStorage keyed by template id). Focus-trapped;
 * Escape = back.
 *
 * @param {Object} props
 * @param {string} props.templateId - template id (reveal key)
 * @param {string[]} props.warnings - taxonomy keys
 * @param {Function} props.onReveal - called when the viewer reveals content
 * @param {Function} props.onBack - called on "Go back" / Escape
 */
const ContentInterstitial = ({ templateId, warnings = [], onReveal, onBack }) => {
  const { t } = useTranslation();
  const showBtnRef = useRef(null);

  // Focus the "Show content" button on mount (initial focus).
  useEffect(() => {
    if (showBtnRef.current) showBtnRef.current.focus();
  }, []);

  const handleShow = () => {
    try {
      sessionStorage.setItem(revealKey(templateId), '1');
    } catch (e) {
      // ignore storage errors
    }
    if (onReveal) onReveal();
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      if (onBack) onBack();
    }
  };

  const names = warnings.map((w) => t(`warnings.type.${w}`, { defaultValue: w }));

  return (
    <div
      className="content-interstitial"
      role="dialog"
      aria-modal="false"
      aria-label={t('warnings.interstitial.title')}
      onKeyDown={handleKeyDown}
    >
      <div className="content-interstitial__icon" aria-hidden="true">
        <AlertIcon size={28} />
      </div>
      <h2 className="content-interstitial__title">{t('warnings.interstitial.title')}</h2>
      <p className="content-interstitial__body">
        {t('warnings.interstitial.body', { reasons: names.join(', ') })}
      </p>
      <p className="content-interstitial__subtext">{t('warnings.interstitial.subtext')}</p>
      <div className="content-interstitial__actions">
        <button
          ref={showBtnRef}
          type="button"
          className="btn btn-primary"
          onClick={handleShow}
        >
          {t('warnings.interstitial.show')}
        </button>
        <button type="button" className="btn btn-ghost" onClick={onBack}>
          {t('warnings.interstitial.back')}
        </button>
      </div>
    </div>
  );
};

// Helper to check whether a template has been revealed this session.
export const isWarnedRevealed = (templateId) => {
  try {
    return sessionStorage.getItem(revealKey(templateId)) === '1';
  } catch (e) {
    return false;
  }
};

export default ContentInterstitial;