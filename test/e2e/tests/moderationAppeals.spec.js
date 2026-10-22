// E2E: moderation appeals feature (49ef66e).
//
// REST contract:
//   - POST   /api/v1/appeals (auth; JSON {moderation_action_id, statement};
//     statement 50-2000 chars -> 400; duplicate action -> 409; missing
//     action -> 404; superseded action -> 409; APPEAL_MAX_OPEN=5 open
//     appeals -> 429 + Retry-After) returns the appeal with state "open"
//   - GET    /api/v1/appeals (auth; caller's appeals newest-first)
//   - GET    /api/v1/appeals/{id} (owner-only; non-owner 404)
//   - admin  GET /api/v1/admin/appeals?state=open (queue, oldest-first,
//     open_count); GET /api/v1/admin/appeals/{id} (detail);
//     POST /api/v1/admin/appeals/{id}/review (-> under_review);
//     POST /api/v1/admin/appeals/{id}/resolve ({outcome: reversed|upheld,
//     note}; note 20-1000 chars -> 400; self-guard: original action actor
//     -> 403; non-admin -> 403); reversed restores template
//     moderation_status -> normal + appeal_resolved notification
// UI: /appeals (list + state chips), /appeals/:id (detail), admin
//     /admin/appeals (queue tabs + counts), /admin/appeals/:id (detail +
//     review/resolve).
// i18n: appeals.* EN/ZH key parity.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  getTemplate,
  getNotifications,
} = require('../helpers/api');

const { execSync } = require('child_process');

const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'Admin_123456';

// Seed a moderation_actions row directly via psql (admin-template-moderation
// is not yet implemented). Returns the action id.
function seedAction(templateId, actorId, actionType = 'soft_delete', status = 'active') {
  const { randomUUID } = require('crypto');
  const actionId = randomUUID();
  const sql = `INSERT INTO moderation_actions (id, template_id, action_type, actor_id, actor_email, reason, status) VALUES ('${actionId}', '${templateId}', '${actionType}', '${actorId}', '${actorId}@example.com', 'spam', '${status}');`;
  execSync(
    `docker exec deployment-postgres-1 psql -U user -d open_prompts -c "${sql}"`,
    { stdio: 'pipe' }
  );
  return actionId;
}

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

