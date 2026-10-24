import notificationPreferencesReducer, {
  optimisticToggle,
  rollbackToggle,
  clearNotificationPreferences,
  fetchNotificationPreferences,
  updateNotificationPreference,
} from './notificationPreferencesSlice';

jest.mock('../services/api', () => ({
  getNotificationPreferences: jest.fn(),
  updateNotificationPreferences: jest.fn(),
}));

const api = require('../services/api');

describe('notificationPreferencesSlice', () => {
  test('initial state has all defaults', () => {
    const state = notificationPreferencesReducer(undefined, { type: '@@INIT' });
    expect(state.prefs).toEqual({
      like_enabled: true,
      favorite_enabled: true,
      fork_enabled: true,
      comment_enabled: true,
      follow_enabled: true,
      paused_all: false,
    });
    expect(state.loaded).toBe(false);
    expect(state.error).toBeNull();
  });

  test('optimisticToggle flips a field and marks it saving', () => {
    let state = notificationPreferencesReducer(undefined, { type: '@@INIT' });
    state = notificationPreferencesReducer(state, optimisticToggle({ field: 'like_enabled', value: false }));
    expect(state.prefs.like_enabled).toBe(false);
    expect(state.saving.like_enabled).toBe(true);
  });

  test('rollbackToggle restores a field and clears saving', () => {
    let state = notificationPreferencesReducer(undefined, { type: '@@INIT' });
    state = notificationPreferencesReducer(state, optimisticToggle({ field: 'like_enabled', value: false }));
    state = notificationPreferencesReducer(state, rollbackToggle({ field: 'like_enabled', value: true }));
    expect(state.prefs.like_enabled).toBe(true);
    expect(state.saving.like_enabled).toBe(false);
  });

  test('clearNotificationPreferences resets to defaults', () => {
    let state = notificationPreferencesReducer(undefined, { type: '@@INIT' });
    state = notificationPreferencesReducer(state, optimisticToggle({ field: 'paused_all', value: true }));
    state = notificationPreferencesReducer(state, clearNotificationPreferences());
    expect(state.prefs.paused_all).toBe(false);
    expect(state.loaded).toBe(false);
  });

  describe('fetchNotificationPreferences', () => {
    test('fulfilled merges server prefs over defaults', async () => {
      api.getNotificationPreferences.mockResolvedValue({
        data: { like_enabled: false, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: false },
      });
      const dispatch = jest.fn();
      const getState = () => ({});
      await fetchNotificationPreferences()(dispatch, getState, undefined);
      const action = dispatch.mock.calls.find((c) => c[0].type === 'notificationPreferences/fetch/fulfilled');
      const state = notificationPreferencesReducer(undefined, action[0]);
      expect(state.prefs.like_enabled).toBe(false);
      expect(state.loaded).toBe(true);
    });

    test('rejected sets error', async () => {
      api.getNotificationPreferences.mockRejectedValue({ response: { data: 'boom' } });
      const dispatch = jest.fn();
      const getState = () => ({});
      await fetchNotificationPreferences()(dispatch, getState, undefined);
      const action = dispatch.mock.calls.find((c) => c[0].type === 'notificationPreferences/fetch/rejected');
      const state = notificationPreferencesReducer(undefined, action[0]);
      expect(state.error).toBeTruthy();
    });
  });

  describe('updateNotificationPreference', () => {
    test('fulfilled updates prefs and clears saving', async () => {
      api.updateNotificationPreferences.mockResolvedValue({
        data: { like_enabled: false, favorite_enabled: true, fork_enabled: true, comment_enabled: true, follow_enabled: true, paused_all: false },
      });
      const dispatch = jest.fn();
      const getState = () => ({});
      await updateNotificationPreference({ field: 'like_enabled', value: false })(dispatch, getState, undefined);
      const action = dispatch.mock.calls.find((c) => c[0].type === 'notificationPreferences/update/fulfilled');
      const state = notificationPreferencesReducer(undefined, action[0]);
      expect(state.prefs.like_enabled).toBe(false);
      expect(state.saving.like_enabled).toBe(false);
    });

    test('rejected clears saving and sets error', async () => {
      api.updateNotificationPreferences.mockRejectedValue({ response: { data: 'boom' } });
      const dispatch = jest.fn();
      const getState = () => ({});
      await updateNotificationPreference({ field: 'like_enabled', value: false })(dispatch, getState, undefined);
      const action = dispatch.mock.calls.find((c) => c[0].type === 'notificationPreferences/update/rejected');
      const state = notificationPreferencesReducer(undefined, action[0]);
      expect(state.saving.like_enabled).toBe(false);
      expect(state.error).toBeTruthy();
    });
  });
});