// Nightwatch configuration for open-prompts e2e tests.
// Primary browser: Firefox via /snap/bin/geckodriver.
// Fallback: Chromium (run with --env chromium) using the snap's chromedriver
// and the real chrome binary (the snap wrapper is not exec-able by
// chromedriver, so we point at the binary inside the snap directly).
module.exports = {
  src_folders: ['tests'],
  output_folder: false,

  test_settings: {
    default: {
      launch_url: 'http://localhost:3000',
      selenium: {
        start_process: false,
        host: 'localhost',
      },
      webdriver: {
        start_process: true,
        server_path: '/snap/bin/geckodriver',
        port: 4444,
        cli_args: [],
      },
      desiredCapabilities: {
        browserName: 'firefox',
        alwaysMatch: {
          'moz:firefoxOptions': {
            args: ['-headless', '--width=1440', '--height=900'],
          },
        },
      },
      screenshots: {
        enabled: true,
        path: 'screenshots',
        on_failure: true,
      },
    },

    firefox: {
      extends: 'default',
    },

    chromium: {
      extends: 'default',
      webdriver: {
        start_process: true,
        server_path: '/snap/bin/chromium.chromedriver',
        port: 9515,
        cli_args: [],
      },
      desiredCapabilities: {
        browserName: 'chrome',
        'goog:chromeOptions': {
          binary: '/snap/chromium/current/usr/lib/chromium-browser/chrome',
          args: ['--headless=new', '--no-sandbox', '--disable-gpu', '--window-size=1440,900'],
        },
      },
    },
  },
};