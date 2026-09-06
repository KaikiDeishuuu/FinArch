import { expect, test, type Page, type Route } from '@playwright/test'

const refreshRoute = '**/api/v1/auth/refresh'

async function mockAuthenticatedSession(page: Page) {
  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token: 'memory-only-access-token',
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })
}

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { turnstile_site_key: '', email_verification_required: false } }),
    })
  })
  await page.route(refreshRoute, async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'session_invalid', message: 'No session' } }),
    })
  })
})

test('redirects unauthenticated app routes to login', async ({ page }) => {
  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await expect(page.getByRole('heading', { name: 'FinArch' })).toBeVisible()
})

test('keeps session recovery retryable across transient and malformed refresh responses', async ({ page }) => {
  let refreshRequests = 0
  let releaseSuccessfulRefresh!: () => void
  const successfulRefreshGate = new Promise<void>((resolve) => {
    releaseSuccessfulRefresh = resolve
  })

  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    refreshRequests += 1
    if (refreshRequests === 1) {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'system_unavailable', message: 'Try later' } }),
      })
      return
    }
    if (refreshRequests === 2) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: { token: 'incomplete-protocol-response' } }),
      })
      return
    }

    await successfulRefreshGate
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token: 'recovered-access-token',
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: /暂时无法验证会话|couldn't verify your session/i })).toBeVisible()
  await expect(page).toHaveURL(/\/$/)
  await expect.poll(() => refreshRequests).toBe(3)

  releaseSuccessfulRefresh()
  await expect(page.getByRole('button', { name: /Log out|退出登录/i }).first()).toBeVisible()
  await expect(page).toHaveURL(/\/$/)
})

test('stays on recovery after bounded failures and allows a manual retry', async ({ page }) => {
  let refreshRequests = 0
  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    refreshRequests += 1
    if (refreshRequests <= 5) {
      await route.fulfill({
        status: 429,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'session_rate_limited', message: 'Try later' } }),
      })
      return
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token: 'manual-recovery-access-token',
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })

  await page.goto('/')
  const retry = page.getByRole('button', { name: /重新验证会话|Retry session check/i })
  await expect(retry).toBeVisible()
  await expect.poll(() => refreshRequests, { timeout: 12_000 }).toBe(5)

  await page.waitForTimeout(650)
  expect(refreshRequests).toBe(5)
  await expect(page).toHaveURL(/\/$/)
  await expect(retry).toBeVisible()

  await retry.click()
  await expect.poll(() => refreshRequests).toBe(6)
  await expect(page.getByRole('button', { name: /Log out|退出登录/i }).first()).toBeVisible()
})

test('does not refresh a protected request for a business 401', async ({ page }) => {
  let refreshRequests = 0
  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    refreshRequests += 1
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token: 'business-error-access-token',
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })

  let transactionRequests = 0
  await page.route('**/api/v1/transactions**', async (route) => {
    transactionRequests += 1
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'not_authorized', message: 'Business rule denied' } }),
    })
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
  await page.route('**/api/v1/budgets**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/recurring-rules**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })

  await page.goto('/')
  await expect(page.getByRole('button', { name: /Log out|退出登录/i }).first()).toBeVisible()
  await expect.poll(() => transactionRequests).toBeGreaterThan(0)
  await page.waitForTimeout(250)
  expect(refreshRequests).toBe(1)
})

test('does not restore an in-memory session when logout overtakes refresh', async ({ page }) => {
  // Force AuthContext onto its storage-event fallback so the test can deliver
  // the cross-tab event synchronously before releasing the refresh response.
  await page.addInitScript(() => {
    Object.defineProperty(window, 'BroadcastChannel', { configurable: true, value: undefined })
  })

  let releaseRefresh!: () => void
  const refreshGate = new Promise<void>((resolve) => {
    releaseRefresh = resolve
  })
  let refreshStarted = 0
  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    refreshStarted += 1
    await refreshGate
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token: 'refresh-that-must-be-ignored',
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })

  await page.goto('/')
  await expect.poll(() => refreshStarted).toBe(1)
  await page.evaluate(() => {
    localStorage.setItem('finarch_logout_pending', '1')
    window.dispatchEvent(new StorageEvent('storage', {
      key: 'finarch_auth_event',
      newValue: JSON.stringify({ event: 'logout_pending', nonce: 'test' }),
    }))
  })
  releaseRefresh()

  await expect(page).toHaveURL(/\/login$/)
  await expect.poll(() => page.evaluate(() => localStorage.getItem('finarch_logout_pending'))).toBe('1')
})

