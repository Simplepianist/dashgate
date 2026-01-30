import { test, expect } from '@playwright/test';
import { login } from '../helpers/auth';

test.describe('DashGate UI', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('app categories and apps render', async ({ page }) => {
    // Categories from test-config.yaml: Monitoring, Development
    await expect(page.locator('.category-section')).toHaveCount(2, {
      timeout: 5_000,
    });

    const categoryNames = page.locator('.category-name');
    await expect(categoryNames.nth(0)).toHaveText('Monitoring');
    await expect(categoryNames.nth(1)).toHaveText('Development');

    // Admin user should see all 3 apps
    const appItems = page.locator('.app-item');
    await expect(appItems).toHaveCount(3, { timeout: 5_000 });
  });

  test('search modal opens with Ctrl+K', async ({ page }) => {
    await page.keyboard.press('Control+k');

    const searchModal = page.locator('#searchModal');
    await expect(searchModal).toBeVisible({ timeout: 3_000 });

    const searchInput = page.locator('#searchInput');
    await expect(searchInput).toBeFocused();
  });

  test('search filters results', async ({ page }) => {
    await page.keyboard.press('Control+k');
    await expect(page.locator('#searchModal')).toBeVisible();

    await page.fill('#searchInput', 'Grafana');

    const results = page.locator('#searchResults .search-result');
    await expect(results).toHaveCount(1, { timeout: 3_000 });
    await expect(results.first()).toContainText('Grafana');
  });

  test('search navigates with arrow keys', async ({ page }) => {
    await page.keyboard.press('Control+k');
    await expect(page.locator('#searchModal')).toBeVisible();

    // Clear and wait for all results
    await page.fill('#searchInput', '');

    const results = page.locator('#searchResults .search-result');
    await expect(results.first()).toBeVisible({ timeout: 3_000 });

    await page.keyboard.press('ArrowDown');
    const activeResult = page.locator(
      '#searchResults .search-result.selected'
    );
    await expect(activeResult).toHaveCount(1, { timeout: 2_000 });
  });

  test('search modal closes with Escape', async ({ page }) => {
    await page.keyboard.press('Control+k');
    await expect(page.locator('#searchModal')).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.locator('#searchModal')).not.toBeVisible();
  });

  test('right-click context menu appears', async ({ page }) => {
    const firstApp = page.locator('.app-item').first();
    await firstApp.click({ button: 'right' });

    const contextMenu = page.locator('#contextMenu');
    await expect(contextMenu).toBeVisible({ timeout: 3_000 });

    await expect(page.locator('#contextMenu [data-action="open"]')).toBeVisible();
    await expect(page.locator('#contextMenu [data-action="copy"]')).toBeVisible();
    await expect(page.locator('#contextMenu [data-action="favorite"]')).toBeVisible();
  });

  test('context menu closes on click elsewhere', async ({ page }) => {
    const firstApp = page.locator('.app-item').first();
    await firstApp.click({ button: 'right' });
    await expect(page.locator('#contextMenu')).toBeVisible();

    await page.locator('body').click();
    await expect(page.locator('#contextMenu')).not.toBeVisible();
  });

  test('welcome greeting shows username', async ({ page }) => {
    const greeting = page.locator('.greeting');
    await expect(greeting).toBeVisible();
    const text = await greeting.textContent();
    expect(text).toBeTruthy();
    expect(text!.length).toBeGreaterThan(0);
  });

  test('unauthenticated access redirects to /login', async ({ page }) => {
    await page.context().clearCookies();
    await page.goto('/');

    await page.waitForURL('**/login', { timeout: 5_000 });
    expect(page.url()).toContain('/login');
  });
});
