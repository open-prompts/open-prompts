import { configureStore } from '@reduxjs/toolkit';
import authReducer from './authSlice';
import adminAuthReducer from './adminAuthSlice';
import notificationsReducer from './notificationsSlice';
import notificationPreferencesReducer from './notificationPreferencesSlice';

export const store = configureStore({
  reducer: {
    auth: authReducer,
    adminAuth: adminAuthReducer,
    notifications: notificationsReducer,
    notificationPreferences: notificationPreferencesReducer,
  },
});
