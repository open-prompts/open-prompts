import React, { useState, useEffect, useRef } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useSelector, useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { MenuIcon, MoonIcon, SunIcon, TranslateIcon, UserIcon, ChevronDownIcon } from './ui/Icons';
import NotificationBell from './notifications/NotificationBell';
import { logout } from '../store/authSlice';
import './Header.scss';

/**
 * Header component for the application.
 * Displays the logo, navigation links, and user authentication status.
 */
const Header = ({ onMenuClick }) => {
  const { t, i18n } = useTranslation();
  const [theme, setTheme] = useState(() => {
    try {
      const s = localStorage.getItem('theme');
      if (s) return s;
    } catch (e) {}
    return (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) ? 'light' : 'dark';
  });
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const user = useSelector((state) => state.auth.user);
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
  const dropdownRef = useRef(null);

  const handleLogout = () => {
    dispatch(logout());
    setIsDropdownOpen(false);
    navigate('/login');
  };

  const toggleDropdown = () => {
    setIsDropdownOpen(!isDropdownOpen);
  };

  const toggleLanguage = () => {
    const currentLang = i18n.language;
    // Check if current language is Chinese (zh or starts with zh)
    const isChinese = currentLang === 'zh' || currentLang.startsWith('zh');
    i18n.changeLanguage(isChinese ? 'en' : 'zh');
  };

  // Menu keyboard pattern: Escape closes + restores focus; ArrowDown/ArrowUp
  // move between items (wrap).
  const menuRef = useRef(null);
  const menuTriggerRef = useRef(null);

  const handleMenuKeyDown = (e) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      setIsDropdownOpen(false);
      if (menuTriggerRef.current) menuTriggerRef.current.focus();
      return;
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    e.preventDefault();
    const menu = menuRef.current;
    if (!menu) return;
    const items = Array.from(menu.querySelectorAll('[role="menuitem"]'));
    if (items.length === 0) return;
    const currentIndex = items.indexOf(document.activeElement);
    let nextIndex;
    if (currentIndex === -1) {
      nextIndex = e.key === 'ArrowDown' ? 0 : items.length - 1;
    } else if (e.key === 'ArrowDown') {
      nextIndex = (currentIndex + 1) % items.length;
    } else {
      nextIndex = (currentIndex - 1 + items.length) % items.length;
    }
    items[nextIndex].focus();
  };

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
        setIsDropdownOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);

  // Sync theme to document and localStorage
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
  const currentLang = (i18n && i18n.language) || 'en';
  const isChinese = currentLang === 'zh' || currentLang.startsWith('zh');

  return (
    <header className="app-header">
      <div className="header-left">
        {onMenuClick && (
          <button
            className="menu-toggle-btn"
            onClick={onMenuClick}
            aria-label={t('common.toggle_menu')}
          >
            <MenuIcon size={22} />
          </button>
        )}
        <Link to="/" className="logo">
          <img src="/images/logo.jpg" alt={t('common.logo_alt')} className="logo-img" />
          <span className="logo-text">Open Prompts</span>
        </Link>
        <nav className="main-nav">
          <Link to="/" className="nav-link">{t('home.title')}</Link>
        </nav>
      </div>
      <div className="header-right">
        <button
          className="icon-btn header-icon-btn"
          onClick={toggleTheme}
          aria-label={theme === 'dark' ? t('header.switch_to_light') : t('header.switch_to_dark')}
          title={theme === 'dark' ? t('header.switch_to_light') : t('header.switch_to_dark')}
        >
          {theme === 'dark' ? <SunIcon size={19} /> : <MoonIcon size={19} />}
        </button>
        <button
          className="lang-switch-btn"
          onClick={toggleLanguage}
          aria-label={t('header.switchLanguage') || 'Switch Language'}
        >
          <TranslateIcon size={19} />
          <span>{isChinese ? t('template.en') : t('template.zh')}</span>
        </button>
        {user && <NotificationBell />}
        {user ? (
          <div className="user-profile" ref={dropdownRef}>
            <button
              className="user-name-btn"
              onClick={toggleDropdown}
              aria-label={t('header.open_user_menu')}
              aria-haspopup="menu"
              aria-expanded={isDropdownOpen}
              ref={menuTriggerRef}
            >
              {user.avatar ? (
                <img src={user.avatar} alt={t('common.avatar_alt')} className="user-avatar" />
              ) : (
                <span className="user-avatar-placeholder">
                  <UserIcon size={18} />
                </span>
              )}
              <span className="user-name">{user.displayName || user.email || t('common.user_fallback')}</span>
              <ChevronDownIcon size={14} className={isDropdownOpen ? 'caret-open' : ''} />
            </button>

            {isDropdownOpen && (
              <div className="profile-dropdown" role="menu" aria-label={t('header.open_user_menu')} ref={menuRef} onKeyDown={handleMenuKeyDown}>
                <div className="dropdown-header">
                  {user.avatar ? (
                    <img src={user.avatar} alt={t('common.avatar_alt')} className="dropdown-avatar" />
                  ) : (
                    <span className="dropdown-avatar-placeholder">
                      {user.displayName ? user.displayName.charAt(0).toUpperCase() : t('common.initial_fallback')}
                    </span>
                  )}
                  <div className="dropdown-user-info">
                    <div className="dropdown-user-name">{user.displayName || t('common.user_fallback')}</div>
                    <div className="dropdown-user-email">{user.email || ''}</div>
                  </div>
                </div>
                <button type="button" role="menuitem" className="dropdown-item" onClick={() => { navigate('/profile'); setIsDropdownOpen(false); }}>
                  {t('header.profile')}
                </button>
                <button type="button" role="menuitem" className="dropdown-item" onClick={() => { navigate('/api-keys'); setIsDropdownOpen(false); }}>
                  {t('api_keys.title', 'API Keys')}
                </button>
                <div className="dropdown-divider" />
                <button type="button" role="menuitem" className="dropdown-item logout" onClick={handleLogout}>
                  {t('header.logout')}
                </button>
              </div>
            )}
          </div>
        ) : (
          <div className="auth-buttons">
            <button className="btn btn-ghost btn-sm" onClick={() => navigate('/login')}>{t('header.login')}</button>
            <button className="btn btn-primary btn-sm" onClick={() => navigate('/register')}>{t('register.title')}</button>
          </div>
        )}
      </div>
    </header>
  );
};

export default Header;
