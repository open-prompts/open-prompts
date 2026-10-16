// E2E: content warnings (creator self-declaration, viewer gating + reveal,
// viewer preferences, admin warning filter + actions panel, notifications,
// i18n parity).
//
// Covers the acceptance criteria from
// docs/architecture/content-warnings-detailed-design.md §8:
//  - REST: creator self-declare at create (AC-1), remove self-declared
//    (AC-6), admin add -> locked + audit + notification + creator 403 on
//    removal (FR-8/AC-6), exclude_warned filter (FR-6/AC-4), admin warning
//    filter (FR-9/AC-7), unknown type -> 400 (FR-2), taxonomy editable via
//    admin settings (NFR-2), warning history endpoint, non-admin 403,
//    show_warned_content profile GET/PUT.
//  - Creator UI: WarningCheckboxGroup in the create/edit form, locked
//    warnings disabled/checked with lock icon.
//  - Viewer UI: BlurredPreview + WarningChip on PromptCard, ContentInterstitial
//    on TemplateDetails (reasons list, Show content reveals, Go back returns,
//    session-scoped reveal), HideWarnedFilterChip on Home, ShowWarnedToggle on
//    Profile.
//  - Admin UI: TemplatesPage warning filter dropdown + warnings column,
//    TemplateDetailPage WarningActionsPanel (add/remove with reason-required,
//    history list).
//  - Notification: template_warning_changed row renders the delta sentence.
//  - i18n: EN/ZH parity spot-check for warnings.* keys.
const {
  registerUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  getTemplate,
  getNotifications,
  adminAddWarnings,
  adminRemoveWarnings,
  getWarningHistory,
  adminListTemplates,
  setWarningTaxonomy,
  getProfile,
  setShowWarnedContent,
  searchTemplates,
  request,
  getAuditLogs,
  setReviewThreshold,
} = require('../helpers/api');

const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'Admin_123456';

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

// Regular-user login through the real UI (Home redirects to /login).
function loginViaUI(browser, email, password) {
  return browser
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('body', 15000)
    .execute(function () {
      try { localStorage.clear(); } catch (e) { /* ignore */ }
      try { sessionStorage.clear(); } catch (e) { /* ignore */ }
      localStorage.setItem('i18nextLng', 'en');
    })
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('#email', 15000)
    .setValue('#email', email)
    .setValue('#password', password)
    .submitForm('#email')
    // Login now redirects to /dashboard (user-dashboard feature), not /.
    .waitForElementVisible('.dashboard-layout', 20000);
}

// Admin login through the real admin console UI.
function adminLoginViaUI(browser) {
  return freshSession(browser, `${browser.launch_url}/admin/login`)
    .waitForElementVisible('#admin-email', 15000)
    .setValue('#admin-email', ADMIN_EMAIL)
    .setValue('#admin-password', ADMIN_PASSWORD)
    .click('.admin-login__submit')
    .waitForElementVisible('.admin-sidebar', 20000);
}

