import React from 'react';
import { render, waitFor } from '@testing-library/react';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { BrowserRouter } from 'react-router-dom';
import Home from './Home';
import authReducer from '../store/authSlice';
import { NotificationProvider } from '../context/NotificationContext';
import * as api from '../services/api';

// Mock the api service
jest.mock('../services/api', () => ({
  ...jest.requireActual('../services/api'),
  getTemplates: jest.fn(),
  getCategories: jest.fn().mockResolvedValue({ data: { categories: [] } }),
  getTags: jest.fn().mockResolvedValue({ data: { tags: [] } }),
}));

// Mock child components to keep the test focused on Home's data fetching
jest.mock('../components/Layout', () => ({ children }) => (
  <div data-testid="layout">{children}</div>
));
jest.mock('../components/PromptCard', () => () => <div data-testid="prompt-card" />);

// Mock react-i18next. The language value is controlled per-test via mockLanguage.
let mockLanguage = 'en';
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
    i18n: { language: mockLanguage },
  }),
}));

const renderWithRedux = (component, { initialState } = {}) => {
  const store = configureStore({
    reducer: { auth: authReducer },
    preloadedState: initialState,
  });
  return render(
    <Provider store={store}>
      <NotificationProvider>
        <BrowserRouter>{component}</BrowserRouter>
      </NotificationProvider>
    </Provider>
  );
};

describe('Home Component language behavior', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    api.getTemplates.mockResolvedValue({
      data: {
        templates: [{ id: 't1', title: 'EN Template' }],
        private_templates: [],
        next_page_token: '',
      },
    });
  });

  test('fetches templates without a language filter', async () => {
    renderWithRedux(<Home />);

    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());

    const params = api.getTemplates.mock.calls[0][0];
    expect(params).not.toHaveProperty('language');
  });

  test('does not refetch templates when UI language changes', async () => {
    const { rerender } = renderWithRedux(<Home />);

    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
    const callsAfterFirstRender = api.getTemplates.mock.calls.length;
    expect(callsAfterFirstRender).toBeGreaterThan(0);

    // Switch UI language to Chinese and re-render.
    // The template list must NOT be refetched: templates are shared
    // between UI languages.
    mockLanguage = 'zh';
    rerender(
      <Provider store={configureStore({ reducer: { auth: authReducer } })}>
        <NotificationProvider>
          <BrowserRouter>
            <Home />
          </BrowserRouter>
        </NotificationProvider>
      </Provider>
    );

    // Give any (unexpected) effect a chance to run.
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(api.getTemplates.mock.calls.length).toBe(callsAfterFirstRender);
  });
});
