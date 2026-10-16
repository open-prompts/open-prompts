import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import authReducer from '../store/authSlice';
import Login from './Login';
import { NotificationProvider } from '../context/NotificationContext';
import * as api from '../services/api';

// Mock the api service
jest.mock('../services/api');

// Mock react-i18next
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

// Mock useNavigate
const mockedNavigate = jest.fn();
jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useNavigate: () => mockedNavigate,
}));

// Helper to render with Redux
const renderWithRedux = (component) => {
  const store = configureStore({
    reducer: { auth: authReducer },
  });
  return {
    ...render(
      <Provider store={store}>
        <NotificationProvider>{component}</NotificationProvider>
      </Provider>
    ),
    store,
  };
};

describe('Login Component', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
  });

  test('renders login form', () => {
    renderWithRedux(
      <BrowserRouter>
        <Login />
      </BrowserRouter>
    );

    expect(screen.getByText('login.title')).toBeInTheDocument();
    expect(screen.getByLabelText('login.email')).toBeInTheDocument();
    expect(screen.getByLabelText('login.password')).toBeInTheDocument();
    expect(screen.getByText('login.submit')).toBeInTheDocument();
  });

  test('handles successful login', async () => {
    api.login.mockResolvedValue({
      data: {
        token: 'fake-token',
        id: 'user123',
        displayName: 'Test User',
      },
    });

    renderWithRedux(
      <BrowserRouter>
        <Login />
      </BrowserRouter>
    );

    fireEvent.change(screen.getByLabelText('login.email'), {
      target: { value: 'test@example.com' },
    });
    fireEvent.change(screen.getByLabelText('login.password'), {
      target: { value: 'password123' },
    });

    fireEvent.click(screen.getByText('login.submit'));

    await waitFor(() => {
      expect(api.login).toHaveBeenCalledWith({
        email: 'test@example.com',
        password: 'password123',
      });
    });

    expect(localStorage.getItem('token')).toBe('fake-token');
    expect(mockedNavigate).toHaveBeenCalledWith('/dashboard');
  });

  test('handles login failure', async () => {
    api.login.mockRejectedValue(new Error('Login failed'));

    renderWithRedux(
      <BrowserRouter>
        <Login />
      </BrowserRouter>
    );

    fireEvent.change(screen.getByLabelText('login.email'), {
      target: { value: 'test@example.com' },
    });
    fireEvent.change(screen.getByLabelText('login.password'), {
      target: { value: 'wrongpassword' },
    });

    fireEvent.click(screen.getByText('login.submit'));

    await waitFor(() => {
      expect(screen.getByText('login.error')).toBeInTheDocument();
    });
  });
});
