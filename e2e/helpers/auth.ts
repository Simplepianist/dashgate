import { Page, APIRequestContext, expect } from '@playwright/test';

export const TEST_ADMIN_USER = 'admin';
export const TEST_ADMIN_PASSWORD = 'testpassword123';
export const TEST_ADMIN_DISPLAY_NAME = 'Test Admin';
export const TEST_ADMIN_EMAIL = 'admin@test.local';
export const TEST_ADMIN_GROUP = 'admins';

/**
 * Selects an auth provider on the setup wizard page.
 * Clicks the .auth-option label which toggles the hidden checkbox via JS.
 */
export async function selectAuthProvider(page: Page, provider: string): Promise<void> {
  await page.locator(`.auth-option[data-provider="${provider}"]`).click();
}

/**
 * Completes the setup wizard with local auth and a default admin account.
 * Assumes the server is freshly started with no prior setup.
 */
export async function completeSetup(
  page: Page,
  opts: {
    username?: string;
    password?: string;
    displayName?: string;
    email?: string;
    adminGroup?: string;
  } = {}
): Promise<void> {
  const {
    username = TEST_ADMIN_USER,
    password = TEST_ADMIN_PASSWORD,
    displayName = TEST_ADMIN_DISPLAY_NAME,
    email = TEST_ADMIN_EMAIL,
    adminGroup = TEST_ADMIN_GROUP,
  } = opts;

  await page.goto('/');
  // Should redirect to /setup on first visit
  await page.waitForURL('**/setup');

  // Step 1: Select local auth provider
  await selectAuthProvider(page, 'local');
  await page.locator('#btnProvidersNext').click();

  // Step: Final — fill admin account details
  await page.waitForSelector('#adminUsername', { state: 'visible' });
  await page.fill('#adminUsername', username);
  await page.fill('#adminPassword', password);
  await page.fill('#adminDisplayName', displayName);
  await page.fill('#adminEmail', email);
  await page.fill('#adminGroup', adminGroup);

  // Complete setup
  await page.locator('#completeSetup').click();

  // Should redirect to login after setup
  await page.waitForURL('**/login', { timeout: 10_000 });
}

/**
 * Completes setup using the API directly (faster, no browser needed).
 */
export async function completeSetupViaAPI(
  request: APIRequestContext,
  baseURL: string,
  opts: {
    username?: string;
    password?: string;
    displayName?: string;
    email?: string;
    adminGroup?: string;
  } = {}
): Promise<void> {
  const {
    username = TEST_ADMIN_USER,
    password = TEST_ADMIN_PASSWORD,
    displayName = TEST_ADMIN_DISPLAY_NAME,
    email = TEST_ADMIN_EMAIL,
    adminGroup = TEST_ADMIN_GROUP,
  } = opts;

  const response = await request.post(`${baseURL}/setup`, {
    data: {
      auth_providers: ['local'],
      admin_username: username,
      admin_password: password,
      admin_display_name: displayName,
      admin_email: email,
      admin_group: adminGroup,
      session_days: 7,
    },
  });

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(`Setup failed: ${response.status()} ${body}`);
  }
}

/**
 * Logs in as the given user via the login form.
 */
export async function login(
  page: Page,
  username = TEST_ADMIN_USER,
  password = TEST_ADMIN_PASSWORD
): Promise<void> {
  await page.goto('/login');
  await page.fill('#username', username);
  await page.fill('#password', password);
  await page.locator('#loginBtn').click();

  // Wait for redirect to DashGate
  await page.waitForURL('**/', { timeout: 10_000 });
}

/**
 * Logs in via the API and sets the session cookie on the page context.
 */
export async function loginViaAPI(
  request: APIRequestContext,
  baseURL: string,
  username = TEST_ADMIN_USER,
  password = TEST_ADMIN_PASSWORD
): Promise<string> {
  const response = await request.post(`${baseURL}/api/auth/login`, {
    data: { username, password },
  });

  if (!response.ok()) {
    const body = await response.text();
    throw new Error(`Login failed: ${response.status()} ${body}`);
  }

  // Extract session cookie
  const cookies = response.headers()['set-cookie'];
  if (!cookies) {
    throw new Error('No session cookie returned from login');
  }

  const match = cookies.match(/dashgate_session=([^;]+)/);
  if (!match) {
    throw new Error('dashgate_session cookie not found in response');
  }

  return match[1];
}

/**
 * Extracts a CSRF token from the page if present in a meta tag or hidden field.
 */
export async function getCSRFToken(page: Page): Promise<string | null> {
  const meta = await page.locator('meta[name="csrf-token"]').first();
  if (await meta.count()) {
    return meta.getAttribute('content');
  }
  const input = await page.locator('input[name="_csrf"]').first();
  if (await input.count()) {
    return input.getAttribute('value');
  }
  return null;
}

/**
 * Ensures the page is set up and logged in as admin. Use this in beforeEach
 * when the test needs an authenticated session.
 */
export async function ensureLoggedInAsAdmin(page: Page): Promise<void> {
  await login(page, TEST_ADMIN_USER, TEST_ADMIN_PASSWORD);
  // Verify we're on the DashGate dashboard
  await expect(page.locator('.greeting')).toBeVisible({ timeout: 5_000 });
}
