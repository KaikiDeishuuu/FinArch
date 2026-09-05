import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import type { Attachment, OCRSuggestion } from '../api/client'
import { useAttachmentMutations } from '../hooks/useAttachments'
import { attachmentOCRText, hasOCRSuggestion } from '../utils/ocr'
import OcrTextDisclosure from './OcrTextDisclosure'
import { deleteAttachment } from '../api/client'

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export default function AttachmentUploader({
  transactionId,
  onUploaded,
  onSuggestion,
  compact = false,
}: {
  transactionId?: string
  onUploaded?: (attachment: Attachment) => void
  onSuggestion?: (suggestion: OCRSuggestion, attachment: Attachment) => void
  compact?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const [runOCR, setRunOCR] = useState(true)
  const [lastAttachment, setLastAttachment] = useState<Attachment | null>(null)
  const mutations = useAttachmentMutations(transactionId)
  const mountedRef = useRef(true)

  useEffect(() => {
    return () => {
      mountedRef.current = false
    }
  }, [])

  async function upload() {
    if (!file) return
    try {
      const attachment = await mutations.upload.mutateAsync({ file, runOCR, kind: 'receipt' })
      if (!mountedRef.current) {
        if (!transactionId) {
          await deleteAttachment(attachment.id).catch(() => undefined)
        }
        return
      }
      toast.success(t('attachments.toast.uploaded'))
      setLastAttachment(attachment)
      setFile(null)
      if (inputRef.current) inputRef.current.value = ''
      onUploaded?.(attachment)
      if (hasOCRSuggestion(attachment.ocr_result?.suggestion)) {
        onSuggestion?.(attachment.ocr_result.suggestion, attachment)
      } else if (attachmentOCRText(attachment)) {
        toast.message(t('attachments.ocr.textReady'))
      } else if (attachment.ocr_status === 'unavailable') {
        toast.message(t('attachments.ocr.unavailable'))
      } else if (attachment.ocr_status === 'failed') {
        toast.error(attachment.ocr_error || t('attachments.ocr.failed'))
      }
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('attachments.toast.failed'))
    }
  }

  return (
    <div className={compact ? 'space-y-2' : 'ledger-panel space-y-1 border-dashed p-4'}>
      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png,image/webp,application/pdf,.jpg,.jpeg,.png,.webp,.pdf"
        className="hidden"
        onChange={(e) => setFile(e.target.files?.[0] ?? null)}
      />
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          className="fin-button fin-button--secondary px-3 py-2 text-xs"
        >
          {t('attachments.choose')}
        </button>
        {file && (
          <span className="font-data min-w-0 truncate text-xs text-[hsl(var(--muted-foreground))]">
            {file.name} · {formatFileSize(file.size)}
          </span>
        )}
      </div>
      <label className="mt-2 flex items-center gap-2 text-xs text-[hsl(var(--muted-foreground))]">
        <input type="checkbox" checked={runOCR} onChange={(e) => setRunOCR(e.target.checked)} className="h-4 w-4 rounded-[3px] border-[hsl(var(--border))] accent-[hsl(var(--mode-accent))]" />
        {t('attachments.runOcr')}
      </label>
      {file && (
        <button
          type="button"
          onClick={upload}
          disabled={mutations.upload.isPending}
          className="fin-button mt-2 px-4 py-2 text-xs"
        >
          {mutations.upload.isPending ? t('common.loading') : t('attachments.upload')}
        </button>
      )}
      {lastAttachment && <OcrTextDisclosure attachment={lastAttachment} />}
    </div>
  )
}
