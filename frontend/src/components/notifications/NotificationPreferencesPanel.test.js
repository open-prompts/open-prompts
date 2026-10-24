import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import notificationPreferencesReducer from '../../store/notificationPreferencesSlice';
import { NotificationProvider } from '../../context/NotificationContext';
import NotificationPreferencesPanel from './NotificationPreferencesPanel';

jest.mock('../../services/api', () => ({
  getNotificationPreferences: jest.fn(),
  updateNotificationPreferences: jest.fn(),
}));

const api = require('../../services/api');

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
    i18n: { language: 'en' },
  }),
}));

const renderPanel = (open = true) => {
  const store = configureStore({
    reducer: { notificationPreferences: notificationPreferencesReducer },
  });
  return render(
    <Provider store={store}>
      <NotificationProvider>
        <NotificationPreferencesPanel open={open} onClose={() => {}} />
      </NotificationProvider>
    </Provider>
  );
};

beforeEach(() => {
  api.getNotificationPreferences.mockResolvedValue({
    data: { like_enabled: true, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: false },
  });
  api.updateNotificationPreferences.mockResolvedValue({
    data: { like_enabled: true, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: false },
  });
});

afterEach(() => jest.clearAllMocks());

describe('NotificationPreferencesPanel', () => {
  test('renders nothing when closed', () => {
    renderPanel(false);
    expect(screen.queryByText('notifPrefs.title')).not.toBeInTheDocument();
  });

  test('renders five type toggles and pause all after load', async () => {
    renderPanel(true);
    expect(await screen.findByText('notifPrefs.pauseAll')).toBeInTheDocument();
    expect(screen.getByText('notifPrefs.type.like')).toBeInTheDocument();
    expect(screen.getByText('notifPrefs.type.favorite')).toBeInTheDocument();
    expect(screen.getByText('notifPrefs.type.fork')).toBeInTheDocument();
    expect(screen.getByText('notifPrefs.type.comment')).toBeInTheDocument();
    expect(screen.getByText('notifPrefs.type.follow')).toBeInTheDocument();
  });

  test('toggling a type fires a PATCH and shows Saved', async () => {
    api.updateNotificationPreferences.mockResolvedValue({
      data: { like_enabled: false, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: false },
    });
    renderPanel(true);
    await screen.findByText('notifPrefs.pauseAll');

    const likeSwitch = screen.getAllByRole('switch')[1];
    fireEvent.click(likeSwitch);

    await waitFor(() => {
      expect(api.updateNotificationPreferences).toHaveBeenCalledWith({ like_enabled: false });
    });
    expect(await screen.findByText(/notifPrefs\.saved/)).toBeInTheDocument();
  });

  test('failed PATCH rolls back the toggle', async () => {
    api.updateNotificationPreferences.mockRejectedValue({ response: { data: 'boom' } });
    renderPanel(true);
    await screen.findByText('notifPrefs.pauseAll');

    const likeSwitch = screen.getAllByRole('switch')[1];
    fireEvent.click(likeSwitch);

    await waitFor(() => {
      expect(api.updateNotificationPreferences).toHaveBeenCalledWith({ like_enabled: false });
    });
    // The switch should roll back to on (aria-checked true).
    await waitFor(() => {
      expect(screen.getAllByRole('switch')[1]).toHaveAttribute('aria-checked', 'true');
    });
  });

  test('pausing all disables type toggles', async () => {
    api.updateNotificationPreferences.mockResolvedValue({
      data: { like_enabled: true, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: true },
    });
    renderPanel(true);
    await screen.findByText('notifPrefs.pauseAll');

    const pauseSwitch = screen.getAllByRole('switch')[0];
    fireEvent.click(pauseSwitch);

    await waitFor(() => {
      expect(api.updateNotificationPreferences).toHaveBeenCalledWith({ paused_all: true });
    });
    expect(await screen.findByText('notifPrefs.pausedNote')).toBeInTheDocument();
    const typeSwitches = screen.getAllByRole('switch').slice(1);
    typeSwitches.forEach((s) => {
      expect(s).toBeDisabled();
    });
  });
});