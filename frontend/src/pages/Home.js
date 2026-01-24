import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { useNotification } from '../context/NotificationContext';
import Layout from '../components/Layout';
import PromptCard from '../components/PromptCard';
import { getTemplates } from '../services/api';
import './Home.scss';

/**
 * Home page component.
 * Displays a grid of prompt templates with filtering and infinite scroll.
 */
const Home = () => {
  const { t, i18n } = useTranslation();
  const { addNotification } = useNotification();
  const { isAuthenticated } = useSelector((state) => state.auth);
  const [templates, setTemplates] = useState([]);
  const [privateTemplates, setPrivateTemplates] = useState([]);
  const STORAGE_KEY = 'home.selectedVisibility';
  const [filters, setFilters] = useState(() => {
    const saved = typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null;
    return {
      visibility: saved !== null ? saved : 'VISIBILITY_PUBLIC', // Default to public unless user had a saved choice
      category: '',
      tags: [],
    };
  });
  const [nextPageToken, setNextPageToken] = useState('');
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const pageSize = 20;

  // Update default filters on auth change
  useEffect(() => {
    if (isAuthenticated) {
      // If logged in, default to mixed view (no visibility filter) only when
      // user has not previously chosen a visibility tab (persisted in localStorage).
      const saved = typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null;
      if (saved === null) {
        setFilters(prev => ({ ...prev, visibility: '' }));
      }
    } else {
      setFilters(prev => ({ ...prev, visibility: 'VISIBILITY_PUBLIC' }));
    }
  }, [isAuthenticated]);

  // Fetch templates when filters or page changes
  const fetchTemplates = useCallback(async (isNewFilter = false) => {
    if (loading) return;
    setLoading(true);

    try {
      // If new filter, use empty token. Else use stored next token.
      const currentToken = isNewFilter ? '' : nextPageToken;

      const params = {
        page_size: pageSize,
        page_token: currentToken,
        language: (i18n.language || 'en').startsWith('zh') ? 'zh' : 'en',
        ...filters,
      };

      // Debug: log outgoing params to help diagnose "My templates" filter issues
      // eslint-disable-next-line no-console
      console.debug('Fetching templates with params:', params);
      const response = await getTemplates(params);
      // Debug: log server response
      // eslint-disable-next-line no-console
      console.debug('Templates response:', response && response.data ? response.data : response);

      // Handle Public Templates (legacy field 'templates')
      const newTemplates = response.data.templates || [];
      // Handle Private Templates (new field)
      const newPrivateTemplates = response.data.private_templates || [];

      // Backend returns next_page_token for mixed view (combined) or simple view
      // Just store it.
      // const newNextToken = response.data.next_page_token || "";
      // Note: Backend logic puts combined token in next_page_token?
      // Let's check backend logic again.
      // Yes: return &pb.ListTemplatesResponse{ ..., NextPageToken: nextPublicToken (Wait!) }
      // I made a mistake in backend. I returned nextPublicToken as NextPageToken.
      // And nextPrivateToken as PrivateNextPageToken.
      // If I want "independent pagination" but a single "Load More" trigger,
      // I should combine them in the frontend or backend.
      // My backend returned specific tokens.
      // Frontend needs to combine them to send back in `page_token`.
      // Format: "public:private"

      let nextTokenToStore = "";
      if (response.data.private_next_page_token) {
          // Mixed mode logic check:
          // Backend should have returned combined token if it wanted client to be opaque?
          // I used `strings.Split` in backend.
          // So I should construct the token here.
          // const pToken = response.data.next_page_token || (newTemplates.length ? "0" : ""); // unsafe assumption?
          // If backend returns "" it means end of list.
          // If backend returns token, it's the offset.
          // My backend logic:
          // nextPublicToken = strconv.Itoa(offset + limit) if more.
          // So if I receive tokens, I combine them.
           nextTokenToStore = `${response.data.next_page_token || ''}:${response.data.private_next_page_token || ''}`;
      } else {
          // Single list mode
          nextTokenToStore = response.data.next_page_token || "";
      }

      if (isNewFilter) {
        setTemplates(newTemplates);
        setPrivateTemplates(newPrivateTemplates);
      } else {
        setTemplates((prev) => [...prev, ...newTemplates]);
        setPrivateTemplates((prev) => [...prev, ...newPrivateTemplates]);
      }

      setNextPageToken(nextTokenToStore);

      // Has More Logic
      // If mixed view, has more if EITHER has token.
      // If single view, has more if token exists.
      if (nextTokenToStore && nextTokenToStore !== ":") {
        setHasMore(true);
      } else {
        setHasMore(false);
      }

    } catch (error) {
      console.error('Failed to fetch templates:', error);
      addNotification({
        kind: 'error',
        title: t('common.error'),
        subtitle: t('home.error_fetch'),
      });
    } finally {
      setLoading(false);
    }
  }, [filters, nextPageToken, loading, pageSize, i18n.language]);

  // Initial load and filter changes
  useEffect(() => {
    fetchTemplates(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters, i18n.language]);
  // removed page dep. added filters.

  // Handle filter changes from Sidebar
  const handleFilterChange = (newFilters) => {
    setFilters((prev) => {
      const merged = { ...prev, ...newFilters };
      try {
        if (typeof window !== 'undefined' && merged.visibility !== undefined) {
          window.localStorage.setItem(STORAGE_KEY, merged.visibility);
        }
      } catch (e) {
        // ignore storage errors
      }
      return merged;
    });
  };

  // Refresh list trigger (can be passed to children if needed)
  const refreshList = () => {
    fetchTemplates(true);
  };

  // Infinite scroll handler
  const handleScroll = (e) => {
    const { scrollTop, clientHeight, scrollHeight } = e.target;
    if (scrollHeight - scrollTop <= clientHeight + 50 && hasMore && !loading) {
      fetchTemplates(false);
    }
  };

  const isMixedView = !filters.visibility && isAuthenticated && !filters.my_likes && !filters.my_favorites;
  const isSpecialView = filters.my_likes || filters.my_favorites;

  // Calculate tags from visible templates for Sidebar
  const visibleTags = useMemo(() => {
    const allTemplates = [...templates, ...privateTemplates];
    const tagCounts = {};
    allTemplates.forEach(t => {
      if (t.tags && Array.isArray(t.tags)) {
        t.tags.forEach(tag => {
          tagCounts[tag] = (tagCounts[tag] || 0) + 1;
        });
      }
    });
    // Convert to array format expected by Sidebar [{ name, count }]
    return Object.entries(tagCounts)
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count);
  }, [templates, privateTemplates]);

  return (
    <Layout
      onFilterChange={handleFilterChange}
      currentFilters={filters}
      onCreateSuccess={refreshList}
      availableTags={visibleTags}
    >
      <div className="home-page" onScroll={handleScroll}>

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

        {loading && <div className="loading">{t('common.loading')}</div>}
        {!loading && templates.length === 0 && privateTemplates.length === 0 && (
          <div className="no-results">{t('home.no_results')}</div>
        )}
      </div>
    </Layout>
  );
};

export default Home;
