import { useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { resetPassword } from '../api/client'

export default function ResetPasswordPage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState('')

  if (!token) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
        <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md text-center">
          <p className="text-red-600 text-sm mb-4">无效的重置链接，请重新申请。</p>
          <Link to="/forgot-password" className="text-sm text-teal-600 hover:underline">重新申请</Link>
        </div>
      </div>
    )
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (newPassword !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setLoading(true)
    try {
      await resetPassword(token, newPassword)
      setSuccess(true)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '重置失败，链接可能已过期，请重新申请')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md">
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-xl bg-teal-600 flex items-center justify-center shadow-md mb-3">
            <span className="text-white text-xl font-bold">¥</span>
          </div>
          <h1 className="text-xl font-bold text-gray-800">设置新密码</h1>
          <p className="text-xs text-gray-400 mt-0.5">FinArch v2</p>
        </div>

        {success ? (
          <div className="text-center">
            <div className="w-12 h-12 rounded-full bg-emerald-100 flex items-center justify-center mx-auto mb-4">
              <svg viewBox="0 0 24 24" fill="none" stroke="#16a34a" strokeWidth={2} className="w-6 h-6">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <p className="text-gray-700 font-medium mb-1">密码重置成功！</p>
            <p className="text-gray-400 text-sm mb-5">请使用新密码登录。</p>
            <Link to="/login" className="bg-teal-600 hover:bg-teal-700 text-white text-sm font-medium px-6 py-2 rounded-lg transition-colors">
              前往登录
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">新密码</label>
              <input
                type="password"
                required
                minLength={8}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-500"
                placeholder="至少 8 位"
                autoFocus
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">确认新密码</label>
              <input
                type="password"
                required
                minLength={8}
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-500"
                placeholder="再次输入新密码"
              />
            </div>
            {error && (
              <div className="bg-red-50 border border-red-200 text-red-700 rounded-lg px-3 py-2 text-sm">{error}</div>
            )}
            <button
              type="submit"
              disabled={loading}
              className="w-full bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white font-medium rounded-lg py-2 text-sm transition-colors"
            >
              {loading ? '提交中...' : '确认重置'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
