import axios from 'axios';

// Create an axios instance with default configuration
const api = axios.create({
  baseURL: '/api/v1', // Base URL for the API
  timeout: 60000, // Request timeout
});

// Add a request interceptor to include auth token if available
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Add a response interceptor to handle 401 errors
api.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    if (error.response && error.response.status === 401) {
      // Clear local storage
      localStorage.removeItem('token');
      localStorage.removeItem('user');

      // Redirect to login page
      // Using window.location to ensure a full redirect,
      // though ideally we could use a history object if we had access to it outside of React context
      if (window.location.pathname !== '/login') {
         window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

// API methods for Templates
export const getTemplates = (params) => {
  return api.get('/templates', { params });
};

// API methods for Dashboard
export const getDashboardOverview = () => {
  return api.get('/dashboard/overview');
};

export const getCategories = (params) => {
  return api.get('/categories', { params });
};

export const getTags = (params) => {
  return api.get('/tags', { params });
};

export const createTemplate = (templateData) => {
  return api.post('/templates', templateData);
};

export const getTemplate = (id) => {
  return api.get(`/templates/${id}`);
};

export const updateTemplate = (id, templateData) => {
  return api.put(`/templates/${id}`, templateData);
};

export const deleteTemplate = (id) => {
  return api.delete(`/templates/${id}`);
};

export const toggleTemplateLike = (id) => {
  return api.post(`/templates/${id}/like`);
};

export const toggleTemplateFavorite = (id) => {
  return api.post(`/templates/${id}/favorite`);
};

export const forkTemplate = (id) => {
  return api.post(`/templates/${id}/fork`);
};

export const listTemplateVersions = (templateId, params) => {
  return api.get(`/templates/${templateId}/versions`, { params });
};

export const getRelatedTemplates = (templateId, limit = 8) => {
  return api.get(`/templates/${templateId}/related`, { params: { limit } });
};

export const getMoreFromAuthor = (templateId, limit = 4) => {
  return api.get(`/templates/${templateId}/more-from-author`, { params: { limit } });
};

export const rateTemplate = (templateId, rating) => {
  return api.post(`/templates/${templateId}/rating`, { rating });
};

export const deleteRating = (templateId) => {
  return api.delete(`/templates/${templateId}/rating`);
};

// API methods for Prompts
export const createPrompt = (promptData) => {
  return api.post('/prompts', promptData);
};

export const listPrompts = (params) => {
  return api.get('/prompts', { params });
};

export const deletePrompt = (id) => {
  return api.delete(`/prompts/${id}`);
};

export const login = (credentials) => {
  return api.post('/login', credentials);
};

export const register = (userData) => {
  return api.post('/register', userData);
};

export const sendVerificationCode = (email, language = 'en') => {
  return api.post('/verification-code', { email, language });
};

export const updateProfile = (userData) => {
  return api.put('/profile', userData);
};

export const getProfile = () => {
  return api.get('/profile');
};

export default api;

// API methods for Template Aliases
export const listTemplateAliases = (templateId) => {
  return api.get(`/templates/${templateId}/aliases`);
};

export const createTemplateAlias = (templateId, aliasData) => {
  return api.post(`/templates/${templateId}/aliases`, aliasData);
};

export const updateTemplateAlias = (templateId, aliasName, aliasData) => {
  return api.put(`/templates/${templateId}/aliases/${aliasName}`, aliasData);
};

export const deleteTemplateAlias = (templateId, aliasName) => {
  return api.delete(`/templates/${templateId}/aliases/${aliasName}`);
};

export const getPromptByAlias = (templateId, aliasName) => {
  return api.get(`/templates/${templateId}/aliases/${aliasName}/prompt`);
};

// API methods for API Keys
export const listAPIKeys = (params) => {
  return api.get('/api-keys', { params });
};

export const createAPIKey = (keyData) => {
  return api.post('/api-keys', keyData);
};

export const deleteAPIKey = (id) => {
  return api.delete(`/api-keys/${id}`);
};

// API methods for Analytics
export const getAnalyticsOverview = (days = 30) =>
  api.get('/analytics/overview', { params: { days } });

export const getTemplateStats = (id, days = 30) =>
  api.get(`/templates/${id}/stats`, { params: { days } });

export const recordTemplateEvent = (id, { type = 'view', sessionId = '' } = {}) =>
  api.post(`/templates/${id}/events`, { type, session_id: sessionId });

// API methods for Notifications
export const getNotifications = (params) => {
  return api.get('/notifications', { params });
};

export const getUnreadNotificationCount = () => {
  return api.get('/notifications/unread-count');
};

export const markNotificationRead = (id) => {
  return api.post(`/notifications/${id}/read`);
};

export const markAllNotificationsRead = () => {
  return api.post('/notifications/read-all');
};

export const deleteNotification = (id) => {
  return api.delete(`/notifications/${id}`);
};

// API methods for Notification Preferences
export const getNotificationPreferences = () => {
  return api.get('/users/me/notification-preferences');
};

export const updateNotificationPreferences = (patch) => {
  return api.patch('/users/me/notification-preferences', patch);
};

// API methods for Comments
export const listTemplateComments = (templateId, params) => {
  return api.get(`/templates/${templateId}/comments`, { params });
};

export const createTemplateComment = (templateId, content, parentCommentId) => {
  return api.post(`/templates/${templateId}/comments`, {
    content,
    ...(parentCommentId ? { parent_comment_id: parentCommentId } : {}),
  });
};

export const updateComment = (commentId, content) => {
  return api.put(`/comments/${commentId}`, { content });
};

export const deleteComment = (commentId) => {
  return api.delete(`/comments/${commentId}`);
};

export const listCommentReplies = (commentId) => {
  return api.get(`/comments/${commentId}/replies`);
};

// API methods for Follow Creators
export const followUser = (userId) => {
  return api.post(`/users/${userId}/follow`);
};

export const unfollowUser = (userId) => {
  return api.delete(`/users/${userId}/follow`);
};

export const getCreatorProfile = (userId) => {
  return api.get(`/users/${userId}/profile`);
};

export const getCreatorTemplates = (userId, params) => {
  return api.get(`/users/${userId}/templates`, { params });
};

export const getFollowers = (userId, params) => {
  return api.get(`/users/${userId}/followers`, { params });
};

export const getFollowing = (userId, params) => {
  return api.get(`/users/${userId}/following`, { params });
};

export const getFollowingFeed = (params) => {
  return api.get('/users/me/following-feed', { params });
};

export const getTopCreators = (params) => {
  return api.get('/users', { params: { sort: 'followers', ...params } });
};

// API methods for Template Collections
export const listPublicCollections = (params) => {
  return api.get('/collections', { params });
};

export const createCollection = (data) => {
  return api.post('/collections', data);
};

export const getCollection = (id) => {
  return api.get(`/collections/${id}`);
};

export const updateCollection = (id, data) => {
  return api.patch(`/collections/${id}`, data);
};

export const deleteCollection = (id) => {
  return api.delete(`/collections/${id}`);
};

export const addTemplateToCollection = (collectionId, templateId) => {
  return api.post(`/collections/${collectionId}/templates`, { template_id: templateId });
};

export const removeTemplateFromCollection = (collectionId, templateId) => {
  return api.delete(`/collections/${collectionId}/templates/${templateId}`);
};

export const reorderCollectionTemplates = (collectionId, templateIds) => {
  return api.put(`/collections/${collectionId}/templates`, { template_ids: templateIds });
};

export const listCollectionsContainingTemplate = (templateId) => {
  return api.get(`/templates/${templateId}/collections`);
};

export const listMyCollections = () => {
  return api.get('/collections/mine');
};

// Moderation Appeals (user surface)
export const createAppeal = (data) => api.post('/appeals', data);
export const listAppeals = (params) => api.get('/appeals', { params });
export const getAppeal = (id) => api.get(`/appeals/${id}`);
// Content reporting (user surface)
export const createReport = (data) => api.post('/reports', data);
export const getMyReport = (item_type, item_id) =>
  api.get('/reports/mine', { params: { item_type, item_id } });
export const withdrawReport = (item_type, item_id) =>
  api.delete('/reports/mine', { data: { item_type, item_id } });