test('finishes a pending logout when the server reports no session', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('finarch_logout_pending', '1')
  })
  let logoutRequests = 0
  let refreshRequests = 0
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (path === '/api/v1/auth/logout') logoutRequests += 1
    if (path === '/api/v1/auth/refresh') refreshRequests += 1
  })
  await page.route('**/api/v1/auth/logout', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ success: false, error: { code: 'session_invalid', message: 'No session' } }),
    })
  })

  await page.goto('/')
  await expect(page).toHaveURL(/\/login$/)
  await expect.poll(() => page.evaluate(() => localStorage.getItem('finarch_logout_pending'))).toBeNull()
  expect(logoutRequests).toBe(1)
  expect(refreshRequests).toBe(0)
})

test('coalesces concurrent 401 responses into one refresh and retries once', async ({ page }) => {
  let refreshRequests = 0
  await page.unroute(refreshRoute)
  await page.route(refreshRoute, async (route) => {
    refreshRequests += 1
    if (refreshRequests === 2) {
      // Keep the shared refresh in flight long enough for both 401 response
      // interceptors to join the same promise.
      await new Promise((resolve) => setTimeout(resolve, 75))
    }
    const token = refreshRequests === 1 ? 'access-before-401' : 'access-after-refresh'
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          token,
          expires_at: new Date(Date.now() + 15 * 60_000).toISOString(),
          user_id: 'u1',
          email: 'demo@example.com',
          username: 'demo',
          nickname: 'Demo',
          role: 'user',
        },
      }),
    })
  })

  let oldTokenRequests = 0
  let releaseOldTokenRequests!: () => void
  const bothOldTokenRequests = new Promise<void>((resolve) => {
    releaseOldTokenRequests = resolve
  })
  const retryHeaders: string[] = []
  const respondToProtectedList = async (route: Route) => {
    const authorization = route.request().headers().authorization ?? ''
    if (authorization === 'Bearer access-before-401') {
      oldTokenRequests += 1
      if (oldTokenRequests === 2) releaseOldTokenRequests()
      await bothOldTokenRequests
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'access_expired', message: 'Expired' } }),
      })
      return
    }
    retryHeaders.push(authorization)
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  }
  await page.route('**/api/v1/transactions**', respondToProtectedList)
  await page.route('**/api/v1/accounts**', respondToProtectedList)
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })
  await page.route('**/api/v1/budgets**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/recurring-rules**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })

  await page.goto('/')
  await expect.poll(() => retryHeaders.length).toBe(2)
  expect(oldTokenRequests).toBe(2)
  expect(refreshRequests).toBe(2)
  expect(retryHeaders).toEqual(['Bearer access-after-refresh', 'Bearer access-after-refresh'])
})

test('renders invalid verification link without backend', async ({ page }) => {
  await page.goto('/verify-email')
  await expect(page.getByRole('heading', { name: /验证失败|Verification failed/i })).toBeVisible()
  await expect(page.getByRole('link', { name: /返回登录|Back to login/i })).toBeVisible()
})

const oneTimeAccountActions = [
  {
    name: 'email verification',
    pagePath: '/verify-email',
    apiPath: '/api/v1/auth/verify-email',
    button: /确认验证邮箱|Verify Email/i,
  },
  {
    name: 'old-email authorization',
    pagePath: '/confirm-email-change-old',
    apiPath: '/api/v1/auth/confirm-email-change-old',
    button: /授权更换邮箱|Authorize Email Change/i,
  },
  {
    name: 'new-email confirmation',
    pagePath: '/confirm-email-change',
    apiPath: '/api/v1/auth/confirm-email-change',
    button: /确认新邮箱|Confirm New Email/i,
  },
  {
    name: 'account deletion',
    pagePath: '/confirm-delete-account',
    apiPath: '/api/v1/auth/confirm-delete-account',
    button: /永久注销账户|Permanently Delete Account/i,
  },
]

