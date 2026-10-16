import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

/**
 * QuickActions — New template / New API key buttons on the overview.
 * @param {Object} props
 * @param {Function} props.onNewTemplate - opens the create-template modal
 */
const QuickActions = ({ onNewTemplate }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  return (
    <div className="quick-actions">
      <button type="button" className="btn btn-primary" onClick={onNewTemplate}>
        {t('dashboard_v2.quick.new_template')}
      </button>
      <button
        type="button"
        className="btn btn-ghost"
        onClick={() => navigate('/api-keys')}
      >
        {t('dashboard_v2.quick.new_api_key')}
      </button>
    </div>
  );
};

export default QuickActions;