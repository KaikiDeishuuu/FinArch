/**
 * Exchange rate utilities.
 * Fetches live rates from frankfurter.app (ECB, free, no API key, CORS OK).
 * Results are cached in localStorage for 6 hours; falls back to hardcoded
 * approximate rates if the request fails or the app is offline.
 *
 * All rates are expressed as "1 foreign currency = N CNY".
 */

const CACHE_KEY = 'finarch_exchange_rates_v1'
const CACHE_TTL = 6 * 60 * 60 * 1000 // 6 hours

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
  } catch { /* ignore */ }
  return null
}

function saveCache(cache: RateCache) {
  try { localStorage.setItem(CACHE_KEY, JSON.stringify(cache)) } catch { /* ignore */ }
}

/**
 * Returns exchange rates (foreign → CNY).
 * Tries localStorage cache first, then live API, then fallback.
 */
export async function fetchRates(): Promise<RateCache> {
  const cached = loadCache()
  if (cached) return cached

  try {
    // base=CNY → response: { "rates": { "USD": 0.1377, "EUR": 0.1274, ... } }
    // meaning 1 CNY = X foreign; invert to get 1 foreign = Y CNY
    const res = await fetch(
      'https://api.frankfurter.app/latest?base=CNY&symbols=USD,EUR,JPY,GBP',
      { signal: AbortSignal.timeout(5000) },
    )
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const data = await res.json() as { date: string; rates: Record<string, number> }

    const rates: Record<string, number> = { CNY: 1 }
    for (const [cur, r] of Object.entries(data.rates)) {
      if (r > 0) rates[cur.toUpperCase()] = 1 / r
    }

    const cache: RateCache = { rates, date: data.date, fetchedAt: Date.now() }
    saveCache(cache)
    return cache
  } catch {
    // Offline or API error — use fallback, but don't cache so next load retries
    return { rates: FALLBACK_RATES, date: '', fetchedAt: 0 }
  }
}

/** Convert an amount in any currency to CNY using provided rates map. */
export function toCNYWithRates(
  amount: number,
  currency: string,
  rates: Record<string, number>,
): number {
  const rate = rates[(currency ?? 'CNY').toUpperCase()] ?? FALLBACK_RATES[(currency ?? 'CNY').toUpperCase()] ?? 1
  return amount * rate
}
