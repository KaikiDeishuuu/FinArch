export type TransactionWorkflowTab = 'all' | 'unreimbursed' | 'reimbursed'
export type TransactionWorkflowKind = 'reimbursement' | 'upload'
export type TransactionWorkflowStage =
  | 'income'
  | 'pending-upload'
  | 'pending-reimbursement'
  | 'reimbursed'
  | 'uploaded'

type TransactionWorkflowState = {
  direction: 'income' | 'expense'
  uploaded: boolean
  reimbursed: boolean
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

  const completed = stage === 'reimbursed' || stage === 'uploaded'
  return tab === 'reimbursed' ? completed : !completed
}