for (const [index, action] of oneTimeAccountActions.entries()) {
  test(`${action.name} requires an explicit click and clears its URL token`, async ({ page }) => {
    await page.route('**/api/v1/auth/logout', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: { message: 'logged out' } }),
      })
    })
    const token = `action-token-${index}`
    let requestCount = 0
    let postedToken = ''
    await page.route(`**${action.apiPath}`, async (route) => {
      requestCount += 1
      postedToken = (route.request().postDataJSON() as { token?: string } | null)?.token ?? ''
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: { message: 'ok' } }),
      })
    })

    await page.goto(`${action.pagePath}#token=${token}`)
    const confirm = page.getByRole('button', { name: action.button })
    await expect(confirm).toBeVisible()
    expect(requestCount).toBe(0)
    expect(await page.evaluate(() => ({ search: location.search, hash: location.hash }))).toEqual({
      search: '',
      hash: '',
    })

    await confirm.click()
    await expect.poll(() => requestCount).toBe(1)
    expect(postedToken).toBe(token)
  })
}

test('password reset clears the URL token but submits it only with the completed form', async ({ page }) => {
  await page.route('**/api/v1/auth/logout', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { message: 'logged out' } }),
    })
  })
  let requestCount = 0
  let requestBody: { token?: string; new_password?: string } = {}
  await page.route('**/api/v1/auth/reset-password', async (route) => {
    requestCount += 1
    requestBody = route.request().postDataJSON() as typeof requestBody
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { message: 'ok' } }),
    })
  })

  await page.goto('/reset-password?token=legacy-reset-token')
  await expect(page.getByRole('heading', { name: /重置密码|Reset Password/i })).toBeVisible()
  expect(requestCount).toBe(0)
  expect(await page.evaluate(() => location.search)).toBe('')

  const passwordInputs = page.locator('input[type=password]')
  await passwordInputs.nth(0).fill('new-password-123')
  await passwordInputs.nth(1).fill('new-password-123')
  await page.getByRole('button', { name: /确认重置|Confirm Reset/i }).click()

  await expect.poll(() => requestCount).toBe(1)
  expect(requestBody).toEqual({
    token: 'legacy-reset-token',
    new_password: 'new-password-123',
  })
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
  await mockAuthenticatedSession(page)
  await page.addInitScript(() => {
    // Older releases persisted a bearer token. Startup must delete it without
    // ever using it for authentication.
    localStorage.setItem('finarch_session', JSON.stringify({ token: 'legacy-secret' }))
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
  await expect.poll(() => page.evaluate(() => localStorage.getItem('finarch_session'))).toBeNull()
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

test('dashboard surfaces unsettled public expenses instead of reporting all clear', async ({ page }) => {
  await mockAuthenticatedSession(page)
  const publicExpense = (id: string, uploaded: boolean) => ({
    id,
    occurred_at: '2026-09-01 09:00:00',
    direction: 'expense',
    source: 'company',
    account_id: 'account-public',
    category: 'office',
    amount_yuan: 50,
    currency: 'CNY',
    base_amount_cents: 5_000,
    base_currency: 'CNY',
    note: id,
    project_id: null,
    reimbursed: false,
    settled: false,
    uploaded,
    mode: 'work',
  })
  await page.route('**/api/v1/transactions**', async (route) => {
    const mode = new URL(route.request().url()).searchParams.get('mode')
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: mode === 'life' ? [] : [publicExpense('public-uploaded', true), publicExpense('public-pending', false)],
      }),
    })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [{ id: 'account-public', name: 'Public Wallet', type: 'public', currency: 'CNY', balance_cents: 100_000, balance_yuan: 1000, is_active: true }],
      }),
    })
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
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { mode: 'work', period_month: '2026-09', total_actual_cents: 0, total_actual_yuan: 0, total_budget: null, category_budgets: [] } }) })
  })
  await page.route('**/api/v1/recurring-rules**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })

  await page.goto('/')
  await expect(page.getByText(/Settlement queue|待核销分析/)).toBeVisible()
  // The all-clear banner must not fire while public expenses are unsettled.
  await expect(page.getByText(/Nothing pending|暂无待办事项/)).toHaveCount(0)
})

