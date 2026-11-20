import { useEffect, useRef, useState } from 'react'
import { FilePlus2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import type { Attachment, OCRSuggestion } from '../api/client'
import { discardDraftAttachment } from '../api/client'
import { useConfig } from '../hooks/useConfig'
import { useAttachmentMutations } from '../hooks/useAttachments'
import { attachmentOCRText, hasOCRSuggestion } from '../utils/ocr'
import OcrTextDisclosure from './OcrTextDisclosure'
import { Button } from './ui/button'
import { cn } from '../lib/utils'

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`
}

export default function AttachmentUploader({
  transactionId,
  onUploaded,
  onUploadingChange,
  onSuggestion,
  compact = false,
}: {
  transactionId?: string
  onUploadingChange?: (uploading: boolean) => void
  onUploaded?: (attachment: Attachment) => void
  onSuggestion?: (suggestion: OCRSuggestion, attachment: Attachment) => void
  compact?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const [file, setFile] = useState<File | null>(null)
  const { attachmentMaxBytes } = useConfig()
  const tooLarge = !!file && attachmentMaxBytes != null && file.size > attachmentMaxBytes
  const limitLabel = attachmentMaxBytes == null ? '' : attachmentMaxBytes % (1024 * 1024) === 0
    ? `${attachmentMaxBytes / (1024 * 1024)} MiB` : `${attachmentMaxBytes} B`
  const [runOCR, setRunOCR] = useState(true)
  const [lastAttachment, setLastAttachment] = useState<Attachment | null>(null)
  const mutations = useAttachmentMutations(transactionId)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  async function upload() {
    if (!file || tooLarge) return
    onUploadingChange?.(true)
    try {
      const attachment = await mutations.upload.mutateAsync({ file, runOCR, kind: 'receipt' })
      if (!mountedRef.current) {
        if (!transactionId) {
          await discardDraftAttachment(attachment.id).catch(() => undefined)
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
    } finally {
      if (mountedRef.current) onUploadingChange?.(false)
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
      {limitLabel && (
        <p role={tooLarge ? 'alert' : undefined} className={cn('text-xs', tooLarge ? 'text-negative' : 'text-muted-foreground')}>
          {t(tooLarge ? 'attachments.tooLarge' : 'attachments.fileLimit', { limit: limitLabel })}
        </p>
      )}
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
          disabled={tooLarge}
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
