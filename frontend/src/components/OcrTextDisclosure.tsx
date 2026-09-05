import { useTranslation } from 'react-i18next'
import type { Attachment } from '../api/client'
import { attachmentOCRText } from '../utils/ocr'

export default function OcrTextDisclosure({ attachment }: { attachment: Attachment }) {
  const { t } = useTranslation()
  const text = attachmentOCRText(attachment)
  if (!text) return null

  return (
    <details className="mt-2 rounded-lg border border-cyan-100 bg-cyan-50/60 px-3 py-2 dark:border-cyan-500/20 dark:bg-cyan-500/10">
      <summary className="cursor-pointer text-xs font-semibold text-cyan-700 dark:text-cyan-300">
        {t('attachments.ocr.viewText')}
      </summary>
      <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words font-sans text-xs leading-5 text-gray-600 dark:text-gray-300">
        {text}
      </pre>
    </details>
  )
}
