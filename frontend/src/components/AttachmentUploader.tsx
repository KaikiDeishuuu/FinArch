import { useEffect, useRef, useState } from 'react'
import { FilePlus2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import type { Attachment, OCRSuggestion } from '../api/client'
import { deleteAttachment } from '../api/client'
import { useAttachmentMutations } from '../hooks/useAttachments'
import { attachmentOCRText, hasOCRSuggestion } from '../utils/ocr'
import OcrTextDisclosure from './OcrTextDisclosure'
import { Button } from './ui/button'
import { cn } from '../lib/utils'

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
    <div
      className={cn(
        'space-y-2',
        !compact && 'rounded-xl border border-dashed border-input bg-muted/45 p-4',
      )}
    >
      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png,image/webp,application/pdf,.jpg,.jpeg,.png,.webp,.pdf"
        className="hidden"
        onChange={(event) => setFile(event.target.files?.[0] ?? null)}
      />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => inputRef.current?.click()}
        >
          <FilePlus2 className="size-3.5" />
          {t('attachments.choose')}
        </Button>
        {file ? (
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {file.name} · {formatFileSize(file.size)}
          </span>
        ) : null}
      </div>
      <label className="mt-2 flex w-fit cursor-pointer items-center gap-2 text-xs text-muted-foreground">
        <input
          type="checkbox"
          checked={runOCR}
          onChange={(event) => setRunOCR(event.target.checked)}
          className="size-4 rounded border-input accent-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
        />
        {t('attachments.runOcr')}
      </label>
      {file ? (
        <Button
          type="button"
          size="sm"
          onClick={upload}
          loading={mutations.upload.isPending}
          loadingText={t('common.loading')}
          className="mt-2"
        >
          <Upload className="size-3.5" />
          {t('attachments.upload')}
        </Button>
      ) : null}
      {lastAttachment ? <OcrTextDisclosure attachment={lastAttachment} /> : null}
    </div>
  )
}
