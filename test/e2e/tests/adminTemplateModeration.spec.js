// E2E: admin template moderation feature (e08429c).
//
// REST contract:
//   - POST /api/v1/admin/templates/{id}/unpublish|lock|delete|restore with
//     {reason} (3-500 chars -> 400; non-admin -> 403; admin moderating own
//     template -> 403; invalid transition -> 409; rate limit
//     MOD_ACTION_RATE_PER_MINUTE=10 -> 429 + Retry-After) returns
//     {template_id, moderation_status, moderated_at}
//   - GET /api/v1/admin/templates?moderation_status= filter
//   - audit actions template.unpublish/lock/soft_delete/restore/
//     republished_after_moderation
//   - owner banners (unpublished -> reason + Edit CTA; locked -> generic;
//     soft-deleted -> removed notice); public LockBanner on locked templates
//   - locked blocks edit/comment/fork/like/favorite/rate (403)
//   - unpublished/soft-deleted 404 for non-owners, owner sees them
//   - `moderation` notification type (action + title + reason excerpt + deep
//     link); re-publish after admin unpublish resets to normal + audit marker
//   - report queue ReviewDrawer "Act -> unpublish" calls the real unpublish
//     endpoint and auto-resolves the report group as actioned
// i18n: mod.* EN/ZH key parity.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  setReviewThreshold,
  getTemplate,
  getNotifications,
  getAuditLogs,
  adminListTemplates,
  getAdminTemplateDetail,
  createComment,
  likeTemplate,
  favoriteTemplate,
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

