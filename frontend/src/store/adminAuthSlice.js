import { createSlice } from '@reduxjs/toolkit';

const STORAGE_KEYS = { token: 'op_admin_token', user: 'op_admin_user' };

/**
 * Initialize admin auth state synchronously from dedicated admin storage keys.
 */
const getInitialState = () => {
  const token = localStorage.getItem(STORAGE_KEYS.token);
  const userStr = localStorage.getItem(STORAGE_KEYS.user);

  if (token && userStr) {
    try {
      return {
        user: JSON.parse(userStr),
        token,
        isAuthenticated: true,
      };
    } catch (e) {
      localStorage.removeItem(STORAGE_KEYS.token);
      localStorage.removeItem(STORAGE_KEYS.user);
    }
  }
  return {
    user: null,
    token: null,
    isAuthenticated: false,
  };
};

const initialState = getInitialState();

/**
 * Admin authentication slice. Mirrors authSlice but persists to separate
 * op_admin_* keys so admin and regular-user sessions stay independent.
 */
export const adminAuthSlice = createSlice({
  name: 'adminAuth',
  initialState,
  reducers: {
    adminLoginSuccess: (state, action) => {
      state.user = action.payload.user;
      state.token = action.payload.token;
      state.isAuthenticated = true;
      localStorage.setItem(STORAGE_KEYS.token, action.payload.token);
      localStorage.setItem(STORAGE_KEYS.user, JSON.stringify(action.payload.user));
    },
    adminLogout: (state) => {
      state.user = null;
      state.token = null;
      state.isAuthenticated = false;
      localStorage.removeItem(STORAGE_KEYS.token);
      localStorage.removeItem(STORAGE_KEYS.user);
    },
    initializeAdminAuth: (state) => state,
  },
});

export const { adminLoginSuccess, adminLogout, initializeAdminAuth } = adminAuthSlice.actions;

export default adminAuthSlice.reducer;
