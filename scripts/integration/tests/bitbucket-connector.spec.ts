import { test, expect } from '@playwright/test';
import { loginAsOwner, navigateToGitProviders } from './shared/login-utils';
import { BITBUCKET_CONFIG } from './shared/config';

test('Add Bitbucket Connector', async ({ page }) => {
  // Set timeout for this test
  test.setTimeout(60000); // 1 minute
  
  // Step 1: Login using owner credentials (preferred for this task)
  await loginAsOwner(page);
  // Wait for connector creation to complete
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
  
  // Step 2: Navigate to Git Providers page
  await navigateToGitProviders(page);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
  
  
  // Select Bitbucket connector type
  await page.getByRole('button', { name: 'Bitbucket' }).click();
  
  // Wait for Bitbucket connector form to load
  await page.waitForTimeout(1000);
  
  // Step 4: Fill in connector name
  await page.locator('#manual-connector-name').click();
  await page.locator('#manual-connector-name').fill(BITBUCKET_CONFIG.CONNECTOR_NAME);
  
  // Step 5: Fill in Bitbucket email
  await page.locator('#manual-email').click();
  await page.locator('#manual-email').fill(BITBUCKET_CONFIG.BITBUCKET_EMAIL);
  
  // Step 6: Fill in Bitbucket API Token
  await page.locator('#manual-api-token').click();
  await page.locator('#manual-api-token').fill(BITBUCKET_CONFIG.BITBUCKET_API_TOKEN);
  
  // Save the connector using the specific button selector
  await page.locator('button[type="submit"]:has-text("Add Connector")').click();
  
  // Wait for validation/connection test
  await page.waitForTimeout(3000);
  
  // Handle the confirmation screen (similar to GitLab connector flow)
  const confirmButton = page.getByRole('button', { name: 'Confirm & Save' });
  if (await confirmButton.isVisible()) {
    await confirmButton.click();
  } else {
    // Alternative confirmation button text
    const altConfirmButton = page.locator('button[type="submit"]:has-text("Confirm")');
    if (await altConfirmButton.isVisible()) {
      await altConfirmButton.click();
    }
  }
  
  // Wait for connector creation to complete
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
  
  // Verify the connector was created successfully
  await expect(page.getByText(BITBUCKET_CONFIG.CONNECTOR_NAME)).toBeVisible();
  
  console.log(`Bitbucket connector '${BITBUCKET_CONFIG.CONNECTOR_NAME}' created successfully`);
});