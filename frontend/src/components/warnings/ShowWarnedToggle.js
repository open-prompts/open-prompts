import React from 'react';
import { useTranslation } from 'react-i18next';
import './ShowWarnedToggle.scss';

/**
 * ShowWarnedToggle — settings switch that persists show_warned_content
 * server-side on the user (FR-5).
 *
 * @param {Object} props
 * @param {boolean} props.checked - current value
 * @param {boolean} props.saving - in-flight save state
 * @param {Function} props.onChange - (next: boolean) => void
 */
const ShowWarnedToggle = ({ checked = false, saving = false, onChange }) => {
  const { t } = useTranslation();
  return (
    <div className="show-warned-toggle">
      <label className="show-warned-toggle__row">
        <span className="show-warned-toggle__text">
          <span className="show-warned-toggle__title">{t('warnings.settings.showAlways')}</span>
          <span className="show-warned-toggle__help">{t('warnings.settings.showAlwaysHelp')}</span>
        </span>
        <input
          type="checkbox"
          className="show-warned-toggle__switch"
          checked={checked}
          disabled={saving}
          onChange={(e) => onChange(e.target.checked)}
        />
      </label>
      {saving && <span className="show-warned-toggle__saving">{t('common.saving')}</span>}
    </div>
  );
};

export default ShowWarnedToggle;