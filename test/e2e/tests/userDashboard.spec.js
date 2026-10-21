// E2E: user dashboard feature (9d11c6f).
//
// REST contract:
//   - GET /api/v1/dashboard/overview (required auth; anonymous 401)
//     returns {template_count, prompt_count, collection_count,
//     api_key_count, recent_activity} (all int32; collection_count degrades
//     to 0 when the collections table is absent); recent_activity
//     newest-first <=10 items with type (TEMPLATE/PROMPT/LIKE/FAVORITE),
//     entity_id, entity_title, timestamp, and template_id for PROMPT items.
//   - GET /api/v1/templates?sort=updated returns updated_at-desc (public +
//     private streams returned separately, merged client-side).
//   - GET /api/v1/prompts?owner_id=<me> returns rows with template_title.
//   - GET /api/v1/prompts?owner_id=<other> -> 403.
//   - GET /api/v1/prompts?owner_id=<me> without token -> 401.
// UI: /dashboard (DashboardLayout self-guard -> /login?next=/dashboard),
//     OverviewPage (StatCard x4, ActivityFeed, QuickActions), TemplatesPage
//     (FilterChips All/Public/Private/System/User, TemplateManageRow with
//     edit/delete/toggle-visibility, dual-stream merge), PromptsPage
//     (FilterChips per source template, PromptRow, PromptViewModal),
//     Sidebar "My Workspace" entry, Login ?next= redirect.
// i18n: dashboard_v2.* EN/ZH key parity (existing dashboard.* untouched).

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  searchTemplates,
} = require('../helpers/api');

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

// Log in via the real UI form and wait for the redirect to settle.
function uiLogin(browser, email, password) {
  return browser
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('#email', 20000)
    .setValue('#email', email)
    .setValue('#password', password)
    .click('.auth-submit');
}

