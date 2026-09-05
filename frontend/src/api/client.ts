import axios from 'axios'
import { isDefinitiveSessionInvalid, isRefreshableAccessFailure } from '../utils/sessionRecovery'
import {
  parseAuthSessionPayload,
  SessionCookieLockUnavailableError,
  SESSION_REQUEST_TIMEOUT_MS,
  withAbortableSessionLock,
  withAbortDeadline,
  withStorageSessionLock,
} from '../utils/sessionLifecycle'
import type { AuthSessionPayload } from '../utils/sessionLifecycle'

// Axios instance with base URL and auth header injection
const client = axios.create({
  baseURL: '/api/v1',
  // Refresh credentials are HttpOnly cookies and are useful only on the
  // same-origin session endpoints. The API intentionally does not support
  // credentialed cross-origin browser sessions.
  withCredentials: true,
})

// Access tokens live only in this JavaScript process. Reload recovery uses the
// opaque HttpOnly refresh cookie, never localStorage.
let _token: string | null = null
let _sessionListener: ((session: AuthResponse | null) => void) | null = null
let _refreshPromise: Promise<AuthResponse> | null = null
let _refreshController: AbortController | null = null
let _logoutPromise: Promise<void> | null = null
let _sessionGeneration = 0
const SESSION_COOKIE_LOCK = 'finarch-session-cookie'

export function setToken(token: string | null) {
  // Clearing a session is also a cancellation fence. A refresh response that
  // was requested before logout must never restore an access token in memory.
  if (token === null) {
    _sessionGeneration += 1
    _refreshController?.abort()
  }
  _token = token
}

export function getToken(): string | null {
  return _token
}

export function setSessionListener(listener: ((session: AuthResponse | null) => void) | null) {
  _sessionListener = listener
}

client.interceptors.request.use((config) => {
  if (_token) {
    config.headers.Authorization = `Bearer ${_token}`
  }
  return config
})

function isSessionLifecycleRequest(url: string | undefined): boolean {
  if (!url) return false
  const path = url.split('?', 1)[0].replace(/\/$/, '')
  return ['/auth/login', '/auth/register', '/auth/refresh', '/auth/logout']
    .some((endpoint) => path.endsWith(endpoint))
}

async function withSessionCookieLock<T>(operation: () => Promise<T>, signal: AbortSignal): Promise<T> {
  signal.throwIfAborted()
  if (typeof navigator !== 'undefined' && navigator.locks) {
    return withAbortableSessionLock(navigator.locks, SESSION_COOKIE_LOCK, signal, operation)
  }
  if (typeof window === 'undefined') {
    return operation()
  }
  let storage: Storage
  try {
    storage = window.localStorage
  } catch {
    throw new SessionCookieLockUnavailableError()
  }
  return withStorageSessionLock(storage, SESSION_COOKIE_LOCK, signal, operation)
}

client.interceptors.response.use(
  (response) => response,
  async (error) => {
    const envelopeMessage = error.response?.data?.error?.message
    if (envelopeMessage && error.response?.data && !error.response.data.message) {
      error.response.data.message = envelopeMessage
    }

    const original = error.config as (typeof error.config & { _finarchRetried?: boolean }) | undefined
    const shouldRefresh = isRefreshableAccessFailure(error) &&
      !!_token &&
      !!original &&
      !original._finarchRetried &&
      !isSessionLifecycleRequest(original.url)

    if (shouldRefresh) {
      original._finarchRetried = true
      try {
        await refreshSession()
        return client.request(original)
      } catch (refreshError) {
        if (isDefinitiveSessionInvalid(refreshError)) {
          setToken(null)
          _sessionListener?.(null)
        }
        return Promise.reject(refreshError)
      }
    }

    // A retried request must never start another refresh loop. If the server
    // still explicitly rejects its session, clear the local access state.
    if (original?._finarchRetried && !!_token && isDefinitiveSessionInvalid(error)) {
      setToken(null)
      _sessionListener?.(null)
    }

    return Promise.reject(error)
  }
)

// ─── Auth ─────────────────────────────────────────────────────────────────────

export interface LoginRequest {
  email: string
  password: string
  captcha_token?: string
}

