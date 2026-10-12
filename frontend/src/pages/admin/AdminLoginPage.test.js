import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import adminAuthReducer from '../../store/adminAuthSlice';
import AdminLoginPage from './AdminLoginPage';
import * as adminApi from '../../services/adminApi';

jest.mock('../../services/adminApi');

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

const mockedNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockedNavigate,
}));

const renderWithRedux = (component) => {
  const store = configureStore({
    reducer: { adminAuth: adminAuthReducer },
  });
  return {
    ...render(
      <Provider store={store}>
        <BrowserRouter>{component}</BrowserRouter>
      </Provider>
    ),
    store,
  };
};

describe('AdminLoginPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
  });

  test('renders the admin login form', () => {
    renderWithRedux(<AdminLoginPage />);

    expect(screen.getByText('admin.login.title')).toBeInTheDocument();
    expect(screen.getByLabelText('admin.login.email')).toBeInTheDocument();
    expect(screen.getByLabelText('admin.login.password')).toBeInTheDocument();
    expect(screen.getByText('admin.login.submit')).toBeInTheDocument();
    expect(screen.getByText('admin.login.back')).toBeInTheDocument();
  });

  test('submit is disabled until both fields are filled', () => {
    renderWithRedux(<AdminLoginPage />);

    const submit = screen.getByText('admin.login.submit').closest('button');
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'admin@example.com' },
    });
    expect(submit).toBeDisabled();

    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'Admin_123456' },
    });
    expect(submit).not.toBeDisabled();
  });

  test('handles successful admin login and navigates to /admin', async () => {
    adminApi.adminLogin.mockResolvedValue({
      data: {
        token: 'admin-token',
        user: { id: 'admin1', email: 'admin@example.com', role: 'admin' },
      },
    });

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'admin@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'Admin_123456' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(adminApi.adminLogin).toHaveBeenCalledWith({
        email: 'admin@example.com',
        password: 'Admin_123456',
      });
    });

    expect(localStorage.getItem('op_admin_token')).toBe('admin-token');
    expect(mockedNavigate).toHaveBeenCalledWith('/admin', { replace: true });
  });

  test('shows no_admin_access error on 403 without banned message', async () => {
    adminApi.adminLogin.mockRejectedValue({
      response: { status: 403, data: { error: 'not an admin' } },
    });

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'user@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'password' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(screen.getByText('admin.login.no_admin_access')).toBeInTheDocument();
    });
  });

  test('shows banned error on 403 with banned in message', async () => {
    adminApi.adminLogin.mockRejectedValue({
      response: { status: 403, data: { error: 'user is banned' } },
    });

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'banned@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'password' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(screen.getByText('admin.login.banned')).toBeInTheDocument();
    });
  });

  test('shows invalid credentials error on 401', async () => {
    adminApi.adminLogin.mockRejectedValue({
      response: { status: 401, data: { error: 'invalid credentials' } },
    });

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'admin@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'wrong' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(screen.getByText('admin.login.invalid')).toBeInTheDocument();
    });
  });

  test('shows rate limited error on 429', async () => {
    adminApi.adminLogin.mockRejectedValue({
      response: { status: 429, data: { error: 'too many attempts' } },
    });

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'admin@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'whatever' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(screen.getByText('admin.login.rate_limited')).toBeInTheDocument();
    });
  });

  test('shows generic error on network failure', async () => {
    adminApi.adminLogin.mockRejectedValue(new Error('Network Error'));

    renderWithRedux(<AdminLoginPage />);

    fireEvent.change(screen.getByLabelText('admin.login.email'), {
      target: { value: 'admin@example.com' },
    });
    fireEvent.change(screen.getByLabelText('admin.login.password'), {
      target: { value: 'whatever' },
    });
    fireEvent.click(screen.getByText('admin.login.submit'));

    await waitFor(() => {
      expect(screen.getByText('admin.common.error')).toBeInTheDocument();
    });
  });

  test('shows session expired notice when expired=1 query param present', () => {
    window.history.pushState({}, 'Admin Login', '/admin/login?expired=1');
    renderWithRedux(<AdminLoginPage />);
    expect(screen.getByText('admin.login.session_expired')).toBeInTheDocument();
    window.history.pushState({}, 'Admin Login', '/');
  });
});