test('announcement board carries the support address and can be restored from settings', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { id: 'u1', email: 'demo@example.com', username: 'demo', nickname: 'Demo', pending_email: '', role: 'user' } }),
    })
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

  const boardName = /Welcome to FinArch|欢迎使用 FinArch/
  await page.goto('/')
  const board = page.getByRole('region', { name: boardName })
  await expect(board).toBeVisible()
  await expect(board.getByRole('link', { name: 'support@farc.dev' })).toHaveAttribute('href', 'mailto:support@farc.dev')

  await board.getByRole('button', { name: /Dismiss|不再显示/ }).click()
  await expect(board).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('region', { name: boardName })).toHaveCount(0)

  // Settings keeps the address reachable and can bring the board back, so a
  // dismissal never buries the only route to support.
  await page.goto('/settings')
  await expect(page.getByRole('link', { name: 'support@farc.dev' })).toHaveAttribute('href', 'mailto:support@farc.dev')
  await page.getByRole('button', { name: /Show announcement again|重新显示公告/ }).click()

  await page.goto('/')
  await expect(page.getByRole('region', { name: boardName })).toBeVisible()
})

test('stats view fits the mobile width so the fixed bottom nav stays on screen', async ({ page }) => {
  // A horizontally overflowing element widens the layout viewport on real
  // phones, which drags `position: fixed; bottom: 0` below the visible area and
  // hides the whole tab bar. Desktop browsers never show it, so assert the
  // overflow itself.
  await page.setViewportSize({ width: 390, height: 844 })
  await mockAuthenticatedSession(page)
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: Array.from({ length: 12 }, (_, index) => ({
          id: 'stats-' + index,
          occurred_at: `2026-0${(index % 9) + 1}-01 09:00:00`,
          direction: index % 2 ? 'income' : 'expense',
          source: 'company',
          account_id: 'account-cny',
          category: 'category-' + (index % 5),
          amount_yuan: 10 + index,
          currency: 'CNY',
          base_amount_cents: (10 + index) * 100,
          base_currency: 'CNY',
          note: 'note ' + index,
          project_id: null,
          reimbursed: false,
          uploaded: true,
          mode: 'work',
        })),
      }),
    })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [{ id: 'account-cny', name: 'Company Wallet', type: 'public', currency: 'CNY', balance_cents: 10_000, balance_yuan: 100, is_active: true }],
      }),
    })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })

  await page.goto('/stats')
  await expect(page.getByRole('navigation').filter({ hasText: /Stats|统计/ })).toBeVisible()
  await expect.poll(() => page.evaluate(() => {
    const root = document.documentElement
    return root.scrollWidth - root.clientWidth
  })).toBe(0)
})

test('keeps system operations unavailable when the config flag is absent', async ({ page }) => {
  await mockAuthenticatedSession(page)

  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { id: 'u1', email: 'demo@example.com', username: 'demo', nickname: 'Demo', pending_email: '', role: 'user' } }),
    })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })

  const operationRequests: string[] = []
  page.on('request', (request) => {
    if (/\/(backup|disaster-recovery)(\/|$)/.test(new URL(request.url()).pathname)) {
      operationRequests.push(request.url())
    }
  })

  await page.goto('/settings')
  await expect(page.getByText(/仅限运维环境|Operational access required/)).toHaveCount(2)
  await expect(page.getByRole('button', { name: /下载备份|Download Backup/ })).toHaveCount(0)
  await expect(page.locator('input[type="file"][accept=".db,.zip"]')).toHaveCount(0)
  await expect.poll(() => operationRequests).toEqual([])

  await page.goto('/disaster-restore')
  await expect(page).toHaveURL(/\/$/)
  await expect.poll(() => operationRequests).toEqual([])

  await page.unroute('**/api/v1/config')
  await page.route('**/api/v1/config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { turnstile_site_key: '', email_verification_required: false, system_operations_enabled: true } }),
    })
  })
  await page.goto('/settings')
  await expect(page.getByText(/请使用受控运维客户端|Use a controlled operations client/)).toHaveCount(2)
  await expect(page.getByRole('button', { name: /下载备份|Download Backup/ })).toHaveCount(0)
  await expect(page.locator('input[type="file"][accept=".db,.zip"]')).toHaveCount(0)
  await page.goto('/disaster-restore')
  await expect(page.getByText(/请使用受控运维客户端|Use a controlled operations client/)).toBeVisible()
  await expect.poll(() => operationRequests).toEqual([])
})

