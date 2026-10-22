import React from 'react';
import { useTranslation } from 'react-i18next';
import { CheckIcon } from './ui/Icons';
import './ReportedMenuItem.scss';

/**
 * ReportedMenuItem is the checkmark state of the report ⋯ menu: "You
 * reported this" with Update / Withdraw submenu (FR-7).
 *
 * @param {Object} props
 * @param {Function} props.onUpdate - reopen the modal prefilled
 * @param {Function} props.onWithdraw - open the withdraw confirm
 */
const ReportedMenuItem = ({ onUpdate, onWithdraw }) => {
  const { t } = useTranslation();
  return (
    <div className="reported-menu-item" role="menuitem">
      <span className="reported-menu-item__label">
        <CheckIcon size={15} />
        {t('reports.reported')}
      </span>
      <div className="reported-menu-item__submenu">
        <button type="button" className="reported-menu-item__action" onClick={onUpdate}>
          {t('reports.update')}
        </button>
        <button type="button" className="reported-menu-item__action" onClick={onWithdraw}>
          {t('reports.withdraw')}
        </button>
      </div>
    </div>
  );
};

export default ReportedMenuItem;