import React from 'react';
import { useTranslation } from 'react-i18next';
import { LockIcon } from '../ui/Icons';
import './WarningCheckboxGroup.scss';

const DEFAULT_TAXONOMY = ['mature', 'violence', 'profanity', 'sensitive'];

/**
 * WarningCheckboxGroup — multi-select from the warning taxonomy. Locked
 * (moderator-applied) warnings render as disabled, checked chips with a lock
 * icon and tooltip.
 *
 * @param {Object} props
 * @param {string[]} props.value - currently selected warning keys
 * @param {string[]} [props.locked] - moderator-applied (creator-immutable) keys
 * @param {string[]} [props.taxonomy] - available keys (defaults to the built-in 4)
 * @param {Function} props.onChange - (next: string[]) => void
 * @param {string} [props.lockedReason] - reason text for the locked tooltip
 */
const WarningCheckboxGroup = ({
  value = [],
  locked = [],
  taxonomy = DEFAULT_TAXONOMY,
  onChange,
  lockedReason = '',
}) => {
  const { t } = useTranslation();

  const toggle = (key) => {
    if (locked.includes(key)) return; // creator cannot change locked warnings
    const next = value.includes(key) ? value.filter((k) => k !== key) : [...value, key];
    onChange(next);
  };

  return (
    <fieldset className="warning-checkbox-group">
      <legend className="warning-checkbox-group__title">{t('warnings.group.title')}</legend>
      <p className="warning-checkbox-group__help">{t('warnings.group.help')}</p>
      <div className="warning-checkbox-group__options">
        {taxonomy.map((key) => {
          const isLocked = locked.includes(key);
          const checked = value.includes(key);
          return (
            <label
              key={key}
              className={`warning-checkbox-group__option ${isLocked ? 'is-locked' : ''}`}
              title={isLocked ? t('warnings.locked.label') : undefined}
            >
              <input
                type="checkbox"
                checked={checked}
                disabled={isLocked}
                onChange={() => toggle(key)}
                aria-disabled={isLocked}
              />
              <span className="warning-checkbox-group__name">
                {t(`warnings.type.${key}`, { defaultValue: key })}
              </span>
              {isLocked && (
                <span
                  className="warning-checkbox-group__lock"
                  title={lockedReason || t('warnings.locked.label')}
                >
                  <LockIcon size={12} />
                </span>
              )}
            </label>
          );
        })}
      </div>
      {locked.length > 0 && (
        <p className="warning-checkbox-group__locked-note">
          {t('warnings.locked.label')}
        </p>
      )}
    </fieldset>
  );
};

export default WarningCheckboxGroup;