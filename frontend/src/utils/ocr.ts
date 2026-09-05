export interface OCRSuggestionLike {
  amount_cents?: number
  amount_yuan?: number
  currency?: string
  occurred_at?: string
  merchant?: string
  invoice_number?: string
  category?: string
  note?: string
}

export interface OCRAttachmentLike {
  ocr_text?: string | null
  ocr_result?: {
    text?: string | null
    suggestion?: OCRSuggestionLike | null
  } | null
}

export function hasOCRSuggestion(suggestion?: OCRSuggestionLike | null): suggestion is OCRSuggestionLike {
  if (!suggestion) return false
  return (
    (typeof suggestion.amount_cents === 'number' && suggestion.amount_cents > 0) ||
    (typeof suggestion.amount_yuan === 'number' && suggestion.amount_yuan > 0) ||
    [
      suggestion.currency,
      suggestion.occurred_at,
      suggestion.merchant,
      suggestion.invoice_number,
      suggestion.category,
      suggestion.note,
    ].some((value) => typeof value === 'string' && value.trim() !== '')
  )
}

export function attachmentOCRText(attachment?: OCRAttachmentLike | null): string {
  const text = attachment?.ocr_text || attachment?.ocr_result?.text || ''
  return text.trim()
}
