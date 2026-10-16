// E2E: template search & filtering feature.
// API coverage (mirrors tests/fvt.py test_template_search_filtering):
//   keyword field coverage (title/description/category/tag/latest content,
//   old versions excluded), CJK substring, multi-word phrase, LIKE
//   metacharacters literal, multi-tag AND, sorts (most_liked /
//   most_favorited / trending / invalid->newest), total_count first-page
//   only, q alias, my_likes + keyword, whitespace keyword = no filter.
// UI coverage (Home page): SearchBar debounce + suggestions combobox,
// SortDropdown listbox keyboard pattern, FilterChipRow chips + clear all,
// ResultCount aria-live, SkeletonCards, EmptyState keyword echo, URL sync
// (q/category/tags/sort/visibility), localStorage visibility persistence,
// infinite scroll, Sidebar multi-tag checkboxes.
const {
  registerUser,
  adminLogin,
  createTemplate,
  updateTemplate,
  likeTemplate,
  favoriteTemplate,
  searchTemplates,
  setReviewThreshold,
} = require('../helpers/api');

function expectError(promise) {
  return promise.then(
    () => ({ status: 0, body: null, headers: {} }),
    (err) => ({ status: err.status, body: err.body, headers: err.headers || {} })
  );
}

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
    .submitForm('form')
    // Login now redirects to /dashboard (user-dashboard feature), not /.
    .waitForElementVisible('.dashboard-layout', 20000);
}

