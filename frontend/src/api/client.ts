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
  captcha_token?: string
}

export interface AuthResponse {
  token: string
  expires_at: string
  user_id: string
  email: string
  username: string
  role: string
}

export async function login(req: LoginRequest): Promise<AuthResponse> {
  const { data } = await client.post('/auth/login', req)
  return data.data
}

export interface RegisterResponse {
  // 201: auto-login (no email verification required)
  token?: string
  expires_at?: string
  user_id?: string
  email?: string
  username?: string
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
  pending_email: string
  role: string
}

export async function getMe(): Promise<UserProfile> {
  const { data } = await client.get('/auth/me')
  return data.data as UserProfile
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
}

export async function matchSubsetSum(
  target: number,
  tolerance: number,
  maxItems: number
): Promise<MatchResult[]> {
  const { data } = await client.post('/match/subset-sum', {
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

// NOTE: getStatsSummary has been removed — it returned raw amount_yuan sums
// across mixed currencies (no conversion). Dashboard now computes balances
// from listTransactions() + live exchange rates on the frontend.

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

export async function restoreBackup(file: File): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  await client.post('/backup/restore', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}
