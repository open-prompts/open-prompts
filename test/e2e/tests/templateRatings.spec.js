// E2E: template ratings feature (8a480eb).
//
// REST contract:
//   - POST   /api/v1/templates/{id}/rating (auth; JSON {rating:1..5};
//     owner self-rate 403; anonymous 401; invalid 0/6/3.5 -> 400;
//     private/hidden -> 404 for non-owner; banned caller 403;
//     rate limit RATING_RATE_PER_MINUTE=20 -> 429 + Retry-After)
//     returns {rating_avg, rating_count, my_rating}
//   - DELETE /api/v1/templates/{id}/rating (auth; idempotent — repeat
//     delete returns identical body)
//   - GET    /api/v1/templates/{id} returns rating_avg/rating_count/
//     my_rating on the template + rating_distribution (5-element array,
//     index 0->1★ ... 4->5★) on the response
//   - GET    /api/v1/templates?sort=top_rated (threshold
//     TOP_RATED_MIN_RATINGS=5; invalid -> newest)
//   - admin  GET /api/v1/admin/templates/{id} summary gains
//     rating_avg/rating_count; admin rating mutation routes -> 404/405
// UI: RatingWidget on TemplateDetails (interactive 5-star, keyboard
//     <-/->/Enter/Esc, optimistic fill + revert, owner hint, clear-with-
//     confirm, anonymous -> /login), RatingHistogram (5-row distribution),
//     StarRating on PromptCard + admin TemplateDetailPage, SortDropdown +
//     useTemplateFilters top_rated option.
// i18n: ratings.* EN/ZH key parity + sort.top_rated.

