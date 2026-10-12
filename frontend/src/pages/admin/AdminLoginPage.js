import React, { useState } from 'react';
import { useNavigate, useSearchParams, Link } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { adminLogin } from '../../services/adminApi';
import { adminLoginSuccess } from '../../store/adminAuthSlice';
import { LockIcon } from '../../components/ui/Icons';
import './AdminLoginPage.scss';

/**
 * Admin console login. Separate surface from the regular user login.
 */
const AdminLoginPage = () => {
  const { t } = useTranslation();
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (busy) return;
    setError('');
    setBusy(true);
    try {
      const resp = await adminLogin({ email, password });
      const data = resp.data || {};
      if (!data.token || !data.user) {
        setError(t('admin.login.invalid'));
        return;
      }
      dispatch(adminLoginSuccess({ token: data.token, user: data.user }));
      const redirect = searchParams.get('redirect');
      navigate(redirect && redirect.startsWith('/admin') ? redirect : '/admin', { replace: true });
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 403) {
        const msg = (err.response.data && err.response.data.error) || '';
        setError(
          msg.includes('banned') ? t('admin.login.banned') : t('admin.login.no_admin_access')
        );
      } else if (status === 401) {
        setError(t('admin.login.invalid'));
      } else if (status === 429) {
        setError(t('admin.login.rate_limited'));
      } else {
        setError(t('admin.common.error'));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="admin-login">
      <div className="admin-login__card">
        <div className="admin-login__brand">
          <span className="admin-login__logo">
            <LockIcon size={18} />
          </span>
          <h1>{t('admin.login.title')}</h1>
          <p>{t('admin.login.subtitle')}</p>
        </div>
        {searchParams.get('expired') === '1' && (
          <div className="admin-login__notice">{t('admin.login.session_expired')}</div>
        )}
        <form onSubmit={handleSubmit} className="admin-login__form">
          <label htmlFor="admin-email">{t('admin.login.email')}</label>
          <input
            id="admin-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            required
            autoFocus
          />
          <label htmlFor="admin-password">{t('admin.login.password')}</label>
          <input
            id="admin-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
          {error && (
            <div className="admin-login__error" role="alert">
              {error}
            </div>
          )}
          <button
            type="submit"
            className="admin-btn admin-btn--primary admin-login__submit"
            disabled={busy || !email || !password}
          >
            {busy ? t('admin.login.signing_in') : t('admin.login.submit')}
          </button>
        </form>
        <Link className="admin-login__back" to="/">
          {t('admin.login.back')}
        </Link>
      </div>
    </div>
  );
};

export default AdminLoginPage;
