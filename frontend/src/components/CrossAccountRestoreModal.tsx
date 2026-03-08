import { useEffect, useRef, useState } from 'react'

interface CrossAccountRestoreModalProps {
  isOpen: boolean
  onClose: () => void
  onSendCode: (email: string) => Promise<void>
  onVerify: (code: string) => Promise<void>
  isLoading: boolean
  emailSent: boolean
  maskedEmail?: string
  t: (key: string, options?: Record<string, unknown>) => string
}

export default function CrossAccountRestoreModal({
  isOpen,
  onClose,
  onSendCode,
  onVerify,
  isLoading,
  emailSent,
  maskedEmail,
  t,
}: CrossAccountRestoreModalProps) {
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const emailRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isOpen) {
      setEmail('')
      setCode('')
      setError('')
      setTimeout(() => emailRef.current?.focus(), 100)
    }
  }, [isOpen])

  if (!isOpen) return null

  async function handleSendCode(e: React.FormEvent) {
    e.preventDefault()
    if (!email) {
      setError(t('settings.restore.crossAccount.requiredEmail'))
      return
    }
    setError('')
    try {
      await onSendCode(email)
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || t('settings.restore.crossAccount.failed'))
    }
  }

  async function handleVerify(e: React.FormEvent) {
    e.preventDefault()
    if (!code) {
      setError(t('settings.restore.crossAccount.requiredCode'))
      return
    }
    setError('')
    try {
      await onVerify(code)
    } catch (err: any) {
      setError(err?.response?.data?.message || err?.message || t('settings.restore.crossAccount.failed'))
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl w-full max-w-md shadow-xl border border-gray-100 dark:border-gray-800/50 overflow-hidden">
        <form onSubmit={emailSent ? handleVerify : handleSendCode} className="p-6 space-y-4">
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('settings.restore.crossAccount.title')}</h2>
          <p className="text-xs text-gray-500 dark:text-gray-400">{t('settings.restore.crossAccount.desc')}</p>

          {!emailSent ? (
            <div>
              <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">{t('settings.restore.crossAccount.originalEmail')}</label>
              <input
                ref={emailRef}
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                  setError('')
                }}
                disabled={isLoading}
                className="w-full border border-gray-200 dark:border-gray-700 rounded-xl px-3 py-2.5 text-sm bg-gray-50 dark:bg-gray-800/50"
              />
            </div>
          ) : (
            <>
              <p className="text-xs text-emerald-600 dark:text-emerald-400">{t('settings.restore.crossAccount.emailSent', { email: maskedEmail || '' })}</p>
              <div>
                <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">{t('settings.restore.crossAccount.verificationCode')}</label>
                <input
                  type="text"
                  value={code}
                  onChange={(e) => {
                    setCode(e.target.value)
                    setError('')
                  }}
                  disabled={isLoading}
                  className="w-full border border-gray-200 dark:border-gray-700 rounded-xl px-3 py-2.5 text-sm bg-gray-50 dark:bg-gray-800/50"
                />
              </div>
            </>
          )}

          {error && <p className="text-xs text-rose-500 mt-1.5">{error}</p>}

          <div className="flex items-center justify-end gap-3 mt-4">
            <button type="button" onClick={onClose} disabled={isLoading} className="px-4 py-2 text-sm">{t('common.cancel')}</button>
            <button type="submit" disabled={isLoading} className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-amber-500 hover:bg-amber-600 rounded-xl">
              {isLoading ? <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" /> : null}
              {emailSent ? t('settings.restore.crossAccount.verify') : t('settings.restore.crossAccount.sendCode')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
