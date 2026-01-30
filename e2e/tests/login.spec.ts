import { test, expect } from '@playwright/test';
import {
  TEST_ADMIN_USER,
  TEST_ADMIN_PASSWORD,
  login,
} from '../helpers/auth';

test.describe('Login', () => {
  test('login page renders correctly', async ({ page }) => {
    await page.goto('/login');

    await expect(page.locator('#loginForm')).toBeVisible();
    await expect(page.locator('#username')).toBeVisible();
    await expect(page.locator('#password')).toBeVisible();
    await expect(page.locator('#loginBtn')).toBeVisible();
  });

  test('successful login redirects to DashGate', async ({ page }) => {
    await page.goto('/login');

    await page.fill('#username', TEST_ADMIN_USER);
    await page.fill('#password', TEST_ADMIN_PASSWORD);
    await page.locator('#loginBtn').click();

    await page.waitForURL('**/', { timeout: 10_000 });
    await expect(page.locator('.greeting')).toBeVisible();
  });

  test('wrong password shows error', async ({ page }) => {
    await page.goto('/login');

    await page.fill('#username', TEST_ADMIN_USER);
    await page.fill('#password', 'wrongpassword');
    await page.locator('#loginBtn').click();

    const errorMsg = page.locator('#errorMessage');
    await expect(errorMsg).toBeVisible({ timeout: 5_000 });
    await expect(errorMsg).toHaveClass(/show/);
  });

  test('already-logged-in redirects to /', async ({ page }) => {
    await login(page);

    await page.goto('/login');
    await page.waitForURL('**/', { timeout: 5_000 });
    expect(page.url()).not.toContain('/login');
  });

  test('OIDC button visibility based on config', async ({ page }) => {
    await page.goto('/login');
    await page.waitForSelector('#loginForm', { state: 'visible' });

    // OIDC section should be hidden since we only configured local auth
    const oidcSection = page.locator('#oidcSection');
    const isVisible = await oidcSection.isVisible().catch(() => false);
    expect(isVisible).toBe(false);
  });
});
