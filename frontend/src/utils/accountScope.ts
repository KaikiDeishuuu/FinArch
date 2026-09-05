export type TransactionSource = 'personal' | 'company'

export function transactionSourceForMode(
  mode: 'work' | 'life',
  requestedSource: string | null,
): TransactionSource {
  if (mode === 'life') return 'personal'
  return requestedSource === 'personal' || requestedSource === 'company'
    ? requestedSource
    : 'company'
}

// The accounts endpoint scopes account types through its mode parameter:
// LIFE exposes personal accounts and WORK exposes public/company accounts.
export function accountModeForTransactionSource(source: TransactionSource): 'work' | 'life' {
  return source === 'personal' ? 'life' : 'work'
}
