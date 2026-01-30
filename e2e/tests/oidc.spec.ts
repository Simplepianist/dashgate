import { test, expect } from '@playwright/test';
import { DashboardServer } from '../helpers/server';
import {
  TEST_ADMIN_USER,
  TEST_ADMIN_PASSWORD,
  TEST_ADMIN_DISPLAY_NAME,
  TEST_ADMIN_EMAIL,
  TEST_ADMIN_GROUP,
  selectAuthProvider,
} from '../helpers/auth';

/**
 * OIDC flow tests require a running Dex container from test-auth/docker-compose.yml.
 * These tests are skipped unless the DEX_URL environment variable is set.
 *
 * To run these tests:
 *   1. cd test-auth && docker-compose up -d
 *   2. DEX_URL=http://localhost:5556 npx playwright test oidc.spec.ts
 */

const DEX_URL = process.env.DEX_URL;
const DEX_USER = process.env.DEX_USER || 'admin@example.com';
const DEX_PASSWORD = process.env.DEX_PASSWORD || 'password';

test.describe('OIDC Authentication', () => {
  test.skip(!DEX_URL, 'DEX_URL not set — skipping OIDC tests');

  let server: DashboardServer;

  test.beforeAll(async ({ browser }) => {
    // OIDC tests use their own server on port 11738 to avoid conflicting
    // with the main webServer on 1738
    server = new DashboardServer({ port: 11738 });
    await server.start();

    // Complete setup with OIDC enabled
    const page = await browser.newPage();
    await page.goto(`${server.baseURL}/setup`);
    await page.waitForURL('**/setup');

    await selectAuthProvider(page, 'local');
    await selectAuthProvider(page, 'oidc');
    await page.locator('#btnProvidersNext').click();

    await page.waitForSelector('#oidcIssuer', { state: 'visible' });
    await page.fill('#oidcIssuer', `${DEX_URL}/dex`);
    await page.fill('#oidcClientID', 'dashgate');
    await page.fill('#oidcClientSecret', 'dashgate-secret');
    await page.fill(
      '#oidcRedirectURL',
      `${server.baseURL}/auth/oidc/callback`
    );
    await page.fill('#oidcScopes', 'openid profile email groups');
    await page.fill('#oidcGroupsClaim', 'groups');

    const nextBtn = page.locator(
      'button:has-text("Next"), #btnOIDCNext, .setup-step.active button.btn-primary'
    );
    if (await nextBtn.isVisible()) {
      await nextBtn.click();
    }

    await page.waitForSelector('#adminUsername', { state: 'visible' });
    await page.fill('#adminUsername', TEST_ADMIN_USER);
    await page.fill('#adminPassword', TEST_ADMIN_PASSWORD);
    await page.fill('#adminDisplayName', TEST_ADMIN_DISPLAY_NAME);
    await page.fill('#adminEmail', TEST_ADMIN_EMAIL);
    await page.fill('#adminGroup', TEST_ADMIN_GROUP);

    await page.locator('#completeSetup').click();
    await page.waitForURL('**/login', { timeout: 15_000 });
    await page.close();
  });

  test.afterAll(async () => {
    await server.stop();
  });

  test('SSO button is visible on login page', async ({ page }) => {
    await page.goto(`${server.baseURL}/login`);

    const oidcSection = page.locator('#oidcSection');
    await expect(oidcSection).toBeVisible({ timeout: 5_000 });

    const oidcBtn = page.locator('.oidc-btn');
    await expect(oidcBtn).toBeVisible();
  });

  test('SSO button redirects to Dex', async ({ page }) => {
    await page.goto(`${server.baseURL}/login`);

    const oidcBtn = page.locator('.oidc-btn');
    await expect(oidcBtn).toBeVisible({ timeout: 5_000 });
    await oidcBtn.click();

    await page.waitForURL(`${DEX_URL}/**`, { timeout: 10_000 });
    expect(page.url()).toContain(DEX_URL);
  });

  test('complete Dex login redirects back with session', async ({ page }) => {
    await page.goto(`${server.baseURL}/login`);

    const oidcBtn = page.locator('.oidc-btn');
    await expect(oidcBtn).toBeVisible({ timeout: 5_000 });
    await oidcBtn.click();

    await page.waitForURL(`${DEX_URL}/**`, { timeout: 10_000 });

    const localConnector = page.locator('a:has-text("Log in with Email")');
    if (await localConnector.isVisible().catch(() => false)) {
      await localConnector.click();
    }

    await page.waitForSelector('input[name="login"], #login', {
      state: 'visible',
      timeout: 10_000,
    });
    await page.fill('input[name="login"], #login', DEX_USER);
    await page.fill('input[name="password"], #password', DEX_PASSWORD);
    await page.locator('button[type="submit"]').click();

    const grantBtn = page.locator('button:has-text("Grant Access")');
    if (await grantBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await grantBtn.click();
    }

    await page.waitForURL(`${server.baseURL}/**`, { timeout: 15_000 });
    expect(page.url()).not.toContain('/login');

    const response = await page.request.get(`${server.baseURL}/api/auth/me`);
    expect(response.ok()).toBeTruthy();

    const body = await response.json();
    expect(body.source).toBe('oidc');
    expect(body.username).toBeTruthy();
  });

  test('/api/auth/me returns source: "oidc" for OIDC users', async ({
    page,
  }) => {
    await page.goto(`${server.baseURL}/login`);

    const oidcBtn = page.locator('.oidc-btn');
    await expect(oidcBtn).toBeVisible({ timeout: 5_000 });
    await oidcBtn.click();

    await page.waitForURL(`${DEX_URL}/**`, { timeout: 10_000 });

    const localConnector = page.locator('a:has-text("Log in with Email")');
    if (await localConnector.isVisible().catch(() => false)) {
      await localConnector.click();
    }

    await page.waitForSelector('input[name="login"], #login', {
      state: 'visible',
      timeout: 10_000,
    });
    await page.fill('input[name="login"], #login', DEX_USER);
    await page.fill('input[name="password"], #password', DEX_PASSWORD);
    await page.locator('button[type="submit"]').click();

    const grantBtn = page.locator('button:has-text("Grant Access")');
    if (await grantBtn.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await grantBtn.click();
    }

    await page.waitForURL(`${server.baseURL}/**`, { timeout: 15_000 });

    const meResponse = await page.request.get(
      `${server.baseURL}/api/auth/me`
    );
    expect(meResponse.ok()).toBeTruthy();

    const meBody = await meResponse.json();
    expect(meBody.source).toBe('oidc');
  });
});
