// E2E: template comments feature (API-level coverage).
// Covers: create + list (newest first, is_own/is_owner flags, comment_count),
// anonymous public read, content validation, edit window (is_edited, position
// kept), author-or-owner delete, pagination, banned user 403, private
// template 404 indistinguishability, admin moderation (list filters, reason
// validation, delete + audit log), owner notification, and per-user rate
// limiting (429 + Retry-After).
//
// NOTE: the CommentsSection UI components exist in frontend/src/components/
// comments/ but are not yet wired into TemplateDetails (no import, no i18n
// keys, no admin page). UI interaction tests will be added once the frontend
// integration lands; this spec pins the backend contract in the meantime.
const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  createComment,
  listComments,
  updateComment,
  deleteComment,
  getTemplate,
  deleteTemplate,
  adminListComments,
  adminDeleteComment,
  getAuditLogs,
  banUser,
  getNotifications,
  setReviewThreshold,
} = require('../helpers/api');

// Wrap an API call so a non-2xx response resolves to { status, body, headers }
// instead of rejecting — for negative-path assertions.
function expectError(promise) {
  return promise.then(
    () => ({ status: 0, body: null, headers: {} }),
    (err) => ({ status: err.status, body: err.body, headers: err.headers || {} })
  );
}

describe('Template comments', function () {
  const owner = {};
  const author = {};
  const stranger = {};
  const template = {};
  let adminToken = '';

  before(function (browser, done) {
    (async () => {
      // Pre-publication review gating (a46c24c) would put every fresh
      // user's first public templates in_review and hide them from other
      // users. This spec tests comments, not gating — disable it for the
      // run (threshold 0), like the FVT legacy suites do.
      const admin = await adminLogin();
      adminToken = admin.token;
      await setReviewThreshold(adminToken, 0, 'e2e template-comments: disable gating');

      Object.assign(owner, await registerUser({ displayName: 'Comments Owner' }));
      Object.assign(author, await registerUser({ displayName: 'Comments Author' }));
      Object.assign(stranger, await registerUser({ displayName: 'Comments Stranger' }));
      const t = await createTemplate(owner.token, {
        title: 'Comments E2E Template',
        content: 'Summarize {{text}} in three bullets.',
      });
      Object.assign(template, t);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  it('creates a comment and lists it newest-first with flags and count', function (browser) {
    browser.perform(async (done) => {
      try {
        const first = await createComment(author.token, template.id, 'First comment');
        browser.assert.strictEqual(first.id && first.id.length > 0, true, 'comment id returned');
        browser.assert.strictEqual(first.content, 'First comment');
        browser.assert.strictEqual(first.template_id, template.id);
        browser.assert.strictEqual(first.is_own, true, 'author sees is_own');
        browser.assert.strictEqual(first.is_owner, false, 'author is not template owner');
        browser.assert.strictEqual(first.is_edited, false, 'fresh comment not edited');
        browser.assert.strictEqual(first.author.id, author.id);
        browser.assert.strictEqual(first.author.display_name, 'Comments Author');

        const second = await createComment(owner.token, template.id, 'Owner reply');
        browser.assert.strictEqual(second.is_owner, true, 'owner comment flagged is_owner');
        browser.assert.strictEqual(second.is_own, true, 'owner is own author');

        const list = await listComments(author.token, template.id);
        browser.assert.strictEqual(list.total_count, 2);
        browser.assert.strictEqual(list.comments.length, 2);
        browser.assert.strictEqual(list.comments[0].content, 'Owner reply', 'newest first');
        browser.assert.strictEqual(list.comments[1].content, 'First comment');
        browser.assert.strictEqual(list.comments[0].is_own, false, 'viewer is not author of newest');

        const tpl = await getTemplate(author.token, template.id);
        browser.assert.strictEqual(tpl.comment_count, 2, 'comment_count bumped');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('allows anonymous read of comments on a public template', function (browser) {
    browser.perform(async (done) => {
      try {
        const list = await listComments(null, template.id);
        browser.assert.strictEqual(list.total_count, 2, 'anonymous list works');
        browser.assert.strictEqual(list.comments[0].is_own, false, 'anonymous viewer has no is_own');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects invalid comment content with 400', function (browser) {
    browser.perform(async (done) => {
      try {
        const blank = await expectError(createComment(author.token, template.id, '   '));
        browser.assert.strictEqual(blank.status, 400, 'whitespace-only rejected');

        const tooLong = await expectError(createComment(author.token, template.id, 'x'.repeat(2001)));
        browser.assert.strictEqual(tooLong.status, 400, 'over-2000 chars rejected');

        const anonymous = await expectError(createComment(null, template.id, 'anon comment'));
        browser.assert.strictEqual(anonymous.status, 401, 'anonymous create rejected');

        const unknownTpl = await expectError(createComment(author.token, 'tpl_does_not_exist', 'hi'));
        browser.assert.strictEqual(unknownTpl.status, 404, 'unknown template 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('edits a comment within the window and keeps its list position', function (browser) {
    browser.perform(async (done) => {
      try {
        const created = await createComment(author.token, template.id, 'Editable original');
        const edited = await updateComment(author.token, created.id, 'Edited content');
        browser.assert.strictEqual(edited.content, 'Edited content');
        browser.assert.strictEqual(edited.is_edited, true, 'is_edited set');

        // Edit keeps created_at, so the comment stays in its position.
        const list = await listComments(author.token, template.id);
        const found = list.comments.find((c) => c.id === created.id);
        browser.assert.strictEqual(found.content, 'Edited content');
        browser.assert.strictEqual(found.is_edited, true);

        // Non-author edit is forbidden.
        const denied = await expectError(updateComment(stranger.token, created.id, 'hijack'));
        browser.assert.strictEqual(denied.status, 403, 'non-author edit 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('paginates comments with page_size', function (browser) {
    browser.perform(async (done) => {
      try {
        // Fresh user so the 5/min rate limit cannot interfere.
        const pager = await registerUser({ displayName: 'Comments Pager' });
        const t = await createTemplate(pager.token, { title: 'Comments Pagination Template' });
        await createComment(pager.token, t.id, 'c1');
        await createComment(pager.token, t.id, 'c2');
        await createComment(pager.token, t.id, 'c3');

        const page1 = await listComments(pager.token, t.id, 1, 2);
        browser.assert.strictEqual(page1.total_count, 3);
        browser.assert.strictEqual(page1.page, 1);
        browser.assert.strictEqual(page1.page_size, 2);
        browser.assert.strictEqual(page1.comments.length, 2);
        browser.assert.strictEqual(page1.comments[0].content, 'c3', 'newest first on page 1');
        browser.assert.strictEqual(page1.comments[1].content, 'c2');

        const page2 = await listComments(pager.token, t.id, 2, 2);
        browser.assert.strictEqual(page2.comments.length, 1);
        browser.assert.strictEqual(page2.comments[0].content, 'c1');

        const page3 = await listComments(pager.token, t.id, 3, 2);
        browser.assert.strictEqual(page3.comments.length, 0, 'page beyond range is empty');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('lets the template owner delete another user’s comment and drops the count', function (browser) {
    browser.perform(async (done) => {
      try {
        const t = await createTemplate(owner.token, { title: 'Owner Delete Template' });
        const c = await createComment(author.token, t.id, 'To be removed by owner');
        const before = await getTemplate(author.token, t.id);
        browser.assert.strictEqual(before.comment_count, 1);

        const denied = await expectError(deleteComment(stranger.token, c.id));
        browser.assert.strictEqual(denied.status, 403, 'stranger delete 403');

        const res = await deleteComment(owner.token, c.id);
        browser.assert.strictEqual(res.success, true, 'owner delete succeeds');

        const after = await getTemplate(author.token, t.id);
        browser.assert.strictEqual(after.comment_count, 0, 'count decremented');

        const gone = await expectError(deleteComment(owner.token, c.id));
        browser.assert.strictEqual(gone.status, 404, 'already-deleted 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('blocks banned users from commenting with 403', function (browser) {
    browser.perform(async (done) => {
      try {
        const banned = await registerUser({ displayName: 'Comments Banned' });
        const ban = await banUser(adminToken, banned.id, 'e2e ban for comment test');
        browser.assert.strictEqual(ban.user.status, 'banned', 'ban applied');

        const res = await expectError(createComment(banned.token, template.id, 'should fail'));
        browser.assert.strictEqual(res.status, 403, 'banned user create 403');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('hides private template comments from non-owners with 404', function (browser) {
    browser.perform(async (done) => {
      try {
        const t = await createTemplate(owner.token, {
          title: 'Private Comments Template',
          visibility: 'private',
        });
        await createComment(owner.token, t.id, 'private note');

        const asOwner = await listComments(owner.token, t.id);
        browser.assert.strictEqual(asOwner.total_count, 1, 'owner reads own private comments');

        const asStranger = await expectError(listComments(stranger.token, t.id));
        browser.assert.strictEqual(asStranger.status, 404, 'non-owner list 404');

        const anonCreate = await expectError(createComment(stranger.token, t.id, 'intrusion'));
        browser.assert.strictEqual(anonCreate.status, 404, 'non-owner create 404 (indistinguishable)');

        const anonList = await expectError(listComments(null, t.id));
        browser.assert.strictEqual(anonList.status, 404, 'anonymous list 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('moderates comments as admin: filters, reason validation, delete + audit log', function (browser) {
    browser.perform(async (done) => {
      try {
        const t = await createTemplate(owner.token, { title: 'Moderation Target Template' });
        const marker = `modmark_${Date.now()}`;
        const c = await createComment(author.token, t.id, `Reportable ${marker}`);

        // Free-text filter matches content. `total` is a proto int64, so
        // protojson serializes it as a JSON string — coerce before compare.
        const byQ = await adminListComments(adminToken, { q: marker });
        browser.assert.strictEqual(parseInt(byQ.total, 10), 1, 'q filter matches');
        browser.assert.strictEqual(byQ.entries[0].id, c.id);
        browser.assert.strictEqual(byQ.entries[0].user_email, author.email, 'moderation exposes author email');
        browser.assert.strictEqual(byQ.entries[0].template_title, 'Moderation Target Template');

        // template_id filter.
        const byTpl = await adminListComments(adminToken, { templateId: t.id });
        browser.assert.strictEqual(
          byTpl.entries.some((e) => e.id === c.id),
          true,
          'template_id filter includes the comment'
        );

        // user_id filter.
        const byUser = await adminListComments(adminToken, { userId: author.id });
        browser.assert.strictEqual(
          byUser.entries.some((e) => e.id === c.id),
          true,
          'user_id filter includes the comment'
        );

        // Reason validation: too short.
        const shortReason = await expectError(adminDeleteComment(adminToken, c.id, 'no'));
        browser.assert.strictEqual(shortReason.status, 400, 'short reason 400');

        // Non-admin cannot moderate.
        const nonAdmin = await expectError(adminDeleteComment(author.token, c.id, 'not an admin reason'));
        browser.assert.strictEqual(nonAdmin.status, 403, 'non-admin moderation 403');

        // Valid delete.
        const del = await adminDeleteComment(adminToken, c.id, 'e2e moderation delete reason');
        browser.assert.strictEqual(del.success, true, 'admin delete succeeds');

        // Audit log entry with action=delete_comment and matching target.
        const logs = await getAuditLogs(adminToken, { action: 'delete_comment' });
        const entry = logs.entries.find((e) => e.target_id === c.id);
        browser.assert.strictEqual(entry !== undefined, true, 'audit log entry exists');
        browser.assert.strictEqual(entry.reason, 'e2e moderation delete reason');
        browser.assert.strictEqual(entry.target_type, 'comment');

        // Comment is gone for regular readers.
        const list = await listComments(author.token, t.id);
        browser.assert.strictEqual(list.total_count, 0, 'comment removed');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('notifies the template owner about a new comment', function (browser) {
    browser.perform(async (done) => {
      try {
        const t = await createTemplate(owner.token, { title: 'Notify Comments Template' });
        await createComment(author.token, t.id, 'Owner should get notified');
        const notifs = await getNotifications(owner.token, { filter: 'all' });
        const commentNotif = notifs.notifications.find((n) => n.type === 'comment');
        browser.assert.strictEqual(commentNotif !== undefined, true, 'comment notification exists');
        browser.assert.strictEqual(commentNotif.actor_id, author.id);
        browser.assert.strictEqual(commentNotif.template_id, t.id);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rate-limits the 6th comment in a minute with 429 + Retry-After', function (browser) {
    browser.perform(async (done) => {
      try {
        const spammer = await registerUser({ displayName: 'Comments Spammer' });
        const t = await createTemplate(spammer.token, { title: 'Rate Limit Template' });
        for (let i = 1; i <= 5; i += 1) {
          await createComment(spammer.token, t.id, `burst ${i}`);
        }
        const sixth = await expectError(createComment(spammer.token, t.id, 'burst 6'));
        browser.assert.strictEqual(sixth.status, 429, '6th comment within a minute rejected');
        const retryAfter = parseInt(sixth.headers['retry-after'], 10);
        browser.assert.strictEqual(
          Number.isInteger(retryAfter) && retryAfter > 0 && retryAfter <= 60,
          true,
          'Retry-After header present with sane value'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});