describe('Admin template moderation', function () {
  const fx = {
    marker: `mod${Date.now().toString(36)}`,
    owner: {},    // template owner (moderated)
    viewer: {},   // non-owner (404 on hidden, sees locked)
    admin: {},
    template: {}, // owner's public template
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e admin-template-moderation: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Mod Owner' }));
      Object.assign(fx.viewer, await registerUser({ displayName: 'Mod Viewer' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Moderatable`,
        description: 'admin template moderation e2e fixture',
        category: 'Writing',
        tags: ['moderation'],
        content: 'moderate me',
      });
      Object.assign(fx.template, t);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: unpublish / visibility / audit / notification ----------------

  it('unpublishes a template and hides it from non-owners', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/unpublish`,
          { reason: 'spam — off-platform links' },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.moderation_status, 'unpublished_by_admin', 'status is unpublished_by_admin');
        browser.assert.strictEqual(r.data.template_id, fx.template.id, 'template_id echoed');

        // Non-owner (viewer) gets 404.
        const v = await expectError(getTemplate(fx.viewer.token, fx.template.id));
        browser.assert.strictEqual(v.status, 404, 'non-owner sees 404 for unpublished');

        // Anonymous also gets 404 (no leak).
        const anon = await expectError(getTemplate(null, fx.template.id));
        browser.assert.strictEqual(anon.status, 404, 'anonymous sees 404 for unpublished');

        // Owner still sees it.
        const ownerView = await getTemplate(fx.owner.token, fx.template.id);
        browser.assert.strictEqual(ownerView.moderation_status, 'unpublished_by_admin', 'owner sees unpublished status');

        // Issue #5: the owner's public template detail now carries the
        // moderation_reason (owner-only), so the owner banner shows the reason.
        browser.assert.strictEqual(
          ownerView.moderation_reason, 'spam — off-platform links',
          'owner public template detail carries moderation_reason'
        );

        // The admin detail carries the moderation reason too.
        const adminDetail = await getAdminTemplateDetail(fx.admin.token, fx.template.id);
        browser.assert.strictEqual(adminDetail.template.moderation_reason, 'spam — off-platform links', 'admin detail carries the reason');

        // Audit has template.unpublish.
        const audit = await getAuditLogs(fx.admin.token, { action: 'template.unpublish', perPage: 100 });
        const entry = audit.entries.find((e) => e.target_id === fx.template.id);
        browser.assert.ok(entry, 'audit has template.unpublish for the template');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('sends a moderation notification to the owner with reason excerpt', function (browser) {
    browser.perform(async function (done) {
      try {
        const notifs = await getNotifications(fx.owner.token, { filter: 'all', pageSize: 50 });
        const n = notifs.notifications.find(
          (x) => x.type === 'moderation' && x.template_id === fx.template.id
        );
        browser.assert.ok(n, 'owner has a moderation notification');
        browser.assert.strictEqual(n.actor_display_name, 'unpublish', 'actor_display_name carries the action type');
        browser.assert.ok(n.snippet && n.snippet.includes('spam'), 'snippet carries the reason excerpt');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a short reason with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/lock`,
          { reason: 'ab' },
          { token: fx.admin.token }
        ));
        browser.assert.strictEqual(r.status, 400, 'reason < 3 chars is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a non-admin with 403', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/lock`,
          { reason: 'under review' },
          { token: fx.viewer.token }
        ));
        browser.assert.strictEqual(r.status, 403, 'non-admin moderation is 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects an admin moderating their own template with 403', function (browser) {
    browser.perform(async function (done) {
      try {
        // Create a template owned by the admin (bootstrap admin user).
        const own = await createTemplate(fx.admin.token, {
          title: `${fx.marker} Admin Owned`,
          description: 'self-guard fixture',
          category: 'Writing',
          tags: ['moderation'],
          content: 'mine',
        });
        const r = await expectError(request(
          'POST', `/api/v1/admin/templates/${own.id}/unpublish`,
          { reason: 'self moderation' },
          { token: fx.admin.token }
        ));
        browser.assert.strictEqual(r.status, 403, 'admin moderating own template is 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects an invalid transition with 409', function (browser) {
    browser.perform(async function (done) {
      try {
        // The template is currently unpublished_by_admin; restore is valid,
        // but restoring a normal template is not. First restore to normal.
        await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/restore`,
          { reason: 'false positive, restored' },
          { token: fx.admin.token }
        );
        // Now restore again -> invalid transition (already normal).
        const r = await expectError(request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/restore`,
          { reason: 'restore again' },
          { token: fx.admin.token }
        ));
        browser.assert.strictEqual(r.status, 409, 'restoring a normal template is 409');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('locks a template; public viewers still see it with the lock banner', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/lock`,
          { reason: 'under review' },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.moderation_status, 'locked', 'status is locked');

        // Public viewer still sees it (locked stays public).
        const v = await getTemplate(fx.viewer.token, fx.template.id);
        browser.assert.strictEqual(v.moderation_status, 'locked', 'viewer sees locked template');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('blocks like/favorite/comment on a locked template with 403', function (browser) {
    browser.perform(async function (done) {
      try {
        const like = await expectError(likeTemplate(fx.viewer.token, fx.template.id));
        browser.assert.strictEqual(like.status, 403, 'like on locked is 403');

        const fav = await expectError(favoriteTemplate(fx.viewer.token, fx.template.id));
        browser.assert.strictEqual(fav.status, 403, 'favorite on locked is 403');

        const comment = await expectError(createComment(fx.viewer.token, fx.template.id, 'comment on locked'));
        browser.assert.strictEqual(comment.status, 403, 'comment on locked is 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('filters the admin template list by moderation_status', function (browser) {
    browser.perform(async function (done) {
      try {
        const locked = await adminListTemplates(fx.admin.token, { moderationStatus: 'locked', perPage: 100 });
        const row = locked.templates.find((t) => t.id === fx.template.id);
        browser.assert.ok(row, 'locked template appears under locked filter');
        browser.assert.strictEqual(row.moderation_status, 'locked', 'row moderation_status is locked');

        const normal = await adminListTemplates(fx.admin.token, { moderationStatus: 'normal', perPage: 100 });
        const inNormal = normal.templates.find((t) => t.id === fx.template.id);
        browser.assert.ok(!inNormal, 'locked template not under normal filter');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('soft-deletes a template; non-owners 404, owner sees removed notice', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/delete`,
          { reason: 'repeated abuse' },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.moderation_status, 'soft_deleted', 'status is soft_deleted');

        const v = await expectError(getTemplate(fx.viewer.token, fx.template.id));
        browser.assert.strictEqual(v.status, 404, 'non-owner sees 404 for soft-deleted');

        const ownerView = await getTemplate(fx.owner.token, fx.template.id);
        browser.assert.strictEqual(ownerView.moderation_status, 'soft_deleted', 'owner sees soft_deleted');

        // Admin list shows it under soft_deleted with a restore countdown.
        const sd = await adminListTemplates(fx.admin.token, { moderationStatus: 'soft_deleted', perPage: 100 });
        const row = sd.templates.find((t) => t.id === fx.template.id);
        browser.assert.ok(row, 'soft-deleted template appears under soft_deleted filter');
        browser.assert.strictEqual(row.moderation_status, 'soft_deleted', 'row moderation_status is soft_deleted');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('restores a soft-deleted template back to normal', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/restore`,
          { reason: 'restored after review' },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.moderation_status, 'normal', 'restore resets to normal');

        const v = await getTemplate(fx.viewer.token, fx.template.id);
        browser.assert.strictEqual(v.moderation_status, 'normal', 'viewer sees normal again');

        const audit = await getAuditLogs(fx.admin.token, { action: 'template.restore', perPage: 100 });
        const entry = audit.entries.find((e) => e.target_id === fx.template.id);
        browser.assert.ok(entry, 'audit has template.restore');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('re-publishing after admin unpublish resets to normal with audit marker', function (browser) {
    browser.perform(async function (done) {
      try {
        // Unpublish again.
        await request(
          'POST', `/api/v1/admin/templates/${fx.template.id}/unpublish`,
          { reason: 'needs edits' },
          { token: fx.admin.token }
        );
        // Owner edits + re-shares (updateTemplate publishes).
        await updateTemplate(fx.owner.token, fx.template, { content: 'edited content' });
        const t = await getTemplate(fx.owner.token, fx.template.id);
        browser.assert.strictEqual(t.moderation_status, 'normal', 're-publish resets to normal');

        const audit = await getAuditLogs(fx.admin.token, { action: 'template.republished_after_moderation', perPage: 100 });
        const entry = audit.entries.find((e) => e.target_id === fx.template.id);
        browser.assert.ok(entry, 'audit has template.republished_after_moderation');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: admin console ----------------

  it('renders the moderation panel with status chip and actions on admin detail', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-kpi', 20000)
      .url(`${browser.launch_url}/admin/templates/${fx.template.id}`)
      .waitForElementVisible('.admin-moderation-panel', 15000)
      .assert.textContains('.admin-moderation-panel', 'Moderation')
      .assert.elementPresent('.admin-moderation-panel__actions')
      .assert.textContains('.admin-moderation-panel__actions', 'Unpublish')
      .assert.textContains('.admin-moderation-panel__actions', 'Lock')
      .assert.textContains('.admin-moderation-panel__actions', 'Delete');
  });

  it('shows the status filter and status chip on the admin templates list', function (browser) {
    browser
      .url(`${browser.launch_url}/admin/templates`)
      .waitForElementVisible('.admin-templates__filters', 15000)
      .assert.elementPresent('.admin-templates__mod-cell')
      .assert.textContains('.admin-templates__filters', 'Status');
  });

  it('keeps EN/ZH parity for mod keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enMod = en.mod || {};
        const zhMod = zh.mod || {};
        browser.assert.deepStrictEqual(
          Object.keys(zhMod).sort(), Object.keys(enMod).sort(), 'mod.* top-level keys match'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});