export interface RegisterRequest {
  email: string
  username: string
  password: string
  nickname?: string
  captcha_token?: string
}

export type AuthResponse = AuthSessionPayload

function parseAuthResponse(payload: unknown): AuthResponse {
  return parseAuthSessionPayload(payload)
}

export async function login(req: LoginRequest): Promise<AuthResponse> {
  return withAbortDeadline(async (signal) => {
    const { data } = await withSessionCookieLock(
      () => client.post('/auth/login', req, { signal, timeout: SESSION_REQUEST_TIMEOUT_MS }),
      signal,
    )
    return parseAuthResponse(data.data)
  }, SESSION_REQUEST_TIMEOUT_MS)
}

class SessionRefreshSupersededError extends Error {
  constructor() {
    super('Session refresh was superseded')
    this.name = 'SessionRefreshSupersededError'
  }
}

export function isSessionRefreshSuperseded(error: unknown): boolean {
  return error instanceof SessionRefreshSupersededError
}

function assertCurrentSessionGeneration(generation: number) {
  if (generation !== _sessionGeneration) throw new SessionRefreshSupersededError()
}

async function rotateSession(generation: number, signal: AbortSignal): Promise<AuthResponse> {
  assertCurrentSessionGeneration(generation)
  let data: unknown
  try {
    const response = await client.post('/auth/refresh', undefined, {
      signal,
      timeout: SESSION_REQUEST_TIMEOUT_MS,
    })
    data = response.data
  } catch (error) {
    // If logout won the race, surface cancellation rather than a refresh
    // failure that could clear the logout-pending fence in AuthContext.
    assertCurrentSessionGeneration(generation)
    throw error
  }
  assertCurrentSessionGeneration(generation)
  const session = parseAuthResponse((data as { data?: unknown } | null)?.data)
  setToken(session.token)
  _sessionListener?.(session)
  return session
}

/**
 * Rotates the HttpOnly refresh cookie and obtains a short-lived access token.
 * One promise protects this tab; Web Locks or the storage bakery fallback
 * serialise cookie mutations across tabs.
 */
export function refreshSession(): Promise<AuthResponse> {
  if (_refreshPromise) return _refreshPromise

  const generation = _sessionGeneration
  const controller = new AbortController()
  _refreshController = controller
  const run = async () => {
    try {
      return await withAbortDeadline(
        (signal) => withSessionCookieLock(() => rotateSession(generation, signal), signal),
        SESSION_REQUEST_TIMEOUT_MS,
        controller.signal,
      )
    } catch (error) {
      assertCurrentSessionGeneration(generation)
      throw error
    }
  }
  _refreshPromise = run().finally(() => {
    if (_refreshController === controller) _refreshController = null
    _refreshPromise = null
  })
  return _refreshPromise
}

export async function logoutSession(): Promise<void> {
  if (_logoutPromise) return _logoutPromise
  _logoutPromise = withAbortDeadline(async (signal) => {
    // Abort and settle this tab's refresh before queuing logout so this context
    // cannot deadlock on its own cookie lock or apply an older response last.
    const refresh = _refreshPromise
    _refreshController?.abort()
    if (refresh) await refresh.catch(() => undefined)
    signal.throwIfAborted()
    await withSessionCookieLock(
      () => client.post('/auth/logout', undefined, {
        signal,
        timeout: SESSION_REQUEST_TIMEOUT_MS,
      }).then(() => undefined),
      signal,
    )
  }, SESSION_REQUEST_TIMEOUT_MS)
    .finally(() => {
      _logoutPromise = null
    })
  return _logoutPromise
}

export interface RegisterResponse {
  // 201: auto-login (no email verification required)
  token?: string
  expires_at?: string
  user_id?: string
  email?: string
  username?: string
  nickname?: string
  role?: string
  // 202: email verification sent
  message?: string
}

export async function register(req: RegisterRequest): Promise<RegisterResponse> {
  return withAbortDeadline(async (signal) => {
    const resp = await withSessionCookieLock(
      () => client.post('/auth/register', req, { signal, timeout: SESSION_REQUEST_TIMEOUT_MS }),
      signal,
    )
    const payload = resp.data.data ?? resp.data
    if (payload && typeof payload.token === 'string') return parseAuthResponse(payload)
    return payload as RegisterResponse
  }, SESSION_REQUEST_TIMEOUT_MS)
}

