import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteAttachment, listTransactionAttachments, runAttachmentOCR, uploadAttachment, uploadTransactionAttachment } from '../api/client'
import type { Attachment } from '../api/client'
import { useAuth } from './useAuth'
import { useMode } from './useMode'
import { TRANSACTIONS_QUERY_KEY } from './useTransactions'

export const ATTACHMENTS_QUERY_KEY = (userId?: string, transactionId?: string) =>
  ['attachments', userId, transactionId] as const

export function useTransactionAttachments(transactionId?: string, enabled = true) {
  const { user } = useAuth()
  return useQuery({
    queryKey: ATTACHMENTS_QUERY_KEY(user?.id, transactionId),
    queryFn: () => listTransactionAttachments(transactionId!),
    enabled: enabled && !!user && !!transactionId,
    staleTime: 30_000,
  })
}

export function useAttachmentMutations(transactionId?: string) {
  const { user } = useAuth()
  const { mode } = useMode()
  const qc = useQueryClient()
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ATTACHMENTS_QUERY_KEY(user?.id, transactionId) })
    void qc.invalidateQueries({ queryKey: TRANSACTIONS_QUERY_KEY(user?.id, mode) })
  }
  const upload = useMutation({
    mutationFn: ({ file, runOCR, kind }: { file: File; runOCR?: boolean; kind?: Attachment['kind'] }) =>
      transactionId
        ? uploadTransactionAttachment(transactionId, file, { run_ocr: runOCR, kind })
        : uploadAttachment(file, { run_ocr: runOCR, kind }),
    onSuccess: invalidate,
  })
  const runOCR = useMutation({ mutationFn: runAttachmentOCR, onSuccess: invalidate })
  const remove = useMutation({ mutationFn: deleteAttachment, onSuccess: invalidate })
  return { upload, runOCR, remove }
}