test('recovers attachment link failures without creating a duplicate transaction', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.addInitScript(() => {
    localStorage.setItem('finarch_exchange_rates_v1', JSON.stringify({
      rates: { CNY: 1, USD: 7.26, EUR: 7.84, JPY: 0.0475, GBP: 9.15 },
      date: '2026-01-01',
      fetchedAt: Date.now(),
    }))
  })

  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [{ id: 'account-usd', name: 'USD Wallet', type: 'public', currency: 'USD', balance_cents: 1234, balance_yuan: 12.34, is_active: true }],
      }),
    })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })
  await page.route('**/api/v1/attachments', async (route) => {
    await route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: {
          id: 'attachment-1',
          transaction_id: null,
          storage_key: 'attachment-1',
          original_filename: 'receipt.png',
          content_type: 'image/png',
          size_bytes: 3,
          sha256: 'test',
          kind: 'receipt',
          ocr_status: 'done',
          ocr_provider: 'test',
          ocr_text: 'Total: 10.00',
          ocr_json: null,
          ocr_result: { provider: 'test', text: 'Total: 10.00', suggestion: {} },
          ocr_error: null,
          created_at: '2026-01-01 00:00:00',
          updated_at: '2026-01-01 00:00:00',
        },
      }),
    })
  })

  let createCalls = 0
  let idempotencyKey = ''
  await page.route('**/api/v1/transactions', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.fallback()
      return
    }
    createCalls += 1
    idempotencyKey = route.request().headers()['idempotency-key'] ?? ''
    await route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { id: 'transaction-1' } }),
    })
  })

  let linkCalls = 0
  await page.route('**/api/v1/attachments/attachment-1/link', async (route) => {
    linkCalls += 1
    if (linkCalls === 1) {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'TEST', message: 'temporary failure' } }),
      })
      return
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { id: 'attachment-1', transaction_id: 'transaction-1' } }),
    })
  })

  await page.goto('/add')
  await expect(page.getByText(/USD Wallet/)).toContainText('$12.34')

  await page.locator('input[type=file]').setInputFiles({
    name: 'receipt.png',
    mimeType: 'image/png',
    buffer: Buffer.from([1, 2, 3]),
  })
  await page.getByRole('button', { name: /^上传$|^Upload$/ }).click()
  await expect(page.getByText(/查看 OCR 原文|View OCR text/)).toBeVisible()
  await expect(page.getByRole('heading', { name: /确认 OCR 建议|Review OCR suggestions/ })).toHaveCount(0)

  await page.locator('input[type=number]').fill('10')
  await page.getByRole('button', { name: /^保存$|^Save$/ }).click()

  await expect(page.getByText(/交易已经保存，不会重复记账|transaction is already saved/i)).toBeVisible()
  expect(createCalls).toBe(1)
  expect(idempotencyKey).not.toBe('')

  await page.getByRole('button', { name: /重试关联附件|Retry attachment links/ }).click()
  await expect(page.getByText(/添加成功，即将跳转|Added successfully, redirecting/)).toBeVisible()
  expect(createCalls).toBe(1)
  expect(linkCalls).toBe(2)
})

