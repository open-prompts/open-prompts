import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { SearchIcon, CloseIcon } from '../ui/Icons';
import './AdminSearchInput.scss';

/**
 * Search input with a 300ms debounce and a clear button.
 */
const AdminSearchInput = ({ value, onChange, placeholder, label }) => {
  const { t } = useTranslation();
  const [inner, setInner] = useState(value);
  const timerRef = useRef(null);
  const valueRef = useRef(value);

  // Keep the local input in sync when the URL state changes externally
  // (e.g. "Clear filters").
  useEffect(() => {
    if (value !== valueRef.current) {
      setInner(value);
      valueRef.current = value;
    }
  }, [value]);

  const handleChange = (e) => {
    const v = e.target.value;
    setInner(v);
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      valueRef.current = v;
      onChange(v);
    }, 300);
  };

  const handleClear = () => {
    setInner('');
    if (timerRef.current) clearTimeout(timerRef.current);
    valueRef.current = '';
    onChange('');
  };

  return (
    <div className="admin-search">
      <SearchIcon size={16} />
      <input
        type="text"
        value={inner}
        onChange={handleChange}
        placeholder={placeholder}
        aria-label={label || placeholder || t('admin.users.search_placeholder')}
      />
      {inner && (
        <button
          type="button"
          className="admin-search__clear"
          onClick={handleClear}
          aria-label={t('admin.users.clear_filters')}
        >
          <CloseIcon size={14} />
        </button>
      )}
    </div>
  );
};

export default AdminSearchInput;
