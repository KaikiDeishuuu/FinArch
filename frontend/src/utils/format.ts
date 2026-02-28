import { toCNYWithRates, FALLBACK_RATES } from './exchangeRates'

/** Mapping from currency code to display symbol. */
const CURRENCY_SYMBOLS: Record<string, string> = {
  CNY: '¥',
  USD: '$',
  EUR: '€',
  JPY: '¥',
  GBP: '£',
}

/**
 * Approximate exchange rates to CNY — used only as a last-resort fallback
 * when the live rate context is not available (e.g. standalone utility calls).
 */
export const EXCHANGE_RATES_TO_CNY: Record<string, number> = FALLBACK_RATES

/** Converts an amount in any currency to CNY equivalent. */
export function toCNY(
  amount: number,
  currency: string,
  rates?: Record<string, number>,
): number {
  return toCNYWithRates(amount, currency, rates ?? FALLBACK_RATES)
}

/** Returns the symbol for a currency code, falling back to the code itself. */
export function currencySymbol(currency: string): string {
  return CURRENCY_SYMBOLS[currency?.toUpperCase()] ?? currency ?? '¥'
}

/**
 * Formats an amount with the correct currency symbol.
 * e.g. formatAmount(100, 'USD') → '$100.00'
 *      formatAmount(500, 'CNY') → '¥500.00'
 */
export function formatAmount(amount: number, currency: string): string {
  const sym = currencySymbol(currency)
  return `${sym}${amount.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

/**
 * Full-precision formatter for tooltips / hover hints.
 * Shows up to 10 significant decimal places, strips trailing zeros.
 * e.g. formatAmountExact(12345.6789, 'CNY') → '¥12,345.6789'
 *      formatAmountExact(80000, 'CNY')       → '¥80,000'
 */
export function formatAmountExact(amount: number, currency: string): string {
  const sym = currencySymbol(currency)
  const formatted = amount.toLocaleString('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 10 })
  return `${sym}${formatted}`
}

/**
 * Compact formatter for summary cards and chart labels.
 * ≥ 1亿  → ¥1.23亿
 * ≥ 1万  → ¥8.00万  (trailing zeros trimmed: ¥8万 / ¥8.5万)
 * < 1万  → ¥999.00  (no decimals if zero: ¥999)
 * For non-CNY/JPY: always full 2-decimal.
 */
export function formatAmountCompact(amount: number, currency: string): string {
  const sym = currencySymbol(currency)
  const isCJK = currency === 'CNY' || currency === 'JPY'
  if (isCJK && Math.abs(amount) >= 1_0000_0000) {
    const v = amount / 1_0000_0000
    return `${sym}${trimZeros(v.toFixed(2))}亿`
  }
  if (isCJK && Math.abs(amount) >= 1_0000) {
    const v = amount / 1_0000
    return `${sym}${trimZeros(v.toFixed(2))}万`
  }
  const full = amount.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return `${sym}${full}`
}

/** Removes unnecessary trailing decimal zeros. e.g. '8.00' → '8', '8.50' → '8.5' */
function trimZeros(s: string): string {
  return s.replace(/\.?0+$/, '')
}

/** Transaction subset type needed for multi-currency totals. */
interface AmountWithCurrency {
  amount_yuan: number
  currency: string
}

/**
 * Aggregates a list of transactions into per-currency totals.
 * Returns a Map<currencyCode, total>.
 */
export function sumByCurrency(items: AmountWithCurrency[]): Map<string, number> {
  const map = new Map<string, number>()
  for (const item of items) {
    const cur = item.currency || 'CNY'
    map.set(cur, (map.get(cur) ?? 0) + item.amount_yuan)
  }
  return map
}

/**
 * Converts all amounts to CNY equivalents and sums them.
 * Pass live `rates` from ExchangeRateContext for accurate conversion.
 */
export function sumInCNY(items: AmountWithCurrency[], rates?: Record<string, number>): number {
  return items.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0)
}

/**
 * Renders a multi-currency total as a string array.
 * e.g. ['¥1,234.00', '$200.00']
 */
export function formatTotals(items: AmountWithCurrency[]): string[] {
  const totals = sumByCurrency(items)
  return Array.from(totals.entries()).map(([cur, amt]) => formatAmount(amt, cur))
}