test('reuses the transaction idempotency key after an ambiguous 502 response', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [{
          id: 'account-cny',
          name: 'Company Wallet',
          type: 'public',
          currency: 'CNY',
          balance_cents: 10_000,
          balance_yuan: 100,
          is_active: true,
        }],
      }),
    })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })

  const keys: string[] = []
  await page.route('**/api/v1/transactions', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.fallback()
      return
    }
    keys.push(route.request().headers()['idempotency-key'] ?? '')
    if (keys.length === 1) {
      await route.fulfill({
        status: 502,
        contentType: 'application/json',
        body: JSON.stringify({
          success: false,
          error: { code: 'bad_gateway', message: 'The upstream response was lost' },
        }),
      })
      return
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { id: 'transaction-replayed' } }),
    })
  })

  await page.goto('/add')
  await page.locator('input[type=number]').fill('10')
  const save = page.getByRole('button', { name: /^保存$|^Save$/ })
  await save.click()
  await expect(page.getByRole('main').getByText('The upstream response was lost')).toBeVisible()
  await save.click()

  await expect(page.getByText(/添加成功，即将跳转|Added successfully, redirecting/)).toBeVisible()
  expect(keys).toHaveLength(2)
  expect(keys[0]).not.toBe('')
  expect(keys[1]).toBe(keys[0])
})

test('warns when browser-side subset matching is truncated', async ({ page }) => {
  await mockAuthenticatedSession(page)
  const transactions = Array.from({ length: 25 }, (_, index) => ({
    id: 'match-' + index,
    occurred_at: '2026-01-01 12:00:00',
    direction: 'expense',
    source: 'personal',
    account_id: 'account-cny',
    category: 'office',
    amount_yuan: 1,
    currency: 'CNY',
    base_amount_cents: 100,
    base_currency: 'CNY',
    note: '',
    project_id: null,
    reimbursed: false,
    uploaded: true,
    mode: 'work',
  }))
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: transactions }),
    })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [{
          id: 'account-cny',
          name: 'Personal Wallet',
          type: 'personal',
          currency: 'CNY',
          balance_cents: 10_000,
          balance_yuan: 100,
          is_active: true,
        }],
      }),
    })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })

  await page.goto('/match')
  await page.locator('input[type=number]').first().fill('1')
  await page.getByRole('button', { name: /开始匹配|Start Match/ }).click()
  await expect(page.getByTestId('match-truncated-warning')).toContainText(
    /缩小账户或类别范围|Narrow the account or category/i,
  )
})

test('match filters reset across modes and only expose currently eligible personal expenses', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.addInitScript(() => {
    localStorage.setItem('finarch-lang', 'en')
    localStorage.setItem('finarch_mode', 'work')
  })

  const transaction = (
    id: string,
    mode: 'work' | 'life',
    accountId: string,
    category: string,
    amount: number,
  ) => ({
    id,
    occurred_at: '2026-09-01 09:00:00',
    direction: 'expense',
    source: 'personal',
    account_id: accountId,
    category,
    amount_yuan: amount,
    currency: 'CNY',
    base_amount_cents: amount * 100,
    base_currency: 'CNY',
    note: id,
    project_id: null,
    reimbursed: mode === 'life',
    uploaded: true,
    mode,
  })
  const workTransactions = [
    transaction('work-a', 'work', 'work-account-a', 'Work-only', 5),
    transaction('work-b', 'work', 'work-account-b', 'Work-secondary', 6),
    { ...transaction('work-company-decoy', 'work', 'work-account-a', 'Company-decoy', 50), source: 'company' },
    { ...transaction('work-upload-decoy', 'work', 'work-account-a', 'Upload-decoy', 50), uploaded: false },
  ]
  const lifeTransactions = [
    transaction('life-a', 'life', 'life-account-a', 'Life-only', 7),
    transaction('life-b', 'life', 'life-account-b', 'Life-secondary', 8),
  ]

  await page.route('**/api/v1/transactions**', async (route) => {
    const mode = new URL(route.request().url()).searchParams.get('mode')
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: mode === 'life' ? lifeTransactions : workTransactions }),
    })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [
          { id: 'work-account-a', name: 'Work Wallet A', type: 'personal', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true },
          { id: 'work-account-b', name: 'Work Wallet B', type: 'personal', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true },
          { id: 'life-account-a', name: 'Life Wallet A', type: 'personal', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true },
          { id: 'life-account-b', name: 'Life Wallet B', type: 'personal', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true },
        ],
      }),
    })
  })

  await page.goto('/match')
  const accountFilter = page.getByRole('combobox').filter({ hasText: 'Account' })
  await accountFilter.click()
  await page.getByRole('option', { name: 'Work Wallet A' }).click()
  const categoryFilter = page.getByRole('combobox').filter({ hasText: 'Category' })
  await categoryFilter.click()
  await expect(page.getByRole('option', { name: 'Company-decoy' })).toHaveCount(0)
  await expect(page.getByRole('option', { name: 'Upload-decoy' })).toHaveCount(0)
  await page.getByRole('option', { name: 'Work-only' }).click()

  await page.getByRole('button', { name: 'LIFE', exact: true }).click()
  await expect(page.getByRole('combobox').filter({ hasText: 'Category' })).toBeVisible()
  await expect(page.getByText('Work-only', { exact: true })).toHaveCount(0)
  await expect(page.getByText('Work Wallet A', { exact: true })).toHaveCount(0)

  await page.locator('input[type=number]').first().fill('7')
  await page.getByRole('button', { name: 'Start Match' }).click()
  await expect(page.getByText('Found 1 combinations')).toBeVisible()
})

