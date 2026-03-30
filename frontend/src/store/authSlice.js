import { createSlice } from '@reduxjs/toolkit';

/**
 * Helper to initialize state synchronously from local storage.
 */
const getInitialState = () => {
  const token = localStorage.getItem('token');
  const userStr = localStorage.getItem('user');

  if (token && userStr) {
    try {
      return {
        user: JSON.parse(userStr),
        token: token,
        isAuthenticated: true,
      };
    } catch (e) {
      console.error('Failed to parse user from local storage', e);
      localStorage.removeItem('token');
      localStorage.removeItem('user');
    }
  }
  return {
    user: null, // User object: { id, displayName, ... }
    token: null, // JWT token
    isAuthenticated: false,
  };
};

/**
 * Initial state for the authentication slice.
 */
const initialState = getInitialState();

/**
 * Authentication slice containing state and reducers for user authentication.
 */
export const authSlice = createSlice({
  name: 'auth',
  initialState,
  reducers: {
    /**
     * Sets the login state with user and token.
     * @param {Object} state - Current state.
     * @param {Object} action - Action containing payload { user, token }.
     */
    loginSuccess: (state, action) => {
      state.user = action.payload.user;
      state.token = action.payload.token;
      state.isAuthenticated = true;
      localStorage.setItem('token', action.payload.token);
      localStorage.setItem('user', JSON.stringify(action.payload.user));
    },
    /**
     * Clears the user state and token (logout).
     * @param {Object} state - Current state.
     */
    logout: (state) => {
      state.user = null;
      state.token = null;
      state.isAuthenticated = false;
      localStorage.removeItem('token');
      localStorage.removeItem('user');
    },
    /**
     * Initializes auth state from local storage.
     * @param {Object} state - Current state.
     */
    initializeAuth: (state) => {
      const token = localStorage.getItem('token');
      const userStr = localStorage.getItem('user');
      if (token && userStr) {
        try {
          state.token = token;
          state.user = JSON.parse(userStr);
          state.isAuthenticated = true;
        } catch (e) {
            console.error("Failed to parse user from local storage", e);
            localStorage.removeItem('token');
            localStorage.removeItem('user');
        }
      }
    },
  },
});

export const { loginSuccess, logout, initializeAuth } = authSlice.actions;

export default authSlice.reducer;
