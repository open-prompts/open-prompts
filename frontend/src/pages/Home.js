import React, { useEffect, useState } from 'react';
import { useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import Layout from '../components/Layout';
import PromptCard from '../components/PromptCard';
import SearchBar from '../components/SearchBar';
import SortDropdown from '../components/SortDropdown';
import FilterChipRow from '../components/FilterChipRow';
import ResultCount from '../components/ResultCount';
import EmptyState from '../components/EmptyState';
import SkeletonCards from '../components/SkeletonCards';
import ReviewStatusCard from '../components/review/ReviewStatusCard';
import HideWarnedFilterChip from '../components/warnings/HideWarnedFilterChip';
import useTemplateFilters from '../hooks/useTemplateFilters';
import { getCategories, getTags } from '../services/api';
import { usePageAnnouncement } from '../utils/a11y';
import './Home.scss';

/**
 * Home page component.
 * Displays a grid of prompt templates with search, filtering, sorting,
 * and infinite scroll. List state is owned by useTemplateFilters.
 */
const Home = () => {
  const { t } = useTranslation();
  const { isAuthenticated, user } = useSelector((state) => state.auth);
  const [taxonomy, setTaxonomy] = useState({ categories: [], tags: [] });

  usePageAnnouncement('home.title');

  const {
    filters,
    setKeyword,
    submitKeyword,
    toggleTag,
    setCategory,
    setSort,
    toggleHideWarned,
    clearAll,
    applyFilterChange,
    templates,
    privateTemplates,
    totalCount,
    hasMore,
    loadingMore,
    isFirstLoading,
    error,
    loadMore,
    retry,
  } = useTemplateFilters({
    pageSize: 20,
    isAuthenticated,
    currentUserId: user?.id || '',
  });

  // Fetch shared taxonomy (categories + tags) once for the sidebar and the
  // search suggestions dropdown.
  useEffect(() => {
    let cancelled = false;
    const fetchTaxonomy = async () => {
      try {
        const [catRes, tagRes] = await Promise.all([getCategories(), getTags()]);
        if (!cancelled) {
          setTaxonomy({
            categories: catRes.data.categories || [],
            tags: tagRes.data.tags || [],
          });
        }
      } catch (err) {
        // Taxonomy is non-critical; suggestions simply stay empty.
        console.error('Failed to fetch taxonomy:', err);
      }
    };
    fetchTaxonomy();
    return () => {
      cancelled = true;
    };
  }, []);

  // Infinite scroll handler
  const handleScroll = (e) => {
    const { scrollTop, clientHeight, scrollHeight } = e.target;
    if (scrollHeight - scrollTop <= clientHeight + 50 && hasMore && !loadingMore && !isFirstLoading) {
      loadMore();
    }
  };

  const handleRemoveKeyword = () => submitKeyword('');
  const handleRemoveCategory = () => setCategory('');
  const handleRemoveSort = () => setSort('newest');

  // Own templates currently in pre-publication review (or rejected):
  // surfaced as status cards above the grid so the creator always knows
  // where their submissions stand.
  const reviewTemplates = React.useMemo(
    () =>
      isAuthenticated
        ? [...privateTemplates, ...templates].filter(
            (tpl) =>
              tpl &&
              tpl.owner_id === (user && user.id) &&
              (tpl.moderation_status === 'in_review' || tpl.moderation_status === 'rejected')
          )
        : [],
    [isAuthenticated, privateTemplates, templates, user]
  );

  return (
    <Layout
      onFilterChange={applyFilterChange}
      currentFilters={{
        category: filters.category,
        visibility: filters.visibility,
        tags: filters.tags,
        owner_id: filters.ownerId,
        my_likes: filters.myLikes,
        my_favorites: filters.myFavorites,
      }}
      categories={taxonomy.categories}
      availableTags={taxonomy.tags}
    >
      <div className="home-page" onScroll={handleScroll}>
        <h1 className="visually-hidden">{t('home.title')}</h1>
        <div className="home-search-row">
          <SearchBar
            value={filters.keyword}
            onChange={setKeyword}
            onSubmit={submitKeyword}
            categories={taxonomy.categories}
            tags={taxonomy.tags}
          />
          <SortDropdown value={filters.sort} onChange={setSort} />
        </div>

        <FilterChipRow
          filters={filters}
          onRemoveKeyword={handleRemoveKeyword}
          onRemoveCategory={handleRemoveCategory}
          onRemoveTag={toggleTag}
          onRemoveSort={handleRemoveSort}
          onClearAll={clearAll}
        />

        <HideWarnedFilterChip active={filters.excludeWarned} onToggle={toggleHideWarned} />

        <ResultCount count={totalCount} />

        {error && !isFirstLoading && (
          <div className="home-error-banner" role="alert">
            <span>{t('search.error.fetch')}</span>
            <button type="button" onClick={retry}>
              {t('search.error.retry')}
            </button>
          </div>
        )}

        {isFirstLoading && <SkeletonCards />}

        {!isFirstLoading && (
          <>
            {reviewTemplates.length > 0 && (
              <div className="home-review-status">
                {reviewTemplates.map((tpl) => (
                  <ReviewStatusCard key={tpl.id} template={tpl} />
                ))}
              </div>
            )}

            {privateTemplates.length > 0 && (
              <div className="templates-grid private-grid">
                {privateTemplates.map((template) => (
                  <PromptCard key={template.id} template={template} />
                ))}
              </div>
            )}

            <div className="templates-grid">
              {templates.map((template) => (
                <PromptCard key={template.id} template={template} />
              ))}
            </div>

            {!error && templates.length === 0 && privateTemplates.length === 0 && (
              <EmptyState keyword={filters.keyword} onClearAll={clearAll} />
            )}

            {loadingMore && <div className="loading">{t('common.loading')}</div>}
          </>
        )}
      </div>
    </Layout>
  );
};

export default Home;