test('work transaction ledger can reveal personal advances and return to public funds', async ({ page }) => {
  await mockAuthenticatedSession(page)
  const transactions = [
    {
      id: 'company-ledger-entry',
      occurred_at: '2026-09-01 09:00:00',
      direction: 'expense',
      source: 'company',
      account_id: 'company-account',
      category: 'office',
      amount_yuan: 20,
      currency: 'CNY',
      base_amount_cents: 2_000,
      base_currency: 'CNY',
      note: 'PUBLIC-LEDGER-ONLY',
      project_id: null,
      reimbursed: false,
      uploaded: false,
      mode: 'work',
    },
    {
      id: 'personal-ledger-entry',
      occurred_at: '2026-09-02 10:00:00',
      direction: 'expense',
      source: 'personal',
      account_id: 'personal-account',
      category: 'travel',
      amount_yuan: 30,
      currency: 'CNY',
      base_amount_cents: 3_000,
      base_currency: 'CNY',
      note: 'PERSONAL-ADVANCE-ONLY',
      project_id: null,
      reimbursed: false,
      uploaded: true,
      mode: 'work',
    },
  ]
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: transactions }) })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: [
          { id: 'company-account', name: 'Public Wallet', type: 'public', currency: 'CNY', balance_cents: 8_000, balance_yuan: 80, is_active: true },
          { id: 'personal-account', name: 'Personal Wallet', type: 'personal', currency: 'CNY', balance_cents: -3_000, balance_yuan: -30, is_active: true },
        ],
      }),
    })
  })

  await page.goto('/transactions?source=personal')
  await expect(page.getByText('PERSONAL-ADVANCE-ONLY').last()).toBeVisible()
  await expect(page.getByText('PUBLIC-LEDGER-ONLY')).toHaveCount(0)

  await page.getByRole('button', { name: /公共账户|Public Account/ }).click()
  await expect(page).toHaveURL(/\/transactions\?source=company$/)
  await expect(page.getByText('PUBLIC-LEDGER-ONLY').last()).toBeVisible()
  await expect(page.getByText('PERSONAL-ADVANCE-ONLY')).toHaveCount(0)
})

