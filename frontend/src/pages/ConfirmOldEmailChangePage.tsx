import { useEffect, useState } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { confirmOldEmailForChange } from '../api/client'

export default function ConfirmOldEmailChangePage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    if (!token) {
      setStatus('error')
      setErrorMsg('无效的授权链接，请重新申请。')
      return
    }
    confirmOldEmailForChange(token)
      .then(() => setStatus('success'))
      .catch((err: unknown) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setErrorMsg(msg || '授权失败，链接可能已过期或已被使用，请重新申请。')
        setStatus('error')
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md text-center">
        {/* Logo */}
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-xl bg-blue-600 flex items-center justify-center shadow-md mb-3">
            <span className="text-white text-xl font-bold">¥</span>
          </div>
          <h1 className="text-xl font-bold text-gray-800">FinArch</h1>
        </div>

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="w-10 h-10 border-4 border-blue-200 border-t-blue-600 rounded-full animate-spin mx-auto" />
            <p className="text-gray-600 text-sm">正在验证授权请求...</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-full bg-blue-100 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="#2563eb" strokeWidth={1.5} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
              </svg>
            </div>
            <p className="text-gray-800 font-semibold text-lg">授权成功！</p>
            <p className="text-gray-500 text-sm leading-relaxed">
              已向您的<strong className="text-gray-700">新邮箱</strong>发送验证链接，请前往新邮箱收件箱，点击链接完成邮箱更换。
            </p>
            <p className="text-xs text-gray-400">新邮箱验证完成前，您仍可使用当前邮箱登录。</p>
            <Link
              to="/login"
              className="inline-block mt-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-6 py-2 rounded-xl transition-colors"
            >
              返回登录
            </Link>
          </div>
        )}

        {status === 'error' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-full bg-red-100 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="#dc2626" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </div>
            <p className="text-gray-800 font-semibold text-lg">授权失败</p>
            <p className="text-gray-500 text-sm">{errorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="inline-block bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-6 py-2 rounded-xl transition-colors"
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
