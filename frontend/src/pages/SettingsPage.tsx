import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Trans, useTranslation } from 'react-i18next'
import {
  changePassword, requestDeleteAccount, requestEmailChange, getMe,
  createAccount, renameAccount, deleteAccount, updateNickname,
} from '../api/client'
import type { UserProfile } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { useAccounts, useInvalidateAccounts } from '../hooks/useAccounts'
import { useTransactions } from '../hooks/useTransactions'
import { useMode } from '../hooks/useMode'
import Select from '../components/Select'
import { CURRENCY_SYMBOLS } from '../constants/currencies'
import { useConfig } from '../hooks/useConfig'

// ─── Password strength (shared logic) ────────────────────────────────────────
type Strength = 'none' | 'weak' | 'medium' | 'strong'
function calcStrength(pw: string): Strength {
  if (!pw) return 'none'
  if (pw.length < 8) return 'weak'
  if (/^\d+$/.test(pw)) return 'weak'
  let s = 0
  if (/[a-z]/.test(pw)) s++
  if (/[A-Z]/.test(pw)) s++
  if (/[0-9]/.test(pw)) s++
  if (/[^a-zA-Z0-9]/.test(pw)) s++
  if (s <= 1) return 'weak'
  if (s === 2) return 'medium'
  return 'strong'
}
function PasswordStrength({ password, t }: { password: string; t: (key: string) => string }) {
  const s = calcStrength(password)
  if (!password) return null
  const bar = { none: 'w-0', weak: 'w-1/3', medium: 'w-2/3', strong: 'w-full' }[s]
  const color = { none: '', weak: 'bg-rose-400', medium: 'bg-amber-400', strong: 'bg-emerald-500' }[s]
  const label = { none: '', weak: t('settings.password.strength.weak'), medium: t('settings.password.strength.medium'), strong: t('settings.password.strength.strong') }[s]
  const tc = { none: '', weak: 'text-rose-500', medium: 'text-amber-600', strong: 'text-emerald-500' }[s]
  return (
    <div className="mt-1.5 space-y-1">
      <div className="h-1 w-full bg-gray-100 dark:bg-gray-800 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
}

// ─── Shared styles ────────────────────────────────────────────────────────────
const labelCls = 'mb-1.5 block text-sm font-medium text-[hsl(var(--foreground))]'
const inputCls = 'fin-input w-full px-3 py-2.5 text-sm'

// ─── Section header ───────────────────────────────────────────────────────────
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-3 flex items-center gap-3">
      <h2 className="page-kicker shrink-0">
        {children}
      </h2>
      <div className="h-px flex-1 bg-[hsl(var(--border))]" />
    </div>
  )
}

function MobileCollapsibleSection({ title, defaultOpen = false, children }: { title: string; defaultOpen?: boolean; children: React.ReactNode }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(defaultOpen)
  return (
    <section className="md:contents">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="ledger-panel flex w-full items-center justify-between gap-3 px-4 py-3 text-left md:hidden"
        aria-expanded={open}
      >
        <span className="page-kicker">{title}</span>
        <span className="text-[11px] font-semibold text-[hsl(var(--mode-accent-strong))]">{open ? t('common.collapse') : t('common.expand')}</span>
      </button>
      <div className={`${open ? 'block' : 'hidden'} md:block mt-3 md:mt-0`}>
        {children}
      </div>
    </section>
  )
}

// ─── Alert components ─────────────────────────────────────────────────────────
function Alert({ type, children }: { type: 'success' | 'error' | 'info' | 'warning'; children: React.ReactNode }) {
  const cls = {
    success: 'bg-emerald-50 dark:bg-emerald-500/10 border-green-200 dark:border-emerald-500/30 text-emerald-700 dark:text-emerald-400',
    error: 'bg-rose-50 dark:bg-rose-500/10 border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-400',
    info: 'bg-[hsl(var(--mode-accent-wash))] border-[hsl(var(--mode-accent))]/35 text-[hsl(var(--mode-accent-strong))]',
    warning: 'bg-amber-50 dark:bg-amber-500/10 border-amber-200 dark:border-amber-500/30 text-amber-800 dark:text-amber-400',
  }[type]
  return (
    <div className={`border rounded-xl px-4 py-3 text-sm ${cls}`}>{children}</div>
  )
}

