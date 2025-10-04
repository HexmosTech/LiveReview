import { test, expect } from '@playwright/test';

// Configuration constants - edit these as needed
const CONFIG = {
  // Site URL
  // BASE_URL: 'https://livereview.hexmos.site/',
  BASE_URL: 'https://manual-talent.apps.hexmos.com/',
  
  // Organization setup
  ORGANIZATION_NAME: 'Hexmos01',
  ADMIN_EMAIL: 'shrijith@hexmos.com',
  ADMIN_PASSWORD: 'MegaSuperAdmin@123',
  
  // License JWT token
  LICENSE_JWT: 'eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6InYxIn0.eyJpc3MiOiJmdy1wYXJzZS1qd3QtbGljZW5jZSIsInN1YiI6InNocmlqaXRoLnN2QGdtYWlsLmNvbSIsImF1ZCI6IkxpdmVSZXZpZXciLCJleHAiOjE3NTk3NjM1MTEsImlhdCI6MTc1OTI0NDEzNCwianRpIjoiNGQ5MTA3NDUtMzJjZi00YzhhLTljODktNmYwYjZiYmFhZWU2IiwibGljSWQiOiJBNjZsZzVicjVZIiwiZW1haWwiOiJzaHJpaml0aC5zdkBnbWFpbC5jb20iLCJhcHBOYW1lIjoiTGl2ZVJldmlldyIsInNlYXRDb3VudCI6MCwidW5saW1pdGVkIjp0cnVlLCJ2ZXIiOjF9.D7LuuD8hrKvkzqo28k1LNLkQgE46AWtwxq4ozJngqb1Fnc_fmUyjUIfR2PcpA0uG6e5HUtWXKt38UK2rwqr65QLbHsTX7lnFWk_JOjTvxlF9EvNwEs8kYAa6u49y6jct9PnkwoenJtwizdfeCzGyJMxpJrsUC8ug3lfAMCnvkV9GliJQh7KpxhA7L7G6OmRD2l-9yRcnGk8ZuXnNYFY_d0S3Y1HOgZVn1VDSJny1Yz4JvMvzKs6O2SMKW5kz7ogjGEx5gq5pgkZENfESvB1mvVKQXEnP3c4DUESxbZeQmr_oh1LbTQoHE0uh9wnH9YnzmfRQ1GcY8XueEi-zXU_2SA',
  
  // User creation
  USER_EMAIL: 'general@hexmos.com',
  USER_FIRST_NAME: 'General',
  USER_LAST_NAME: 'Mega',
  USER_ROLE: 'owner',
  USER_PASSWORD: 'MegaGeneral@123',
  
  // AI Provider (Google Gemini)
  GEMINI_API_KEY: 'AIzaSyBclg5XgmwtO7nE8Vxbwe9G5MU32c1Q6EI',
  
  // Git Provider (GitLab)
  GITLAB_CONNECTOR_NAME: 'GL2130',
  GITLAB_PAT: 'REDACTED_GITLAB_PAT_2nm86MQp1OjJiCA.01.0y1uxj51i',
  GITLAB_INSTANCE_URL: 'https://git.apps.hexmos.com'
};