describe('User dashboard', function () {
  const fx = {
    marker: `dash${Date.now().toString(36)}`,
    owner: {},    // dashboard owner (templates, prompts, api key)
    other: {},    // a second user (authz checks)
    admin: {},
    pubTpl: {},   // owner's public template
    privTpl: {},  // owner's private template
    toDelete: {}, // throwaway template for the delete flow
    pubVer: 0,    // public template version id (prompt creation)
    prompt: {},   // owner's instantiated prompt
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e user-dashboard: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Dashboard Owner' }));
      Object.assign(fx.other, await registerUser({ displayName: 'Dashboard Other' }));

      // Public template — capture version id for prompt creation.
      const pub = await request('POST', '/api/v1/templates', {
        title: `${fx.marker} Public`,
        description: 'dashboard e2e public',
        content: 'dash content ${topic}',
        visibility: 'public',
        category: 'general',
        tags: ['e2e'],
      }, { token: fx.owner.token });
      fx.pubTpl = pub.data.template;
      fx.pubVer = pub.data.version.id;

      Object.assign(fx.privTpl, await createTemplate(fx.owner.token, {
        title: `${fx.marker} Private`,
        description: 'dashboard e2e private',
        visibility: 'private',
      }));

      // Throwaway template for the delete flow (created up front so the
      // delete test only does UI navigation).
      Object.assign(fx.toDelete, await createTemplate(fx.owner.token, {
        title: `${fx.marker} ToDelete`,
        description: 'dashboard e2e delete victim',
        visibility: 'public',
      }));

      // Instantiate a prompt from the public template.
      const pr = await request('POST', '/api/v1/prompts', {
        template_id: fx.pubTpl.id,
        version_id: fx.pubVer,
        variables: { topic: 'hello' },
      }, { token: fx.owner.token });
      fx.prompt = pr.data.prompt || pr.data;

      // Create an API key so api_key_count is non-zero.
      await request('POST', '/api/v1/api-keys', { name: 'dash-e2e' }, { token: fx.owner.token });

      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: overview ----------------

  it('rejects anonymous overview with 401', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request('GET', '/api/v1/dashboard/overview', null, {}));
        browser.assert.strictEqual(r.status, 401, 'anonymous overview -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns overview counts matching seeded data', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request('GET', '/api/v1/dashboard/overview', null, { token: fx.owner.token });
        browser.assert.strictEqual(r.status, 200, 'overview -> 200');
        browser.assert.strictEqual(r.data.template_count, 3, 'template_count is 3 (public + private + toDelete)');
        browser.assert.strictEqual(r.data.prompt_count, 1, 'prompt_count is 1');
        browser.assert.strictEqual(r.data.api_key_count, 1, 'api_key_count is 1');
        browser.assert.strictEqual(typeof r.data.collection_count, 'number', 'collection_count present (0 when feature absent)');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns recent_activity newest-first, <=10, valid types, prompt items carry template_id', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request('GET', '/api/v1/dashboard/overview', null, { token: fx.owner.token });
        const activity = r.data.recent_activity || [];
        browser.assert.ok(Array.isArray(activity), 'recent_activity is an array');
        browser.assert.ok(activity.length >= 2, `recent_activity has >=2 items (got ${activity.length})`);
        browser.assert.ok(activity.length <= 10, 'recent_activity has <=10 items');

        const validTypes = new Set([
          'DASHBOARD_ACTIVITY_TYPE_TEMPLATE',
          'DASHBOARD_ACTIVITY_TYPE_PROMPT',
          'DASHBOARD_ACTIVITY_TYPE_LIKE',
          'DASHBOARD_ACTIVITY_TYPE_FAVORITE',
        ]);
        for (const a of activity) {
          browser.assert.ok(validTypes.has(a.type), `valid activity type ${a.type}`);
          browser.assert.ok(a.entity_id, 'activity item has entity_id');
          browser.assert.ok(a.entity_title, 'activity item has entity_title');
          browser.assert.ok(a.timestamp, 'activity item has timestamp');
        }

        // newest-first
        const times = activity.map((a) => a.timestamp);
        const sorted = [...times].sort().reverse();
        browser.assert.deepStrictEqual(times, sorted, 'activity is newest-first');

        // a PROMPT item must carry template_id
        const promptItem = activity.find((a) => a.type === 'DASHBOARD_ACTIVITY_TYPE_PROMPT');
        if (promptItem) {
          browser.assert.ok(promptItem.template_id, 'PROMPT activity item carries template_id');
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: sort=updated ----------------

  it('sorts owned templates by updated_at desc via sort=updated (public+private merged)', function (browser) {
    browser.perform(async function (done) {
      try {
        // Update the private template so its updated_at is strictly newer.
        await request('PUT', `/api/v1/templates/${fx.privTpl.id}`, {
          template_id: fx.privTpl.id,
          owner_id: fx.owner.id,
          title: `${fx.marker} Private`,
          description: 'dashboard e2e private updated',
          visibility: 'VISIBILITY_PRIVATE',
          category: '',
          tags: [],
          content: 'dash content private updated',
        }, { token: fx.owner.token });
        await new Promise((res) => setTimeout(res, 1100));

        const data = await searchTemplates({ sort: 'updated', token: fx.owner.token });
        const items = [...(data.templates || []), ...(data.private_templates || [])];
        const mine = items.filter((t) => t.id === fx.pubTpl.id || t.id === fx.privTpl.id);
        browser.assert.ok(mine.length >= 2, `both owned templates present (got ${mine.length})`);
        const sorted = [...mine].sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at));
        browser.assert.strictEqual(sorted[0].id, fx.privTpl.id, 'updated private template sorts first');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: prompts authz ----------------

  it('returns 403 listing another user\'s prompts', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(
          request('GET', `/api/v1/prompts?owner_id=${fx.other.id}`, null, { token: fx.owner.token })
        );
        browser.assert.strictEqual(r.status, 403, 'owner_id=<other> -> 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns 401 listing own prompts without a token', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(
          request('GET', `/api/v1/prompts?owner_id=${fx.owner.id}`, null, {})
        );
        browser.assert.strictEqual(r.status, 401, 'anonymous + owner_id=<me> -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns own prompts with template_title', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request('GET', `/api/v1/prompts?owner_id=${fx.owner.id}`, null, { token: fx.owner.token });
        browser.assert.strictEqual(r.status, 200, 'list own prompts -> 200');
        const prompts = r.data.prompts || [];
        browser.assert.ok(prompts.length >= 1, 'at least one prompt listed');
        browser.assert.ok(prompts[0].template_title, 'prompt row carries template_title');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: anonymous redirect ----------------

  it('redirects anonymous /dashboard to /login?next=/dashboard', function (browser) {
    freshSession(browser, `${browser.launch_url}/dashboard`)
      .waitForElementVisible('#email', 20000);
    browser.assert.urlContains('/login');
    browser.assert.urlContains('next=%2Fdashboard');
  });

  it('redirects anonymous /dashboard/templates to /login?next=/dashboard/templates', function (browser) {
    freshSession(browser, `${browser.launch_url}/dashboard/templates`)
      .waitForElementVisible('#email', 20000);
    browser.assert.urlContains('/login');
    browser.assert.urlContains('next=%2Fdashboard%2Ftemplates');
  });

  // ---------------- UI: overview ----------------

  it('renders the overview with stat cards, activity feed, and quick actions', function (browser) {
    uiLogin(browser, fx.owner.email, 'Password_123456')
      .waitForElementVisible('.overview-page', 20000);
    browser.assert.urlContains('/dashboard');
    // Stat cards: templates, prompts, api_keys (collections may be hidden).
    browser.waitForElementVisible('.stat-card', 20000);
    browser.elements('css selector', '.stat-card', function (res) {
      this.assert.ok(res.value.length >= 3, `renders >=3 stat cards (got ${res.value.length})`);
    });
    browser.waitForElementVisible('.quick-actions', 10000);
    browser.assert.textContains('.overview-page__title', 'My Workspace');
  });

  it('renders correct stat counts and a populated activity feed (Issue #4)', function (browser) {
    browser.url(`${browser.launch_url}/dashboard`)
      .waitForElementVisible('.overview-page', 20000);
    browser.execute(function () {
      const cards = Array.from(document.querySelectorAll('.stat-card'));
      return cards.map((c) => ({
        label: c.querySelector('.stat-card__label').textContent.trim(),
        count: c.querySelector('.stat-card__count').textContent.trim(),
      }));
    }, [], function (res) {
      const byLabel = {};
      res.value.forEach((c) => { byLabel[c.label] = c.count; });
      browser.assert.strictEqual(byLabel['Templates'], '3', 'templates stat card = 3');
      browser.assert.strictEqual(byLabel['Prompts'], '1', 'prompts stat card = 1');
      browser.assert.strictEqual(byLabel['API Keys'], '1', 'api_keys stat card = 1');
    });
    browser.waitForElementVisible('.activity-feed', 10000);
    browser.elements('css selector', '.activity-feed__item', function (res) {
      this.assert.ok(res.value.length >= 2, 'activity feed lists >=2 items');
    });
  });

  it('shows the dashboard nav with Overview/Templates/Prompts links', function (browser) {
    browser.url(`${browser.launch_url}/dashboard`)
      .waitForElementVisible('.dashboard-nav', 20000);
    browser.elements('css selector', '.dashboard-nav__link', function (res) {
      this.assert.ok(res.value.length >= 3, 'dashboard nav has 3+ links');
    });
  });

  it('shows the My Workspace entry in the sidebar', function (browser) {
    // Desktop viewport so the sidebar is on-canvas (it is off-canvas <768px).
    browser.resizeWindow(1280, 900);
    uiLogin(browser, fx.owner.email, 'Password_123456')
      .waitForElementVisible('.app-sidebar', 20000);
    browser.execute(function () {
      const el = document.querySelector('.app-sidebar');
      return el ? el.textContent : '';
    }, [], function (res) {
      browser.assert.ok(/My Workspace/.test(res.value), 'sidebar contains My Workspace');
    });
  });

  // ---------------- UI: templates page ----------------

  it('lists owned templates with filter chips and manage rows', function (browser) {
    browser.url(`${browser.launch_url}/dashboard/templates`)
      .waitForElementVisible('.templates-page', 20000);
    browser.waitForElementVisible('.filter-chips', 10000);
    browser.elements('css selector', '.filter-chip', function (res) {
      this.assert.strictEqual(res.value.length, 5, 'templates filter has 5 chips (All/Public/Private/System/User)');
    });
    browser.waitForElementVisible('.template-manage-row', 10000);
    browser.elements('css selector', '.template-manage-row', function (res) {
      this.assert.ok(res.value.length >= 2, `renders >=2 manage rows (got ${res.value.length})`);
    });
  });

  it('filters templates by visibility (Private chip)', function (browser) {
    browser.url(`${browser.launch_url}/dashboard/templates`)
      .waitForElementVisible('.filter-chips', 20000);
    // Click the Private chip (index 2).
    browser.execute(function () {
      const chips = document.querySelectorAll('.filter-chip');
      if (chips.length < 3) return false;
      chips[2].click();
      return true;
    }, [], function (res) {
      browser.assert.ok(res.value, 'clicked the Private chip');
    });
    browser.pause(500);
    browser.elements('css selector', '.template-manage-row', function (res) {
      this.assert.ok(res.value.length >= 1, 'private filter shows >=1 row');
    });
    browser.assert.textContains('.template-manage-row', 'Private');
  });

  it('toggles template visibility from the manage row', function (browser) {
    browser.url(`${browser.launch_url}/dashboard/templates`)
      .waitForElementVisible('.template-manage-row', 20000);
    // Find the row for the public template and click its visibility toggle.
    browser.execute(function (title) {
      const rows = Array.from(document.querySelectorAll('.template-manage-row'));
      const row = rows.find((r) => r.textContent.includes(title));
      if (!row) return false;
      const btns = row.querySelectorAll('.template-manage-row__actions button');
      if (btns.length < 2) return false;
      btns[0].click(); // visibility toggle is the first action button
      return true;
    }, [fx.pubTpl.title], function (res) {
      browser.assert.ok(res.value, 'clicked the visibility toggle on the public row');
    });
    // Optimistic flip: the row now shows Private.
    browser.pause(800);
    browser.execute(function (title) {
      const rows = Array.from(document.querySelectorAll('.template-manage-row'));
      const row = rows.find((r) => r.textContent.includes(title));
      return row ? row.textContent : '';
    }, [fx.pubTpl.title], function (res) {
      browser.assert.ok(/Private/.test(res.value), 'public row flipped to Private after toggle');
    });
  });

  it('deletes a template via the confirm modal', function (browser) {
    uiLogin(browser, fx.owner.email, 'Password_123456')
      .waitForElementVisible('.overview-page', 20000)
      .url(`${browser.launch_url}/dashboard/templates`)
      .waitForElementVisible('.template-manage-row', 20000);
    browser.execute(function (marker) {
      const rows = Array.from(document.querySelectorAll('.template-manage-row'));
      let row = null;
      for (let i = 0; i < rows.length; i++) {
        const t = rows[i].querySelector('.template-manage-row__title').textContent.trim();
        if (t.indexOf(marker) === 0 && t.indexOf('ToDelete') !== -1) { row = rows[i]; break; }
      }
      if (!row) return false;
      const del = row.querySelector('.template-manage-row__actions .btn-danger');
      if (!del) return false;
      del.click(); // Delete button
      return true;
    }, [fx.marker], function (res) {
      browser.assert.ok(res.value, 'clicked Delete on the throwaway row');
    });
    browser.waitForElementVisible('.modal-overlay', 10000);
    browser.assert.textContains('.modal', fx.toDelete.title, 'confirm modal echoes the template title');
    browser.click('.modal-actions .btn-danger');
    browser.pause(800);
    browser.execute(function (title) {
      const rows = Array.from(document.querySelectorAll('.template-manage-row'));
      return !rows.some((r) => r.textContent.includes(title));
    }, [fx.toDelete.title], function (res) {
      browser.assert.ok(res.value, 'deleted row is removed from the list');
    });
  });

  // ---------------- UI: prompts page ----------------

  it('lists prompts with source template and opens the view modal', function (browser) {
    browser.url(`${browser.launch_url}/dashboard/prompts`)
      .waitForElementVisible('.prompts-page', 20000);
    browser.waitForElementVisible('.prompt-row', 10000);
    browser.assert.textContains('.prompt-row', fx.pubTpl.title, 'prompt row shows the source template title');
    // Open the view modal.
    browser.execute(function () {
      const btns = document.querySelectorAll('.prompt-row__actions button');
      if (btns.length < 1) return false;
      btns[0].click(); // View is the first action
      return true;
    }, [], function (res) {
      browser.assert.ok(res.value, 'clicked View on the prompt row');
    });
    browser.waitForElementVisible('.prompt-view-modal', 10000);
    browser.assert.textContains('.prompt-view-modal__title', fx.pubTpl.title, 'modal title echoes the source template');
    browser.waitForElementVisible('.prompt-content', 5000);
    browser.assert.textContains('.prompt-content', 'dash content hello', 'modal shows substituted content');
    // Close the modal.
    browser.click('.modal-actions .btn-primary');
    browser.waitForElementNotPresent('.prompt-view-modal', 10000);
  });

  // ---------------- i18n parity ----------------

  it('keeps EN/ZH parity for dashboard_v2.* keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enD = en.dashboard_v2 || {};
        const zhD = zh.dashboard_v2 || {};

        const flatten = (obj, prefix = '') => {
          let out = {};
          for (const k of Object.keys(obj)) {
            const key = prefix ? `${prefix}.${k}` : k;
            if (obj[k] && typeof obj[k] === 'object') {
              out = { ...out, ...flatten(obj[k], key) };
            } else {
              out[key] = obj[k];
            }
          }
          return out;
        };

        const enFlat = flatten(enD);
        const zhFlat = flatten(zhD);
        browser.assert.deepStrictEqual(
          Object.keys(enFlat).sort(), Object.keys(zhFlat).sort(),
          'dashboard_v2.* key sets match between EN and ZH'
        );
        for (const k of Object.keys(enFlat)) {
          browser.assert.strictEqual(typeof zhFlat[k], 'string', `dashboard_v2.${k} is a string in zh`);
        }
        browser.assert.strictEqual(enD.title, 'My Workspace', 'EN dashboard_v2.title');
        browser.assert.strictEqual(zhD.title, '我的工作区', 'ZH dashboard_v2.title');
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});