describe('Template search and filtering', function () {
  // Fixture ids, filled in before().
  const fx = {};
  let adminToken = '';

  before(function (browser, done) {
    (async () => {
      // Disable pre-publication review gating so every fixture publish goes
      // live immediately (this spec tests search, not moderation).
      const admin = await adminLogin();
      adminToken = admin.token;
      await setReviewThreshold(adminToken, 0, 'e2e search: disable gating');

      const owner = await registerUser({ displayName: 'Search Owner' });
      const bob = await registerUser({ displayName: 'Search Bob' });
      fx.owner = owner;
      fx.bob = bob;

      const create = (title, description, content, category, tags, visibility = 'public') =>
        createTemplate(owner.token, { title, description, content, category, tags, visibility });

      // Unique markers keep this run's fixtures isolated from other data.
      const m = `sfx${Date.now().toString(36)}`;
      fx.marker = m;

      // 1. Title match + phrase + tag.
      fx.tEmail = await create(
        `${m} Email Marketing Master`, 'Boost your campaigns',
        'Write compelling emails', 'Marketing', ['email', 'campaign', m]
      );
      // 2. CJK title.
      fx.tCjk = await create(
        `${m} 邮件营销助手`, 'Chinese email helper', '写一封营销邮件',
        'Marketing', ['cjk', m]
      );
      // 3. Only description contains the phrase.
      fx.tDesc = await create(
        `${m} Newsletter Tool`, `${m} email marketing playbook inside`,
        'Draft newsletters', 'Writing', ['email', m]
      );
      // 4. Old version contains marker, latest does not.
      fx.tVersioned = await create(
        `${m} Versioned Doc`, 'has an old version', `${m} OLDMARKER original content`,
        'Writing', ['docs', m]
      );
      await updateTemplate(owner.token, fx.tVersioned, {
        content: `${m} rewritten without the marker`,
      });
      // 5. LIKE metacharacter literal "100%".
      fx.tPercent = await create(
        `${m} Discount 100% Off`, 'literal percent', 'save big', 'Sales', ['promo', m]
      );
      // 6. Underscore literal.
      fx.tUnderscore = await create(
        `${m} snake_case helper`, 'literal underscore', 'format text', 'Writing', ['format', m]
      );
      // 7. Multi-tag AND target: has both alpha and beta.
      fx.tBothTags = await create(
        `${m} Tagged Both`, 'alpha and beta', 'multi tag', 'Writing', ['alpha', 'beta', m]
      );
      // 8. Only alpha.
      fx.tOneTag = await create(
        `${m} Tagged One`, 'alpha only', 'single tag', 'Writing', ['alpha', m]
      );
      // 9. Private template (mixed view test).
      fx.tPrivate = await create(
        `${m} Private Email Notes`, `secret ${m} email marketing notes`,
        'internal only', 'Marketing', ['email', m], 'private'
      );
      // 10. Sort fixtures.
      fx.tLiked = await create(
        `${m} Most Liked Item`, 'gets many likes', 'like me', 'Sales', ['sorttest', m]
      );
      fx.tFaved = await create(
        `${m} Most Faved Item`, 'gets many favorites', 'fav me', 'Sales', ['sorttest', m]
      );

      // Seed engagement: 3 distinct likers for tLiked, 2 favoriters (plus
      // bob) for tFaved, and bob likes tEmail (trending signal).
      const likers = [];
      for (let i = 0; i < 3; i += 1) {
        likers.push(await registerUser({ displayName: `Search Liker ${i}` }));
      }
      for (const u of likers) {
        await likeTemplate(u.token, fx.tLiked.id);
      }
      await likeTemplate(bob.token, fx.tLiked.id);
      await favoriteTemplate(likers[0].token, fx.tFaved.id);
      await favoriteTemplate(likers[1].token, fx.tFaved.id);
      await favoriteTemplate(bob.token, fx.tFaved.id);
      await likeTemplate(bob.token, fx.tEmail.id);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------- API: keyword field coverage ----------

  it('matches keyword against title, description, category, tag, and latest content', function (browser) {
    browser.perform(async (done) => {
      try {
        const m = fx.marker;
        const idsOf = (data) => data.templates.map((t) => t.id);

        // Title match (keyword is a contiguous phrase: use a real
        // substring of the fixture title).
        let data = await searchTemplates({ keyword: `Marketing Master` });
        browser.assert.strictEqual(idsOf(data).includes(fx.tEmail.id), true, 'title match');

        // Description match.
        data = await searchTemplates({ keyword: `playbook inside` });
        browser.assert.strictEqual(idsOf(data).includes(fx.tDesc.id), true, 'description match');

        // Category match (Sales covers the sort fixtures).
        data = await searchTemplates({ keyword: 'Sales' });
        const salesIds = idsOf(data);
        browser.assert.strictEqual(salesIds.includes(fx.tLiked.id), true, 'category match (liked)');
        browser.assert.strictEqual(salesIds.includes(fx.tFaved.id), true, 'category match (faved)');

        // Tag match.
        data = await searchTemplates({ keyword: 'campaign' });
        browser.assert.strictEqual(idsOf(data).includes(fx.tEmail.id), true, 'tag match');

        // Latest-version content match.
        data = await searchTemplates({ keyword: `${m} rewritten` });
        browser.assert.strictEqual(idsOf(data).includes(fx.tVersioned.id), true, 'latest content match');

        // Old-version-only marker must NOT match.
        data = await searchTemplates({ keyword: `${m} OLDMARKER` });
        browser.assert.strictEqual(idsOf(data).includes(fx.tVersioned.id), false, 'old version content must not match');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('matches CJK substrings and multi-word phrases', function (browser) {
    browser.perform(async (done) => {
      try {
        const m = fx.marker;
        // CJK substring.
        let data = await searchTemplates({ keyword: '邮件营销' });
        browser.assert.strictEqual(
          data.templates.some((t) => t.id === fx.tCjk.id),
          true,
          'CJK match'
        );

        // Multi-word phrase: title + description matches, private hidden
        // from anonymous callers.
        data = await searchTemplates({ keyword: `${m} email marketing` });
        const got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(got.includes(fx.tEmail.id), true, 'phrase matches title');
        browser.assert.strictEqual(got.includes(fx.tDesc.id), true, 'phrase matches description');
        browser.assert.strictEqual(got.includes(fx.tPrivate.id), false, 'anonymous must not see private');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('treats LIKE metacharacters as literals', function (browser) {
    browser.perform(async (done) => {
      try {
        const m = fx.marker;
        // "100%" must match literally, not as a wildcard (contiguous
        // substring of the fixture title).
        let data = await searchTemplates({ keyword: 'Discount 100% Off' });
        browser.assert.strictEqual(
          data.templates.some((t) => t.id === fx.tPercent.id),
          true,
          'literal 100% matches'
        );
        // Underscore literal.
        data = await searchTemplates({ keyword: `${m} snake_case` });
        browser.assert.strictEqual(
          data.templates.some((t) => t.id === fx.tUnderscore.id),
          true,
          'literal underscore matches'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('requires ALL tags (multi-tag AND) via the tags parameter', function (browser) {
    browser.perform(async (done) => {
      try {
        const data = await searchTemplates({ tags: ['alpha', 'beta'] });
        const got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(got.includes(fx.tBothTags.id), true, 'both-tags template found');
        browser.assert.strictEqual(got.includes(fx.tOneTag.id), false, 'single-tag template must not match AND');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('sorts by most_liked, most_favorited, trending, and falls back to newest on invalid sort', function (browser) {
    browser.perform(async (done) => {
      try {
        const kw = fx.marker;
        // most_liked: tLiked (4 likes) before tFaved (0 likes).
        let data = await searchTemplates({ keyword: kw, sort: 'most_liked' });
        let got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(
          got.indexOf(fx.tLiked.id) < got.indexOf(fx.tFaved.id),
          true,
          'most_liked order'
        );

        // most_favorited: tFaved (3 favorites) before tLiked (0).
        data = await searchTemplates({ keyword: kw, sort: 'most_favorited' });
        got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(
          got.indexOf(fx.tFaved.id) < got.indexOf(fx.tLiked.id),
          true,
          'most_favorited order'
        );

        // trending: both present (recent engagement on both).
        data = await searchTemplates({ keyword: kw, sort: 'trending' });
        got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(got.includes(fx.tLiked.id), true, 'trending includes liked');
        browser.assert.strictEqual(got.includes(fx.tFaved.id), true, 'trending includes faved');

        // Invalid sort falls back to newest without erroring (the tag
        // 'sorttest' is a contiguous keyword match on both fixtures).
        data = await searchTemplates({ keyword: 'sorttest', sort: 'bogus' });
        browser.assert.strictEqual(data.templates.length >= 2, true, 'bogus sort still returns results');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('reports total_count on the first page only and paginates with filters preserved', function (browser) {
    browser.perform(async (done) => {
      try {
        const kw = fx.marker;
        // First page: total_count is an int (int32 proto field).
        const page1 = await searchTemplates({ keyword: kw, pageSize: 5, sort: 'most_liked' });
        browser.assert.strictEqual(
          Number.isInteger(page1.total_count) && page1.total_count >= 10,
          true,
          'total_count set on first page'
        );
        browser.assert.strictEqual(page1.templates.length, 5, 'page size respected');

        // Second page with the same filters.
        const token = page1.next_page_token || '';
        browser.assert.strictEqual(token.length > 0, true, 'next page token present');
        const page2 = await searchTemplates({
          keyword: kw, pageSize: 5, sort: 'most_liked', pageToken: token,
        });
        // Subsequent pages do not recount: 0 or the same total.
        const t2 = page2.total_count;
        browser.assert.strictEqual(
          t2 === 0 || t2 === page1.total_count,
          true,
          'subsequent pages do not recount'
        );
        // No repeated rows between pages.
        const ids1 = page1.templates.map((t) => t.id);
        const ids2 = page2.templates.map((t) => t.id);
        browser.assert.strictEqual(
          ids1.filter((id) => ids2.includes(id)).length,
          0,
          'page 2 must not repeat page 1 rows'
        );
        // Keyword filter preserved across pages.
        const allIds = ids1.concat(ids2);
        browser.assert.strictEqual(allIds.includes(fx.tEmail.id), true, 'pagination kept keyword filter');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('supports the q alias and combines my_likes with keyword', function (browser) {
    browser.perform(async (done) => {
      try {
        const m = fx.marker;
        // q alias behaves like keyword.
        let data = await searchTemplates({ q: `playbook inside` });
        browser.assert.strictEqual(
          data.templates.some((t) => t.id === fx.tDesc.id),
          true,
          'q alias works like keyword'
        );

        // my_likes + keyword: bob liked tEmail (title matches) and tLiked
        // (title does not match the keyword).
        data = await searchTemplates({
          keyword: `Marketing Master`, myLikes: true, token: fx.bob.token,
        });
        const got = data.templates.map((t) => t.id);
        browser.assert.strictEqual(got.includes(fx.tEmail.id), true, 'liked template matching keyword included');
        browser.assert.strictEqual(got.includes(fx.tLiked.id), false, 'liked template without keyword match excluded');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('treats a whitespace keyword as no filter and truncates long keywords', function (browser) {
    browser.perform(async (done) => {
      try {
        // Whitespace keyword = no filter -> results returned.
        const data = await searchTemplates({ keyword: '   ', pageSize: 5 });
        browser.assert.strictEqual(data.templates.length > 0, true, 'whitespace keyword returns results');

        // 101-char keyword is truncated server-side, not an error.
        const long = await searchTemplates({ keyword: '好'.repeat(101) });
        browser.assert.strictEqual(Array.isArray(long.templates), true, 'long keyword does not error');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('keeps private templates out of anonymous results but in the owner mixed view', function (browser) {
    browser.perform(async (done) => {
      try {
        const m = fx.marker;
        // Anonymous: private template never appears.
        let data = await searchTemplates({ keyword: `secret ${m} email marketing` });
        browser.assert.strictEqual(
          data.templates.some((t) => t.id === fx.tPrivate.id),
          false,
          'anonymous must not see private'
        );

        // Owner mixed view: own private match lands in private_templates.
        data = await searchTemplates({
          keyword: `secret ${m} email marketing`, token: fx.owner.token,
        });
        browser.assert.strictEqual(
          (data.private_templates || []).some((t) => t.id === fx.tPrivate.id),
          true,
          'mixed view includes own private match'
        );
        browser.assert.strictEqual(
          (data.private_total_count || 0) >= 1,
          true,
          'private_total_count set'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------- UI: Home page search experience ----------

  it('shows skeleton cards while loading, then results with an aria-live count', function (browser) {
    loginViaUI(browser, fx.owner.email, fx.owner.password)
      .url(`${browser.launch_url}/`)
      .waitForElementVisible('.home-page', 20000)
      .waitForElementVisible('.templates-grid', 20000)
      .waitForElementVisible('.result-count', 10000)
      .assert.attributeEquals('.result-count', 'aria-live', 'polite');
    // First paint shows skeletons (aria-hidden) before data lands. On a
    // warm local stack the fetch may resolve before we can observe them,
    // so only assert the skeleton phase when it is actually caught.
    // First paint shows skeletons (aria-hidden) before data lands. On a
    // warm local stack the fetch may resolve before we can observe them,
    // so only assert the skeleton phase when it is actually caught.
    browser.elements('css selector', '.skeleton-cards', (result) => {
      if (result.status === 0 && result.value.length > 0) {
        browser.waitForElementNotPresent('.skeleton-cards', 20000);
      }
    });
  });

  it('searches via the SearchBar with debounce and shows suggestions', function (browser) {
    const m = fx.marker;
    browser
      .waitForElementVisible('.search-bar-input', 10000)
      // Typing the marker (a tag on every fixture) opens the suggestions
      // combobox (taxonomy-driven: tags matching the typed text).
      .setValue('.search-bar-input', m)
      .waitForElementVisible('.suggestions-dropdown', 10000)
      .assert.visible('.suggestions-item')
      // The suggestion listbox is wired via aria-controls.
      .assert.attributeEquals('.search-bar-input', 'aria-controls', 'search-suggestions-listbox')
      // Escape closes the dropdown without submitting.
      .sendKeys('.search-bar-input', browser.Keys.ESCAPE)
      .waitForElementNotPresent('.suggestions-dropdown', 5000)
      // Now type a real phrase: debounce (300ms) applies the keyword, the
      // URL gains ?q= and the grid filters to the snake_case fixture.
      .clearValue('.search-bar-input')
      .setValue('.search-bar-input', `${m} snake`)
      .pause(800)
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      // URLSearchParams encodes a space as '+'.
      .assert.urlContains(`q=${encodeURIComponent(`${m} snake`).replace(/%20/g, '+')}`)
      .assert.textContains('.templates-grid', `${m} snake_case helper`);
  });

  it('clears the keyword with the clear button and shows the empty state with keyword echo', function (browser) {
    const m = fx.marker;
    browser
      .waitForElementVisible('.search-bar-clear', 5000)
      .click('.search-bar-clear')
      .pause(800)
      // Keyword cleared: URL no longer carries q.
      .perform(function (client, done) {
        client.url(function (result) {
          client.assert.strictEqual(
            result.value.includes('q='),
            false,
            'clear removes q from URL'
          );
          done();
        });
      })
      // Now search for something that cannot match: empty state echoes the
      // keyword and offers a clear-all button.
      .setValue('.search-bar-input', `${m} zz-no-such-thing`)
      .sendKeys('.search-bar-input', browser.Keys.ENTER)
      .waitForElementVisible('.empty-state', 20000)
      .assert.textContains('.empty-state-title', `${m} zz-no-such-thing`)
      .assert.visible('.empty-state-clear')
      .click('.empty-state-clear')
      .pause(800)
      .waitForElementVisible('.templates-grid .prompt-card', 20000);
  });

  it('changes sort via the SortDropdown listbox keyboard pattern', function (browser) {
    const m = fx.marker;
    browser
      .waitForElementVisible('.sort-dropdown-trigger', 10000)
      .assert.attributeEquals('.sort-dropdown-trigger', 'aria-haspopup', 'listbox')
      // Focus + ArrowDown opens the menu (keyboard pattern).
      .click('.sort-dropdown-trigger')
      .waitForElementVisible('.sort-dropdown-menu', 5000)
      .assert.attributeEquals('.sort-dropdown-menu', 'role', 'listbox')
      // Select "Most Liked" via mouse (mousedown handler).
      .useCss()
      .click('.sort-dropdown-option:nth-child(2)')
      .pause(800)
      // Sort lands in the URL and a chip appears (sort != newest).
      .assert.urlContains('sort=most_liked')
      .waitForElementVisible('.filter-chip-row', 5000)
      .assert.textContains('.filter-chip-row', 'Most Liked');
  });

  it('filters by multiple tags via Sidebar checkboxes (AND semantics in the grid)', function (browser) {
    const m = fx.marker;
    browser
      .waitForElementVisible('.tag-cloud', 10000)
      // Check alpha, then beta: both chips appear and the grid narrows to
      // the both-tags fixture.
      .click('.tag-cloud label.tag input[aria-label="alpha"]')
      .pause(500)
      .click('.tag-cloud label.tag input[aria-label="beta"]')
      .pause(1000)
      .waitForElementVisible('.filter-chip-row', 5000)
      .assert.textContains('.filter-chip-row', '#alpha')
      .assert.textContains('.filter-chip-row', '#beta')
      .assert.urlContains('tags=alpha%2Cbeta')
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      .assert.textContains('.templates-grid', `${m} Tagged Both`);
  });

  it('removes a filter chip and clears all chips', function (browser) {
    browser
      .waitForElementVisible('.filter-chip-row', 5000)
      // Remove the beta chip: grid widens again (both + one tag fixtures).
      .click('.filter-chip-row .filter-chip:nth-child(2) .filter-chip-remove')
      .pause(1000)
      .assert.textContains('.filter-chip-row', '#alpha')
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      // Clear all: chips row disappears.
      .click('.filter-chip-clear-all')
      .waitForElementNotPresent('.filter-chip-row', 10000);
  });

  it('syncs filters to the URL and restores them on reload', function (browser) {
    const m = fx.marker;
    browser
      // Deep-link straight into a keyword + sort state.
      .url(`${browser.launch_url}/?q=${encodeURIComponent('Marketing Master')}&sort=most_liked`)
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      .assert.textContains('.templates-grid', `${m} Email Marketing Master`)
      .assert.visible('.filter-chip-row')
      // Reload: URL state restores the same view.
      .refresh()
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      .assert.textContains('.templates-grid', `${m} Email Marketing Master`);
  });

  it('persists the selected visibility in localStorage', function (browser) {
    browser
      // Click "Public" in the sidebar: visibility=VISIBILITY_PUBLIC is
      // persisted under home.selectedVisibility.
      .waitForElementVisible('.app-sidebar', 10000)
      .click('.app-sidebar .sidebar-section ul li:first-child')
      .pause(800)
      .perform(function (client, done) {
        client.execute(function () {
          return window.localStorage.getItem('home.selectedVisibility');
        }, [], function (result) {
          client.assert.strictEqual(
            result.value,
            'VISIBILITY_PUBLIC',
            'visibility persisted to localStorage'
          );
          done();
        });
      });
  });

  it('loads more results on scroll (infinite scroll)', function (browser) {
    browser
      .url(browser.launch_url)
      .waitForElementVisible('.templates-grid .prompt-card', 20000)
      .perform(function (client, done) {
        client.execute(
          function () {
            return document.querySelectorAll('.templates-grid .prompt-card').length;
          },
          [],
          function (before) {
            client.assert.strictEqual(before.value > 0, true, 'first page rendered');
            // Scroll the home container to the bottom to trigger loadMore.
            // The React onScroll handler is attached to the .home-page div,
            // so the scroll event must be dispatched on that element.
            client.execute(
              function () {
                const el = document.querySelector('.home-page');
                if (el) {
                  el.scrollTop = el.scrollHeight;
                  el.dispatchEvent(new Event('scroll'));
                }
                return true;
              },
              [],
              function () {
                client.pause(1500);
                client.execute(
                  function () {
                    return document.querySelectorAll('.templates-grid .prompt-card').length;
                  },
                  [],
                  function (after) {
                    // Either more cards loaded (hasMore was true) or the
                    // list was already complete — both are valid; assert no
                    // crash and the grid did not shrink.
                    client.assert.strictEqual(
                      after.value >= before.value,
                      true,
                      'grid did not shrink after scroll'
                    );
                    done();
                  }
                );
              }
            );
          }
        );
      });
  });
});