export async function forgotPassword(email: string): Promise<void> {
  await client.post('/auth/forgot-password', { email })
}

export async function resetPassword(token: string, newPassword: string): Promise<void> {
  await client.post('/auth/reset-password', { token, new_password: newPassword })
}

export async function verifyEmail(token: string): Promise<void> {
  await client.post('/auth/verify-email', { token })
}

export async function resendVerification(email: string): Promise<void> {
  await client.post('/auth/resend-verification', { email })
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await client.post('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
  })
}

export async function requestDeleteAccount(): Promise<void> {
  await client.post('/auth/request-delete-account')
}

export async function confirmDeleteAccount(token: string): Promise<void> {
  await client.post('/auth/confirm-delete-account', { token })
}

export async function requestEmailChange(newEmail: string, currentPassword: string): Promise<void> {
  await client.post('/auth/request-email-change', { new_email: newEmail, current_password: currentPassword })
}

export async function confirmEmailChange(token: string): Promise<void> {
  await client.post('/auth/confirm-email-change', { token })
}

export async function confirmOldEmailForChange(token: string): Promise<void> {
  await client.post('/auth/confirm-email-change-old', { token })
}

export interface UserProfile {
  id: string
  email: string
  username: string
  nickname: string
  pending_email: string
  role: string
}

export async function getMe(): Promise<UserProfile> {
  const { data } = await client.get('/auth/me')
  return data.data as UserProfile
}

export async function updateNickname(nickname: string): Promise<void> {
  await client.patch('/auth/nickname', { nickname })
}

export interface AppConfig {
  turnstile_site_key: string
  captcha_enabled?: boolean
  email_verification_required: boolean
  system_operations_enabled?: boolean
}

export async function getAppConfig(): Promise<AppConfig> {
  const { data } = await client.get('/config')
  return data.data as AppConfig
}

// ─── Transactions ─────────────────────────────────────────────────────────────

export type AppMode = "work" | "life"

export interface Transaction {
  mode: AppMode

  id: string
  occurred_at: string
  transaction_time?: number
  created_at?: string
  updated_at?: string
  reported_at?: string | null
  reimbursed_at?: string | null
  direction: 'income' | 'expense'
  source: 'company' | 'personal'
  account_id: string
  category: string
  amount_yuan: number
  currency: string
  base_amount_cents?: number
  base_currency?: string
  exchange_rate?: number
  exchange_rate_source?: string
  exchange_rate_at?: number
  note: string
  project_id: string | null
  reimbursed: boolean
  uploaded: boolean
  attachment_key?: string | null
  has_attachment?: boolean
  recurring_rule_id?: string | null
  recurring_occurrence_date?: string | null
}

export interface CreateTransactionRequest {
  occurred_at: string // YYYY-MM-DD HH:mm:ss
  direction: string
  source: string
  account_id?: string
  category: string
  amount_yuan: number
  currency?: string
  note?: string
  project_id?: string
}

export async function listTransactions(mode: AppMode = "work"): Promise<Transaction[]> {
  const { data } = await client.get('/transactions', { params: { mode } })
  return data.data
}

export async function createTransaction(
  req: CreateTransactionRequest & { mode?: AppMode },
  idempotencyKey?: string,
): Promise<Transaction> {
  const { data } = await client.post('/transactions', req, {
    headers: idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : undefined,
  })
  return data.data
}

export async function toggleReimbursed(id: string): Promise<Transaction> {
  const { data } = await client.patch(`/transactions/${id}/reimburse`)
  return data.data
}

export async function toggleUploaded(id: string): Promise<{ id: string; uploaded: boolean }> {
  const { data } = await client.patch(`/transactions/${id}/upload`)
  return data.data
}

// ─── Accounts ─────────────────────────────────────────────────────────────────

export interface Account {
  id: string
  name: string
  type: 'personal' | 'public'
  currency: string
  balance_cents: number
  balance_yuan: number
  is_active: boolean
}

export async function listAccounts(mode: AppMode = "work"): Promise<Account[]> {
  const { data } = await client.get('/accounts', { params: { mode } })
  return data.data
}

