// E2E: notification preferences feature (f04e467).
//
// REST contract:
//   - GET  /api/v1/users/me/notification-preferences (auth; fresh user returns
//     all defaults: like/favorite/fork/comment/follow true, paused_all false;
//     anonymous -> 401)
//   - PATCH /api/v1/users/me/notification-preferences (partial update; only
//     provided fields written; non-boolean value -> 400; unknown field ignored
//     200; rate limit NOTIF_PREFS_RATE_PER_MINUTE=30 -> 429 + Retry-After)
//   - suppression: disabling likes suppresses like notifications (no row, no
//     badge, no history); pause-all suppresses all five types; unpause
//     restores
// UI: NotificationPreferencesPanel modal (gear icon on NotificationsPage +
//     bell dropdown footer link), toggles for each type + pause-all, Saved
//     indicator, paused note disabling type toggles.
// i18n: notifPrefs.* EN/ZH key parity.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  likeTemplate,
  unlikeTemplate,
  favoriteTemplate,
  createComment,
  followUser,
  getNotifications,
} = require('../helpers/api');

// Wrap an API call so a non-2xx response resolves to { status, body, headers }
// instead of rejecting — for negative-path assertions.
function expectError(promise) {
  return promise.then(
    () => ({ status: 0, body: null, headers: {} }),
    (err) => ({ status: err.status, body: err.body, headers: err.headers || {} })
  );
}

function loginViaUI(browser, email, password) {
  return browser
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('body', 15000)
    .execute(function () {
      try { localStorage.clear(); } catch (e) { /* ignore */ }
      localStorage.setItem('i18nextLng', 'en');
    })
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('#email', 15000)
    .setValue('#email', email)
    .setValue('#password', password)
    .submitForm('form');
}

