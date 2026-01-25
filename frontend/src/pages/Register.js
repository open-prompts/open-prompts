import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { TextInput, PasswordInput, Button, Form } from '@carbon/react';
import { register, sendVerificationCode } from '../services/api';
import { loginSuccess } from '../store/authSlice';
import { useNotification } from '../context/NotificationContext';
import './Register.scss';
import AuthBackground from '../components/AuthBackground';

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
  const [formErrors, setFormErrors] = useState({});
  const [loading, setLoading] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const [countdown, setCountdown] = useState(0);

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
      setFormErrors((prev) => ({ ...prev, email: t('register.email') + ' is required' }));
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

    const errors = {};
    if (!formData.id) errors.id = t('register.id') + ' is required';
    if (!formData.email) errors.email = t('register.email') + ' is required';
    if (!formData.password) errors.password = t('register.password') + ' is required';
    if (!formData.verificationCode) errors.verificationCode = t('register.verification_code') + ' is required';

    // Check password validity only if it exists
    if (formData.password && !validatePassword(formData.password)) {
      errors.password = t('register.password_error');
    }

    if (Object.keys(errors).length > 0) {
        setFormErrors(errors);
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
    <div className="register-page">
      <AuthBackground />
      <div className="register-form-container">
        <h2>{t('register.title')}</h2>
        <Form onSubmit={handleSubmit} noValidate>
          <TextInput
            id="id"
            labelText={t('register.id')}
            value={formData.id}
            onChange={handleChange}
            placeholder="unique_username"
            invalid={!!formErrors.id}
            invalidText={formErrors.id}
          />
          <TextInput
            id="email"
            labelText={t('register.email')}
            value={formData.email}
            onChange={handleChange}
            placeholder="user@example.com"
            invalid={!!formErrors.email}
            invalidText={formErrors.email}
          />
          <div className="verification-row">
            <TextInput
              id="verificationCode"
              labelText={t('register.verification_code')}
              value={formData.verificationCode}
              onChange={handleChange}
              placeholder="123456"
              invalid={!!formErrors.verificationCode}
              invalidText={formErrors.verificationCode}
            />
            <Button
                kind="tertiary"
                onClick={handleSendCode}
                disabled={countdown > 0 || !formData.email || !validateEmail(formData.email) || isSending}
                className="send-code-btn"
            >
              {isSending ? '...' : (countdown > 0 ? t('register.resend_in', { seconds: countdown }) : t('register.send_code'))}
            </Button>
          </div>
          <TextInput
            id="displayName"
            labelText={t('register.display_name')}
            value={formData.displayName}
            onChange={handleChange}
            placeholder="John Doe"
            required
          />
          <PasswordInput
            id="password"
            labelText={t('register.password')}
            value={formData.password}
            onChange={handleChange}
            invalid={!!formErrors.password}
            invalidText={formErrors.password}
          />
          <Button
            type="submit"
            className="register-button"
            disabled={loading}
            isSelected={loading}
          >
            {loading ? 'Registering...' : t('register.submit')}
          </Button>
        </Form>
        <Link to="/login" className="login-link">
          {t('register.login_link')}
        </Link>
      </div>
    </div>
  );
};

export default Register;
