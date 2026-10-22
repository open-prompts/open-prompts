// E2E: comment replies & threads feature (6205022).
//
// REST contract:
//   - POST /api/v1/templates/{id}/comments with optional parent_comment_id
//     (top-level when absent; reply when set). Parent must be a top-level
//     comment on the SAME template -> 400; unknown parent -> 400; cross-
//     template parent -> 400; thread full (51st reply) -> 400; replies share
//     the 5/min + 50/day rate limit -> 429 + Retry-After. A reply is flagged
//     is_owner when the template owner posts it.
//   - GET /api/v1/comments/{id}/replies (oldest-first, <=50, optional auth;
//     private template owner-only 404).
//   - Top-level GET /api/v1/templates/{id}/comments items carry reply_count +
//     has_creator_reply.
//   - DELETE /api/v1/comments/{id} cascades replies when the target is
//     top-level and decrements reply_count when it is a reply.
//   - admin GET /api/v1/admin/comments entries carry
//     parent_author_display_name (empty for top-level).
//   - admin DELETE /api/v1/admin/comments/{id} follows cascade + audit.
//   - comment_reply notification targets the PARENT comment author (not the
//     template owner); self-replies suppressed; payload carries a <=100-char
//     snippet.
//
// UI: admin /admin/comments parent column ("in reply to {author}" vs "—"),
//     notifications page renders the comment_reply snippet.
// i18n: comments.*, admin.comments.*, notifications.event.comment_reply,
//     notifications.snippet.comment_reply EN/ZH parity.
//
// NOTE: the CommentsSection/ReplyComposer/RepliesBucket UI components exist
// in frontend/src/components/comments/ but are not yet wired into
// TemplateDetails (no import). The reply/bucket/composer UI is therefore
// pinned via the API contract here (same pattern as templateComments.spec.js);
// the wired UI surfaces (admin comments page, notifications page) are tested
// directly.
const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  createComment,
  createReply,
  listCommentReplies,
  listComments,
  deleteComment,
  getTemplate,
  adminListComments,
  adminDeleteComment,
  getAuditLogs,
  getNotifications,
  setReviewThreshold,
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