export async function createAccount(
  name: string,
  type: 'personal' | 'public',
  mode: AppMode,
  currency = 'CNY'
): Promise<Account> {
  const { data } = await client.post('/accounts', { name, type, currency, mode })
  return data.data
}

export async function renameAccount(id: string, name: string): Promise<void> {
  await client.patch(`/accounts/${id}`, { name })
}

export async function deleteAccount(id: string): Promise<void> {
  await client.delete(`/accounts/${id}`)
}

// ─── Match ────────────────────────────────────────────────────────────────────

export interface MatchResultItem {
  id: string
  occurred_at: string
  transaction_time?: number
  created_at?: string
  updated_at?: string
  reported_at?: string | null
  reimbursed_at?: string | null
  direction: string
  source: string
  category: string
  amount_yuan: number
  currency: string
  base_amount_cents?: number
  base_currency?: string
  exchange_rate?: number
  exchange_rate_source?: string
  exchange_rate_at?: number
  note: string
  project_id: string
  uploaded: boolean
}

export interface MatchResult {
  ids: string[]
  total: number
  error: number
  project_count: number
  item_count: number
  items?: MatchResultItem[]
  // V2 fields from integer-cent backend
  total_cents?: number
  error_cents?: number
  score?: number
  time_pruned?: boolean
}

export async function matchSubsetSum(
  target: number,
  tolerance: number,
  maxItems: number
): Promise<MatchResult[]> {
  const targetCents = Math.round(target * 100)
  const toleranceCents = Math.round(tolerance * 100)
  const { data } = await client.post('/match/subset-sum', {
    target_cents: targetCents,
    tolerance_cents: toleranceCents,
    target_yuan: target,
    tolerance_yuan: tolerance,
    max_items: maxItems,
  })
  return data.data
}

// ─── Stats ────────────────────────────────────────────────────────────────────

export interface PoolBalance {
  company_balance: number
  personal_outstanding: number
}

export interface MonthlyStat {
  year: number
  month: number
  income: number
  expense: number
  reimbursed: number // 已报销的个人垫付金额
}

export interface CategoryStat {
  category: string
  total: number
  count: number
}

export interface ProjectStat {
  project_id: string
  project_name: string
  income: number
  expense: number
  net: number
}

export interface AccountBalanceHistoryPoint {
  date: string
  balance: number
}

export async function getStatsSummary(): Promise<PoolBalance> {
  const { data } = await client.get('/stats/summary')
  return data.data
}

export async function getStatsMonthly(year?: number): Promise<MonthlyStat[]> {
  const params = year ? { year } : {}
  const { data } = await client.get('/stats/monthly', { params })
  return data.data
}

export async function getStatsByCategory(dateFrom?: string, dateTo?: string): Promise<CategoryStat[]> {
  const params: Record<string, string> = {}
  if (dateFrom) params.date_from = dateFrom
  if (dateTo) params.date_to = dateTo
  const { data } = await client.get('/stats/by-category', { params })
  return data.data
}

export async function getStatsByProject(): Promise<ProjectStat[]> {
  const { data } = await client.get('/stats/by-project')
  return data.data
}

export async function getAccountBalanceHistory(
  mode: AppMode,
  range: '7d' | '30d' | '90d' | '1y' | 'all',
  accountId?: string
): Promise<AccountBalanceHistoryPoint[]> {
  const params: Record<string, string> = { mode, range }
  if (accountId) params.account_id = accountId
  const { data } = await client.get('/stats/account-balance-history', { params })
  return data.data
}

// ─── Budgets ──────────────────────────────────────────────────────────────────

