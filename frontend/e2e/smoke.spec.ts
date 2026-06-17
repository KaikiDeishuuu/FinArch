import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { turnstile_site_key: '', email_verification_required: false } }),
    })
  })
})

test('redirects unauthenticated app routes to login', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'FinArch' })).toBeVisible()
})

test('renders invalid verification link without backend', async ({ page }) => {
  await page.goto('/verify-email')
  await expect(page.getByRole('heading', { name: /验证失败|Verification failed/i })).toBeVisible()
  await expect(page.getByRole('link', { name: /返回登录|Back to login/i })).toBeVisible()
})

test('renders login on mobile viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'FinArch' })).toBeVisible()
  await expect(page.locator('form').getByRole('button', { name: /登录|Login/i })).toBeVisible()
})

test('renders protected mobile shell with compact top actions', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.addInitScript(() => {
    const payload = { exp: Math.floor(Date.now() / 1000) + 7200 }
    const token = `e30.${btoa(JSON.stringify(payload)).replace(/=/g, '')}.sig`
    localStorage.setItem('finarch_session', JSON.stringify({
      token,
      expiresAt: Date.now() + 7200_000,
      user: { id: 'u1', email: 'demo@example.com', username: 'demo', nickname: 'Demo', role: 'user' },
    }))
    localStorage.setItem('finarch_exchange_rates_v1', JSON.stringify({
      rates: { CNY: 1, USD: 7.26, EUR: 7.84, JPY: 0.0475, GBP: 9.15 },
      date: '2026-01-01',
      fetchedAt: Date.now(),
    }))
  })
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })
  await page.route((url) => url.pathname === '/api/v1/budgets', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/budgets/summary**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { mode: 'work', period_month: '2026-01', total_actual_cents: 0, total_actual_yuan: 0, total_budget: null, category_budgets: [] } }) })
  })
  await page.route('**/api/v1/recurring-rules**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })

  await page.goto('/')
  await expect(page.getByRole('banner')).toBeVisible()
  await expect(page.getByRole('button', { name: /More|更多/ })).toBeVisible()
  await expect(page.getByRole('navigation').filter({ hasText: /Home|概览/ })).toBeVisible()
  await expect(page.getByText(/Budget Progress|预算进度/)).toBeVisible()

  await page.getByRole('button', { name: /More|更多/ }).click()
  const moreDialog = page.getByRole('dialog', { name: /More tools|更多功能/ })
  await expect(moreDialog).toBeVisible()
  await expect(moreDialog.getByRole('link', { name: /Exchange|汇率/ })).toBeVisible()
  await expect(moreDialog.getByRole('link', { name: /Settings|设置/ })).toBeVisible()
  await moreDialog.getByRole('link', { name: /Budgets|预算/ }).click()
  await expect(page.getByRole('heading', { name: /Budgets|预算管理/ })).toBeVisible()
})
