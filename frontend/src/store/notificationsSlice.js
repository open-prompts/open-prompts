import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import {
  getNotifications,
  getUnreadNotificationCount,
  markNotificationRead,
  markAllNotificationsRead,
  deleteNotification,
} from '../services/api';

/**
 * Fetch a page of notifications.
 * @param {Object} arg - { filter: 'all'|'unread', reset: boolean, pageSize?: number, pageToken?: string }
 *   reset=true replaces the list (tab switch / dropdown refresh);
 *   reset=false appends (Load more, pass the current nextPageToken).
 */
export const fetchNotifications = createAsyncThunk(
  'notifications/fetchList',
  async ({ filter = 'all', reset = true, pageSize = 20, pageToken = '' } = {}, { rejectWithValue }) => {
    try {
      const params = { filter, page_size: pageSize };
      if (!reset && pageToken) {
        params.page_token = pageToken;
      }
      const response = await getNotifications(params);
      return { filter, reset, data: response.data };
    } catch (error) {
      return rejectWithValue(error.response?.data || error.message);
    }
  }
);

export const fetchUnreadCount = createAsyncThunk(
  'notifications/fetchUnreadCount',
  async (_, { rejectWithValue }) => {
    try {
      const response = await getUnreadNotificationCount();
      return response.data.count;
    } catch (error) {
      return rejectWithValue(error.response?.data || error.message);
    }
  }
);

export const markRead = createAsyncThunk(
  'notifications/markRead',
  async (id, { rejectWithValue }) => {
    try {
      await markNotificationRead(id);
      return id;
    } catch (error) {
      return rejectWithValue({ id, error: error.response?.data || error.message });
    }
  }
);

export const markAllRead = createAsyncThunk(
  'notifications/markAllRead',
  async (_, { rejectWithValue }) => {
    try {
      const response = await markAllNotificationsRead();
      return response.data.updated_count || 0;
    } catch (error) {
      return rejectWithValue(error.response?.data || error.message);
    }
  }
);

export const removeNotification = createAsyncThunk(
  'notifications/delete',
  async (id, { rejectWithValue }) => {
    try {
      await deleteNotification(id);
      return id;
    } catch (error) {
      return rejectWithValue({ id, error: error.response?.data || error.message });
    }
  }
);

const initialState = {
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

const notificationsSlice = createSlice({
  name: 'notifications',
  initialState,
  reducers: {
    setFilter(state, action) {
      state.filter = action.payload;
    },
    clearNotifications() {
      return { ...initialState };
    },
  },
  extraReducers: (builder) => {
    builder
      // fetchNotifications
      .addCase(fetchNotifications.pending, (state) => {
        state.loading = true;
        state.error = null;
      })
      .addCase(fetchNotifications.fulfilled, (state, action) => {
        const { filter, reset, data } = action.payload;
        state.loading = false;
        state.lastFetchedAt = Date.now();
        state.filter = filter;
        state.unreadCount = data.unread_count || 0;
        state.nextPageToken = data.next_page_token || '';
        state.hasMore = Boolean(data.next_page_token);
        const items = data.notifications || [];
        if (reset) {
          state.items = items;
        } else {
          // Append, deduping by id (safety against pagination races).
          const seen = new Set(state.items.map((n) => n.id));
          state.items = state.items.concat(items.filter((n) => !seen.has(n.id)));
        }
      })
      .addCase(fetchNotifications.rejected, (state, action) => {
        state.loading = false;
        state.error = action.payload || 'Failed to load notifications';
      })
      // fetchUnreadCount
      .addCase(fetchUnreadCount.fulfilled, (state, action) => {
        state.unreadCount = action.payload || 0;
        state.lastFetchedAt = Date.now();
      })
      // markRead: optimistic update handled in pending, revert on reject
      .addCase(markRead.pending, (state, action) => {
        const item = state.items.find((n) => n.id === action.meta.arg);
        if (item && !item.read_at) {
          item.read_at = new Date().toISOString();
          state.unreadCount = Math.max(0, state.unreadCount - 1);
        }
      })
      .addCase(markRead.rejected, (state, action) => {
        const item = state.items.find((n) => n.id === action.meta.arg);
        if (item) {
          item.read_at = '';
          state.unreadCount += 1;
        }
      })
      // markAllRead
      .addCase(markAllRead.pending, (state) => {
        state.unreadCount = 0;
        state.items.forEach((n) => {
          n.read_at = n.read_at || new Date().toISOString();
        });
      })
      .addCase(markAllRead.rejected, (state) => {
        // Revert is impractical without a refetch; the caller shows a toast
        // and can dispatch fetchNotifications to reconcile.
      })
      // removeNotification
      .addCase(removeNotification.pending, (state, action) => {
        const idx = state.items.findIndex((n) => n.id === action.meta.arg);
        if (idx !== -1) {
          const [removed] = state.items.splice(idx, 1);
          if (!removed.read_at) {
            state.unreadCount = Math.max(0, state.unreadCount - 1);
          }
        }
      })
      .addCase(removeNotification.rejected, (state, action) => {
        // Row already removed optimistically; caller toasts the error and
        // can refetch to reconcile.
      });
  },
});

export const { setFilter, clearNotifications } = notificationsSlice.actions;

export default notificationsSlice.reducer;