const {
  request,
  registerUser,
  adminLogin,
  createTemplate,
  setReviewThreshold,
  getTemplate,
  searchTemplates,
  getAdminTemplateDetail,
  banUser,
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

describe('Template ratings', function () {
  const fx = {
    marker: `rt${Date.now().toString(36)}`,
    owner: {},    // template owner (sees owner hint, no widget)
    rater: {},    // rates the template
    second: {},   // second rater for aggregate math
    banned: {},   // banned caller
    admin: {},
    template: {}, // owner's public template
    privateTpl: {}, // owner's private template
  };

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      fx.admin = admin;
      await setReviewThreshold(admin.token, 0, 'e2e template-ratings: disable gating');

      Object.assign(fx.owner, await registerUser({ displayName: 'Ratings Owner' }));
      Object.assign(fx.rater, await registerUser({ displayName: 'Ratings Rater' }));
      Object.assign(fx.second, await registerUser({ displayName: 'Ratings Second' }));
      Object.assign(fx.banned, await registerUser({ displayName: 'Ratings Banned' }));

      const t = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Rateable`,
        description: 'ratings e2e fixture',
        category: 'Writing',
        tags: ['ratings'],
        content: 'rate me',
      });
      Object.assign(fx.template, t);

      const priv = await createTemplate(fx.owner.token, {
        title: `${fx.marker} Private`,
        visibility: 'private',
      });
      Object.assign(fx.privateTpl, priv);

      // Ban the banned user so its rating attempts are rejected with 403.
      await banUser(admin.token, fx.banned.id, 'e2e ratings ban');
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: rate / re-rate / delete ----------------

  it('rates a template and reflects avg/count/my_rating', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/templates/${fx.template.id}/rating`,
          { rating: 4 }, { token: fx.rater.token }
        );
        browser.assert.strictEqual(r.data.rating_avg, 4, 'avg is 4 after one 4-star');
        browser.assert.strictEqual(r.data.rating_count, 1, 'count is 1');
        browser.assert.strictEqual(r.data.my_rating, 4, 'my_rating is 4');

        // Detail reflects the rating + distribution.
        const detail = await getTemplate(fx.rater.token, fx.template.id);
        browser.assert.strictEqual(detail.rating_avg, 4, 'detail rating_avg is 4');
        browser.assert.strictEqual(detail.rating_count, 1, 'detail rating_count is 1');
        browser.assert.strictEqual(detail.my_rating, 4, 'detail my_rating is 4');

        const raw = await request('GET', `/api/v1/templates/${fx.template.id}`, null, { token: fx.rater.token });
        browser.assert.deepStrictEqual(
          raw.data.rating_distribution, [0, 0, 0, 1, 0],
          'distribution has one 4-star (index 3)'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('re-rating replaces the row (count unchanged, avg recomputed)', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await request(
          'POST', `/api/v1/templates/${fx.template.id}/rating`,
          { rating: 2 }, { token: fx.rater.token }
        );
        browser.assert.strictEqual(r.data.rating_avg, 2, 'avg is 2 after re-rate');
        browser.assert.strictEqual(r.data.rating_count, 1, 'count stays 1 (row replaced)');
        browser.assert.strictEqual(r.data.my_rating, 2, 'my_rating is 2');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('a second rater moves the aggregate average', function (browser) {
    browser.perform(async function (done) {
      try {
        // rater currently 2, second rates 5 -> avg 3.5, count 2.
        const r = await request(
          'POST', `/api/v1/templates/${fx.template.id}/rating`,
          { rating: 5 }, { token: fx.second.token }
        );
        browser.assert.strictEqual(r.data.rating_avg, 3.5, 'avg is 3.5 (2+5)/2');
        browser.assert.strictEqual(r.data.rating_count, 2, 'count is 2');
        browser.assert.strictEqual(r.data.my_rating, 5, 'second my_rating is 5');

        const raw = await request('GET', `/api/v1/templates/${fx.template.id}`, null, { token: fx.rater.token });
        browser.assert.deepStrictEqual(
          raw.data.rating_distribution, [0, 1, 0, 0, 1],
          'distribution: one 2-star (idx1) + one 5-star (idx4)'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('deleting a rating is idempotent and clears my_rating', function (browser) {
    browser.perform(async function (done) {
      try {
        const del = await request(
          'DELETE', `/api/v1/templates/${fx.template.id}/rating`,
          null, { token: fx.rater.token }
        );
        browser.assert.strictEqual(del.data.rating_avg, 5, 'avg is 5 after rater deletes (only second 5 remains)');
        browser.assert.strictEqual(del.data.rating_count, 1, 'count is 1');
        browser.assert.strictEqual(del.data.my_rating, 0, 'my_rating is 0 after delete');

        const del2 = await request(
          'DELETE', `/api/v1/templates/${fx.template.id}/rating`,
          null, { token: fx.rater.token }
        );
        browser.assert.deepStrictEqual(
          del2.data, del.data,
          'repeat delete returns identical body (idempotent)'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: validation / authz ----------------

  it('rejects owner self-rate, anonymous, and invalid ratings', function (browser) {
    browser.perform(async function (done) {
      try {
        const ownerRate = await expectError(
          request('POST', `/api/v1/templates/${fx.template.id}/rating`, { rating: 5 }, { token: fx.owner.token })
        );
        browser.assert.strictEqual(ownerRate.status, 403, 'owner self-rate -> 403');

        const anon = await expectError(
          request('POST', `/api/v1/templates/${fx.template.id}/rating`, { rating: 5 }, {})
        );
        browser.assert.strictEqual(anon.status, 401, 'anonymous rate -> 401');

        for (const bad of [0, 6, 3.5]) {
          const r = await expectError(
            request('POST', `/api/v1/templates/${fx.template.id}/rating`, { rating: bad }, { token: fx.second.token })
          );
          browser.assert.strictEqual(r.status, 400, `rating ${bad} -> 400`);
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('returns 404 for a private template rated by a non-owner', function (browser) {
    browser.perform(async function (done) {
      try {
        const r = await expectError(
          request('POST', `/api/v1/templates/${fx.privateTpl.id}/rating`, { rating: 5 }, { token: fx.second.token })
        );
        browser.assert.strictEqual(r.status, 404, 'non-owner rate of private -> 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects a banned caller (401/403)', function (browser) {
    browser.perform(async function (done) {
      try {
        // Banning revokes the caller's token, so the gateway may reject with
        // 401 (invalid token) or 403 (banned). The FVT accepts either.
        const r = await expectError(
          request('POST', `/api/v1/templates/${fx.template.id}/rating`, { rating: 5 }, { token: fx.banned.token })
        );
        browser.assert.ok(
          r.status === 401 || r.status === 403,
          `banned caller rate -> ${r.status} (401/403)`
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('enforces the rating rate limit with 429 + Retry-After', function (browser) {
    browser.perform(async function (done) {
      try {
        // RATING_RATE_PER_MINUTE=20. Hammer a fresh template with rapid
        // changes; the 21st should 429 with a Retry-After header.
        const t = await createTemplate(fx.owner.token, {
          title: `${fx.marker} RateLimit ${Date.now()}`, visibility: 'public',
        });
        let hit = -1;
        let retryAfter = null;
        for (let i = 0; i < 25; i++) {
          const r = await expectError(
            request('POST', `/api/v1/templates/${t.id}/rating`, { rating: (i % 5) + 1 }, { token: fx.second.token })
          );
          if (r.status === 429) {
            hit = i;
            retryAfter = r.headers['retry-after'];
            break;
          }
          browser.assert.strictEqual(r.status, 0, `rate change #${i + 1} under the limit is 2xx`);
        }
        browser.assert.ok(hit >= 19, `rate limit hit at/after change 20 (index ${hit})`);
        browser.assert.ok(retryAfter !== null, '429 response carries Retry-After header');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: top_rated sort ----------------

  it('sorts by top_rated with the 5-rating threshold', function (browser) {
    browser.perform(async function (done) {
      try {
        // The rate-limit test above exhausted fx.second's per-account quota
        // (20/min), so use fresh raters here. Each distinct rater must rate
        // once to reach count>=5 (re-rating the same user replaces the row).
        const t = await createTemplate(fx.owner.token, {
          title: `${fx.marker} TopRated ${Date.now()}`, visibility: 'public',
        });
        for (let i = 0; i < 5; i++) {
          const rater = await registerUser({ displayName: `Ratings TopRater${i}` });
          await request('POST', `/api/v1/templates/${t.id}/rating`, { rating: 5 }, { token: rater.token });
        }
        const top = await searchTemplates({ sort: 'top_rated' });
        browser.assert.ok(Array.isArray(top.templates), 'top_rated returns a templates array');
        const eligible = top.templates.filter((x) => x.rating_count >= 5);
        for (let i = 1; i < eligible.length; i++) {
          browser.assert.ok(
            eligible[i - 1].rating_avg >= eligible[i].rating_avg,
            `top_rated avg desc at index ${i}`
          );
        }
        const mine = top.templates.find((x) => x.id === t.id);
        browser.assert.ok(mine, 'the 5-rated template appears in top_rated');
        browser.assert.strictEqual(mine.rating_count, 5, 'eligible template has count 5');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: admin surfaces ----------------

  it('exposes rating_avg/rating_count on the admin template summary', function (browser) {
    browser.perform(async function (done) {
      try {
        const detail = await getAdminTemplateDetail(fx.admin.token, fx.template.id);
        browser.assert.strictEqual(detail.template.rating_avg, 5, 'admin summary rating_avg is 5');
        browser.assert.strictEqual(detail.template.rating_count, 1, 'admin summary rating_count is 1');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects admin rating mutation routes with 404/405', function (browser) {
    browser.perform(async function (done) {
      try {
        const post = await expectError(
          request('POST', `/api/v1/admin/templates/${fx.template.id}/rating`, { rating: 5 }, { token: fx.admin.token })
        );
        browser.assert.ok(
          post.status === 404 || post.status === 405,
          `admin rating POST -> ${post.status} (404/405)`
        );
        const del = await expectError(
          request('DELETE', `/api/v1/admin/templates/${fx.template.id}/rating`, null, { token: fx.admin.token })
        );
        browser.assert.ok(
          del.status === 404 || del.status === 405,
          `admin rating DELETE -> ${del.status} (404/405)`
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI: TemplateDetails rating widget ----------------

  it('renders the rating widget and histogram on TemplateDetails', function (browser) {
    freshSession(browser, `${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.rating-block', 20000)
      .waitForElementVisible('.rating-widget', 10000)
      .waitForElementVisible('.rating-widget__stars', 10000);
    browser.elements('css selector', '.rating-widget__star', function (res) {
      this.assert.strictEqual(res.value.length, 5, 'widget renders 5 stars');
    });
    // Histogram: 5 rows with text percentages.
    browser.waitForElementVisible('.rating-histogram', 10000);
    browser.elements('css selector', '.rating-histogram__row', function (res) {
      this.assert.strictEqual(res.value.length, 5, 'histogram renders 5 rows');
    });
    browser.elements('css selector', '.rating-histogram__pct', function (res) {
      this.assert.ok(res.value.length === 5, 'histogram rows carry percentage text');
    });
  });

  it('rates via the widget and reflects the change optimistically', function (browser) {
    // Sign in as the rater, open the detail, click the 4th star.
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('form', 20000)
      .setValue('input[type="email"], input[name="email"]', fx.rater.email)
      .setValue('input[type="password"], input[name="password"]', 'Password_123456')
      .submitForm('form');
    browser.pause(1500);

    browser.url(`${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.rating-widget', 20000)
      .waitForElementVisible('.rating-widget__star', 10000);
    // Click the 4th star (index 3).
    browser.execute(function () {
      const stars = document.querySelectorAll('.rating-widget__star');
      if (stars.length < 4) return false;
      stars[3].click();
      return true;
    }, [], function (res) {
      browser.assert.ok(res.value, 'clicked the 4th star');
    });
    // Optimistic fill: 4 stars active.
    browser.waitForElementVisible('.rating-widget__star.active', 10000);
    browser.elements('css selector', '.rating-widget__star.active', function (res) {
      this.assert.strictEqual(res.value.length, 4, '4 stars filled after commit');
    });
    // Clear button appears once my_rating is set.
    browser.waitForElementVisible('.rating-widget__clear', 10000);
  });

  it('clears a rating via the confirm popover', function (browser) {
    browser.url(`${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.rating-widget__clear', 20000)
      .click('.rating-widget__clear')
      .waitForElementVisible('.rating-widget__confirm', 10000)
      .click('.rating-widget__confirm-yes');
    // After clear, the clear button disappears (my_rating -> 0).
    browser.waitForElementNotPresent('.rating-widget__clear', 10000);
  });

  it('shows the owner hint instead of an interactive widget for the owner', function (browser) {
    freshSession(browser, `${browser.launch_url}/login`)
      .waitForElementVisible('form', 20000)
      .setValue('input[type="email"], input[name="email"]', fx.owner.email)
      .setValue('input[type="password"], input[name="password"]', 'Password_123456')
      .submitForm('form');
    browser.pause(1500);

    browser.url(`${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.rating-widget--owner', 20000)
      .waitForElementVisible('.rating-widget__hint', 10000);
    browser.assert.not.elementPresent('.rating-widget__stars', 'owner sees no interactive stars');
  });

  it('routes anonymous rating clicks to /login', function (browser) {
    freshSession(browser, `${browser.launch_url}/templates/${fx.template.id}`)
      .waitForElementVisible('.rating-widget__star', 20000);
    browser.execute(function () {
      const stars = document.querySelectorAll('.rating-widget__star');
      if (stars.length < 1) return false;
      stars[0].click();
      return true;
    }, [], function (res) {
      browser.assert.ok(res.value, 'clicked a star anonymously');
    });
    browser.waitForElementVisible('form', 20000);
    browser.assert.urlContains('/login');
  });

  // ---------------- UI: top_rated sort on Home ----------------

  it('offers the top_rated option in the Home sort dropdown', function (browser) {
    freshSession(browser, `${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      .waitForElementVisible('.sort-dropdown-trigger', 10000)
      .click('.sort-dropdown-trigger')
      .waitForElementVisible('.sort-dropdown-menu', 10000);
    browser.elements('css selector', '.sort-dropdown-option', function (res) {
      this.assert.ok(res.value.length >= 5, 'sort dropdown lists 5+ options');
    });
    browser.execute(function () {
      const opts = Array.from(document.querySelectorAll('.sort-dropdown-option'))
        .map((o) => o.textContent.trim());
      return opts;
    }, [], function (res) {
      browser.assert.ok(
        res.value.some((t) => /top rated/i.test(t)),
        'top_rated option present in the sort dropdown'
      );
    });
  });

  // ---------------- UI: admin template detail rating display ----------------

  it('shows the read-only rating on the admin TemplateDetailPage', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-sidebar', 20000)
      .url(`${browser.launch_url}/admin/templates/${fx.template.id}`)
      .waitForElementVisible('.admin-template-detail__header', 20000);
    browser.assert.textContains('.admin-template-detail__header', 'Rating');
  });

  // ---------------- i18n parity ----------------

  it('keeps EN/ZH parity for ratings.* and sort.top_rated keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enR = en.ratings || {};
        const zhR = zh.ratings || {};
        browser.assert.deepStrictEqual(
          Object.keys(enR).sort(), Object.keys(zhR).sort(),
          'ratings.* top-level key sets match'
        );
        for (const k of Object.keys(enR)) {
          browser.assert.strictEqual(
            typeof zhR[k], 'string', `ratings.${k} is a string in zh`
          );
        }
        browser.assert.strictEqual(
          en.search.sort.top_rated, 'Top rated', 'EN sort.top_rated'
        );
        browser.assert.strictEqual(
          zh.search.sort.top_rated, '评分最高', 'ZH sort.top_rated'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});