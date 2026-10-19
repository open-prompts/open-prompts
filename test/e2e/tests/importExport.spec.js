// E2E: template import/export.
//
// Covers the acceptance criteria from
// docs/architecture/import-export-detailed-design.md §6.3:
//  - REST: GET /api/v1/admin/templates/{id} returns `category` on the summary
//    and `change_note` per version (NULL -> ""), so admin exports are
//    byte-compatible with user exports; non-admin -> 403.
//  - UI: ExportButton on the owner's TemplateDetails (absent for non-owner),
//    ImportButton on Home for authenticated users (absent anonymous),
//    ExportButton on the admin TemplateDetailPage.
//  - i18n: EN/ZH parity for all import_export.* keys.
//
// NOTE: the full owner-export -> second-account-import round trip involves
// browser file download/upload, which is not reliably automatable in headless
// Nightwatch. The backend read-path contract (which the export builder relies
// on) is pinned here via the admin API, and the UI presence/absence of the
// export/import entry points is covered. The client-side file validation
// (parseTemplateExport) is covered by frontend unit tests.
const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  setReviewThreshold,
  getAdminTemplateDetail,
} = require('../helpers/api');

const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'Admin_123456';

// Wrap an API call so a non-2xx response resolves to { status, body, headers }
// instead of rejecting — for negative-path assertions.
function expectError(promise) {
  return promise.then(
    () => ({ status: 0, body: null, headers: {} }),
    (err) => ({ status: err.status, body: err.body, headers: err.headers || {} })
  );
}

// Fresh browser session: clear storage so stale sessions never leak
// between tests, and pin the UI language to English for text assertions.
function freshSession(browser, url) {
  return browser
    .url(url)
    .waitForElementVisible('body', 15000)
    .execute(function () {
      try { localStorage.clear(); } catch (e) { /* ignore */ }
      try { sessionStorage.clear(); } catch (e) { /* ignore */ }
      localStorage.setItem('i18nextLng', 'en');
    })
    .url(url);
}

describe('Template import/export', function () {
  // Shared fixtures.
  const fx = {
    marker: `ie${Date.now().toString(36)}`,
    owner: {},   // template owner (sees ExportButton)
    stranger: {}, // non-owner (no ExportButton)
    admin: {},
    template: {}, // owner's public template with 2 versions
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e import-export spec: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'ImportExport Owner' }));
      Object.assign(fx.stranger, await registerUser({ displayName: 'ImportExport Stranger' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Exportable`,
        description: 'import/export parity',
        category: 'Writing',
        tags: ['export'],
        content: 'v1 content',
      });
      Object.assign(fx.template, t);
      // v2 with a change note.
      await updateTemplate(fx.owner.token, t, {
        content: 'v2 content',
        changeNote: 'second revision',
      });
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------- REST contract ----------

  it('returns category on the admin summary and change_note per version (NULL -> "")', function (browser) {
    browser.perform(async (done) => {
      try {
        const detail = await getAdminTemplateDetail(fx.admin.token, fx.template.id);
        browser.assert.strictEqual(detail.template.category, 'Writing', 'admin summary carries category');
        browser.assert.strictEqual(Array.isArray(detail.versions), true, 'versions is an array');
        browser.assert.strictEqual(detail.versions.length, 2, 'two versions');
        // Newest first.
        browser.assert.strictEqual(detail.versions[0].version, 2, 'v2 first (newest)');
        browser.assert.strictEqual(detail.versions[0].change_note, 'second revision', 'v2 change_note');
        browser.assert.strictEqual(detail.versions[1].version, 1, 'v1 second');
        browser.assert.strictEqual(detail.versions[1].change_note, '', 'v1 change_note NULL -> ""');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a non-admin caller on the admin template detail with 403', function (browser) {
    browser.perform(async (done) => {
      try {
        const r = await expectError(
          request('GET', `/api/v1/admin/templates/${fx.template.id}`, null, { token: fx.stranger.token })
        );
        browser.assert.strictEqual(r.status, 403, 'non-admin on admin detail -> 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------- UI ----------

  it('shows the ExportButton to the owner on TemplateDetails', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.owner.email)
      .setValue('#password', fx.owner.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.template-details-page', 20000)
      .assert.elementPresent('.export-btn')
      .assert.textContains('.export-btn', 'Export');
  });

  it('does not show the ExportButton to a non-owner', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.stranger.email)
      .setValue('#password', fx.stranger.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.template-details-page', 20000)
      .assert.elementNotPresent('.export-btn');
  });

  it('shows the ImportButton on Home for an authenticated user', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.owner.email)
      .setValue('#password', fx.owner.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      .assert.elementPresent('.import-btn')
      .assert.textContains('.import-btn', 'Import');
  });

  it('does not show the ImportButton to an anonymous visitor', function (browser) {
    freshSession(browser, `${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      .assert.elementNotPresent('.import-btn');
  });

  it('shows the ExportButton on the admin TemplateDetailPage', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-sidebar', 20000)
      .url(`${browser.launch_url}/admin/templates/${fx.template.id}`)
      .waitForElementVisible('.admin-template-detail__header', 20000)
      .assert.elementPresent('.export-btn')
      .assert.textContains('.export-btn', 'Export');
  });

  // ---------- i18n parity ----------

  it('keeps EN/ZH parity for the import_export.* keys', function (browser) {
    browser.perform(async (done) => {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enKeys = Object.keys(en.import_export || {});
        const zhKeys = Object.keys(zh.import_export || {});
        browser.assert.strictEqual(
          enKeys.length === zhKeys.length,
          true,
          `import_export.* key count matches (EN ${enKeys.length} vs ZH ${zhKeys.length})`
        );
        // Spot-check the exact strings.
        browser.assert.strictEqual(en.import_export.export, 'Export');
        browser.assert.strictEqual(zh.import_export.export, '导出');
        browser.assert.strictEqual(en.import_export.import, 'Import');
        browser.assert.strictEqual(zh.import_export.import, '导入');
        browser.assert.strictEqual(en.import_export.import_title, 'Import template');
        browser.assert.strictEqual(zh.import_export.import_title, '导入模板');
        browser.assert.strictEqual(en.import_export.confirm, 'Import template');
        browser.assert.strictEqual(zh.import_export.confirm, '导入模板');
        browser.assert.strictEqual(en.import_export.import_success, 'Template imported successfully.');
        browser.assert.strictEqual(zh.import_export.import_success, '模板导入成功。');
        browser.assert.strictEqual(en.import_export.export_success, 'Exported "{{title}}" ({{count}} versions)');
        browser.assert.strictEqual(zh.import_export.export_success, '已导出“{{title}}”（{{count}} 个版本）');
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});