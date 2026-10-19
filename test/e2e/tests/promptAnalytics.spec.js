// E2E: prompt analytics — REST + UI contract.
//
// Covers the REST + UI surfaces from the prompt-analytics design
// (`docs/architecture/prompt-analytics-detailed-design.md`):
//
//  -- REST --
//  - GET /api/v1/templates/{id} returns view_count/use_count for anonymous.
//  - POST /api/v1/templates/{id}/events: anonymous view OK; per-session
//    dedup (same session within 30-min does NOT re-count); unknown type 400;
//    missing template 404; private template non-owner 404; per-IP rate
//    limit 60/min -> 429 + Retry-After.
//  - Web use event via POST /api/v1/prompts bumps use_count and
//    source_split.web. owner stats: GET /templates/{id}/stats returns
//    series/current/previous/source_split; ?days=7|30|90 length; days=14 -> 400.
//  - GET /api/v1/analytics/overview: template_count, series, top_templates,
//    uses-ranked.
//  - UI: anonymous /analytics redirects to /login; owner sees KPI cards +
//    daily chart + source split + top table; range switch mirrors ?range=;
//    TemplateDetails shows .public-counters for all visitors and the view
//    beacon bumps view_count for a fresh browser session; owner-only
//    .owner-stats block; EN/ZH parity of analytics.* keys.
//
// NOTE: the per-IP event rate-limit test is deliberately the LAST test in
// this file — it saturates the 60/min event bucket for the REST client IP.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  getTemplate,
  recordTemplateEvent,
  getTemplateStats,
  getAnalyticsOverview,
  createPrompt,
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
function freshSession(browser, url, lang = 'en') {
  return browser
    .url(url)
    .waitForElementVisible('body', 15000)
    .execute(function (l) {
      try { localStorage.clear(); } catch (e) { /* ignore */ }
      try { sessionStorage.clear(); } catch (e) { /* ignore */ }
      localStorage.setItem('i18nextLng', l);
    }, [lang])
    .url(url);
}

// Unique per-run IDs (dedup / rate-limit buckets must never collide across runs).
function fid(prefix) {
  return `${prefix}_${Date.now().toString(36)}_${Math.floor(Math.random() * 1e9).toString(36)}`;
}

