import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { TextInput, PasswordInput, Button, Form } from '@carbon/react';
import { login } from '../services/api';
import { loginSuccess } from '../store/authSlice';
import { useNotification } from '../context/NotificationContext';
import './Login.scss';
import AuthBackground from '../components/AuthBackground';

/**
 * Login Page Component
 * Allows users to authenticate with their email and password.
 */
const Login = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const { addNotification } = useNotification();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [formErrors, setFormErrors] = useState({});
  const [loading, setLoading] = useState(false);

  /**
   * Handles the form submission.
   * @param {Event} e - The form submission event.
   */
  const handleSubmit = async (e) => {
    e.preventDefault();
    setFormErrors({});

    // Manual Validation
    const errors = {};
    if (!email) errors.email = t('login.email') + ' is required'; // Or specific translation key
    if (!password) errors.password = t('login.password') + ' is required';

    if (Object.keys(errors).length > 0) {
        setFormErrors(errors);
        return;
    }

    setLoading(true);

    try {
      const response = await login({ email, password });
      // Assuming the response contains the token and user info
      const { token, id, display_name, avatar } = response.data;

      // Dispatch login success action
      dispatch(loginSuccess({
        token,
        user: { id, displayName: display_name, email, avatar }
      }));

      // Redirect to home page
      navigate('/');
    } catch (err) {
      console.error('Login error:', err);
      addNotification({
        kind: 'error',
        title: t('login.title'),
        subtitle: t('login.error'),
      });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-page">
      <AuthBackground />
      <div className="login-form-container">
        <h2>{t('login.title')}</h2>
        <Form onSubmit={handleSubmit} noValidate>
          <TextInput
            id="email"
            labelText={t('login.email')}
            value={email}
            onChange={(e) => {
                setEmail(e.target.value);
                if (formErrors.email) setFormErrors({...formErrors, email: ''});
            }}
            placeholder="user@example.com"
            invalid={!!formErrors.email}
            invalidText={formErrors.email}
          />
          <PasswordInput
            id="password"
            labelText={t('login.password')}
            value={password}
            onChange={(e) => {
                setPassword(e.target.value);
                if (formErrors.password) setFormErrors({...formErrors, password: ''});
            }}
            invalid={!!formErrors.password}
            invalidText={formErrors.password}
          />
          <Button
            type="submit"
            className="login-button"
            disabled={loading}
            isSelected={loading}
          >
            {loading ? 'Logging in...' : t('login.submit')}
          </Button>
        </Form>
        <Link to="/register" className="register-link">
          {t('login.register_link')}
        </Link>
        <Link to="/" className="back-home-link">
          {t('login.back_to_home')}
        </Link>
      </div>
    </div>
  );
};

export default Login;
