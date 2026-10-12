import React from 'react';
import './KpiCard.scss';

/**
 * Dashboard KPI card with optional click-through navigation.
 */
const KpiCard = ({ label, value, onClick, testId }) => (
  <button
    type="button"
    className="admin-kpi"
    onClick={onClick}
    disabled={!onClick}
    data-testid={testId}
  >
    <span className="admin-kpi__label">{label}</span>
    <span className="admin-kpi__value">{value}</span>
  </button>
);

export default KpiCard;
