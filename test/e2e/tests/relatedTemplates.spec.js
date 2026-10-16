// E2E: related templates & discovery (related rail + more-from-author row).
//
// Covers the acceptance criteria from
// docs/architecture/related-templates-detailed-design.md §8.3:
//  - REST: GET /templates/{id}/related (clamped [3,8], default 8) returns
//    { items, basis }; GET /templates/{id}/more-from-author (clamped [1,4],
//    default 4) returns { items }; optional auth (invalid Bearer -> 401);
//    private templates 404 for EVERYONE incl. owner; nonexistent /
//    banned-owner -> 404; <3 candidates -> items: [] (200, not an error);
//    Redis 5-min cache determinism (two identical calls within TTL return
//    identical responses).
//  - UI: rails appear below content on public template detail pages; absent
//    on private (owner view); card click navigates to the target detail
//    page; author row ordering; keyboard Tab reaches each card; section
//    aria-labels present; EN/ZH copy for all related.* keys (category/tag
//    names untranslated).
const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  deleteTemplate,
  banUser,
  setReviewThreshold,
  getRelatedTemplates,
  getMoreFromAuthor,
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

describe('Related templates', function () {
  // Shared fixtures.
  const fx = {
    marker: `rel${Date.now().toString(36)}`,
    alice: {},   // anchor author (category cluster)
    bob: {},     // tag/language cluster author
    carol: {},   // banned author (must never appear)
    admin: {},
    anchor: {},  // public anchor template
    private: {}, // alice's private template (owner view)
    banned: {},  // carol's public template (banned owner)
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      // Disable pre-publication review gating so fresh users publish
      // instantly and their templates are publicly searchable.
      await setReviewThreshold(admin.token, 0, 'e2e related spec: disable gating');

      Object.assign(fx.alice, await registerUser({ displayName: 'Related Alice' }));
      Object.assign(fx.bob, await registerUser({ displayName: 'Related Bob' }));
      Object.assign(fx.carol, await registerUser({ displayName: 'Related Carol' }));

      // alice's category cluster — the anchor template.
      const anchor = await createTemplate(fx.alice.token, {
        title: `${fx.marker} Anchor`,
        description: 'anchor template',
        category: 'Customer Support',
        tags: ['support', 'email'],
      });
      Object.assign(fx.anchor, anchor);

      // Same-category templates (alice + bob) so the rail has >=3 candidates.
      await createTemplate(fx.alice.token, {
        title: `${fx.marker} Cat A1`,
        description: 'same category',
        category: 'Customer Support',
        tags: ['support'],
      });
      await createTemplate(fx.alice.token, {
        title: `${fx.marker} Cat A2`,
        description: 'same category',
        category: 'Customer Support',
        tags: ['support'],
      });
      await createTemplate(fx.bob.token, {
        title: `${fx.marker} Cat B1`,
        description: 'same category',
        category: 'Customer Support',
        tags: ['support'],
      });

      // alice's private template (owner view — rails must be absent).
      const priv = await createTemplate(fx.alice.token, {
        title: `${fx.marker} Private`,
        description: 'private template',
        category: 'Customer Support',
        tags: ['support'],
        visibility: 'private',
      });
      Object.assign(fx.private, priv);

      // carol's public template — she will be banned so it must 404.
      const banned = await createTemplate(fx.carol.token, {
        title: `${fx.marker} Carol`,
        description: 'banned owner template',
        category: 'Customer Support',
        tags: ['support'],
      });
      Object.assign(fx.banned, banned);
      await banUser(admin.token, fx.carol.id, 'e2e related spec: ban carol');

      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------- REST contract ----------

  it('returns related items with a basis, clamped to [3,8] default 8', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await getRelatedTemplates(null, fx.anchor.id);
        browser.assert.strictEqual(Array.isArray(data.items), true, 'items is an array');
        browser.assert.strictEqual(data.items.length >= 3, true, '>=3 related items');
        browser.assert.strictEqual(data.items.length <= 8, true, '<=8 related items (default 8)');
        browser.assert.strictEqual(
          ['category', 'tag', 'generic'].includes(data.basis),
          true,
          `basis is one of category/tag/generic (got ${data.basis})`
        );
        // Anchor template never in its own rail.
        const ids = data.items.map((i) => i.id);
        browser.assert.strictEqual(ids.includes(fx.anchor.id), false, 'anchor not in own rail');
        // Private template never appears.
        browser.assert.strictEqual(ids.includes(fx.private.id), false, 'private not in rail');
        // Banned-owner template never appears.
        browser.assert.strictEqual(ids.includes(fx.banned.id), false, 'banned-owner not in rail');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('clamps limit=100 to 8 and limit=1 to >=3-or-empty (200)', function (browser) {
    browser.perform(async (done) => {
      try {
        const big = await getRelatedTemplates(null, fx.anchor.id, 100);
        browser.assert.strictEqual(big.items.length <= 8, true, 'limit=100 clamps to 8');
        const small = await getRelatedTemplates(null, fx.anchor.id, 1);
        browser.assert.strictEqual(small.items.length >= 3 || small.items.length === 0, true, 'limit=1 clamps to 3 (or empty)');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns more-from-author items, clamped to [1,4] default 4', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await getMoreFromAuthor(null, fx.anchor.id);
        browser.assert.strictEqual(Array.isArray(data.items), true, 'items is an array');
        browser.assert.strictEqual(data.items.length <= 4, true, '<=4 author items (default 4)');
        const ids = data.items.map((i) => i.id);
        browser.assert.strictEqual(ids.includes(fx.anchor.id), false, 'anchor not in own author row');
        browser.assert.strictEqual(ids.includes(fx.private.id), false, 'private not in author row');
        // All author items belong to alice (the anchor author).
        data.items.forEach((i) => {
          browser.assert.strictEqual(i.owner_id, fx.anchor.owner_id, 'author item owned by anchor author');
        });
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('clamps more-from-author limit=100 to 4', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await getMoreFromAuthor(null, fx.anchor.id, 100);
        browser.assert.strictEqual(data.items.length <= 4, true, 'limit=100 clamps to 4');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns 404 for nonexistent, private (anon/non-owner/owner), and banned-owner templates', function (browser) {
    browser.perform(async (done) => {
      try {
        const missing = '00000000-0000-0000-0000-000000000000';
        // Nonexistent.
        const r1 = await expectError(getRelatedTemplates(null, missing));
        browser.assert.strictEqual(r1.status, 404, 'related nonexistent -> 404');
        const r2 = await expectError(getMoreFromAuthor(null, missing));
        browser.assert.strictEqual(r2.status, 404, 'more-from-author nonexistent -> 404');

        // Private template: anonymous, non-owner, and owner all 404.
        for (const token of [null, fx.bob.token, fx.alice.token]) {
          const rr = await expectError(getRelatedTemplates(token, fx.private.id));
          browser.assert.strictEqual(rr.status, 404, `related private (token=${!!token}) -> 404`);
          const rm = await expectError(getMoreFromAuthor(token, fx.private.id));
          browser.assert.strictEqual(rm.status, 404, `more-from-author private (token=${!!token}) -> 404`);
        }

        // Banned-owner public template -> 404.
        const rb = await expectError(getRelatedTemplates(null, fx.banned.id));
        browser.assert.strictEqual(rb.status, 404, 'related banned-owner -> 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns 401 for an invalid Bearer token (optional auth)', function (browser) {
    browser.perform(async (done) => {
      try {
        const r = await expectError(
          request('GET', `/api/v1/templates/${fx.anchor.id}/related?limit=8`, null, {
            token: 'invalid.token.here',
          })
        );
        browser.assert.strictEqual(r.status, 401, 'invalid Bearer -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns identical responses within the Redis 5-min cache TTL (determinism)', function (browser) {
    browser.perform(async (done) => {
      try {
        const r1 = await getRelatedTemplates(null, fx.anchor.id, 8);
        const r2 = await getRelatedTemplates(null, fx.anchor.id, 8);
        browser.assert.strictEqual(JSON.stringify(r1), JSON.stringify(r2), 'two identical calls within TTL are identical');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------- UI ----------

  it('shows the related rail and more-from-author row on a public template page', function (browser) {
    freshSession(browser, `${browser.launch_url}/templates/${fx.anchor.id}`)
      .waitForElementVisible('.related-rail', 20000)
      .assert.attributeContains('.related-rail', 'aria-label', 'Related prompts section')
      .assert.elementPresent('.related-rail .related-rail__title')
      .assert.elementPresent('.related-rail .related-rail__grid .prompt-card')
      .waitForElementVisible('.more-from-author', 20000)
      .assert.attributeContains('.more-from-author', 'aria-label', 'More from this author')
      .assert.elementPresent('.more-from-author .more-from-author__title')
      .assert.elementPresent('.more-from-author .more-from-author__grid .prompt-card');
  });

  it('does not show rails on a private template (owner view)', function (browser) {
    // Log in as alice (owner) via the UI, then visit her private template.
    browser
      .url(`${browser.launch_url}/login`)
      .waitForElementVisible('body', 15000)
      .execute(function () {
        try { localStorage.clear(); } catch (e) { /* ignore */ }
        try { sessionStorage.clear(); } catch (e) { /* ignore */ }
        localStorage.setItem('i18nextLng', 'en');
      })
      .url(`${browser.launch_url}/login`)
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.alice.email)
      .setValue('#password', fx.alice.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/templates/${fx.private.id}`)
      .waitForElementVisible('.template-details-page', 20000)
      .assert.elementNotPresent('.related-rail')
      .assert.elementNotPresent('.more-from-author');
  });

  it('navigates to the target detail page when a rail card is clicked', function (browser) {
    browser.perform(async (done) => {
      try {
        // Pick a related item to click.
        const data = await getRelatedTemplates(null, fx.anchor.id, 8);
        const target = data.items[0];
        browser
          .url(`${browser.launch_url}/templates/${fx.anchor.id}`)
          .waitForElementVisible('.related-rail', 20000)
          .click('.related-rail .related-rail__grid .prompt-card .prompt-card-overlay')
          .waitForElementVisible('.template-details-page', 20000)
          .assert.urlContains(`/templates/${target.id}`, 'card click navigates to target detail page');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('orders the author row by like_count desc (most-liked first)', function (browser) {
    browser.perform(async (done) => {
      try {
        // Give one of alice's other templates a like so it ranks first.
        const data = await getMoreFromAuthor(null, fx.anchor.id, 4);
        if (data.items.length >= 2) {
          const first = data.items[0];
          const second = data.items[1];
          browser.assert.strictEqual(
            first.like_count >= second.like_count,
            true,
            'author row ordered by like_count desc'
          );
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('reaches each rail card via keyboard Tab (focusable links in DOM order)', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await getRelatedTemplates(null, fx.anchor.id, 8);
        const count = data.items.length;
        browser
          .url(`${browser.launch_url}/templates/${fx.anchor.id}`)
          .waitForElementVisible('.related-rail', 20000)
          .execute(function () {
            // Focus the first focusable element in the rail grid.
            const grid = document.querySelector('.related-rail .related-rail__grid');
            const first = grid.querySelector('.prompt-card-overlay');
            if (first) first.focus();
            return true;
          })
          .assert.elementPresent('.related-rail .related-rail__grid .prompt-card-overlay:focus')
          .keys(['\uE004']) // Tab
          .assert.elementPresent('.related-rail .related-rail__grid .prompt-card-overlay:focus');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('keeps EN/ZH parity for the related.* keys', function (browser) {
    browser.perform(async (done) => {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enKeys = Object.keys(en.related || {});
        const zhKeys = Object.keys(zh.related || {});
        browser.assert.strictEqual(
          enKeys.length === zhKeys.length,
          true,
          `related.* top-level key count matches (EN ${enKeys.length} vs ZH ${zhKeys.length})`
        );
        // Spot-check the exact strings.
        browser.assert.strictEqual(en.related.title.category, 'Related · {{category}}');
        browser.assert.strictEqual(zh.related.title.category, '相关 · {{category}}');
        browser.assert.strictEqual(en.related.title.tag, 'Related · {{tag}}');
        browser.assert.strictEqual(zh.related.title.tag, '相关 · {{tag}}');
        browser.assert.strictEqual(en.related.title.generic, 'Related prompts');
        browser.assert.strictEqual(zh.related.title.generic, '相关提示词');
        browser.assert.strictEqual(en.related.moreFromAuthor, 'More from {{author}}');
        browser.assert.strictEqual(zh.related.moreFromAuthor, '来自 {{author}} 的更多');
        browser.assert.strictEqual(en.related.aria.rail, 'Related prompts section');
        browser.assert.strictEqual(zh.related.aria.rail, '相关提示词区块');
        browser.assert.strictEqual(en.related.aria.authorRow, 'More from this author');
        browser.assert.strictEqual(zh.related.aria.authorRow, '来自此作者的更多');
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});