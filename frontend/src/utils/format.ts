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
 * Compact formatter for chart labels (skips decimals, uses 万 for large CNY values).
 * For non-CNY currencies, always shows 2 decimals.
 */
export function formatAmountCompact(amount: number, currency: string): string {
  const sym = currencySymbol(currency)
  if ((currency === 'CNY' || currency === 'JPY') && amount >= 10000) {
    return `${sym}${(amount / 10000).toFixed(1)}万`
  }
  return `${sym}${amount.toLocaleString('zh-CN', { maximumFractionDigits: 0 })}`
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
