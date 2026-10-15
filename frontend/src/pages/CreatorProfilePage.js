import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import Layout from '../components/Layout';
import PromptCard from '../components/PromptCard';
import UserCard from '../components/UserCard';
import FollowButton from '../components/FollowButton';
import EmptyState from '../components/EmptyState';
import { getCreatorProfile, getCreatorTemplates, getFollowers, getFollowing } from '../services/api';
import { formatCount } from '../utils/formatCount';
import { usePageAnnouncement } from '../utils/a11y';
import './CreatorProfilePage.scss';

const PAGE_SIZE = 20;

/**
 * Public creator profile: header (avatar, name, bio, Follows-you badge,
 * counts, joined date) + tabs Templates / Followers / Following. Own profile
 * shows an "Edit profile" link to /profile.
 */
const CreatorProfilePage = () => {
  const { id } = useParams();
  const { t, i18n } = useTranslation();
  const { user: currentUser } = useSelector((state) => state.auth);
  const lang = i18n.language || 'en';

  const [profile, setProfile] = useState(null);
  const [tab, setTab] = useState('templates');
  const [templates, setTemplates] = useState([]);
  const [users, setUsers] = useState([]);
  const [totalCount, setTotalCount] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState('');

  usePageAnnouncement('creators.title');

  const isOwn = currentUser && currentUser.id === id;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setNotFound(false);
    setError('');
    setProfile(null);
    setTemplates([]);
    setUsers([]);
    setTotalCount(0);
    setPage(1);
    setTab('templates');

    const loadProfile = async () => {
      try {
        const resp = await getCreatorProfile(id);
        if (!cancelled) setProfile(resp.data);
      } catch (err) {
        if (!cancelled) {
          const status = err.response && err.response.status;
          if (status === 404) setNotFound(true);
          else setError((err.response && err.response.data && err.response.data.error) || t('creators.error_load'));
        }
      }
    };
    loadProfile();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // Load the active tab's data.
  useEffect(() => {
    if (!profile) return;
    let cancelled = false;
    setLoading(true);
    const loadTab = async () => {
      try {
        if (tab === 'templates') {
          const resp = await getCreatorTemplates(id, { page_size: PAGE_SIZE, page_token: page > 1 ? String((page - 1) * PAGE_SIZE) : '' });
          if (!cancelled) {
            setTemplates(resp.data.templates || []);
            setTotalCount(resp.data.total_count || 0);
          }
        } else if (tab === 'followers') {
          const resp = await getFollowers(id, { page, page_size: PAGE_SIZE });
          if (!cancelled) {
            setUsers(resp.data.users || []);
            setTotalCount(resp.data.total_count || 0);
          }
        } else if (tab === 'following') {
          const resp = await getFollowing(id, { page, page_size: PAGE_SIZE });
          if (!cancelled) {
            setUsers(resp.data.users || []);
            setTotalCount(resp.data.total_count || 0);
          }
        }
      } catch (err) {
        if (!cancelled) setError((err.response && err.response.data && err.response.data.error) || t('creators.error_load'));
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    loadTab();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, tab, page, profile]);

  const handleTabChange = (next) => {
    setPage(1);
    setTab(next);
  };

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE));

  if (notFound) {
    return (
      <Layout showSidebar={false}>
        <div className="creator-profile creator-profile--notfound">
          <h1 className="visually-hidden">{t('creators.title')}</h1>
          <EmptyState title={t('creators.not_found')} />
        </div>
      </Layout>
    );
  }

  return (
    <Layout showSidebar={false}>
      <div className="creator-profile">
        <h1 className="visually-hidden">{t('creators.title')}</h1>
        {error && <div className="creator-profile__error" role="alert">{error}</div>}
        {profile && (
          <>
            <header className="creator-profile__header">
              <div className="creator-profile__avatar" aria-hidden="true">
                {profile.avatar ? (
                  <img src={profile.avatar} alt="" />
                ) : (
                  <span className="creator-profile__avatar-fallback">{(profile.display_name || profile.id || '?').charAt(0).toUpperCase()}</span>
                )}
              </div>
              <div className="creator-profile__identity">
                <div className="creator-profile__name-row">
                  <h2 className="creator-profile__name">{profile.display_name || profile.id}</h2>
                  {profile.follows_you && (
                    <span className="creator-profile__follows-you">{t('creators.follows_you')}</span>
                  )}
                </div>
                {profile.bio && <p className="creator-profile__bio">{profile.bio}</p>}
                <div className="creator-profile__meta">
                  <span>{t('creators.followers', { count: formatCount(profile.follower_count || 0, lang) })}</span>
                  <span>·</span>
                  <span>{t('creators.following_count', { count: formatCount(profile.following_count || 0, lang) })}</span>
                  {profile.created_at && (
                    <>
                      <span>·</span>
                      <span>{t('creators.joined', { date: new Date(profile.created_at).toLocaleDateString(lang) })}</span>
                    </>
                  )}
                </div>
              </div>
              <div className="creator-profile__actions">
                {isOwn ? (
                  <Link to="/profile" className="creator-profile__edit-btn">{t('creators.edit_profile')}</Link>
                ) : (
                  <FollowButton userId={profile.id} initialFollowing={!!profile.is_following} />
                )}
              </div>
            </header>

            <nav className="creator-profile__tabs" role="tablist" aria-label={t('creators.title')}>
              <button
                type="button"
                role="tab"
                aria-selected={tab === 'templates'}
                className={`creator-profile__tab ${tab === 'templates' ? 'is-active' : ''}`}
                onClick={() => handleTabChange('templates')}
              >
                {t('creators.tab_templates')}
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={tab === 'followers'}
                className={`creator-profile__tab ${tab === 'followers' ? 'is-active' : ''}`}
                onClick={() => handleTabChange('followers')}
              >
                {t('creators.tab_followers')}
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={tab === 'following'}
                className={`creator-profile__tab ${tab === 'following' ? 'is-active' : ''}`}
                onClick={() => handleTabChange('following')}
              >
                {t('creators.tab_following')}
              </button>
            </nav>

            <div className="creator-profile__content">
              {loading && <div className="creator-profile__loading">{t('common.loading')}</div>}
              {!loading && tab === 'templates' && (
                <>
                  {templates.length === 0 ? (
                    <EmptyState title={t('creators.empty_templates')} />
                  ) : (
                    <div className="templates-grid">
                      {templates.map((tpl) => <PromptCard key={tpl.id} template={tpl} />)}
                    </div>
                  )}
                </>
              )}
              {!loading && tab === 'followers' && (
                <>
                  {users.length === 0 ? (
                    <EmptyState title={t('creators.empty_followers')} />
                  ) : (
                    <div className="creator-profile__user-list">
                      {users.map((u) => <UserCard key={u.id} user={u} locale={lang} />)}
                    </div>
                  )}
                </>
              )}
              {!loading && tab === 'following' && (
                <>
                  {users.length === 0 ? (
                    <EmptyState title={t('creators.empty_following')} />
                  ) : (
                    <div className="creator-profile__user-list">
                      {users.map((u) => <UserCard key={u.id} user={u} locale={lang} />)}
                    </div>
                  )}
                </>
              )}
              {!loading && totalPages > 1 && (
                <div className="creator-profile__pagination">
                  <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>
                    {t('admin.common.prev')}
                  </button>
                  <span>{t('admin.common.page', { page, total: totalPages })}</span>
                  <button type="button" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
                    {t('admin.common.next')}
                  </button>
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </Layout>
  );
};

export default CreatorProfilePage;