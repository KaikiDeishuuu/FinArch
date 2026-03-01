import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import {
  changePassword, downloadBackup, restoreBackup,
  requestDeleteAccount, requestEmailChange, getMe,
  createAccount, renameAccount,
} from '../api/client'
import type { UserProfile } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { useAccounts, useInvalidateAccounts } from '../hooks/useAccounts'

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
function PasswordStrength({ password }: { password: string }) {
  const s = calcStrength(password)
  if (!password) return null
  const bar = { none: 'w-0', weak: 'w-1/3', medium: 'w-2/3', strong: 'w-full' }[s]
  const color = { none: '', weak: 'bg-rose-400', medium: 'bg-amber-400', strong: 'bg-emerald-500' }[s]
  const label = { none: '', weak: '弱', medium: '中等 — 添加特殊字符可进一步增强', strong: '强' }[s]
  const tc = { none: '', weak: 'text-rose-500', medium: 'text-amber-600', strong: 'text-emerald-500' }[s]
  return (
    <div className="mt-1.5 space-y-1">
      <div className="h-1 w-full bg-gray-100 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
}

// ─── Shared styles ────────────────────────────────────────────────────────────
const labelCls = 'block text-sm font-medium text-gray-700 mb-1'
const inputCls = 'w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-teal-500 focus:border-transparent transition bg-white'

// ─── Section header ───────────────────────────────────────────────────────────
function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 mb-3">
      <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider whitespace-nowrap">
        {children}
      </span>
      <div className="flex-1 h-px bg-gray-100" />
    </div>
  )
}

// ─── Alert components ─────────────────────────────────────────────────────────
function Alert({ type, children }: { type: 'success' | 'error' | 'info' | 'warning'; children: React.ReactNode }) {
  const cls = {
    success: 'bg-emerald-50 border-green-200 text-emerald-700',
    error:   'bg-rose-50 border-rose-200 text-rose-700',
    info:    'bg-teal-50 border-teal-200 text-teal-700',
    warning: 'bg-amber-50 border-amber-200 text-amber-800',
  }[type]
  return (
    <div className={`border rounded-xl px-4 py-3 text-sm ${cls}`}>{children}</div>
  )
}

