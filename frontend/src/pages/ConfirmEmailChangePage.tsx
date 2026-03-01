import { useEffect, useState } from 'react'
import { useSearchParams, Link, useNavigate } from 'react-router-dom'
import { confirmEmailChange } from '../api/client'

export default function ConfirmEmailChangePage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const navigate = useNavigate()

  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    if (!token) {
      setStatus('error')
      setErrorMsg('无效的邮箱验证链接，请重新申请。')
      return
    }
    confirmEmailChange(token)
      .then(() => {
        setStatus('success')
        // Redirect to login so user re-authenticates with new email
        setTimeout(() => navigate('/login?email_changed=1', { replace: true }), 3000)
      })
      .catch((err: unknown) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setErrorMsg(msg || '验证失败，链接可能已过期或已被使用，请重新申请。')
        setStatus('error')
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md text-center">
        {/* Logo */}
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-xl bg-teal-600 flex items-center justify-center shadow-md mb-3">
            <span className="text-white text-xl font-bold">¥</span>
          </div>
          <h1 className="text-xl font-bold text-gray-800">FinArch</h1>
        </div>

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="w-10 h-10 border-4 border-teal-200 border-t-blue-600 rounded-full animate-spin mx-auto" />
            <p className="text-gray-600 text-sm">正在验证邮箱变更请求...</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-full bg-emerald-100 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="#16a34a" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
            </div>
            <p className="text-gray-800 font-semibold text-lg">邮箱变更成功！</p>
            <p className="text-gray-500 text-sm">
              您的登录邮箱已成功更新，请使用新邮箱重新登录。
            </p>
            <p className="text-gray-400 text-xs">3 秒后自动跳转至登录页...</p>
            <Link
              to="/login?email_changed=1"
              className="inline-block bg-teal-600 hover:bg-teal-700 text-white text-sm font-medium px-6 py-2 rounded-xl transition-colors"
            >
              立即前往登录
            </Link>
          </div>
        )}

        {status === 'error' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-full bg-rose-100 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="#dc2626" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </div>
            <p className="text-gray-800 font-semibold text-lg">验证失败</p>
            <p className="text-gray-500 text-sm">{errorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="inline-block bg-teal-600 hover:bg-teal-700 text-white text-sm font-medium px-6 py-2 rounded-xl transition-colors"
              >
                返回设置页重新申请
              </Link>
              <Link to="/login" className="text-sm text-gray-400 hover:text-gray-600 transition-colors">
                返回登录
              </Link>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
