import { useState, useRef } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useAuth } from '../contexts/AuthContext'
import { useConfig } from '../contexts/ConfigContext'
import { resendVerification } from '../api/client'

// ─── Password Strength Indicator ─────────────────────────────────────────────
type Strength = 'none' | 'weak' | 'medium' | 'strong'

function calcStrength(pw: string): Strength {
  if (!pw) return 'none'
  if (pw.length < 8) return 'weak'
  if (/^\d+$/.test(pw)) return 'weak'
  let score = 0
  if (/[a-z]/.test(pw)) score++
  if (/[A-Z]/.test(pw)) score++
  if (/[0-9]/.test(pw)) score++
  if (/[^a-zA-Z0-9]/.test(pw)) score++
  if (score <= 1) return 'weak'
  if (score === 2) return 'medium'
  return 'strong'
}

function PasswordStrength({ password }: { password: string }) {
  const s = calcStrength(password)
  if (!password) return null
  const bar = { none: 'w-0', weak: 'w-1/3', medium: 'w-2/3', strong: 'w-full' }[s]
  const color = { none: '', weak: 'bg-red-400', medium: 'bg-amber-400', strong: 'bg-green-500' }[s]
  const label = { none: '', weak: '弱 — 建议混入大小写字母、数字和符号', medium: '中等 — 添加特殊字符可进一步增强', strong: '强' }[s]
  const tc = { none: '', weak: 'text-red-500', medium: 'text-amber-600', strong: 'text-green-600' }[s]
  return (
    <div className="mt-1.5 space-y-1">
      <div className="h-1 w-full bg-gray-100 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
}

export default function LoginPage() {
  const { login, register } = useAuth()
  const { turnstileSiteKey, loaded: configLoaded } = useConfig()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
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
  const accountDeleted = searchParams.get('deleted') === '1'
  const emailChanged = searchParams.get('email_changed') === '1'

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
        const pending = await register({ email, username, password, captcha_token: captchaToken || undefined })
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

  const inputClass = 'w-full border border-gray-200 rounded-xl px-3 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent transition bg-white'

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
          <img src="/logo.svg" alt="FinArch" className="w-14 h-14 rounded-2xl shadow-md mb-3" />
          <h1 className="text-xl font-bold text-gray-800">FinArch</h1>
          <p className="text-xs text-gray-400 mt-0.5">收支与报销管理 v2</p>
        </div>

        {justVerified && (
          <div className="mb-4 bg-green-50 border border-green-200 text-green-700 rounded-lg px-3 py-2 text-sm flex items-center gap-2">
            <span>✓</span><span>邮箱验证成功，请登录</span>
          </div>
        )}
        {accountDeleted && (
          <div className="mb-4 bg-slate-50 border border-slate-200 text-slate-700 rounded-xl px-3 py-2 text-sm flex items-center gap-2">
            <span>✓</span><span>账户已注销，感谢您使用 FinArch。</span>
          </div>
        )}
        {emailChanged && (
          <div className="mb-4 bg-green-50 border border-green-200 text-green-700 rounded-xl px-3 py-2 text-sm flex items-center gap-2">
            <span>✓</span><span>邮箱已更新，请使用新邮箱登录。</span>
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
              <label className="block text-sm font-medium text-gray-700 mb-1">用户名</label>
              <input type="text" required value={username} onChange={(e) => setUsername(e.target.value)}
                className={inputClass} placeholder="字母、数字或下划线，注册后不可更改"
                autoComplete="username" />
              <p className="mt-1 text-xs text-gray-400">注册后无法修改，请谨慎选择。</p>
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
              className={inputClass} placeholder={mode === 'register' ? '至少 8 位，建议大小写 + 数字 + 符号' : '请输入密码'} />
            {mode === 'register' && <PasswordStrength password={password} />}
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