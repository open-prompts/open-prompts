import React from 'react';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import notificationsReducer, {
  fetchNotifications,
  markAllRead,
} from '../../store/notificationsSlice';
import notificationPreferencesReducer from '../../store/notificationPreferencesSlice';
import { NotificationProvider } from '../../context/NotificationContext';
import NotificationBell from './NotificationBell';

jest.mock('../../hooks/useNotificationPolling', () => jest.fn());

jest.mock('../../services/api', () => ({
  getNotifications: jest.fn(),
  getUnreadNotificationCount: jest.fn(),
  markNotificationRead: jest.fn(),
  markAllNotificationsRead: jest.fn(),
  deleteNotification: jest.fn(),
}));

const api = require('../../services/api');

// Mock i18n so translations resolve to their keys (with {{count}} interpolation)
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts && opts.count !== undefined ? `${key}:${opts.count}` : key),
    i18n: { language: 'en' },
  }),
}));

const makeNotification = (id, overrides = {}) => ({
  id,
  type: 'like',
  actor_id: 'actor_1',
  actor_display_name: 'Actor',
  actor_avatar: '',
  template_id: 't1',
  template_title: 'Template',
  read_at: '',
  created_at: '2025-01-01T00:00:00Z',
  ...overrides,
});

const emptyState = {
  items: [],
  unreadCount: 0,
  filter: 'all',
  totalCount: 0,
  nextPageToken: '',
  hasMore: false,
  loading: false,
  error: null,
  lastFetchedAt: 0,
};

const renderBell = (notificationsState) => {
  const actions = [];
  const actionLogger = () => (next) => (action) => {
    actions.push(action.type);
    return next(action);
  };
  const store = configureStore({
    reducer: { notifications: notificationsReducer, notificationPreferences: notificationPreferencesReducer },
    preloadedState: notificationsState
      ? { notifications: notificationsState }
      : undefined,
    middleware: (getDefaultMiddleware) => getDefaultMiddleware().concat(actionLogger),
  });
  render(
    <Provider store={store}>
      <BrowserRouter>
        <NotificationProvider>
          <NotificationBell />
        </NotificationProvider>
      </BrowserRouter>
    </Provider>
  );
  return { actions };
};

beforeEach(() => {
  api.getNotifications.mockResolvedValue({
    data: { notifications: [], next_page_token: '', unread_count: 0 },
  });
  api.getUnreadNotificationCount.mockResolvedValue({ data: { count: 0 } });
  api.markNotificationRead.mockResolvedValue({ data: { success: true } });
  api.markAllNotificationsRead.mockResolvedValue({ data: { success: true, updated_count: 0 } });
  api.deleteNotification.mockResolvedValue({ data: { success: true } });
});

afterEach(() => jest.clearAllMocks());

describe('NotificationBell', () => {
  test('renders bell button with zero-state aria label and no badge', () => {
    renderBell();
    const btn = screen.getByRole('button', { name: /notifications/i });
    expect(btn).toHaveAttribute('aria-label', 'notifications.bell_aria_zero');
    expect(screen.queryByText('5')).toBeNull();
  });

  test('shows badge with count and unread aria label', () => {
    renderBell({ ...emptyState, unreadCount: 5 });
    const btn = screen.getByRole('button', { name: /notifications/i });
    expect(btn).toHaveAttribute('aria-label', 'notifications.bell_aria:5');
    expect(screen.getByText('5')).toBeInTheDocument();
  });

  test('caps badge display at 99+', () => {
    renderBell({ ...emptyState, unreadCount: 150 });
    expect(screen.getByText('99+')).toBeInTheDocument();
  });

  test('dropdown opens on click and dispatches fetchNotifications', async () => {
    const { actions } = renderBell();
    fireEvent.click(screen.getByRole('button', { name: /notifications/i }));

    await waitFor(() => {
      expect(actions).toContain(fetchNotifications.pending.type);
    });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('notifications.view_all')).toBeInTheDocument();
  });

  test('dropdown shows empty state after fetch resolves with no items', async () => {
    renderBell();
    fireEvent.click(screen.getByRole('button', { name: /notifications/i }));
    await waitFor(() => {
      expect(screen.getByText('notifications.empty.dropdown')).toBeInTheDocument();
    });
  });

  test('dropdown shows error state with retry button when fetch fails', async () => {
    api.getNotifications.mockRejectedValueOnce({ response: { data: 'boom' } });
    renderBell();
    fireEvent.click(screen.getByRole('button', { name: /notifications/i }));
    await waitFor(() => {
      expect(screen.getByText('notifications.error.fetch')).toBeInTheDocument();
    });
    expect(screen.getByText('notifications.error.retry')).toBeInTheDocument();
  });

  test('dropdown lists items and mark-all button when unread', () => {
    renderBell({
      ...emptyState,
      items: [makeNotification('n1'), makeNotification('n2', { read_at: 'x' })],
      unreadCount: 1,
      totalCount: 2,
    });
    fireEvent.click(screen.getByRole('button', { name: /notifications/i }));
    const list = screen.getByRole('list');
    expect(within(list).getAllByRole('listitem')).toHaveLength(2);
    expect(screen.getByText('notifications.mark_all_read')).toBeInTheDocument();
  });

  test('mark-all dispatches markAllRead', async () => {
    const { actions } = renderBell({
      ...emptyState,
      items: [makeNotification('n1')],
      unreadCount: 1,
      totalCount: 1,
    });
    fireEvent.click(screen.getByRole('button', { name: /notifications/i }));
    fireEvent.click(screen.getByText('notifications.mark_all_read'));

    await waitFor(() => {
      expect(actions).toContain(markAllRead.pending.type);
    });
  });

  test('closes dropdown on Escape', () => {
    renderBell();
    const btn = screen.getByRole('button', { name: /notifications/i });
    fireEvent.click(btn);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
