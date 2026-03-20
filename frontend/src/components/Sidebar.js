import React, { useEffect, useState } from 'react';
import { useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { useNotification } from '../context/NotificationContext';
import { getCategories, getTags } from '../services/api';
import './Sidebar.scss';

/**
 * Sidebar component.
 * Displays navigation for Prompt Square, My Prompts, and Tag Cloud.
 * @param {Object} props - Component props
 * @param {Function} props.onFilterChange - Callback when a filter is selected
 * @param {Object} props.currentFilters - Current active filters
 * @param {Array} props.availableTags - Tags to display (overrides fetching)
 */
const Sidebar = ({ onFilterChange, currentFilters, availableTags, mobileOpen, onClose }) => {
  const { t, i18n } = useTranslation();
  const { addNotification } = useNotification();
  const { isAuthenticated, user } = useSelector((state) => state.auth);
  const [categories, setCategories] = useState([]);
  const [tags, setTags] = useState([]);
  const [showAllCategories, setShowAllCategories] = useState(false);

  const handleSelection = (callback) => {
    if (callback) callback();
    if (onClose && window.innerWidth <= 768) {
        onClose();
    }
  };

  useEffect(() => {
    // Fetch categories and tags on mount
    const fetchData = async () => {
      const language = (i18n.language || 'en').startsWith('zh') ? 'zh' : 'en';
      try {
        const [catRes, tagRes] = await Promise.all([
             getCategories({ language }),
             availableTags ? Promise.resolve({ data: { tags: [] } }) : getTags({ language })
        ]);
        setCategories(catRes.data.categories || []);
        if (!availableTags) {
             setTags(tagRes.data.tags || []);
        }
      } catch (error) {
        console.error('Failed to fetch sidebar data:', error);
        addNotification({ kind: 'warning', title: t('common.warning'), subtitle: t('sidebar.error_fetch') });
      }
    };
    fetchData();
  }, [availableTags, i18n.language, addNotification, t]); // Re-run if availableTags or language changes

  // Use availableTags if present, otherwise fetched tags.
  const displayedTags = availableTags || tags;

  // Limit categories to top 10 unless "More" is clicked
  const displayedCategories = showAllCategories ? categories : categories.slice(0, 10);
  // const displayedPrivateCategories = privateCategories.slice(0, 10); // Assume limit for private too

  const handleCategoryClick = (category, visibility) => {
    handleSelection(() => {
        onFilterChange({
            category,
            visibility,
            tags: [],
            owner_id: '',
            my_likes: false,
            my_favorites: false
        });
    });
  };

  const handleTagClick = (tag) => {
    handleSelection(() => {
        onFilterChange({
            tags: [tag],
            my_likes: false,
            my_favorites: false
        });
    });
  };

  const handleSpecialFilterClick = (type) => {
    handleSelection(() => {
      if (type === 'likes') {
          onFilterChange({
            category: '',
            tags: [],
            visibility: '', // Search all visibilities
            owner_id: '',
            my_likes: true,
            my_favorites: false
          });
      } else if (type === 'favorites') {
          onFilterChange({
            category: '',
            tags: [],
            visibility: '',
            owner_id: '',
            my_likes: false,
            my_favorites: true
          });
      }
    });
  };

  const handleMyAllClick = () => {
    handleSelection(() => {
      onFilterChange({
        category: '',
        tags: [],
        visibility: '',
        owner_id: user?.id,
        my_likes: false,
        my_favorites: false
      });
    });
  };

  const isActive = (type, value, visibility) => {
    if (!currentFilters) return false;
    if (type === 'all-public') return currentFilters.visibility === 'VISIBILITY_PUBLIC' && !currentFilters.category && !currentFilters.my_likes && !currentFilters.my_favorites;
    if (type === 'all-mine') return !currentFilters.visibility && currentFilters.owner_id === user?.id && !currentFilters.my_likes && !currentFilters.my_favorites;
    if (type === 'category') return currentFilters.category === value && currentFilters.visibility === visibility;
    if (type === 'tag') return currentFilters.tags && currentFilters.tags.includes(value);
    if (type === 'likes') return !!currentFilters.my_likes;
    if (type === 'favorites') return !!currentFilters.my_favorites;
    return false;
  };

  return (
    <aside className={`app-sidebar ${mobileOpen ? 'mobile-open' : ''}`}>
      <div className="sidebar-section">
        <h3>{t('dashboard.title')}</h3>
        <ul>
          <li
            className={isActive('all-public') ? 'active' : ''}
            onClick={() => handleCategoryClick(null, 'VISIBILITY_PUBLIC')}
          >
            {t('create_template.visibility_public')}
          </li>
          {displayedCategories.map((cat) => (
            <li
              key={cat.name}
              className={isActive('category', cat.name, 'VISIBILITY_PUBLIC') ? 'active' : ''}
              onClick={() => handleCategoryClick(cat.name, 'VISIBILITY_PUBLIC')}
            >
              {cat.name} <span className="count">({cat.count})</span>
            </li>
          ))}
        </ul>
        {categories.length > 10 && !showAllCategories && (
          <button className="btn-more" onClick={() => setShowAllCategories(true)}>
            {t('common.more')}
          </button>
        )}
      </div>

      {isAuthenticated && (
        <div className="sidebar-section">
          <h3>{t('dashboard.my_templates')}</h3>
          <ul>
            <li
              className={isActive('all-mine') ? 'active' : ''}
              onClick={handleMyAllClick}
            >
              {t('dashboard.all_mine')}
            </li>
            <li
              className={isActive('likes') ? 'active' : ''}
              onClick={() => handleSpecialFilterClick('likes')}
            >
              {t('dashboard.my_likes')}
            </li>
            <li
              className={isActive('favorites') ? 'active' : ''}
              onClick={() => handleSpecialFilterClick('favorites')}
            >
              {t('dashboard.favorites')}
            </li>
            {/* Private Categories could go here if we filtered them separately,
                but typically they are just subsets of My Prompts */}
          </ul>
        </div>
      )}


      <div className="sidebar-section">
        <h3>{t('create_template.label_tags')}</h3>
        <div className="tag-cloud">
          {displayedTags.map((tag) => (
            <span
              key={tag.name}
              className={`tag ${isActive('tag', tag.name) ? 'active' : ''}`}
              onClick={() => handleTagClick(tag.name)}
            >
              {tag.name} <small>{tag.count > 0 ? `(${tag.count})` : ''}</small>
            </span>
          ))}
          {displayedTags.length === 0 && <span className="no-tags">{t('sidebar.no_tags')}</span>}
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;
