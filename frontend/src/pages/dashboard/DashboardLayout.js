import React, { useEffect, useState } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate, NavLink, Outlet, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Layout from '../../components/Layout';
import { hasCollections } from '../../utils/collections';
import './DashboardLayout.scss';

/**
 * DashboardLayout — the authenticated user's personal workspace shell.
 * Self-guards: redirects to /login?next=… when unauthenticated. Renders a
 * DashboardNav plus the nested page via <Outlet/>.
 */
const DashboardLayout = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { token } = useSelector((state) => state.auth);
  const [collectionsAvailable, setCollectionsAvailable] = useState(true);

  useEffect(() => {
    if (!token) {
      navigate(`/login?next=${encodeURIComponent(location.pathname)}`);
    }
  }, [token, navigate, location.pathname]);

  useEffect(() => {
    let mounted = true;
    hasCollections().then((ok) => {
      if (mounted) setCollectionsAvailable(ok);
    });
    return () => {
      mounted = false;
    };
  }, []);

  return (
    <Layout showCreateButton={false}>
      <div className="dashboard-layout">
        <nav className="dashboard-nav" aria-label={t('dashboard_v2.title')}>
          <NavLink to="/dashboard" end className="dashboard-nav__link">
            {t('dashboard_v2.nav.overview')}
          </NavLink>
          <NavLink to="/dashboard/templates" className="dashboard-nav__link">
            {t('dashboard_v2.nav.templates')}
          </NavLink>
          <NavLink to="/dashboard/prompts" className="dashboard-nav__link">
            {t('dashboard_v2.nav.prompts')}
          </NavLink>
          {collectionsAvailable && (
            <NavLink to="/collections/mine" className="dashboard-nav__link">
              {t('dashboard_v2.nav.collections')}
            </NavLink>
          )}
          <NavLink to="/api-keys" className="dashboard-nav__link">
            {t('dashboard_v2.nav.api_keys')}
          </NavLink>
        </nav>
        <main className="dashboard-content">
          <Outlet />
        </main>
      </div>
    </Layout>
  );
};

export default DashboardLayout;