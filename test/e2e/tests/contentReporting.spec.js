// E2E: content reporting feature (5391f77 + fe24176).
//
// REST contract:
//   - POST   /api/v1/reports (auth; JSON {item_type, item_id, reason, note};
//     invalid item_type/reason -> 400; note > 500 -> 400; own content -> 400;
//     banned -> 403; anonymous -> 401; item not found/not public -> 404;
//     20/hour rate limit -> 429 + Retry-After) returns the report with
//     status "open"; re-report upserts (no duplicate)
//   - GET    /api/v1/reports/mine?item_type=&item_id= (caller's open report
//     or empty)
//   - DELETE /api/v1/reports/mine (withdraw, hard delete)
//   - admin  GET /api/v1/admin/reports?status=&item_type=&reason=&page=
//     (grouped queue, open-first by count then recency, 50/page)
//   - admin  GET /api/v1/admin/reports/{item_type}/{item_id} (detail with
//     every report + reporter display name)
//   - admin  POST /api/v1/admin/reports/{item_type}/{item_id}/resolve
//     ({resolution: dismissed|actioned, reason}; reason 3-500 -> 400;
//     no open reports -> 409; non-admin -> 403) -> audit
//     report.dismiss/report.actioned
//   - deleting a comment (admin or user path) auto-resolves its open reports
//     as actioned (FR-12)
//   - GET /api/v1/admin/stats -> open_reports KPI
// UI: ReportMenu/ReportModal on template ⋯ menu + comment actions,
//     /admin/reports ReportsPage (filters + queue table + Review drawer),
//     ReviewDrawer (item context + reports + Dismiss/Act with ReasonPopover),
//     OpenReports KPI card on dashboard.
// i18n: reports.* / admin.reports.* EN/ZH key parity.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  createComment,
  setReviewThreshold,
  getTemplate,
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

