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
import { getCategories, getTags, getFollowingFeed, getTopCreators } from '../services/api';
import UserCard from '../components/UserCard';
import ImportButton from '../components/ImportButton';
import CollectionPicker from '../components/collections/CollectionPicker';
import { usePageAnnouncement } from '../utils/a11y';
import './Home.scss';

/**
 * Home page component.
 * Displays a grid of prompt templates with search, filtering, sorting,
 * and infinite scroll. List state is owned by useTemplateFilters.
 */
const Home = () => {
  const { t, i18n } = useTranslation();
  const { isAuthenticated, user } = useSelector((state) => state.auth);
  const [taxonomy, setTaxonomy] = useState({ categories: [], tags: [] });
  const [activeTab, setActiveTab] = useState('browse');

  // Following feed state (only used when the Following tab is active).
  const [feedTemplates, setFeedTemplates] = useState([]);
  const [feedNextToken, setFeedNextToken] = useState('');
  const [feedLoading, setFeedLoading] = useState(false);
  const [feedError, setFeedError] = useState(false);
  const [suggested, setSuggested] = useState([]);
  const [pickerTemplate, setPickerTemplate] = useState(null);

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

  // Load the following feed when the Following tab is active.
  useEffect(() => {
    if (activeTab !== 'following' || !isAuthenticated) return;
    let cancelled = false;
    setFeedLoading(true);
    setFeedError(false);
    const loadFeed = async () => {
      try {
        const resp = await getFollowingFeed({ page_size: 20 });
        if (!cancelled) {
          setFeedTemplates(resp.data.templates || []);
          setFeedNextToken(resp.data.next_page_token || '');
        }
      } catch (err) {
        if (!cancelled) setFeedError(true);
      } finally {
        if (!cancelled) setFeedLoading(false);
      }
    };
    loadFeed();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isAuthenticated]);

  // Load top-5 suggested creators for the empty-feed state.
  useEffect(() => {
    if (activeTab !== 'following' || !isAuthenticated) return;
    let cancelled = false;
    const loadSuggested = async () => {
      try {
        const resp = await getTopCreators({ limit: 5 });
        if (!cancelled) setSuggested(resp.data.users || []);
      } catch (err) {
        // suggestions are non-critical
      }
    };
    loadSuggested();
    return () => {
      cancelled = true;
    };
  }, [activeTab, isAuthenticated]);

  const handleLoadMoreFeed = async () => {
    if (!feedNextToken || feedLoading) return;
    setFeedLoading(true);
    try {
      const resp = await getFollowingFeed({ page_size: 20, page_token: feedNextToken });
      setFeedTemplates((prev) => [...prev, ...(resp.data.templates || [])]);
      setFeedNextToken(resp.data.next_page_token || '');
    } catch (err) {
      setFeedError(true);
    } finally {
      setFeedLoading(false);
    }
  };

  const handleTabChange = (next) => {
    setActiveTab(next);
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
        <nav className="home-tabs" role="tablist" aria-label={t('home.title')}>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'browse'}
            className={`home-tab ${activeTab === 'browse' ? 'is-active' : ''}`}
            onClick={() => handleTabChange('browse')}
          >
            {t('home.browse_tab')}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'following'}
            className={`home-tab ${activeTab === 'following' ? 'is-active' : ''}`}
            onClick={() => handleTabChange('following')}
          >
            {t('home.following.tab')}
          </button>
        </nav>

        {activeTab === 'following' ? (
          <div className="home-following">
            {!isAuthenticated ? (
              <div className="home-following__login-prompt">
                <p>{t('home.following.login_prompt')}</p>
              </div>
            ) : (
              <>
                {feedError && (
                  <div className="home-error-banner" role="alert">
                    <span>{t('search.error.fetch')}</span>
                  </div>
                )}
                {feedLoading && feedTemplates.length === 0 && <SkeletonCards />}
                {!feedLoading && feedTemplates.length === 0 && (
                  <div className="home-following__empty">
                    <EmptyState title={t('home.following.empty')} />
                    {suggested.length > 0 && (
                      <div className="home-following__suggested">
                        <h3>{t('home.following.suggested')}</h3>
                        <div className="creator-profile__user-list">
                          {suggested.map((u) => <UserCard key={u.id} user={u} locale={i18n.language || 'en'} />)}
                        </div>
                      </div>
                    )}
                  </div>
                )}
                {feedTemplates.length > 0 && (
                  <>
                    <div className="templates-grid">
                      {feedTemplates.map((template) => (
                        <PromptCard key={template.id} template={template} onAddToCollection={setPickerTemplate} />
                      ))}
                    </div>
                    {feedNextToken && (
                      <button type="button" className="home-following__load-more" onClick={handleLoadMoreFeed} disabled={feedLoading}>
                        {feedLoading ? t('common.loading') : t('notifications.load_more')}
                      </button>
                    )}
                  </>
                )}
              </>
            )}
          </div>
        ) : (
        <>
        <div className="home-search-row">
          <SearchBar
            value={filters.keyword}
            onChange={setKeyword}
            onSubmit={submitKeyword}
            categories={taxonomy.categories}
            tags={taxonomy.tags}
          />
          <SortDropdown value={filters.sort} onChange={setSort} />
          {isAuthenticated && <ImportButton />}
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
                  <PromptCard key={template.id} template={template} onAddToCollection={setPickerTemplate} />
                ))}
              </div>
            )}

            <div className="templates-grid">
              {templates.map((template) => (
                <PromptCard key={template.id} template={template} onAddToCollection={setPickerTemplate} />
              ))}
            </div>

            {!error && templates.length === 0 && privateTemplates.length === 0 && (
              <EmptyState keyword={filters.keyword} onClearAll={clearAll} />
            )}

            {loadingMore && <div className="loading">{t('common.loading')}</div>}
          </>
        )}
        </>
        )}
      </div>
      {pickerTemplate && (
        <CollectionPicker template={pickerTemplate} onClose={() => setPickerTemplate(null)} />
      )}
    </Layout>
  );
};

export default Home;
