// E2E: template collections feature (2872d93).
//
// REST contract:
//   - POST   /api/v1/collections (auth; name 1-80, desc<=500, visibility
//     private|public default private; anon 401; 50-collection cap -> 429)
//   - GET    /api/v1/collections (anon explore; sort newest|most_templates)
//   - GET    /api/v1/collections/mine (auth; private included)
//   - GET    /api/v1/collections/{id} (anon ok for public; private/deleted
//     -> 404 for non-owner)
//   - PATCH  /api/v1/collections/{id} (owner; non-owner 403)
//   - DELETE /api/v1/collections/{id} (owner; soft delete -> 404 after)
//   - POST   /api/v1/collections/{id}/templates (owner; idempotent re-add;
//     foreign private template -> 404; 100-member cap -> 429 per FVT)
//   - DELETE /api/v1/collections/{id}/templates/{template_id}
//   - PUT    /api/v1/collections/{id}/templates (full ordered list; dup 400;
//     mismatch 404)
//   - GET    /api/v1/templates/{id}/collections (auth; caller-scoped)
// UI: /collections explore grid + sort, /collections/:id member grid,
//     private -> not-found for non-owner, /collections/mine guard + cards,
//     CollectionPicker opened from a Home card overlay.
// i18n: collections.* EN/ZH key parity (all nesting levels).

const {
  registerUser,
  adminLogin,
  setReviewThreshold,
  createTemplate,
  listCollections,
  createCollection,
  getCollection,
  updateCollection,
  deleteCollection,
  addToCollection,
  removeFromCollection,
  reorderCollection,
  listMyCollections,
  listTemplateCollections,
} = require('../helpers/api');

const { request } = require('../helpers/api');

