import notificationsReducer, {
  setFilter,
  clearNotifications,
  fetchNotifications,
  fetchUnreadCount,
  markRead,
  markAllRead,
  removeNotification,
} from './notificationsSlice';

jest.mock('../services/api', () => ({
  getNotifications: jest.fn(),
  getUnreadNotificationCount: jest.fn(),
  markNotificationRead: jest.fn(),
  markAllNotificationsRead: jest.fn(),
  deleteNotification: jest.fn(),
}));

const api = require('../services/api');

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

describe('notificationsSlice', () => {
  test('initial state', () => {
    const state = notificationsReducer(undefined, { type: '@@INIT' });
    expect(state.items).toEqual([]);
    expect(state.unreadCount).toBe(0);
    expect(state.filter).toBe('all');
    expect(state.loading).toBe(false);
    expect(state.hasMore).toBe(false);
  });

  test('setFilter changes the filter', () => {
    const state = notificationsReducer(undefined, setFilter('unread'));
    expect(state.filter).toBe('unread');
  });

  test('clearNotifications resets to initial state', () => {
    let state = notificationsReducer(undefined, { type: '@@INIT' });
    state = { ...state, items: [makeNotification('n1')], unreadCount: 5, filter: 'unread' };
    state = notificationsReducer(state, clearNotifications());
    expect(state.items).toEqual([]);
    expect(state.unreadCount).toBe(0);
    expect(state.filter).toBe('all');
  });

  describe('fetchNotifications', () => {
    test('fulfilled with reset replaces items', () => {
      const data = {
        notifications: [makeNotification('n1'), makeNotification('n2')],
        next_page_token: '20',
        unread_count: 2,
      };
      const action = {
        type: fetchNotifications.fulfilled.type,
        payload: { filter: 'all', reset: true, data },
      };
      const state = notificationsReducer(undefined, action);
      expect(state.items).toHaveLength(2);
      expect(state.unreadCount).toBe(2);
      expect(state.nextPageToken).toBe('20');
      expect(state.hasMore).toBe(true);
      expect(state.loading).toBe(false);
    });

    test('fulfilled with reset=false appends and dedupes by id', () => {
      const existing = { ...makeNotification('n1') };
      let state = {
        ...notificationsReducer(undefined, { type: '@@INIT' }),
        items: [existing],
      };
      const data = {
        notifications: [makeNotification('n1'), makeNotification('n2')],
        next_page_token: '',
        unread_count: 1,
      };
      const action = {
        type: fetchNotifications.fulfilled.type,
        payload: { filter: 'all', reset: false, data },
      };
      state = notificationsReducer(state, action);
      expect(state.items.map((n) => n.id)).toEqual(['n1', 'n2']);
      expect(state.hasMore).toBe(false);
    });

    test('pending sets loading and clears error', () => {
      let state = { ...notificationsReducer(undefined, { type: '@@INIT' }), error: 'boom' };
      state = notificationsReducer(state, { type: fetchNotifications.pending.type });
      expect(state.loading).toBe(true);
      expect(state.error).toBeNull();
    });

    test('rejected stores the error', () => {
      const state = notificationsReducer(undefined, {
        type: fetchNotifications.rejected.type,
        payload: 'server error',
      });
      expect(state.loading).toBe(false);
      expect(state.error).toBe('server error');
    });
  });

  describe('fetchUnreadCount', () => {
    test('fulfilled updates unreadCount', () => {
      const state = notificationsReducer(undefined, {
        type: fetchUnreadCount.fulfilled.type,
        payload: 7,
      });
      expect(state.unreadCount).toBe(7);
    });
  });

  describe('markRead', () => {
    test('pending optimistically marks read and decrements unreadCount', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1'), makeNotification('n2')],
        unreadCount: 2,
      };
      state = notificationsReducer(state, {
        type: markRead.pending.type,
        meta: { arg: 'n1' },
      });
      expect(state.items[0].read_at).not.toBe('');
      expect(state.unreadCount).toBe(1);
    });

    test('pending on already-read item does not decrement again', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1', { read_at: '2025-01-01T00:00:00Z' })],
        unreadCount: 0,
      };
      state = notificationsReducer(state, {
        type: markRead.pending.type,
        meta: { arg: 'n1' },
      });
      expect(state.unreadCount).toBe(0);
    });

    test('rejected reverts the optimistic update', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1')],
        unreadCount: 0,
      };
      state = notificationsReducer(state, {
        type: markRead.rejected.type,
        meta: { arg: 'n1' },
      });
      expect(state.items[0].read_at).toBe('');
      expect(state.unreadCount).toBe(1);
    });
  });

  describe('markAllRead', () => {
    test('pending marks all items read and zeroes unreadCount', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1'), makeNotification('n2', { read_at: 'x' })],
        unreadCount: 1,
      };
      state = notificationsReducer(state, { type: markAllRead.pending.type });
      expect(state.unreadCount).toBe(0);
      expect(state.items.every((n) => n.read_at)).toBe(true);
    });
  });

  describe('removeNotification', () => {
    test('pending removes the item and decrements unreadCount when unread', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1'), makeNotification('n2')],
        unreadCount: 2,
      };
      state = notificationsReducer(state, {
        type: removeNotification.pending.type,
        meta: { arg: 'n1' },
      });
      expect(state.items.map((n) => n.id)).toEqual(['n2']);
      expect(state.unreadCount).toBe(1);
    });

    test('pending removes a read item without touching unreadCount', () => {
      let state = notificationsReducer(undefined, { type: '@@INIT' });
      state = {
        ...state,
        items: [makeNotification('n1', { read_at: 'x' })],
        unreadCount: 0,
      };
      state = notificationsReducer(state, {
        type: removeNotification.pending.type,
        meta: { arg: 'n1' },
      });
      expect(state.items).toEqual([]);
      expect(state.unreadCount).toBe(0);
    });
  });
});

