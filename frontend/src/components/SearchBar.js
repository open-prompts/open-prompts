import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import useDismissable from '../hooks/useDismissable';
import SuggestionsDropdown from './SuggestionsDropdown';
import './SearchBar.scss';

/**
 * Search input with 300ms debounce, Enter submit, clear button, and a
 * suggestions combobox (categories then tags, max 5 rows).
 *
 * @param {Object} props
 * @param {string} props.value - current keyword
 * @param {Function} props.onChange - debounced keyword update
 * @param {Function} props.onSubmit - immediate keyword submit (Enter)
 * @param {Array} props.categories - [{ name, count }] taxonomy data
 * @param {Array} props.tags - [{ name, count }] taxonomy data
 */
const SearchBar = ({ value, onChange, onSubmit, categories = [], tags = [] }) => {
  const { t } = useTranslation();
  const [inputValue, setInputValue] = useState(value);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const inputRef = useRef(null);

  useEffect(() => {
    setInputValue(value);
  }, [value]);

  const suggestions =
    inputValue.trim().length > 0
      ? [
          ...categories
            .filter((c) => c.name.toLowerCase().includes(inputValue.trim().toLowerCase()))
            .slice(0, 3)
            .map((c) => ({ type: 'category', name: c.name })),
          ...tags
            .filter((tg) => tg.name.toLowerCase().includes(inputValue.trim().toLowerCase()))
            .slice(0, 2)
            .map((tg) => ({ type: 'tag', name: tg.name })),
        ].slice(0, 5)
      : [];

  const dismissRef = useDismissable(showSuggestions, () => {
    setShowSuggestions(false);
    setHighlightedIndex(-1);
  });

  const handleChange = (e) => {
    const next = e.target.value;
    setInputValue(next);
    setShowSuggestions(true);
    setHighlightedIndex(-1);
    onChange(next);
  };

  const handleSubmit = (keyword) => {
    if (keyword !== undefined) setInputValue(keyword);
    setShowSuggestions(false);
    setHighlightedIndex(-1);
    onSubmit(keyword !== undefined ? keyword : inputValue);
  };

  const handleClear = () => {
    setInputValue('');
    setShowSuggestions(false);
    setHighlightedIndex(-1);
    onChange('');
    onSubmit('');
    inputRef.current?.focus();
  };

  const handleKeyDown = (e) => {
    if (!showSuggestions || suggestions.length === 0) {
      if (e.key === 'Enter') {
        e.preventDefault();
        handleSubmit();
      }
      return;
    }
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setHighlightedIndex((prev) => (prev + 1) % suggestions.length);
        break;
      case 'ArrowUp':
        e.preventDefault();
        setHighlightedIndex((prev) => (prev <= 0 ? suggestions.length - 1 : prev - 1));
        break;
      case 'Enter':
        e.preventDefault();
        if (highlightedIndex >= 0) {
          handleSubmit(suggestions[highlightedIndex].name);
        } else {
          handleSubmit();
        }
        break;
      case 'Escape':
        e.preventDefault();
        setShowSuggestions(false);
        setHighlightedIndex(-1);
        break;
      default:
        break;
    }
  };

  return (
    <div className="search-bar" ref={dismissRef}>
      <div className="search-bar-input-wrap" role="search">
        <input
          ref={inputRef}
          type="text"
          className="search-bar-input"
          placeholder={t('search.placeholder')}
          value={inputValue}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onFocus={() => setShowSuggestions(true)}
          aria-label={t('search.placeholder')}
          role="combobox"
          aria-expanded={showSuggestions && suggestions.length > 0}
          aria-controls="search-suggestions-listbox"
          aria-autocomplete="list"
        />
        {inputValue && (
          <button
            type="button"
            className="search-bar-clear"
            onClick={handleClear}
            aria-label={t('search.clear')}
          >
            ×
          </button>
        )}
      </div>
      {showSuggestions && suggestions.length > 0 && (
        <SuggestionsDropdown
          suggestions={suggestions}
          highlightedIndex={highlightedIndex}
          onSelect={handleSubmit}
          onHighlight={setHighlightedIndex}
        />
      )}
    </div>
  );
};

export default SearchBar;
