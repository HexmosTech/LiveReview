// Ported-in-spirit from scripts/integration login helpers.  Kept standalone so
// `make e2e-auth` can bootstrap a cached browser session without starting the
// whole test suite.
//
// Modes:
//   E2E_AUTH_MODE=password  -> /admin email+password login (local accounts)
//   E2E_AUTH_MODE=otp       -> automate the Hexmos email + fixed OTP flow
//   E2E_AUTH_MODE=browser   -> open the normal Hexmos SSO login and wait for the
//                              user to complete it
//
// The resulting session is written to E2E_AUTH_OUT (mode 0600).  Secrets are
// never printed to stdout.

const fs = require('fs');
const path = require('path');
// Resolve from the existing integration workspace so the harness needs no
// second Playwright install.
const { chromium } = require(path.join(__dirname, '..', 'integration', 'node_modules', 'playwright'));

const baseUrl = (process.env.E2E_BASE_URL || 'http://localhost:8081').replace(/\/+$/, '');
const headed = process.env.E2E_HEADED !== '0';
const timeoutMs = Number(process.env.E2E_AUTH_TIMEOUT_MS || '600000');
const outPath = process.env.E2E_AUTH_OUT;
const email = process.env.E2E_EMAIL || '';
const password = process.env.E2E_PASSWORD || '';
const otp = process.env.E2E_OTP || '';
const mode = process.env.E2E_AUTH_MODE || (password ? 'password' : (otp ? 'otp' : 'browser'));

if (!outPath) {
  console.error('E2E_AUTH_OUT is required');
  process.exit(2);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function readSession(page) {
  return page.evaluate(() => {
    const get = (k) => {
      try { return localStorage.getItem(k); } catch (_) { return null; }
    };
    return {
      accessToken: get('accessToken'),
      refreshToken: get('refreshToken'),
      currentOrgId: get('currentOrgId'),
    };
  });
}

async function waitForSession(page, deadline) {
  while (Date.now() < deadline) {
    const session = await readSession(page);
    if (session.accessToken && session.refreshToken) return session;
    await sleep(750);
  }
  return null;
}

async function fetchMe(accessToken) {
  const resp = await fetch(`${baseUrl}/api/v1/auth/me`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  });
  if (!resp.ok) throw new Error(`/auth/me returned HTTP ${resp.status}`);
  return resp.json();
}

(async () => {
  const browser = await chromium.launch({ headless: !headed });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  const deadline = Date.now() + timeoutMs;

  try {
    if (mode === 'password') {
      if (!email || !password) {
        throw new Error('E2E_EMAIL and E2E_PASSWORD are required for password mode');
      }
      // App.tsx renders the email/password form only on the real /admin path
      // (not the hash route), even though the rest of the app is HashRouter.
      await page.goto(`${baseUrl}/admin`, { waitUntil: 'domcontentloaded' });
      await page.locator('input[type="email"]').fill(email);
      await page.locator('input[type="password"]').fill(password);
      await page.getByRole('button', { name: /sign in/i }).click();
    } else {
      await page.goto(baseUrl, { waitUntil: 'domcontentloaded' });
      const sso = page.getByRole('button', { name: /sign in with hexmos/i });
      await sso.waitFor({ state: 'visible', timeout: 30000 });
      await sso.click();

      if (mode === 'otp') {
        if (!email || !otp) {
          throw new Error('E2E_EMAIL and E2E_OTP are required for OTP mode');
        }
        // Hexmos One: email -> "Sign in with OTP" -> 6-digit OTP -> app return.
        const emailInput = page.getByPlaceholder('Email address');
        await emailInput.waitFor({ state: 'visible', timeout: 30000 });
        await emailInput.fill(email);
        await page.getByRole('button', { name: /sign in with otp/i }).click();

        const otpInput = page.getByPlaceholder('6-digit OTP');
        await otpInput.waitFor({ state: 'visible', timeout: 30000 });
        await otpInput.fill(otp);
        await page.getByRole('button', { name: /submit otp/i }).click();
      } else {
        console.error('Complete the Hexmos sign-in in the opened browser window.');
      }
    }

    const session = await waitForSession(page, deadline);
    if (!session) throw new Error('Timed out waiting for LiveReview tokens in localStorage');

    const me = await fetchMe(session.accessToken);
    const payload = {
      email: (me.user && me.user.email) || email,
      accessToken: session.accessToken,
      refreshToken: session.refreshToken,
      currentOrgId: session.currentOrgId,
      user: me.user || {},
      organizations: me.organizations || [],
    };

    fs.mkdirSync(path.dirname(outPath), { recursive: true, mode: 0o700 });
    fs.writeFileSync(outPath, JSON.stringify(payload, null, 2), { mode: 0o600 });
    console.log(JSON.stringify({ ok: true, email: payload.email, organizations: payload.organizations.length }));
  } finally {
    await browser.close();
  }
})().catch((err) => {
  console.error(err && err.message ? err.message : String(err));
  process.exit(1);
});