// ─── Main component ───────────────────────────────────────────────────────────
export default function SettingsPage() {
  const { user } = useAuth()
  const { data: accounts = [], isLoading: acctLoading } = useAccounts()
  const invalidateAccounts = useInvalidateAccounts()
  // ── Profile data ──────────────────────────────────────────────────────────
  const [profile, setProfile] = useState<UserProfile | null>(null)
  useEffect(() => {
    getMe().then(setProfile).catch(() => {/* ignore */})
  }, [])

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
    if (newPw !== confirmPw) { setPwError('两次输入的新密码不一致'); return }
    if (newPw.length < 8) { setPwError('新密码至少需要 8 位'); return }
    setPwLoading(true)
    try {
      await changePassword(currentPw, newPw)
      setPwSuccess(true)
      setCurrentPw(''); setNewPw(''); setConfirmPw('')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setPwError(msg || '修改失败，请检查当前密码是否正确')
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
      await requestEmailChange(newEmail)
      setEmailSent(true)
      setNewEmail('')
      // Refresh profile to show pending_email
      getMe().then(setProfile).catch(() => {/* ignore */})
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setEmailError(msg || '请求失败，请稍后重试')
    } finally {
      setEmailLoading(false)
    }
  }

  // ── Backup ────────────────────────────────────────────────────────────────
  const [backupLoading, setBackupLoading] = useState(false)
  const [backupError, setBackupError] = useState('')

  async function handleDownloadBackup() {
    setBackupError('')
    setBackupLoading(true)
    try { await downloadBackup() }
    catch { setBackupError('备份下载失败，请重试') }
    finally { setBackupLoading(false) }
  }

  // ── Restore ───────────────────────────────────────────────────────────────
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const [restoreLoading, setRestoreLoading] = useState(false)
  const [restoreError, setRestoreError] = useState('')
  const [restoreSuccess, setRestoreSuccess] = useState(false)
  const [restoreConfirm, setRestoreConfirm] = useState(false)

  async function handleRestore() {
    if (!restoreFile) return
    setRestoreError(''); setRestoreSuccess(false); setRestoreLoading(true)
    try {
      await restoreBackup(restoreFile)
      setRestoreSuccess(true)
      setRestoreFile(null); setRestoreConfirm(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setRestoreError(msg || '恢复失败，请检查文件是否为有效的 FinArch 备份')
    } finally { setRestoreLoading(false) }
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
      setDeleteError(msg || '请求失败，请重试')
      setDeleteStep('confirm')
    }
  }
  // ── Account management ─────────────────────────────────────────────────
  const [newAcctName, setNewAcctName] = useState('')
  const [newAcctType, setNewAcctType] = useState<'personal' | 'public'>('personal')
  const [newAcctCurrency, setNewAcctCurrency] = useState('CNY')
  const [newAcctLoading, setNewAcctLoading] = useState(false)
  const [newAcctError, setNewAcctError] = useState('')
  const [newAcctSuccess, setNewAcctSuccess] = useState(false)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [renameLoading, setRenameLoading] = useState(false)

  async function handleCreateAccount(e: FormEvent) {
    e.preventDefault()
    setNewAcctError('')
    setNewAcctSuccess(false)
    if (!newAcctName.trim()) { setNewAcctError('请输入账户名称'); return }
    setNewAcctLoading(true)
    try {
      await createAccount(newAcctName.trim(), newAcctType, newAcctCurrency)
      await invalidateAccounts()
      setNewAcctName('')
      setNewAcctSuccess(true)
      setTimeout(() => setNewAcctSuccess(false), 2000)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setNewAcctError(msg || '创建失败')
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
  const displayName = profile?.username || user?.username || user?.email || '—'
  const currentEmail = profile?.email || user?.email || '—'
  const pendingEmail = profile?.pending_email

  return (
    <div className="pb-8 max-w-4xl">
      {/* Page header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">账户设置</h1>
        <p className="text-sm text-gray-400 mt-1">管理您的账户信息、安全设置与数据备份</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
        {/* ── Fund Accounts ───────────────────────────── full width ── */}
        <div className="md:col-span-2">
          <SectionLabel>资金账户</SectionLabel>
          <div className="bg-white rounded-2xl border border-gray-100 p-5 space-y-4">
            {/* Account list */}
            {acctLoading ? (
              <div className="flex items-center gap-2 text-sm text-gray-400">
                <span className="w-4 h-4 border-2 border-gray-300 border-t-teal-500 rounded-full animate-spin" />
                加载中…
              </div>
            ) : accounts.length === 0 ? (
              <p className="text-sm text-gray-400">暂无账户，请在下方创建第一个资金账户。</p>
            ) : (
              <div className="divide-y divide-gray-50">
                {accounts.map(a => {
                  const isPublic = a.type === 'public'
                  const typeBadge = isPublic
                    ? 'bg-teal-50 text-teal-700 border border-teal-100'
                    : 'bg-amber-50 text-amber-700 border border-amber-100'
                  const typeLabel = isPublic ? '公司' : '个人'
                  const balanceColor = a.balance_yuan >= 0 ? 'text-emerald-600' : 'text-rose-500'
                  return (
                    <div key={a.id} className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
                      <span className={`text-xs font-semibold px-2 py-0.5 rounded-full shrink-0 ${typeBadge}`}>{typeLabel}</span>
                      {renamingId === a.id ? (
                        <form onSubmit={(e) => { e.preventDefault(); handleRenameAccount(a.id) }}
                          className="flex flex-1 gap-2">
                          <input
                            autoFocus
                            value={renameValue}
                            onChange={(e) => setRenameValue(e.target.value)}
                            className="flex-1 border border-teal-300 rounded-lg px-2.5 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-teal-400"
                          />
                          <button type="submit" disabled={renameLoading}
                            className="text-xs bg-teal-600 text-white px-3 py-1 rounded-lg disabled:opacity-50">
                            {renameLoading ? '保存中…' : '保存'}
                          </button>
                          <button type="button" onClick={() => setRenamingId(null)}
                            className="text-xs text-gray-400 hover:text-gray-600 px-2 py-1">取消</button>
                        </form>
                      ) : (
                        <>
                          <span className="flex-1 text-sm font-medium text-gray-800 min-w-0 truncate">{a.name}</span>
                          <span className={`text-sm font-bold tabular-nums shrink-0 ${balanceColor}`}>
                            {a.balance_yuan >= 0 ? '' : '−'}¥{Math.abs(a.balance_yuan).toFixed(2)}
                            <span className="text-xs font-normal text-gray-400 ml-1">{a.currency}</span>
                          </span>
                          <button
                            type="button"
                            onClick={() => { setRenamingId(a.id); setRenameValue(a.name) }}
                            className="text-xs text-gray-400 hover:text-teal-600 px-2 py-1 rounded-lg hover:bg-gray-50 transition-colors shrink-0"
                          >改名</button>
                        </>
                      )}
                    </div>
                  )
                })}
              </div>
            )}

            {/* Create account form */}
            <div className="border-t border-gray-100 pt-4">
              <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">新建账户</p>
              <form onSubmit={handleCreateAccount} className="flex flex-wrap gap-2">
                <input
                  value={newAcctName}
                  onChange={(e) => setNewAcctName(e.target.value)}
                  placeholder="账户名称"
                  className={`${inputCls} flex-1 min-w-32 py-2`}
                />
                <select
                  value={newAcctType}
                  onChange={(e) => setNewAcctType(e.target.value as 'personal' | 'public')}
                  className={`${inputCls} w-auto py-2`}
                >
                  <option value="personal">个人账户</option>
                  <option value="public">公司账户</option>
                </select>
                <select
                  value={newAcctCurrency}
                  onChange={(e) => setNewAcctCurrency(e.target.value)}
                  className={`${inputCls} w-auto py-2`}
                >
                  <option value="CNY">CNY</option>
                  <option value="USD">USD</option>
                  <option value="EUR">EUR</option>
                </select>
                <button type="submit" disabled={newAcctLoading}
                  className="bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white text-sm font-medium px-4 py-2 rounded-xl transition-colors">
                  {newAcctLoading ? '创建中…' : '+ 新建'}
                </button>
              </form>
              {newAcctError && <p className="text-xs text-rose-500 mt-2">{newAcctError}</p>}
              {newAcctSuccess && <p className="text-xs text-emerald-600 mt-2">✓ 账户已创建</p>}
            </div>
          </div>
        </div>
        {/* ── Profile ─────────────────────────────────────────── full width ── */}
        <div className="md:col-span-2">
          <SectionLabel>账户信息</SectionLabel>
          <div className="bg-white rounded-2xl border border-gray-100 p-5">
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 rounded-full bg-gradient-to-br from-teal-400 to-emerald-600 text-white flex items-center justify-center text-xl font-bold shrink-0">
                {(displayName[0] ?? '?').toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  <p className="font-semibold text-gray-900 text-base">{displayName}</p>
                  <span className="text-[10px] font-medium bg-gray-100 text-gray-500 px-2 py-0.5 rounded-full">用户名 · 不可修改</span>
                </div>
                <p className="text-sm text-gray-500 mt-0.5">{currentEmail}</p>
                {pendingEmail && (
                  <p className="text-xs text-amber-600 mt-1 flex items-center gap-1">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="w-3 h-3 shrink-0"><circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/></svg>
                    待验证新邮箱：{pendingEmail}
                  </p>
                )}
              </div>
            </div>
          </div>
        </div>

        {/* ── Change email ─────────────────────────────────────── col 1 ── */}
        <div className="flex flex-col">
          <SectionLabel>更换邮箱</SectionLabel>
          <div className="bg-white rounded-2xl border border-gray-100 p-5 space-y-4 flex-1">
            <div className="space-y-0.5">
              <p className="text-sm text-gray-700 font-medium">当前邮箱</p>
              <p className="text-sm text-gray-500">{currentEmail}</p>
            </div>

            {pendingEmail && !emailSent && (
              <Alert type="warning">
                <p className="font-medium mb-0.5">有待验证的邮箱变更请求</p>
                <p>授权邮件将发至当前邮箱，新邮箱 <strong>{pendingEmail}</strong> 尚待验证。<br />请先完成当前请求再提交新申请。</p>
              </Alert>
            )}

            {emailSent ? (
              <Alert type="success">
                <p className="font-medium mb-0.5">授权邮件已发送至当前邮箱</p>
                请检查 <strong>{currentEmail}</strong> 的收件箱，点击邮件中的「授权更换」链接。<br />
                授权后系统将向新邮箱 <strong>{profile?.pending_email}</strong> 发送最终验证邮件。
              </Alert>
            ) : (
              <form onSubmit={handleRequestEmailChange} className="space-y-3">
                <div>
                  <label className={labelCls}>新邮箱地址</label>
                  <input type="email" required value={newEmail}
                    onChange={(e) => setNewEmail(e.target.value)}
                    className={inputCls} placeholder="输入新邮箱地址" />
                </div>
                {emailError && <Alert type="error">{emailError}</Alert>}
                <button type="submit" disabled={emailLoading}
                  className="bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white text-sm font-medium px-5 py-2.5 rounded-xl transition-colors">
                  {emailLoading ? '发送中...' : '发送授权邮件'}
                </button>
              </form>
            )}
          </div>
        </div>

        {/* ── Change password ──────────────────────────────────── col 2 ── */}
        <div className="flex flex-col">
          <SectionLabel>安全</SectionLabel>
          <div className="bg-white rounded-2xl border border-gray-100 p-5 flex-1">
            {pwSuccess && <div className="mb-4"><Alert type="success">✓ 密码修改成功！</Alert></div>}
            <form onSubmit={handleChangePassword} className="space-y-4">
              <div>
                <label className={labelCls}>当前密码</label>
                <input type="password" required value={currentPw}
                  onChange={(e) => setCurrentPw(e.target.value)}
                  className={inputCls} placeholder="请输入当前密码" autoComplete="current-password" />
              </div>
              <div>
                <label className={labelCls}>新密码</label>
                <input type="password" required minLength={8} value={newPw}
                  onChange={(e) => setNewPw(e.target.value)}
                  className={inputCls} placeholder="至少 8 位，建议大小写 + 数字 + 符号" autoComplete="new-password" />
                <PasswordStrength password={newPw} />
              </div>
              <div>
                <label className={labelCls}>确认新密码</label>
                <input type="password" required minLength={8} value={confirmPw}
                  onChange={(e) => setConfirmPw(e.target.value)}
                  className={inputCls} placeholder="再次输入新密码" autoComplete="new-password" />
                {confirmPw && newPw !== confirmPw && (
                  <p className="mt-1 text-xs text-rose-500">两次输入的密码不一致</p>
                )}
              </div>
              {pwError && <Alert type="error">{pwError}</Alert>}
              <button type="submit" disabled={pwLoading || (!!confirmPw && newPw !== confirmPw)}
                className="w-full bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white font-medium rounded-xl py-2.5 text-sm transition-colors">
                {pwLoading ? '提交中...' : '修改密码'}
              </button>
            </form>
          </div>
        </div>

        {/* ── Backup ───────────────────────────────────────────── col 1 ── */}
        <div className="flex flex-col">
          <SectionLabel>数据备份</SectionLabel>
          <div className="bg-white rounded-2xl border border-gray-100 p-5 flex-1">
            <p className="text-xs text-gray-400 mb-4">下载当前数据库的完整快照（标准 SQLite 格式），可用于迁移或恢复。</p>
            {backupError && <div className="mb-3"><Alert type="error">{backupError}</Alert></div>}
            <button type="button" onClick={handleDownloadBackup} disabled={backupLoading}
              className="inline-flex items-center gap-2 bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white text-sm font-medium px-4 py-2.5 rounded-xl transition-colors">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
                <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
              </svg>
              {backupLoading ? '生成中...' : '下载备份'}
            </button>
          </div>
        </div>

        {/* ── Restore ──────────────────────────────────────────── col 2 ── */}
        <div className="flex flex-col">
          <SectionLabel>数据恢复</SectionLabel>
          <div className="bg-white rounded-2xl border border-amber-100 p-5 flex-1">
            <p className="text-xs text-gray-400 mb-4">
              上传之前下载的 <code className="font-mono bg-gray-100 px-1 rounded">.db</code> 备份文件，将<strong>覆盖当前所有数据</strong>，操作不可撤销。
            </p>
            {restoreSuccess && <div className="mb-3"><Alert type="success">✓ 数据恢复成功！</Alert></div>}
            {restoreError && <div className="mb-3"><Alert type="error">{restoreError}</Alert></div>}
            <div className="space-y-3">
              <input ref={fileInputRef} type="file" accept=".db"
                onChange={(e) => {
                  const f = e.target.files?.[0] ?? null
                  setRestoreFile(f); setRestoreConfirm(false)
                  setRestoreError(''); setRestoreSuccess(false)
                }}
                className="block w-full text-sm text-gray-500 file:mr-3 file:py-2 file:px-3 file:rounded-lg file:border-0 file:text-sm file:font-medium file:bg-gray-100 file:text-gray-700 hover:file:bg-gray-200 transition"
              />
              {restoreFile && !restoreConfirm && (
                <div className="bg-amber-50 border border-amber-200 rounded-xl p-3 text-sm text-amber-800">
                  <p className="font-semibold mb-1">确认恢复：{restoreFile.name}</p>
                  <p className="text-xs text-amber-600 mb-2">此操作将<strong>覆盖当前所有数据</strong>，恢复为备份时的状态。</p>
                  <button type="button" onClick={() => setRestoreConfirm(true)}
                    className="bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold px-3 py-1.5 rounded-lg transition">
                    我已了解，确认恢复
                  </button>
                </div>
              )}
              {restoreFile && restoreConfirm && (
                <button type="button" onClick={handleRestore} disabled={restoreLoading}
                  className="inline-flex items-center gap-2 bg-rose-500 hover:bg-rose-600 disabled:opacity-50 text-white text-sm font-medium px-4 py-2.5 rounded-xl transition-colors">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
                    <polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 102.13-9.36L1 10"/>
                  </svg>
                  {restoreLoading ? '恢复中...' : '立即恢复数据'}
                </button>
              )}
            </div>
          </div>
        </div>

        {/* ── Danger zone ─────────────────────────────────────── full width ── */}
        <div className="md:col-span-2">
          <SectionLabel>危险区域</SectionLabel>
          <div className="bg-white rounded-2xl border border-rose-200 p-5">
            <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
              <div className="flex-1 min-w-0">
                <h3 className="text-sm font-semibold text-rose-600 mb-1">注销账户</h3>
                <p className="text-xs text-gray-400">
                  将永久删除您的账户及所有数据（标签、资金池、交易记录），此操作<strong>不可撤销</strong>。
                </p>
              </div>
              <div className="shrink-0">
                {deleteStep === 'sent' ? (
                  <Alert type="success">
                    ✓ 验证邮件已发送至 <strong>{currentEmail}</strong>，请在 1 小时内点击链接以完成账户注销。
                  </Alert>
                ) : deleteStep === 'confirm' || deleteStep === 'loading' ? (
                  <div className="bg-rose-50 border border-rose-200 rounded-xl p-4 space-y-3 max-w-sm">
                    <p className="text-sm font-semibold text-rose-700">确认注销账户？</p>
                    <p className="text-xs text-rose-600">
                      我们将向 <strong>{currentEmail}</strong> 发送确认邮件，点击链接后账户将被<strong>永久删除</strong>。
                    </p>
                    {deleteError && <Alert type="error">{deleteError}</Alert>}
                    <div className="flex gap-2">
                      <button type="button" onClick={handleRequestDelete} disabled={deleteStep === 'loading'}
                        className="bg-rose-600 hover:bg-rose-700 disabled:opacity-50 text-white text-sm font-semibold px-4 py-2 rounded-xl transition-colors">
                        {deleteStep === 'loading' ? '发送中...' : '发送注销确认邮件'}
                      </button>
                      <button type="button" onClick={() => { setDeleteStep('idle'); setDeleteError('') }}
                        disabled={deleteStep === 'loading'}
                        className="text-sm text-gray-500 hover:text-gray-700 px-4 py-2 rounded-xl border border-gray-200 hover:border-gray-300 transition-colors">
                        取消
                      </button>
                    </div>
                  </div>
                ) : (
                  <button type="button" onClick={() => setDeleteStep('confirm')}
                    className="inline-flex items-center gap-2 border border-rose-200 hover:bg-rose-50 text-rose-600 text-sm font-medium px-4 py-2.5 rounded-xl transition-colors">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
                      <polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6"/>
                      <path d="M10 11v6M14 11v6M9 6V4a1 1 0 011-1h4a1 1 0 011 1v2"/>
                    </svg>
                    申请注销账户
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  )
}
