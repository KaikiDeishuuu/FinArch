import { useEffect, useRef, useState } from 'react'

interface CrossAccountRestoreModalProps {
  isOpen: boolean
  onClose: () => void
  onSubmit: (email: string, password: string) => Promise<void>
  isLoading: boolean
  t: (key: string) => string
}

export default function CrossAccountRestoreModal({
  isOpen,
  onClose,
  onSubmit,
  isLoading,
  t,
}: CrossAccountRestoreModalProps) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const emailRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (isOpen) {
      setEmail('')
      setPassword('')
      setShowPassword(false)
      setError('')
      setTimeout(() => emailRef.current?.focus(), 100)
    }
  }, [isOpen])

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape' && isOpen && !isLoading) {
        onClose()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen, isLoading, onClose])

  if (!isOpen) return null

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!email || !password) {
      setError(t('settings.restore.crossAccount.required'))
      return
    }
    setError('')
    try {
      await onSubmit(email, password)
    } catch (err: any) {
      const msg =
        err?.response?.data?.message ||
        err?.message ||
        t('settings.restore.crossAccount.failed')
      setError(msg)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm animate-in fade-in duration-200">
      <div
        className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl w-full max-w-md shadow-xl border border-gray-100 dark:border-gray-800/50 overflow-hidden transform animate-in zoom-in-95 duration-200"
        role="dialog"
        aria-modal="true"
      >
        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">
            {t('settings.restore.crossAccount.title')}
          </h2>
          <p className="text-xs text-gray-500 dark:text-gray-400">
            {t('settings.restore.crossAccount.desc')}
          </p>

          <div className="space-y-3">
            <div>
              <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
                {t('settings.restore.crossAccount.originalEmail')}
              </label>
              <input
                ref={emailRef}
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value)
                  setError('')
                }}
                disabled={isLoading}
                className="w-full border border-gray-200 dark:border-gray-700 rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition bg-gray-50 dark:bg-gray-800/50 dark:text-gray-200 dark:placeholder-gray-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-600 dark:text-gray-300 mb-1">
                {t('settings.restore.crossAccount.originalPassword')}
              </label>
              <div className="relative">
                <input
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    setError('')
                  }}
                  disabled={isLoading}
                  className="w-full border border-gray-200 dark:border-gray-700 rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition bg-gray-50 dark:bg-gray-800/50 dark:text-gray-200 dark:placeholder-gray-500 pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  disabled={isLoading}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                >
                  {showPassword ? (
                    <svg
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={2}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className="w-4 h-4"
                    >
                      <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20C5 20 1 12 1 12a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24M1 1l22 22" />
                    </svg>
                  ) : (
                    <svg
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth={2}
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className="w-4 h-4"
                    >
                      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                      <circle cx="12" cy="12" r="3" />
                    </svg>
                  )}
                </button>
              </div>
            </div>
            {error && (
              <p className="text-xs text-rose-500 mt-1.5">{error}</p>
            )}
          </div>

          <div className="flex items-center justify-end gap-3 mt-4">
            <button
              type="button"
              onClick={onClose}
              disabled={isLoading}
              className="px-4 py-2 text-sm font-medium text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 rounded-xl transition-colors disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={isLoading || !email || !password}
              className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-amber-500 hover:bg-amber-600 rounded-xl transition-colors disabled:opacity-50"
            >
              {isLoading ? (
                <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
              ) : null}
              {isLoading
                ? t('settings.restore.crossAccount.submitting')
                : t('settings.restore.crossAccount.submit')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

