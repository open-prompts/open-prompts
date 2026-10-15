import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { register, sendVerificationCode } from '../services/api';
import { loginSuccess } from '../store/authSlice';
import { useNotification } from '../context/NotificationContext';
import { AlertIcon } from '../components/ui/Icons';
import './Register.scss';
import AuthBackground from '../components/AuthBackground';
import { usePageAnnouncement } from '../utils/a11y';

/**
 * Register Page Component
 * Allows new users to create an account.
 */
const Register = () => {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const { addNotification } = useNotification();
  const [formData, setFormData] = useState({
    id: '',
    email: '',
    password: '',
    displayName: '',
    verificationCode: '',
  });
  const [showPassword, setShowPassword] = useState(false);
  const [formErrors, setFormErrors] = useState({});
  const [generalError, setGeneralError] = useState('');
  const [loading, setLoading] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const [countdown, setCountdown] = useState(0);

  usePageAnnouncement('register.title');

  // Handle countdown timer
  useEffect(() => {
    let timer;
    if (countdown > 0) {
      timer = setTimeout(() => setCountdown(countdown - 1), 1000);
    }
    return () => clearTimeout(timer);
  }, [countdown]);

  // Validate email format
  const validateEmail = (email) => {
    const re = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return re.test(email);
  };

  // Validate password complexity
  const validatePassword = (pwd) => {
    if (pwd.length <= 8) return false;
    let complexity = 0;
    if (/[a-z]/.test(pwd)) complexity++;
    if (/[A-Z]/.test(pwd)) complexity++;
    if (/[0-9]/.test(pwd)) complexity++;
    return complexity >= 2;
  };

  /**
   * Handles input changes.
   * @param {Event} e - The input change event.
   */
  const handleChange = (e) => {
    const { id, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [id]: value,
    }));
    // Clear error when user types
    if (formErrors[id]) {
        setFormErrors(prev => ({ ...prev, [id]: '' }));
    }
  };

  /**
   * Handles sending verification code.
   */
  const handleSendCode = async () => {
    if (!formData.email) {
      setFormErrors((prev) => ({ ...prev, email: t('register.field_required', { field: t('register.email') }) }));
      return;
    }
    if (!validateEmail(formData.email)) {
      setFormErrors((prev) => ({ ...prev, email: t('register.email_invalid') }));
      return;
    }

    setIsSending(true);
    try {
      await sendVerificationCode(formData.email, i18n.language);
      setCountdown(60);
      addNotification({
        kind: 'success',
        title: t('register.send_code'),
        subtitle: t('register.code_sent'),
      });
    } catch (err) {
      console.error('Send code error:', err);
      addNotification({
        kind: 'error',
        title: t('register.send_code'),
        subtitle: t('register.send_failed'),
      });
    } finally {
      setIsSending(false);
    }
  };

  /**
   * Handles the form submission.
   * @param {Event} e - The form submission event.
   */
  const handleSubmit = async (e) => {
    e.preventDefault();
    setFormErrors({});
    setGeneralError('');

    const errors = {};
    if (!formData.id) errors.id = t('register.field_required', { field: t('register.id') });
    if (!formData.email) errors.email = t('register.field_required', { field: t('register.email') });
    if (!formData.password) errors.password = t('register.field_required', { field: t('register.password') });
    if (!formData.verificationCode) errors.verificationCode = t('register.field_required', { field: t('register.verification_code') });

    // Check password validity only if it exists
    if (formData.password && !validatePassword(formData.password)) {
      errors.password = t('register.password_error');
    }

    if (Object.keys(errors).length > 0) {
        setFormErrors(errors);
        setGeneralError(t('register.error_required_fields'));
        // Move focus to the first invalid field for screen-reader users.
        const firstInvalid = document.querySelector('.field-invalid .input');
        if (firstInvalid) firstInvalid.focus();
        return;
    }

    setLoading(true);

    try {
      const response = await register({
        id: formData.id,
        email: formData.email,
        password: formData.password,
        displayName: formData.displayName,
        verification_code: formData.verificationCode
      });
      // Assuming the response contains the token and user info
      const { token, id } = response.data;

      // Dispatch login success action
      // Note: Register response might not return displayName, rely on form data
      dispatch(loginSuccess({
        token,
        user: { id, email: formData.email, displayName: formData.displayName }
      }));

      // Redirect to home page
      navigate('/');
    } catch (err) {
      console.error('Registration error:', err);
      addNotification({
        kind: 'error',
        title: t('register.title'),
        subtitle: t('register.error'),
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page register-page">
      <AuthBackground />
      <div className="auth-card register-card">
        <div className="auth-card-head">
          <h1>{t('register.title')}</h1>
          <p className="auth-card-sub">{t('register.subtitle')}</p>
        </div>
        <form onSubmit={handleSubmit} noValidate>
          {generalError && (
            <div className="form-alert">
              <AlertIcon size={16} />
              <span>{generalError}</span>
            </div>
          )}

          <div className={`field ${formErrors.id ? 'field-invalid' : ''}`}>
            <label className="field-label" htmlFor="id">{t('register.id')}</label>
            <input
              id="id"
              type="text"
              className="input"
              value={formData.id}
              autoComplete="username"
              onChange={handleChange}
              placeholder={t('register.ph_id')}
            />
            {formErrors.id && <span className="field-error">{formErrors.id}</span>}
          </div>

          <div className={`field ${formErrors.email ? 'field-invalid' : ''}`}>
            <label className="field-label" htmlFor="email">{t('register.email')}</label>
            <input
              id="email"
              type="email"
              className="input"
              value={formData.email}
              autoComplete="email"
              onChange={handleChange}
              placeholder={t('register.ph_email')}
            />
            {formErrors.email && <span className="field-error">{formErrors.email}</span>}
          </div>

          <div className={`field ${formErrors.verificationCode ? 'field-invalid' : ''}`}>
            <label className="field-label" htmlFor="verificationCode">{t('register.verification_code')}</label>
            <div className="verification-row">
              <input
                id="verificationCode"
                type="text"
                className="input"
                value={formData.verificationCode}
                onChange={handleChange}
                placeholder={t('register.ph_code')}
              />
              <button
                type="button"
                className="btn btn-secondary send-code-btn"
                onClick={handleSendCode}
                disabled={countdown > 0 || !formData.email || !validateEmail(formData.email) || isSending}
              >
                {isSending ? '...' : (countdown > 0 ? t('register.resend_in', { seconds: countdown }) : t('register.send_code'))}
              </button>
            </div>
            {formErrors.verificationCode && <span className="field-error">{formErrors.verificationCode}</span>}
          </div>

          <div className="field">
            <label className="field-label" htmlFor="displayName">{t('register.display_name')}</label>
            <input
              id="displayName"
              type="text"
              className="input"
              value={formData.displayName}
              onChange={handleChange}
              placeholder={t('register.ph_display_name')}
              required
            />
          </div>

          <div className={`field ${formErrors.password ? 'field-invalid' : ''}`}>
            <label className="field-label" htmlFor="password">{t('register.password')}</label>
            <div className="password-wrap">
              <input
                id="password"
                type={showPassword ? 'text' : 'password'}
                className="input"
                value={formData.password}
                autoComplete="new-password"
                onChange={handleChange}
              />
              <button
                type="button"
                className="password-toggle"
                onClick={() => setShowPassword(s => !s)}
                aria-label={showPassword ? t('common.hide_password') : t('common.show_password')}
                tabIndex={-1}
              >
                {showPassword ? '🙈' : '👁'}
              </button>
            </div>
            {formErrors.password && <span className="field-error">{formErrors.password}</span>}
          </div>

          <button
            type="submit"
            className="btn btn-primary btn-block auth-submit"
            disabled={loading}
          >
            {loading ? t('register.registering') : t('register.submit')}
          </button>
        </form>
        <div className="auth-links">
          <Link to="/login" className="login-link">
            {t('register.login_link')}
          </Link>
        </div>
      </div>
    </div>
  );
};

export default Register;
