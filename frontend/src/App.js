import React, { useEffect, lazy, Suspense } from 'react';
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

// Admin console: lazy-loaded so the main bundle stays small.
const AdminLayout = lazy(() => import('./components/admin/AdminLayout'));
const AdminLoginPage = lazy(() => import('./pages/admin/AdminLoginPage'));
const DashboardPage = lazy(() => import('./pages/admin/DashboardPage'));
const UsersPage = lazy(() => import('./pages/admin/UsersPage'));
const UserDetailPage = lazy(() => import('./pages/admin/UserDetailPage'));
const TemplatesPage = lazy(() => import('./pages/admin/TemplatesPage'));
const TemplateDetailPage = lazy(() => import('./pages/admin/TemplateDetailPage'));
const ReviewPage = lazy(() => import('./pages/admin/ReviewPage'));
const ReviewDetailPage = lazy(() => import('./pages/admin/ReviewDetailPage'));
const AuditLogPage = lazy(() => import('./pages/admin/AuditLogPage'));
const NotificationsPage = lazy(() => import('./pages/NotificationsPage'));
const CreatorProfilePage = lazy(() => import('./pages/CreatorProfilePage'));

const PageSpinner = () => (
  <div className="admin-page-loading" role="status">
    Loading…
  </div>
);

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
            <Route
              path="/creators/:id"
              element={
                <Suspense fallback={<PageSpinner />}>
                  <CreatorProfilePage />
                </Suspense>
              }
            />
            <Route
              path="/notifications"
              element={
                <Suspense fallback={<PageSpinner />}>
                  <NotificationsPage />
                </Suspense>
              }
            />
            <Route
              path="/admin/login"
              element={
                <Suspense fallback={<PageSpinner />}>
                  <AdminLoginPage />
                </Suspense>
              }
            />
            <Route
              path="/admin"
              element={
                <Suspense fallback={<PageSpinner />}>
                  <AdminLayout />
                </Suspense>
              }
            >
              <Route
                index
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <DashboardPage />
                  </Suspense>
                }
              />
              <Route
                path="users"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <UsersPage />
                  </Suspense>
                }
              />
              <Route
                path="users/:id"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <UserDetailPage />
                  </Suspense>
                }
              />
              <Route
                path="templates"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <TemplatesPage />
                  </Suspense>
                }
              />
              <Route
                path="templates/:id"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <TemplateDetailPage />
                  </Suspense>
                }
              />
              <Route
                path="review"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <ReviewPage />
                  </Suspense>
                }
              />
              <Route
                path="review/:id"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <ReviewDetailPage />
                  </Suspense>
                }
              />
              <Route
                path="audit-log"
                element={
                  <Suspense fallback={<PageSpinner />}>
                    <AuditLogPage />
                  </Suspense>
                }
              />
            </Route>
          </Routes>
        </div>
      </NotificationProvider>
    </Router>
  );
}

export default App;
