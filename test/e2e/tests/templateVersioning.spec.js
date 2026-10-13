// E2E: template versioning feature.
// Covers: version history panel (newest first, Latest badge, change notes,
// usage meta), row selection, compare diff modal (stats, swap, copy),
// owner-only restore flow with confirm modal and toast, owner gating for
// non-owners, change note input in edit mode, and EN/ZH i18n.
const { registerUser, createTemplate, updateTemplate } = require('../helpers/api');

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
    // Wait for the login POST to finish and the app to land on the home
    // page before navigating further; issuing .url() while the request is
    // still in flight aborts it and leaves the session without a token.
    .waitForElementVisible('.App', 15000)
    .waitForElementNotPresent('#email', 10000);
}

function openTemplate(browser, templateId) {
  return browser
    .url(`${browser.launch_url}/templates/${templateId}`)
    .waitForElementVisible('.template-details-page', 20000);
}

describe('Template versioning', function () {
  const owner = {};
  const nonOwner = {};
  const template = {};

  before(function (browser, done) {
    (async () => {
      Object.assign(owner, await registerUser({ displayName: 'Versioning Owner' }));
      Object.assign(nonOwner, await registerUser({ displayName: 'Versioning NonOwner' }));
      const t = await createTemplate(owner.token, {
        title: 'Versioning E2E Template',
        content: 'You are a helpful assistant.\nWrite a haiku about {{topic}}.',
      });
      Object.assign(template, t);
      // Two updates -> three versions total (v1, v2, v3).
      await updateTemplate(owner.token, t, {
        content: 'You are an expert copywriter.\nWrite a haiku about {{topic}}.\nKeep it under 20 words.',
        changeNote: 'Second revision',
      });
      await updateTemplate(owner.token, t, {
        content: 'You are an expert copywriter.\nWrite a sonnet about {{topic}}.',
        changeNote: 'Third revision',
      });
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  it('opens the history panel with versions newest first and Latest badge', function (browser) {
    loginViaUI(browser, owner.email, owner.password);
    openTemplate(browser, template.id)
      .waitForElementVisible('.history-toggle', 10000)
      .assert.textContains('.history-toggle', 'History')
      .click('.history-toggle')
      .waitForElementVisible('.version-history-panel', 10000)
      .assert.textContains('#version-history-title', 'Version history');

    // Newest first: v3, v2, v1. Latest badge only on the first row.
    browser
      .waitForElementVisible('.version-history-panel__list .version-row', 10000)
      .assert.textContains('.version-history-panel__list', 'v3')
      .assert.textContains('.version-history-panel__list', 'v2')
      .assert.textContains('.version-history-panel__list', 'v1')
      .assert.textContains('.version-history-panel__list', 'Latest')
      .assert.textContains('.version-history-panel__list', 'Second revision')
      .assert.textContains('.version-history-panel__list', 'Third revision');

    const badgeCount = {};
    browser.execute(
      function () { return document.querySelectorAll('.version-history-panel__list .version-row__latest-badge').length; },
      [],
      (result) => {
        browser.assert.strictEqual(result.value, 1, 'exactly one Latest badge is rendered');
      }
    );

    // Usage meta shows chars/lines/prompts for each row.
    browser.assert.textContains('.version-history-panel__list', 'prompts');
  });

  it('selects an older version row and updates the version select and preview', function (browser) {
    browser
      .useXpath()
      .click('//ul[contains(@class,"version-history-panel__list")]//button[@aria-label="v1"]')
      .useCss()
      .pause(500);

    // The selected row gets the .selected class; the version select label
    // shows v1 and the preview shows the v1 content.
    browser
      .assert.visible('.version-row.selected')
      .assert.textContains('.version-row.selected .version-row__number', 'v1')
      .assert.textContains('.custom-select__label', 'v1')
      .assert.textContains('.preview-area', 'helpful assistant');

    // Selecting back the latest version restores its content in the preview.
    browser
      .useXpath()
      .click('//ul[contains(@class,"version-history-panel__list")]//button[@aria-label="v3"]')
      .useCss()
      .pause(500)
      .assert.textContains('.preview-area', 'sonnet');
  });

  it('compares two versions in the diff modal with stats, swap and copy', function (browser) {
    browser
      .useXpath()
      .click('//ul[contains(@class,"version-history-panel__list")]//button[@aria-label="Compare v1"]')
      .useCss()
      .waitForElementVisible('.modal-overlay', 10000)
      .assert.textContains('.modal-title', 'Compare versions')
      .assert.visible('[data-testid="diff-stats"]');

    // v1 -> v3: "helpful assistant" removed, "sonnet" added, "Keep it under
    // 20 words." removed. Stats text uses the U+2212 minus sign.
    browser
      .assert.textContains('[data-testid="diff-stats"]', '+2')
      .assert.textContains('[data-testid="diff-stats"]', '\u22122')
      .assert.visible('.diff-line--remove')
      .assert.visible('.diff-line--add');

    // Swap flips the A/B selects (labels show v{n}).
    browser.execute(
      function () {
        return Array.from(document.querySelectorAll('.diff-modal__selector .custom-select__label')).map((el) => el.textContent.trim());
      },
      [],
      (result) => {
        const before = result.value;
        browser
          .click('.diff-modal__swap')
          .pause(500)
          .execute(
            function () {
              return Array.from(document.querySelectorAll('.diff-modal__selector .custom-select__label')).map((el) => el.textContent.trim());
            },
            [],
            (after) => {
              browser.assert.deepStrictEqual(
                after.value,
                [before[1], before[0]],
                'swap exchanges the A and B version selects'
              );
            }
          );
      }
    );

    // Copy diff triggers the copied toast.
    browser
      .useXpath()
      .click('//div[contains(@class,"diff-modal__footer")]//button[contains(., "Copy diff")]')
      .useCss()
      .waitForElementVisible('.notification-toast.success', 5000)
      .assert.textContains('.notification-toast', 'Diff copied to clipboard');

    // Close via the footer Close button.
    browser
      .useXpath()
      .click('//div[contains(@class,"diff-modal__footer")]//button[contains(., "Close")]')
      .useCss()
      .waitForElementNotPresent('.modal-overlay', 10000);
  });

  it('restores an old version as a new version with a toast (owner)', function (browser) {
    browser
      .useXpath()
      .click('//ul[contains(@class,"version-history-panel__list")]//button[@aria-label="Restore v1"]')
      .useCss()
      .waitForElementVisible('.modal-overlay', 10000)
      .assert.textContains('.modal-title', 'Restore version')
      .assert.textContains('.modal-confirm-text', 'as a new version (v4)');

    browser
      // Dismiss any toast left over from the previous test (e.g. the diff
      // copy toast) so the wait below observes the restore toast, not a
      // stale one.
      .waitForElementNotPresent('.notification-toast', 5000)
      .click('.modal-footer .btn-primary')
      // The toast auto-hides after 3s; capture its text immediately via
      // execute (a chained textContains can race with the auto-dismiss).
      .waitForElementVisible('.notification-toast.success', 10000)
      .execute(function () {
        var el = document.querySelector('.notification-toast');
        return el ? el.innerText : '';
      }, [], function (result) {
        browser.assert.ok(
          String(result.value).indexOf('Version restored as v4') !== -1,
          'toast shows "Version restored as v4"'
        );
      });

    // The version list refreshes with the new v4 carrying the restore note.
    browser
      .waitForElementVisible('.version-history-panel__list', 10000)
      .pause(1000)
      .assert.textContains('.version-history-panel__list', 'v4')
      .assert.textContains('.version-history-panel__list', 'Restored from v1');

    // The restored content becomes the selected version.
    browser
      .assert.textContains('.custom-select__label', 'v4')
      .assert.textContains('.preview-area', 'helpful assistant');
  });

  it('hides restore and edit controls from a non-owner', function (browser) {
    loginViaUI(browser, nonOwner.email, nonOwner.password);
    openTemplate(browser, template.id)
      .waitForElementVisible('.history-toggle', 10000)
      .click('.history-toggle')
      .waitForElementVisible('.version-history-panel', 10000)
      .waitForElementVisible('.version-history-panel__list .version-row', 10000);

    // History and compare are available, restore and edit are not.
    browser
      .assert.visible('button[aria-label="Compare v1"]')
      .assert.elementNotPresent('.version-row__restore')
      .assert.elementNotPresent('.edit-btn');
  });

  it('saves a new version with a change note from edit mode (owner)', function (browser) {
    loginViaUI(browser, owner.email, owner.password);
    openTemplate(browser, template.id)
      .waitForElementVisible('.edit-btn', 10000)
      .click('.edit-btn')
      .waitForElementVisible('#change-note', 10000)
      .assert.textContains('.change-note-counter', '0/200');

    // Save is disabled until the content differs from the selected version.
    browser.assert.attributeEquals('.save-btn', 'disabled', 'true');

    browser
      .setValue('#change-note', 'Fourth revision via UI')
      .setValue('.template-content textarea', 'You are a helpful assistant.\nWrite a limerick about {{topic}}.')
      .pause(300)
      .click('.save-btn')
      .waitForElementVisible('.notification-toast.success', 10000)
      .assert.textContains('.notification-toast', 'Template updated and new version created!');

    // The new version appears in the history with its note.
    browser
      .click('.history-toggle')
      .waitForElementVisible('.version-history-panel__list', 10000)
      .pause(1000)
      .assert.textContains('.version-history-panel__list', 'v5')
      .assert.textContains('.version-history-panel__list', 'Fourth revision via UI');
  });

  it('translates all versioning strings when switching to Chinese', function (browser) {
    loginViaUI(browser, owner.email, owner.password);
    openTemplate(browser, template.id)
      .waitForElementVisible('.history-toggle', 10000)
      .assert.textContains('.history-toggle', 'History');

    browser
      .click('.lang-switch-btn')
      .pause(800)
      .assert.textContains('.history-toggle', '历史')
      .click('.history-toggle')
      .waitForElementVisible('.version-history-panel', 10000)
      .assert.textContains('#version-history-title', '版本历史')
      .assert.textContains('.version-history-panel__list', '最新')
      .assert.visible('button[aria-label="比较 v1"]')
      .assert.visible('button[aria-label="恢复 v1"]');

    // Restore confirm modal in Chinese.
    browser
      .click('button[aria-label="恢复 v1"]')
      .waitForElementVisible('.modal-overlay', 10000)
      .assert.textContains('.modal-title', '恢复版本')
      .click('.modal-footer .btn-ghost') // Cancel
      .waitForElementNotPresent('.modal-overlay', 10000);

    // Switch back to English for any later runs in this session.
    browser
      .click('.lang-switch-btn')
      .pause(800)
      // The panel is open, so the toggle reads "Hide history" (lowercase h).
      .assert.textContains('.history-toggle', 'Hide history');
  });
});
