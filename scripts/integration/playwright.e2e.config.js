// Local E2E Playwright config.  UI tests reuse the integration harness's
// installed Playwright but run against the selected tunnel/dev server with a
// cached browser session created by `make e2e-auth` / `make dev-up`.
const { defineConfig } = require('@playwright/test');

const baseURL = (process.env.E2E_BASE_URL || 'http://localhost:8081').replace(/\/+$/, '');
const artifacts = process.env.E2E_ARTIFACTS || '/tmp/livereview-e2e-artifacts';

module.exports = defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 15_000 },
  forbidOnly: true,
  reporter: [
    ['list'],
    ['html', { outputFolder: `${artifacts}/playwright-report`, open: 'never' }],
  ],
  outputDir: `${artifacts}/playwright-results`,
  use: {
    baseURL,
    headless: process.env.E2E_HEADED !== '1',
    storageState: process.env.E2E_PLAYWRIGHT_STATE || undefined,
    trace: 'off',
    video: 'off',
    screenshot: 'only-on-failure',
    viewport: { width: 1440, height: 1000 },
  },
});
