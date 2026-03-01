import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { forgotPassword } from '../api/client'
import { useThemeColor } from '../hooks/useThemeColor'
import { LogoMark } from '../components/Brand'

export default function ForgotPasswordPage() {
  useThemeColor('#7c3aed')
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
    <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
      <div className="flex-[1]" />
      <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-8 relative z-10">
        <div className="flex flex-col items-center mb-6">
          <LogoMark size={48} className="rounded-2xl shadow-lg shadow-violet-500/20 mb-3" />
          <h1 className="text-xl font-bold text-gray-800">重置密码</h1>
          <p className="text-xs text-gray-400 mt-0.5">收支 · 报销 · 智能匹配</p>
        </div>

        {sent ? (
          <div className="text-center">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-emerald-400 to-emerald-500 flex items-center justify-center mx-auto mb-5 shadow-lg shadow-emerald-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 mb-2">链接已发送</h2>
            <p className="text-gray-500 text-sm mb-1 leading-relaxed">如果该邮箱已注册，重置链接将在几分钟内发送。</p>
            <p className="text-gray-400 text-xs mb-6">请检查收件箱（含垃圾邮件），链接有效期 1 小时。</p>
            <Link to="/login" className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]">
              返回登录
            </Link>
          </div>
        ) : (
          <>
            <p className="text-sm text-gray-500 mb-5 text-center">
              输入您的注册邮箱，我们将发送密码重置链接。
            </p>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1.5">注册邮箱</label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full border border-gray-200/80 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500/40 focus:border-violet-300 transition-all bg-white/80 backdrop-blur-sm placeholder:text-gray-300"
                  placeholder="user@example.com"
                  autoFocus
                />
              </div>
              {error && (
                <div className="bg-rose-50 border border-rose-200 text-rose-700 rounded-xl px-4 py-3 text-sm">{error}</div>
              )}
              <button
                type="submit"
                disabled={loading}
                className="w-full bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 disabled:opacity-50 text-white font-semibold rounded-xl py-3 text-sm transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
              >
                {loading ? '发送中...' : '发送重置链接'}
              </button>
            </form>
            <div className="text-center mt-5">
              <Link to="/login" className="text-xs text-gray-400 hover:text-violet-600 transition-colors font-medium">
                返回登录
              </Link>
            </div>
          </>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
