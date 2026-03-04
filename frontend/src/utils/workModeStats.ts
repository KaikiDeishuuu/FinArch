interface MonthlyEntry {
    month: number
    income: number
    expense: number
    reimbursed: number
}

interface WorkModeAdjustments {
    totalReimbursed: number
    adjustedNet: number
}

/**
 * Calculates reimbursement-based adjustments for WORK mode statistics.
 * This function should ONLY be called in WORK mode.
 * LIFE mode should not import or execute this logic.
 */
export function calculateWorkModeAdjustments(
    monthly: MonthlyEntry[],
    totalIncome: number,
    totalExpense: number,
): WorkModeAdjustments {
    const totalReimbursed = monthly.reduce((s, m) => s + (m.reimbursed ?? 0), 0)
    const adjustedNet = totalIncome - totalExpense + totalReimbursed
    return { totalReimbursed, adjustedNet }
}