describe('Moderation appeals', function () {
  const fx = {
    marker: `ap${Date.now().toString(36)}`,
    owner: {},    // template owner who appeals
    other: {},    // non-owner (404 on detail)
    admin: {},
    template: {}, // owner's public template
    actionId: '', // seeded moderation action
    appealId: '', // created appeal
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e moderation-appeals: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Appeal Owner' }));
      Object.assign(fx.other, await registerUser({ displayName: 'Appeal Other' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Appealable`,
        description: 'moderation appeals e2e fixture',
        category: 'Writing',
        tags: ['appeals'],
        content: 'appeal me',
      });
      Object.assign(fx.template, t);

      // Seed a moderation action against the owner's template. The actor is a
      // moderator (NOT the bootstrap admin) so the admin can resolve it.
      fx.actionId = seedAction(fx.template.id, 'moderator1');

      // Create an appeal as the owner.
      const statement = 'This decision is wrong because the links point to my company product documentation, not off-platform promotion, and the template follows the community guidelines.';
      const r = await request(
        'POST', '/api/v1/appeals',
        { moderation_action_id: fx.actionId, statement },
        { token: fx.owner.token }
      );
      fx.appealId = r.data.id;
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: create / list / detail ----------------

  it('creates an appeal and lists it for the owner', function (browser) {
    browser.perform(async function (done) {
      try {
        const list = await request('GET', '/api/v1/appeals', null, { token: fx.owner.token });
        const mine = list.data.appeals.find((a) => a.id === fx.appealId);
        browser.assert.ok(mine, 'owner sees the appeal in /appeals');
        browser.assert.strictEqual(mine.state, 'open', 'appeal state is open');
        browser.assert.strictEqual(mine.template_id, fx.template.id, 'template_id matches');
        browser.assert.strictEqual(mine.action_type, 'soft_delete', 'action_type is soft_delete');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a duplicate appeal for the same action with 409', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/appeals',
          { moderation_action_id: fx.actionId, statement: 'This is a duplicate appeal statement that is long enough to pass the fifty character minimum validation.' },
          { token: fx.owner.token }
        ));
        browser.assert.strictEqual(r.status, 409, 'duplicate appeal is 409');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a short statement with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        const action = seedAction(fx.template.id, 'moderator1');
        const r = await expectError(request(
          'POST', '/api/v1/appeals',
          { moderation_action_id: action, statement: 'short' },
          { token: fx.owner.token }
        ));
        browser.assert.strictEqual(r.status, 400, 'short statement is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a missing moderation action with 404', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/appeals',
          { moderation_action_id: '00000000-0000-0000-0000-000000000000', statement: 'This statement is long enough to pass the fifty character minimum validation for an appeal.' },
          { token: fx.owner.token }
        ));
        browser.assert.strictEqual(r.status, 404, 'missing action is 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns 404 for a non-owner on the appeal detail', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'GET', `/api/v1/appeals/${fx.appealId}`, null, { token: fx.other.token }
        ));
        browser.assert.strictEqual(r.status, 404, 'non-owner detail is 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('requires auth to create an appeal (401)', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/appeals',
          { moderation_action_id: fx.actionId, statement: 'This statement is long enough to pass the fifty character minimum validation for an appeal.' }
        ));
        browser.assert.strictEqual(r.status, 401, 'anonymous create is 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- Admin: queue / review / resolve ----------------

  it('shows the appeal in the admin open queue', function (browser) {
    browser.perform(async function (done) {
      try {
        const q = await request('GET', '/api/v1/admin/appeals?state=open', null, { token: fx.admin.token });
        browser.assert.ok(q.data.open_count >= 1, 'open_count >= 1');
        const entry = q.data.entries.find((e) => e.id === fx.appealId);
        browser.assert.ok(entry, 'appeal is in the admin open queue');
        browser.assert.strictEqual(entry.template_id, fx.template.id, 'queue template_id matches');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a non-admin from the admin queue with 403', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'GET', '/api/v1/admin/appeals?state=open', null, { token: fx.owner.token }
        ));
        browser.assert.strictEqual(r.status, 403, 'non-admin queue is 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('reviews the appeal to under_review', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/appeals/${fx.appealId}/review`, null, { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.state, 'under_review', 'review moves to under_review');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('resolves the appeal as reversed and restores the template', function (browser) {
    browser.perform(async function (done) {
      try {
        const note = 'Reviewed the links; they are legitimate product documentation. Reversed.';
        const r = await request(
          'POST', `/api/v1/admin/appeals/${fx.appealId}/resolve`,
          { outcome: 'reversed', note },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.state, 'resolved_reversed', 'resolve reversed sets state');

        // Template restored to normal.
        const detail = await getTemplate(fx.owner.token, fx.template.id);
        browser.assert.strictEqual(detail.moderation_status, 'normal', 'template restored to normal');

        // Owner gets an appeal_resolved notification.
        const notifs = await getNotifications(fx.owner.token);
        const types = notifs.notifications.map((n) => n.type);
        browser.assert.ok(types.includes('appeal_resolved'), 'owner has appeal_resolved notification');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a short resolution note with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        // Create a fresh appeal to resolve.
        const action = seedAction(fx.template.id, 'moderator1');
        const stmt = 'This decision is wrong because the template follows the community guidelines and the links are legitimate documentation.';
        const created = await request(
          'POST', '/api/v1/appeals',
          { moderation_action_id: action, statement: stmt },
          { token: fx.owner.token }
        );
        const r = await expectError(request(
          'POST', `/api/v1/admin/appeals/${created.data.id}/resolve`,
          { outcome: 'upheld', note: 'short' },
          { token: fx.admin.token }
        ));
        browser.assert.strictEqual(r.status, 400, 'short note is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: user appeals page + admin queue ----------------

  it('renders the user appeals list with the appeal card', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('form', 15000)
      .setValue('input[type="email"], input[name="email"]', fx.owner.email)
      .setValue('input[type="password"], input[name="password"]', fx.owner.password)
      .submitForm('form');
    browser.pause(1500);
    browser.url(`${browser.launch_url}/appeals`)
      .waitForElementVisible('.appeals-page', 20000)
      .assert.elementPresent('.appeal-card')
      .assert.textContains('.appeal-card', fx.template.title);
  });

  it('renders the appeal detail page with the statement', function (browser) {
    browser
      .url(`${browser.launch_url}/appeals/${fx.appealId}`)
      .waitForElementVisible('.appeal-detail', 15000)
      .assert.elementPresent('.appeal-detail__statement')
      .assert.textContains('.appeal-detail__statement', 'product documentation');
  });

  it('renders the admin appeals queue with the appeal row', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-kpi', 20000)
      .url(`${browser.launch_url}/admin/appeals`)
      .waitForElementVisible('.appeals-queue', 15000)
      .assert.elementPresent('.appeals-queue__tab')
      .assert.textContains('.appeals-queue', fx.template.title);
  });

  it('keeps EN/ZH parity for appeals keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enC = en.appeal || {};
        const zhC = zh.appeal || {};
        const enKeys = Object.keys(enC).sort();
        const zhKeys = Object.keys(zhC).sort();
        browser.assert.deepStrictEqual(zhKeys, enKeys, 'appeal.* top-level keys match');
        for (const k of enKeys) {
          const ev = enC[k];
          const zv = zhC[k];
          if (ev && typeof ev === 'object') {
            browser.assert.deepStrictEqual(
              Object.keys(zv || {}).sort(),
              Object.keys(ev).sort(),
              `appeal.${k} nested keys match`
            );
          }
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});