describe('Content reporting', function () {
  const fx = {
    marker: `rp${Date.now().toString(36)}`,
    owner: {},    // template owner (cannot report own content)
    reporter: {}, // reports the template + comment
    other: {},    // second reporter for aggregate
    admin: {},
    template: {}, // owner's public template
    comment: {},  // a comment on the template
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e content-reporting: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Report Owner' }));
      Object.assign(fx.reporter, await registerUser({ displayName: 'Report Reporter' }));
      Object.assign(fx.other, await registerUser({ displayName: 'Report Other' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Reportable`,
        description: 'content reporting e2e fixture',
        category: 'Writing',
        tags: ['reports'],
        content: 'report me',
      });
      Object.assign(fx.template, t);

      // A comment on the template (reporter reports it).
      const c = await createComment(fx.owner.token, fx.template.id, 'a comment to report');
      fx.comment = { id: (c.comment && c.comment.id) || c.id };
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: create / mine / withdraw ----------------

  it('creates a report and reflects it in /reports/mine', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'spam', note: 'repeated posting' },
          { token: fx.reporter.token }
        );
        browser.assert.strictEqual(r.data.status, 'open', 'report status is open');
        browser.assert.strictEqual(r.data.item_type, 'template', 'item_type is template');
        browser.assert.strictEqual(r.data.reason, 'spam', 'reason is spam');

        const mine = await request(
          'GET', `/api/v1/reports/mine?item_type=template&item_id=${fx.template.id}`,
          null, { token: fx.reporter.token }
        );
        browser.assert.strictEqual(mine.data.report.reason, 'spam', 'mine reflects the report');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('upserts a re-report with a different reason (no duplicate)', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'offensive', note: 'updated' },
          { token: fx.reporter.token }
        );
        browser.assert.strictEqual(r.data.reason, 'offensive', 're-report upserts reason');

        const mine = await request(
          'GET', `/api/v1/reports/mine?item_type=template&item_id=${fx.template.id}`,
          null, { token: fx.reporter.token }
        );
        browser.assert.strictEqual(mine.data.report.reason, 'offensive', 'mine reflects upsert');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects reporting own content with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'spam' },
          { token: fx.owner.token }
        ));
        browser.assert.strictEqual(r.status, 400, 'own content is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects an invalid reason with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'bogus' },
          { token: fx.reporter.token }
        ));
        browser.assert.strictEqual(r.status, 400, 'invalid reason is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects anonymous report with 401', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'spam' }
        ));
        browser.assert.strictEqual(r.status, 401, 'anonymous report is 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('withdraws a report via DELETE /reports/mine', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'DELETE', '/api/v1/reports/mine',
          { item_type: 'template', item_id: fx.template.id },
          { token: fx.reporter.token }
        );
        browser.assert.strictEqual(r.data.success, true, 'withdraw succeeds');

        const mine = await request(
          'GET', `/api/v1/reports/mine?item_type=template&item_id=${fx.template.id}`,
          null, { token: fx.reporter.token }
        );
        browser.assert.strictEqual(mine.data.report, null, 'mine is empty after withdraw');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- Admin: queue / detail / resolve ----------------

  it('shows the report in the admin open queue', function (browser) {
    browser.perform(async function (done) {
      try {
        // Re-create a report for the admin queue test.
        await request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'spam' },
          { token: fx.reporter.token }
        );
        const q = await request('GET', '/api/v1/admin/reports?status=open', null, { token: fx.admin.token });
        const group = q.data.groups.find((g) => g.item_id === fx.template.id);
        browser.assert.ok(group, 'report group is in the admin open queue');
        browser.assert.strictEqual(group.item_type, 'template', 'group item_type is template');
        browser.assert.ok(group.open_count >= 1, 'open_count >= 1');
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
          'GET', '/api/v1/admin/reports?status=open', null, { token: fx.reporter.token }
        ));
        browser.assert.strictEqual(r.status, 403, 'non-admin queue is 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('resolves the report group as dismissed and audits it', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/admin/reports/template/${fx.template.id}/resolve`,
          { resolution: 'dismissed', reason: 'Reviewed; content is legitimate.' },
          { token: fx.admin.token }
        );
        browser.assert.strictEqual(r.data.success, true, 'resolve succeeds');
        browser.assert.ok(r.data.resolved_count >= 1, 'resolved_count >= 1');

        // Queue no longer shows it as open.
        const q = await request('GET', '/api/v1/admin/reports?status=open', null, { token: fx.admin.token });
        const group = q.data.groups.find((g) => g.item_id === fx.template.id);
        browser.assert.ok(!group, 'resolved group no longer in open queue');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects resolving with no open reports (409)', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(request(
          'POST', `/api/v1/admin/reports/template/${fx.template.id}/resolve`,
          { resolution: 'dismissed', reason: 'No open reports remain to resolve.' },
          { token: fx.admin.token }
        ));
        browser.assert.strictEqual(r.status, 409, 'no open reports is 409');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('auto-resolves a comment report when the comment is deleted', function (browser) {
    browser.perform(async function (done) {
      try {
        // Report the comment.
        await request(
          'POST', '/api/v1/reports',
          { item_type: 'comment', item_id: fx.comment.id, reason: 'offensive' },
          { token: fx.reporter.token }
        );
        // Admin deletes the comment -> auto-resolve as actioned.
        const del = await request(
          'DELETE', `/api/v1/admin/comments/${fx.comment.id}`,
          { reason: 'e2e auto-resolve test' }, { token: fx.admin.token }
        );
        browser.assert.strictEqual(del.status, 200, 'admin comment delete succeeds');

        // The comment report is no longer open.
        const q = await request('GET', '/api/v1/admin/reports?status=open', null, { token: fx.admin.token });
        const group = q.data.groups.find((g) => g.item_id === fx.comment.id);
        browser.assert.ok(!group, 'comment report auto-resolved (not open)');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: admin reports page + KPI ----------------

  it('renders the admin reports queue with the report row', function (browser) {
    browser.perform(async function (done) {
      try {
        // Seed a fresh open report for the UI test.
        await request(
          'POST', '/api/v1/reports',
          { item_type: 'template', item_id: fx.template.id, reason: 'spam' },
          { token: fx.reporter.token }
        );
        done();
      } catch (err) {
        done(err);
      }
    });
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-kpi', 20000)
      .url(`${browser.launch_url}/admin/reports`)
      .waitForElementVisible('.reports-queue', 15000)
      .assert.elementPresent('.reports-queue__filters')
      .assert.textContains('.reports-queue', fx.template.title);
  });

  it('shows the OpenReports KPI on the admin dashboard', function (browser) {
    browser
      .url(`${browser.launch_url}/admin`)
      .waitForElementVisible('.admin-kpi', 20000)
      .assert.textContains('.admin-dashboard__grid', 'Open reports');
  });

  it('keeps EN/ZH parity for reports keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enC = en.reports || {};
        const zhC = zh.reports || {};
        browser.assert.deepStrictEqual(
          Object.keys(zhC).sort(), Object.keys(enC).sort(), 'reports.* top-level keys match'
        );
        const enA = en.admin && en.admin.reports || {};
        const zhA = zh.admin && zh.admin.reports || {};
        browser.assert.deepStrictEqual(
          Object.keys(zhA).sort(), Object.keys(enA).sort(), 'admin.reports.* keys match'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});