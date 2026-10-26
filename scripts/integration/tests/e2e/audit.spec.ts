import { test, expect } from '@playwright/test';

const auditUrl = '/#/reports/audit';
const reviewsUrl = '/#/reviews';

async function shellBox(page: import('@playwright/test').Page) {
  const container = page.locator('div.container.mx-auto.px-4.py-8').first();
  await expect(container).toBeVisible();
  const box = await container.boundingBox();
  expect(box).not.toBeNull();
  return box!;
}

test.describe('audit page', () => {
  test('renders the audit report with the Reviews page shell', async ({ page }) => {
    await page.goto(auditUrl);
    await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();
    await expect(page.getByText('Activity', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'CSV' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'NDJSON' })).toBeVisible();
    await expect(page.getByPlaceholder('Search action, resource, path…')).toBeVisible();
    await expect(page.locator('th', { hasText: 'Time' }).first()).toBeVisible();

    const audit = await shellBox(page);
    await page.goto(reviewsUrl);
    await expect(page.locator('div.container.mx-auto.px-4.py-8').first()).toBeVisible();
    const reviews = await shellBox(page);

    expect(Math.abs(audit.x - reviews.x)).toBeLessThanOrEqual(2);
    expect(Math.abs(audit.width - reviews.width)).toBeLessThanOrEqual(2);
    expect(Math.abs(audit.y - reviews.y)).toBeLessThanOrEqual(2);
  });

  test('filters, AI usage tab, and UI export work', async ({ page }) => {
    await page.goto(auditUrl);
    await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();

    const search = page.getByPlaceholder('Search action, resource, path…');
    await search.fill('auth.login');
    await page.waitForTimeout(700);
    await expect(page.locator('body')).not.toContainText('Failed to load audit events');

    await page.getByRole('button', { name: 'AI usage' }).click();
    await page.waitForTimeout(500);
    await expect(page.locator('body')).not.toContainText('Failed to load AI usage');
    await page.getByRole('button', { name: 'Activity' }).click();

    const csv = page.getByRole('button', { name: 'CSV' });
    await expect(csv).toBeVisible();
    const downloadPromise = page.waitForEvent('download', { timeout: 30_000 });
    await csv.click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/\.csv$/);
  });

});

test.describe('super admin audit view', () => {
  test.use({
    storageState: process.env.E2E_PLAYWRIGHT_STATE_SUPER || undefined,
  });

  test('super admin can open retention and all-org controls', async ({ page }) => {
    await page.goto(auditUrl);
    await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retention' })).toBeVisible();
    await page.getByRole('button', { name: 'Retention' }).click();
    await expect(page.getByText('Retention & integrity')).toBeVisible();
    await expect(page.getByText('Verify hash chain')).toBeVisible();
    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.getByText('All organizations')).toBeVisible();
  });
});

test.describe('member audit view', () => {
  test.use({
    storageState: process.env.E2E_PLAYWRIGHT_STATE_MEMBER || undefined,
  });

  test('member sees only their own activity and no admin controls', async ({ page }) => {
    await page.goto(auditUrl);
    await expect(page.getByRole('heading', { name: 'Audit log' })).toBeVisible();
    await expect(page.getByText(/You are viewing your own activity/)).toBeVisible();
    await expect(page.getByText('All organizations')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Retention' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Archive' })).toHaveCount(0);
  });
});
