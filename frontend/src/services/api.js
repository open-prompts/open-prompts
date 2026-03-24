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
