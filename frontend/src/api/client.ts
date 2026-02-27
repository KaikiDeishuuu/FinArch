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

client.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      _token = null
      // redirect to login
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// ─── Auth ─────────────────────────────────────────────────────────────────────

export interface LoginRequest {
  email: string
  password: string
}

export interface RegisterRequest {
  email: string
  name: string
  password: string
}

export interface AuthResponse {
  token: string
  expires_at: string
  user_id: string
  email: string
  name: string
  role: string
}

export async function login(req: LoginRequest): Promise<AuthResponse> {
  const { data } = await client.post('/auth/login', req)
  return data.data
}

export async function register(req: RegisterRequest): Promise<AuthResponse> {
  const { data } = await client.post('/auth/register', req)
  return data.data
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await client.post('/auth/change-password', {
    current_password: currentPassword,
    new_password: newPassword,
  })
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
  note: string
  project_id: string
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
