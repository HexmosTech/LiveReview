import { Page } from '@playwright/test';
import { SHARED_CONFIG } from './config';

/**
 * Login to LiveReview using super admin credentials
 * @param page - Playwright page object
 * @returns Promise<void>
 */
export async function loginAsSuperAdmin(page: Page): Promise<void> {
  // Navigate to the login page
  await page.goto(SHARED_CONFIG.BASE_URL);
  
  // Wait for page to be fully loaded
  await page.waitForLoadState('networkidle');
  
  // Check if we're already logged in by looking for dashboard elements
  const isDashboard = await page.locator('[data-testid="dashboard"]').isVisible().catch(() => false);
  const hasLogoutButton = await page.getByRole('button', { name: 'Logout' }).isVisible().catch(() => false);
  
  if (isDashboard || hasLogoutButton) {
    console.log('Already logged in, skipping login process');
    return;
  }
  
  // Check if we're on the organization setup page (first time setup)
  const isSetupPage = await page.getByRole('textbox', { name: 'Organization Name' }).isVisible().catch(() => false);
  
  if (isSetupPage) {
    console.log('Organization not set up yet, cannot login. Please run the setup test first.');
    throw new Error('Organization setup required. Run the main setup test first.');
  }
  
  // Look for login form elements
  const emailField = page.locator('input[id="email-address"], input[type="email"], input[name="email"]');
  const passwordField = page.locator('input[id="password"], input[type="password"], input[name="password"]');
  const submitButton = page.locator('button[type="submit"], button:has-text("Sign in"), button:has-text("Login")');
  
  // Wait for login form to be visible
  await emailField.waitFor({ state: 'visible', timeout: 10000 });
  
  // Fill in credentials
  await emailField.fill(SHARED_CONFIG.SUPER_ADMIN_EMAIL);
  await passwordField.fill(SHARED_CONFIG.SUPER_ADMIN_PASSWORD);
  
  // Submit the form
  await submitButton.click();
  
  // Wait for successful login - check for navigation or dashboard elements
  await page.waitForLoadState('networkidle');
  
  // Verify login was successful
  const currentUrl = page.url();
  if (currentUrl.includes('dashboard') || currentUrl !== SHARED_CONFIG.BASE_URL) {
    console.log('Login successful');
  } else {
    throw new Error('Login may have failed - still on login page');
  }
}

/**
 * Login to LiveReview using owner credentials
 * @param page - Playwright page object
 * @returns Promise<void>
 */
export async function loginAsOwner(page: Page): Promise<void> {
  // Navigate to the login page
  await page.goto(SHARED_CONFIG.BASE_URL);
  
  // Wait for page to be fully loaded
  await page.waitForLoadState('networkidle');
  
  // Check if we're already logged in by looking for dashboard elements
  const isDashboard = await page.locator('[data-testid="dashboard"]').isVisible().catch(() => false);
  const hasLogoutButton = await page.getByRole('button', { name: 'Logout' }).isVisible().catch(() => false);
  
  if (isDashboard || hasLogoutButton) {
    console.log('Already logged in, skipping login process');
    return;
  }
  
  // Check if we're on the organization setup page (first time setup)
  const isSetupPage = await page.getByRole('textbox', { name: 'Organization Name' }).isVisible().catch(() => false);
  
  if (isSetupPage) {
    console.log('Organization not set up yet, cannot login. Please run the setup test first.');
    throw new Error('Organization setup required. Run the main setup test first.');
  }
  
  // Look for login form elements
  const emailField = page.locator('input[id="email-address"], input[type="email"], input[name="email"]');
  const passwordField = page.locator('input[id="password"], input[type="password"], input[name="password"]');
  const submitButton = page.locator('button[type="submit"], button:has-text("Sign in"), button:has-text("Login")');
  
  // Wait for login form to be visible
  await emailField.waitFor({ state: 'visible', timeout: 10000 });
  
  // Fill in owner credentials
  await emailField.fill(SHARED_CONFIG.OWNER_EMAIL);
  await passwordField.fill(SHARED_CONFIG.OWNER_PASSWORD);
  
  // Submit the form
  await submitButton.click();
  
  // Wait for successful login - check for navigation or dashboard elements
  await page.waitForLoadState('networkidle');
  
  // Verify login was successful
  /*
  const currentUrl = page.url();
  if (currentUrl.includes('dashboard') || currentUrl !== SHARED_CONFIG.BASE_URL) {
    console.log('Login as owner successful');
  } else {
    throw new Error('Login may have failed - still on login page');
  }
  */
}

/**
 * Navigate to Git Providers page
 * @param page - Playwright page object
 * @returns Promise<void>
 */
export async function navigateToGitProviders(page: Page): Promise<void> {
  // Navigate to settings if not already there
  await page.getByRole('link', { name: 'Settings' }).click();
  await page.waitForLoadState('networkidle');
  
  // Navigate to Git Providers
  await page.getByRole('link', { name: 'Git Providers' }).click();
  await page.waitForLoadState('networkidle');
}