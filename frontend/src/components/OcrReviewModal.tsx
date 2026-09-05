import * as Dialog from '@radix-ui/react-dialog'
import { useTranslation } from 'react-i18next'
import type { OCRSuggestion } from '../api/client'
import { hasOCRSuggestion } from '../utils/ocr'

export default function OcrReviewModal({
  suggestion,
  onApply,
  onClose,
}: {
  suggestion: OCRSuggestion | null
  onApply: (suggestion: OCRSuggestion) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  if (!hasOCRSuggestion(suggestion)) return null
  const rows = [
    ['amount', suggestion.amount_yuan ? String(suggestion.amount_yuan) : ''],
    ['date', suggestion.occurred_at || ''],
    ['merchant', suggestion.merchant || ''],
    ['category', suggestion.category || ''],
    ['note', suggestion.note || ''],
  ].filter(([, value]) => value)
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[80] bg-[#101815]/60 backdrop-blur-[2px]" />
        <Dialog.Content className="ledger-panel fixed left-1/2 top-1/2 z-[81] max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 overflow-y-auto outline-none">
          <div className="border-b border-[hsl(var(--border))] px-5 py-4">
            <Dialog.Title className="font-display text-lg font-bold text-[hsl(var(--foreground))]">
              {t('attachments.ocr.reviewTitle')}
            </Dialog.Title>
            <Dialog.Description className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">
              {t('attachments.ocr.reviewDesc')}
            </Dialog.Description>
          </div>
          <div className="divide-y divide-[hsl(var(--border))] bg-[hsl(var(--muted))]/45 px-5">
            {rows.length === 0 ? (
              <p className="py-4 text-sm text-[hsl(var(--muted-foreground))]">{t('attachments.ocr.noSuggestion')}</p>
            ) : rows.map(([key, value]) => (
              <div key={key} className="grid grid-cols-[minmax(5.5rem,0.7fr)_minmax(0,1.3fr)] gap-3 py-3 text-sm">
                <span className="font-data text-[11px] font-semibold uppercase tracking-[0.08em] text-[hsl(var(--muted-foreground))]">{t(`attachments.ocr.fields.${key}`)}</span>
                <span className="break-words text-right font-semibold text-[hsl(var(--foreground))]">{value}</span>
              </div>
            ))}
          </div>
          <div className="flex justify-end gap-2 border-t border-[hsl(var(--border))] px-5 py-4">
            <Dialog.Close asChild>
              <button type="button" className="fin-button fin-button--secondary px-4 py-2 text-sm">
                {t('common.cancel')}
              </button>
            </Dialog.Close>
            <button type="button" onClick={() => onApply(suggestion)} className="fin-button px-4 py-2 text-sm">
              {t('attachments.ocr.apply')}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