export interface Budget {
  id: string
  mode: AppMode
  period_month: string
  category: string
  amount_cents: number
  amount_yuan: number
  currency: string
  base_currency: string
  base_amount_cents: number
  base_amount_yuan: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface BudgetProgress {
  budget: Budget
  actual_cents: number
  actual_yuan: number
  remaining_cents: number
  remaining_yuan: number
  usage_ratio: number
  status: 'ok' | 'warning' | 'over'
}

export interface BudgetSummary {
  mode: AppMode
  period_month: string
  total_actual_cents: number
  total_actual_yuan: number
  total_budget: BudgetProgress | null
  category_budgets: BudgetProgress[]
}

export interface UpsertBudgetRequest {
  mode?: AppMode
  period_month?: string
  category?: string
  amount_cents?: number
  amount_yuan?: number
  currency?: string
  base_currency?: string
  base_amount_cents?: number
}

export async function listBudgets(mode: AppMode = 'work', period: string): Promise<Budget[]> {
  const { data } = await client.get('/budgets', { params: { mode, period } })
  return data.data
}

export async function createBudget(req: UpsertBudgetRequest & { mode: AppMode }): Promise<Budget> {
  const { data } = await client.post('/budgets', req)
  return data.data
}

export async function updateBudget(id: string, req: UpsertBudgetRequest): Promise<Budget> {
  const { data } = await client.patch(`/budgets/${id}`, req)
  return data.data
}

export async function deleteBudget(id: string): Promise<void> {
  await client.delete(`/budgets/${id}`)
}

export async function getBudgetSummary(mode: AppMode = 'work', period: string): Promise<BudgetSummary> {
  const { data } = await client.get('/budgets/summary', { params: { mode, period } })
  return data.data
}


// ─── Recurring Transactions ───────────────────────────────────────────────────

export type RecurringFrequency = 'daily' | 'weekly' | 'monthly' | 'yearly'
export type RecurringRuleStatus = 'active' | 'paused' | 'ended'
export type MonthEndPolicy = 'clamp' | 'skip'

export interface RecurringRule {
  id: string
  mode: AppMode
  name: string
  status: RecurringRuleStatus
  account_id: string
  type: 'income' | 'expense'
  direction: 'income' | 'expense'
  category: string
  amount_cents: number
  amount_yuan: number
  currency: string
  exchange_rate: number
  note: string
  project_id: string | null
  frequency: RecurringFrequency
  interval: number
  start_date: string
  end_date: string | null
  time_of_day: string
  timezone: string
  day_of_week: number | null
  day_of_month: number | null
  month_end_policy: MonthEndPolicy
  next_run_at: number
  next_occurred_at: string
  last_generated_for: string | null
  catch_up_enabled: boolean
  created_at: string
  updated_at: string
}

export interface RecurringInstance {
  id: string
  rule_id: string
  occurrence_date: string
  scheduled_at: number
  occurred_at: string
  transaction_id: string | null
  status: 'generating' | 'generated' | 'skipped' | 'failed'
  error: string | null
  created_at: string
  updated_at: string
}

export interface RecurringOccurrencePreview {
  occurrence_date: string
  scheduled_at: number
  occurred_at: string
}

export interface UpsertRecurringRuleRequest {
  mode?: AppMode
  name?: string
  status?: RecurringRuleStatus
  account_id?: string
  type?: 'income' | 'expense'
  direction?: 'income' | 'expense'
  category?: string
  amount_cents?: number
  amount_yuan?: number
  currency?: string
  exchange_rate?: number
  note?: string
  project_id?: string
  frequency?: RecurringFrequency
  interval?: number
  start_date?: string
  end_date?: string | null
  time_of_day?: string
  timezone?: string
  day_of_week?: number | null
  day_of_month?: number | null
  month_end_policy?: MonthEndPolicy
  catch_up_enabled?: boolean
}

export async function listRecurringRules(mode: AppMode = 'work'): Promise<RecurringRule[]> {
  const { data } = await client.get('/recurring-rules', { params: { mode } })
  return data.data
}

export async function createRecurringRule(req: UpsertRecurringRuleRequest & { mode: AppMode }): Promise<RecurringRule> {
  const { data } = await client.post('/recurring-rules', req)
  return data.data
}

export async function updateRecurringRule(id: string, req: UpsertRecurringRuleRequest): Promise<RecurringRule> {
  const { data } = await client.patch(`/recurring-rules/${id}`, req)
  return data.data
}

export async function updateRecurringRuleStatus(id: string, status: RecurringRuleStatus): Promise<RecurringRule> {
  const { data } = await client.patch(`/recurring-rules/${id}/status`, { status })
  return data.data
}

export async function deleteRecurringRule(id: string): Promise<void> {
  await client.delete(`/recurring-rules/${id}`)
}

export async function listRecurringInstances(id: string): Promise<RecurringInstance[]> {
  const { data } = await client.get(`/recurring-rules/${id}/instances`)
  return data.data
}

export async function generateRecurringNow(id: string): Promise<{ generated: number; skipped: number; failed: number; errors?: string[] }> {
  const { data } = await client.post(`/recurring-rules/${id}/generate-now`)
  return data.data
}

export async function previewRecurringOccurrences(req: UpsertRecurringRuleRequest & { mode: AppMode; count?: number }): Promise<RecurringOccurrencePreview[]> {
  const { data } = await client.get('/recurring-rules/preview', { params: req })
  return data.data
}

// ─── Attachments and OCR ──────────────────────────────────────────────────────

export interface OCRSuggestion {
  amount_cents?: number
  amount_yuan?: number
  currency?: string
  occurred_at?: string
  merchant?: string
  invoice_number?: string
  category?: string
  note?: string
  confidence?: number
}

export interface OCRResult {
  provider: string
  text: string
  suggestion?: OCRSuggestion | null
  raw?: unknown
}

export interface Attachment {
  id: string
  transaction_id: string | null
  storage_key: string
  original_filename: string
  content_type: string
  size_bytes: number
  sha256: string
  kind: 'receipt' | 'invoice' | 'other'
  ocr_status: 'not_requested' | 'pending' | 'processing' | 'done' | 'failed' | 'unavailable'
  ocr_provider: string | null
  ocr_text: string | null
  ocr_json: string | null
  ocr_result?: OCRResult
  ocr_error: string | null
  created_at: string
  updated_at: string
}

export async function uploadAttachment(file: File, opts: { transaction_id?: string; kind?: Attachment['kind']; run_ocr?: boolean } = {}): Promise<Attachment> {
  const form = new FormData()
  form.append('file', file)
  if (opts.transaction_id) form.append('transaction_id', opts.transaction_id)
  if (opts.kind) form.append('kind', opts.kind)
  if (opts.run_ocr) form.append('run_ocr', 'true')
  const { data } = await client.post('/attachments', form, { headers: { 'Content-Type': 'multipart/form-data' } })
  return data.data
}

export async function uploadTransactionAttachment(transactionId: string, file: File, opts: { kind?: Attachment['kind']; run_ocr?: boolean } = {}): Promise<Attachment> {
  const form = new FormData()
  form.append('file', file)
  if (opts.kind) form.append('kind', opts.kind)
  if (opts.run_ocr) form.append('run_ocr', 'true')
  const { data } = await client.post(`/transactions/${transactionId}/attachments`, form, { headers: { 'Content-Type': 'multipart/form-data' } })
  return data.data
}

export async function listTransactionAttachments(transactionId: string): Promise<Attachment[]> {
  const { data } = await client.get(`/transactions/${transactionId}/attachments`)
  return data.data
}

export async function getAttachment(id: string): Promise<Attachment> {
  const { data } = await client.get(`/attachments/${id}`)
  return data.data
}

export async function runAttachmentOCR(id: string): Promise<Attachment> {
  const { data } = await client.post(`/attachments/${id}/ocr`)
  return data.data
}

export async function linkAttachment(id: string, transactionId: string): Promise<Attachment> {
  const { data } = await client.post(`/attachments/${id}/link`, { transaction_id: transactionId })
  return data.data
}

export async function deleteAttachment(id: string): Promise<void> {
  await client.delete(`/attachments/${id}`)
}

export async function downloadAttachment(id: string, filename: string): Promise<void> {
  const resp = await client.get(`/attachments/${id}/download`, { responseType: 'blob' })
  const url = URL.createObjectURL(new Blob([resp.data]))
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'attachment'
  a.click()
  URL.revokeObjectURL(url)
}

// ─── Device Heartbeat & Online Count ──────────────────────────────────────────

/** Send device heartbeat to keep this device marked as online */
export async function sendHeartbeat(deviceId: string): Promise<void> {
  await client.post('/auth/heartbeat', { device_id: deviceId })
}

/** Get the number of currently online devices for the authenticated user */
export async function getOnlineDevices(): Promise<{ count: number }> {
  const { data } = await client.get('/auth/devices/online')
  return data.data
}
