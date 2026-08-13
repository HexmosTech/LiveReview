import { Page } from '@playwright/test';

// Local dev server started via `npm start` in ui/ (webpack-dev-server, cloud-mode build).
export const LOCAL_BASE_URL = 'http://localhost:8081/';
// Self-hosted-mode build (LIVEREVIEW_IS_CLOUD=false) — only needed for LicenseStatusBar
// coverage, which is unmounted entirely on a cloud-mode build. Start with:
//   cd ui && LIVEREVIEW_BUILD_MODE=selfhosted npx webpack serve --mode development --port 8082
export const SELF_HOSTED_BASE_URL = 'http://localhost:8082/';

const FAKE_USER = {
  id: 999001,
  email: 'e2e-notifications@example.com',
  name: 'E2E Notifications',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  default_org_id: 1,
};

const FAKE_ORG = {
  id: 1,
  name: 'E2E Test Org',
  description: '',
  is_active: true,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  subscription_plan: 'free_30k',
  subscription_status: 'active',
  created_by_user_id: 999001,
  member_count: 1,
};

const FAKE_ORG_INFO = { id: 1, name: 'E2E Test Org', role: 'owner' };

export interface ConnectorProgressFixture {
  connector_id: number;
  connector_name: string;
  provider: string;
  phase: 'discovering' | 'installing' | 'ready' | 'error';
  total_projects: number;
  connected_projects: number;
  message: string;
}

export interface MockSessionOptions {
  /** Populates DashboardData.connector_setup_progress — drives the connector banner. */
  connectorSetupProgress?: ConnectorProgressFixture[];
  /** Populates /quota/status + /billing/status — drives quota banners/BillingChip mirroring. */
  quota?: { usagePct: number; blocked: boolean; locUsed: number; locLimit: number };
  /** Days until license expiry (negative = already expired). Omit for a healthy, far-out license. */
  licenseExpiresInDays?: number;
  /** Defaults to LOCAL_BASE_URL. Pass SELF_HOSTED_BASE_URL for LicenseStatusBar coverage. */
  baseUrl?: string;
}

function isoDaysFromNow(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return d.toISOString();
}

/**
 * Fully mocks the authenticated session + every GET endpoint the Navbar/Dashboard/
 * License shell needs on mount, then navigates to the local dev server. No real
 * backend auth or seeded data required — every request is intercepted at the
 * browser network layer, so this never touches the real DB or a shared environment.
 */
export async function mockAuthenticatedSession(page: Page, opts: MockSessionOptions = {}): Promise<void> {
  const baseUrl = opts.baseUrl ?? LOCAL_BASE_URL;

  await page.addInitScript(() => {
    localStorage.setItem('accessToken', 'e2e-fake-access-token');
    localStorage.setItem('refreshToken', 'e2e-fake-refresh-token');
    localStorage.setItem('currentOrgId', '1');
  });

  // Catch-all FIRST (Playwright gives later-registered routes priority, so the
  // specific mocks below override this): any /api/v1 call we didn't anticipate
  // gets a benign empty 200 instead of falling through to the real backend,
  // which would 401 on our fake token and cascade into a real logout redirect.
  await page.route('**/api/v1/**', (route) => route.fulfill({ json: {} }));
  // The CSRF endpoint is a bare (non-/api/v1) fetch — only reached before
  // non-GET requests (e.g. the dashboard refresh POST below).
  await page.route('**/csrf-token', (route) => route.fulfill({ json: { csrfToken: 'e2e-fake-csrf' } }));

  await page.route('**/api/v1/auth/me', (route) =>
    route.fulfill({ json: { user: FAKE_USER, organizations: [FAKE_ORG_INFO] } })
  );
  await page.route('**/api/v1/auth/setup-status', (route) =>
    route.fulfill({ json: { setup_required: false } })
  );
  await page.route('**/api/v1/system/info', (route) =>
    route.fulfill({
      json: {
        deployment_mode: 'production',
        capabilities: { webhooks_enabled: true, manual_triggers_only: false, external_access: true, proxy_mode: false },
      },
    })
  );
  await page.route('**/api/v1/production-url', (route) =>
    route.fulfill({ json: { url: baseUrl, success: true, message: '' } })
  );
  await page.route('**/api/v1/organizations', (route) =>
    route.fulfill({ json: { organizations: [FAKE_ORG], default_org_id: 1 } })
  );

  await page.route('**/api/v1/dashboard/refresh', (route) =>
    route.fulfill({ json: { message: 'ok', last_updated: new Date().toISOString() } })
  );
  await page.route('**/api/v1/dashboard', (route) =>
    route.fulfill({
      json: {
        total_reviews: 0,
        total_comments: 0,
        connected_providers: 1,
        active_ai_connectors: 1,
        connector_setup_progress: opts.connectorSetupProgress ?? [],
        api_url: baseUrl,
        cli_installed: true,
        recent_activity: [],
        performance_metrics: {},
        system_status: {},
        last_updated: new Date().toISOString(),
      },
    })
  );

  const usagePct = opts.quota?.usagePct ?? 10;
  const blocked = opts.quota?.blocked ?? false;
  const locUsed = opts.quota?.locUsed ?? 3000;
  const locLimit = opts.quota?.locLimit ?? 30000;

  await page.route('**/api/v1/billing/status', (route) =>
    route.fulfill({
      json: {
        billing: { current_plan_code: 'free_30k', loc_used_month: locUsed, billing_period_end: isoDaysFromNow(20) },
        available_plans: [{ plan_code: 'free_30k', monthly_loc_limit: locLimit }],
      },
    })
  );
  await page.route('**/api/v1/quota/status', (route) =>
    route.fulfill({
      json: {
        plan_type: 'free_30k',
        envelope: {
          usage_pct: usagePct,
          blocked,
          loc_used_month: locUsed,
          loc_limit_month: locLimit,
          plan_code: 'free_30k',
          billing_period_end: isoDaysFromNow(20),
        },
      },
    })
  );
  await page.route('**/api/v1/billing/usage/me', (route) =>
    route.fulfill({
      json: { member: { total_billable_loc: locUsed, operation_count: 5, usage_share_percent: 100 } },
    })
  );
  await page.route('**/api/v1/billing/upgrade/request-status', (route) =>
    route.fulfill({ json: { request: null } })
  );

  const licenseDays = opts.licenseExpiresInDays;
  await page.route('**/api/v1/license/status', (route) =>
    route.fulfill({
      json: {
        status: 'active',
        subject: 'E2E Test Org',
        appName: 'LiveReview',
        unlimited: false,
        seatCount: 10,
        activeUsers: 1,
        assignedSeats: 1,
        expiresAt: licenseDays !== undefined ? isoDaysFromNow(licenseDays) : isoDaysFromNow(200),
        lastValidatedAt: new Date().toISOString(),
        lastValidationCode: 'ok',
      },
    })
  );

  await page.goto(baseUrl);
  // Dashboard polls in the background (5min/60s intervals), so 'networkidle'
  // never settles — wait for the concrete element every test actually needs.
  await page.waitForSelector('button[aria-label="Notifications"], button[aria-label*="unread notifications"]', {
    timeout: 20000,
  });
}
