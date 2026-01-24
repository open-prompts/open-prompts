import React from 'react';
import ReactDOM from 'react-dom/client';
import './fonts/index.css';
import './index.scss';
import App from './App';
import { Provider } from 'react-redux';
import { store } from './store/store';
import { NotificationProvider } from './context/NotificationContext';
import './i18n'; // Import i18n configuration

// Apply initial theme early to avoid flash
try {
  const savedTheme = localStorage.getItem('theme');
  const prefersLight = window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches;
  const initialTheme = savedTheme || (prefersLight ? 'light' : 'dark');
  if (initialTheme === 'light') {
    document.documentElement.classList.add('light-theme');
  } else {
    document.documentElement.classList.remove('light-theme');
  }
} catch (e) {
  // ignore if localStorage not available
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <Provider store={store}>
      <NotificationProvider>
        <App />
      </NotificationProvider>
    </Provider>
  </React.StrictMode>
);
