import { ScanText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { OCRSuggestion } from '../api/client'
import { hasOCRSuggestion } from '../utils/ocr'
import { Button } from './ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/dialog'

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
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent>
        <DialogHeader>
          <div className="mb-2 grid size-9 place-items-center rounded-lg bg-accent-soft text-accent">
            <ScanText className="size-4.5" />
          </div>
          <DialogTitle>{t('attachments.ocr.reviewTitle')}</DialogTitle>
          <DialogDescription>{t('attachments.ocr.reviewDesc')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2 rounded-lg border border-border bg-muted/55 p-3">
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('attachments.ocr.noSuggestion')}</p>
          ) : rows.map(([key, value]) => (
            <div key={key} className="flex justify-between gap-3 text-sm">
              <span className="text-muted-foreground">{t(`attachments.ocr.fields.${key}`)}</span>
              <span className="text-right font-semibold text-foreground">{value}</span>
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="button" onClick={() => onApply(suggestion)}>
            {t('attachments.ocr.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
