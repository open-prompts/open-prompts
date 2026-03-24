import React, { useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import { useDispatch } from 'react-redux';
import { initializeAuth } from './store/authSlice';
import { NotificationProvider } from './context/NotificationContext';
import Home from './pages/Home';
import Login from './pages/Login';
import Register from './pages/Register';
import TemplateDetails from './pages/TemplateDetails';
import Profile from './pages/Profile';
import APIKeys from './pages/APIKeys';
import './index.scss'; // Global styles

/**
 * Main App component.
 * Sets up the router and defines the routes.
 */
function App() {
  const dispatch = useDispatch();

  useEffect(() => {
    dispatch(initializeAuth());
  }, [dispatch]);

  return (
    <Router>
      <NotificationProvider>
        <div className="App">
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />
            <Route path="/profile" element={<Profile />} />
            <Route path="/api-keys" element={<APIKeys />} />
            <Route path="/templates/:id" element={<TemplateDetails />} />
          </Routes>
        </div>
      </NotificationProvider>
    </Router>
  );
}

export default App;
