import axios from 'axios';

const TOKEN_KEY = 'op_admin_token';
const USER_KEY = 'op_admin_user';

// Separate axios instance for the admin console: distinct token storage and
// 401 handling so admin and regular-user sessions never interfere.
const adminApi = axios.create({ baseURL: '/api/v1', timeout: 60000 });

adminApi.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

adminApi.interceptors.response.use(
  (response) => {
    ensureFreshAdminToken();
    return response;
  },
  (error) => {
    if (
      error.response &&
      error.response.status === 401 &&
      window.location.pathname.startsWith('/admin') &&
      window.location.pathname !== '/admin/login'
    ) {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
      const redirect = encodeURIComponent(
        window.location.pathname + window.location.search
      );
      window.location.href = `/admin/login?redirect=${redirect}`;
    }
    return Promise.reject(error);
  }
);

// Decode the JWT payload (base64url) without a library.
export const decodeAdminToken = (token) => {
  try {
    const payload = token.split('.')[1];
    const json = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
    return JSON.parse(json);
  } catch (e) {
    return null;
  }
};

// Sliding session: if the token was issued more than 10 minutes ago, refresh
// it in the background so short admin sessions do not expire mid-task.
let refreshPromise = null;
export const ensureFreshAdminToken = () => {
  const token = localStorage.getItem(TOKEN_KEY);
  if (!token) return Promise.resolve();
  const claims = decodeAdminToken(token);
  if (!claims || !claims.iat) return Promise.resolve();
  const ageSeconds = Date.now() / 1000 - claims.iat;
  if (ageSeconds <= 10 * 60) return Promise.resolve();
  if (refreshPromise) return refreshPromise;

  refreshPromise = axios
    .post(
      '/api/v1/admin/session/refresh',
      {},
      { headers: { Authorization: `Bearer ${token}` } }
    )
    .then((resp) => {
      refreshPromise = null;
      if (resp.data && resp.data.token) {
        localStorage.setItem(TOKEN_KEY, resp.data.token);
      }
      return resp.data;
    })
    .catch((err) => {
      refreshPromise = null;
      throw err;
    });
  return refreshPromise;
};

// Admin auth
export const adminLogin = (credentials) => adminApi.post('/admin/login', credentials);
export const adminRefreshSession = () => adminApi.post('/admin/session/refresh');
// Stats
export const getAdminStats = () => adminApi.get('/admin/stats');
// Users
export const getAdminUsers = (params) => adminApi.get('/admin/users', { params });
export const getAdminUser = (id) => adminApi.get(`/admin/users/${id}`);
export const banAdminUser = (id, reason) => adminApi.post(`/admin/users/${id}/ban`, { reason });
export const unbanAdminUser = (id, reason) => adminApi.post(`/admin/users/${id}/unban`, { reason });
export const changeAdminUserRole = (id, role, reason) =>
  adminApi.post(`/admin/users/${id}/role`, { role, reason });
// Templates
export const getAdminTemplates = (params) => adminApi.get('/admin/templates', { params });
export const getAdminTemplate = (id) => adminApi.get(`/admin/templates/${id}`);
// Content warnings (admin)
export const updateTemplateWarnings = (id, { add, remove, reason }) =>
  adminApi.post(`/admin/templates/${id}/warnings`, { add, remove, reason });
export const removeTemplateWarnings = (id, { add, remove, reason }) =>
  adminApi.delete(`/admin/templates/${id}/warnings`, { data: { add, remove, reason } });
export const getWarningHistory = (id, params) =>
  adminApi.get(`/admin/templates/${id}/warnings/history`, { params });
// Pre-publication review
export const getReviewQueue = (params) => adminApi.get('/admin/review', { params });
export const getReviewMetrics = () => adminApi.get('/admin/review/metrics');
export const getReviewSubmission = (id) => adminApi.get(`/admin/review/${id}`);
export const approveReviewSubmission = (id) => adminApi.post(`/admin/review/${id}/approve`);
export const rejectReviewSubmission = (id, reasonCode, note) =>
  adminApi.post(`/admin/review/${id}/reject`, { reason_code: reasonCode, note });
// Platform settings
export const getPlatformSettings = () => adminApi.get('/admin/settings');
export const updatePlatformSetting = (key, value, reason) =>
  adminApi.put('/admin/settings', { key, value: String(value), reason });
// Audit
export const getAdminAuditLogs = (params) => adminApi.get('/admin/audit-log', { params });
// Comments
export const getAdminComments = (params) => adminApi.get('/admin/comments', { params });
export const deleteAdminComment = (id, reason) =>
  adminApi.delete(`/admin/comments/${id}`, { data: { reason } });
// Moderation Appeals (admin surface)
export const listAdminAppeals = (params) => adminApi.get('/admin/appeals', { params });
export const getAdminAppeal = (id) => adminApi.get(`/admin/appeals/${id}`);
export const reviewAppeal = (id) => adminApi.post(`/admin/appeals/${id}/review`);
export const resolveAppeal = (id, data) => adminApi.post(`/admin/appeals/${id}/resolve`, data);

export default adminApi;
