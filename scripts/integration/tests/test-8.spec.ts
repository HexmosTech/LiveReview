import { test, expect } from '@playwright/test';

test('test', async ({ page }) => {
  // Set longer timeout for this test since it has many steps
  test.setTimeout(120000); // 2 minutes
  
  await page.goto('https://livereview.hexmos.site/');
  
  // Wait for page to be fully loaded
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('textbox', { name: 'Organization Name' }).click();
  await page.getByRole('textbox', { name: 'Organization Name' }).fill('Hexmos01');
  await page.getByRole('textbox', { name: 'Organization Name' }).press('Tab');
  await page.getByRole('textbox', { name: 'Admin Email' }).fill('shrijith@hexmos.com');
  await page.getByRole('textbox', { name: 'Admin Email' }).press('Tab');
  await page.getByRole('textbox', { name: 'Password' }).fill('MegaSuperAdmin@123');
  await page.getByRole('button', { name: 'Complete Setup' }).click();
  
  // Wait for navigation after setup completion
  await page.waitForLoadState('networkidle');
  
  await page.getByRole('textbox', { name: 'Paste license JWT here' }).click();
  await page.getByRole('textbox', { name: 'Paste license JWT here' }).fill('eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6InYxIn0.eyJpc3MiOiJmdy1wYXJzZS1qd3QtbGljZW5jZSIsInN1YiI6InNocmlqaXRoLnN2QGdtYWlsLmNvbSIsImF1ZCI6IkxpdmVSZXZpZXciLCJleHAiOjE3NTk3NjM1MTEsImlhdCI6MTc1OTI0NDEzNCwianRpIjoiNGQ5MTA3NDUtMzJjZi00YzhhLTljODktNmYwYjZiYmFhZWU2IiwibGljSWQiOiJBNjZsZzVicjVZIiwiZW1haWwiOiJzaHJpaml0aC5zdkBnbWFpbC5jb20iLCJhcHBOYW1lIjoiTGl2ZVJldmlldyIsInNlYXRDb3VudCI6MCwidW5saW1pdGVkIjp0cnVlLCJ2ZXIiOjF9.D7LuuD8hrKvkzqo28k1LNLkQgE46AWtwxq4ozJngqb1Fnc_fmUyjUIfR2PcpA0uG6e5HUtWXKt38UK2rwqr65QLbHsTX7lnFWk_JOjTvxlF9EvNwEs8kYAa6u49y6jct9PnkwoenJtwizdfeCzGyJMxpJrsUC8ug3lfAMCnvkV9GliJQh7KpxhA7L7G6OmRD2l-9yRcnGk8ZuXnNYFY_d0S3Y1HOgZVn1VDSJny1Yz4JvMvzKs6O2SMKW5kz7ogjGEx5gq5pgkZENfESvB1mvVKQXEnP3c4DUESxbZeQmr_oh1LbTQoHE0uh9wnH9YnzmfRQ1GcY8XueEi-zXU_2SA');
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
  await page.getByRole('textbox', { name: 'Email Address' }).fill('general@hexmos.com');
  await page.getByRole('textbox', { name: 'Email Address' }).press('Tab');
  await page.getByRole('textbox', { name: 'First Name' }).fill('General');
  await page.getByRole('textbox', { name: 'First Name' }).press('Tab');
  await page.getByRole('textbox', { name: 'Last Name' }).fill('Mega');
  await page.getByLabel('Role').selectOption('owner');
  await page.getByRole('textbox', { name: 'Password', exact: true }).click();
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill('MegaGeneral@123');
  await page.getByRole('textbox', { name: 'Password', exact: true }).press('Tab');
  await page.getByRole('textbox', { name: 'Confirm Password' }).fill('MegaGeneral@123');
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
  await page.getByRole('textbox', { name: 'API Key' }).fill('AIzaSyBclg5XgmwtO7nE8Vxbwe9G5MU32c1Q6EI');
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
  await page.getByRole('textbox', { name: 'Connector Name' }).fill('GL2130');
  await page.locator('#manual-pat').click();
  await page.locator('#manual-pat').click();
  await page.locator('#manual-pat').fill('REDACTED_GITLAB_PAT_2nm86MQp1OjJiCA.01.0y1uxj51i');
  await page.locator('#manual-pat').click();
  await page.getByRole('textbox', { name: 'Instance URL' }).click();
  await page.getByRole('textbox', { name: 'Instance URL' }).fill('https://git.apps.hexmos.com');
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