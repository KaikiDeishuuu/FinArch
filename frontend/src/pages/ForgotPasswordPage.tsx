import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { forgotPassword } from '../api/client'

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await forgotPassword(email)
      setSent(true)
    } catch {
      setError('发送失败，请稍后重试')
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
          <h1 className="text-xl font-bold text-gray-800">重置密码</h1>
          <p className="text-xs text-gray-400 mt-0.5">FinArch v2</p>
        </div>

        {sent ? (
          <div className="text-center">
            <div className="w-12 h-12 rounded-full bg-emerald-100 flex items-center justify-center mx-auto mb-4">
              <svg viewBox="0 0 24 24" fill="none" stroke="#16a34a" strokeWidth={2} className="w-6 h-6">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <p className="text-gray-600 text-sm mb-1">如果该邮箱已注册，重置链接将在几分钟内发送。</p>
            <p className="text-gray-400 text-xs mb-6">请检查收件箱（含垃圾邮件），链接有效期 1 小时。</p>
            <Link to="/login" className="text-sm text-teal-600 hover:underline">返回登录</Link>
          </div>
        ) : (
          <>
            <p className="text-sm text-gray-500 mb-5 text-center">
              输入您的注册邮箱，我们将发送密码重置链接。
            </p>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">注册邮箱</label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-teal-500"
                  placeholder="user@example.com"
                  autoFocus
                />
              </div>
              {error && (
                <div className="bg-rose-50 border border-rose-200 text-rose-700 rounded-lg px-3 py-2 text-sm">{error}</div>
              )}
              <button
                type="submit"
                disabled={loading}
                className="w-full bg-teal-600 hover:bg-teal-700 disabled:opacity-50 text-white font-medium rounded-lg py-2 text-sm transition-colors"
              >
                {loading ? '发送中...' : '发送重置链接'}
              </button>
            </form>
            <div className="text-center mt-4">
              <Link to="/login" className="text-xs text-gray-400 hover:text-teal-600 transition-colors">
                返回登录
              </Link>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
