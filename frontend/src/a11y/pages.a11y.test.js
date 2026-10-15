import React from 'react';
import { render } from '@testing-library/react';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { MemoryRouter } from 'react-router-dom';
import { axe, toHaveNoViolations } from 'jest-axe';
import authReducer from '../store/authSlice';
import { NotificationProvider } from '../context/NotificationContext';
import Home from '../pages/Home';
import Login from '../pages/Login';
import Register from '../pages/Register';
import TemplateDetails from '../pages/TemplateDetails';
import Profile from '../pages/Profile';
import APIKeys from '../pages/APIKeys';

expect.extend(toHaveNoViolations);

// Mock the api service so pages render without a backend.
jest.mock('../services/api', () => ({
  ...jest.requireActual('../services/api'),
  getTemplates: jest.fn().mockResolvedValue({
    data: { templates: [], private_templates: [], next_page_token: '' },
  }),
  getCategories: jest.fn().mockResolvedValue({ data: { categories: [] } }),
  getTags: jest.fn().mockResolvedValue({ data: { tags: [] } }),
  getTemplate: jest.fn().mockResolvedValue({
    data: {
      id: 't1',
      title: 'Test Template',
      description: 'A test template',
      visibility: 'VISIBILITY_PUBLIC',
      owner_id: 'u1',
      content_warnings: [],
      locked_warnings: [],
      latest_version: { content: 'Hello name' },
    },
  }),
  listTemplateVersions: jest.fn().mockResolvedValue({ data: { versions: [] } }),
  listPrompts: jest.fn().mockResolvedValue({ data: { prompts: [] } }),
  getProfile: jest.fn().mockResolvedValue({
    data: { display_name: 'Test User', email: 'test@example.com', avatar: '', show_warned_content: false },
  }),
}));

// Mock react-i18next so translations resolve to their keys.
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
    i18n: { language: 'en' },
  }),
}));

// Mock child components that pull in heavy dependencies.
jest.mock('../components/Layout', () => ({ children }) => (
  <main id="main-content" tabIndex={-1}>{children}</main>
));
jest.mock('../components/PromptCard', () => () => <div data-testid="prompt-card" />);
jest.mock('../components/Header', () => () => <header data-testid="header" />);
jest.mock('../components/AuthBackground', () => () => null);
jest.mock('../components/APIKeyManager', () => () => <div data-testid="api-key-manager" />);

const renderWithProviders = (ui, { initialState = {} } = {}) => {
  const store = configureStore({
    reducer: { auth: authReducer },
    preloadedState: initialState,
  });
  return render(
    <Provider store={store}>
      <NotificationProvider>
        <MemoryRouter>{ui}</MemoryRouter>
      </NotificationProvider>
    </Provider>
  );
};

// jsdom cannot compute layout, so axe's color-contrast rule is excluded here.
// Contrast is enforced by the token audit table + manual both-themes pass.
const axeOptions = {
  rules: {
    'color-contrast': { enabled: false },
  },
};

describe('Accessibility axe gate (0 critical / 0 serious)', () => {
  test('Home page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<Home />);
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });

  test('Login page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<Login />);
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });

  test('Register page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<Register />);
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });

  test('TemplateDetails page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<TemplateDetails />);
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });

  test('Profile page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<Profile />, {
      initialState: { auth: { user: { id: 'u1', displayName: 'Test User' }, token: 'tok' } },
    });
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });

  test('APIKeys page has no critical/serious violations', async () => {
    const { container } = renderWithProviders(<APIKeys />, {
      initialState: { auth: { user: { id: 'u1' }, token: 'tok' } },
    });
    const results = await axe(container, axeOptions);
    expect(results).toHaveNoViolations();
  });
});