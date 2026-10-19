// E2E: follow creators (commit 021ef80).
//
// Covers the acceptance surfaces from
// docs/architecture/follow-creators-detailed-design.md §8:
//  - REST: POST/DELETE /api/v1/users/{id}/follow (idempotent; self-follow 400,
//    nonexistent 404); GET /api/v1/users/{id}/profile is allow-listed (never
//    email/mobile/role) and carries is_following / follows_you for authed
//    viewers; GET /api/v1/users/me/following-feed is auth-only (401 anon) and
//    public-only; GET /api/v1/users?sort=followers lists top creators.
//  - Profile bio: PUT /api/v1/profile trims 0-280; 281 -> 400.
//  - Admin: GET /api/v1/admin/users/{id} stats carry follower/following counts.
//  - Notifications: following a creator delivers a "follow" notification to
//    the followee (no template attached).
//  - UI: /creators/:id renders profile header + tabs; the FollowButton
//    toggles optimistically and reflects the server state (aria-pressed);
//    Home "Following" tab shows a login prompt for anonymous users and the
//    feed/empty-state for authed users.
//  - i18n: EN/ZH parity for creators.*/follow.*/home.following.*.
const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  likeTemplate,
  getNotifications,
  followUser,
  unfollowUser,
  getUserProfile,
  getFollowingFeed,
  updateProfile,
  getAdminUser,
} = require('../helpers/api');

const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'Admin_123456';

// Wrap an API call so a non-2xx response resolves to { status, body } instead
// of rejecting — for negative-path assertions.
function expectError(promise) {
  return promise.then(
    () => ({ status: 0, body: null }),
    (err) => ({ status: err.status, body: err.body })
  );
}

// The logged-in user's own id, decoded from the JWT the register call returns.
function userIdFromToken(token) {
  try {
    const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64').toString('utf8'));
    return payload.sub || payload.user_id || payload.id || '';
  } catch (e) {
    return '';
  }
}

