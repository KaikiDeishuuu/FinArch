import { useState, useRef } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useAuth } from '../contexts/AuthContext'
import { useConfig } from '../contexts/ConfigContext'

export default function LoginPage() {
  const { login, register } = useAuth()
  const { turnstileSiteKey, loaded: configLoaded } = useConfig()
  const navigate = useNavigate()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [captchaToken, setCaptchaToken] = useState<string>('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const turnstileRef = useRef<TurnstileInstance>(null)

  // Reset captcha widget when switching auth mode
  function switchMode(next: 'login' | 'register') {
    setMode(next)
    setError('')
    setCaptchaToken('')
    turnstileRef.current?.reset()
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (turnstileSiteKey && !captchaToken) {
      setError('请先完成人机验证')
      return
    }
    setLoading(true)
    try {
      if (mode === 'login') {
        await login({ email, password, captcha_token: captchaToken || undefined })
      } else {
        await register({ email, name, password, captcha_token: captchaToken || undefined })
      }
      navigate('/')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '操作失败，请重试')
      // Reset captcha on failure so the user must re-verify
      setCaptchaToken('')
      turnstileRef.current?.reset()
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-2xl shadow-xl p-8 w-full max-w-md">
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-xl bg-blue-600 flex items-center justify-center shadow-md mb-3">
            <span className="text-white text-xl font-bold">¥</span>
          </div>
          <h1 className="text-xl font-bold text-gray-800">科研经费管理系统</h1>
          <p className="text-xs text-gray-400 mt-0.5">FinArch v2</p>
        </div>

        <div className="flex rounded-lg bg-gray-100 p-1 mb-6">
          <button
            className={`flex-1 rounded-md py-2 text-sm font-medium transition-all ${mode === 'login' ? 'bg-white shadow text-blue-600' : 'text-gray-500'}`}
            onClick={() => switchMode('login')}
          >登录</button>
          <button
            className={`flex-1 rounded-md py-2 text-sm font-medium transition-all ${mode === 'register' ? 'bg-white shadow text-blue-600' : 'text-gray-500'}`}
            onClick={() => switchMode('register')}
          >注册</button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === 'register' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">姓名</label>
              <input
                type="text"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="请输入姓名"
              />
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">邮箱</label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="user@example.com"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">密码</label>
            <input
              type="password"
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder={mode === 'register' ? '至少 8 位' : '请输入密码'}
            />
          </div>

          {/* Cloudflare Turnstile – only rendered when server reports a site key */}
          {turnstileSiteKey && configLoaded && (
            <div className="flex justify-center">
              <Turnstile
                ref={turnstileRef}
                siteKey={turnstileSiteKey}
                onSuccess={(token) => setCaptchaToken(token)}
                onExpire={() => setCaptchaToken('')}
                onError={() => {
                  setCaptchaToken('')
                  setError('人机验证加载失败，请刷新页面重试')
                }}
                options={{ theme: 'light', language: 'zh-cn' }}
              />
            </div>
          )}

          {error && (
            <div className="bg-red-50 border border-red-200 text-red-700 rounded-lg px-3 py-2 text-sm">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading || (!!turnstileSiteKey && !captchaToken)}
            className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-medium rounded-lg py-2 text-sm transition-colors"
          >
            {loading ? '处理中...' : mode === 'login' ? '登录' : '注册'}
          </button>
        </form>

        <p className="text-xs text-gray-400 text-center mt-6">
          演示账号：admin@example.com / password123
        </p>
      </div>
    </div>
  )
}
