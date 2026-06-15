import { useTranslation } from 'react-i18next'
import type { OCRSuggestion } from '../api/client'

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
  if (!suggestion) return null
  const rows = [
    ['amount', suggestion.amount_yuan ? String(suggestion.amount_yuan) : ''],
    ['date', suggestion.occurred_at || ''],
    ['merchant', suggestion.merchant || ''],
    ['category', suggestion.category || ''],
    ['note', suggestion.note || ''],
  ].filter(([, value]) => value)
  return (
    <div className="fixed inset-0 z-[80] flex items-center justify-center bg-black/40 px-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-2xl border border-gray-100 bg-white p-5 shadow-2xl dark:border-gray-800 dark:bg-[hsl(260,15%,11%)]">
        <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('attachments.ocr.reviewTitle')}</h2>
        <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t('attachments.ocr.reviewDesc')}</p>
        <div className="mt-4 space-y-2 rounded-xl bg-gray-50 p-3 dark:bg-gray-800/60">
          {rows.length === 0 ? (
            <p className="text-sm text-gray-400">{t('attachments.ocr.noSuggestion')}</p>
          ) : rows.map(([key, value]) => (
            <div key={key} className="flex justify-between gap-3 text-sm">
              <span className="text-gray-400">{t(`attachments.ocr.fields.${key}`)}</span>
              <span className="text-right font-semibold text-gray-700 dark:text-gray-200">{value}</span>
            </div>
          ))}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" onClick={onClose} className="rounded-xl border border-gray-200 px-4 py-2 text-sm font-semibold text-gray-500 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800">
            {t('common.cancel')}
          </button>
          <button type="button" onClick={() => onApply(suggestion)} className="rounded-xl bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700">
            {t('attachments.ocr.apply')}
          </button>
        </div>
      </div>
    </div>
  )
}
