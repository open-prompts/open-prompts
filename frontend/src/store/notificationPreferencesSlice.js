import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import {
  getNotificationPreferences,
  updateNotificationPreferences,
} from '../services/api';

const DEFAULT_PREFS = {
  like_enabled: true,
  favorite_enabled: true,
  fork_enabled: true,
  comment_enabled: true,
  follow_enabled: true,
  paused_all: false,
};

const FIELDS = [
  'like_enabled',
  'favorite_enabled',
  'fork_enabled',
  'comment_enabled',
  'follow_enabled',
  'paused_all',
];

const initialSaving = () => FIELDS.reduce((acc, f) => ({ ...acc, [f]: false }), {});

/**
 * Fetch the caller's effective notification preferences (defaults when no
 * row exists). Called when the panel opens.
 */
export const fetchNotificationPreferences = createAsyncThunk(
  'notificationPreferences/fetch',
  async (_, { rejectWithValue }) => {
    try {
      const response = await getNotificationPreferences();
      return response.data;
    } catch (error) {
      return rejectWithValue(error.response?.data || error.message);
    }
  }
);

/**
 * PATCH a single preference field. The caller applies the optimistic update
 * before dispatching; on failure the caller rolls back.
 * @param {Object} arg - { field: string, value: boolean }
 */
export const updateNotificationPreference = createAsyncThunk(
  'notificationPreferences/update',
  async ({ field, value }, { rejectWithValue }) => {
    try {
      const response = await updateNotificationPreferences({ [field]: value });
      return { field, data: response.data };
    } catch (error) {
      return rejectWithValue({ field, error: error.response?.data || error.message });
    }
  }
);

const initialState = {
  prefs: { ...DEFAULT_PREFS },
  loaded: false,
  saving: initialSaving(),
  error: null,
};

const notificationPreferencesSlice = createSlice({
  name: 'notificationPreferences',
  initialState,
  reducers: {
    // Optimistic toggle: flip the field locally and mark it saving.
    optimisticToggle(state, action) {
      const { field, value } = action.payload;
      state.prefs[field] = value;
      state.saving[field] = true;
    },
    // Rollback a failed toggle to its previous value.
    rollbackToggle(state, action) {
      const { field, value } = action.payload;
      state.prefs[field] = value;
      state.saving[field] = false;
    },
    clearNotificationPreferences() {
      return { ...initialState, prefs: { ...DEFAULT_PREFS }, saving: initialSaving() };
    },
  },
  extraReducers: (builder) => {
    builder
      .addCase(fetchNotificationPreferences.pending, (state) => {
        state.error = null;
      })
      .addCase(fetchNotificationPreferences.fulfilled, (state, action) => {
        state.prefs = { ...DEFAULT_PREFS, ...action.payload };
        state.loaded = true;
        state.error = null;
      })
      .addCase(fetchNotificationPreferences.rejected, (state, action) => {
        state.error = action.payload || 'Failed to load notification preferences';
      })
      .addCase(updateNotificationPreference.fulfilled, (state, action) => {
        const { field, data } = action.payload;
        state.prefs = { ...DEFAULT_PREFS, ...data };
        state.saving[field] = false;
        state.error = null;
      })
      .addCase(updateNotificationPreference.rejected, (state, action) => {
        const { field } = action.payload || {};
        if (field) {
          state.saving[field] = false;
        }
        state.error = action.payload?.error || 'Failed to save notification preference';
      });
  },
});

export const { optimisticToggle, rollbackToggle, clearNotificationPreferences } = notificationPreferencesSlice.actions;

export default notificationPreferencesSlice.reducer;