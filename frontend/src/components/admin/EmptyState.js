import React from 'react';
import './EmptyState.scss';

/**
 * Empty-state placeholder for lists and tables.
 */
const EmptyState = ({ title, action }) => (
  <div className="admin-empty">
    <p className="admin-empty__title">{title}</p>
    {action}
  </div>
);

export default EmptyState;
