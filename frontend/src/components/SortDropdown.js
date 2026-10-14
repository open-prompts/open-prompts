import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import useDismissable from '../hooks/useDismissable';
import './SortDropdown.scss';

const OPTIONS = ['newest', 'most_liked', 'most_favorited', 'trending'];

/**
 * Custom sort dropdown with listbox keyboard pattern (↑↓ Enter Esc).
 *
 * @param {Object} props
 * @param {string} props.value - current sort slug
 * @param {Function} props.onChange - called with the new sort slug
 */
const SortDropdown = ({ value, onChange }) => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(
    Math.max(0, OPTIONS.indexOf(value))
  );

  const dismissRef = useDismissable(open, () => setOpen(false));

  const close = () => {
    setOpen(false);
    setHighlightedIndex(Math.max(0, OPTIONS.indexOf(value)));
  };

  const select = (option) => {
    onChange(option);
    close();
  };

  const handleKeyDown = (e) => {
    switch (e.key) {
      case 'Enter':
      case ' ':
        e.preventDefault();
        if (open) {
          select(OPTIONS[highlightedIndex]);
        } else {
          setOpen(true);
        }
        break;
      case 'ArrowDown':
        e.preventDefault();
        if (open) {
          setHighlightedIndex((prev) => (prev + 1) % OPTIONS.length);
        } else {
          setOpen(true);
        }
        break;
      case 'ArrowUp':
        e.preventDefault();
        if (open) {
          setHighlightedIndex((prev) => (prev <= 0 ? OPTIONS.length - 1 : prev - 1));
        }
        break;
      case 'Escape':
        e.preventDefault();
        close();
        break;
      default:
        break;
    }
  };

  return (
    <div className="sort-dropdown" ref={dismissRef}>
      <button
        type="button"
        className="sort-dropdown-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => (open ? close() : setOpen(true))}
        onKeyDown={handleKeyDown}
      >
        <span className="sort-dropdown-label">{t('search.sort.label')}</span>
        <span className="sort-dropdown-value">{t(`search.sort.${value}`)}</span>
        <span className={`sort-dropdown-arrow ${open ? 'open' : ''}`} aria-hidden="true">▾</span>
      </button>
      {open && (
        <ul className="sort-dropdown-menu" role="listbox" aria-label={t('search.sort.label')}>
          {OPTIONS.map((option, index) => (
            <li
              key={option}
              role="option"
              aria-selected={option === value}
              className={`sort-dropdown-option ${
                option === value ? 'selected' : ''
              } ${index === highlightedIndex ? 'highlighted' : ''}`}
              onMouseEnter={() => setHighlightedIndex(index)}
              onMouseDown={(e) => {
                e.preventDefault();
                select(option);
              }}
            >
              {t(`search.sort.${option}`)}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default SortDropdown;
