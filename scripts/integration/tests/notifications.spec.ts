import { test, expect } from '@playwright/test';
import { mockAuthenticatedSession, SELF_HOSTED_BASE_URL } from './shared/mockSession';

// These tests run entirely against a local dev server (see mockSession.ts for the
// exact URL) with every backend call intercepted at the network layer — no real
// login, seeded org, or shared environment required. That lets each test deterministically
// drive the exact backend state (connector phase, quota %, license expiry) needed to
// exercise the notification it's checking, which isn't practical against a real environment.
//
// Prereq: `cd ui && npm start` (webpack-dev-server on :8081) must already be running.
//
// Note: the onboarding-stepper notification (see Dashboard.tsx) is added unconditionally
// once dashboard data loads, so a freshly-mocked session always starts with exactly one
// unread item — there is no truly "empty" state to assert against.

test.describe('Unified notification system', () => {
  test('onboarding nudge is the only notification when nothing else needs attention', async ({ page }) => {
    await mockAuthenticatedSession(page, { quota: { usagePct: 10, blocked: false, locUsed: 3000, locLimit: 30000 } });

    const bellBtn = page.getByTestId('notification-bell');
    await expect(bellBtn).toHaveAttribute('aria-label', '1 unread notifications');

    await bellBtn.click();
    const tray = page.getByTestId('notification-tray');
    await expect(tray.getByText('Complete your onboarding checklist')).toBeVisible();
    await expect(tray.getByText(/LOC Usage|Quota Exceeded|Installing webhooks/)).toHaveCount(0);
  });

  test('connector webhook install progress shows inline and mirrors into the tray, dismiss persists across reload', async ({ page }) => {
    await mockAuthenticatedSession(page, {
      connectorSetupProgress: [
        {
          connector_id: 4242,
          connector_name: 'github',
          provider: 'github',
          phase: 'installing',
          total_projects: 147,
          connected_projects: 134,
          message: '',
        },
      ],
    });

    const expectedMessage = 'github (github): Installing webhooks 134/147 (91%)';

    // Inline dashboard banner
    await expect(page.getByText(expectedMessage)).toBeVisible();

    // Mirrored into the tray (alongside the always-present onboarding nudge)
    const bellBtn = page.getByTestId('notification-bell');
    await expect(bellBtn).toHaveAttribute('aria-label', '2 unread notifications');
    await bellBtn.click();
    const tray = page.getByTestId('notification-tray');
    await expect(tray.getByText(expectedMessage)).toBeVisible();
    await bellBtn.click(); // close tray

    // "Don't show again" permanently dismisses both the inline banner and the tray entry
    await page.getByRole('button', { name: "Don't show again" }).click();
    await expect(page.getByText(expectedMessage)).toHaveCount(0);

    await page.reload();
    await page.waitForSelector('[data-testid="notification-bell"]');
    await expect(page.getByText(expectedMessage)).toHaveCount(0);
  });

  test('quota nearing limit surfaces a toast and tray notification via the billing chip', async ({ page }) => {
    await mockAuthenticatedSession(page, { quota: { usagePct: 95, blocked: false, locUsed: 28500, locLimit: 30000 } });

    // Toast fires once, unprompted (react-hot-toast renders with role="status")
    await expect(page.getByRole('status').filter({ hasText: 'LOC Usage Nearing Limit' })).toBeVisible();

    const bellBtn = page.getByTestId('notification-bell');
    await bellBtn.click();
    const tray = page.getByTestId('notification-tray');
    await expect(tray.getByText('LOC Usage Nearing Limit')).toBeVisible();
    await expect(tray.getByText(/28,500.*30,000.*95%/)).toBeVisible();
  });

  test('quota blocked surfaces an error-severity toast and tray notification', async ({ page }) => {
    await mockAuthenticatedSession(page, { quota: { usagePct: 100, blocked: true, locUsed: 30000, locLimit: 30000 } });

    await expect(page.getByRole('status').filter({ hasText: 'Quota Exceeded' })).toBeVisible();

    const bellBtn = page.getByTestId('notification-bell');
    await bellBtn.click();
    const tray = page.getByTestId('notification-tray');
    await expect(tray.getByText('Quota Exceeded', { exact: true })).toBeVisible();
    await expect(tray.getByText(/Reviews are blocked/)).toBeVisible();
  });

  test('license expiring soon surfaces a tray notification with a Renew action', async ({ page }) => {
    // LicenseStatusBar (and the expiry notification it drives) only renders on a
    // self-hosted build (`if (isCloudMode()) return null`) — LIVEREVIEW_IS_CLOUD is
    // baked in at webpack build time, so this needs its own server, not just a route
    // mock. Start it with:
    //   cd ui && LIVEREVIEW_BUILD_MODE=selfhosted npx webpack serve --mode development --port 8082
    await mockAuthenticatedSession(page, { licenseExpiresInDays: 5, baseUrl: SELF_HOSTED_BASE_URL });

    await expect(page.getByTestId('license-status-bar')).toBeVisible();
    await expect(page.getByText('License Expiring Soon')).toBeVisible();

    const bellBtn = page.getByTestId('notification-bell');
    await bellBtn.click();
    const tray = page.getByTestId('notification-tray');
    await expect(tray.getByText('License Expiring Soon')).toBeVisible();
    await expect(tray.getByRole('button', { name: 'Renew License' })).toBeVisible();
  });
});