function OperationsNotice({ title, description }: { title: string; description: string }) {
  return (
    <div className="flex items-start gap-3 rounded-[8px] border border-dashed border-[hsl(var(--mode-accent))]/40 bg-[hsl(var(--mode-accent-wash))] px-4 py-3">
      <div className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-[6px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--mode-accent-strong))]" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="h-3.5 w-3.5"><rect x="3" y="11" width="18" height="10" rx="2" /><path d="M7 11V7a5 5 0 0110 0v4" /></svg>
      </div>
      <div>
        <p className="text-sm font-semibold text-[hsl(var(--foreground))]">{title}</p>
        <p className="mt-0.5 text-xs leading-relaxed text-[hsl(var(--muted-foreground))]">{description}</p>
      </div>
    </div>
  )
}

// ─── Main component ───────────────────────────────────────────────────────────
export default function SettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, clearSession, updateUser } = useAuth()
  const { systemOperationsEnabled } = useConfig()
  const { isWorkMode, mode } = useMode()
  const { data: accounts = [], isLoading: acctLoading } = useAccounts()
  const invalidateAccounts = useInvalidateAccounts()
  const { data: transactions = [] } = useTransactions()
  // ── Profile data ──────────────────────────────────────────────────────────
  const [profile, setProfile] = useState<UserProfile | null>(null)
  useEffect(() => {
    getMe().then(setProfile).catch(() => {/* ignore */ })
  }, [])

  // ── Nickname ──────────────────────────────────────────────────────────────
  const [editingNickname, setEditingNickname] = useState(false)
  const [nicknameInput, setNicknameInput] = useState('')
  const [nicknameLoading, setNicknameLoading] = useState(false)
  const [nicknameError, setNicknameError] = useState('')
  const [nicknameSuccess, setNicknameSuccess] = useState(false)

  function startEditNickname() {
    setNicknameInput(profile?.nickname || user?.nickname || '')
    setNicknameError('')
    setNicknameSuccess(false)
    setEditingNickname(true)
  }

  async function handleSaveNickname() {
    if (!nicknameInput.trim()) { setNicknameError(t('settings.profile.nicknameRequired')); return }
    if (nicknameInput.length > 20) { setNicknameError(t('settings.profile.nicknameMaxLen')); return }
    setNicknameLoading(true)
    setNicknameError('')
    try {
      await updateNickname(nicknameInput.trim())
      updateUser({ nickname: nicknameInput.trim() })
      setProfile(prev => prev ? { ...prev, nickname: nicknameInput.trim() } : prev)
      setNicknameSuccess(true)
      setEditingNickname(false)
      setTimeout(() => setNicknameSuccess(false), 2000)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setNicknameError(msg || t('settings.profile.toast.error'))
    } finally {
      setNicknameLoading(false)
    }
  }

  // ── Change password ───────────────────────────────────────────────────────
  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwLoading, setPwLoading] = useState(false)
  const [pwError, setPwError] = useState('')
  const [pwSuccess, setPwSuccess] = useState(false)

  async function handleChangePassword(e: FormEvent) {
    e.preventDefault()
    setPwError('')
    setPwSuccess(false)
    if (newPw !== confirmPw) { setPwError(t('settings.password.toast.mismatch')); return }
    if (newPw.length < 8) { setPwError(t('settings.password.minLength')); return }
    setPwLoading(true)
    try {
      await changePassword(currentPw, newPw)
      setPwSuccess(true)
      setCurrentPw(''); setNewPw(''); setConfirmPw('')
      // Password changes revoke every server-side session, including this one.
      // Drop the in-memory access token immediately instead of waiting for the
      // next API request to discover the revocation.
      clearSession()
      navigate('/login?password_changed=1', { replace: true })
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setPwError(msg || t('settings.password.toast.error'))
    } finally {
      setPwLoading(false)
    }
  }

  // ── Change email ──────────────────────────────────────────────────────────
  const [newEmail, setNewEmail] = useState('')
  const [emailLoading, setEmailLoading] = useState(false)
  const [emailError, setEmailError] = useState('')
  const [emailSent, setEmailSent] = useState(false)

  async function handleRequestEmailChange(e: FormEvent) {
    e.preventDefault()
    setEmailError('')
    setEmailSent(false)
    setEmailLoading(true)
    try {
      const currentPassword = window.prompt(t('settings.password.currentPlaceholder')) || ""
      if (!currentPassword) { throw new Error(t('common.cancel')) }
      await requestEmailChange(newEmail, currentPassword)
      setEmailSent(true)
      setNewEmail('')
      // Refresh profile to show pending_email
      getMe().then(setProfile).catch(() => {/* ignore */ })
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setEmailError(msg || t('settings.changeEmail.toast.error'))
    } finally {
      setEmailLoading(false)
    }
  }

  // ── Delete account ────────────────────────────────────────────────────────
  const [deleteStep, setDeleteStep] = useState<'idle' | 'confirm' | 'loading' | 'sent'>('idle')
  const [deleteError, setDeleteError] = useState('')

  async function handleRequestDelete() {
    setDeleteStep('loading')
    setDeleteError('')
    try {
      await requestDeleteAccount()
      setDeleteStep('sent')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setDeleteError(msg || t('settings.danger.toast.error'))
      setDeleteStep('confirm')
    }
  }
  // ── Account management ─────────────────────────────────────────────────
  const [newAcctName, setNewAcctName] = useState('')
  // Mode-restricted: WORK=public only, LIFE=personal only
  const [newAcctType, setNewAcctType] = useState<'personal' | 'public'>(isWorkMode ? 'public' : 'personal')
  useEffect(() => { setNewAcctType(isWorkMode ? 'public' : 'personal') }, [isWorkMode])
  const [newAcctCurrency, setNewAcctCurrency] = useState('CNY')
  const [newAcctLoading, setNewAcctLoading] = useState(false)
  const [newAcctError, setNewAcctError] = useState('')
  const [newAcctSuccess, setNewAcctSuccess] = useState(false)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [renameLoading, setRenameLoading] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [deleteAcctLoading, setDeleteAcctLoading] = useState(false)
  const [deleteAcctError, setDeleteAcctError] = useState('')

  async function handleCreateAccount(e: FormEvent) {
    e.preventDefault()
    setNewAcctError('')
    setNewAcctSuccess(false)
    if (!newAcctName.trim()) { setNewAcctError(t('settings.accounts.nameRequired')); return }
    setNewAcctLoading(true)
    try {
      await createAccount(newAcctName.trim(), newAcctType, mode, newAcctCurrency)
      await invalidateAccounts()
      setNewAcctName('')
      setNewAcctSuccess(true)
      setTimeout(() => setNewAcctSuccess(false), 2000)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setNewAcctError(msg || t('settings.accounts.toast.createError'))
    } finally { setNewAcctLoading(false) }
  }

  async function handleRenameAccount(id: string) {
    if (!renameValue.trim()) return
    setRenameLoading(true)
    try {
      await renameAccount(id, renameValue.trim())
      await invalidateAccounts()
      setRenamingId(null)
    } catch { /* ignore */ } finally { setRenameLoading(false) }
  }

  async function handleDeleteAccount(id: string) {
    // UX fast-fail: block deletion if this account has unreimbursed expense transactions.
    // The backend enforces this too, but checking here avoids the round-trip.
    const hasUnreimbursed = transactions.some(
      (tx) => tx.account_id === id && tx.direction === 'expense' && !tx.reimbursed
    )
    if (hasUnreimbursed) {
      toast.error(t('settings.accounts.toast.hasUnreimbursed'))
      return
    }
    setDeleteAcctError('')
    setDeleteAcctLoading(true)
    try {
      await deleteAccount(id)
      await invalidateAccounts()
      setDeletingId(null)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setDeleteAcctError(msg || t('settings.accounts.toast.deleteError'))
    } finally { setDeleteAcctLoading(false) }
  }

  // Count accounts per type to determine if delete is allowed
  const personalCount = accounts.filter(a => a.type === 'personal').length
  const publicCount = accounts.filter(a => a.type === 'public').length
  function canDelete(a: { type: string }) {
    return a.type === 'personal' ? personalCount > 1 : publicCount > 1
  }

  const displayName = profile?.nickname || profile?.username || user?.nickname || user?.username || user?.email || '—'
  const currentEmail = profile?.email || user?.email || '—'
  const pendingEmail = profile?.pending_email

  return (
    <div className="pb-8 max-w-4xl">
      {/* Page header */}
      <div className="ledger-rail mb-6 pl-5">
        <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-[1.75rem]">{t('settings.title')}</h1>
        <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('settings.subtitle')}</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        {/* ── Fund Accounts ───────────────────────────── full width ── */}
        <div className="md:col-span-2">
          <SectionLabel>{t('settings.sections.accounts')}</SectionLabel>
          <div className="ledger-panel space-y-4 p-5">
            {/* Account list */}
            {acctLoading ? (
              <div className="flex items-center gap-2 text-sm text-gray-400 dark:text-gray-500">
                <span className="h-4 w-4 animate-spin rounded-full border-2 border-[hsl(var(--border))] border-t-[hsl(var(--mode-accent))]" />
                {t('common.loading')}
              </div>
            ) : accounts.length === 0 ? (
              <p className="text-sm text-gray-400 dark:text-gray-500">{t('settings.accounts.noAccounts')}</p>
            ) : (
              <div className="divide-y divide-gray-50 dark:divide-gray-800/50">
                {accounts.map(a => {
                  const isPublic = a.type === 'public'
                  const typeBadge = isPublic
                    ? 'bg-sky-50 dark:bg-sky-500/10 text-sky-700 dark:text-sky-400 border border-sky-100 dark:border-sky-500/30'
                    : 'bg-amber-50 dark:bg-amber-500/10 text-amber-700 dark:text-amber-400 border border-amber-100 dark:border-amber-500/30'
                  const typeLabel = isPublic ? t('settings.accounts.publicLabel') : t('settings.accounts.personalLabel')
                  const balanceColor = a.balance_yuan >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-500 dark:text-rose-400'
                  return (
                    <div key={a.id} className="flex flex-wrap items-center gap-2 py-3 first:pt-0 last:pb-0">
                      <span className={`text-xs font-semibold px-2 py-0.5 rounded-full shrink-0 ${typeBadge}`}>{typeLabel}</span>
                      {renamingId === a.id ? (
                        <form onSubmit={(e) => { e.preventDefault(); handleRenameAccount(a.id) }}
                          className="flex flex-1 gap-2">
                          <input
                            autoFocus
                            value={renameValue}
                            onChange={(e) => setRenameValue(e.target.value)}
                            className="fin-input flex-1 px-2.5 py-1 text-sm"
                          />
                          <button type="submit" disabled={renameLoading}
                            className="rounded-[6px] bg-[hsl(var(--primary))] px-3 py-1 text-xs text-[hsl(var(--primary-foreground))] disabled:opacity-50">
                            {renameLoading ? t('common.saving') : t('common.save')}
                          </button>
                          <button type="button" onClick={() => setRenamingId(null)}
                            className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 px-2 py-1">{t('common.cancel')}</button>
                        </form>
                      ) : (
                        <>
                          <span className="flex-1 text-sm font-medium text-gray-800 dark:text-gray-200 min-w-0 truncate">{a.name}</span>
                          <span className={`text-sm font-bold tabular-nums shrink-0 ${balanceColor}`}>
                            {a.balance_yuan >= 0 ? '' : '−'}{CURRENCY_SYMBOLS[a.currency] ?? a.currency} {Math.abs(a.balance_yuan).toFixed(2)}
                            <span className="text-xs font-normal text-gray-400 dark:text-gray-500 ml-1">{a.currency}</span>
                          </span>
                          <button
                            type="button"
                            onClick={() => { setRenamingId(a.id); setRenameValue(a.name) }}
                            className="shrink-0 rounded-[6px] px-2 py-1 text-xs text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))] hover:text-[hsl(var(--mode-accent-strong))]"
                          >{t('settings.accounts.rename')}</button>
                          {canDelete(a) && (
                            deletingId === a.id ? (
                              <span className="flex items-center gap-1.5 shrink-0">
                                <button
                                  type="button"
                                  disabled={deleteAcctLoading}
                                  onClick={() => handleDeleteAccount(a.id)}
                                  className="rounded-[6px] bg-[hsl(var(--expense))] px-2.5 py-1 text-xs text-[hsl(var(--primary-foreground))] transition-[filter] hover:brightness-90 disabled:opacity-50"
                                >{deleteAcctLoading ? t('settings.accounts.deleting') : t('settings.accounts.confirmDelete')}</button>
                                <button
                                  type="button"
                                  onClick={() => { setDeletingId(null); setDeleteAcctError('') }}
                                  className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 px-1.5 py-1"
                                >{t('common.cancel')}</button>
                              </span>
                            ) : (
                              <button
                                type="button"
                                onClick={() => { setDeletingId(a.id); setDeleteAcctError('') }}
                                className="text-xs text-gray-400 hover:text-rose-500 dark:hover:text-rose-400 px-2 py-1 rounded-lg hover:bg-gray-50 dark:hover:bg-gray-800/50 transition-colors shrink-0"
                              >{t('settings.accounts.delete')}</button>
                            )
                          )}
                        </>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
            {deleteAcctError && <p className="text-xs text-rose-500 mt-2">{deleteAcctError}</p>}

            {/* Create account form */}
            <div className="border-t border-gray-100 dark:border-gray-800 pt-4">
              <p className="text-xs font-semibold text-gray-400 dark:text-gray-500 tracking-wide mb-3">{t('settings.accounts.newTitle')}</p>
              <form onSubmit={handleCreateAccount} className="flex flex-wrap items-end gap-2">
                <input
                  value={newAcctName}
                  onChange={(e) => setNewAcctName(e.target.value)}
                  placeholder={t('settings.accounts.namePlaceholder')}
                  className={`${inputCls} flex-1 min-w-32 py-2`}
                />
                <div className="w-28">
                  <Select
                    value={newAcctType}
                    onChange={(v) => setNewAcctType(v as 'personal' | 'public')}
                    size="sm"
                    options={isWorkMode
                      ? [{ value: 'public', label: t('settings.accounts.publicAccount') }]
                      : [{ value: 'personal', label: t('settings.accounts.personalAccount') }]
                    }
                  />
                </div>
                <div className="w-20">
                  <Select
                    value={newAcctCurrency}
                    onChange={setNewAcctCurrency}
                    size="sm"
                    options={[
                      { value: 'CNY', label: 'CNY' },
                      { value: 'USD', label: 'USD' },
                      { value: 'EUR', label: 'EUR' },
                    ]}
                  />
                </div>
                <button type="submit" disabled={newAcctLoading}
                  className="rounded-[7px] bg-[hsl(var(--primary))] px-4 py-2 text-sm font-medium text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50">
                  {newAcctLoading ? t('settings.accounts.creating') : t('settings.accounts.create')}
                </button>
              </form>
              {newAcctError && <p className="text-xs text-rose-500 mt-2">{newAcctError}</p>}
              {newAcctSuccess && <p className="text-xs text-emerald-600 dark:text-emerald-400 mt-2">{t('settings.accounts.toast.created')}</p>}
            </div>
          </div>
        </div>
        {/* ── Profile ─────────────────────────────────────────── full width ── */}
        <div className="md:col-span-2">
          <SectionLabel>{t('settings.sections.profile')}</SectionLabel>
          <div className="ledger-panel p-5">
            <div className="flex items-center gap-4">
              <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[10px] border border-[hsl(var(--mode-accent))]/35 bg-[hsl(var(--mode-accent-wash))] font-data text-xl font-bold text-[hsl(var(--mode-accent-strong))]">
                {(displayName[0] ?? '?').toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                {/* Nickname row */}
                <div className="flex items-center gap-2 flex-wrap">
                  {editingNickname ? (
                    <div className="flex items-center gap-2 flex-wrap">
                      <input type="text" value={nicknameInput} onChange={e => setNicknameInput(e.target.value)}
                        className="w-36 rounded-[7px] border border-[hsl(var(--border))] bg-[hsl(var(--control))] px-2.5 py-1 text-sm font-semibold text-[hsl(var(--foreground))] transition-colors focus:border-[hsl(var(--ring))] focus:outline-none focus:ring-2 focus:ring-[hsl(var(--ring))]/15"
                        maxLength={20} autoFocus
                        onKeyDown={e => { if (e.key === 'Enter') handleSaveNickname(); if (e.key === 'Escape') setEditingNickname(false) }} />
                      <button onClick={handleSaveNickname} disabled={nicknameLoading}
                        className="rounded-[6px] bg-[hsl(var(--primary))] px-2.5 py-1 text-xs font-medium text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50">
                        {nicknameLoading ? t('common.saving') : t('common.save')}
                      </button>
                      <button onClick={() => setEditingNickname(false)}
                        className="text-xs text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 px-1.5 py-1 transition-colors">{t('common.cancel')}</button>
                    </div>
                  ) : (
                    <>
                      <p className="font-semibold text-gray-900 dark:text-gray-100 text-base">{displayName}</p>
                      <button onClick={startEditNickname}
                        className="cursor-pointer rounded px-2 py-0.5 text-[10px] font-medium text-[hsl(var(--mode-accent-strong))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))]">
                        {t('settings.profile.nicknameTip')}
                      </button>
                      {nicknameSuccess && <span className="text-[10px] text-emerald-500 dark:text-emerald-400 font-medium">{t('settings.profile.toast.updated')}</span>}
                    </>
                  )}
                </div>
                {nicknameError && <p className="text-xs text-rose-500 mt-1">{nicknameError}</p>}
                {/* Username */}
                <div className="flex items-center gap-2 mt-1">
                  <p className="text-sm text-gray-400 dark:text-gray-500">@{profile?.username || user?.username}</p>
                  <span className="text-[10px] font-medium bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500 px-1.5 py-0.5 rounded-full">{t('settings.profile.usernameTip')}</span>
                </div>
                {/* Email */}
                <p className="text-sm text-gray-500 dark:text-gray-400 mt-0.5">{currentEmail}</p>
                {pendingEmail && (
                  <p className="text-xs text-amber-600 dark:text-amber-400 mt-1 flex items-center gap-1">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="w-3 h-3 shrink-0"><circle cx="12" cy="12" r="10" /><path d="M12 8v4M12 16h.01" /></svg>
                    {t('settings.changeEmail.pendingTo')}{pendingEmail}
                  </p>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* ── Change email ─────────────────────────────────────── col 1 ── */}
        <MobileCollapsibleSection title={t('settings.sections.changeEmail')}>
        <div className="flex flex-col">
          <div className="hidden md:block"><SectionLabel>{t('settings.sections.changeEmail')}</SectionLabel></div>
          <div className="ledger-panel flex-1 space-y-4 p-5">
            <div className="space-y-0.5">
              <p className="text-sm text-gray-700 dark:text-gray-300 font-medium">{t('settings.changeEmail.currentEmail')}</p>
              <p className="text-sm text-gray-500 dark:text-gray-400">{currentEmail}</p>
            </div>

            {pendingEmail && !emailSent && (
              <Alert type="warning">
                <p className="font-medium mb-0.5">{t('settings.changeEmail.pendingTitle')}</p>
                <p>
                  <Trans
                    i18nKey="settings.changeEmail.pendingDesc"
                    values={{ email: pendingEmail }}
                    components={{ strong: <strong /> }}
                  />
                </p>
              </Alert>
            )}

            {emailSent ? (
              <Alert type="success">
                <p className="font-medium mb-0.5">{t('settings.changeEmail.sentTitle')}</p>
                <p>
                  <Trans
                    i18nKey="settings.changeEmail.sentDesc"
                    values={{ currentEmail, pendingEmail: profile?.pending_email }}
                    components={{ strong: <strong /> }}
                  />
                </p>
              </Alert>
            ) : (
              <form onSubmit={handleRequestEmailChange} className="space-y-3">
                <div>
                  <label className={labelCls}>{t('settings.changeEmail.newEmail')}</label>
                  <input type="email" required value={newEmail}
                    onChange={(e) => setNewEmail(e.target.value)}
                    className={inputCls} placeholder={t('settings.changeEmail.placeholder')} />
                </div>
                {emailError && <Alert type="error">{emailError}</Alert>}
                <button type="submit" disabled={emailLoading}
                  className="rounded-[7px] bg-[hsl(var(--primary))] px-5 py-2.5 text-sm font-medium text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50">
                  {emailLoading ? t('settings.changeEmail.sending') : t('settings.changeEmail.submit')}
                </button>
              </form>
            )}
          </div>
        </div>
        </MobileCollapsibleSection>

        {/* ── Change password ──────────────────────────────────── col 2 ── */}
        <MobileCollapsibleSection title={t('settings.sections.security')}>
        <div className="flex flex-col">
          <div className="hidden md:block"><SectionLabel>{t('settings.sections.security')}</SectionLabel></div>
          <div className="ledger-panel flex-1 p-5">
            {pwSuccess && <div className="mb-4"><Alert type="success">{t('settings.password.toast.success')}</Alert></div>}
            <form onSubmit={handleChangePassword} className="space-y-4">
              <div>
                <label className={labelCls}>{t('settings.password.current')}</label>
                <input type="password" required value={currentPw}
                  onChange={(e) => setCurrentPw(e.target.value)}
                  className={inputCls} placeholder={t('settings.password.currentPlaceholder')} autoComplete="current-password" />
              </div>
              <div>
                <label className={labelCls}>{t('settings.password.new')}</label>
                <input type="password" required minLength={8} value={newPw}
                  onChange={(e) => setNewPw(e.target.value)}
                  className={inputCls} placeholder={t('settings.password.newPlaceholder')} autoComplete="new-password" />
                <PasswordStrength password={newPw} t={t} />
              </div>
              <div>
                <label className={labelCls}>{t('settings.password.confirm')}</label>
                <input type="password" required minLength={8} value={confirmPw}
                  onChange={(e) => setConfirmPw(e.target.value)}
                  className={inputCls} placeholder={t('settings.password.confirmPlaceholder')} autoComplete="new-password" />
                {confirmPw && newPw !== confirmPw && (
                  <p className="mt-1 text-xs text-rose-500">{t('settings.password.toast.mismatch')}</p>
                )}
              </div>
              {pwError && <Alert type="error">{pwError}</Alert>}
              <button type="submit" disabled={pwLoading || (!!confirmPw && newPw !== confirmPw)}
                className="w-full rounded-[7px] bg-[hsl(var(--primary))] py-2.5 text-sm font-medium text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50">
                {pwLoading ? t('settings.password.submitting') : t('settings.password.submit')}
              </button>
            </form>
          </div>
        </div>
        </MobileCollapsibleSection>

        {/* ── Backup ───────────────────────────────────────────── col 1 ── */}
        <MobileCollapsibleSection title={t('settings.sections.backup')}>
        <div className="flex flex-col">
          <div className="hidden md:block"><SectionLabel>{t('settings.sections.backup')}</SectionLabel></div>
          <div className="ledger-panel flex-1 p-5">
            <OperationsNotice
              title={t(systemOperationsEnabled ? 'settings.operationsRestricted.title' : 'settings.operationsDisabled.title')}
              description={t(systemOperationsEnabled ? 'settings.operationsRestricted.desc' : 'settings.operationsDisabled.desc')}
            />
          </div>
        </div>
        </MobileCollapsibleSection>

        {/* ── Restore ──────────────────────────────────────────── col 2 ── */}
        <MobileCollapsibleSection title={t('settings.sections.restore')}>
        <div className="flex flex-col">
          <div className="hidden md:block"><SectionLabel>{t('settings.sections.restore')}</SectionLabel></div>
          <div className="ledger-panel flex-1 border-amber-500/30 p-5">
            <OperationsNotice
              title={t(systemOperationsEnabled ? 'settings.operationsRestricted.title' : 'settings.operationsDisabled.title')}
              description={t(systemOperationsEnabled ? 'settings.operationsRestricted.desc' : 'settings.operationsDisabled.desc')}
            />
          </div>
        </div>
        </MobileCollapsibleSection>

        {/* ── Danger zone ─────────────────────────────────────── full width ── */}
        <MobileCollapsibleSection title={t('settings.sections.danger')}>
        <div className="md:col-span-2">
          <div className="hidden md:block"><SectionLabel>{t('settings.sections.danger')}</SectionLabel></div>
          <div className="ledger-panel border-[hsl(var(--expense))]/35 p-5">
            <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
              <div className="flex-1 min-w-0">
                <h3 className="text-sm font-semibold text-rose-600 dark:text-rose-400 mb-1">{t('settings.danger.deleteAccount')}</h3>
                <p className="text-xs text-gray-400 dark:text-gray-500">{t('settings.danger.deleteDesc')}</p>
              </div>
              <div className="shrink-0">
                {deleteStep === 'sent' ? (
                  <Alert type="success">
                    <span>
                      <Trans
                        i18nKey="settings.danger.emailSent"
                        values={{ email: currentEmail }}
                        components={{ strong: <strong /> }}
                      />
                    </span>
                  </Alert>
                ) : deleteStep === 'confirm' || deleteStep === 'loading' ? (
                  <div className="bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 rounded-xl p-4 space-y-3 max-w-sm">
                    <p className="text-sm font-semibold text-rose-700 dark:text-rose-400">{t('settings.danger.confirmTitle')}</p>
                    <p className="text-xs text-rose-600 dark:text-rose-400">
                      <Trans
                        i18nKey="settings.danger.confirmDesc"
                        values={{ email: currentEmail }}
                        components={{ strong: <strong /> }}
                      />
                    </p>
                    {deleteError && <Alert type="error">{deleteError}</Alert>}
                    <div className="flex flex-wrap gap-2">
                      <button type="button" onClick={handleRequestDelete} disabled={deleteStep === 'loading'}
                        className="rounded-[7px] bg-[hsl(var(--expense))] px-4 py-2 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-[filter] hover:brightness-90 disabled:opacity-50">
                        {deleteStep === 'loading' ? t('settings.danger.sending') : t('settings.danger.sendConfirm')}
                      </button>
                      <button type="button" onClick={() => { setDeleteStep('idle'); setDeleteError('') }}
                        disabled={deleteStep === 'loading'}
                        className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300 px-4 py-2 rounded-xl border border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600 transition-colors">
                        {t('common.cancel')}
                      </button>
                    </div>
                  </div>
                ) : (
                  <button type="button" onClick={() => setDeleteStep('confirm')}
                    className="inline-flex items-center gap-2 border border-rose-200 dark:border-rose-500/30 hover:bg-rose-50 dark:hover:bg-rose-500/10 text-rose-600 dark:text-rose-400 text-sm font-medium px-4 py-2.5 rounded-xl transition-colors">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
                      <polyline points="3 6 5 6 21 6" /><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" />
                      <path d="M10 11v6M14 11v6M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2" />
                    </svg>
                    {t('settings.danger.deleteButton')}
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
        </MobileCollapsibleSection>

      </div>
    </div>
  )
}
