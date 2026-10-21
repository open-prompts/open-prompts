import React from 'react';
import { useTranslation } from 'react-i18next';
import './AppealStateChip.scss';

const STATE_TONES = {
  open: 'open',
  under_review: 'under-review',
  resolved_upheld: 'upheld',
  resolved_reversed: 'reversed',
};

/**
 * Appeal state chip: open / under review / upheld / reversed with text
 * labels (color is never the only cue).
 */
const AppealStateChip = ({ state, superseded = false }) => {
  const { t } = useTranslation();
  if (superseded) {
    return <span className="appeal-chip appeal-chip--superseded">{t('appeal.superseded')}</span>;
  }
  const tone = STATE_TONES[state] || 'open';
  const label = t(`appeal.state.${state}`, { defaultValue: state });
  return <span className={`appeal-chip appeal-chip--${tone}`}>{label}</span>;
};

export default AppealStateChip;