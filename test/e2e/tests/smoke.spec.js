// Smoke test: the open-prompts frontend loads and renders.
// Verifies the local stack (http://localhost:3000) is reachable and the React
// app boots (title + root content present).
describe('Smoke - frontend loads', function () {
  it('opens the home page and renders the app', function (browser) {
    browser
      .url(browser.launch_url)
      .waitForElementVisible('body', 15000)
      .assert.titleEquals('Open Prompts')
      .waitForElementVisible('#root', 15000)
      // The React app should render some visible content inside #root.
      .waitForElementVisible('#root *', 15000, 'React app rendered content')
      .end();
  });
});
