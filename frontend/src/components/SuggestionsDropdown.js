import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Combobox listbox of search suggestions (categories then tags).
 *
 * @param {Object} props
 * @param {Array} props.suggestions - [{ type: 'category'|'tag', name }]
 * @param {number} props.highlightedIndex - keyboard-highlighted row
 * @param {Function} props.onSelect - called with the suggestion name
 * @param {Function} props.onHighlight - hover/keyboard highlight sync
 */
const SuggestionsDropdown = ({ suggestions, highlightedIndex, onSelect, onHighlight }) => {
  const { t } = useTranslation();

  return (
    <ul
      className="suggestions-dropdown"
      id="search-suggestions-listbox"
      role="listbox"
    >
      {suggestions.map((suggestion, index) => (
        <li
          key={`${suggestion.type}-${suggestion.name}`}
          role="option"
          aria-selected={index === highlightedIndex}
          className={`suggestions-item ${index === highlightedIndex ? 'highlighted' : ''}`}
          onMouseEnter={() => onHighlight(index)}
          onMouseDown={(e) => {
            e.preventDefault();
            onSelect(suggestion.name);
          }}
        >
          <span className="suggestions-type">
            {suggestion.type === 'category'
              ? t('search.suggestions.category')
              : t('search.suggestions.tag')}
          </span>
          <span className="suggestions-name">{suggestion.name}</span>
        </li>
      ))}
    </ul>
  );
};

export default SuggestionsDropdown;
