import { test, expect } from '@playwright/test';
import { login } from '../helpers/auth';

test.describe('Logout', () => {
  test('logout clears session cookie', async ({ page }) => {
    await login(page);
    await expect(page.locator('.greeting')).toBeVisible();

    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();

    const logoutBtn = page.locator('.settings-logout-btn');
    await expect(logoutBtn).toBeVisible();
    await logoutBtn.click();

    await page.waitForURL('**/login', { timeout: 5_000 });
    expect(page.url()).toContain('/login');

    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(
      (c) => c.name === 'dashgate_session'
    );
    expect(
      !sessionCookie ||
        sessionCookie.value === '' ||
        sessionCookie.expires < Date.now() / 1000
    ).toBeTruthy();
  });

  test('post-logout DashGate access redirects to login', async ({
    page,
  }) => {
    await login(page);
    await expect(page.locator('.greeting')).toBeVisible();

    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();
    await page.locator('.settings-logout-btn').click();
    await page.waitForURL('**/login', { timeout: 5_000 });

    await page.goto('/');
    await page.waitForURL('**/login', { timeout: 5_000 });
    expect(page.url()).toContain('/login');
  });
});