test('transaction add links preserve source and life mode always remounts a personal form', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.addInitScript(() => {
    localStorage.setItem('finarch-lang', 'en')
    localStorage.setItem('finarch_mode', 'work')
  })
  await page.route('**/api/v1/transactions**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [] }) })
  })
  await page.route('**/api/v1/accounts**', async (route) => {
    const mode = new URL(route.request().url()).searchParams.get('mode')
    const account = mode === 'life'
      ? { id: 'personal-account', name: 'Personal Wallet', type: 'personal', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true }
      : { id: 'company-account', name: 'Public Wallet', type: 'public', currency: 'CNY', balance_cents: 0, balance_yuan: 0, is_active: true }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: [account] }) })
  })

  await page.goto('/transactions?source=personal')
  await page.getByRole('main').getByRole('link', { name: 'Add', exact: true }).click()
  await expect(page).toHaveURL(/\/add\?source=personal$/)
  await expect(page.getByRole('button', { name: 'Personal Account', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('combobox').filter({ hasText: 'Personal Wallet' })).toBeVisible()

  await page.goto('/transactions?source=company')
  await page.getByRole('main').getByRole('link', { name: 'Add', exact: true }).click()
  await expect(page).toHaveURL(/\/add\?source=company$/)
  await expect(page.getByRole('button', { name: 'Public Account', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('combobox').filter({ hasText: 'Public Wallet' })).toBeVisible()

  await page.getByRole('button', { name: 'LIFE', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Personal Account', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.getByRole('button', { name: 'Public Account', exact: true })).toBeDisabled()
  await expect(page.getByRole('combobox').filter({ hasText: 'Personal Wallet' })).toBeVisible()
  await expect(page.getByRole('combobox').filter({ hasText: 'Public Wallet' })).toHaveCount(0)
})

test('creates a work personal advance with a life account and opens it in pending reimbursements', async ({ page }) => {
  await mockAuthenticatedSession(page)
  await page.addInitScript(() => {
    localStorage.setItem('finarch-lang', 'en')
    localStorage.setItem('finarch_mode', 'work')
  })

  const accountModes: Array<string | null> = []
  await page.route('**/api/v1/accounts**', async (route) => {
    const mode = new URL(route.request().url()).searchParams.get('mode')
    accountModes.push(mode)
    const accounts = mode === 'life'
      ? [{ id: 'life-personal-account', name: 'Life Personal Wallet', type: 'personal', currency: 'CNY', balance_cents: 12_000, balance_yuan: 120, is_active: true }]
      : [{ id: 'work-public-account', name: 'Work Public Wallet', type: 'public', currency: 'CNY', balance_cents: 50_000, balance_yuan: 500, is_active: true }]
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: accounts }),
    })
  })

  let createPayload: Record<string, unknown> | null = null
  let createdTransaction: Record<string, unknown> | null = null
  const transactionListModes: Array<string | null> = []
  await page.route('**/api/v1/transactions**', async (route) => {
    if (route.request().method() === 'POST') {
      createPayload = route.request().postDataJSON() as Record<string, unknown>
      createdTransaction = {
        ...createPayload,
        id: 'created-personal-advance',
        project_id: createPayload.project_id ?? null,
        base_amount_cents: 4_250,
        base_currency: 'CNY',
        reimbursed: false,
        uploaded: false,
      }
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: createdTransaction }),
      })
      return
    }

    const mode = new URL(route.request().url()).searchParams.get('mode')
    transactionListModes.push(mode)
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        data: mode === 'work' && createdTransaction ? [createdTransaction] : [],
      }),
    })
  })
  await page.route('**/api/v1/auth/heartbeat', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: {} }) })
  })
  await page.route('**/api/v1/auth/devices/online', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true, data: { count: 1 } }) })
  })

  await page.goto('/add')
  await expect(page.getByRole('combobox').filter({ hasText: 'Work Public Wallet' })).toBeVisible()

  const source = page.getByRole('group', { name: 'Source' })
  await source.getByRole('button', { name: 'Personal Account', exact: true }).click()
  await expect.poll(() => accountModes.includes('life')).toBe(true)
  await expect(page.getByRole('combobox').filter({ hasText: 'Life Personal Wallet' })).toBeVisible()

  await page.getByPlaceholder('0.00').fill('42.50')
  await page.getByPlaceholder('Enter description').fill('E2E-PERSONAL-ADVANCE')
  await page.getByRole('button', { name: 'Save', exact: true }).click()

  await expect.poll(() => createPayload).not.toBeNull()
  expect(createPayload).toMatchObject({
    mode: 'work',
    direction: 'expense',
    source: 'personal',
    account_id: 'life-personal-account',
    amount_yuan: 42.5,
    note: 'E2E-PERSONAL-ADVANCE',
  })
  await expect.poll(() => {
    const url = new URL(page.url())
    return `${url.pathname}${url.search}`
  }).toBe('/transactions?source=personal')

  expect(transactionListModes).toContain('work')
  const transactionRow = page.locator('article').filter({ hasText: 'E2E-PERSONAL-ADVANCE' })
  await expect(transactionRow).toBeVisible()
  await expect(transactionRow).toContainText('Life Personal Wallet')
  await page.getByRole('button', { name: 'Pending 1', exact: true }).click()
  await expect(transactionRow).toBeVisible()
})
