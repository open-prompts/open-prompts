import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Layout from '../components/Layout';
import CollectionCard from '../components/collections/CollectionCard';
import CollectionEmptyState from '../components/collections/CollectionEmptyState';
import { listPublicCollections } from '../services/api';
import { usePageAnnouncement } from '../utils/a11y';
import './CollectionsPage.scss';

const PAGE_SIZE = 20;

/**
 * CollectionsPage — the public explore feed at /collections. Grid of
 * CollectionCards with infinite scroll (20/page) and a sort dropdown.
 */
const CollectionsPage = () => {
  const { t } = useTranslation();
  const [collections, setCollections] = useState([]);
  const [sort, setSort] = useState('newest');
  const [pageToken, setPageToken] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const requestIdRef = useRef(0);

  usePageAnnouncement('collections.explore_title');

  const load = (nextToken, nextSort) => {
    const rid = ++requestIdRef.current;
    setLoading(true);
    setError(false);
    listPublicCollections({ page_size: PAGE_SIZE, page_token: nextToken, sort: nextSort })
      .then((res) => {
        if (rid !== requestIdRef.current) return;
        const items = res.data.collections || [];
        setCollections((prev) => (nextToken ? [...prev, ...items] : items));
        setPageToken(res.data.next_page_token || '');
        setHasMore(!!res.data.next_page_token);
        setLoading(false);
      })
      .catch(() => {
        if (rid !== requestIdRef.current) return;
        setError(true);
        setLoading(false);
      });
  };

  useEffect(() => {
    setCollections([]);
    setPageToken('');
    setHasMore(true);
    load('', sort);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sort]);

  const handleScroll = () => {
    if (loading || !hasMore) return;
    if (window.innerHeight + window.scrollY >= document.body.offsetHeight - 200) {
      load(pageToken, sort);
    }
  };

  useEffect(() => {
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, hasMore, pageToken, sort]);

  return (
    <Layout>
      <div className="collections-page">
        <div className="collections-page__header">
          <h1 className="collections-page__title">{t('collections.explore_title')}</h1>
          <label className="collections-page__sort">
            <span>{t('search.sort.label')}</span>
            <select value={sort} onChange={(e) => setSort(e.target.value)}>
              <option value="newest">{t('search.sort.newest')}</option>
              <option value="most_templates">{t('collections.sort.most_templates')}</option>
            </select>
          </label>
        </div>

        {error && <p className="collections-page__error">{t('collections.error.load')}</p>}

        {!error && collections.length === 0 && !loading && (
          <CollectionEmptyState variant="explore" />
        )}

        <div className="collections-page__grid">
          {collections.map((c) => <CollectionCard key={c.id} collection={c} />)}
        </div>

        {loading && <div className="collections-page__loading">{t('common.loading')}</div>}
      </div>
    </Layout>
  );
};

export default CollectionsPage;