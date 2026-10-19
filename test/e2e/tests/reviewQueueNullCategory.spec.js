// E2E: review-queue NULL-category regression (issue #3, fixed by 8b7ab9e
// `fix(review): coalesce null category in review queue queries`).
//
// Before the fix, a pending review submission whose template row had a NULL
// category crashed the whole admin review scan with
// `converting NULL to string is unsupported` on t.category (repository
// scanned the nullable column into a Go string). The fix wraps t.category in
// COALESCE(t.category, '') in `queueRowColumns` and the GetSubmissionDetail
// query, so NULL scans as "".
//
// Replays the original repro through REST only:
//  - gating ON (threshold 3) + brand-new creator → publish lands in review;
//  - template created with an empty category in the payload
//    → templates.category is NULL;
//  - GET /api/v1/admin/review        → 200, row carries category === "";
//  - GET /api/v1/admin/review/{id}   → 200, detail carries category === "";
//  - approving the submission clears the queue entry (self-cleanup).
const {
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  listReviewQueue,
  getReviewSubmission,
  approveReviewSubmission,
} = require('../helpers/api');

describe('Review queue NULL category (issue #3)', function () {
  const fx = {
    marker: `ncat${Date.now().toString(36)}`,
    admin: {},
    creator: {},
    template: {},
    submissionId: '',
  };

  before(function (browser, done) {
    (async () => {
      fx.admin = await adminLogin();
      // Gating ON so a brand-new creator's publish lands in the review queue.
      await setReviewThreshold(fx.admin.token, 3, 'e2e issue #3 spec: enable gating');
      fx.creator = await registerUser({ displayName: 'NullCat Creator' });
      // Template created WITHOUT a category (empty string in the payload)
      // → templates.category is NULL for this row.
      fx.template = await createTemplate(fx.creator.token, {
        title: `${fx.marker} No Category Template`,
        description: 'issue #3 NULL-category regression fixture',
        content: 'null category content',
        category: '',
        tags: [fx.marker],
        visibility: 'VISIBILITY_PUBLIC',
      });
      done();
    })().catch(done);
  });

  after(function (browser, done) {
    (async () => {
      // Restore gating OFF so the rest of the suite publishes instantly.
      try {
        await setReviewThreshold(fx.admin.token, 0, 'e2e issue #3 spec: disable gating');
      } catch (e) { /* best-effort */ }
      done();
    })().catch(done);
    browser.end();
  });

  // ---------- REST: queue scan with a NULL-category row ----------

  it('lists the NULL-category pending submission in the review queue with empty category', function (browser) {
    browser.perform(async (done) => {
      browser.assert.strictEqual(
        fx.template.moderation_status,
        'in_review',
        'gated creator publish lands in review'
      );

      // The whole queue must still scan (this is the crash the fix targets).
      const q = await listReviewQueue(fx.admin.token, { perPage: 100 });
      browser.assert.strictEqual(Array.isArray(q.items), true, 'queue listing returns items');

      const item = (q.items || []).find((it) => it.template_id === fx.template.id);
      browser.assert.strictEqual(!!item, true, 'NULL-category submission present in queue');
      browser.assert.strictEqual(item.category, '', 'queue row category coalesced to empty string');
      browser.assert.strictEqual(item.attempt_number, 1, 'first attempt');
      fx.submissionId = item.id;
      done();
    });
  });

  it('returns the submission detail with empty category instead of crashing', function (browser) {
    browser.perform(async (done) => {
      const detail = await getReviewSubmission(fx.admin.token, fx.submissionId);
      browser.assert.strictEqual(!!detail, true, 'submission detail is returned');
      browser.assert.strictEqual(detail.category, '', 'detail category coalesced to empty string');
      done();
    });
  });

  it('approves the NULL-category submission and clears the queue entry', function (browser) {
    browser.perform(async (done) => {
      const res = await approveReviewSubmission(fx.admin.token, fx.submissionId);
      browser.assert.strictEqual(res.status, 'approved', 'NULL-category submission approvable');

      const q = await listReviewQueue(fx.admin.token, { perPage: 100 });
      const stillThere = (q.items || []).some((it) => it.template_id === fx.template.id);
      browser.assert.strictEqual(stillThere, false, 'approved submission left the queue');
      done();
    });
  });
});
