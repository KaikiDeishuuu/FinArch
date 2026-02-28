/** Mapping from currency code to display symbol. */
const CURRENCY_SYMBOLS: Record<string, string> = {
  CNY: '¥',
  USD: '$',
  EUR: '€',
  JPY: '¥',
  GBP: '£',
}

/**
 * Approximate exchange rates to CNY (1 unit of foreign currency = N CNY).
 * Used for aggregated totals, stats and PDF summaries.
 * Individual transaction amounts are always shown in their original currency.
 */
export const EXCHANGE_RATES_TO_CNY: Record<string, number> = {
  CNY: 1,
  USD: 7.2,
  EUR: 7.8,
  JPY: 0.05,
  GBP: 9.0,
}

/** Converts an amount in any currency to CNY equivalent. */
export function toCNY(amount: number, currency: string): number {
  const rate = EXCHANGE_RATES_TO_CNY[(currency ?? 'CNY').toUpperCase()] ?? 1
  return amount * rate
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
 * Used for aggregate totals in summary cards, PDF exports, etc.
 */
export function sumInCNY(items: AmountWithCurrency[]): number {
  return items.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY'), 0)
}

/**
 * Renders a multi-currency total as a string array.
 * e.g. ['¥1,234.00', '$200.00']
 */
export function formatTotals(items: AmountWithCurrency[]): string[] {
  const totals = sumByCurrency(items)
  return Array.from(totals.entries()).map(([cur, amt]) => formatAmount(amt, cur))
}
