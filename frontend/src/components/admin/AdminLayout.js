import React, { useState, useEffect } from 'react';
import { NavLink, Outlet, Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { SunIcon, MoonIcon, TranslateIcon, UserIcon } from '../ui/Icons';
import { adminLogout } from '../../store/adminAuthSlice';
import './AdminLayout.scss';

/**
 * Admin console shell: sidebar navigation, top bar with theme/language
 * toggles and logout, plus the route guard for /admin/*.
 */
const AdminLayout = () => {
  const { t, i18n } = useTranslation();
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const location = useLocation();
  const adminUser = useSelector((state) => state.adminAuth.user);
  const isAuthenticated = useSelector((state) => state.adminAuth.isAuthenticated);

  const [theme, setTheme] = useState(() => {
    try {
      const s = localStorage.getItem('theme');
      if (s) return s;
    } catch (e) {}
    return 'dark';
  });

  useEffect(() => {
    try {
      if (theme === 'light') {
        document.documentElement.classList.add('light-theme');
      } else {
        document.documentElement.classList.remove('light-theme');
      }
      localStorage.setItem('theme', theme);
    } catch (e) {
      // ignore
    }
  }, [theme]);

  const toggleTheme = () => setTheme((s) => (s === 'dark' ? 'light' : 'dark'));

  const toggleLanguage = () => {
    const currentLang = i18n.language || 'en';
    const isChinese = currentLang === 'zh' || currentLang.startsWith('zh');
    i18n.changeLanguage(isChinese ? 'en' : 'zh');
  };

  const handleLogout = () => {
    dispatch(adminLogout());
    navigate('/admin/login');
  };

  // Route guard: no admin token → login with redirect back here.
  if (!isAuthenticated) {
    const redirect = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/admin/login?redirect=${redirect}`} replace />;
  }

  // Stale local state (token present but user is not an admin): the API is
  // still the boundary, but show a clear 403 instead of a broken console.
  if (!adminUser || adminUser.role !== 'admin') {
    return (
      <div className="admin-shell">
        <div className="admin-forbidden">
          <h1>403</h1>
          <p>{t('admin.common.forbidden')}</p>
          <button className="admin-btn admin-btn--primary" onClick={handleLogout}>
            {t('admin.common.logout')}
          </button>
        </div>
      </div>
    );
  }

  const navItems = [
    { to: '/admin', label: t('admin.nav.dashboard'), end: true },
    { to: '/admin/users', label: t('admin.nav.users') },
    { to: '/admin/templates', label: t('admin.nav.templates') },
    { to: '/admin/review', label: t('admin.nav.review') },
    { to: '/admin/appeals', label: t('admin.nav.appeals') },
    { to: '/admin/reports', label: t('admin.nav.reports') },
    { to: '/admin/comments', label: t('admin.nav.comments') },
    { to: '/admin/audit-log', label: t('admin.nav.audit_log') },
  ];

  return (
    <div className="admin-shell">
      <aside className="admin-sidebar">
        <div className="admin-sidebar__brand">
          <span className="admin-sidebar__logo">OP</span>
          <span className="admin-sidebar__title">{t('admin.nav.brand')}</span>
        </div>
        <nav className="admin-sidebar__nav">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `admin-sidebar__link${isActive ? ' admin-sidebar__link--active' : ''}`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="admin-sidebar__footer">
          <a className="admin-sidebar__link admin-sidebar__link--muted" href="/">
            {t('admin.nav.back_to_app')}
          </a>
        </div>
      </aside>
      <div className="admin-main">
        <header className="admin-topbar">
          <div className="admin-topbar__user">
            <UserIcon size={18} />
            <span title={adminUser.email}>{adminUser.display_name || adminUser.email}</span>
          </div>
          <div className="admin-topbar__actions">
            <button
              className="icon-btn admin-topbar__icon-btn"
              onClick={toggleTheme}
              aria-label={t('admin.common.theme')}
              title={t('admin.common.theme')}
            >
              {theme === 'dark' ? <SunIcon size={18} /> : <MoonIcon size={18} />}
            </button>
            <button
              className="admin-topbar__lang-btn"
              onClick={toggleLanguage}
              aria-label={t('admin.common.language')}
              title={t('admin.common.language')}
            >
              <TranslateIcon size={18} />
              <span>{i18n.language && (i18n.language === 'zh' || i18n.language.startsWith('zh')) ? t('template.en') : t('template.zh')}</span>
            </button>
            <button className="admin-btn admin-btn--ghost" onClick={handleLogout}>
              {t('admin.common.logout')}
            </button>
          </div>
        </header>
        <main className="admin-content">
          <Outlet />
        </main>
      </div>
    </div>
  );
};

export default AdminLayout;
