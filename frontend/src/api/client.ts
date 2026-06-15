import axios from 'axios'

// Axios instance with base URL and auth header injection
const client = axios.create({
  baseURL: '/api/v1',
})

// Token stored in memory (not localStorage for XSS protection)
let _token: string | null = null

export function setToken(token: string | null) {
  _token = token
}

export function getToken(): string | null {
  return _token
}

client.interceptors.request.use((config) => {
  if (_token) {
    config.headers.Authorization = `Bearer ${_token}`
  }
  return config
})

let _redirectingToLogin = false

client.interceptors.response.use(
  (response) => response,
  (error) => {
    const envelopeMessage = error.response?.data?.error?.message
    if (envelopeMessage && error.response?.data && !error.response.data.message) {
      error.response.data.message = envelopeMessage
    }

    const errCode = error.response?.data?.error?.code ?? error.response?.data?.code
    const isAuthFailure = errCode === 'AUTH_SESSION_EXPIRED' || errCode === 'AUTH_INVALID_TOKEN' ||
      (error.response?.status === 401 && _token)

    if (isAuthFailure && !_redirectingToLogin) {
      _redirectingToLogin = true
      _token = null
      localStorage.removeItem('finarch_session')
      window.location.href = '/login'
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

export interface AuthResponse {
  token: string
  expires_at: string
  user_id: string
  email: string
  username: string
  nickname: string
  role: string
}

export async function login(req: LoginRequest): Promise<AuthResponse> {
  const { data } = await client.post('/auth/login', req)
  return data.data
}

export async function refreshToken(): Promise<AuthResponse> {
  const { data } = await client.post('/auth/refresh', {})
  return data.data
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
  const resp = await client.post('/auth/register', req)
  return resp.data.data ?? resp.data
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

export async function createTransaction(req: CreateTransactionRequest & { mode?: AppMode }): Promise<Transaction> {
  const { data } = await client.post('/transactions', req)
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
  suggestion: OCRSuggestion
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

// ─── Backup & Restore ────────────────────────────────────────────────────────
export interface BackupInfo {
  transactions: number
  accounts: number
  schema_version: number
  db_size_bytes: number
  journal_mode: string
}

export async function getBackupInfo(): Promise<BackupInfo> {
  const { data } = await client.get('/backup/info')
  return data.data
}

export async function requestBackupExportToken(currentPassword: string): Promise<string> {
  const { data } = await client.post('/backup/export-request', { current_password: currentPassword })
  const payload = data.data ?? data
  return payload.token as string
}

export async function downloadBackup(exportToken: string): Promise<void> {
  const resp = await client.get('/backup/download', { params: { export_token: exportToken }, responseType: 'blob' })
  const cd = resp.headers['content-disposition'] ?? ''
  const match = cd.match(/filename="([^"]+)"/)
  const filename = match ? match[1] : `finarch_backup_${new Date().toISOString().slice(0, 10)}.db`
  const url = URL.createObjectURL(new Blob([resp.data]))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export async function restoreBackup(
  file: File,
): Promise<{ code?: string; message: string; restored_version: number; migrated_to: number }> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await client.post('/backup/restore', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data.data
}

export interface RestoreVerificationResponse {
  restore_id: string
  masked_email: string
  expires_in: number
  message: string
}

export async function sendRestoreVerification(file: File, originalEmail: string): Promise<RestoreVerificationResponse> {
  const form = new FormData()
  form.append('file', file)
  form.append('original_email', originalEmail)
  const { data } = await client.post('/backup/restore/send-verification', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data.data
}

export async function verifyRestoreCode(restoreId: string, code: string): Promise<{ restore_token: string; message: string }> {
  const { data } = await client.post('/backup/restore/verify', {
    restore_id: restoreId,
    code,
  })
  return data.data
}

export async function executeRestore(restoreToken: string): Promise<{ code?: string; message: string; restored_version: number; migrated_to: number }> {
  const { data } = await client.post('/backup/restore/execute', {
    restore_token: restoreToken,
  })
  return data.data
}

// ─── Disaster Recovery (public, no auth required) ────────────────────────────

export interface RestoreRequestResponse {
  restore_id: string
  masked_email: string
  expires_in: number
}

/** Step 1: Upload backup file → receive masked email + restore_id */
export async function disasterRestoreRequest(file: File): Promise<RestoreRequestResponse> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await axios.post('/api/v1/backup/restore-request', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data.data
}

/** Step 2: Submit verification code → restore completes */
export async function disasterRestoreConfirm(
  restoreId: string,
  code: string,
): Promise<{ message: string; restored_version: number; migrated_to: number }> {
  const { data } = await axios.post('/api/v1/backup/restore-confirm', {
    restore_id: restoreId,
    code,
  })
  return data.data
}

export interface DisasterSnapshot {
  snapshot_id: string
  created_at: string
  schema_version: number
  app_version: string
  environment: string
  db_size: number
  has_metadata: boolean
}

export async function listDisasterSnapshots(): Promise<DisasterSnapshot[]> {
  const { data } = await client.get('/disaster-recovery/snapshots')
  return data.data
}

export async function authorizeDisasterRecovery(currentPassword: string): Promise<{ token: string; expires_in: number }> {
  const { data } = await client.post('/disaster-recovery/authorize', { current_password: currentPassword })
  return data.data
}

export async function executeDisasterRecovery(snapshotId: string, authorizationToken: string, allowMissingMetadata = false): Promise<{
  message: string
  recovery_id: string
  snapshot_id: string
  schema_before: number
  schema_after: number
  migration_applied: boolean
  duration_ms: number
}> {
  const { data } = await client.post('/disaster-recovery/restore', {
    snapshot_id: snapshotId,
    confirm: true,
    allow_missing_metadata: allowMissingMetadata,
    authorization_token: authorizationToken,
  })
  return data.data
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
