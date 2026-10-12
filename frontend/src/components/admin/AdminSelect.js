import React from 'react';
import { ChevronDownIcon } from '../ui/Icons';
import './AdminSelect.scss';

/**
 * Styled select dropdown for admin filters.
 */
const AdminSelect = ({ value, onChange, options, label, testId }) => (
  <div className="admin-select">
    {label && <span className="admin-select__label">{label}</span>}
    <div className="admin-select__control">
      <select value={value} onChange={onChange} aria-label={label} data-testid={testId}>
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      <ChevronDownIcon size={16} />
    </div>
  </div>
);

export default AdminSelect;
