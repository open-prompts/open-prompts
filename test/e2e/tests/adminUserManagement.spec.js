// E2E: admin user management console.
// Covers: route guard, login (invalid / non-admin / valid), dashboard KPIs,
// user list + search/status/role filters, user detail, ban/unban with the
// mandatory reason modal, role change, and the audit log view.
const { registerUser, adminLogin } = require('../helpers/api');

const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'Admin_123456';

// Fresh browser session: clear storage so stale sessions never leak between
// tests, and pin the UI language to English for text assertions.
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

describe('Admin user management', function () {
  // Shared fixtures created once for the whole suite.
  const victim = {};

  before(function (browser, done) {
    (async () => {
      const v = await registerUser({ displayName: 'Admin E2E Victim' });
      Object.assign(victim, v);
      done();
    })().catch(done);
  });

  after(function (browser) {
    browser.end();
  });

  it('redirects unauthenticated visitors to the admin login', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/users`)
      .pause(500)
      .waitForElementVisible('.admin-login', 15000)
      .assert.urlContains('/admin/login')
      .assert.urlContains('redirect=');
  });

  it('shows an error for invalid admin credentials', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', 'admin@example.com')
      .setValue('#admin-password', 'wrong-password')
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-login__error', 10000)
      .assert.textContains('.admin-login__error', 'Invalid email or password.');
  });

  it('rejects a regular user account', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', victim.email)
      .setValue('#admin-password', victim.password)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-login__error', 10000)
      .assert.textContains('.admin-login__error', 'This account does not have admin access.');
  });

  it('signs the admin in and shows the dashboard KPIs', function (browser) {
    freshSession(browser, `${browser.launch_url}/admin/login`)
      .waitForElementVisible('#admin-email', 15000)
      .setValue('#admin-email', ADMIN_EMAIL)
      .setValue('#admin-password', ADMIN_PASSWORD)
      .click('.admin-login__submit')
      .waitForElementVisible('.admin-kpi', 20000)
      .assert.urlContains('/admin')
      .assert.textContains('.admin-dashboard__grid', 'Total Users')
      .assert.textContains('.admin-dashboard__grid', 'Banned Users');
  });

  it('lists users and filters by search, status and role', function (browser) {
    browser
      .url(`${browser.launch_url}/admin/users`)
      .waitForElementVisible('.admin-table tbody tr', 20000)
      .assert.elementPresent('.admin-users__filters');

    // Search by the victim's unique email (300ms debounce in the UI).
    browser
      .clearValue('.admin-search input')
      .setValue('.admin-search input', victim.email)
      .pause(1200)
      .waitForElementVisible('.admin-table tbody tr', 15000)
      .assert.textContains('.admin-table tbody', victim.email);

    // Status filter: the active victim must not appear among banned users.
    // React-controlled selects need a native change event, so drive them via JS.
    const setSelect = (label, value) => browser.execute(
      function (label, value) {
        const el = document.querySelector(`select[aria-label="${label}"]`);
        const setter = Object.getOwnPropertyDescriptor(
          window.HTMLSelectElement.prototype, 'value'
        ).set;
        setter.call(el, value);
        el.dispatchEvent(new Event('change', { bubbles: true }));
      },
      [label, value]
    );

    setSelect('Status', 'banned');
    browser
      .pause(1200)
      .waitForElementVisible('.admin-empty__title', 15000)
      .assert.textContains('.admin-empty__title', 'No users match your filters')
      .assert.visible('.admin-empty .admin-btn--ghost');

    // Clear filters via the empty-state button, then filter by role: the
    // victim is a regular user and must not appear in the admins-only list.
    browser
      .click('.admin-empty .admin-btn--ghost')
      .pause(1200)
      .waitForElementVisible('.admin-table tbody tr', 15000);

    setSelect('Role', 'admin');
    browser
      .pause(1200)
      .waitForElementVisible('.admin-table tbody tr', 15000)
      .assert.not.textContains('.admin-table tbody', victim.email);
  });

  it('opens the user detail page from the list', function (browser) {
    browser
      .url(`${browser.launch_url}/admin/users`)
      .waitForElementVisible('.admin-table tbody tr', 20000)
      .useXpath()
      .waitForElementVisible(`//tr[contains(., "${victim.email}")]`, 15000)
      .useCss();

    // geckodriver's interactability check fails inside the overflow-x table
    // wrapper, so trigger the row's native DOM click instead.
    browser
      .execute(function (email) {
        const rows = Array.from(document.querySelectorAll('tbody tr'));
        const row = rows.find((r) => r.textContent.includes(email));
        if (row) row.click();
        return !!row;
      }, [victim.email])
      .waitForElementVisible('.admin-user-detail__header', 20000)
      .assert.textContains('.admin-user-detail__identity', victim.email)
      .assert.textContains('.admin-user-detail__badges', 'Active');
  });

  it('bans a user with a reason via the modal', function (browser) {
    browser
      .waitForElementVisible('.admin-user-detail__actions .admin-btn--danger', 10000)
      .click('.admin-user-detail__actions .admin-btn--danger')
      .waitForElementVisible('.modal-overlay', 10000)
      .assert.visible('#admin-reason-input');

    // A too-short reason must keep the confirm button disabled.
    browser
      .setValue('#admin-reason-input', 'no')
      .pause(300)
      .assert.attributeEquals('.modal-footer .btn-danger', 'disabled', 'true');

    browser
      .clearValue('#admin-reason-input')
      .setValue('#admin-reason-input', 'e2e ban reason')
      .pause(300)
      .click('.modal-footer .btn-danger')
      .waitForElementNotPresent('.modal-overlay', 15000)
      .waitForElementVisible('.admin-user-detail__badges', 15000)
      .assert.textContains('.admin-user-detail__badges', 'Banned')
      .assert.textContains('.admin-user-detail__history', 'e2e ban reason');
  });

  it('unbans the user with a reason', function (browser) {
    browser
      .waitForElementVisible('.admin-user-detail__actions .admin-btn--primary', 10000)
      .click('.admin-user-detail__actions .admin-btn--primary')
      .waitForElementVisible('.modal-overlay', 10000)
      .setValue('#admin-reason-input', 'e2e unban reason')
      .pause(300)
      .click('.modal-footer .btn-primary')
      .waitForElementNotPresent('.modal-overlay', 15000)
      .waitForElementVisible('.admin-user-detail__badges', 15000)
      .assert.textContains('.admin-user-detail__badges', 'Active')
      .assert.textContains('.admin-user-detail__history', 'e2e unban reason');
  });

  it('promotes the user to admin and back', function (browser) {
    browser
      .waitForElementVisible('.admin-user-detail__actions .admin-btn--ghost', 10000)
      .click('.admin-user-detail__actions .admin-btn--ghost')
      .waitForElementVisible('.modal-overlay', 10000)
      .setValue('#admin-reason-input', 'e2e promote reason')
      .pause(300)
      .click('.modal-footer .btn-primary')
      .waitForElementNotPresent('.modal-overlay', 15000)
      .waitForElementVisible('.admin-user-detail__badges', 15000)
      .assert.textContains('.admin-user-detail__badges', 'Admin');

    // Demote back to regular user. Wait out the promote success toast (3s)
    // so it cannot obscure the button.
    browser
      .waitForElementVisible('.admin-user-detail__actions .admin-btn--ghost', 10000)
      .pause(3200)
      .click('.admin-user-detail__actions .admin-btn--ghost')
      .waitForElementVisible('.modal-overlay', 10000)
      .setValue('#admin-reason-input', 'e2e demote reason')
      .pause(300)
      .click('.modal-footer .btn-primary')
      .waitForElementNotPresent('.modal-overlay', 15000)
      .waitForElementVisible('.admin-user-detail__badges', 15000)
      .assert.textContains('.admin-user-detail__badges', 'User');
  });

  it('shows the ban/unban/role actions in the audit log', function (browser) {
    browser
      .url(`${browser.launch_url}/admin/audit-log`)
      .waitForElementVisible('.admin-audit__filters', 20000)
      .waitForElementVisible('.admin-table tbody tr', 20000)
      .assert.textContains('.admin-page-title', 'Audit Log');

    // The victim's id should appear as a target link for our actions.
    browser
      .useXpath()
      .waitForElementVisible(`//button[contains(@class, "admin-audit__target-link")][contains(., "${victim.id}")]`, 15000)
      .useCss();

    // Action filter narrows to ban entries only.
    browser
      .setValue('select[aria-label="Action"]', 'ban')
      .pause(1000)
      .waitForElementVisible('.admin-table tbody tr', 15000)
      .assert.textContains('.admin-table tbody', 'Ban')
      .assert.not.textContains('.admin-table tbody', 'Role change');
  });
});
