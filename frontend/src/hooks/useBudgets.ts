import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createBudget, deleteBudget, getBudgetSummary, listBudgets, updateBudget } from '../api/client'
import type { AppMode, UpsertBudgetRequest } from '../api/client'
import { useAuth } from './useAuth'
import { useMode } from './useMode'

export function currentBudgetMonth(date = new Date()) {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  return `${date.getFullYear()}-${month}`
}

export const BUDGETS_QUERY_KEY = (userId?: string, mode: AppMode = 'work', period = currentBudgetMonth()) =>
  ['budgets', userId, mode, period] as const

export const BUDGET_SUMMARY_QUERY_KEY = (userId?: string, mode: AppMode = 'work', period = currentBudgetMonth()) =>
  ['budgets-summary', userId, mode, period] as const

export function useBudgets(period = currentBudgetMonth()) {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: BUDGETS_QUERY_KEY(user?.id, mode, period),
    queryFn: () => listBudgets(mode, period),
    enabled: !!user,
    staleTime: 30_000,
  })
}

export function useBudgetSummary(period = currentBudgetMonth()) {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: BUDGET_SUMMARY_QUERY_KEY(user?.id, mode, period),
    queryFn: () => getBudgetSummary(mode, period),
    enabled: !!user,
    staleTime: 30_000,
  })
}

export function useBudgetMutations(period = currentBudgetMonth()) {
  const { user } = useAuth()
  const { mode } = useMode()
  const qc = useQueryClient()
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: BUDGETS_QUERY_KEY(user?.id, mode, period) })
    void qc.invalidateQueries({ queryKey: BUDGET_SUMMARY_QUERY_KEY(user?.id, mode, period) })
    void qc.invalidateQueries({ queryKey: ['transactions', user?.id, mode] })
  }

  const create = useMutation({
    mutationFn: (req: UpsertBudgetRequest) => createBudget({ ...req, mode, period_month: req.period_month || period }),
    onSuccess: invalidate,
  })
  const update = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertBudgetRequest }) => updateBudget(id, { ...req, mode, period_month: req.period_month || period }),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: deleteBudget,
    onSuccess: invalidate,
  })

  return { create, update, remove }
}
