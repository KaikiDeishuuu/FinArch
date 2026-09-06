import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Trans, useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import {
  AlertTriangle,
  ChevronDown,
  LifeBuoy,
  LockKeyhole,
  Pencil,
  Trash2,
  WalletCards,
} from 'lucide-react'
import { toast } from 'sonner'
import {
  changePassword,
  createAccount,
  deleteAccount,
  getMe,
  renameAccount,
  requestDeleteAccount,
  requestEmailChange,
  updateNickname,
  type UserProfile,
} from '../api/client'
import { PasswordStrength } from '../components/PasswordStrength'
import Select from '../components/Select'
import { Alert } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { Field, Input, Label } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
import { Spinner } from '../components/ui/spinner'
import { SUPPORT_EMAIL } from '../constants/app'
import { CURRENCY_SYMBOLS } from '../constants/currencies'
import { useAccounts, useInvalidateAccounts } from '../hooks/useAccounts'
import { useAuth } from '../hooks/useAuth'
import { useConfig } from '../hooks/useConfig'
import { useMode } from '../hooks/useMode'
import { useTransactions } from '../hooks/useTransactions'
import { isAnnouncementDismissed, restoreAnnouncement } from '../utils/announcement'
import { cn } from '../lib/utils'

function SectionHeading({ children }: { children: ReactNode }) {
  return (
    <h2 className="mb-2 text-xs font-semibold uppercase tracking-[0.1em] text-muted-foreground">
      {children}
    </h2>
  )
}

