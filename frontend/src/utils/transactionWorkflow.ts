export type TransactionWorkflowTab = 'all' | 'unreimbursed' | 'reimbursed'
export type TransactionWorkflowKind = 'reimbursement' | 'settlement' | 'upload'
export type TransactionWorkflowStage =
  | 'income'
  | 'pending-upload'
  | 'pending-reimbursement'
  | 'reimbursed'
  | 'pending-settlement'
  | 'settled'
  | 'uploaded'

type TransactionWorkflowState = {
  direction: 'income' | 'expense'
  uploaded: boolean
  reimbursed: boolean
  settled?: boolean
}

export function transactionWorkflowStage(
  transaction: TransactionWorkflowState,
  workflowKind: TransactionWorkflowKind,
): TransactionWorkflowStage {
  if (transaction.direction === 'income') return 'income'

  if (workflowKind === 'reimbursement') {
    if (transaction.reimbursed) return 'reimbursed'
    return transaction.uploaded ? 'pending-reimbursement' : 'pending-upload'
  }

  // Settlement clears a public-account expense with finance. It reads `settled`
  // only: public money was never fronted by the user, so `reimbursed` must not
  // leak into this lane.
  if (workflowKind === 'settlement') {
    if (transaction.settled) return 'settled'
    return transaction.uploaded ? 'pending-settlement' : 'pending-upload'
  }

  return transaction.uploaded ? 'uploaded' : 'pending-upload'
}

export function isTransactionInWorkflowTab(
  transaction: TransactionWorkflowState,
  tab: TransactionWorkflowTab,
  workflowKind: TransactionWorkflowKind,
) {
  if (tab === 'all') return true
  const stage = transactionWorkflowStage(transaction, workflowKind)
  if (stage === 'income') return false

  const completed = stage === 'reimbursed' || stage === 'settled' || stage === 'uploaded'
  return tab === 'reimbursed' ? completed : !completed
}
