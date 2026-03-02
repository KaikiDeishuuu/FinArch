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
    // Only redirect to login when we had an active token (i.e. session expired).
    // If _token is null the 401 came from an unauthenticated request (e.g. wrong
    // password during login) — let the caller handle the error normally.
    if (error.response?.status === 401 && _token && !_redirectingToLogin) {
      _redirectingToLogin = true
      _token = null
      // Clear persisted session so expired token is not reloaded on next page render
      sessionStorage.removeItem('finarch_session')
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

export async function requestEmailChange(newEmail: string): Promise<void> {
  await client.post('/auth/request-email-change', { new_email: newEmail })
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
  email_verification_required: boolean
}

export async function getAppConfig(): Promise<AppConfig> {
  const { data } = await client.get('/config')
  return data.data as AppConfig
}

// ─── Transactions ─────────────────────────────────────────────────────────────

export interface Transaction {
  id: string
  occurred_at: string
  direction: 'income' | 'expense'
  source: 'company' | 'personal'
  account_id: string
  category: string
  amount_yuan: number
  currency: string
  note: string
  project_id: string | null
  reimbursed: boolean
  uploaded: boolean
}

export interface CreateTransactionRequest {
  occurred_at: string
  direction: string
  source: string
  account_id?: string
  category: string
  amount_yuan: number
  currency?: string
  note?: string
  project_id?: string
}

export async function listTransactions(): Promise<Transaction[]> {
  const { data } = await client.get('/transactions')
  return data.data
}

export async function createTransaction(req: CreateTransactionRequest): Promise<Transaction> {
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

export async function listAccounts(): Promise<Account[]> {
  const { data } = await client.get('/accounts')
  return data.data
}

export async function createAccount(
  name: string,
  type: 'personal' | 'public',
  currency = 'CNY'
): Promise<Account> {
  const { data } = await client.post('/accounts', { name, type, currency })
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
  direction: string
  source: string
  category: string
  amount_yuan: number
  currency: string
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

export async function downloadBackup(): Promise<void> {
  const resp = await client.get('/backup/download', { responseType: 'blob' })
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

export async function restoreBackup(file: File): Promise<{ message: string; restored_version: number; migrated_to: number }> {
  const form = new FormData()
  form.append('file', file)
  const { data } = await client.post('/backup/restore', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
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
