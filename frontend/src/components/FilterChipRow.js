import React from 'react';
import { useTranslation } from 'react-i18next';
import './FilterChipRow.scss';

/**
 * Removable chips for each active filter plus a "Clear all" button.
 *
 * @param {Object} props
 * @param {Object} props.filters - { keyword, category, tags, sort }
 * @param {Function} props.onRemoveKeyword
 * @param {Function} props.onRemoveCategory
 * @param {Function} props.onRemoveTag - called with the tag name
 * @param {Function} props.onRemoveSort
 * @param {Function} props.onClearAll
 */
const FilterChipRow = ({
  filters,
  onRemoveKeyword,
  onRemoveCategory,
  onRemoveTag,
  onRemoveSort,
  onClearAll,
}) => {
  const { t } = useTranslation();

  const chips = [];
  if (filters.keyword) {
    chips.push({
      key: 'keyword',
      label: `"${filters.keyword}"`,
      remove: onRemoveKeyword,
    });
  }
  if (filters.category) {
    chips.push({ key: 'category', label: filters.category, remove: onRemoveCategory });
  }
  filters.tags.forEach((tag) => {
    chips.push({ key: `tag-${tag}`, label: `#${tag}`, remove: () => onRemoveTag(tag) });
  });
  if (filters.sort && filters.sort !== 'newest') {
    chips.push({
      key: 'sort',
      label: t(`search.sort.${filters.sort}`),
      remove: onRemoveSort,
    });
  }

  if (chips.length === 0) return null;

  return (
    <div className="filter-chip-row" role="group" aria-label={t('search.chips.clear_all')}>
      {chips.map((chip) => (
        <span className="filter-chip" key={chip.key}>
          <span className="filter-chip-label">{chip.label}</span>
          <button
            type="button"
            className="filter-chip-remove"
            onClick={chip.remove}
            aria-label={t('search.chips.remove_filter', { label: chip.label })}
          >
            ×
          </button>
        </span>
      ))}
      <button type="button" className="filter-chip-clear-all" onClick={onClearAll}>
        {t('search.chips.clear_all')}
      </button>
    </div>
  );
};

export default FilterChipRow;