describe('Comment replies & threads', function () {
  const fx = {
    marker: `crt${Date.now().toString(36)}`,
    owner: {},      // template owner (creator)
    author: {},     // posts the top-level comment; receives comment_reply
    stranger: {},   // non-owner, non-author
    admin: {},
    template: {},   // public template
    root: {},       // top-level comment by author
    reply: {},      // owner's reply to root
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e comment-replies-threads: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Thread Owner' }));
      Object.assign(fx.author, await registerUser({ displayName: 'Thread Author' }));
      Object.assign(fx.stranger, await registerUser({ displayName: 'Thread Stranger' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Threaded`,
        description: 'comment replies e2e fixture',
        category: 'Writing',
        tags: ['threads'],
        content: 'thread me',
      });
      Object.assign(fx.template, t);

      // Author posts a top-level comment; owner replies to it.
      const root = await createComment(fx.author.token, fx.template.id, 'Thread root comment');
      Object.assign(fx.root, root);
      const reply = await createReply(fx.owner.token, fx.template.id, 'Owner reply one', root.id);
      Object.assign(fx.reply, reply);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: reply flow ----------------

  it('creates a reply and lists it oldest-first with reply_count + has_creator_reply', function (browser) {
    browser.perform(async function (done) {
      try {
        // Root created in before(): reply_count should be 1, has_creator_reply true.
        const top = await listComments(fx.author.token, fx.template.id);
        const root = top.comments.find((c) => c.id === fx.root.id);
        browser.assert.ok(root, 'root present in top-level list');
        browser.assert.strictEqual(root.reply_count, 1, 'reply_count is 1');
        browser.assert.strictEqual(root.has_creator_reply, true, 'has_creator_reply true (owner replied)');

        // Reply is flagged is_owner (template owner posted it).
        browser.assert.strictEqual(fx.reply.is_owner, true, 'owner reply flagged is_owner');

        // Replies list oldest-first.
        const replies = await listCommentReplies(fx.author.token, fx.root.id);
        browser.assert.strictEqual(replies.replies.length, 1, 'one reply');
        browser.assert.strictEqual(replies.replies[0].id, fx.reply.id, 'reply listed');
        browser.assert.strictEqual(replies.replies[0].content, 'Owner reply one');

        // comment_count counts top-level only — replies do not bump it.
        const tpl = await getTemplate(fx.author.token, fx.template.id);
        browser.assert.strictEqual(tpl.comment_count, 1, 'comment_count is 1 (top-level only)');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects reply-to-reply, cross-template parent, and unknown parent with 400', function (browser) {
    browser.perform(async function (done) {
      try {
        // Reply-to-reply: parent is a reply (not top-level) -> 400.
        const nested = await expectError(
          createReply(fx.author.token, fx.template.id, 'nested', fx.reply.id)
        );
        browser.assert.strictEqual(nested.status, 400, 'reply-to-reply is 400');

        // Cross-template parent: a top-level comment on a different template -> 400.
        const otherTpl = await createTemplate(fx.owner.token, { title: `${fx.marker} Other` });
        const otherRoot = await createComment(fx.owner.token, otherTpl.id, 'other root');
        const cross = await expectError(
          createReply(fx.author.token, fx.template.id, 'cross', otherRoot.id)
        );
        browser.assert.strictEqual(cross.status, 400, 'cross-template parent is 400');

        // Unknown parent -> 400.
        const ghost = await expectError(
          createReply(fx.author.token, fx.template.id, 'ghost', '00000000-0000-0000-0000-000000000000')
        );
        browser.assert.strictEqual(ghost.status, 400, 'unknown parent is 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('enforces the thread cap: 51st reply is 400', function (browser) {
    browser.perform(async function (done) {
      try {
        // Fresh user so the 5/min rate limit cannot interfere.
        const capRootUser = await registerUser({ displayName: 'Thread Cap Root' });
        const capRoot = await createComment(capRootUser.token, fx.template.id, 'Cap root');

        // Pool of fresh users to post 50 replies without tripping the 5/min limit.
        const pool = [];
        for (let i = 0; i < 10; i += 1) {
          pool.push(await registerUser({ displayName: `Thread Cap ${i}` }));
        }
        for (let i = 0; i < 50; i += 1) {
          const u = pool[i % pool.length];
          await createReply(u.token, fx.template.id, `cap reply ${i}`, capRoot.id);
        }

        const fiftyFirst = await expectError(
          createReply(pool[0].token, fx.template.id, '51st reply', capRoot.id)
        );
        browser.assert.strictEqual(fiftyFirst.status, 400, '51st reply is 400 (thread full)');

        // Anonymous can still read the 50-reply thread on a public template.
        const replies = await listCommentReplies(null, capRoot.id);
        browser.assert.strictEqual(replies.replies.length, 50, 'anonymous reads 50 replies');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('notifies the parent author with a snippet and suppresses self-replies', function (browser) {
    browser.perform(async function (done) {
      try {
        // Author is the parent author of fx.root; owner's reply should have
        // notified the author (not the template owner).
        const notifs = await getNotifications(fx.author.token, { filter: 'all' });
        const replyNotifs = notifs.notifications.filter((n) => n.type === 'comment_reply');
        browser.assert.ok(replyNotifs.length >= 1, 'parent author has a comment_reply notification');
        const mine = replyNotifs.find((n) => n.actor_id === fx.owner.id);
        browser.assert.ok(mine, 'notification actor is the replier (owner)');
        browser.assert.strictEqual(mine.template_id, fx.template.id, 'notification template matches');
        browser.assert.ok(mine.snippet && mine.snippet.length > 0, 'notification carries a snippet');
        browser.assert.ok(mine.snippet.length <= 100, 'snippet is <= 100 chars');

        // Self-reply: author replies to own comment -> no comment_reply to self.
        const selfReply = await createReply(fx.author.token, fx.template.id, 'self reply', fx.root.id);
        browser.assert.ok(selfReply.id, 'self reply created');
        const after = await getNotifications(fx.author.token, { filter: 'all' });
        const selfNotifs = after.notifications.filter(
          (n) => n.type === 'comment_reply' && n.actor_id === fx.author.id
        );
        browser.assert.strictEqual(selfNotifs.length, 0, 'self-reply sends no notification');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('deletes a reply (reply_count -1) and cascades a top-level delete', function (browser) {
    browser.perform(async function (done) {
      try {
        // Fresh thread to avoid mutating fx.root used by later tests.
        const delRootUser = await registerUser({ displayName: 'Thread Del Root' });
        const delRoot = await createComment(delRootUser.token, fx.template.id, 'Del root');
        const r1 = await createReply(fx.owner.token, fx.template.id, 'del reply 1', delRoot.id);
        const r2 = await createReply(fx.author.token, fx.template.id, 'del reply 2', delRoot.id);

        // Delete a reply -> reply_count -1, sibling intact, comment_count unchanged.
        const before = await getTemplate(fx.author.token, fx.template.id);
        const delReply = await deleteComment(fx.author.token, r2.id);
        browser.assert.strictEqual(delReply.success, true, 'reply delete succeeds');
        const topAfterReply = await listComments(fx.author.token, fx.template.id);
        const rootAfterReply = topAfterReply.comments.find((c) => c.id === delRoot.id);
        browser.assert.strictEqual(rootAfterReply.reply_count, 1, 'reply_count decremented to 1');
        const after = await getTemplate(fx.author.token, fx.template.id);
        browser.assert.strictEqual(after.comment_count, before.comment_count, 'comment_count unchanged by reply delete');

        // Delete the top-level comment -> replies cascade, comment_count -1.
        const delTop = await deleteComment(fx.owner.token, delRoot.id);
        browser.assert.strictEqual(delTop.success, true, 'top-level delete succeeds');
        const afterTop = await getTemplate(fx.author.token, fx.template.id);
        browser.assert.strictEqual(afterTop.comment_count, before.comment_count - 1, 'comment_count -1 after top-level delete');

        // Replies of the deleted top-level are gone (404).
        const gone = await expectError(listCommentReplies(fx.author.token, delRoot.id));
        browser.assert.strictEqual(gone.status, 404, 'replies of deleted top-level are 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('exposes the parent column in the admin list and admin-deletes a reply with audit', function (browser) {
    browser.perform(async function (done) {
      try {
        // Dedicated template so the admin list is not paginated past the
        // thread (the shared template accumulates 50+ cap replies).
        const adminTpl = await createTemplate(fx.owner.token, { title: `${fx.marker} Admin Thread` });
        const adminRoot = await createComment(fx.author.token, adminTpl.id, 'Admin root');
        const adminReply = await createReply(fx.owner.token, adminTpl.id, 'Admin reply', adminRoot.id);

        // Admin list should carry parent_author_display_name for the reply and
        // empty for the root.
        const list = await adminListComments(fx.admin.token, { templateId: adminTpl.id });
        const rootEntry = list.entries.find((e) => e.id === adminRoot.id);
        const replyEntry = list.entries.find((e) => e.id === adminReply.id);
        browser.assert.ok(rootEntry, 'root in admin list');
        browser.assert.ok(replyEntry, 'reply in admin list');
        browser.assert.strictEqual(
          rootEntry.parent_author_display_name || '',
          '',
          'top-level entry has empty parent_author_display_name'
        );
        browser.assert.strictEqual(
          replyEntry.parent_author_display_name,
          fx.author.displayName,
          'reply entry carries parent author display name'
        );

        // Admin delete the reply with a reason -> audit log entry.
        const del = await adminDeleteComment(fx.admin.token, adminReply.id, 'e2e reply moderation reason');
        browser.assert.strictEqual(del.success, true, 'admin reply delete succeeds');
        const logs = await getAuditLogs(fx.admin.token, { action: 'delete_comment' });
        const entry = logs.entries.find((e) => e.target_id === adminReply.id);
        browser.assert.ok(entry, 'audit log entry for admin-deleted reply');
        browser.assert.strictEqual(entry.target_type, 'comment');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('hides private template threads from non-owners with 404', function (browser) {
    browser.perform(async function (done) {
      try {
        // Fresh owner so the 5/min rate limit (shared with comments) is not
        // exhausted by the earlier reply-heavy tests.
        const privOwner = await registerUser({ displayName: 'Thread Priv Owner' });
        const priv = await createTemplate(privOwner.token, {
          title: `${fx.marker} Private Thread`,
          visibility: 'private',
        });
        const privRoot = await createComment(privOwner.token, priv.id, 'private root');
        await createReply(privOwner.token, priv.id, 'private reply', privRoot.id);

        const asOwner = await listCommentReplies(privOwner.token, privRoot.id);
        browser.assert.strictEqual(asOwner.replies.length, 1, 'owner reads own private thread');

        const asStranger = await expectError(listCommentReplies(fx.stranger.token, privRoot.id));
        browser.assert.strictEqual(asStranger.status, 404, 'non-owner private thread read is 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rate-limits the 6th reply in a minute with 429 + Retry-After', function (browser) {
    browser.perform(async function (done) {
      try {
        const rlUser = await registerUser({ displayName: 'Thread Rate Limit' });
        const rlRoot = await createComment(rlUser.token, fx.template.id, 'rl root'); // comment 1
        // 4 replies = comments 2-5 (within the 5/min limit).
        for (let i = 0; i < 4; i += 1) {
          await createReply(rlUser.token, fx.template.id, `rl reply ${i}`, rlRoot.id);
        }
        // The 5th reply is the 6th comment in the minute -> 429.
        const sixth = await expectError(
          createReply(rlUser.token, fx.template.id, 'rl 6th', rlRoot.id)
        );
        browser.assert.strictEqual(sixth.status, 429, '6th reply within a minute rejected');
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

  // ---------------- UI: admin comments page + notifications ----------------

  it('renders the admin comments parent column', function (browser) {
    // Seed a fresh top-level + reply so the admin page has a reply row to show.
    browser.perform(async function (done) {
      try {
        const uiRoot = await createComment(fx.author.token, fx.template.id, 'UI parent root');
        await createReply(fx.owner.token, fx.template.id, 'UI parent reply', uiRoot.id);
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
      .url(`${browser.launch_url}/admin/comments`)
      .waitForElementVisible('.admin-comments', 15000)
      .assert.elementPresent('.admin-table')
      .assert.textContains('.admin-comments', 'in reply to Thread Author');
  });

  it('renders the comment_reply snippet on the notifications page', function (browser) {
    // Ensure the author has a comment_reply notification with a snippet.
    browser.perform(async function (done) {
      try {
        const notifs = await getNotifications(fx.author.token, { filter: 'all' });
        const replyNotif = notifs.notifications.find((n) => n.type === 'comment_reply');
        browser.assert.ok(replyNotif, 'author has a comment_reply notification for UI');
        done();
      } catch (err) {
        done(err);
      }
    });
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('form', 15000)
      .setValue('input[type="email"], input[name="email"]', fx.author.email)
      .setValue('input[type="password"], input[name="password"]', fx.author.password)
      .submitForm('form');
    browser.pause(1500);
    browser.url(`${browser.launch_url}/notifications`)
      .waitForElementVisible('.notifications-page', 20000)
      .assert.elementPresent('.notification-item')
      .assert.elementPresent('.notification-snippet');
  });

  it('keeps EN/ZH parity for comments, admin.comments, and notifications reply keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');

        // comments.* top-level keys match.
        const enC = en.comments || {};
        const zhC = zh.comments || {};
        browser.assert.deepStrictEqual(
          Object.keys(zhC).sort(),
          Object.keys(enC).sort(),
          'comments.* top-level keys match'
        );

        // admin.comments.* keys match.
        const enAC = (en.admin || {}).comments || {};
        const zhAC = (zh.admin || {}).comments || {};
        browser.assert.deepStrictEqual(
          Object.keys(zhAC).sort(),
          Object.keys(enAC).sort(),
          'admin.comments.* keys match'
        );

        // notifications.event.comment_reply + notifications.snippet.comment_reply present in both.
        browser.assert.ok(en.notifications.event.comment_reply, 'EN event.comment_reply present');
        browser.assert.ok(zh.notifications.event.comment_reply, 'ZH event.comment_reply present');
        browser.assert.ok(en.notifications.snippet.comment_reply, 'EN snippet.comment_reply present');
        browser.assert.ok(zh.notifications.snippet.comment_reply, 'ZH snippet.comment_reply present');
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});