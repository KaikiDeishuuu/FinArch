import { FALLBACK_RATES, toCNYWithRates } from './exchangeRates'

export interface ConvertibleTransactionAmount {
  amount_yuan: number
  currency?: string
  base_amount_cents?: number
  base_currency?: string
}

export interface ConvertibleAccountBalance {
  id: string
  currency: string
  balance_yuan: number
}

export interface BalanceHistoryPointLike {
  date: string
  balance: number
}

export interface AccountBalanceHistorySeries {
  account: Pick<ConvertibleAccountBalance, 'id' | 'currency'>
  points: BalanceHistoryPointLike[]
}

/**
 * Returns a transaction's CNY value, preferring the backend-booked base amount.
 * The fallback path is retained for legacy rows that predate base amount fields.
 */
export function transactionAmountToCNY(
  transaction: ConvertibleTransactionAmount,
  rates: Record<string, number> = FALLBACK_RATES,
): number {
  if (
    typeof transaction.base_amount_cents === 'number' &&
    Number.isFinite(transaction.base_amount_cents) &&
    transaction.base_currency
  ) {
    return toCNYWithRates(transaction.base_amount_cents / 100, transaction.base_currency, rates)
  }
  return toCNYWithRates(transaction.amount_yuan, transaction.currency || 'CNY', rates)
}

/** Converts an account's own-currency cached balance to CNY. */
export function accountBalanceToCNY(
  account: Pick<ConvertibleAccountBalance, 'balance_yuan' | 'currency'>,
  rates: Record<string, number> = FALLBACK_RATES,
): number {
  return toCNYWithRates(account.balance_yuan, account.currency || 'CNY', rates)
}

/**
 * Merges per-account histories without ever adding raw values of different
 * currencies. Missing trailing dates carry the latest known account balance.
 */
export function aggregateAccountBalanceHistories(
  series: AccountBalanceHistorySeries[],
  rates: Record<string, number> = FALLBACK_RATES,
): BalanceHistoryPointLike[] {
  const dates = Array.from(new Set(series.flatMap((item) => item.points.map((point) => point.date)))).sort()
  const cursors = series.map(() => ({ index: 0, balance: 0 }))

  return dates.map((date) => {
    let total = 0
    series.forEach((item, seriesIndex) => {
      const cursor = cursors[seriesIndex]
      while (cursor.index < item.points.length && item.points[cursor.index].date <= date) {
        cursor.balance = item.points[cursor.index].balance
        cursor.index += 1
      }
      total += toCNYWithRates(cursor.balance, item.account.currency || 'CNY', rates)
    })
    return { date, balance: total }
  })
}
