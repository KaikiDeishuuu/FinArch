import { useState, useEffect, useRef } from 'react'

interface BackupPasswordModalProps {
    isOpen: boolean
    onClose: () => void
    onSubmit: (password: string) => void
    isLoading: boolean
    t: (key: string) => string
}

export default function BackupPasswordModal({ isOpen, onClose, onSubmit, isLoading, t }: BackupPasswordModalProps) {
    const [password, setPassword] = useState('')
    const [showPassword, setShowPassword] = useState(false)
    const [error, setError] = useState('')
    const inputRef = useRef<HTMLInputElement>(null)

    useEffect(() => {
        if (isOpen) {
            setPassword('')
            setShowPassword(false)
            setError('')
            setTimeout(() => inputRef.current?.focus(), 100)
        }
    }, [isOpen])

    // handle Esc
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

    function handleSubmit(e: React.FormEvent) {
        e.preventDefault()
        if (!password) {
            setError(t('settings.password.currentPlaceholder'))
            return
        }
        setError('')
        onSubmit(password)
    }

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm animate-in fade-in duration-200">
            <div
                className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl w-full max-w-sm shadow-xl border border-gray-100 dark:border-gray-800/50 overflow-hidden transform animate-in zoom-in-95 duration-200"
                role="dialog"
                aria-modal="true"
            >
                <form onSubmit={handleSubmit} className="p-6">
                    <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100 mb-4">
                        {t('settings.backup.passwordPrompt')}
                    </h2>

                    <div className="mb-4">
                        <div className="relative">
                            <input
                                ref={inputRef}
                                type={showPassword ? 'text' : 'password'}
                                value={password}
                                onChange={(e) => { setPassword(e.target.value); setError('') }}
                                disabled={isLoading}
                                placeholder={t('settings.password.currentPlaceholder')}
                                className="w-full border border-gray-200 dark:border-gray-700 rounded-xl px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 transition bg-gray-50 dark:bg-gray-800/50 dark:text-gray-200 dark:placeholder-gray-500 pr-10"
                            />
                            <button
                                type="button"
                                onClick={() => setShowPassword(!showPassword)}
                                disabled={isLoading}
                                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                            >
                                {showPassword ? (
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24M1 1l22 22" /></svg>
                                ) : (
                                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" /><circle cx="12" cy="12" r="3" /></svg>
                                )}
                            </button>
                        </div>
                        {error && <p className="text-xs text-rose-500 mt-1.5">{error}</p>}
                    </div>

                    <div className="flex items-center justify-end gap-3 mt-6">
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
                            disabled={isLoading || !password}
                            className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-violet-600 hover:bg-violet-700 rounded-xl transition-colors disabled:opacity-50"
                        >
                            {isLoading ? (
                                <span className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                            ) : null}
                            {isLoading ? t('settings.backup.generating') : t('settings.backup.download')}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    )
}
