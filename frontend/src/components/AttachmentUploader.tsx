import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import type { Attachment, OCRSuggestion } from '../api/client'
import { useAttachmentMutations } from '../hooks/useAttachments'

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
  const mutations = useAttachmentMutations(transactionId)

  async function upload() {
    if (!file) return
    try {
      const attachment = await mutations.upload.mutateAsync({ file, runOCR, kind: 'receipt' })
      toast.success(t('attachments.toast.uploaded'))
      setFile(null)
      if (inputRef.current) inputRef.current.value = ''
      onUploaded?.(attachment)
      if (attachment.ocr_result?.suggestion) {
        onSuggestion?.(attachment.ocr_result.suggestion, attachment)
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
    <div className={compact ? 'space-y-2' : 'rounded-2xl border border-dashed border-violet-200 bg-violet-50/40 p-4 dark:border-violet-500/30 dark:bg-violet-500/5'}>
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
          className="rounded-xl border border-violet-200 bg-white px-3 py-2 text-xs font-semibold text-violet-600 transition-colors hover:bg-violet-50 dark:border-violet-500/30 dark:bg-white/[0.03] dark:text-violet-300 dark:hover:bg-violet-500/10"
        >
          {t('attachments.choose')}
        </button>
        {file && (
          <span className="min-w-0 truncate text-xs text-gray-500 dark:text-gray-400">
            {file.name} · {formatFileSize(file.size)}
          </span>
        )}
      </div>
      <label className="mt-2 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
        <input type="checkbox" checked={runOCR} onChange={(e) => setRunOCR(e.target.checked)} className="rounded border-gray-300 text-violet-600 focus:ring-violet-500" />
        {t('attachments.runOcr')}
      </label>
      {file && (
        <button
          type="button"
          onClick={upload}
          disabled={mutations.upload.isPending}
          className="mt-2 rounded-xl bg-violet-600 px-4 py-2 text-xs font-semibold text-white transition-colors hover:bg-violet-700 disabled:opacity-50"
        >
          {mutations.upload.isPending ? t('common.loading') : t('attachments.upload')}
        </button>
      )}
    </div>
  )
}
