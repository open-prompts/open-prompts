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
    // Node's http.request does NOT auto-compute Content-Length when the
    // body is passed via write() — without this header a DELETE (or any
    // method) with a body is an invalid HTTP/1.1 frame and Go servers
    // reject it with 400 before any handler runs.
    if (payload) {
      headers['Content-Length'] = Buffer.byteLength(payload);
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
            resolve({ status: res.statusCode, data: parsed, headers: res.headers });
          } else {
            const err = new Error(
              `API ${method} ${path} -> ${res.statusCode}: ${data.slice(0, 300)}`
            );
            err.status = res.statusCode;
            err.body = parsed;
            err.headers = res.headers;
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

async function createTemplate(token, { title, description = 'e2e fixture', content = 'Hello ${name}', visibility = 'public', category = 'general', tags = ['e2e'], contentWarnings } = {}) {
  const body = {
    title: title || `E2E Template ${Date.now()}`,
    description,
    content,
    visibility,
    category,
    tags,
  };
  if (contentWarnings !== undefined) body.content_warnings = contentWarnings;
  const resp = await request('POST', '/api/v1/templates', body, { token });
  return resp.data.template;
}

// Update a template via PUT. A new version is created only when the content
// differs from the latest version; change_note (<=200 chars) is attached to it.
// contentWarnings (array) is the full desired set; locked warnings are enforced
// server-side (403 if a locked warning is dropped).
async function updateTemplate(token, template, { content, changeNote = '', contentWarnings } = {}) {
  const body = {
    template_id: template.id,
    owner_id: template.owner_id,
    title: template.title,
    description: template.description || '',
    visibility: 'public',
    category: template.category || 'general',
    tags: template.tags || [],
    content,
    change_note: changeNote,
  };
  if (contentWarnings !== undefined) body.content_warnings = contentWarnings;
  const resp = await request('PUT', `/api/v1/templates/${template.id}`, body, { token });
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

// page_size defaults to 20 server-side (max 50); newest comments first.
async function listComments(token, templateId, page = 1, pageSize) {
  let path = `/api/v1/templates/${templateId}/comments?page=${page}`;
  if (pageSize) {
    path += `&page_size=${pageSize}`;
  }
  const resp = await request('GET', path, null, { token });
  return resp.data;
}

// Edit the caller's own comment within the 15-minute window.
async function updateComment(token, commentId, content) {
  const resp = await request('PUT', `/api/v1/comments/${commentId}`, { content }, { token });
  return resp.data;
}

// Delete a comment (author or template owner).
async function deleteComment(token, commentId) {
  const resp = await request('DELETE', `/api/v1/comments/${commentId}`, null, { token });
  return resp.data;
}

async function getTemplate(token, templateId) {
  const resp = await request('GET', `/api/v1/templates/${templateId}`, null, { token });
  return resp.data.template;
}

// owner_id is required by the gateway for DELETE /templates/{id}.
async function deleteTemplate(token, templateId, ownerId) {
  const resp = await request('DELETE', `/api/v1/templates/${templateId}?owner_id=${ownerId}`, null, { token });
  return resp.data;
}

// Admin comment moderation list. params: { q, templateId, userId, page, perPage }
async function adminListComments(adminToken, { q = '', templateId = '', userId = '', page = 1, perPage = 20 } = {}) {
  const params = new URLSearchParams({ q, template_id: templateId, user_id: userId, page: String(page), per_page: String(perPage) });
  const resp = await request('GET', `/api/v1/admin/comments?${params.toString()}`, null, { token: adminToken });
  return resp.data;
}

// reason: 3-500 chars, required. Writes an audit log entry.
async function adminDeleteComment(adminToken, commentId, reason) {
  const resp = await request('DELETE', `/api/v1/admin/comments/${commentId}`, { reason }, { token: adminToken });
  return resp.data;
}

// params: { action, actor, page, perPage }
async function getAuditLogs(adminToken, { action = '', actor = '', page = 1, perPage = 20 } = {}) {
  const params = new URLSearchParams({ action, actor, page: String(page), per_page: String(perPage) });
  const resp = await request('GET', `/api/v1/admin/audit-log?${params.toString()}`, null, { token: adminToken });
  return resp.data;
}

async function banUser(adminToken, userId, reason) {
  const resp = await request('POST', `/api/v1/admin/users/${userId}/ban`, { reason }, { token: adminToken });
  return resp.data;
}

async function getNotifications(token, { filter = 'all', pageSize = 20, pageToken = '' } = {}) {
  const params = new URLSearchParams({ filter, page_size: String(pageSize) });
  if (pageToken) params.set('page_token', pageToken);
  const resp = await request('GET', `/api/v1/notifications?${params.toString()}`, null, { token });
  return resp.data;
}

// Pre-publication review gating: set review_approval_threshold via the admin
// settings API. 0 disables gating entirely (fresh users publish instantly);
// the default is 3. Specs that are not about review gating disable it up
// front, mirroring the FVT legacy suites.
async function setReviewThreshold(adminToken, value, reason) {
  const resp = await request('PUT', '/api/v1/admin/settings', {
    key: 'review_approval_threshold',
    value: String(value),
    reason,
  }, { token: adminToken });
  return resp.data;
}

// List the admin review queue (pending pre-publication submissions).
// params: { category, agingOnly, resubmissionOnly, page, perPage }
async function listReviewQueue(adminToken, { category = '', agingOnly = false, resubmissionOnly = false, page = 1, perPage = 20 } = {}) {
  const params = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  if (category) params.set('category', category);
  if (agingOnly) params.set('aging', 'true');
  if (resubmissionOnly) params.set('resubmission_only', 'true');
  const resp = await request('GET', `/api/v1/admin/review?${params.toString()}`, null, { token: adminToken });
  return resp.data;
}

async function getReviewMetrics(adminToken) {
  const resp = await request('GET', '/api/v1/admin/review/metrics', null, { token: adminToken });
  return resp.data;
}

async function getReviewSubmission(adminToken, submissionId) {
  const resp = await request('GET', `/api/v1/admin/review/${submissionId}`, null, { token: adminToken });
  return resp.data.submission;
}

async function approveReviewSubmission(adminToken, submissionId) {
  const resp = await request('POST', `/api/v1/admin/review/${submissionId}/approve`, null, { token: adminToken });
  return resp.data;
}

// reasonCode must be one of: spam, plagiarism, harmful, lowQuality, other.
async function rejectReviewSubmission(adminToken, submissionId, reasonCode, note = '') {
  const body = { reason_code: reasonCode };
  if (note) body.note = note;
  const resp = await request('POST', `/api/v1/admin/review/${submissionId}/reject`, body, { token: adminToken });
  return resp.data;
}

// Search public templates. params: { keyword, q, sort, category, tags,
// visibility, pageToken, pageSize, myLikes, excludeWarned, token }
async function searchTemplates({ keyword, q, sort, category, tags, visibility, pageToken, pageSize, myLikes, excludeWarned, token } = {}) {
  const params = new URLSearchParams();
  if (keyword !== undefined) params.set('keyword', keyword);
  if (q !== undefined) params.set('q', q);
  if (sort !== undefined) params.set('sort', sort);
  if (category) params.set('category', category);
  if (tags && tags.length > 0) tags.forEach((tg) => params.append('tags', tg));
  if (visibility) params.set('visibility', visibility);
  if (pageToken) params.set('page_token', pageToken);
  if (pageSize) params.set('page_size', String(pageSize));
  if (myLikes) params.set('my_likes', 'true');
  if (excludeWarned) params.set('exclude_warned', 'true');
  const resp = await request('GET', `/api/v1/templates?${params.toString()}`, null, token ? { token } : {});
  return resp.data;
}

// Admin: add content warnings to a template. reason (3-500 chars) required.
async function adminAddWarnings(adminToken, templateId, add, reason) {
  const resp = await request('POST', `/api/v1/admin/templates/${templateId}/warnings`, { add, remove: [], reason }, { token: adminToken });
  return resp.data;
}

// Admin: remove content warnings from a template. reason (3-500 chars) required.
async function adminRemoveWarnings(adminToken, templateId, remove, reason) {
  const resp = await request('DELETE', `/api/v1/admin/templates/${templateId}/warnings`, { add: [], remove, reason }, { token: adminToken });
  return resp.data;
}

// Admin: list warning change history for a template.
async function getWarningHistory(adminToken, templateId, { page = 1, perPage = 20 } = {}) {
  const params = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  const resp = await request('GET', `/api/v1/admin/templates/${templateId}/warnings/history?${params.toString()}`, null, { token: adminToken });
  return resp.data;
}

// Admin: list templates with an optional warning-state filter ("" | "none" | a key).
async function adminListTemplates(adminToken, { warning = '', q = '', page = 1, perPage = 20 } = {}) {
  const params = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  if (warning) params.set('warning', warning);
  if (q) params.set('q', q);
  const resp = await request('GET', `/api/v1/admin/templates?${params.toString()}`, null, { token: adminToken });
  return resp.data;
}

// Admin: set the content_warning_taxonomy platform setting (JSON array string).
async function setWarningTaxonomy(adminToken, taxonomyArray, reason) {
  const resp = await request('PUT', '/api/v1/admin/settings', {
    key: 'content_warning_taxonomy',
    value: JSON.stringify(taxonomyArray),
    reason,
  }, { token: adminToken });
  return resp.data;
}

// Get the caller's profile (carries show_warned_content).
async function getProfile(token) {
  const resp = await request('GET', '/api/v1/profile', null, { token });
  return resp.data;
}

// Set the caller's show_warned_content viewer preference.
async function setShowWarnedContent(token, value) {
  const resp = await request('PUT', '/api/v1/profile', { show_warned_content: value }, { token });
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
  updateComment,
  deleteComment,
  getTemplate,
  deleteTemplate,
  adminListComments,
  adminDeleteComment,
  getAuditLogs,
  banUser,
  getNotifications,
  setReviewThreshold,
  listReviewQueue,
  getReviewMetrics,
  getReviewSubmission,
  approveReviewSubmission,
  rejectReviewSubmission,
  searchTemplates,
  adminAddWarnings,
  adminRemoveWarnings,
  getWarningHistory,
  adminListTemplates,
  setWarningTaxonomy,
  getProfile,
  setShowWarnedContent,
};
