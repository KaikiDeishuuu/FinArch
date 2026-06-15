import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createRecurringRule,
  deleteRecurringRule,
  generateRecurringNow,
  listRecurringInstances,
  listRecurringRules,
  previewRecurringOccurrences,
  updateRecurringRule,
  updateRecurringRuleStatus,
} from '../api/client'
import type { AppMode, RecurringRuleStatus, UpsertRecurringRuleRequest } from '../api/client'
import { useAuth } from './useAuth'
import { useMode } from './useMode'

export const RECURRING_RULES_QUERY_KEY = (userId?: string, mode: AppMode = 'work') =>
  ['recurring-rules', userId, mode] as const

export const RECURRING_INSTANCES_QUERY_KEY = (userId?: string, ruleId?: string) =>
  ['recurring-instances', userId, ruleId] as const

export const RECURRING_PREVIEW_QUERY_KEY = (userId?: string, mode: AppMode = 'work', req?: UpsertRecurringRuleRequest) =>
  ['recurring-preview', userId, mode, req] as const

export function useRecurringRules() {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: RECURRING_RULES_QUERY_KEY(user?.id, mode),
    queryFn: () => listRecurringRules(mode),
    enabled: !!user,
    staleTime: 30_000,
  })
}

export function useRecurringInstances(ruleId?: string) {
  const { user } = useAuth()
  return useQuery({
    queryKey: RECURRING_INSTANCES_QUERY_KEY(user?.id, ruleId),
    queryFn: () => listRecurringInstances(ruleId!),
    enabled: !!user && !!ruleId,
    staleTime: 30_000,
  })
}

export function useRecurringPreview(req: UpsertRecurringRuleRequest & { count?: number }, enabled = true) {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: RECURRING_PREVIEW_QUERY_KEY(user?.id, mode, req),
    queryFn: () => previewRecurringOccurrences({ ...req, mode }),
    enabled: enabled && !!user,
    staleTime: 15_000,
  })
}

export function useRecurringMutations() {
  const { user } = useAuth()
  const { mode } = useMode()
  const qc = useQueryClient()
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: RECURRING_RULES_QUERY_KEY(user?.id, mode) })
    void qc.invalidateQueries({ queryKey: ['transactions', user?.id, mode] })
    void qc.invalidateQueries({ queryKey: ['budgets-summary', user?.id, mode] })
  }

  const create = useMutation({
    mutationFn: (req: UpsertRecurringRuleRequest) => createRecurringRule({ ...req, mode }),
    onSuccess: invalidate,
  })
  const update = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpsertRecurringRuleRequest }) => updateRecurringRule(id, { ...req, mode }),
    onSuccess: invalidate,
  })
  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: RecurringRuleStatus }) => updateRecurringRuleStatus(id, status),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: deleteRecurringRule,
    onSuccess: invalidate,
  })
  const generateNow = useMutation({
    mutationFn: generateRecurringNow,
    onSuccess: invalidate,
  })

  return { create, update, setStatus, remove, generateNow }
}