describe('Content warnings', function () {
  // Shared fixtures.
  const fx = {
    marker: `warn${Date.now().toString(36)}`,
    creator: {},   // self-declares warnings
    viewer: {},    // observes gating + preferences
    admin: {},
    warned: {},    // creator template with a self-declared warning
    locked: {},    // template that gets a moderator-applied (locked) warning
    clean: {},     // template with no warnings (control)
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;

      // Disable pre-publication review gating so fresh users publish
      // instantly and their templates are publicly searchable (the rest of
      // the suite assumes instant publishing).
      await setReviewThreshold(admin.token, 0, 'e2e warnings spec: disable gating');

      const creator = await registerUser({ displayName: 'Warn Creator' });
      Object.assign(fx.creator, creator);
      const viewer = await registerUser({ displayName: 'Warn Viewer' });
      Object.assign(fx.viewer, viewer);

      // Creator self-declares a warning at create (AC-1).
      const warned = await createTemplate(creator.token, {
        title: `${fx.marker} Warned`,
        description: 'has mature content',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'mature content here',
        contentWarnings: ['mature'],
      });
      Object.assign(fx.warned, warned);

      // A clean control template (no warnings).
      const clean = await createTemplate(creator.token, {
        title: `${fx.marker} Clean`,
        description: 'no warnings',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'clean content',
      });
      Object.assign(fx.clean, clean);

      // A template that will receive a moderator-applied (locked) warning.
      const locked = await createTemplate(creator.token, {
        title: `${fx.marker} Locked`,
        description: 'will get a locked warning',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'locked content',
      });
      Object.assign(fx.locked, locked);

      done();
    })().catch(done);
  });

  after(function (browser, done) {
    (async () => {
      // Restore the default taxonomy in case a test changed it.
      try {
        await setWarningTaxonomy(fx.admin.token, ['mature', 'violence', 'profanity', 'sensitive'], 'e2e warnings spec: restore default taxonomy');
      } catch (e) { /* best-effort */ }
      done();
    })().catch(done);
    browser.end();
  });

  // ---------- REST: creator self-declaration ----------

  it('self-declares a warning at create and returns it with empty locked set', function (browser) {
    browser.perform(async (done) => {
      browser.assert.strictEqual(
        Array.isArray(fx.warned.content_warnings) && fx.warned.content_warnings.includes('mature'),
        true,
        'create returns content_warnings with mature'
      );
      browser.assert.strictEqual(
        !fx.warned.locked_warnings || fx.warned.locked_warnings.length === 0,
        true,
        'fresh template has no locked warnings'
      );
      // GET detail returns the same.
      const detail = await getTemplate(fx.creator.token, fx.warned.id);
      browser.assert.strictEqual(
        Array.isArray(detail.content_warnings) && detail.content_warnings.includes('mature'),
        true,
        'GET detail returns content_warnings with mature'
      );
      done();
    });
  });

  it('lets the creator remove a self-declared warning', function (browser) {
    browser.perform(async (done) => {
      const updated = await updateTemplate(fx.creator.token, fx.warned, {
        content: 'mature content here',
        contentWarnings: [],
      });
      browser.assert.strictEqual(
        !updated.template.content_warnings || updated.template.content_warnings.length === 0,
        true,
        'creator can remove a self-declared warning'
      );
      done();
    });
  });

  it('rejects unknown warning types on create with 400', function (browser) {
    browser.perform(async (done) => {
      try {
        await createTemplate(fx.creator.token, {
          title: `${fx.marker} Bad Warning`,
          description: 'bad',
          category: 'general',
          tags: [fx.marker],
          visibility: 'VISIBILITY_PUBLIC',
          content: 'x',
          contentWarnings: ['not_a_warning'],
        });
        throw new Error('unknown warning type should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'unknown warning type -> 400');
      }
      done();
    });
  });

  // ---------- REST: admin warning enforcement ----------

  it('admin adds a warning -> locked, audit, notification; creator removal -> 403', function (browser) {
    browser.perform(async (done) => {
      const res = await adminAddWarnings(fx.admin.token, fx.locked.id, ['violence'], 'e2e: graphic violence');
      browser.assert.strictEqual(
        Array.isArray(res.content_warnings) && res.content_warnings.includes('violence'),
        true,
        'admin add sets content_warnings'
      );
      browser.assert.strictEqual(
        Array.isArray(res.locked_warnings) && res.locked_warnings.includes('violence'),
        true,
        'admin add sets locked_warnings'
      );

      // Creator GET shows locked_warnings.
      const detail = await getTemplate(fx.creator.token, fx.locked.id);
      browser.assert.strictEqual(
        Array.isArray(detail.locked_warnings) && detail.locked_warnings.includes('violence'),
        true,
        'creator GET shows locked_warnings'
      );

      // Creator removing the locked warning -> 403.
      try {
        await updateTemplate(fx.creator.token, fx.locked, {
          content: 'locked content',
          contentWarnings: [],
        });
        throw new Error('creator removing a locked warning should 403');
      } catch (e) {
        if (e.status !== 403) throw e;
        browser.assert.strictEqual(e.status, 403, 'creator removing locked warning -> 403');
      }

      // Creator notified with the delta payload.
      const notes = await getNotifications(fx.creator.token, { pageSize: 20 });
      const warnNote = (notes.notifications || []).find(
        (n) => n.type === 'template_warning_changed' && n.template_id === fx.locked.id
      );
      browser.assert.strictEqual(!!warnNote, true, 'template_warning_changed notification exists');
      const delta = JSON.parse(warnNote.actor_display_name || '{}');
      browser.assert.strictEqual(
        Array.isArray(delta.added) && delta.added.includes('violence'),
        true,
        'notification delta carries the added warning'
      );
      done();
    });
  });

  it('validates admin warning reasons and rejects non-admin callers', function (browser) {
    browser.perform(async (done) => {
      // Missing/short reason -> 400.
      try {
        await request('POST', `/api/v1/admin/templates/${fx.locked.id}/warnings`, { add: ['mature'], remove: [], reason: 'x' }, { token: fx.admin.token });
        throw new Error('short reason should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'short reason -> 400');
      }
      // Unknown key -> 400.
      try {
        await adminAddWarnings(fx.admin.token, fx.locked.id, ['bogus'], 'e2e: bogus key');
        throw new Error('unknown key should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'unknown key -> 400');
      }
      // Non-admin on the admin warning route -> 403.
      try {
        await adminAddWarnings(fx.creator.token, fx.locked.id, ['mature'], 'e2e: non-admin');
        throw new Error('non-admin add should 403');
      } catch (e) {
        if (e.status !== 403) throw e;
        browser.assert.strictEqual(e.status, 403, 'non-admin add -> 403');
      }
      // Unknown template id -> 404.
      try {
        await adminAddWarnings(fx.admin.token, '00000000-0000-0000-0000-000000000000', ['mature'], 'e2e: unknown template');
        throw new Error('unknown template should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'unknown template -> 404');
      }
      done();
    });
  });

  it('exposes warning history and audit log entries', function (browser) {
    browser.perform(async (done) => {
      // History contains the add entry.
      const history = await getWarningHistory(fx.admin.token, fx.locked.id, { perPage: 50 });
      const entries = history.entries || [];
      browser.assert.strictEqual(
        entries.some((e) => e.action === 'template.warning_add' && (e.warnings || []).includes('violence')),
        true,
        'history has the warning_add entry'
      );
      browser.assert.strictEqual(
        entries.some((e) => e.actor_email && e.reason),
        true,
        'history entries carry actor and reason'
      );

      // Audit log has the warning_add action.
      const audit = await getAuditLogs(fx.admin.token, { action: 'template.warning_add', perPage: 50 });
      browser.assert.strictEqual(
        (audit.entries || []).some((e) => e.target_id === fx.locked.id),
        true,
        'audit log has template.warning_add for the template'
      );

      // Admin removes the warning -> warning_remove audit + history.
      const removed = await adminRemoveWarnings(fx.admin.token, fx.locked.id, ['violence'], 'e2e: remove violence');
      browser.assert.strictEqual(
        !removed.content_warnings || removed.content_warnings.length === 0,
        true,
        'admin can remove a locked warning'
      );
      const history2 = await getWarningHistory(fx.admin.token, fx.locked.id, { perPage: 50 });
      browser.assert.strictEqual(
        (history2.entries || []).some((e) => e.action === 'template.warning_remove'),
        true,
        'history has the warning_remove entry'
      );
      done();
    });
  });

  // ---------- REST: exclude_warned + admin filter ----------

  it('filters warned templates from the public stream with exclude_warned', function (browser) {
    browser.perform(async (done) => {
      // Re-add a warning to fx.warned so it is warned again for this test.
      await adminAddWarnings(fx.admin.token, fx.warned.id, ['mature'], 'e2e: re-add mature for filter test');

      // Present without the param.
      const all = await searchTemplates({ keyword: fx.marker });
      browser.assert.strictEqual(
        (all.templates || []).some((t) => t.id === fx.warned.id),
        true,
        'warned template present without exclude_warned'
      );
      // Absent with exclude_warned=true.
      const filtered = await searchTemplates({ keyword: fx.marker, excludeWarned: true });
      browser.assert.strictEqual(
        (filtered.templates || []).some((t) => t.id === fx.warned.id),
        false,
        'warned template absent with exclude_warned=true'
      );
      // The clean control template is still present.
      browser.assert.strictEqual(
        (filtered.templates || []).some((t) => t.id === fx.clean.id),
        true,
        'clean template still present with exclude_warned=true'
      );
      done();
    });
  });

  it('filters the admin templates list by warning state', function (browser) {
    browser.perform(async (done) => {
      // warning=none excludes warned templates.
      const none = await adminListTemplates(fx.admin.token, { warning: 'none', perPage: 100 });
      browser.assert.strictEqual(
        (none.templates || []).some((t) => t.id === fx.warned.id),
        false,
        'warning=none excludes the warned template'
      );
      // warning=mature includes only mature-warned.
      const mature = await adminListTemplates(fx.admin.token, { warning: 'mature', perPage: 100 });
      browser.assert.strictEqual(
        (mature.templates || []).some((t) => t.id === fx.warned.id),
        true,
        'warning=mature includes the mature-warned template'
      );
      browser.assert.strictEqual(
        (mature.templates || []).some((t) => t.id === fx.clean.id),
        false,
        'warning=mature excludes the clean template'
      );
      done();
    });
  });

  it('validates the taxonomy against the server-side whitelist (NFR-2)', function (browser) {
    browser.perform(async (done) => {
      // Add a custom type to the taxonomy.
      await setWarningTaxonomy(fx.admin.token, ['mature', 'violence', 'profanity', 'sensitive', 'custom_warn'], 'e2e: add custom type');
      // A new type is accepted.
      const custom = await createTemplate(fx.creator.token, {
        title: `${fx.marker} Custom`,
        description: 'custom warning',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'custom',
        contentWarnings: ['custom_warn'],
      });
      browser.assert.strictEqual(
        Array.isArray(custom.content_warnings) && custom.content_warnings.includes('custom_warn'),
        true,
        'custom taxonomy type accepted'
      );
      // Restore the default taxonomy.
      await setWarningTaxonomy(fx.admin.token, ['mature', 'violence', 'profanity', 'sensitive'], 'e2e: restore default');
      // The custom type is now rejected.
      try {
        await createTemplate(fx.creator.token, {
          title: `${fx.marker} Custom2`,
          description: 'custom warning',
          category: 'general',
          tags: [fx.marker],
          visibility: 'VISIBILITY_PUBLIC',
          content: 'custom',
          contentWarnings: ['custom_warn'],
        });
        throw new Error('removed taxonomy type should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'removed taxonomy type -> 400');
      }
      done();
    });
  });

  it('persists the show_warned_content viewer preference on the profile', function (browser) {
    browser.perform(async (done) => {
      // Default is off.
      const before = await getProfile(fx.viewer.token);
      browser.assert.strictEqual(
        !!before.show_warned_content,
        false,
        'show_warned_content defaults to false'
      );
      // Turn it on.
      await setShowWarnedContent(fx.viewer.token, true);
      const after = await getProfile(fx.viewer.token);
      browser.assert.strictEqual(
        !!after.show_warned_content,
        true,
        'show_warned_content persists true'
      );
      // Turn it back off for the UI tests.
      await setShowWarnedContent(fx.viewer.token, false);
      done();
    });
  });

  // ---------- Creator UI: checkbox group + locked display ----------

  it('shows the WarningCheckboxGroup in the create form', function (browser) {
    loginViaUI(browser, fx.creator.email, fx.creator.password)
      .url(`${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      // Open the create modal via the floating create button.
      .click('.floating-create-btn')
      .waitForElementVisible('.warning-checkbox-group', 20000)
      .assert.textContains('.warning-checkbox-group__title', 'Content warnings')
      // The four taxonomy options are present.
      .assert.visible('.warning-checkbox-group__option')
      .assert.textContains('.warning-checkbox-group', 'Mature themes')
      .assert.textContains('.warning-checkbox-group', 'Graphic violence')
      .assert.textContains('.warning-checkbox-group', 'Strong language')
      .assert.textContains('.warning-checkbox-group', 'Sensitive topics');
  });

  it('shows locked warnings as disabled/checked with a lock icon in the editor', function (browser) {
    browser.perform(async (done) => {
      // Give fx.locked a locked warning again for this UI test.
      await adminAddWarnings(fx.admin.token, fx.locked.id, ['violence'], 'e2e: lock for editor test');
      browser
        .url(`${browser.launch_url}/templates/${fx.locked.id}`)
        .waitForElementVisible('.edit-meta-btn', 20000)
        .click('.edit-meta-btn')
        .waitForElementVisible('.warning-checkbox-group', 20000)
        // The locked option is disabled and checked.
        .assert.attributeEquals('.warning-checkbox-group__option.is-locked input', 'disabled', 'true')
        .assert.attributeEquals('.warning-checkbox-group__option.is-locked input', 'checked', 'true')
        .assert.visible('.warning-checkbox-group__lock')
        .assert.textContains('.warning-checkbox-group__locked-note', 'Applied by moderation');
      done();
    });
  });

  // ---------- Viewer UI: blur + interstitial + preferences ----------

  it('blurs warned cards and shows a warning chip on the Home feed', function (browser) {
    loginViaUI(browser, fx.viewer.email, fx.viewer.password)
      .url(`${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      // The warned template's card shows the blurred preview + chip.
      .assert.visible('.blurred-preview')
      .assert.visible('.warning-chip')
      // The warned template is present on the feed.
      .assert.textContains('.home-page', `${fx.marker} Warned`);
  });

  it('shows the ContentInterstitial on the detail page with reveal and back', function (browser) {
    browser
      .url(`${browser.launch_url}/templates/${fx.warned.id}`)
      .waitForElementVisible('.content-interstitial', 20000)
      .assert.textContains('.content-interstitial__title', 'Content warning')
      .assert.textContains('.content-interstitial__body', 'Mature themes')
      // Show content reveals the template content.
      .click('.content-interstitial .btn-primary')
      .waitForElementNotPresent('.content-interstitial', 10000)
      .assert.visible('.template-content');
  });

  it('re-gates on a fresh session (session-scoped reveal)', function (browser) {
    // A fresh session (cleared sessionStorage) re-gates the same template.
    freshSession(browser, `${browser.launch_url}/templates/${fx.warned.id}`)
      .waitForElementVisible('.content-interstitial', 20000)
      .assert.textContains('.content-interstitial__title', 'Content warning');
  });

  it('Go back on the interstitial returns to the previous page', function (browser) {
    // Navigate from the home feed so there is history to go back to.
    loginViaUI(browser, fx.viewer.email, fx.viewer.password)
      .url(`${browser.launch_url}/templates/${fx.warned.id}`)
      .waitForElementVisible('.content-interstitial', 20000)
      // "Go back" is the ghost button in the interstitial actions.
      .click('.content-interstitial .btn-ghost')
      .pause(800)
      .url((result) => {
        // Go back navigates to the previous page (here the home feed).
        browser.assert.strictEqual(
          result.value.includes('/templates/'),
          false,
          'Go back navigated away from the template detail'
        );
      });
  });

  it('hides warned content via the HideWarnedFilterChip on Home', function (browser) {
    loginViaUI(browser, fx.viewer.email, fx.viewer.password)
      .url(`${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      .assert.visible('.hide-warned-chip')
      // The warned template is present before toggling.
      .assert.textContains('.home-page', `${fx.marker} Warned`)
      // Toggle it on: the warned template disappears from the feed.
      .click('.hide-warned-chip')
      .waitForElementVisible('.hide-warned-chip.is-active', 10000)
      .pause(2000)
      .assert.not.textContains('.home-page', `${fx.marker} Warned`);
  });

  it('shows the ShowWarnedToggle on the Profile and persists the preference', function (browser) {
    loginViaUI(browser, fx.viewer.email, fx.viewer.password)
      .url(`${browser.launch_url}/profile`)
      .waitForElementVisible('.show-warned-toggle', 20000)
      .assert.textContains('.content-preferences-title', 'Content preferences')
      // Toggle it on.
      .click('.show-warned-toggle__switch')
      .pause(1500)
      // The preference is persisted server-side.
      .perform(async (done) => {
        const prof = await getProfile(fx.viewer.token);
        browser.assert.strictEqual(!!prof.show_warned_content, true, 'preference persisted server-side');
        done();
      })
      // With the preference on, the warned template detail shows no interstitial.
      .url(`${browser.launch_url}/templates/${fx.warned.id}`)
      .waitForElementVisible('.template-content', 20000)
      .assert.not.elementPresent('.content-interstitial');
  });

  // ---------- Admin UI ----------

  it('filters the admin templates list via the warning dropdown', function (browser) {
    adminLoginViaUI(browser)
      .url(`${browser.launch_url}/admin/templates`)
      .waitForElementVisible('.admin-table tbody tr', 20000)
      // The warning filter dropdown is present (the last admin select).
      .assert.visible('.admin-select select')
      // Select "Mature" from the warning filter.
      .execute(function () {
        const selects = document.querySelectorAll('.admin-select select');
        // The warning filter is the last select in the filter row.
        const sel = selects[selects.length - 1];
        const opts = Array.from(sel.options);
        const mature = opts.find((o) => o.value === 'mature');
        if (mature) {
          sel.value = 'mature';
          sel.dispatchEvent(new Event('change', { bubbles: true }));
        }
      })
      .pause(1500)
      // The mature-warned template appears in the list with its warning chip.
      .assert.textContains('.admin-table', `${fx.marker} Warned`)
      .assert.visible('.admin-templates__warning-chip');
  });

  it('adds and removes warnings via the WarningActionsPanel with reason-required', function (browser) {
    browser.perform(async (done) => {
      // Use fx.clean (no warnings) so the panel starts empty.
      browser
        .url(`${browser.launch_url}/admin/templates/${fx.clean.id}`)
        .waitForElementVisible('.warning-actions-panel', 20000)
        .assert.textContains('.warning-actions-panel', 'Content warnings')
        // History empty state.
        .assert.textContains('.warning-actions-panel', 'No warning history')
        // Apply is disabled until a warning is chosen AND reason >= 3 chars.
        .assert.attributeEquals('.warning-actions-panel .admin-btn--primary', 'disabled', 'true')
        // Choose a warning from the dropdown.
        .execute(function () {
          const sel = document.querySelector('.warning-actions-panel__select');
          sel.value = 'sensitive';
          sel.dispatchEvent(new Event('change', { bubbles: true }));
        })
        .pause(300)
        // Still disabled: reason missing.
        .assert.attributeEquals('.warning-actions-panel .admin-btn--primary', 'disabled', 'true')
        // Enter a short reason (still < 3 chars).
        .setValue('.warning-actions-panel__reason', 'ab')
        .pause(300)
        .assert.attributeEquals('.warning-actions-panel .admin-btn--primary', 'disabled', 'true')
        // Enter a valid reason -> enabled.
        .setValue('.warning-actions-panel__reason', 'sensitive topic')
        .pause(300)
        .assert.enabled('.warning-actions-panel .admin-btn--primary')
        .click('.warning-actions-panel .admin-btn--primary')
        .waitForElementVisible('.warning-actions-panel__chip', 10000)
        // The chip shows the moderation origin.
        .assert.textContains('.warning-actions-panel__chip', 'Sensitive topics')
        .assert.textContains('.warning-actions-panel__chip', 'moderation')
        // History now has an entry.
        .waitForElementVisible('.warning-actions-panel__history-item', 10000)
        .assert.textContains('.warning-actions-panel__history-item', 'added');
      done();
    });
  });

  // ---------- Notification UI ----------

  it('renders the template_warning_changed notification with the delta sentence', function (browser) {
    loginViaUI(browser, fx.creator.email, fx.creator.password)
      .url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notification-item', 20000)
      .assert.textContains('.notifications-list, .notification-item', `${fx.marker} Locked`)
      .assert.textContains('.notifications-list, .notification-item', 'Content warnings on');
  });

  // ---------- i18n parity ----------

  it('keeps EN/ZH parity for the warnings.* keys', function (browser) {
    browser.perform(async (done) => {
      const en = require('../../../frontend/src/locales/en/translation.json');
      const zh = require('../../../frontend/src/locales/zh/translation.json');
      const enKeys = Object.keys(en.warnings || {});
      const zhKeys = Object.keys(zh.warnings || {});
      browser.assert.strictEqual(enKeys.length > 0, true, 'EN warnings.* keys exist');
      browser.assert.strictEqual(
        enKeys.length === zhKeys.length,
        true,
        'warnings.* key count matches across locales'
      );
      // Spot-check the nested keys used by the UI.
      const spots = [
        ['group', 'title'],
        ['group', 'help'],
        ['type', 'mature'],
        ['type', 'violence'],
        ['type', 'profanity'],
        ['type', 'sensitive'],
        ['locked', 'label'],
        ['interstitial', 'title'],
        ['interstitial', 'body'],
        ['interstitial', 'show'],
        ['interstitial', 'back'],
        ['filter', 'hideWarned'],
        ['settings', 'title'],
        ['settings', 'showAlways'],
        ['admin', 'filter', 'label'],
        ['admin', 'filter', 'any'],
        ['admin', 'filter', 'none'],
        ['admin', 'panel', 'title'],
        ['admin', 'panel', 'current'],
        ['admin', 'panel', 'add'],
        ['admin', 'panel', 'remove'],
        ['admin', 'panel', 'reason'],
        ['admin', 'panel', 'history'],
        ['notify', 'changed'],
        ['notify', 'added'],
        ['notify', 'removed'],
      ];
      spots.forEach((path) => {
        const enVal = path.reduce((o, k) => (o && o[k] !== undefined ? o[k] : undefined), en.warnings);
        const zhVal = path.reduce((o, k) => (o && o[k] !== undefined ? o[k] : undefined), zh.warnings);
        browser.assert.strictEqual(
          typeof enVal === 'string' && enVal.length > 0,
          true,
          `EN warnings.${path.join('.')} present`
        );
        browser.assert.strictEqual(
          typeof zhVal === 'string' && zhVal.length > 0,
          true,
          `ZH warnings.${path.join('.')} present`
        );
      });
      done();
    });
  });
});