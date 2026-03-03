/**
 * Exchange rate utilities.
 * Fetches live rates from public APIs and caches locally for 6 hours.
 * Falls back to hardcoded approximate rates when all providers fail.
 *
 * All rates are expressed as "1 foreign currency = N CNY".
 */

const CACHE_KEY = 'finarch_exchange_rates_v1'
const CACHE_TTL = 6 * 60 * 60 * 1000 // 6 hours
const LIVE_SYMBOLS = ['USD', 'EUR', 'JPY', 'GBP'] as const
const REQUEST_TIMEOUT_MS = 6000

export interface RateCache {
  /** Map: currency code → CNY equivalent (1 unit = N CNY) */
  rates: Record<string, number>
  /** ISO date string from the API, e.g. "2026-03-01" */
  date: string
  fetchedAt: number
}

/** Hardcoded fallback rates (approximate, used when API is unreachable). */
export const FALLBACK_RATES: Record<string, number> = {
  CNY: 1,
  USD: 7.26,
  EUR: 7.84,
  JPY: 0.0475,
  GBP: 9.15,
}

function loadCache(): RateCache | null {
  try {
    const raw = localStorage.getItem(CACHE_KEY)
    if (!raw) return null
    const parsed: RateCache = JSON.parse(raw)
    if (Date.now() - parsed.fetchedAt < CACHE_TTL) return parsed
  } catch {
    // ignore invalid cache data
  }
  return null
}

function saveCache(cache: RateCache) {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(cache))
  } catch {
    // ignore storage write errors
  }
}

async function fetchJSON(url: string): Promise<unknown> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  try {
    const res = await fetch(url, { signal: controller.signal })
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    return res.json()
  } finally {
    clearTimeout(timer)
  }
}

function buildRateCache(date: string, rates: Record<string, number>): RateCache | null {
  const normalized: Record<string, number> = { CNY: 1 }
  for (const symbol of LIVE_SYMBOLS) {
    const value = rates[symbol]
    if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return null
    normalized[symbol] = value
  }
  return { rates: normalized, date, fetchedAt: Date.now() }
}

async function fetchFromFrankfurter(): Promise<RateCache | null> {
  const raw = await fetchJSON('https://api.frankfurter.app/latest?from=CNY&to=USD,EUR,JPY,GBP') as {
    date?: string
    rates?: Record<string, number>
  }
  if (!raw?.rates || typeof raw.date !== 'string') return null

  // API returns: 1 CNY = X foreign. Convert to: 1 foreign = Y CNY.
  const converted: Record<string, number> = {}
  for (const [cur, r] of Object.entries(raw.rates)) {
    if (typeof r !== 'number' || !Number.isFinite(r) || r <= 0) return null
    converted[cur.toUpperCase()] = 1 / r
  }
  return buildRateCache(raw.date, converted)
}

async function fetchFromOpenERAPI(): Promise<RateCache | null> {
  const raw = await fetchJSON('https://open.er-api.com/v6/latest/CNY') as {
    time_last_update_utc?: string
    rates?: Record<string, number>
  }
  if (!raw?.rates) return null

  const converted: Record<string, number> = {}
  for (const symbol of LIVE_SYMBOLS) {
    const v = raw.rates[symbol]
    if (typeof v !== 'number' || !Number.isFinite(v) || v <= 0) return null
    converted[symbol] = 1 / v
  }

  const date = typeof raw.time_last_update_utc === 'string'
    ? new Date(raw.time_last_update_utc).toISOString().slice(0, 10)
    : new Date().toISOString().slice(0, 10)

  return buildRateCache(date, converted)
}

/**
 * Returns exchange rates (foreign → CNY).
 * Tries localStorage cache first, then live APIs, then fallback.
 */
export async function fetchRates(): Promise<RateCache> {
  const cached = loadCache()
  if (cached) return cached

  const providers = [fetchFromFrankfurter, fetchFromOpenERAPI]
  for (const provider of providers) {
    try {
      const cache = await provider()
      if (!cache) continue
      saveCache(cache)
      return cache
    } catch {
      // try the next provider
    }
  }

  // API unavailable — use fallback, but don't cache so next load retries.
  return { rates: FALLBACK_RATES, date: '', fetchedAt: 0 }
}

/** Convert an amount in any currency to CNY using provided rates map. */
export function toCNYWithRates(
  amount: number,
  currency: string,
  rates: Record<string, number>,
): number {
  const code = (currency ?? 'CNY').toUpperCase()
  const rate = rates[code] ?? FALLBACK_RATES[code] ?? 1
  return amount * rate
}
