// Node-side API helpers for e2e specs. Runs in the Nightwatch test process
// (not the browser), talking to the backend REST gateway directly.
// NOTE: this file lives outside src_folders (tests/) on purpose — Nightwatch
// loads every .js file under tests/ as a suite.

const http = require('http');

const API_HOST = process.env.E2E_API_HOST || '127.0.0.1';
const API_PORT = process.env.E2E_API_PORT ? parseInt(process.env.E2E_API_PORT, 10) : 8080;

function request(method, path, body, { token } = {}) {
  return new Promise((resolve, reject) => {
    const payload = body ? JSON.stringify(body) : null;
    const headers = { 'Content-Type': 'application/json' };
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    const req = http.request(
      {
        host: API_HOST,
        port: API_PORT,
        path,
        method,
        headers,
      },
      (res) => {
        let data = '';
        res.setEncoding('utf8');
        res.on('data', (chunk) => { data += chunk; });
        res.on('end', () => {
          let parsed = null;
          try {
            parsed = data ? JSON.parse(data) : null;
          } catch (e) {
            parsed = null;
          }
          if (res.statusCode >= 200 && res.statusCode < 300) {
            resolve({ status: res.statusCode, data: parsed });
          } else {
            const err = new Error(
              `API ${method} ${path} -> ${res.statusCode}: ${data.slice(0, 300)}`
            );
            err.status = res.statusCode;
            err.body = parsed;
            reject(err);
          }
        });
      }
    );
    req.on('error', reject);
    if (payload) {
      req.write(payload);
    }
    req.end();
  });
}

const uniqueId = (prefix) => `${prefix}_${Date.now()}_${Math.floor(Math.random() * 100000)}`;

// Register a fresh user via the public API. The verification code for
// @example.com addresses is the fixed "123456" (same as FVT).
async function registerUser({ username, email, password = 'Password_123456', displayName } = {}) {
  const id = username || uniqueId('e2e_user');
  const mail = email || `${id}@example.com`;
  await request('POST', '/api/v1/verification-code', { email: mail });
  const resp = await request('POST', '/api/v1/register', {
    id,
    email: mail,
    password,
    display_name: displayName || `E2E ${id}`,
    verification_code: '123456',
  });
  return { id: resp.data.id, token: resp.data.token, email: mail, password, displayName: displayName || `E2E ${id}` };
}

async function loginUser(email, password) {
  const resp = await request('POST', '/api/v1/login', { email, password });
  return {
    id: resp.data.id,
    token: resp.data.token,
    displayName: resp.data.display_name || '',
    email,
  };
}

async function adminLogin(email = 'admin@example.com', password = 'Admin_123456') {
  const resp = await request('POST', '/api/v1/admin/login', { email, password });
  return { token: resp.data.token, user: resp.data.user };
}

async function createTemplate(token, { title, description = 'e2e fixture', content = 'Hello ${name}', visibility = 'public', category = 'general', tags = ['e2e'] } = {}) {
  const resp = await request('POST', '/api/v1/templates', {
    title: title || `E2E Template ${Date.now()}`,
    description,
    content,
    visibility,
    category,
    tags,
  }, { token });
  return resp.data.template;
}

// Update a template via PUT. A new version is created only when the content
// differs from the latest version; change_note (<=200 chars) is attached to it.
async function updateTemplate(token, template, { content, changeNote = '' } = {}) {
  const resp = await request('PUT', `/api/v1/templates/${template.id}`, {
    template_id: template.id,
    owner_id: template.owner_id,
    title: template.title,
    description: template.description || '',
    visibility: 'public',
    category: template.category || 'general',
    tags: template.tags || [],
    content,
    change_note: changeNote,
  }, { token });
  return resp.data;
}

async function likeTemplate(token, templateId) {
  return request('POST', `/api/v1/templates/${templateId}/like`, null, { token });
}

// Like and favorite are toggles: a second POST removes the state.
async function unlikeTemplate(token, templateId) {
  return request('POST', `/api/v1/templates/${templateId}/like`, null, { token });
}

async function favoriteTemplate(token, templateId) {
  return request('POST', `/api/v1/templates/${templateId}/favorite`, null, { token });
}

async function unfavoriteTemplate(token, templateId) {
  return request('POST', `/api/v1/templates/${templateId}/favorite`, null, { token });
}

async function createComment(token, templateId, content) {
  const resp = await request('POST', `/api/v1/templates/${templateId}/comments`, { content }, { token });
  return resp.data;
}

async function listComments(token, templateId, page = 1) {
  const resp = await request('GET', `/api/v1/templates/${templateId}/comments?page=${page}`, null, { token });
  return resp.data;
}

async function getNotifications(token, { filter = 'all', pageSize = 20, pageToken = '' } = {}) {
  const params = new URLSearchParams({ filter, page_size: String(pageSize) });
  if (pageToken) params.set('page_token', pageToken);
  const resp = await request('GET', `/api/v1/notifications?${params.toString()}`, null, { token });
  return resp.data;
}

module.exports = {
  request,
  uniqueId,
  registerUser,
  loginUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  likeTemplate,
  unlikeTemplate,
  favoriteTemplate,
  unfavoriteTemplate,
  createComment,
  listComments,
  getNotifications,
};