describe('Notification preferences', function () {
  const fx = {
    marker: `np${Date.now().toString(36)}`,
    owner: {},    // preferences under test
    actor: {},    // generates engagement notifications for the owner
    ratelimit: {}, // dedicated user for the rate-limit test (keeps owner's window clean)
    admin: {},
    template: {}, // owner's public template
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e notification-preferences: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Prefs Owner' }));
      Object.assign(fx.actor, await registerUser({ displayName: 'Prefs Actor' }));
      Object.assign(fx.ratelimit, await registerUser({ displayName: 'Prefs RateLimit' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Prefs Template`,
        description: 'notification preferences e2e fixture',
        category: 'Writing',
        tags: ['prefs'],
        content: 'prefs content',
      });
      Object.assign(fx.template, t);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: defaults / auth ----------------

  it('returns all defaults for a fresh user', function (browser) {
    browser.perform(async function (done) {
      try {
        const resp = await request(
          'GET', '/api/v1/users/me/notification-preferences', null,
          { token: fx.owner.token }
        );
        browser.assert.strictEqual(resp.data.like_enabled, true, 'like_enabled default true');
        browser.assert.strictEqual(resp.data.favorite_enabled, true, 'favorite_enabled default true');
        browser.assert.strictEqual(resp.data.fork_enabled, true, 'fork_enabled default true');
        browser.assert.strictEqual(resp.data.comment_enabled, true, 'comment_enabled default true');
        browser.assert.strictEqual(resp.data.follow_enabled, true, 'follow_enabled default true');
        browser.assert.strictEqual(resp.data.paused_all, false, 'paused_all default false');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects anonymous access with 401 on GET and PATCH', function (browser) {
    browser.perform(async function (done) {
      try {
        const g = await expectError(request('GET', '/api/v1/users/me/notification-preferences'));
        browser.assert.strictEqual(g.status, 401, 'anonymous GET -> 401');
        const p = await expectError(request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { like_enabled: false }
        ));
        browser.assert.strictEqual(p.status, 401, 'anonymous PATCH -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: partial update ----------------

  it('PATCH subset updates only those fields', function (browser) {
    browser.perform(async function (done) {
      try {
        const resp = await request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { like_enabled: false }, { token: fx.owner.token }
        );
        browser.assert.strictEqual(resp.data.like_enabled, false, 'like_enabled updated to false');
        browser.assert.strictEqual(resp.data.favorite_enabled, true, 'favorite_enabled unchanged');
        browser.assert.strictEqual(resp.data.fork_enabled, true, 'fork_enabled unchanged');
        browser.assert.strictEqual(resp.data.comment_enabled, true, 'comment_enabled unchanged');
        browser.assert.strictEqual(resp.data.follow_enabled, true, 'follow_enabled unchanged');
        browser.assert.strictEqual(resp.data.paused_all, false, 'paused_all unchanged');

        // GET reflects the persisted state.
        const get = await request(
          'GET', '/api/v1/users/me/notification-preferences', null,
          { token: fx.owner.token }
        );
        browser.assert.strictEqual(get.data.like_enabled, false, 'GET reflects persisted like_enabled=false');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: suppression of like notifications ----------------

  it('disabling likes suppresses like notifications', function (browser) {
    browser.perform(async function (done) {
      try {
        // Baseline unread count (owner has like_enabled=false from prior test).
        const before = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        const baseline = before.data.count || 0;

        // Actor likes the owner's template.
        await likeTemplate(fx.actor.token, fx.template.id);

        const after = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        browser.assert.strictEqual(
          after.data.count, baseline,
          'unread count unchanged when likes disabled'
        );

        const list = await getNotifications(fx.owner.token, { filter: 'all' });
        const likeNotifs = (list.notifications || []).filter((n) => n.type === 'like');
        browser.assert.strictEqual(likeNotifs.length, 0, 'no like notification in history');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: pause-all ----------------

  it('pause-all suppresses all five types', function (browser) {
    browser.perform(async function (done) {
      try {
        const pause = await request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { paused_all: true }, { token: fx.owner.token }
        );
        browser.assert.strictEqual(pause.data.paused_all, true, 'paused_all set to true');

        const before = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        const baseline = before.data.count || 0;

        // favorite + comment + follow while paused -> no notifications.
        await favoriteTemplate(fx.actor.token, fx.template.id);
        await createComment(fx.actor.token, fx.template.id, 'paused comment');
        await followUser(fx.actor.token, fx.owner.id);

        const after = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        browser.assert.strictEqual(
          after.data.count, baseline,
          'unread count unchanged while paused'
        );

        // History remains browsable (endpoint still 200).
        const list = await getNotifications(fx.owner.token, { filter: 'all' });
        browser.assert.ok(Array.isArray(list.notifications), 'history remains browsable while paused');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('unpause restores like notifications', function (browser) {
    browser.perform(async function (done) {
      try {
        // Unpause + re-enable likes.
        await request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { paused_all: false }, { token: fx.owner.token }
        );
        await request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { like_enabled: true }, { token: fx.owner.token }
        );

        const before = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        const baseline = before.data.count || 0;

        // Unlike then re-like so the next like is a fresh ON toggle that emits.
        await unlikeTemplate(fx.actor.token, fx.template.id);
        await likeTemplate(fx.actor.token, fx.template.id);

        const after = await request(
          'GET', '/api/v1/notifications/unread-count', null, { token: fx.owner.token }
        );
        browser.assert.strictEqual(
          after.data.count, baseline + 1,
          'unread count increments by one after unpause + like'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: validation / rate limit ----------------

  it('rejects a non-boolean value with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        const resp = await expectError(request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { like_enabled: 'yes' }, { token: fx.owner.token }
        ));
        browser.assert.strictEqual(resp.status, 400, 'non-boolean PATCH -> 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('ignores an unknown field with 200 and unchanged prefs', function (browser) {
    browser.perform(async function (done) {
      try {
        const resp = await request(
          'PATCH', '/api/v1/users/me/notification-preferences',
          { bogus_field: true }, { token: fx.owner.token }
        );
        browser.assert.strictEqual(resp.status, 200, 'unknown field PATCH -> 200');
        browser.assert.strictEqual(resp.data.like_enabled, true, 'like_enabled unchanged');
        browser.assert.strictEqual(resp.data.paused_all, false, 'paused_all unchanged');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rate limits PATCHes with 429 + Retry-After', function (browser) {
    browser.perform(async function (done) {
      try {
        let got429 = false;
        let retryAfter = null;
        for (let i = 0; i < 35; i++) {
          try {
            await request(
              'PATCH', '/api/v1/users/me/notification-preferences',
              { like_enabled: true }, { token: fx.ratelimit.token }
            );
            // 200: within the window, keep going.
          } catch (err) {
            if (err.status === 429) {
              got429 = true;
              retryAfter = err.headers['retry-after'];
              break;
            }
            browser.assert.fail(`unexpected PATCH status ${err.status}`);
          }
        }
        browser.assert.strictEqual(got429, true, 'expected a 429 after exceeding the rate limit');
        browser.assert.ok(retryAfter, '429 response includes Retry-After header');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: panel via gear icon ----------------

  it('opens the preferences panel from the notifications page gear icon', function (browser) {
    loginViaUI(browser, fx.owner.email, fx.owner.password)
      // Wait for the post-login home page so the session token is persisted
      // before navigating to the notifications page.
      .waitForElementVisible('.notification-bell-btn', 20000)
      .url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notifications-page', 20000)
      .click('.notifications-prefs-gear')
      .waitForElementVisible('.notif-prefs-modal', 15000)
      .waitForElementVisible('.notif-prefs-body', 15000)
      .assert.visible('.notif-prefs-pause')
      .assert.visible('.notif-prefs-row')
      .assert.textContains('.notif-prefs-modal', 'Notification preferences')
      .assert.textContains('.notif-prefs-modal', 'Pause all notifications')
      .assert.textContains('.notif-prefs-modal', 'Likes')
      .assert.textContains('.notif-prefs-modal', 'Favorites')
      .assert.textContains('.notif-prefs-modal', 'Forks')
      .assert.textContains('.notif-prefs-modal', 'Comments')
      .assert.textContains('.notif-prefs-modal', 'Follows')
      .assert.textContains('.notif-prefs-modal', 'Changes save automatically.');
  });

  it('toggling off Likes auto-saves with a Saved indicator', function (browser) {
    browser
      .waitForElementVisible('.notif-prefs-row', 15000)
      // The Likes row is the first type row after the pause row.
      .useXpath()
      .click('(//div[contains(@class, "notif-prefs-row")][not(contains(@class, "notif-prefs-pause"))])[1]//button[@role="switch"]')
      .useCss()
      .waitForElementVisible('.notif-prefs-saved', 15000)
      .assert.textContains('.notif-prefs-saved', 'Saved');
  });

  it('pause-all disables type toggles and shows the paused note', function (browser) {
    browser
      .waitForElementVisible('.notif-prefs-pause', 15000)
      .click('.notif-prefs-pause button[role="switch"]')
      .waitForElementVisible('.notif-prefs-paused-note', 15000)
      .assert.textContains('.notif-prefs-paused-note', 'Paused all is on')
      .assert.attributeContains(
        '.notif-prefs-row:not(.notif-prefs-pause) button[role="switch"]',
        'aria-disabled', 'true'
      );
  });

  it('unpausing restores the type toggles', function (browser) {
    browser
      .waitForElementVisible('.notif-prefs-pause', 15000)
      .click('.notif-prefs-pause button[role="switch"]')
      .waitForElementNotPresent('.notif-prefs-paused-note', 15000)
      .assert.attributeContains(
        '.notif-prefs-row:not(.notif-prefs-pause) button[role="switch"]',
        'aria-disabled', 'false'
      );
  });

  it('opens the same panel from the bell dropdown footer link', function (browser) {
    browser
      .url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notifications-page', 20000)
      .click('.notification-bell-btn')
      .waitForElementVisible('.notification-dropdown', 10000)
      .click('.notification-dropdown-prefs')
      .waitForElementVisible('.notif-prefs-modal', 15000)
      .assert.visible('.notif-prefs-body')
      .assert.textContains('.notif-prefs-modal', 'Notification preferences');
  });

  // ---------------- i18n: ZH parity ----------------

  it('renders the panel in Chinese with notifPrefs.* parity', function (browser) {
    browser
      .execute(function () {
        localStorage.setItem('i18nextLng', 'zh');
      })
      .url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notifications-page', 20000)
      .click('.notifications-prefs-gear')
      .waitForElementVisible('.notif-prefs-modal', 15000)
      .waitForElementVisible('.notif-prefs-body', 15000)
      .assert.textContains('.notif-prefs-modal', '通知偏好')
      .assert.textContains('.notif-prefs-modal', '暂停所有通知')
      .assert.textContains('.notif-prefs-modal', '点赞')
      .assert.textContains('.notif-prefs-modal', '收藏')
      .assert.textContains('.notif-prefs-modal', '评论')
      .assert.textContains('.notif-prefs-modal', '关注')
      .assert.textContains('.notif-prefs-modal', '更改自动保存。');
  });
});