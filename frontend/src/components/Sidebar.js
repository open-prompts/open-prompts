import React, { useEffect, useState } from 'react';
import { useSelector } from 'react-redux';
import { Link, useNavigate } from 'react-router-dom';
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
 * @param {Array} props.categories - Shared taxonomy categories (overrides fetching)
 */
const Sidebar = ({ onFilterChange, currentFilters, availableTags, categories: sharedCategories, mobileOpen, onClose }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addNotification } = useNotification();
  const { isAuthenticated, user } = useSelector((state) => state.auth);
  const [categories, setCategories] = useState([]);
  const [tags, setTags] = useState([]);
  const [showAllCategories, setShowAllCategories] = useState(false);

  // When the sidebar is rendered outside the browse page (e.g. dashboard,
  // collections, analytics), there is no onFilterChange handler. Category,
  // tag and special-filter selections then navigate back to the browse page,
  // carrying the filter through the URL so it is applied there.
  const applyOrNavigate = (partial) => {
    if (onFilterChange) {
      onFilterChange(partial);
      return;
    }
    const params = new URLSearchParams();
    if (partial.category) params.set('category', partial.category);
    if (partial.tags && partial.tags.length > 0) params.set('tags', partial.tags.join(','));
    if (partial.visibility) params.set('visibility', partial.visibility);
    if (partial.my_likes) params.set('my_likes', '1');
    if (partial.my_favorites) params.set('my_favorites', '1');
    const qs = params.toString();
    navigate(qs ? `/?${qs}` : '/');
  };

  const handleSelection = (callback) => {
    if (callback) callback();
    if (onClose && window.innerWidth <= 768) {
        onClose();
    }
  };

  useEffect(() => {
    // Fetch categories and tags on mount unless shared taxonomy data is provided.
    if (sharedCategories) {
      setCategories(sharedCategories);
      return undefined;
    }
    const fetchData = async () => {
      // Note: no language filter on purpose — categories and tags are shared
      // across UI languages. The backend still supports an explicit language
      // filter for callers who want it.
      try {
        const [catRes, tagRes] = await Promise.all([
             getCategories(),
             availableTags ? Promise.resolve({ data: { tags: [] } }) : getTags()
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
  }, [sharedCategories, availableTags, addNotification, t]); // Re-run if shared data changes. Language switch intentionally does not refetch.

  // Use availableTags if present, otherwise fetched tags.
  const displayedTags = availableTags || tags;

  // Limit categories to top 10 unless "More" is clicked
  const displayedCategories = showAllCategories ? categories : categories.slice(0, 10);
  // const displayedPrivateCategories = privateCategories.slice(0, 10); // Assume limit for private too

  const handleCategoryClick = (category, visibility) => {
    handleSelection(() => {
        applyOrNavigate({
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
        const currentTags = currentFilters?.tags || [];
        const nextTags = currentTags.includes(tag)
          ? currentTags.filter((tg) => tg !== tag)
          : [...currentTags, tag];
        applyOrNavigate({
            tags: nextTags,
            my_likes: false,
            my_favorites: false
        });
    });
  };

  const handleSpecialFilterClick = (type) => {
    handleSelection(() => {
      if (type === 'likes') {
          applyOrNavigate({
            category: '',
            tags: [],
            visibility: '', // Search all visibilities
            owner_id: '',
            my_likes: true,
            my_favorites: false
          });
      } else if (type === 'favorites') {
          applyOrNavigate({
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
      applyOrNavigate({
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
          <li className={isActive('all-public') ? 'active' : ''}>
            <button
              type="button"
              className="sidebar-item"
              aria-current={isActive('all-public') ? 'true' : undefined}
              onClick={() => handleCategoryClick(null, 'VISIBILITY_PUBLIC')}
            >
              {t('create_template.visibility_public')}
            </button>
          </li>
          {displayedCategories.map((cat) => (
            <li
              key={cat.name}
              className={isActive('category', cat.name, 'VISIBILITY_PUBLIC') ? 'active' : ''}
            >
              <button
                type="button"
                className="sidebar-item"
                aria-current={isActive('category', cat.name, 'VISIBILITY_PUBLIC') ? 'true' : undefined}
                onClick={() => handleCategoryClick(cat.name, 'VISIBILITY_PUBLIC')}
              >
                {cat.name} <span className="count">({cat.count})</span>
              </button>
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
          <h3>{t('dashboard_v2.title')}</h3>
          <ul>
            <li>
              <Link to="/dashboard" className="sidebar-item">
                {t('dashboard_v2.nav.overview')}
              </Link>
            </li>
          </ul>
        </div>
      )}

      {isAuthenticated && (
        <div className="sidebar-section">
          <h3>{t('dashboard.my_templates')}</h3>
          <ul>
            <li className={isActive('all-mine') ? 'active' : ''}>
              <button
                type="button"
                className="sidebar-item"
                aria-current={isActive('all-mine') ? 'true' : undefined}
                onClick={handleMyAllClick}
              >
                {t('dashboard.all_mine')}
              </button>
            </li>
            <li className={isActive('likes') ? 'active' : ''}>
              <button
                type="button"
                className="sidebar-item"
                aria-current={isActive('likes') ? 'true' : undefined}
                onClick={() => handleSpecialFilterClick('likes')}
              >
                {t('dashboard.my_likes')}
              </button>
            </li>
            <li className={isActive('favorites') ? 'active' : ''}>
              <button
                type="button"
                className="sidebar-item"
                aria-current={isActive('favorites') ? 'true' : undefined}
                onClick={() => handleSpecialFilterClick('favorites')}
              >
                {t('dashboard.favorites')}
              </button>
            </li>
            <li>
              <Link to="/collections/mine" className="sidebar-item">
                {t('collections.nav_mine')}
              </Link>
            </li>
            <li>
              <Link to="/analytics" className="sidebar-item">
                {t('analytics.title')}
              </Link>
            </li>
            {/* Private Categories could go here if we filtered them separately,
                but typically they are just subsets of My Prompts */}
          </ul>
        </div>
      )}

      {!isAuthenticated && (
        <div className="sidebar-section">
          <h3>{t('collections.nav_explore')}</h3>
          <ul>
            <li>
              <Link to="/collections" className="sidebar-item">
                {t('collections.nav_explore')}
              </Link>
            </li>
          </ul>
        </div>
      )}


      <div className="sidebar-section">
        <h3>{t('create_template.label_tags')}</h3>
        <div className="tag-cloud">
          {displayedTags.map((tag) => {
            const active = isActive('tag', tag.name);
            return (
              <label
                key={tag.name}
                className={`tag tag-toggle ${active ? 'active' : ''}`}
              >
                <input
                  type="checkbox"
                  checked={active}
                  onChange={() => handleTagClick(tag.name)}
                  aria-label={tag.name}
                />
                <span className="tag-name">{tag.name}</span>
                <small>{tag.count > 0 ? `(${tag.count})` : ''}</small>
              </label>
            );
          })}
          {displayedTags.length === 0 && <span className="no-tags">{t('sidebar.no_tags')}</span>}
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;
