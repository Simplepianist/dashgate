import { test, expect } from '@playwright/test';
import { login } from '../helpers/auth';

test.describe('Settings', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('settings modal opens', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();

    const settingsModal = page.locator('#settingsModal');
    await expect(settingsModal).toBeVisible({ timeout: 3_000 });

    await expect(page.locator('.settings-title')).toHaveText('Settings');
  });

  test('settings modal closes on backdrop click', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();

    // Call closeSettings() directly — backdrop may be obscured by the modal
    await page.evaluate(() => (window as any).closeSettings());
    await expect(page.locator('#settingsModal')).not.toBeVisible();
  });

  test('theme switching works', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();

    await page.locator('[data-tab="general"]').click();

    // Radio inputs are display:none with custom styling.
    // Click the label text instead.
    await page.locator('label:has(input[name="theme"][value="light"])').click();
    await page.waitForTimeout(500);
    const html = page.locator('html');
    const theme = await html.getAttribute('data-theme');
    expect(theme).toBe('light');

    await page.locator('label:has(input[name="theme"][value="dark"])').click();
    await page.waitForTimeout(500);
    const darkTheme = await html.getAttribute('data-theme');
    expect(darkTheme).toBe('dark');
  });

  test('accent color changes', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();

    await page.locator('[data-tab="general"]').click();

    const greenOption = page.locator('.color-option[data-color="green"]');
    await greenOption.click();

    await expect(greenOption).toHaveClass(/active|selected/, {
      timeout: 2_000,
    });
  });

  test('widget visibility toggles', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();

    await page.locator('[data-tab="general"]').click();

    // Scroll to the widget toggle section first
    await page.locator('#showWidgets').scrollIntoViewIfNeeded();

    // The checkbox is hidden; click its label or toggle via JS
    const isChecked = await page.locator('#showWidgets').isChecked();
    await page.evaluate((shouldCheck) => {
      const cb = document.getElementById('showWidgets') as HTMLInputElement;
      cb.checked = shouldCheck;
      cb.dispatchEvent(new Event('change', { bubbles: true }));
    }, !isChecked);
    await page.waitForTimeout(300);

    await page.evaluate(() => (window as any).closeSettings());
    await page.waitForTimeout(300);

    const widgetsSidebar = page.locator('.widgets-sidebar');
    if (!isChecked) {
      await expect(widgetsSidebar).toBeVisible();
    } else {
      await expect(widgetsSidebar).not.toBeVisible();
    }
  });

  test('settings persist after reload', async ({ page }) => {
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();
    await page.locator('[data-tab="general"]').click();

    // Click the label for the light theme radio
    await page.locator('label:has(input[name="theme"][value="light"])').click();
    await page.waitForTimeout(500);

    await page.evaluate(() => (window as any).closeSettings());

    await page.reload();
    await page.waitForSelector('.greeting', { state: 'visible' });

    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBe('light');

    // Reset back to dark
    await page.locator('.dock-item').nth(4).click();
    await expect(page.locator('#settingsModal')).toBeVisible();
    await page.locator('[data-tab="general"]').click();
    await page.locator('label:has(input[name="theme"][value="dark"])').click();
    await page.waitForTimeout(300);
    await page.evaluate(() => (window as any).closeSettings());
  });
});
