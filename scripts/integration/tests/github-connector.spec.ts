import { test, expect } from '@playwright/test';
import { loginAsOwner, navigateToGitProviders } from './shared/login-utils';
import { GITHUB_CONFIG } from './shared/config';

test('Add GitHub Connector', async ({ page }) => {
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
  
  
  // Select GitHub connector type
  await page.getByRole('button', { name: 'GitHub' }).click();
  
  // Wait for GitHub connector form to load
  await page.waitForTimeout(1000);
  
  // Step 4: Fill in connector name
  await page.getByRole('textbox', { name: 'Connector Name' }).click();
  await page.getByRole('textbox', { name: 'Connector Name' }).fill(GITHUB_CONFIG.CONNECTOR_NAME);
  
  // Step 5: Fill in GitHub username (if required)
  const usernameField = page.locator('#manual-username');
  if (await usernameField.isVisible()) {
    await usernameField.click();
    await usernameField.fill(GITHUB_CONFIG.GITHUB_USERNAME);
  }
  
  // Step 6: Fill in GitHub Personal Access Token
  await page.locator('#manual-pat').click();
  await page.locator('#manual-pat').fill(GITHUB_CONFIG.GITHUB_TOKEN);
  
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
  
  
  console.log(`GitHub connector '${GITHUB_CONFIG.CONNECTOR_NAME}' created successfully`);
});