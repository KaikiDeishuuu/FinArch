import { useState, useRef } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useAuth } from '../contexts/AuthContext'
import { useConfig } from '../contexts/ConfigContext'
import { resendVerification } from '../api/client'

export default function LoginPage() {
  const { login, register } = useAuth()
  const { turnstileSiteKey, loaded: configLoaded } = useConfig()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [captchaToken, setCaptchaToken] = useState<string>('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [pendingVerification, setPendingVerification] = useState(false)
  const [unverifiedEmail, setUnverifiedEmail] = useState('')
  const [resendLoading, setResendLoading] = useState(false)
  const [resendDone, setResendDone] = useState(false)
  const turnstileRef = useRef<TurnstileInstance>(null)

  const justVerified = searchParams.get('verified') === '1'
  const tokenError = searchParams.get('error') === 'invalid_token'

  function switchMode(next: 'login' | 'register') {
    setMode(next)
    setError('')
    setPendingVerification(false)
    setUnverifiedEmail('')
    setCaptchaToken('')
    turnstileRef.current?.reset()
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setPendingVerification(false)
    if (turnstileSiteKey && !captchaToken) {
      setError('请先完成人机验证')
      return
    }
    setLoading(true)
    try {
      if (mode === 'login') {
        await login({ email, password, captcha_token: captchaToken || undefined })
        navigate('/')
      } else {
        const pending = await register({ email, name, password, captcha_token: captchaToken || undefined })
        if (pending) {
          setPendingVerification(true)
        } else {
          navigate('/')
        }
      }
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
      if (msg.includes('邮箱尚未验证') || msg.includes('email_not_verified')) {
        setUnverifiedEmail(email)
      } else {
        setError(msg || '操作失败，请重试')
      }
      setCaptchaToken('')
      turnstileRef.current?.reset()
    } finally {
      setLoading(false)
    }
  }

  async function handleResend() {
    setResendLoading(true)
    try {
      await resendVerification(unverifiedEmail || email)
      setResendDone(true)
    } finally {
      setResendLoading(false)
    }
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500'

  if (pendingVerification) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
        <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md text-center">
          <div className="w-14 h-14 rounded-full bg-blue-100 flex items-center justify-center mx-auto mb-4">
            <svg viewBox="0 0 24 24" fill="none" stroke="#2563eb" strokeWidth={1.5} className="w-7 h-7">
              <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
            </svg>
          </div>
          <h2 className="text-xl font-bold text-gray-800 mb-2">请验证您的邮箱</h2>
          <p className="text-gray-500 text-sm mb-6">
            验证邮件已发送至 <span className="font-medium text-gray-700">{email}</span>，
            请点击邮件中的链接完成验证后再登录。
          </p>
          {!resendDone ? (
            <button onClick={handleResend} disabled={resendLoading}
              className="text-sm text-blue-600 hover:underline disabled:opacity-50">
              {resendLoading ? '发送中...' : '没收到邮件？重新发送'}
            </button>
          ) : (
            <p className="text-sm text-green-600">验证邮件已重新发送</p>
          )}
          <button onClick={() => switchMode('login')}
            className="mt-4 block w-full text-center text-sm text-gray-400 hover:text-gray-600">
            返回登录
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md">
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-xl bg-blue-600 flex items-center justify-center shadow-md mb-3">
            <span className="text-white text-xl font-bold">¥</span>
          </div>
          <h1 className="text-xl font-bold text-gray-800">FinArch</h1>
          <p className="text-xs text-gray-400 mt-0.5">收支与报销管理 v2</p>
        </div>

        {justVerified && (
          <div className="mb-4 bg-green-50 border border-green-200 text-green-700 rounded-lg px-3 py-2 text-sm flex items-center gap-2">
            <span>✓</span><span>邮箱验证成功，请登录</span>
          </div>
        )}
        {tokenError && (
          <div className="mb-4 bg-red-50 border border-red-200 text-red-700 rounded-lg px-3 py-2 text-sm">
            验证链接无效或已过期，请重新发送验证邮件
          </div>
        )}

        <div className="flex rounded-lg bg-gray-100 p-1 mb-6">
          <button className={`flex-1 rounded-md py-2 text-sm font-medium transition-all ${mode === 'login' ? 'bg-white shadow text-blue-600' : 'text-gray-500'}`}
            onClick={() => switchMode('login')}>登录</button>
          <button className={`flex-1 rounded-md py-2 text-sm font-medium transition-all ${mode === 'register' ? 'bg-white shadow text-blue-600' : 'text-gray-500'}`}
            onClick={() => switchMode('register')}>注册</button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === 'register' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">姓名</label>
              <input type="text" required value={name} onChange={(e) => setName(e.target.value)}
                className={inputClass} placeholder="请输入姓名" />
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">邮箱</label>
            <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)}
              className={inputClass} placeholder="user@example.com" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">密码</label>
            <input type="password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)}
              className={inputClass} placeholder={mode === 'register' ? '至少 8 位' : '请输入密码'} />
          </div>

          {turnstileSiteKey && configLoaded && (
            <div className="flex justify-center">
              <Turnstile ref={turnstileRef} siteKey={turnstileSiteKey}
                onSuccess={(token) => setCaptchaToken(token)}
                onExpire={() => setCaptchaToken('')}
                onError={() => { setCaptchaToken(''); setError('人机验证加载失败，请刷新页面重试') }}
                options={{ theme: 'light', language: 'zh-cn' }} />
            </div>
          )}

          {error && (
            <div className="bg-red-50 border border-red-200 text-red-700 rounded-lg px-3 py-2 text-sm">{error}</div>
          )}

          {unverifiedEmail && (
            <div className="bg-amber-50 border border-amber-200 text-amber-800 rounded-lg px-3 py-2 text-sm space-y-1">
              <p>邮箱尚未验证，请检查收件箱并点击验证链接。</p>
              {!resendDone ? (
                <button type="button" onClick={handleResend} disabled={resendLoading}
                  className="text-blue-600 hover:underline text-xs disabled:opacity-50">
                  {resendLoading ? '发送中...' : '重新发送验证邮件'}
                </button>
              ) : (
                <p className="text-xs text-green-600">验证邮件已重新发送</p>
              )}
            </div>
          )}

          <button type="submit" disabled={loading || (!!turnstileSiteKey && !captchaToken)}
            className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-medium rounded-lg py-2 text-sm transition-colors">
            {loading ? '处理中...' : mode === 'login' ? '登录' : '注册'}
          </button>
        </form>

        {mode === 'login' && (
          <div className="text-center mt-4">
            <Link to="/forgot-password" className="text-xs text-gray-400 hover:text-blue-600 transition-colors">
              忘记密码？
            </Link>
          </div>
        )}
      </div>
    </div>
  )
}