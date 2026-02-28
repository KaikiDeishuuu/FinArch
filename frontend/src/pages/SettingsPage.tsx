import { useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { changePassword, downloadBackup, restoreBackup } from '../api/client'
import { useAuth } from '../contexts/AuthContext'

export default function SettingsPage() {
  const { user } = useAuth()
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)

  // Backup
  const [backupLoading, setBackupLoading] = useState(false)
  const [backupError, setBackupError] = useState('')

  // Restore
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const [restoreLoading, setRestoreLoading] = useState(false)
  const [restoreError, setRestoreError] = useState('')
  const [restoreSuccess, setRestoreSuccess] = useState(false)
  const [restoreConfirm, setRestoreConfirm] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setSuccess(false)
    if (newPassword !== confirmPassword) { setError('两次输入的新密码不一致'); return }
    if (newPassword.length < 8) { setError('新密码至少需要 8 位'); return }
    setLoading(true)
    try {
      await changePassword(currentPassword, newPassword)
      setSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '修改失败，请重试')
    } finally {
      setLoading(false)
    }
  }

  async function handleDownloadBackup() {
    setBackupError('')
    setBackupLoading(true)
    try {
      await downloadBackup()
    } catch {
      setBackupError('备份下载失败，请重试')
    } finally {
      setBackupLoading(false)
    }
  }

  async function handleRestore() {
    if (!restoreFile) return
    setRestoreError('')
    setRestoreSuccess(false)
    setRestoreLoading(true)
    try {
      await restoreBackup(restoreFile)
      setRestoreSuccess(true)
      setRestoreFile(null)
      setRestoreConfirm(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setRestoreError(msg || '恢复失败，请检查文件是否为有效的 FinArch 备份')
    } finally {
      setRestoreLoading(false)
    }
  }

  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'
  const inputClass = 'w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition'

  return (
    <div className="space-y-5 max-w-lg">
      <div>
        <h1 className="text-2xl font-bold text-gray-800">账户设置</h1>
        <p className="text-sm text-gray-400 mt-1">管理您的账户信息与数据备份</p>
      </div>

      {/* User info card */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-4">基本信息</h2>
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-lg font-bold shrink-0">
            {((user?.name || user?.email || '?')[0]).toUpperCase()}
          </div>
          <div>
            <p className="font-medium text-gray-800">{user?.name || '—'}</p>
            <p className="text-sm text-gray-400">{user?.email}</p>
          </div>
        </div>
      </div>

      {/* Change password card */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-4">修改密码</h2>
        {success && (
          <div className="mb-4 bg-green-50 border border-green-200 text-green-700 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
            <span>✓</span><span>密码修改成功！</span>
          </div>
        )}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className={labelClass}>当前密码</label>
            <input type="password" required value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} className={inputClass} placeholder="请输入当前密码" />
          </div>
          <div>
            <label className={labelClass}>新密码</label>
            <input type="password" required minLength={8} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className={inputClass} placeholder="至少 8 位" />
          </div>
          <div>
            <label className={labelClass}>确认新密码</label>
            <input type="password" required minLength={8} value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} className={inputClass} placeholder="再次输入新密码" />
          </div>
          {error && <div className="bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-3 text-sm">{error}</div>}
          <button type="submit" disabled={loading} className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-medium rounded-xl py-2.5 text-sm transition-colors">
            {loading ? '提交中...' : '修改密码'}
          </button>
        </form>
      </div>

      {/* Backup card */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-1">数据备份</h2>
        <p className="text-xs text-gray-400 mb-4">下载当前数据库的完整快照，文件为标准 SQLite 格式，可用于迁移或恢复。</p>
        {backupError && <div className="mb-3 bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-3 text-sm">{backupError}</div>}
        <button
          onClick={handleDownloadBackup}
          disabled={backupLoading}
          className="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium px-4 py-2.5 rounded-xl transition-colors"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
            <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>
          </svg>
          {backupLoading ? '生成中...' : '下载备份'}
        </button>
      </div>

      {/* Restore card */}
      <div className="bg-white rounded-2xl shadow-sm border border-amber-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-1">数据恢复</h2>
        <p className="text-xs text-gray-400 mb-4">上传之前下载的 <code className="font-mono bg-gray-100 px-1 rounded">.db</code> 备份文件，将覆盖当前所有数据，操作不可撤销。</p>

        {restoreSuccess && (
          <div className="mb-3 bg-green-50 border border-green-200 text-green-700 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
            <span>✓</span><span>数据恢复成功！页面将在 3 秒后刷新。</span>
          </div>
        )}
        {restoreError && <div className="mb-3 bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-3 text-sm">{restoreError}</div>}

        <div className="space-y-3">
          <div>
            <label className={labelClass}>选择备份文件</label>
            <input
              ref={fileInputRef}
              type="file"
              accept=".db"
              onChange={(e) => {
                const f = e.target.files?.[0] ?? null
                setRestoreFile(f)
                setRestoreConfirm(false)
                setRestoreError('')
                setRestoreSuccess(false)
              }}
              className="block w-full text-sm text-gray-500 file:mr-3 file:py-2 file:px-3 file:rounded-lg file:border-0 file:text-sm file:font-medium file:bg-gray-100 file:text-gray-700 hover:file:bg-gray-200 transition"
            />
          </div>

          {restoreFile && !restoreConfirm && (
            <div className="bg-amber-50 border border-amber-200 rounded-xl p-3 text-sm text-amber-800">
              <p className="font-semibold mb-1">确认恢复：{restoreFile.name}</p>
              <p className="text-xs text-amber-600 mb-2">此操作将<strong>覆盖当前所有数据</strong>，恢复为备份时的状态，无法撤销。</p>
              <button
                onClick={() => setRestoreConfirm(true)}
                className="bg-amber-500 hover:bg-amber-600 text-white text-xs font-semibold px-3 py-1.5 rounded-lg transition"
              >
                我已了解，确认恢复
              </button>
            </div>
          )}

          {restoreFile && restoreConfirm && (
            <button
              onClick={handleRestore}
              disabled={restoreLoading}
              className="flex items-center gap-2 bg-red-500 hover:bg-red-600 disabled:opacity-50 text-white text-sm font-medium px-4 py-2.5 rounded-xl transition-colors"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
                <polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 102.13-9.36L1 10"/>
              </svg>
              {restoreLoading ? '恢复中...' : '立即恢复数据'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}