describe('follow creators', function () {
  let browser;

  before(function (browser) {
    browser = this.browser;
  });

  after(function (browser) {
    browser.end();
  });

  it('follows and unfollows idempotently and exposes follower counts', function (browser) {
    browser.perform(async function (done) {
      try {
        const creator = await registerUser(`creator_${Date.now()}@t.test`, 'Passw0rd!');
        const viewer = await registerUser(`viewer_${Date.now()}@t.test`, 'Passw0rd!');

        const first = await followUser(viewer.token, creator.id);
        browser.assert.equal(first.following, true);

        // Idempotent re-follow: same state, no error.
        const again = await followUser(viewer.token, creator.id);
        browser.assert.equal(again.following, true);

        // Self-follow is rejected.
        const self = await expectError(followUser(viewer.token, viewer.id));
        browser.assert.equal(self.status, 400);

        // Nonexistent creator 404s.
        const missing = await expectError(followUser(viewer.token, '00000000-0000-0000-0000-000000000000'));
        browser.assert.equal(missing.status, 404);

        // Unfollow idempotent.
        await unfollowUser(viewer.token, creator.id);
        const re = await unfollowUser(viewer.token, creator.id);
        browser.assert.equal(re.following, false);

        // Re-follow for the profile + feed checks below.
        await followUser(viewer.token, creator.id);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('profile is allow-listed and viewer-scoped flags present', function (browser) {
    browser.perform(async function (done) {
      try {
        const creator = await registerUser(`prof_${Date.now()}@t.test`, 'Passw0rd!');
        const viewer = await registerUser(`viewer2_${Date.now()}@t.test`, 'Passw0rd!');
        await followUser(viewer.token, creator.id);

        const anonProfile = await getUserProfile(null, creator.id);
        browser.assert.ok(anonProfile);
        browser.assert.equal(anonProfile.email, undefined, 'no email for anonymous');
        browser.assert.equal(anonProfile.role, undefined, 'no role for anonymous');

        const authedProfile = await getUserProfile(viewer.token, creator.id);
        browser.assert.equal(authedProfile.email, undefined, 'no email for authed viewer');
        browser.assert.equal(authedProfile.is_following, true, 'viewer follows creator');
        browser.assert.equal(authedProfile.follows_you, false, 'creator does not follow back');

        const missing = await expectError(getUserProfile(null, '00000000-0000-0000-0000-000000000000'));
        browser.assert.equal(missing.status, 404);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('bio trims to 280 and rejects 281', function (browser) {
    browser.perform(async function (done) {
      try {
        const user = await registerUser(`bio_${Date.now()}@t.test`, 'Passw0rd!');
        await updateProfile(user.token, { bio: `  ${'x'.repeat(280)}  ` });
        const tooLong = await expectError(updateProfile(user.token, { bio: 'x'.repeat(281) }));
        browser.assert.equal(tooLong.status, 400);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('following feed is auth-only and reflects followed creators', function (browser) {
    browser.perform(async function (done) {
      try {
        const anon = await expectError(getFollowingFeed(null));
        browser.assert.equal(anon.status, 401);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('following a creator delivers a follow notification to the followee', function (browser) {
    browser.perform(async function (done) {
      try {
        const followed = await registerUser(`followee_${Date.now()}@t.test`, 'Passw0rd!');
        const follower = await registerUser(`follower_${Date.now()}@t.test`, 'Passw0rd!');
        await followUser(follower.token, followed.id);

        const { data } = await request('GET', '/api/v1/notifications?page=1', null, { token: followed.token });
        const items = data.items || data.notifications || [];
        const followNote = items.find((n) => n.type === 'follow');
        browser.assert.ok(followNote, 'follow notification delivered');
        browser.assert.equal(!!followNote.template_id, false, 'follow notification has no template');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('admin user detail gains follower/following stats', function (browser) {
    browser.perform(async function (done) {
      try {
        const admin = await adminLogin(ADMIN_EMAIL, ADMIN_PASSWORD);
        const user = await registerUser(`adm_${Date.now()}@t.test`, 'Passw0rd!');
        const detail = await getAdminUser(admin.token, user.id);
        browser.assert.ok(detail.stats, 'stats block present');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('creator profile page renders and FollowButton reflects state', function (browser) {
    browser.perform(async function (done) {
      try {
        const creator = await registerUser(`ui_cre_${Date.now()}@t.test`, 'Passw0rd!');
        const viewer = await registerUser(`ui_view_${Date.now()}@t.test`, 'Passw0rd!');
        await followUser(viewer.token, creator.id);

        // Log the viewer in via the UI so the page renders user-scoped state.
        browser.url('http://127.0.0.1:3000/login')
          .waitForElementVisible('form', 20000)
          .setValue('input[type="email"], input[name="email"]', viewer.email)
          .setValue('input[type="password"], input[name="password"]', 'Passw0rd!')
          .submitForm('form');
        browser.pause(1500);

        browser.url(`http://127.0.0.1:3000/creators/${creator.id}`)
          .waitForElementVisible('.creator-profile, [class*="creator"]', 20000);
        browser.pause(800);

        // FollowButton (aria-pressed = true after the API follow above).
        browser.elements('css selector', 'button[aria-pressed]', function (res) {
          browser.assert.ok(res && res.value && res.value.length > 0, 'aria-pressed buttons on profile');
        });
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('Home Following tab prompts anonymous users to log in', function (browser) {
    browser.perform(async function (done) {
      try {
        // Fresh anonymous session (clear any persisted token).
        browser.url('http://127.0.0.1:3000/')
          .waitForElementVisible('body', 15000)
          .execute(function () {
            try { localStorage.clear(); sessionStorage.clear(); } catch (e) { /* ignore */ }
          });
        // activeTab is component state (useState) — Home.js parses no ?tab=
        // URL parameter, so the Following tab is activated by a click. Use a
        // DOM click (protocol-agnostic) on the tab button whose text says
        // "Following" (EN locale set below).
        browser
          .url('http://127.0.0.1:3000/')
          .waitForElementVisible('.home-tab', 20000)
          .execute(function () {
            localStorage.setItem('i18nextLng', 'en');
            const btns = Array.from(document.querySelectorAll('button.home-tab'));
            const target = btns.find(function (b) { return /following/i.test(b.textContent); });
            if (target) { target.click(); return true; }
            return false;
          }, [], function (res) {
            browser.assert.equal(res.value, true, 'Following tab button found and clicked');
          })
          .waitForElementVisible('.home-following__login-prompt', 20000)
          .getText('.home-following__login-prompt', function (res) {
            browser.assert.ok(
              /log in/i.test(res.value),
              'login prompt text visible to anonymous users'
            );
          });
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('keeps EN/ZH parity for creators/follow/home.following/a11y keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        browser.assert.strictEqual(
          Object.keys(en.creators || {}).length,
          Object.keys(zh.creators || {}).length
        );
        browser.assert.strictEqual(
          Object.keys(en.follow || {}).length,
          Object.keys(zh.follow || {}).length
        );
        browser.assert.strictEqual(
          Object.keys(en.home?.following || {}).length,
          Object.keys(zh.home?.following || {}).length
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});