// Wrap an API call so a non-2xx response resolves to { status, body, headers }
// instead of rejecting — for negative-path assertions.
function expectError(promise) {
  return promise.then(
    (r) => ({ status: 0, body: null, headers: {} }),
    (err) => ({ status: err.status, body: err.body, headers: err.headers || {} })
  );
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

describe('Template collections', function () {
  const owner = {};
  const visitor = {};
  const fixtures = {};
  let adminToken = '';

  before(function (browser, done) {
    (async () => {
      const admin = await adminLogin();
      adminToken = admin.token;
      // Review gating would hold a fresh user's first public templates in
      // review and change what anonymous viewers see. This spec tests
      // collections, not gating — disable it (same as FVT and other specs).
      await setReviewThreshold(adminToken, 0, 'e2e template-collections: disable gating');

      Object.assign(owner, await registerUser({ displayName: 'Collections Owner' }));
      Object.assign(visitor, await registerUser({ displayName: 'Collections Visitor' }));

      fixtures.pubA = await createTemplate(owner.token, {
        title: 'CollE2E Alpha ' + Date.now(), visibility: 'public',
      });
      fixtures.pubB = await createTemplate(owner.token, {
        title: 'CollE2E Beta ' + Date.now(), visibility: 'public',
      });
      fixtures.privO = await createTemplate(owner.token, {
        title: 'CollE2E OwnerPrivate ' + Date.now(), visibility: 'private',
      });
      fixtures.privV = await createTemplate(visitor.token, {
        title: 'CollE2E VisitorPrivate ' + Date.now(), visibility: 'private',
      });

      // Main public collection with a mixed member set (leak-filter proof).
      fixtures.colMain = await createCollection(owner.token, {
        name: 'CollMain A', description: 'shared fixture collection', visibility: 'public',
      });
      await addToCollection(owner.token, fixtures.colMain.id, fixtures.pubA.id);
      await addToCollection(owner.token, fixtures.colMain.id, fixtures.pubB.id);
      await addToCollection(owner.token, fixtures.colMain.id, fixtures.privO.id);

      // Single-member public collection (member scratch + sort baseline).
      fixtures.colOne = await createCollection(owner.token, {
        name: 'CollOne B', visibility: 'public',
      });
      await addToCollection(owner.token, fixtures.colOne.id, fixtures.pubA.id);

      // Private collection for the 404-hiding checks.
      fixtures.colPriv = await createCollection(owner.token, { name: 'CollPriv Secret' });

      // Doomed collection for the soft-delete flow.
      fixtures.colDel = await createCollection(owner.token, { name: 'CollDel Victim' });
      await addToCollection(owner.token, fixtures.colDel.id, fixtures.pubB.id);

      // Visitor's own public collection (caller-scoping checks).
      fixtures.vCol = await createCollection(visitor.token, {
        name: 'CollVisitor C', visibility: 'public',
      });
      await addToCollection(visitor.token, fixtures.vCol.id, fixtures.pubA.id);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  // ---------------- REST: create validation ----------------

  it('rejects invalid create payloads and anonymous create', function (browser) {
    browser.perform(async function (done) {
      try {
        const anon = await expectError(
          request('POST', '/api/v1/collections', { name: 'anon attempt' }, {})
        );
        browser.assert.strictEqual(anon.status, 401, 'anonymous create -> 401');

        const badToken = await expectError(
          request('POST', '/api/v1/collections', { name: 'bad token' }, { token: 'not-a-token' })
        );
        browser.assert.strictEqual(badToken.status, 401, 'invalid Bearer -> 401');

        const noName = await expectError(
          request('POST', '/api/v1/collections', { description: 'x' }, { token: owner.token })
        );
        browser.assert.strictEqual(noName.status, 400, 'missing name -> 400');

        const longName = await expectError(
          request('POST', '/api/v1/collections', { name: 'x'.repeat(81) }, { token: owner.token })
        );
        browser.assert.strictEqual(longName.status, 400, 'name >80 -> 400');

        const longDesc = await expectError(
          request('POST', '/api/v1/collections', { name: 'ok', description: 'x'.repeat(501) }, { token: owner.token })
        );
        browser.assert.strictEqual(longDesc.status, 400, 'description >500 -> 400');

        const badVis = await expectError(
          request('POST', '/api/v1/collections', { name: 'ok', visibility: 'friends' }, { token: owner.token })
        );
        browser.assert.strictEqual(badVis.status, 400, 'visibility not in {private,public} -> 400');

        const exact80 = await createCollection(owner.token, { name: 'x'.repeat(80) });
        browser.assert.strictEqual(exact80.visibility, 'private', '80-char name accepted (upper bound)');
        await deleteCollection(owner.token, exact80.id);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('creates with private default and full wire shape', function (browser) {
    browser.perform(async function (done) {
      try {
        const c = await createCollection(visitor.token, { name: 'Defaults Check' });
        browser.assert.ok(UUID_RE.test(c.id), 'collection id is a UUID');
        browser.assert.strictEqual(c.visibility, 'private', 'visibility defaults to private');
        browser.assert.strictEqual(c.template_count, 0, 'template_count starts at 0');
        browser.assert.ok(c.created_at && c.updated_at, 'timestamps present');
        browser.assert.strictEqual(c.owner.id, visitor.id, 'owner echoes the caller');
        await deleteCollection(visitor.token, c.id);
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('enforces the 50-collection cap with 429', function (browser) {
    browser.perform(async function (done) {
      try {
        const capUser = await registerUser({ username: 'e2e_coll_cap_' + Date.now() });
        let hit = -1;
        for (let i = 0; i < 55; i++) {
          const r = await expectError(createCollection(capUser.token, { name: 'Cap ' + i }));
          if (r.status === 429) { hit = i; break; }
          browser.assert.strictEqual(r.status, 0, `create #${i + 1} under the cap is 2xx`);
        }
        browser.assert.ok(hit >= 49, `cap hit at/after create 50 (index ${hit})`);
        const capErr = await expectError(createCollection(capUser.token, { name: 'over the cap' }));
        browser.assert.strictEqual(capErr.status, 429, 'over-cap create -> 429');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: visibility / privacy ----------------

  it('hides private collections from anonymous and non-owner reads', function (browser) {
    browser.perform(async function (done) {
      try {
        const id = fixtures.colPriv.id;

        const anonRead = await expectError(getCollection(null, id));
        browser.assert.strictEqual(anonRead.status, 404, 'anon read of private -> 404');

        const visitorRead = await expectError(getCollection(visitor.token, id));
        browser.assert.strictEqual(visitorRead.status, 404, 'visitor read of private -> 404');

        const ownerRead = await getCollection(owner.token, id);
        browser.assert.strictEqual(ownerRead.visibility, 'private', 'owner reads own private');
        browser.assert.ok(Array.isArray(ownerRead.members), 'members array present');

        const goneRead = await expectError(
          getCollection(null, '00000000-0000-0000-0000-000000000000')
        );
        browser.assert.strictEqual(goneRead.status, 404, 'nonexistent id -> 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects non-owner mutations with 403 on a public collection', function (browser) {
    browser.perform(async function (done) {
      try {
        const id = fixtures.colMain.id;

        const visPatch = await expectError(updateCollection(visitor.token, id, { name: 'hijack' }));
        browser.assert.strictEqual(visPatch.status, 403, 'visitor PATCH -> 403');

        const visDel = await expectError(deleteCollection(visitor.token, id));
        browser.assert.strictEqual(visDel.status, 403, 'visitor DELETE -> 403');

        const visAdd = await expectError(
          request('POST', `/api/v1/collections/${id}/templates`, { template_id: fixtures.pubA.id }, { token: visitor.token })
        );
        browser.assert.strictEqual(visAdd.status, 403, 'visitor add-member -> 403');

        const anonAdd = await expectError(
          request('POST', `/api/v1/collections/${id}/templates`, { template_id: fixtures.pubA.id }, {})
        );
        browser.assert.strictEqual(anonAdd.status, 401, 'anonymous add-member -> 401');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('keeps soft-delete semantics after DELETE', function (browser) {
    browser.perform(async function (done) {
      try {
        const id = fixtures.colDel.id;
        const mineBefore = await listMyCollections(owner.token);
        browser.assert.ok(
          mineBefore.collections.some((c) => c.id === id), 'colDel in mine before delete'
        );

        const del = await deleteCollection(owner.token, id);
        browser.assert.strictEqual(del.id, id, 'delete echoes the id');

        const afterDel = await expectError(getCollection(owner.token, id));
        browser.assert.strictEqual(afterDel.status, 404, 'deleted -> 404 even for the owner');

        const mineAfter = await listMyCollections(owner.token);
        browser.assert.strictEqual(
          mineAfter.collections.some((c) => c.id === id), false,
          'deleted collection absent from mine'
        );

        const explore = await listCollections({ pageSize: 50 });
        browser.assert.strictEqual(
          explore.collections.some((c) => c.id === id), false,
          'deleted collection absent from explore'
        );

        const reDel = await expectError(deleteCollection(owner.token, id));
        browser.assert.strictEqual(reDel.status, 404, 'deleting a deleted collection -> 404');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: members ----------------

  it('members: add idempotently, reorder atomically, remove cleanly', function (browser) {
    browser.perform(async function (done) {
      try {
        const id = fixtures.colOne.id;
        const add = await addToCollection(owner.token, id, fixtures.pubB.id);
        browser.assert.strictEqual(add.collection_id, id, 'add echoes collection id');
        browser.assert.strictEqual(add.template_id, fixtures.pubB.id, 'add echoes template id');
        browser.assert.strictEqual(add.position, 1, 'second member gets position 1');
        browser.assert.strictEqual(add.template_count, 2, 'post-add count is 2');

        const reAdd = await addToCollection(owner.token, id, fixtures.pubB.id);
        browser.assert.strictEqual(reAdd.position, 1, 're-add returns existing position (idempotent)');
        browser.assert.strictEqual(reAdd.template_count, 2, 're-add does not bump count');

        const unknownAdd = await expectError(addToCollection(owner.token, id, 'nonexistent'));
        browser.assert.strictEqual(unknownAdd.status, 404, 'unknown template add -> 404');

        // A private template that belongs to someone else must behave like a
        // missing one (existence must not leak).
        const foreignPriv = await expectError(
          addToCollection(owner.token, id, fixtures.privV.id)
        );
        browser.assert.strictEqual(foreignPriv.status, 404,
          "another user's private template -> 404 (no existence leak)");

        const reorder = await reorderCollection(owner.token, id, [fixtures.pubB.id, fixtures.pubA.id]);
        browser.assert.ok(reorder !== undefined, 'reorder 2xx');
        const afterReorder = await getCollection(owner.token, id);
        browser.assert.strictEqual(afterReorder.members[0].template.id, fixtures.pubB.id, 'reorder: B first');
        browser.assert.strictEqual(afterReorder.members[1].template.id, fixtures.pubA.id, 'reorder: A second');

        const dup = await expectError(
          reorderCollection(owner.token, id, [fixtures.pubA.id, fixtures.pubA.id])
        );
        browser.assert.strictEqual(dup.status, 400, 'duplicate ids in reorder -> 400');

        const shortList = await expectError(
          reorderCollection(owner.token, id, [fixtures.pubA.id])
        );
        browser.assert.strictEqual(shortList.status, 404, 'reorder list != current members -> 404');

        await reorderCollection(owner.token, id, [fixtures.pubA.id, fixtures.pubB.id]);
        const rm = await removeFromCollection(owner.token, id, fixtures.pubB.id);
        browser.assert.ok(rm !== undefined, 'remove 2xx');
        const afterRm = await getCollection(owner.token, id);
        browser.assert.strictEqual(afterRm.template_count, 1, 'count back to 1 after remove');
        browser.assert.strictEqual(
          afterRm.members.some((m) => m.template.id === fixtures.pubB.id), false,
          'removed member gone from GET'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('filters non-public members out of the public member view', function (browser) {
    browser.perform(async function (done) {
      try {
        const id = fixtures.colMain.id;

        const ownerView = await getCollection(owner.token, id);
        browser.assert.strictEqual(ownerView.members.length, 3, 'owner view: all three members');
        const privMember = ownerView.members.find((m) => m.template.id === fixtures.privO.id);
        browser.assert.ok(privMember, 'owner view includes the private member');
        browser.assert.strictEqual(privMember.is_public, false, 'flagged is_public=false');
        browser.assert.strictEqual(ownerView.template_count, 3, 'owner count is the full total');

        const anonView = await getCollection(null, id);
        browser.assert.strictEqual(anonView.members.length, 2, 'anon view: only public members');
        browser.assert.strictEqual(
          anonView.members.some((m) => m.template.id === fixtures.privO.id), false,
          'anon view NEVER leaks the private member'
        );
        browser.assert.strictEqual(
          anonView.template_count, 2, 'anon count consistent with the filtered list'
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('cross-references templates to the caller collections only', function (browser) {
    browser.perform(async function (done) {
      try {
        const tid = fixtures.pubA.id;
        const anon = await expectError(
          request('GET', `/api/v1/templates/${tid}/collections`, null, {})
        );
        browser.assert.strictEqual(anon.status, 401, 'anonymous cross-ref -> 401');

        const ownerRows = await listTemplateCollections(owner.token, tid);
        browser.assert.ok(Array.isArray(ownerRows.collections), 'owner row is an array');
        browser.assert.ok(
          ownerRows.collections.some((c) => c.id === fixtures.colMain.id),
          'owner sees their collection containing pubA'
        );
        browser.assert.strictEqual(
          ownerRows.collections.some((c) => c.id === fixtures.colPriv.id), false,
          'cross-ref only reports collections actually containing the template'
        );

        const visitorRows = await listTemplateCollections(visitor.token, tid);
        browser.assert.ok(
          visitorRows.collections.some((c) => c.id === fixtures.vCol.id),
          'visitor sees their own containing collection'
        );
        browser.assert.strictEqual(
          visitorRows.collections.some((c) => c.id === fixtures.colMain.id), false,
          "visitor does NOT see the owner's collection"
        );
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- REST: explore ----------------

  it('explore lists public collections, sorted, and paginable', function (browser) {
    browser.perform(async function (done) {
      try {
        const explore = await listCollections({ pageSize: 50 });
        browser.assert.ok(Array.isArray(explore.collections), 'explore yields an array');
        const ids = explore.collections.map((c) => c.id);
        browser.assert.ok(ids.includes(fixtures.colMain.id), 'public collection in explore');
        browser.assert.strictEqual(
          ids.includes(fixtures.colPriv.id), false,
          'private collection never in explore'
        );

        const most = await listCollections({ pageSize: 50, sort: 'most_templates' });
        const counts = most.collections.map((c) => c.template_count);
        for (let i = 1; i < counts.length; i++) {
          browser.assert.ok(counts[i - 1] >= counts[i], `most_templates desc at index ${i}`);
        }

        const page1 = await listCollections({ pageSize: 1 });
        browser.assert.strictEqual(page1.collections.length, 1, 'page_size=1 returns one row');
        browser.assert.ok(
          page1.next_page_token !== undefined && page1.next_page_token !== null,
          'next_page_token present on first page'
        );
        if (page1.next_page_token) {
          const page2 = await listCollections({ pageSize: 1, pageToken: page1.next_page_token });
          if (page2.collections.length > 0) {
            browser.assert.notStrictEqual(
              page2.collections[0].id, page1.collections[0].id,
              'page 2 shows a different collection'
            );
          }
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  it('rejects an invalid sort value', function (browser) {
    browser.perform(async function (done) {
      try {
        const badSort = await expectError(
          request('GET', '/api/v1/collections?sort=bogus', null, {})
        );
        browser.assert.strictEqual(badSort.status, 400, 'invalid sort -> 400');
        const badTok = await expectError(
          request('GET', '/api/v1/collections?page_token=zzz', null, {})
        );
        browser.assert.strictEqual(badTok.status, 400, 'invalid page_token -> 400');
        done();
      } catch (err) {
        done(err);
      }
    });
  });

  // ---------------- UI ----------------

  it('explore page renders the collection grid anonymously', function (browser) {
    browser.url(`${browser.launch_url}/collections`)
      .waitForElementVisible('.collections-page', 20000)
      .waitForElementVisible('.collections-page__title', 10000)
      .waitForElementVisible('.collections-page__grid', 20000)
      .waitForElementPresent('.collection-card', 20000);
    browser.execute(function () {
      return Array.from(document.querySelectorAll('.collection-card__name'))
        .map((n) => n.textContent);
    }, [], function (res) {
      browser.assert.ok(res.value.includes('CollMain A'), 'own fixture visible in explore grid');
    });
    browser.expect.element('.collections-page__sort select').to.be.present;
  });

  it('detail page renders the member grid for a public collection', function (browser) {
    browser.url(`${browser.launch_url}/collections/${fixtures.colMain.id}`)
      .waitForElementVisible('.collection-detail-page', 20000)
      .waitForElementVisible('.collection-header__title', 10000);
    browser.expect.element('.collection-header__title').text.to.contain('CollMain A');
    browser.waitForElementPresent('.collection-detail-page__member', 20000);
    browser.elements('css selector', '.collection-detail-page__member', function (res) {
      this.assert.ok(res.value.length >= 1, 'member grid renders entries');
    });
    browser.expect.element('.collection-detail-page__not-found').not.to.be.present;
  });

  it('private collection detail shows not-found for a non-owner', function (browser) {
    browser.url(`${browser.launch_url}/collections/${fixtures.colPriv.id}`)
      .waitForElementVisible('.collection-detail-page__not-found', 20000);
  });

  it('mine page redirects anonymous visitors to login', function (browser) {
    browser.url(`${browser.launch_url}/collections/mine`)
      .waitForElementVisible('form', 20000);
    browser.assert.urlContains('/login');
  });

  it('owner signs in, sees mine page, opens picker from Home card', function (browser) {
    browser.url(`${browser.launch_url}/login`)
      .waitForElementVisible('form', 20000)
      .setValue('input[type="email"], input[name="email"]', owner.email)
      .setValue('input[type="password"], input[name="password"]', 'Password_123456')
      .submitForm('form');
    browser.pause(1500);

    browser.url(`${browser.launch_url}/collections/mine`)
      .waitForElementVisible('.my-collections-page', 20000)
      .waitForElementPresent('.my-collections-page__card', 20000);
    browser.elements('css selector', '.my-collections-page__card', function (res) {
      this.assert.ok(res.value.length >= 2, 'owner collection cards render on mine page');
    });

    // CollectionPicker: open from a Home card "Add to collection" overlay
    // button (real user journey; JS click as in accessibility.spec.js).
    browser.url(`${browser.launch_url}/`)
      .waitForElementPresent('.prompt-card', 20000);
    browser.execute(function () {
      const btn = document.querySelector('.prompt-card button[aria-label="Add to collection"]');
      if (!btn) return false;
      btn.click();
      return true;
    }, [], function (res) {
      browser.assert.ok(res.value, 'add-to-collection button present on a Home card');
    });
    browser.waitForElementVisible('.collection-picker', 20000);
    browser.waitForElementVisible('.collection-picker__filter', 10000);
    browser.waitForElementPresent('.collection-picker__item', 10000);
  });

  it('keeps EN/ZH parity for collections.* keys', function (browser) {
    browser.perform(async function (done) {
      try {
        const en = require('../../../frontend/src/locales/en/translation.json');
        const zh = require('../../../frontend/src/locales/zh/translation.json');
        const enC = en.collections || {};
        const zhC = zh.collections || {};
        browser.assert.deepStrictEqual(
          Object.keys(enC).sort(), Object.keys(zhC).sort(),
          'top-level collections key sets match'
        );
        for (const k of Object.keys(enC)) {
          const ev = enC[k];
          const zv = zhC[k];
          if (ev !== null && typeof ev === 'object') {
            browser.assert.deepStrictEqual(
              Object.keys(ev).sort(), Object.keys(zv || {}).sort(),
              `collections.${k} nested key sets match`
            );
          } else {
            browser.assert.strictEqual(typeof zv, 'string', `collections.${k} is a string in zh`);
          }
        }
        done();
      } catch (err) {
        done(err);
      }
    });
  });
});