describe('notificationsSlice thunks', () => {
  afterEach(() => jest.clearAllMocks());

  test('fetchNotifications passes filter and page params', async () => {
    api.getNotifications.mockResolvedValue({
      data: { notifications: [], next_page_token: '', unread_count: 0 },
    });
    const dispatch = jest.fn();
    await fetchNotifications({ filter: 'unread', reset: false, pageSize: 10, pageToken: '10' })(
      dispatch,
      undefined
    );
    expect(api.getNotifications).toHaveBeenCalledWith({
      filter: 'unread',
      page_size: 10,
      page_token: '10',
    });
  });

  test('fetchNotifications omits page_token on reset', async () => {
    api.getNotifications.mockResolvedValue({
      data: { notifications: [], next_page_token: '', unread_count: 0 },
    });
    const dispatch = jest.fn();
    await fetchNotifications({ filter: 'all', reset: true, pageToken: 'stale' })(dispatch, undefined);
    expect(api.getNotifications).toHaveBeenCalledWith({ filter: 'all', page_size: 20 });
  });

  test('fetchUnreadCount returns the count', async () => {
    api.getUnreadNotificationCount.mockResolvedValue({ data: { count: 4 } });
    const dispatch = jest.fn();
    const action = await fetchUnreadCount()(dispatch, undefined);
    expect(action.payload).toBe(4);
  });

  test('markRead rejects with id on API failure', async () => {
    api.markNotificationRead.mockRejectedValue({ response: { data: 'nope' } });
    const dispatch = jest.fn();
    const result = await markRead('n1')(dispatch, undefined);
    expect(result.payload).toEqual({ id: 'n1', error: 'nope' });
  });

  test('markAllRead returns updated_count', async () => {
    api.markAllNotificationsRead.mockResolvedValue({ data: { updated_count: 3 } });
    const dispatch = jest.fn();
    const action = await markAllRead()(dispatch, undefined);
    expect(action.payload).toBe(3);
  });

  test('removeNotification returns the id', async () => {
    api.deleteNotification.mockResolvedValue({ data: { success: true } });
    const dispatch = jest.fn();
    const action = await removeNotification('n9')(dispatch, undefined);
    expect(action.payload).toBe('n9');
  });
});
