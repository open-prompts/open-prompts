import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import authReducer from '../store/authSlice';
import Header from './Header';

// Helper to render with Redux
const renderWithRedux = (component, { initialState } = {}) => {
  const store = configureStore({
    reducer: { auth: authReducer },
    preloadedState: initialState,
  });
  return {
    ...render(
      <Provider store={store}>
        {component}
      </Provider>
    ),
    store,
  };
};

test('renders header with logo and navigation', () => {
  renderWithRedux(
    <BrowserRouter>
      <Header />
    </BrowserRouter>
  );

  const logoElement = screen.getByText(/Open Prompts/i);
  expect(logoElement).toBeInTheDocument();

  const homeLink = screen.getByText(/Home/i);
  expect(homeLink).toBeInTheDocument();
});

test('renders login and register buttons when not logged in', () => {
  renderWithRedux(
    <BrowserRouter>
      <Header />
    </BrowserRouter>,
    { initialState: { auth: { user: null } } }
  );

  const loginButton = screen.getByText(/Login/i);
  expect(loginButton).toBeInTheDocument();

  const registerButton = screen.getByText(/Register/i);
  expect(registerButton).toBeInTheDocument();
});

test('renders user profile when logged in', () => {
  const user = { displayName: 'John Doe', email: 'john@example.com' };
  renderWithRedux(
    <BrowserRouter>
      <Header />
    </BrowserRouter>,
    { initialState: { auth: { user } } }
  );

  const userElement = screen.getByText(/John Doe/i);
  expect(userElement).toBeInTheDocument();

  // Check dropdown toggle
  fireEvent.click(userElement);
  const logoutButton = screen.getByText(/Logout/i);
  expect(logoutButton).toBeInTheDocument();
});