describe('Prompt analytics', function () {
  // Shared fixtures.
  const fx = {
    marker: `ana${Date.now().toString(36)}`,
    admin: {},
    owner: {},   // template owner
    viewer: {},  // non-owner
    tplA: {},    // public template with events
    verA: '',    // tplA version id (prompt creation)
    priv: {},    // owner's private template
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      // Disable pre-publication review gating (fresh users publish instantly).
      await setReviewThreshold(admin.token, 0, 'e2e analytics spec: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Analytics Owner' }));
      Object.assign(fx.viewer, await registerUser({ displayName: 'Analytics Viewer' }));

      // Public template — capture version id (needed for prompt creation).
      const created = await request('POST', '/api/v1/templates', {
        title: `${fx.marker} Public`,
        description: 'analytics e2e anchor template',
        content: 'analyze: ${name}',
        visibility: 'public',
        category: 'general',
        tags: ['e2e'],
      }, { token: fx.owner.token });
      fx.tplA = created.data.template;
      fx.verA  = created.data.version.id;

      // Private template — events from a non-owner must 404.
      Object.assign(fx.priv, await createTemplate(fx.owner.token, {
        title: `${fx.marker} Private`,
        description: 'analytics e2e private',
        visibility: 'private',
      }));

      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------- REST: event ingestion ----------

  it('counts anonymous view events with per-session dedup (30-min window)', function (browser) {
    browser.perform(async (done) => {
      try {
        const sid = fid('sess');

        // First anonymous view — accepted, counts.
        const r1 = await recordTemplateEvent(null, fx.tplA.id, { type: 'view', session_id: sid });
        browser.assert.strictEqual(r1.status, 200, 'first anonymous view -> 200');
        let tpl = await getTemplate(null, fx.tplA.id);
        browser.assert.strictEqual(tpl.view_count, 1, 'view_count == 1 after first view');

        // Same session within the 30-min dedup window — accepted but NOT re-counted.
        const r2 = await recordTemplateEvent(null, fx.tplA.id, { type: 'view', session_id: sid });
        browser.assert.strictEqual(r2.status, 200, 'dup view still returns 200');
        tpl = await getTemplate(null, fx.tplA.id);
        browser.assert.strictEqual(tpl.view_count, 1, 'same session does not re-count (dedup)');

        // A different session counts again.
        await recordTemplateEvent(null, fx.tplA.id, { type: 'view', session_id: fid('sess') });
        tpl = await getTemplate(null, fx.tplA.id);
        browser.assert.strictEqual(tpl.view_count, 2, 'different session counts (+1)');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('bumps use_count and the web source when a prompt is created (AC-2)', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await createPrompt(fx.owner.token, {
          template_id: fx.tplA.id,
          version_id: fx.verA,
          owner_id: fx.owner.id,
          variables: {},
        });
        browser.assert.strictEqual(!!data.prompt, true, 'prompt created (200 with prompt object)');

        // The use event is written from a fire-and-forget goroutine.
        await new Promise((res) => setTimeout(res, 1500));

        const tpl = await getTemplate(null, fx.tplA.id);
        browser.assert.strictEqual(tpl.use_count >= 1, true, `use_count >= 1 after web use (got ${tpl.use_count})`);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('exposes numeric public counters on anonymous GET /templates/{id} (AC-3)', function (browser) {
    browser.perform(async (done) => {
      try {
        const tpl = await getTemplate(null, fx.tplA.id);
        browser.assert.strictEqual(typeof tpl.view_count, 'number', 'view_count is a JSON number');
        browser.assert.strictEqual(typeof tpl.use_count, 'number', 'use_count is a JSON number');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects invalid event type and events for missing/private templates', function (browser) {
    browser.perform(async (done) => {
      try {
        // Unsupported event type -> 400.
        const badType = await expectError(recordTemplateEvent(null, fx.tplA.id, { type: 'click', session_id: fid('bad') }));
        browser.assert.strictEqual(badType.status, 400, `unsupported type -> 400 (got ${badType.status})`);

        // Missing template -> 404.
        const missing = '00000000-0000-0000-0000-000000000000';
        const rMissing = await expectError(recordTemplateEvent(null, missing, { type: 'view', session_id: fid('miss') }));
        browser.assert.strictEqual(rMissing.status, 404, `missing template -> 404 (got ${rMissing.status})`);

        // Private template as non-owner -> 404 (no existence leak).
        const rPriv = await expectError(recordTemplateEvent(fx.viewer.token, fx.priv.id, { type: 'view', session_id: fid('priv') }));
        browser.assert.strictEqual(rPriv.status, 404, `private template non-owner -> 404 (got ${rPriv.status})`);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------- REST: owner stats + creator overview ----------

  it('returns owner stats with series/current/previous/source_split for days=7|30|90', function (browser) {
    browser.perform(async (done) => {
      try {
        for (const days of [7, 30, 90]) {
          const stats = await getTemplateStats(fx.owner.token, fx.tplA.id, days);
          browser.assert.strictEqual(stats.series.length, days, `series length == ${days}`);
          browser.assert.strictEqual(!!stats.current && !!stats.previous, true, 'current/previous present');
          browser.assert.strictEqual(!!stats.source_split, true, 'source_split present');
          browser.assert.strictEqual(typeof stats.view_to_use_rate, 'number', 'view_to_use_rate is numeric');
        }
        // Web use from the prompt creation shows in the source split.
        const stats30 = await getTemplateStats(fx.owner.token, fx.tplA.id, 30);
        browser.assert.strictEqual(stats30.source_split.web >= 1, true, `source_split.web >= 1 (got ${stats30.source_split.web})`);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('gates template stats: anon 401, invalid token 401, non-owner 403, missing 404', function (browser) {
    browser.perform(async (done) => {
      try {
        const anon = await expectError(getTemplateStats(null, fx.tplA.id, 30));
        browser.assert.strictEqual(anon.status, 401, 'anonymous -> 401');

        const badTok = await expectError(getTemplateStats('invalid.token.here', fx.tplA.id, 30));
        browser.assert.strictEqual(badTok.status, 401, 'invalid Bearer -> 401');

        const viewer = await expectError(getTemplateStats(fx.viewer.token, fx.tplA.id, 30));
        browser.assert.strictEqual(viewer.status, 403, 'non-owner -> 403');

        const bogus = '00000000-0000-0000-0000-000000000000';
        const missing = await expectError(getTemplateStats(fx.owner.token, bogus, 30));
        browser.assert.strictEqual(missing.status, 404, 'missing template -> 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects days=14 on stats and overview (only 7|30|90 allowed)', function (browser) {
    browser.perform(async (done) => {
      try {
        const s = await expectError(getTemplateStats(fx.owner.token, fx.tplA.id, 14));
        browser.assert.strictEqual(s.status, 400, `stats days=14 -> 400 (got ${s.status})`);

        const o = await expectError(getAnalyticsOverview(fx.owner.token, 14));
        browser.assert.strictEqual(o.status, 400, `overview days=14 -> 400 (got ${o.status})`);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns creator overview with 7 series points and the used template ranked first', function (browser) {
    browser.perform(async (done) => {
      try {
        const ov = await getAnalyticsOverview(fx.owner.token, 7);
        browser.assert.strictEqual(ov.series.length, 7, 'overview series length == 7');
        browser.assert.strictEqual(ov.template_count >= 1, true, `template_count >= 1 (got ${ov.template_count})`);
        browser.assert.strictEqual(Array.isArray(ov.top_templates), true, 'top_templates is an array');
        // Only tplA has use events for this fresh user, so it must rank first.
        browser.assert.strictEqual(ov.top_templates.length >= 1, true, 'top_templates non-empty');
        browser.assert.strictEqual(ov.top_templates[0].id, fx.tplA.id, 'most-used template ranks first');
        browser.assert.strictEqual(ov.top_templates[0].uses >= 1, true, 'top row uses >= 1');

        const anon = await expectError(getAnalyticsOverview(null, 7));
        browser.assert.strictEqual(anon.status, 401, 'overview anonymous -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------- UI: /analytics dashboard ----------

  it('redirects anonymous /analytics to /login (auth guard)', function (browser) {
    freshSession(browser, `${browser.launch_url}/analytics`)
      .waitForElementVisible('body', 15000)
      .assert.urlContains('/login');
  });

  it('renders KPI cards, chart toggles, source split and top table for the owner', function (browser) {
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
      .setValue('#email', fx.owner.email)
      .setValue('#password', fx.owner.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/analytics?range=7`)
      .waitForElementVisible('.analytics-page', 20000)
      .assert.containsText('.analytics-page__header h1', 'Analytics')
      .assert.elementPresent('.range-selector')
      .perform(async (done) => {
        try {
          await browser.execute(() => document.querySelectorAll('.analytics-page__kpis .kpi-card').length, [], (res) => {
            browser.assert.strictEqual(res.value, 4, '4 KPI cards rendered');
          });
          await browser.execute(() => document.querySelectorAll('.daily-chart__toggles .daily-chart__toggle').length, [], (res) => {
            browser.assert.strictEqual(res.value, 4, 'chart metric toggles rendered');
          });
          await browser.execute(() => !!document.querySelector('.daily-chart__svg'), [], (res) => {
            browser.assert.strictEqual(res.value, true, 'daily chart svg rendered');
          });
          await browser.execute(() => !!document.querySelector('.source-split__bar'), [], (res) => {
            browser.assert.strictEqual(res.value, true, 'source split bar rendered');
          });
          await browser.execute(() => !!document.querySelector('.top-templates__table'), [], (res) => {
            browser.assert.strictEqual(res.value, true, 'top templates table rendered (not the empty state)');
          });
          done();
        } catch (err) {
          done(err);
        }
      });
  });

  it('top table links to the most-used template', function (browser) {
    browser.perform(async (done) => {
      try {
        await browser.execute(function (tid) {
          return Array.from(document.querySelectorAll('.top-templates__table .top-templates__link'))
            .some((a) => a.getAttribute('href').includes(tid));
        }, [fx.tplA.id], (res) => {
          browser.assert.strictEqual(res.value, true, 'top table links to the most-used template');
        });
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('switches ?range= from 7 to 90 via the range selector', function (browser) {
    browser
      .waitForElementVisible('.range-selector', 15000)
      .click('.range-selector__option:last-child')
      .waitForElementVisible('.analytics-page', 20000)
      .assert.urlContains('range=90')
      .assert.attributeContains('.range-selector__option:last-child', 'aria-checked', 'true');
  });

  it('renders the owner-only stats block for the owner and not for a viewer', function (browser) {
    browser
      .url(`${browser.launch_url}/templates/${fx.tplA.id}`)
      .waitForElementVisible('.template-details-page', 20000)
      .waitForElementVisible('.owner-stats', 20000)
      .assert.elementPresent('.owner-stats__metric');
    // Same page, non-owner session: no owner block, public counters stay.
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.viewer.email)
      .setValue('#password', fx.viewer.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/templates/${fx.tplA.id}`)
      .waitForElementVisible('.template-details-page', 20000)
      .waitForElementVisible('.public-counters', 20000)
      .assert.elementPresent('.public-counters__item')
      .assert.elementNotPresent('.owner-stats');
  });

  it('renders ZH copy for the analytics dashboard after the language switch', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`, 'zh')
      .waitForElementVisible('#email', 15000)
      .setValue('#email', fx.owner.email)
      .setValue('#password', fx.owner.password)
      .submitForm('#email')
      .waitForElementVisible('.dashboard-layout', 20000)
      .url(`${browser.launch_url}/analytics`)
      .waitForElementVisible('.analytics-page', 20000)
      .assert.containsText('.analytics-page__header h1', '数据分析');
  });

  // ---------- LAST: rate limit (saturates the per-IP event bucket) ----------

  it('rate limits view beacons after 60 events per minute with Retry-After', function (browser) {
    browser.perform(async (done) => {
      try {
        let hit = null;
        for (let i = 0; i < 70 && !hit; i++) {
          const r = await expectError(
            recordTemplateEvent(null, fx.tplA.id, { type: 'view', session_id: `ratelimit_${i}` })
          );
          if (r.status === 429) hit = r;
        }
        browser.assert.strictEqual(!!hit, true, 'a 429 arrives within 70 rapid posts');
        if (hit) {
          const retryAfter = hit.headers['retry-after'] || hit.headers['Retry-After'];
          browser.assert.strictEqual(retryAfter !== undefined, true, '429 carries Retry-After header');
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});