function MobileCollapsibleSection({
  title,
  defaultOpen = false,
  children,
}: {
  title: string
  defaultOpen?: boolean
  children: ReactNode
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(defaultOpen)
  const contentId = useId()

  return (
    <section className="md:contents">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3 text-left shadow-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:hidden"
        aria-controls={contentId}
        aria-expanded={open}
      >
        <span className="text-xs font-semibold uppercase tracking-[0.08em] text-muted-foreground">{title}</span>
        <span className="flex items-center gap-1 text-xs font-medium text-accent">
          {open ? t('common.collapse') : t('common.expand')}
          <ChevronDown className={cn('size-3.5 transition-transform', open && 'rotate-180')} />
        </span>
      </button>
      <div id={contentId} className={cn('mt-3 md:mt-0', open ? 'block' : 'hidden md:block')}>
        {children}
      </div>
    </section>
  )
}

/**
 * Keeps the support address reachable after the dashboard announcement has been
 * dismissed, and lets the reader bring that board back.
 */
function SupportSection() {
  const { t } = useTranslation()
  const [dismissed, setDismissed] = useState(isAnnouncementDismissed)

  function showAnnouncementAgain() {
    restoreAnnouncement()
    setDismissed(false)
    toast.success(t('settings.support.restored'))
  }

  return (
    <MobileCollapsibleSection title={t('settings.sections.support')}>
      <div className="md:col-span-2">
        <div className="hidden md:block"><SectionHeading>{t('settings.sections.support')}</SectionHeading></div>
        <Card>
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0 flex-1">
              <CardTitle>{t('settings.support.title')}</CardTitle>
              <CardDescription>
                {t('settings.support.desc')}{' '}
                <a
                  href={`mailto:${SUPPORT_EMAIL}`}
                  className="font-medium text-accent underline underline-offset-2 hover:no-underline"
                >
                  {SUPPORT_EMAIL}
                </a>
              </CardDescription>
              {dismissed ? null : (
                <p className="mt-2 text-xs text-muted-foreground">{t('settings.support.alreadyVisible')}</p>
              )}
            </div>
            <div className="shrink-0">
              <Button variant="outline" onClick={showAnnouncementAgain}>
                <LifeBuoy className="size-4" />
                {t('settings.support.showAnnouncement')}
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </MobileCollapsibleSection>
  )
}

function OperationsNotice({ title, description }: { title: string; description: string }) {
  return (
    <Alert variant="info" className="items-start">
      <LockKeyhole className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <div>
        <p className="font-semibold">{title}</p>
        <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{description}</p>
      </div>
    </Alert>
  )
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user, clearSession, updateUser } = useAuth()
  const { systemOperationsEnabled } = useConfig()
  const initialSystemOperationsEnabled = useRef(systemOperationsEnabled)
  const { isWorkMode, mode } = useMode()
  const { data: accounts = [], isLoading: acctLoading } = useAccounts()
  const invalidateAccounts = useInvalidateAccounts()
  const { data: workTransactions = [], isLoading: workTransactionsLoading } = useTransactions('work')
  const { data: lifeTransactions = [], isLoading: lifeTransactionsLoading } = useTransactions('life')
  const transactions = [...workTransactions, ...lifeTransactions]
  const transactionsLoading = workTransactionsLoading || lifeTransactionsLoading

  const [profile, setProfile] = useState<UserProfile | null>(null)
  const profileRequestId = useRef(0)

  function refreshProfile() {
    const requestId = profileRequestId.current + 1
    profileRequestId.current = requestId
    return getMe().then((nextProfile) => {
      if (requestId === profileRequestId.current) setProfile(nextProfile)
      return nextProfile
    })
  }

  useEffect(() => {
    void refreshProfile().catch(() => { /* Profile details are optional here. */ })
    return () => {
      profileRequestId.current += 1
    }
  }, [])

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
    const nickname = nicknameInput.trim()
    if (!nickname) {
      setNicknameError(t('settings.profile.nicknameRequired'))
      return
    }
    if (Array.from(nickname).length > 20) {
      setNicknameError(t('settings.profile.nicknameMaxLen'))
      return
    }

    setNicknameLoading(true)
    setNicknameError('')
    try {
      profileRequestId.current += 1
      await updateNickname(nickname)
      updateUser({ nickname })
      setProfile((current) => current ? { ...current, nickname } : current)
      setNicknameSuccess(true)
      setEditingNickname(false)
      window.setTimeout(() => setNicknameSuccess(false), 2000)
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setNicknameError(message || t('settings.profile.toast.error'))
    } finally {
      setNicknameLoading(false)
    }
  }

  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')
  const [pwLoading, setPwLoading] = useState(false)
  const [pwError, setPwError] = useState('')
  const [pwSuccess, setPwSuccess] = useState(false)

  async function handleChangePassword(event: FormEvent) {
    event.preventDefault()
    setPwError('')
    setPwSuccess(false)
    if (newPw !== confirmPw) {
      setPwError(t('settings.password.toast.mismatch'))
      return
    }
    if (newPw.length < 8) {
      setPwError(t('settings.password.minLength'))
      return
    }

    setPwLoading(true)
    try {
      await changePassword(currentPw, newPw)
      setPwSuccess(true)
      setCurrentPw('')
      setNewPw('')
      setConfirmPw('')
      clearSession()
      navigate('/login?password_changed=1', { replace: true })
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setPwError(message || t('settings.password.toast.error'))
    } finally {
      setPwLoading(false)
    }
  }

  const [newEmail, setNewEmail] = useState('')
  const [emailCurrentPw, setEmailCurrentPw] = useState('')
  const [emailLoading, setEmailLoading] = useState(false)
  const [emailError, setEmailError] = useState('')
  const [emailSent, setEmailSent] = useState(false)

  async function handleRequestEmailChange(event: FormEvent) {
    event.preventDefault()
    setEmailError('')
    setEmailSent(false)
    setEmailLoading(true)
    try {
      await requestEmailChange(newEmail, emailCurrentPw)
      setEmailSent(true)
      setNewEmail('')
      setEmailCurrentPw('')
      void refreshProfile().catch(() => { /* Keep the successful state if refresh fails. */ })
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setEmailError(message || t('settings.changeEmail.toast.error'))
    } finally {
      setEmailLoading(false)
    }
  }

  const [deleteStep, setDeleteStep] = useState<'idle' | 'confirm' | 'loading' | 'sent'>('idle')
  const [deleteError, setDeleteError] = useState('')

  async function handleRequestDelete() {
    setDeleteStep('loading')
    setDeleteError('')
    try {
      await requestDeleteAccount()
      setDeleteStep('sent')
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setDeleteError(message || t('settings.danger.toast.error'))
      setDeleteStep('confirm')
    }
  }

  const [newAcctName, setNewAcctName] = useState('')
  const [newAcctType, setNewAcctType] = useState<'personal' | 'public'>(isWorkMode ? 'public' : 'personal')
  useEffect(() => {
    setNewAcctType(isWorkMode ? 'public' : 'personal')
  }, [isWorkMode])
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

  async function handleCreateAccount(event: FormEvent) {
    event.preventDefault()
    setNewAcctError('')
    setNewAcctSuccess(false)
    if (!newAcctName.trim()) {
      setNewAcctError(t('settings.accounts.nameRequired'))
      return
    }

    setNewAcctLoading(true)
    try {
      await createAccount(newAcctName.trim(), newAcctType, mode, newAcctCurrency)
      await invalidateAccounts()
      setNewAcctName('')
      setNewAcctSuccess(true)
      window.setTimeout(() => setNewAcctSuccess(false), 2000)
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setNewAcctError(message || t('settings.accounts.toast.createError'))
    } finally {
      setNewAcctLoading(false)
    }
  }

  async function handleRenameAccount(id: string) {
    if (!renameValue.trim()) return
    setRenameLoading(true)
    try {
      await renameAccount(id, renameValue.trim())
      await invalidateAccounts()
      setRenamingId(null)
    } catch {
      // The surrounding list remains usable if the update fails.
    } finally {
      setRenameLoading(false)
    }
  }

  async function handleDeleteAccount(id: string) {
    if (transactionsLoading) return

    const hasTransactionHistory = transactions.some(
      (transaction) => transaction.account_id === id,
    )
    if (hasTransactionHistory) {
      toast.error(t('settings.accounts.toast.hasTransactions'))
      return
    }

    setDeleteAcctError('')
    setDeleteAcctLoading(true)
    try {
      await deleteAccount(id)
      await invalidateAccounts()
      setDeletingId(null)
    } catch (error: unknown) {
      const message = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
      setDeleteAcctError(message || t('settings.accounts.toast.deleteError'))
    } finally {
      setDeleteAcctLoading(false)
    }
  }

  const personalCount = accounts.filter((account) => account.type === 'personal').length
  const publicCount = accounts.filter((account) => account.type === 'public').length
  const canDelete = (account: { type: string }) => account.type === 'personal' ? personalCount > 1 : publicCount > 1

  const displayName = profile?.nickname || profile?.username || user?.nickname || user?.username || user?.email || '—'
  const currentEmail = profile?.email || user?.email || '—'
  const pendingEmail = profile?.pending_email
  const effectiveSystemOperationsEnabled = initialSystemOperationsEnabled.current || systemOperationsEnabled
  const operationsTitle = t(effectiveSystemOperationsEnabled ? 'settings.operationsRestricted.title' : 'settings.operationsDisabled.title')
  const operationsDescription = t(effectiveSystemOperationsEnabled ? 'settings.operationsRestricted.desc' : 'settings.operationsDisabled.desc')

  return (
    <div className="mx-auto max-w-5xl space-y-6 pb-8">
      <PageHeader title={t('settings.title')} description={t('settings.subtitle')} />

      <section>
        <SectionHeading>{t('settings.sections.accounts')}</SectionHeading>
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t('settings.sections.accounts')}</CardTitle>
              <CardDescription>{t('settings.accounts.newTitle')}</CardDescription>
            </div>
            <Badge variant="mode">{isWorkMode ? 'WORK' : 'LIFE'}</Badge>
          </CardHeader>
          <CardContent className="space-y-4">
            {acctLoading ? (
              <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
                <Spinner size="md" />
                {t('common.loading')}
              </div>
            ) : accounts.length === 0 ? (
              <EmptyState title={t('settings.accounts.noAccounts')} icon={<WalletCards />} />
            ) : (
              <div className="divide-y divide-border">
                {accounts.map((account) => {
                  const isPublic = account.type === 'public'
                  const typeLabel = isPublic ? t('settings.accounts.publicLabel') : t('settings.accounts.personalLabel')

                  return (
                    <div key={account.id} className="flex flex-wrap items-center gap-2 py-3 first:pt-0 last:pb-0">
                      <Badge variant={isPublic ? 'accent' : 'warning'}>{typeLabel}</Badge>
                      {renamingId === account.id ? (
                        <form
                          onSubmit={(event) => {
                            event.preventDefault()
                            void handleRenameAccount(account.id)
                          }}
                          className="flex min-w-56 flex-1 flex-wrap gap-2"
                        >
                          <Input
                            autoFocus
                            value={renameValue}
                            onChange={(event) => setRenameValue(event.target.value)}
                            className="min-w-36 flex-1"
                            aria-label={t('settings.accounts.rename')}
                          />
                          <Button type="submit" size="sm" loading={renameLoading} loadingText={t('common.saving')}>
                            {t('common.save')}
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setRenamingId(null)}>
                            {t('common.cancel')}
                          </Button>
                        </form>
                      ) : (
                        <>
                          <span className="min-w-24 flex-1 truncate text-sm font-medium text-foreground">{account.name}</span>
                          <span className={cn(
                            'shrink-0 text-sm font-semibold tabular-nums',
                            account.balance_yuan >= 0 ? 'text-positive' : 'text-negative',
                          )}>
                            {account.balance_yuan >= 0 ? '' : '−'}
                            {CURRENCY_SYMBOLS[account.currency] ?? account.currency} {Math.abs(account.balance_yuan).toFixed(2)}
                            <span className="ml-1 font-mono text-[10px] font-normal text-muted-foreground">{account.currency}</span>
                          </span>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                              setRenamingId(account.id)
                              setRenameValue(account.name)
                            }}
                          >
                            <Pencil className="size-3.5" />
                            {t('settings.accounts.rename')}
                          </Button>
                          {canDelete(account) ? (
                            deletingId === account.id ? (
                              <div className="flex items-center gap-1.5">
                                <Button
                                  variant="danger"
                                  size="sm"
                                  loading={deleteAcctLoading}
                                  loadingText={t('settings.accounts.deleting')}
                                  disabled={transactionsLoading}
                                  onClick={() => void handleDeleteAccount(account.id)}
                                >
                                  {t('settings.accounts.confirmDelete')}
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setDeletingId(null)
                                    setDeleteAcctError('')
                                  }}
                                >
                                  {t('common.cancel')}
                                </Button>
                              </div>
                            ) : (
                              <Button
                                variant="ghost"
                                size="sm"
                                className="hover:text-negative"
                                onClick={() => {
                                  setDeletingId(account.id)
                                  setDeleteAcctError('')
                                }}
                              >
                                <Trash2 className="size-3.5" />
                                {t('settings.accounts.delete')}
                              </Button>
                            )
                          ) : null}
                        </>
                      )}
                    </div>
                  )
                })}
              </div>
            )}

            {deleteAcctError ? <Alert variant="negative">{deleteAcctError}</Alert> : null}

            <form onSubmit={handleCreateAccount} className="grid gap-3 border-t border-border pt-4 sm:grid-cols-[minmax(0,1fr)_8rem_6rem_auto] sm:items-end">
              <Field label={t('settings.accounts.newTitle')} error={newAcctError || undefined}>
                <Input
                  value={newAcctName}
                  onChange={(event) => setNewAcctName(event.target.value)}
                  placeholder={t('settings.accounts.namePlaceholder')}
                />
              </Field>
              <div>
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">{t('settings.accounts.publicLabel')}</p>
                <Select
                  value={newAcctType}
                  onChange={(value) => setNewAcctType(value as 'personal' | 'public')}
                  size="sm"
                  options={isWorkMode
                    ? [{ value: 'public', label: t('settings.accounts.publicAccount') }]
                    : [{ value: 'personal', label: t('settings.accounts.personalAccount') }]}
                />
              </div>
              <div>
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">{newAcctCurrency}</p>
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
              <Button type="submit" loading={newAcctLoading} loadingText={t('settings.accounts.creating')}>
                {t('settings.accounts.create')}
              </Button>
            </form>
            {newAcctSuccess ? <Alert variant="positive">{t('settings.accounts.toast.created')}</Alert> : null}
          </CardContent>
        </Card>
      </section>

      <section>
        <SectionHeading>{t('settings.sections.profile')}</SectionHeading>
        <Card>
          <div className="flex items-start gap-4">
            <div className="grid size-12 shrink-0 place-items-center rounded-full border border-border bg-muted text-base font-semibold text-foreground" aria-hidden="true">
              {(displayName[0] ?? '?').toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              {editingNickname ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Input
                    type="text"
                    value={nicknameInput}
                    onChange={(event) => setNicknameInput(event.target.value)}
                    className="w-48"
                    autoFocus
                    aria-label={t('settings.profile.nicknameTip')}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') void handleSaveNickname()
                      if (event.key === 'Escape') setEditingNickname(false)
                    }}
                  />
                  <Button size="sm" loading={nicknameLoading} loadingText={t('common.saving')} onClick={() => void handleSaveNickname()}>
                    {t('common.save')}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setEditingNickname(false)}>
                    {t('common.cancel')}
                  </Button>
                </div>
              ) : (
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-base font-semibold text-foreground">{displayName}</p>
                  <Button variant="ghost" size="sm" onClick={startEditNickname}>
                    <Pencil className="size-3.5" />
                    {t('settings.profile.nicknameTip')}
                  </Button>
                  {nicknameSuccess ? <Badge variant="positive">{t('settings.profile.toast.updated')}</Badge> : null}
                </div>
              )}
              {nicknameError ? <p className="mt-1 text-xs text-negative">{nicknameError}</p> : null}
              <div className="mt-1 flex flex-wrap items-center gap-2">
                <p className="text-sm text-muted-foreground">@{profile?.username || user?.username}</p>
                <Badge>{t('settings.profile.usernameTip')}</Badge>
              </div>
              <p className="mt-0.5 text-sm text-muted-foreground">{currentEmail}</p>
              {pendingEmail ? (
                <p className="mt-1 flex items-center gap-1.5 text-xs text-warning">
                  <AlertTriangle className="size-3.5 shrink-0" />
                  {t('settings.changeEmail.pendingTo')}{pendingEmail}
                </p>
              ) : null}
            </div>
          </div>
        </Card>
      </section>

      <div className="grid gap-5 md:grid-cols-2 [&>section:last-child]:md:col-span-2">
        <MobileCollapsibleSection title={t('settings.sections.changeEmail')}>
          <div className="flex h-full flex-col">
            <div className="hidden md:block"><SectionHeading>{t('settings.sections.changeEmail')}</SectionHeading></div>
            <Card className="flex-1">
              <CardHeader>
                <div>
                  <CardTitle>{t('settings.sections.changeEmail')}</CardTitle>
                  <CardDescription>{t('settings.changeEmail.currentEmail')}: {currentEmail}</CardDescription>
                </div>
              </CardHeader>
              <CardContent>
                {pendingEmail && !emailSent ? (
                  <Alert variant="warning" className="mb-4 block">
                    <p className="font-semibold">{t('settings.changeEmail.pendingTitle')}</p>
                    <p className="mt-0.5">
                      <Trans
                        i18nKey="settings.changeEmail.pendingDesc"
                        values={{ email: pendingEmail }}
                        components={{ strong: <strong /> }}
                      />
                    </p>
                  </Alert>
                ) : null}

                {emailSent ? (
                  <Alert variant="positive" className="block">
                    <p className="font-semibold">{t('settings.changeEmail.sentTitle')}</p>
                    <p className="mt-0.5">
                      <Trans
                        i18nKey="settings.changeEmail.sentDesc"
                        values={{ currentEmail, pendingEmail: profile?.pending_email }}
                        components={{ strong: <strong /> }}
                      />
                    </p>
                  </Alert>
                ) : (
                  <form onSubmit={handleRequestEmailChange} className="space-y-3">
                    <Field label={t('settings.changeEmail.newEmail')}>
                      <Input
                        type="email"
                        required
                        value={newEmail}
                        onChange={(event) => setNewEmail(event.target.value)}
                        placeholder={t('settings.changeEmail.placeholder')}
                        autoComplete="email"
                      />
                    </Field>
                    <Field label={t('settings.password.current')}>
                      <Input
                        type="password"
                        required
                        value={emailCurrentPw}
                        onChange={(event) => setEmailCurrentPw(event.target.value)}
                        placeholder={t('settings.password.currentPlaceholder')}
                        autoComplete="current-password"
                      />
                    </Field>
                    {emailError ? <Alert variant="negative">{emailError}</Alert> : null}
                    <Button type="submit" loading={emailLoading} loadingText={t('settings.changeEmail.sending')}>
                      {t('settings.changeEmail.submit')}
                    </Button>
                  </form>
                )}
              </CardContent>
            </Card>
          </div>
        </MobileCollapsibleSection>

        <MobileCollapsibleSection title={t('settings.sections.security')}>
          <div className="flex h-full flex-col">
            <div className="hidden md:block"><SectionHeading>{t('settings.sections.security')}</SectionHeading></div>
            <Card className="flex-1">
              <CardHeader>
                <div>
                  <CardTitle>{t('settings.sections.security')}</CardTitle>
                  <CardDescription>{t('settings.password.minLength')}</CardDescription>
                </div>
                <LockKeyhole className="size-4 text-muted-foreground" aria-hidden="true" />
              </CardHeader>
              <CardContent>
                {pwSuccess ? <Alert variant="positive" className="mb-4">{t('settings.password.toast.success')}</Alert> : null}
                <form onSubmit={handleChangePassword} className="space-y-4">
                  <Field label={t('settings.password.current')}>
                    <Input
                      type="password"
                      required
                      value={currentPw}
                      onChange={(event) => setCurrentPw(event.target.value)}
                      placeholder={t('settings.password.currentPlaceholder')}
                      autoComplete="current-password"
                    />
                  </Field>
                  <div className="grid gap-1.5">
                    <Label htmlFor="settings-new-password">{t('settings.password.new')}</Label>
                    <Input
                      id="settings-new-password"
                      type="password"
                      required
                      minLength={8}
                      value={newPw}
                      onChange={(event) => setNewPw(event.target.value)}
                      placeholder={t('settings.password.newPlaceholder')}
                      autoComplete="new-password"
                    />
                    <PasswordStrength password={newPw} />
                  </div>
                  <Field
                    label={t('settings.password.confirm')}
                    error={confirmPw && newPw !== confirmPw ? t('settings.password.toast.mismatch') : undefined}
                  >
                    <Input
                      type="password"
                      required
                      minLength={8}
                      value={confirmPw}
                      onChange={(event) => setConfirmPw(event.target.value)}
                      placeholder={t('settings.password.confirmPlaceholder')}
                      autoComplete="new-password"
                    />
                  </Field>
                  {pwError ? <Alert variant="negative">{pwError}</Alert> : null}
                  <Button
                    type="submit"
                    className="w-full"
                    loading={pwLoading}
                    loadingText={t('settings.password.submitting')}
                    disabled={Boolean(confirmPw && newPw !== confirmPw)}
                  >
                    {t('settings.password.submit')}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div>
        </MobileCollapsibleSection>

        <MobileCollapsibleSection title={t('settings.sections.backup')}>
          <div className="flex h-full flex-col">
            <div className="hidden md:block"><SectionHeading>{t('settings.sections.backup')}</SectionHeading></div>
            <Card className="flex-1">
              <CardHeader>
                <CardTitle>{t('settings.sections.backup')}</CardTitle>
              </CardHeader>
              <CardContent>
                <OperationsNotice title={operationsTitle} description={operationsDescription} />
              </CardContent>
            </Card>
          </div>
        </MobileCollapsibleSection>

        <MobileCollapsibleSection title={t('settings.sections.restore')}>
          <div className="flex h-full flex-col">
            <div className="hidden md:block"><SectionHeading>{t('settings.sections.restore')}</SectionHeading></div>
            <Card className="flex-1 border-warning/35">
              <CardHeader>
                <CardTitle>{t('settings.sections.restore')}</CardTitle>
              </CardHeader>
              <CardContent>
                <OperationsNotice title={operationsTitle} description={operationsDescription} />
              </CardContent>
            </Card>
          </div>
        </MobileCollapsibleSection>

        <SupportSection />

        <MobileCollapsibleSection title={t('settings.sections.danger')}>
          <div className="md:col-span-2">
            <div className="hidden md:block"><SectionHeading>{t('settings.sections.danger')}</SectionHeading></div>
            <Card className="border-negative/35">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0 flex-1">
                  <CardTitle className="text-negative">{t('settings.danger.deleteAccount')}</CardTitle>
                  <CardDescription>{t('settings.danger.deleteDesc')}</CardDescription>
                </div>
                <div className="shrink-0">
                  {deleteStep === 'sent' ? (
                    <Alert variant="positive">
                      <Trans
                        i18nKey="settings.danger.emailSent"
                        values={{ email: currentEmail }}
                        components={{ strong: <strong /> }}
                      />
                    </Alert>
                  ) : deleteStep === 'confirm' || deleteStep === 'loading' ? (
                    <div className="max-w-sm space-y-3 rounded-lg border border-negative/35 bg-negative-soft p-4 text-negative">
                      <p className="text-sm font-semibold">{t('settings.danger.confirmTitle')}</p>
                      <p className="text-xs">
                        <Trans
                          i18nKey="settings.danger.confirmDesc"
                          values={{ email: currentEmail }}
                          components={{ strong: <strong /> }}
                        />
                      </p>
                      {deleteError ? <Alert variant="negative">{deleteError}</Alert> : null}
                      <div className="flex flex-wrap gap-2">
                        <Button
                          variant="danger"
                          loading={deleteStep === 'loading'}
                          loadingText={t('settings.danger.sending')}
                          onClick={() => void handleRequestDelete()}
                        >
                          {t('settings.danger.sendConfirm')}
                        </Button>
                        <Button
                          variant="outline"
                          disabled={deleteStep === 'loading'}
                          onClick={() => {
                            setDeleteStep('idle')
                            setDeleteError('')
                          }}
                        >
                          {t('common.cancel')}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <Button variant="danger" onClick={() => setDeleteStep('confirm')}>
                      <Trash2 className="size-4" />
                      {t('settings.danger.deleteButton')}
                    </Button>
                  )}
                </div>
              </div>
            </Card>
          </div>
        </MobileCollapsibleSection>
      </div>

    </div>
  )
}
