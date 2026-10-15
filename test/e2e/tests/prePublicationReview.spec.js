// E2E: pre-publication review (gating, admin review console, creator
// surfaces, notifications, i18n parity).
//
// Covers the acceptance criteria from
// docs/architecture/pre-publication-review-detailed-design.md §8:
//  - REST: queue listing (oldest-first, aging flag, attempt numbers),
//    metrics, approve/reject with reason validation, resubmission chain,
//    already-decided 409, unknown submission 404, settings GET/PUT,
//    audit log entries.
//  - 404-hiding: gated/rejected templates are invisible to non-owners
//    (GET, fork, prompts) but visible to the owner.
//  - Creator UI: Home review status cards (in_review + rejected),
//    TemplateDetails owner banners, PromptCard moderation badge,
//    PublishReviewModal after a gated publish, share button.
//  - Notifications: approved/rejected rows with reason.
//  - Admin console: /admin/review queue page (metrics, tabs, table),
//    /admin/review/:id detail (keyboard A/R/→, reject dialog).
//  - i18n: EN/ZH parity spot-check for review.* keys.
//
// NOTE: this spec turns gating ON (threshold 3) in before() and restores
// it to 0 (disabled) in after() so the rest of the suite (which assumes
// instant publishing) keeps passing.
const {
  registerUser,
  loginUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  getTemplate,
  getNotifications,
  setReviewThreshold,
  listReviewQueue,
  getReviewMetrics,
  getReviewSubmission,
  approveReviewSubmission,
  rejectReviewSubmission,
  request,
  getAuditLogs,
  banUser,
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
      localStorage.setItem('i18nextLng', 'en');
    })
    .url(`${browser.launch_url}/login`)
    .waitForElementVisible('#email', 15000)
    .setValue('#email', email)
    .setValue('#password', password)
    .submitForm('#email')
    .waitForElementVisible('.home-page', 20000);
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

// Find the queue item for a template id.
async function queueItemFor(adminToken, templateId) {
  const q = await listReviewQueue(adminToken, { perPage: 100 });
  return (q.items || []).find((it) => it.template_id === templateId) || null;
}

