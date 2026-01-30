import { test, expect } from '@playwright/test';
import {
  TEST_ADMIN_USER,
  TEST_ADMIN_PASSWORD,
  TEST_ADMIN_DISPLAY_NAME,
  TEST_ADMIN_EMAIL,
  TEST_ADMIN_GROUP,
  selectAuthProvider,
} from '../helpers/auth';

test.describe('Setup Wizard', () => {
  test('redirects to /setup on first visit', async ({ page }) => {
    await page.goto('/');
    await page.waitForURL('**/setup');
    expect(page.url()).toContain('/setup');
  });

  test('shows auth provider options', async ({ page }) => {
    await page.goto('/setup');

    await expect(page.locator('.auth-option[data-provider="local"]')).toBeVisible();
    await expect(page.locator('.auth-option[data-provider="ldap"]')).toBeVisible();
    await expect(page.locator('.auth-option[data-provider="oidc"]')).toBeVisible();
    await expect(page.locator('.auth-option[data-provider="proxy"]')).toBeVisible();
  });

  test('next button is disabled until provider selected', async ({ page }) => {
    await page.goto('/setup');

    const nextBtn = page.locator('#btnProvidersNext');
    await expect(nextBtn).toBeDisabled();

    await selectAuthProvider(page, 'local');
    await expect(nextBtn).toBeEnabled();
  });

  test('shows LDAP config step when LDAP selected', async ({ page }) => {
    await page.goto('/setup');

    await selectAuthProvider(page, 'ldap');
    await page.locator('#btnProvidersNext').click();

    await expect(page.locator('#ldapServer')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#ldapBindDN')).toBeVisible();
    await expect(page.locator('#ldapBaseDN')).toBeVisible();
  });

  test('shows OIDC config step when OIDC selected', async ({ page }) => {
    await page.goto('/setup');

    await selectAuthProvider(page, 'oidc');
    await page.locator('#btnProvidersNext').click();

    await expect(page.locator('#oidcIssuer')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#oidcClientID')).toBeVisible();
    await expect(page.locator('#oidcClientSecret')).toBeVisible();
    await expect(page.locator('#oidcRedirectURL')).toBeVisible();
  });

  test('validates password length on setup', async ({ page }) => {
    await page.goto('/setup');

    await selectAuthProvider(page, 'local');
    await page.locator('#btnProvidersNext').click();

    await page.waitForSelector('#adminUsername', { state: 'visible' });
    await page.fill('#adminUsername', TEST_ADMIN_USER);
    await page.fill('#adminPassword', 'short'); // Too short
    await page.fill('#adminDisplayName', TEST_ADMIN_DISPLAY_NAME);
    await page.fill('#adminGroup', TEST_ADMIN_GROUP);

    await page.locator('#completeSetup').click();

    const errorMsg = page.locator('#errorMessage');
    await expect(errorMsg).toBeVisible({ timeout: 5_000 });
  });

  test('completes local auth setup end-to-end', async ({ page }) => {
    await page.goto('/setup');

    await selectAuthProvider(page, 'local');
    await page.locator('#btnProvidersNext').click();

    await page.waitForSelector('#adminUsername', { state: 'visible' });
    await page.fill('#adminUsername', TEST_ADMIN_USER);
    await page.fill('#adminPassword', TEST_ADMIN_PASSWORD);
    await page.fill('#adminDisplayName', TEST_ADMIN_DISPLAY_NAME);
    await page.fill('#adminEmail', TEST_ADMIN_EMAIL);
    await page.fill('#adminGroup', TEST_ADMIN_GROUP);

    await page.locator('#completeSetup').click();

    await page.waitForURL('**/login', { timeout: 10_000 });
    expect(page.url()).toContain('/login');
  });

  test('prevents re-setup after completion', async ({ page }) => {
    await page.goto('/setup');

    // Should redirect away from setup
    await page.waitForURL(/\/(login)?$/, { timeout: 10_000 });
    expect(page.url()).not.toContain('/setup');
  });
});