test('test', async ({ page }) => {
  // Set longer timeout for this test since it has many steps
  test.setTimeout(120000); // 2 minutes
  
  await page.goto(CONFIG.BASE_URL);
  
  // Wait for page to be fully loaded
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('textbox', { name: 'Organization Name' }).click();
  await page.getByRole('textbox', { name: 'Organization Name' }).fill(CONFIG.ORGANIZATION_NAME);
  await page.getByRole('textbox', { name: 'Organization Name' }).press('Tab');
  await page.getByRole('textbox', { name: 'Admin Email' }).fill(CONFIG.ADMIN_EMAIL);
  await page.getByRole('textbox', { name: 'Admin Email' }).press('Tab');
  await page.getByRole('textbox', { name: 'Password' }).fill(CONFIG.ADMIN_PASSWORD);
  await page.getByRole('button', { name: 'Complete Setup' }).click();
  
  // Wait for navigation after setup completion
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('textbox', { name: 'Paste license JWT here' }).click();
  await page.getByRole('textbox', { name: 'Paste license JWT here' }).fill(CONFIG.LICENSE_JWT);
  await page.getByRole('button', { name: 'Save Token' }).click();
  
  // Wait for token save to complete and navigation
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('link', { name: 'Settings' }).click();
  
  // Wait for settings page to load
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('button', { name: 'User Management' }).click();
  await page.getByRole('button', { name: 'Instance' }).click();
  await page.getByRole('button', { name: 'Save' }).click();
  
  // Wait for save operation to complete
  await page.waitForTimeout(1000);
  
  await page.getByRole('button', { name: 'User Management' }).click();
  await page.getByRole('link', { name: 'Add User' }).click();
  
  // Wait for Add User page to load
  await page.waitForLoadState('networkidle');
  await page.getByRole('textbox', { name: 'Email Address' }).click();
  await page.getByRole('textbox', { name: 'Email Address' }).fill(CONFIG.USER_EMAIL);
  await page.getByRole('textbox', { name: 'Email Address' }).press('Tab');
  await page.getByRole('textbox', { name: 'First Name' }).fill(CONFIG.USER_FIRST_NAME);
  await page.getByRole('textbox', { name: 'First Name' }).press('Tab');
  await page.getByRole('textbox', { name: 'Last Name' }).fill(CONFIG.USER_LAST_NAME);
  await page.getByLabel('Role').selectOption(CONFIG.USER_ROLE);
  await page.getByRole('textbox', { name: 'Password', exact: true }).click();
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill(CONFIG.USER_PASSWORD);
  await page.getByRole('textbox', { name: 'Password', exact: true }).press('Tab');
  await page.getByRole('textbox', { name: 'Confirm Password' }).fill(CONFIG.USER_PASSWORD);
  await page.getByRole('button', { name: 'Create User' }).click();
  
  // Wait for user creation to complete - this is a critical point where it often fails
  await page.waitForLoadState('networkidle');
  // Add extra wait to ensure any loaders/spinners complete
  await page.waitForTimeout(2000);
  
  await page.getByRole('link', { name: 'AI Providers' }).click();
  
  // Wait for AI Providers page to load
  await page.waitForLoadState('networkidle');
  await page.getByRole('button', { name: 'Add Connector' }).click();
  
  // Wait for the connector modal/form to load
  await page.waitForTimeout(1000);
  
  await page.getByRole('button', { name: 'Google Gemini Recommended' }).click();
  await page.getByRole('textbox', { name: 'API Key' }).click();
  await page.getByRole('textbox', { name: 'API Key' }).fill(CONFIG.GEMINI_API_KEY);
  await page.getByRole('button', { name: 'Save Connector' }).click();
  
  // Wait for AI connector creation to complete - another critical failure point
  await page.waitForLoadState('networkidle');
  // Add extra wait for any success messages or redirects
  await page.waitForTimeout(2000);
  
  await page.getByRole('link', { name: 'Git Providers' }).click();
  
  // Wait for Git Providers page to load
  await page.waitForLoadState('networkidle');
  await page.getByRole('button', { name: 'Self-Hosted GitLab' }).click();
  
  // Wait for GitLab connector form to load
  await page.waitForTimeout(1000);
  
  await page.getByRole('textbox', { name: 'Connector Name' }).click();
  await page.getByRole('textbox', { name: 'Connector Name' }).fill(CONFIG.GITLAB_CONNECTOR_NAME);
  await page.locator('#manual-pat').click();
  await page.locator('#manual-pat').click();
  await page.locator('#manual-pat').fill(CONFIG.GITLAB_PAT);
  await page.locator('#manual-pat').click();
  await page.getByRole('textbox', { name: 'Instance URL' }).click();
  await page.getByRole('textbox', { name: 'Instance URL' }).fill(CONFIG.GITLAB_INSTANCE_URL);
  await page.getByRole('button', { name: 'Add Connector' }).click();
  
  // Wait for validation/connection test
  await page.waitForTimeout(3000);
  
  await page.getByRole('button', { name: 'Confirm & Save' }).click();
  
  // Wait for git connector creation to complete
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
  
  await page.getByRole('link', { name: 'Dashboard' }).click();
  
  // Wait for dashboard to load
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('button', { name: 'Logout' }).click();
});