describe('Pre-publication review', function () {
  // Shared fixtures.
  const fx = {
    marker: `rvw${Date.now().toString(36)}`,
    newbie: {},       // gated creator (0 approved templates)
    established: {},  // ungated creator (3 approved templates)
    observer: {},     // second user: must get 404s
    admin: {},
    gated: {},        // newbie template stuck in review
    approved: {},     // newbie template that gets approved
    rejected: {},     // newbie template that gets rejected
    resubmitted: {},  // newbie template rejected then resubmitted (attempt 2)
    establishedTpl: {},
  };

  before(function (browser, done) {
    (async () => {
      // Turn gating ON (default threshold 3).
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 3, 'e2e review spec: enable gating');

      // Gated creator + observer.
      const newbie = await registerUser({ displayName: 'Review Newbie' });
      Object.assign(fx.newbie, newbie);
      const observer = await registerUser({ displayName: 'Review Observer' });
      Object.assign(fx.observer, observer);

      // Established creator: approved_template_count only increments via
      // admin review approval (IncrementApprovedCount is its sole writer),
      // so seed the counter by pushing 3 templates through review and
      // approving each. Gating stays ON the whole time.
      const established = await registerUser({ displayName: 'Review Established' });
      Object.assign(fx.established, established);
      for (let i = 0; i < 3; i += 1) {
        const tpl = await createTemplate(established.token, {
          title: `${fx.marker} Established ${i}`,
          description: 'established seed',
          category: 'general',
          tags: [fx.marker],
          visibility: 'VISIBILITY_PUBLIC',
          content: `established content ${i}`,
        });
        const item = await queueItemFor(admin.token, tpl.id);
        if (!item) throw new Error(`established seed ${i} not in review queue`);
        await approveReviewSubmission(admin.token, item.id);
      }

      // Newbie publishes → gated (in_review).
      const gated = await createTemplate(newbie.token, {
        title: `${fx.marker} Gated Submission`,
        description: 'waiting in review',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'gated content',
      });
      Object.assign(fx.gated, gated);

      // A second newbie submission that the spec will approve.
      const approved = await createTemplate(newbie.token, {
        title: `${fx.marker} To Approve`,
        description: 'will be approved',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'approve me',
      });
      Object.assign(fx.approved, approved);

      // A third newbie submission that the spec will reject, then the
      // newbie edits and re-publishes (attempt 2).
      const rejected = await createTemplate(newbie.token, {
        title: `${fx.marker} To Reject`,
        description: 'will be rejected',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'reject me',
      });
      Object.assign(fx.rejected, rejected);

      // A fourth newbie submission driving the resubmission chain, so
      // fx.rejected stays rejected for the creator-UI tests.
      const resubmitted = await createTemplate(newbie.token, {
        title: `${fx.marker} To Resubmit`,
        description: 'will be rejected then resubmitted',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'resubmit me',
      });
      Object.assign(fx.resubmitted, resubmitted);

      done();
    })().catch(done);
  });

  after(function (browser, done) {
    (async () => {
      // Restore gating OFF so the rest of the suite publishes instantly.
      try {
        await setReviewThreshold(fx.admin.token, 0, 'e2e review spec: disable gating');
      } catch (e) { /* best-effort */ }
      done();
    })().catch(done);
    browser.end();
  });

  // ---------- REST: gating behavior ----------

  it('gates newbie publishes but not established creators', function (browser) {
    browser.perform(async (done) => {
      // Newbie publish landed in review.
      browser.assert.strictEqual(
        fx.gated.moderation_status,
        'in_review',
        'newbie publish is gated'
      );
      browser.assert.strictEqual(
        fx.gated.published_at === null || fx.gated.published_at === undefined || fx.gated.published_at === '',
        true,
        'gated template has no published_at'
      );

      // Established creator (3 approved) publishes instantly.
      const tpl = await createTemplate(fx.established.token, {
        title: `${fx.marker} Established Live`,
        description: 'instant publish',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'instant content',
      });
      Object.assign(fx.establishedTpl, tpl);
      browser.assert.strictEqual(
        tpl.moderation_status,
        'normal',
        'established creator publishes instantly'
      );
      browser.assert.strictEqual(
        typeof tpl.published_at === 'string' && tpl.published_at.length > 0,
        true,
        'instant publish sets published_at'
      );
      done();
    });
  });

  it('hides in_review templates from non-owners (GET, fork, prompts)', function (browser) {
    browser.perform(async (done) => {
      // Anonymous GET → 404.
      try {
        await request('GET', `/api/v1/templates/${fx.gated.id}`);
        throw new Error('anonymous GET of in_review template should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'anonymous GET -> 404');
      }
      // Observer GET → 404.
      try {
        await getTemplate(fx.observer.token, fx.gated.id);
        throw new Error('observer GET of in_review template should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'observer GET -> 404');
      }
      // Observer fork → 404.
      try {
        await request('POST', `/api/v1/templates/${fx.gated.id}/fork`, null, { token: fx.observer.token });
        throw new Error('observer fork of in_review template should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'observer fork -> 404');
      }
      // Observer prompts GET → 404.
      try {
        await request('GET', `/api/v1/prompts/${fx.gated.id}`);
        throw new Error('observer prompts GET of in_review template should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'observer prompts GET -> 404');
      }
      // Owner GET → 200 with in_review.
      const own = await getTemplate(fx.newbie.token, fx.gated.id);
      browser.assert.strictEqual(own.moderation_status, 'in_review', 'owner still sees own template');
      done();
    });
  });

  // ---------- REST: admin review queue ----------

  it('lists the queue oldest-first with aging flag and attempt numbers', function (browser) {
    browser.perform(async (done) => {
      const q = await listReviewQueue(fx.admin.token, { perPage: 100 });
      const items = q.items || [];
      browser.assert.strictEqual(items.length > 0, true, 'queue is non-empty');

      // Our gated submission is present.
      const gatedItem = items.find((it) => it.template_id === fx.gated.id);
      browser.assert.strictEqual(!!gatedItem, true, 'gated submission in queue');
      browser.assert.strictEqual(gatedItem.attempt_number, 1, 'first attempt');
      browser.assert.strictEqual(gatedItem.is_resubmission, false, 'not a resubmission');
      browser.assert.strictEqual(gatedItem.aging, false, 'fresh submission not aging');

      // Ordering: submitted_at ascending (oldest first).
      for (let i = 1; i < items.length; i += 1) {
        const prev = new Date(items[i - 1].submitted_at).getTime();
        const cur = new Date(items[i].submitted_at).getTime();
        if (prev > cur) {
          throw new Error(`queue not oldest-first at index ${i}`);
        }
      }
      browser.assert.strictEqual(true, true, 'queue ordered oldest-first');

      // int64 total serializes as a string (protojson EmitUnpopulated).
      browser.assert.strictEqual(typeof q.total, 'string', 'total is an int64 string');
      done();
    });
  });

  it('validates reject reasons and rejects with spam hiding the template', function (browser) {
    browser.perform(async (done) => {
      const item = await queueItemFor(fx.admin.token, fx.rejected.id);
      browser.assert.strictEqual(!!item, true, 'reject target in queue');

      // Missing reason_code → 400.
      try {
        await request('POST', `/api/v1/admin/review/${item.id}/reject`, {}, { token: fx.admin.token });
        throw new Error('reject without reason_code should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'missing reason -> 400');
      }
      // Unknown reason → 400.
      try {
        await rejectReviewSubmission(fx.admin.token, item.id, 'not-a-reason');
        throw new Error('unknown reason should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'unknown reason -> 400');
      }
      // Non-admin → 403.
      try {
        await rejectReviewSubmission(fx.newbie.token, item.id, 'spam');
        throw new Error('non-admin reject should 403');
      } catch (e) {
        if (e.status !== 403) throw e;
        browser.assert.strictEqual(e.status, 403, 'non-admin reject -> 403');
      }

      // Valid reject with spam + note.
      const res = await rejectReviewSubmission(fx.admin.token, item.id, 'spam', 'bulk content');
      browser.assert.strictEqual(res.status, 'rejected', 'reject returns rejected');

      // Template now hidden: observer 404, owner sees rejected.
      try {
        await getTemplate(fx.observer.token, fx.rejected.id);
        throw new Error('observer should not see rejected template');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'rejected hidden from observer');
      }
      const own = await getTemplate(fx.newbie.token, fx.rejected.id);
      browser.assert.strictEqual(own.moderation_status, 'rejected', 'owner sees rejected status');

      // Creator notified with the reason code.
      const notes = await getNotifications(fx.newbie.token, { pageSize: 20 });
      const rejectedNote = (notes.notifications || []).find(
        (n) => n.type === 'template_rejected' && n.template_id === fx.rejected.id
      );
      browser.assert.strictEqual(!!rejectedNote, true, 'template_rejected notification exists');
      browser.assert.strictEqual(
        rejectedNote.actor_display_name,
        'spam',
        'notification carries the reason code'
      );

      // Already-decided submission → 409.
      try {
        await rejectReviewSubmission(fx.admin.token, item.id, 'spam');
        throw new Error('double decision should 409');
      } catch (e) {
        if (e.status !== 409) throw e;
        browser.assert.strictEqual(e.status, 409, 'already-decided -> 409');
      }
      done();
    });
  });

  it('approves a submission, publishes the template and notifies the creator', function (browser) {
    browser.perform(async (done) => {
      const item = await queueItemFor(fx.admin.token, fx.approved.id);
      browser.assert.strictEqual(!!item, true, 'approve target in queue');

      const res = await approveReviewSubmission(fx.admin.token, item.id);
      browser.assert.strictEqual(res.status, 'approved', 'approve returns approved');

      // Template is public now: anonymous GET works.
      const pub = await request('GET', `/api/v1/templates/${fx.approved.id}`);
      browser.assert.strictEqual(
        pub.data.template.moderation_status,
        'normal',
        'approved template is normal'
      );
      browser.assert.strictEqual(
        typeof pub.data.template.published_at === 'string' && pub.data.template.published_at.length > 0,
        true,
        'approved template has published_at'
      );

      // Creator notified.
      const notes = await getNotifications(fx.newbie.token, { pageSize: 20 });
      const approvedNote = (notes.notifications || []).find(
        (n) => n.type === 'template_approved' && n.template_id === fx.approved.id
      );
      browser.assert.strictEqual(!!approvedNote, true, 'template_approved notification exists');

      // Already-decided → 409.
      try {
        await approveReviewSubmission(fx.admin.token, item.id);
        throw new Error('double approve should 409');
      } catch (e) {
        if (e.status !== 409) throw e;
        browser.assert.strictEqual(e.status, 409, 'double approve -> 409');
      }
      done();
    });
  });

  it('supports the resubmission chain with attempt 2 and prior reason', function (browser) {
    browser.perform(async (done) => {
      // Reject the resubmission fixture first.
      const firstItem = await queueItemFor(fx.admin.token, fx.resubmitted.id);
      browser.assert.strictEqual(!!firstItem, true, 'resubmission target in queue');
      await rejectReviewSubmission(fx.admin.token, firstItem.id, 'lowQuality', 'not ready');

      // Newbie edits the rejected template and re-publishes.
      const updated = await updateTemplate(fx.newbie.token, fx.resubmitted, {
        content: 'much better content',
        changeNote: 'addressed quality concerns',
      });
      browser.assert.strictEqual(
        updated.template.moderation_status,
        'in_review',
        'resubmission goes back to review'
      );

      const item = await queueItemFor(fx.admin.token, fx.resubmitted.id);
      browser.assert.strictEqual(!!item, true, 'resubmission in queue');
      browser.assert.strictEqual(item.attempt_number, 2, 'attempt number 2');
      browser.assert.strictEqual(item.is_resubmission, true, 'flagged as resubmission');

      // Detail shows the chain with the prior rejection.
      const detail = await getReviewSubmission(fx.admin.token, item.id);
      browser.assert.strictEqual(Array.isArray(detail.chain), true, 'chain present');
      const first = (detail.chain || []).find((c) => c.attempt_number === 1);
      browser.assert.strictEqual(!!first, true, 'chain has attempt 1');
      browser.assert.strictEqual(first.status, 'rejected', 'attempt 1 rejected');
      browser.assert.strictEqual(first.reject_reason_code, 'lowQuality', 'prior reason code kept');
      done();
    });
  });

  it('reports metrics with open count, decisions today and rejection rates', function (browser) {
    browser.perform(async (done) => {
      const m = await getReviewMetrics(fx.admin.token);
      // int64 fields serialize as strings.
      browser.assert.strictEqual(typeof m.open_count, 'string', 'open_count int64 string');
      browser.assert.strictEqual(parseInt(m.open_count, 10) >= 1, true, 'at least one open item');
      browser.assert.strictEqual(
        parseInt(m.decisions_today, 10) >= 2,
        true,
        'approve + reject counted today'
      );
      const rates = m.rejection_rate_by_reason || [];
      const spam = rates.find((r) => r.reason_code === 'spam');
      browser.assert.strictEqual(!!spam, true, 'spam rejection rate present');
      done();
    });
  });

  it('exposes settings with validation and audit log entries', function (browser) {
    browser.perform(async (done) => {
      // GET lists the three review keys.
      const res = await request('GET', '/api/v1/admin/settings', null, { token: fx.admin.token });
      const keys = (res.data.settings || []).map((s) => s.key);
      browser.assert.strictEqual(keys.includes('review_approval_threshold'), true, 'threshold key');
      browser.assert.strictEqual(keys.includes('review_aging_hours'), true, 'aging key');
      browser.assert.strictEqual(keys.includes('review_regate_count'), true, 'regate key');

      // PUT validation: negative value → 400 (design: threshold >= 0,
      // 0 disables gating; other keys >= 1).
      try {
        await setReviewThreshold(fx.admin.token, -1, 'out of range');
        throw new Error('negative threshold should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'negative threshold -> 400');
      }
      // PUT validation: aging hours 0 → 400 (must be >= 1).
      try {
        await request('PUT', '/api/v1/admin/settings', {
          key: 'review_aging_hours',
          value: '0',
          reason: 'zero aging is invalid',
        }, { token: fx.admin.token });
        throw new Error('aging 0 should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'aging hours 0 -> 400');
      }
      // PUT validation: short reason → 400.
      try {
        await setReviewThreshold(fx.admin.token, 3, 'x');
        throw new Error('short reason should 400');
      } catch (e) {
        if (e.status !== 400) throw e;
        browser.assert.strictEqual(e.status, 400, 'short reason -> 400');
      }

      // Audit log has the review decisions.
      const approveLogs = await getAuditLogs(fx.admin.token, { action: 'review_approve', perPage: 10 });
      browser.assert.strictEqual(
        (approveLogs.entries || []).length > 0,
        true,
        'review_approve audit entries exist'
      );
      const rejectLogs = await getAuditLogs(fx.admin.token, { action: 'review_reject', perPage: 10 });
      browser.assert.strictEqual(
        (rejectLogs.entries || []).length > 0,
        true,
        'review_reject audit entries exist'
      );
      done();
    });
  });

  it('returns 404 for unknown submissions and re-gates banned creators', function (browser) {
    browser.perform(async (done) => {
      // Unknown submission id → 404.
      try {
        await getReviewSubmission(fx.admin.token, '00000000-0000-0000-0000-000000000000');
        throw new Error('unknown submission should 404');
      } catch (e) {
        if (e.status !== 404) throw e;
        browser.assert.strictEqual(e.status, 404, 'unknown submission -> 404');
      }

      // Ban the established creator → unban → next publish is gated again
      // (ban clears the approved-count fast path). Banning invalidates the
      // user's session token, so re-login after the unban.
      await banUser(fx.admin.token, fx.established.id, 'e2e review spec: regate probe');
      await request('POST', `/api/v1/admin/users/${fx.established.id}/unban`, { reason: 'e2e review spec: unban' }, { token: fx.admin.token });
      const reauth = await loginUser(fx.established.email, fx.established.password);
      fx.established.token = reauth.token;
      const tpl = await createTemplate(fx.established.token, {
        title: `${fx.marker} After Ban`,
        description: 'should be gated again',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
        content: 'regated content',
      });
      browser.assert.strictEqual(
        tpl.moderation_status,
        'in_review',
        'banned-then-unbanned creator is re-gated'
      );
      done();
    });
  });

  // ---------- Creator UI surfaces ----------

  it('shows review status cards on Home for in_review and rejected templates', function (browser) {
    loginViaUI(browser, fx.newbie.email, fx.newbie.password)
      .waitForElementVisible('.home-review-status', 20000)
      // At least one in-review card and one rejected card.
      .assert.textContains('.home-review-status', `${fx.marker} Gated Submission`)
      .assert.textContains('.home-review-status', `${fx.marker} To Reject`)
      // The rejected variant carries the rejected modifier class.
      .assert.visible('.review-status-card--rejected')
      // Status labels are translated. The card status label is rendered
      // with CSS text-transform: uppercase, so assert the uppercased text.
      .assert.textContains('.home-review-status', 'IN REVIEW')
      .assert.textContains('.home-review-status', 'REJECTED');
  });

  it('shows the owner banner and moderation badge on TemplateDetails and PromptCard', function (browser) {
    browser
      .url(`${browser.launch_url}/templates/${fx.gated.id}`)
      .waitForElementVisible('.review-owner-banner--in-review', 20000)
      .assert.textContains('.review-owner-banner--in-review', 'pre-publication review')
      // Rejected variant on the rejected template.
      .url(`${browser.launch_url}/templates/${fx.rejected.id}`)
      .waitForElementVisible('.review-owner-banner--rejected', 20000)
      .assert.textContains('.review-owner-banner--rejected', 'rejected');
  });

  it('opens the PublishReviewModal after a gated publish via the share button', function (browser) {
    browser.perform(async (done) => {
      // Create a fresh private template as the newbie, then share it via
      // the UI: the publish is gated and the modal appears.
      const tpl = await createTemplate(fx.newbie.token, {
        title: `${fx.marker} Modal Probe`,
        description: 'modal probe',
        category: 'general',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PRIVATE',
        content: 'modal content',
      });
      browser
        .url(`${browser.launch_url}/templates/${tpl.id}`)
        .waitForElementVisible('.action-btn.share', 20000)
        .click('.action-btn.share')
        .waitForElementVisible('.modal-title', 20000)
        .assert.textContains('.modal-title', 'Submitted for review')
        .assert.visible('.publish-review-modal__body')
        // Done is the secondary (ghost) button; the primary button is
        // "View status" which navigates away instead of closing.
        .click('.modal-footer .btn-ghost')
        .waitForElementNotPresent('.publish-review-modal', 10000);
      done();
    });
  });

  it('renders review decision notifications with the reason', function (browser) {
    loginViaUI(browser, fx.newbie.email, fx.newbie.password)
      .url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notification-item', 20000)
      // Approved row: actor-less sentence with the template title.
      .assert.textContains('.notifications-list, .notification-item', `${fx.marker} To Approve`)
      // Rejected row carries the reason code.
      .assert.textContains('.notifications-list, .notification-item', 'spam');
  });

  // ---------- Admin console UI ----------

  it('renders the review queue page with metrics, tabs and rows', function (browser) {
    adminLoginViaUI(browser)
      .url(`${browser.launch_url}/admin/review`)
      .waitForElementVisible('.review-queue__metrics', 20000)
      .assert.textContains('.review-queue__metrics', 'Open:')
      .assert.textContains('.review-queue__metrics', 'Today:')
      // Tabs use the tablist pattern.
      .assert.visible('.review-queue__tabs')
      .assert.attributeEquals('.review-queue__tab:first-child', 'role', 'tab')
      // The All tab is oldest-first with 20 rows/page; leftover pending
      // submissions from earlier runs can push this run's fixtures past
      // page 1. Switch to the Resubmissions tab (attempt > 1 only) where
      // this run's resubmitted fixture is deterministically listed.
      .waitForElementVisible('.admin-table tbody tr', 20000)
      .click('.review-queue__tab:nth-child(3)')
      .waitForElementVisible('.admin-table tbody tr', 20000)
      .assert.textContains('.admin-table', `${fx.marker} To Resubmit`)
      .assert.visible('.review-queue__attempt-chip');
  });

  it('shows the detail page with chain, keyboard approve and reject dialog', function (browser) {
    browser.perform(async (done) => {
      const item = await queueItemFor(fx.admin.token, fx.gated.id);
      browser.assert.strictEqual(!!item, true, 'gated item still queued');
      browser
        .url(`${browser.launch_url}/admin/review/${item.id}`)
        .waitForElementVisible('.review-detail', 20000)
        .assert.textContains('.review-detail', `${fx.marker} Gated Submission`)
        // Content preview + author summary.
        .assert.visible('.review-detail__content')
        .assert.visible('.review-detail__author')
        // Keyboard: R opens the reject dialog.
        .sendKeys('body', 'r')
        .waitForElementVisible('.review-reject-dialog', 10000)
        // Submit is disabled until a reason radio is chosen.
        .assert.attributeEquals('.modal-footer .btn-danger', 'disabled', 'true')
        .assert.visible('.review-reject-dialog__hint')
        // Choose spam + note.
        .click('input[name="reject-reason"][value="spam"]')
        .setValue('#reject-note', 'e2e keyboard reject')
        // React removes the disabled attribute entirely once enabled, so
        // assert enabled state rather than attributeEquals 'false'.
        .assert.enabled('.modal-footer .btn-danger')
        .click('.modal-footer .btn-danger')
        // After the decision the page auto-advances: to the next
        // submission's detail page when one exists, else the queue. With
        // leftover pending items from earlier runs a next item almost
        // always exists, so assert the URL left this submission and the
        // dialog closed, then verify the decision via REST.
        .waitForElementNotPresent('.review-reject-dialog', 10000)
        .pause(500)
        .url((result) => {
          browser.assert.strictEqual(
            result.value.includes(`/admin/review/${item.id}`),
            false,
            'reject advanced away from the submission'
          );
        });
      // REST: the gated template is now rejected with the spam reason.
      // The GET submission response exposes decisions via chain entries
      // (no top-level status field), and the browser's reject call may
      // still be in flight right after the navigation — poll briefly.
      let chain = null;
      for (let i = 0; i < 20; i += 1) {
        // eslint-disable-next-line no-await-in-loop
        const after = await getReviewSubmission(fx.admin.token, item.id);
        chain = (after && after.chain) || [];
        if (chain.some((c) => c.status === 'rejected')) break;
        // eslint-disable-next-line no-await-in-loop
        await new Promise((r) => setTimeout(r, 250));
      }
      const decided = chain.find((c) => c.attempt_number === item.attempt_number);
      browser.assert.strictEqual(!!decided && decided.status, 'rejected', 'submission decided rejected');
      browser.assert.strictEqual(decided && decided.reject_reason_code, 'spam', 'reject reason kept');
      done();
    });
  });

  it('approves via the keyboard A shortcut and auto-advances', function (browser) {
    browser.perform(async (done) => {
      // The resubmitted template (attempt 2) is still pending.
      const item = await queueItemFor(fx.admin.token, fx.resubmitted.id);
      browser.assert.strictEqual(!!item, true, 'attempt 2 still queued');
      browser
        .url(`${browser.launch_url}/admin/review/${item.id}`)
        .waitForElementVisible('.review-detail', 20000)
        // A approves and auto-advances (next detail page or the queue).
        .sendKeys('body', 'a')
        .pause(1000)
        .url((result) => {
          browser.assert.strictEqual(
            result.value.includes(`/admin/review/${item.id}`),
            false,
            'approve advanced away from the submission'
          );
        });
      // Verify the template is now public. Poll with the owner's token:
      // until the browser's approve call lands the template is still
      // in_review and hidden from anonymous callers.
      let tpl = null;
      for (let i = 0; i < 20; i += 1) {
        // eslint-disable-next-line no-await-in-loop
        const resp = await request('GET', `/api/v1/templates/${fx.resubmitted.id}`, null, { token: fx.newbie.token });
        tpl = resp.data.template;
        if (tpl.moderation_status === 'normal') break;
        // eslint-disable-next-line no-await-in-loop
        await new Promise((r) => setTimeout(r, 250));
      }
      browser.assert.strictEqual(
        tpl.moderation_status,
        'normal',
        'keyboard approve published the template'
      );
      done();
    });
  });

  it('shows the not-found page when a non-owner opens an in_review URL', function (browser) {
    loginViaUI(browser, fx.observer.email, fx.observer.password)
      .url(`${browser.launch_url}/templates/${fx.gated.id}`)
      .pause(1500)
      .assert.textContains('.not-found', 'Template not found');
  });

  // ---------- i18n parity ----------

  it('keeps EN/ZH parity for the review.* and notification keys', function (browser) {
    browser.perform(async (done) => {
      // The locale bundles are statically imported into the frontend
      // build (not served over HTTP), so load them straight from the repo.
      const en = require('../../../frontend/src/locales/en/translation.json');
      const zh = require('../../../frontend/src/locales/zh/translation.json');
      const enKeys = Object.keys(en.review || {});
      const zhKeys = Object.keys(zh.review || {});
      browser.assert.strictEqual(enKeys.length > 0, true, 'EN review.* keys exist');
      browser.assert.strictEqual(
        enKeys.length === zhKeys.length,
        true,
        'review.* key count matches across locales'
      );
      // Spot-check the nested keys used by the UI.
      const spots = [
        ['status', 'inReview'],
        ['status', 'rejected'],
        ['banner', 'in_review'],
        ['banner', 'rejected'],
        ['modal', 'title'],
        ['queue', 'title'],
        ['queue', 'open'],
        ['queue', 'aging'],
        ['queue', 'resubmissions'],
      ];
      spots.forEach(([section, key]) => {
        browser.assert.strictEqual(
          !!(en.review && en.review[section] && en.review[section][key]),
          true,
          `EN review.${section}.${key} present`
        );
        browser.assert.strictEqual(
          !!(zh.review && zh.review[section] && zh.review[section][key]),
          true,
          `ZH review.${section}.${key} present`
        );
      });
      // Notification event keys.
      browser.assert.strictEqual(
        !!(en.notifications && en.notifications.event && en.notifications.event.template_approved),
        true,
        'EN notifications.event.template_approved present'
      );
      browser.assert.strictEqual(
        !!(zh.notifications && zh.notifications.event && zh.notifications.event.template_approved),
        true,
        'ZH notifications.event.template_approved present'
      );
      browser.assert.strictEqual(
        !!(en.notifications && en.notifications.event && en.notifications.event.template_rejected),
        true,
        'EN notifications.event.template_rejected present'
      );
      browser.assert.strictEqual(
        !!(zh.notifications && zh.notifications.event && zh.notifications.event.template_rejected),
        true,
        'ZH notifications.event.template_rejected present'
      );
      // Admin nav.
      browser.assert.strictEqual(
        !!(en.admin && en.admin.nav && en.admin.nav.review),
        true,
        'EN admin.nav.review present'
      );
      browser.assert.strictEqual(
        !!(zh.admin && zh.admin.nav && zh.admin.nav.review),
        true,
        'ZH admin.nav.review present'
      );
      done();
    });
  });
});
