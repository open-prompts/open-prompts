import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { getTemplates } from '../services/api';

const STORAGE_KEY = 'home.selectedVisibility';
const HIDE_WARNED_KEY = 'home.hideWarned';
const SORT_WHITELIST = ['newest', 'most_liked', 'most_favorited', 'trending', 'top_rated'];

const normalizeSort = (value) =>
  SORT_WHITELIST.includes(value) ? value : 'newest';

const parseTagsParam = (value) =>
  value
    ? value
        .split(',')
        .map((tag) => tag.trim())
        .filter(Boolean)
    : [];

/**
 * Central state owner for the template list: keyword/category/tags/sort/
 * visibility filters, URL sync, debounced keyword, request race guarding,
 * pagination, and result counts.
 *
 * @param {Object} options
 * @param {number} options.pageSize - page size for list requests
 * @param {boolean} options.isAuthenticated - auth state (drives default visibility)
 * @param {string} options.currentUserId - used for "my templates" owner filter
 */
const useTemplateFilters = ({ pageSize = 20, isAuthenticated = false, currentUserId = '' } = {}) => {
  const [searchParams, setSearchParams] = useSearchParams();

  // Restore order: URL -> localStorage visibility -> auth-state default.
  const [filters, setFilters] = useState(() => {
    const urlVisibility = searchParams.get('visibility') || '';
    const saved =
      typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null;
    const defaultVisibility = urlVisibility || saved || (isAuthenticated ? '' : 'VISIBILITY_PUBLIC');
    // exclude_warned: URL (?hideWarned=1) -> localStorage (logged-in) -> off.
    const urlHideWarned = searchParams.get('hideWarned') === '1';
    const savedHideWarned =
      typeof window !== 'undefined' ? window.localStorage.getItem(HIDE_WARNED_KEY) === '1' : false;
    return {
      keyword: searchParams.get('q') || '',
      category: searchParams.get('category') || '',
      tags: parseTagsParam(searchParams.get('tags')),
      sort: normalizeSort(searchParams.get('sort') || ''),
      visibility: defaultVisibility,
      myLikes: false,
      myFavorites: false,
      ownerId: '',
      excludeWarned: urlHideWarned || savedHideWarned,
    };
  });

  const [templates, setTemplates] = useState([]);
  const [privateTemplates, setPrivateTemplates] = useState([]);
  const [totalCount, setTotalCount] = useState(null);
  const [privateTotalCount, setPrivateTotalCount] = useState(null);
  const [nextPageToken, setNextPageToken] = useState('');
  const [hasMore, setHasMore] = useState(true);
  const [isFirstLoading, setIsFirstLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState(null);

  const requestIdRef = useRef(0);
  const debounceTimerRef = useRef(null);
  const filtersRef = useRef(filters);
  filtersRef.current = filters;

  // Keep the persisted visibility in sync with auth changes when the user
  // has not made an explicit choice.
  useEffect(() => {
    if (!isAuthenticated) {
      setFilters((prev) =>
        prev.visibility === 'VISIBILITY_PUBLIC'
          ? prev
          : { ...prev, visibility: 'VISIBILITY_PUBLIC' }
      );
    }
  }, [isAuthenticated]);

  // Sync discrete filter changes to the URL.
  const syncUrl = useCallback(
    (next, { replace = false } = {}) => {
      const params = {};
      if (next.keyword) params.q = next.keyword;
      if (next.category) params.category = next.category;
      if (next.tags && next.tags.length > 0) params.tags = next.tags.join(',');
      if (next.sort && next.sort !== 'newest') params.sort = next.sort;
      if (next.visibility) params.visibility = next.visibility;
      if (next.excludeWarned) params.hideWarned = '1';
      setSearchParams(params, { replace });
    },
    [setSearchParams]
  );

  const persistVisibility = useCallback((visibility) => {
    try {
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(STORAGE_KEY, visibility);
      }
    } catch (e) {
      // ignore storage errors
    }
  }, []);

  const buildParams = useCallback(
    (currentFilters, pageToken) => {
      const params = {
        page_size: pageSize,
        page_token: pageToken,
        sort: currentFilters.sort,
      };
      if (currentFilters.keyword) params.keyword = currentFilters.keyword;
      if (currentFilters.category) params.category = currentFilters.category;
      if (currentFilters.tags && currentFilters.tags.length > 0) {
        params.tags = currentFilters.tags;
      }
      if (currentFilters.visibility) params.visibility = currentFilters.visibility;
      if (currentFilters.ownerId) params.owner_id = currentFilters.ownerId;
      if (currentFilters.myLikes) params.my_likes = true;
      if (currentFilters.myFavorites) params.my_favorites = true;
      if (currentFilters.excludeWarned) params.exclude_warned = true;
      return params;
    },
    [pageSize]
  );

  // Core fetch. `pageToken === ''` means a fresh query (first page).
  const fetchPage = useCallback(
    async (pageToken) => {
      const requestId = ++requestIdRef.current;
      const isFirstPage = pageToken === '';
      if (isFirstPage) {
        setIsFirstLoading(true);
      } else {
        setLoadingMore(true);
      }
      setError(null);

      try {
        const params = buildParams(filtersRef.current, pageToken);
        const response = await getTemplates(params);
        // Race guard: discard stale responses.
        if (requestId !== requestIdRef.current) return;

        const newTemplates = response.data.templates || [];
        const newPrivateTemplates = response.data.private_templates || [];

        let nextTokenToStore = '';
        if (response.data.private_next_page_token !== undefined) {
          nextTokenToStore = `${response.data.next_page_token || ''}:${
            response.data.private_next_page_token || ''
          }`;
        } else {
          nextTokenToStore = response.data.next_page_token || '';
        }

        if (isFirstPage) {
          setTemplates(newTemplates);
          setPrivateTemplates(newPrivateTemplates);
          setTotalCount(
            response.data.total_count !== undefined ? Number(response.data.total_count) : null
          );
          setPrivateTotalCount(
            response.data.private_total_count !== undefined
              ? Number(response.data.private_total_count)
              : null
          );
        } else {
          setTemplates((prev) => [...prev, ...newTemplates]);
          setPrivateTemplates((prev) => [...prev, ...newPrivateTemplates]);
        }

        setNextPageToken(nextTokenToStore);
        setHasMore(Boolean(nextTokenToStore) && nextTokenToStore !== ':');
      } catch (err) {
        if (requestId !== requestIdRef.current) return;
        setError(err);
      } finally {
        if (requestId === requestIdRef.current) {
          if (isFirstPage) setIsFirstLoading(false);
          else setLoadingMore(false);
        }
      }
    },
    [buildParams]
  );

  // Refetch whenever filters change (any filter change resets pagination).
  useEffect(() => {
    fetchPage('');
  }, [filters, fetchPage]);

  const loadMore = useCallback(() => {
    if (!hasMore || isFirstLoading || loadingMore || !nextPageToken) return;
    fetchPage(nextPageToken);
  }, [hasMore, isFirstLoading, loadingMore, nextPageToken, fetchPage]);

  const retry = useCallback(() => {
    fetchPage('');
  }, [fetchPage]);

  // --- Filter mutators ---

  const setKeyword = useCallback(
    (value) => {
      setFilters((prev) => {
        const next = { ...prev, keyword: value };
        syncUrl(next, { replace: true });
        return next;
      });
    },
    [syncUrl]
  );

  // Debounced keyword update used by the search input; Enter bypasses it.
  const debouncedSetKeyword = useCallback(
    (value) => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      debounceTimerRef.current = setTimeout(() => {
        setKeyword(value);
      }, 300);
    },
    [setKeyword]
  );

  useEffect(
    () => () => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    },
    []
  );

  const submitKeyword = useCallback(
    (value) => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
      setKeyword(value);
    },
    [setKeyword]
  );

  const setCategory = useCallback(
    (category) => {
      setFilters((prev) => {
        // Category click resets tags (existing sidebar behavior).
        const next = { ...prev, category, tags: [] };
        syncUrl(next);
        return next;
      });
    },
    [syncUrl]
  );

  const toggleTag = useCallback(
    (tag) => {
      setFilters((prev) => {
        const tags = prev.tags.includes(tag)
          ? prev.tags.filter((t) => t !== tag)
          : [...prev.tags, tag];
        const next = { ...prev, tags };
        syncUrl(next);
        return next;
      });
    },
    [syncUrl]
  );

  const setSort = useCallback(
    (sort) => {
      setFilters((prev) => {
        const next = { ...prev, sort: normalizeSort(sort) };
        syncUrl(next);
        return next;
      });
    },
    [syncUrl]
  );

  const setVisibility = useCallback(
    (visibility) => {
      persistVisibility(visibility);
      setFilters((prev) => {
        const next = { ...prev, visibility };
        syncUrl(next);
        return next;
      });
    },
    [syncUrl, persistVisibility]
  );

  // Toggle the "hide warned content" filter. Persists per user (localStorage)
  // and syncs to the URL (?hideWarned=1) for shareability (FR-6).
  const toggleHideWarned = useCallback(() => {
    setFilters((prev) => {
      const next = { ...prev, excludeWarned: !prev.excludeWarned };
      try {
        if (typeof window !== 'undefined') {
          window.localStorage.setItem(HIDE_WARNED_KEY, next.excludeWarned ? '1' : '0');
        }
      } catch (e) {
        // ignore storage errors
      }
      syncUrl(next);
      return next;
    });
  }, [syncUrl]);

  const clearAll = useCallback(() => {
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    setFilters((prev) => {
      // Visibility falls back to the persisted default.
      const saved =
        typeof window !== 'undefined' ? window.localStorage.getItem(STORAGE_KEY) : null;
      const visibility = saved !== null ? saved : prev.visibility;
      const next = {
        ...prev,
        keyword: '',
        category: '',
        tags: [],
        sort: 'newest',
        visibility,
      };
      syncUrl(next);
      return next;
    });
  }, [syncUrl]);

  // Sidebar-style partial updates (keeps the existing onFilterChange contract).
  const applyFilterChange = useCallback(
    (partial) => {
      setFilters((prev) => {
        const next = {
          ...prev,
          keyword: partial.keyword !== undefined ? partial.keyword : prev.keyword,
          category: partial.category !== undefined ? partial.category : prev.category,
          tags: partial.tags !== undefined ? partial.tags : prev.tags,
          sort: partial.sort !== undefined ? normalizeSort(partial.sort) : prev.sort,
          visibility: partial.visibility !== undefined ? partial.visibility : prev.visibility,
          ownerId: partial.owner_id !== undefined ? partial.owner_id : prev.ownerId,
          myLikes: partial.my_likes !== undefined ? partial.my_likes : prev.myLikes,
          myFavorites:
            partial.my_favorites !== undefined ? partial.my_favorites : prev.myFavorites,
        };
        if (partial.visibility !== undefined) persistVisibility(partial.visibility);
        syncUrl(next);
        return next;
      });
    },
    [syncUrl, persistVisibility]
  );

  const combinedTotalCount = useMemo(() => {
    if (totalCount === null) return null;
    return totalCount + (privateTotalCount || 0);
  }, [totalCount, privateTotalCount]);

  return {
    filters,
    setKeyword: debouncedSetKeyword,
    submitKeyword,
    toggleTag,
    setCategory,
    setSort,
    setVisibility,
    toggleHideWarned,
    clearAll,
    applyFilterChange,
    templates,
    privateTemplates,
    totalCount: combinedTotalCount,
    nextPageToken,
    hasMore,
    loadingMore,
    isFirstLoading,
    error,
    loadMore,
    retry,
  };
};

export default useTemplateFilters;
