import { useState, useRef } from 'react'
import { toast } from 'sonner'
import { Link } from 'react-router-dom'
import { disasterRestoreRequest, disasterRestoreConfirm } from '../api/client'

type Step = 'upload' | 'verify' | 'done'

export default function DisasterRestorePage() {
  const [step, setStep] = useState<Step>('upload')
  const [file, setFile] = useState<File | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [restoreId, setRestoreId] = useState('')
  const [maskedEmail, setMaskedEmail] = useState('')
  const [code, setCode] = useState('')
  const [result, setResult] = useState<{ message: string; restored_version: number; migrated_to: number } | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  async function handleUpload() {
    if (!file) return
    setLoading(true)
    setError('')
    try {
      const resp = await disasterRestoreRequest(file)
      setRestoreId(resp.restore_id)
      setMaskedEmail(resp.masked_email)
      setStep('verify')
      toast.success('验证码已发送至数据持有人邮箱')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '上传失败，请检查文件是否为有效的 FinArch 备份')
    } finally {
      setLoading(false)
    }
  }

  async function handleVerify() {
    if (!code || code.length !== 6) {
      setError('请输入 6 位验证码')
      return
    }
    setLoading(true)
    setError('')
    try {
      const res = await disasterRestoreConfirm(restoreId, code)
      setResult(res)
      setStep('done')
      toast.success('数据恢复成功！')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '验证失败，请重试')
    } finally {
      setLoading(false)
    }
  }

  function formatFileSize(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / 1048576).toFixed(1) + ' MB'
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-md">
        {/* Header */}
        <div className="text-center mb-8">
          {/* Inline logo */}
          <div className="flex items-end justify-center gap-1.5 mb-4">
            <div className="w-2.5 h-4 bg-indigo-400 rounded-t-sm" />
            <div className="w-2.5 h-6 bg-violet-400 rounded-t-sm" />
            <div className="w-2.5 h-9 bg-emerald-400 rounded-t-sm" />
          </div>
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">灾难恢复</h1>
          <p className="text-sm text-gray-500 mt-1">
            在数据完全丢失时，通过备份文件和邮箱验证恢复数据
          </p>
        </div>

        {/* Steps indicator */}
        <div className="flex items-center justify-center gap-3 mb-8">
          {(['upload', 'verify', 'done'] as const).map((s, i) => (
            <div key={s} className="flex items-center gap-3">
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-bold transition-colors ${
                step === s ? 'bg-violet-600 text-white' :
                (['upload', 'verify', 'done'].indexOf(step) > i) ? 'bg-emerald-500 text-white' :
                'bg-gray-200 text-gray-400'
              }`}>
                {(['upload', 'verify', 'done'].indexOf(step) > i) ? (
                  <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                  </svg>
                ) : i + 1}
              </div>
              {i < 2 && (
                <div className={`w-12 h-0.5 ${
                  (['upload', 'verify', 'done'].indexOf(step) > i) ? 'bg-emerald-400' : 'bg-gray-200'
                }`} />
              )}
            </div>
          ))}
        </div>

        {/* Card */}
        <div className="bg-white rounded-2xl border border-gray-100/80 shadow-sm overflow-hidden">

          {/* Step 1: Upload */}
          {step === 'upload' && (
            <div className="p-6 space-y-5">
              <div>
                <h2 className="text-lg font-semibold text-gray-800">上传备份文件</h2>
                <p className="text-sm text-gray-500 mt-1">
                  选择 FinArch 备份数据库文件 (.db)，系统将提取数据持有人邮箱并发送验证码
                </p>
              </div>

              {/* File drop zone */}
              <div
                onClick={() => fileRef.current?.click()}
                className={`border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-colors ${
                  file ? 'border-violet-300 bg-violet-50' : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50'
                }`}
              >
                <input
                  ref={fileRef}
                  type="file"
                  accept=".db"
                  className="hidden"
                  onChange={(e) => {
                    const f = e.target.files?.[0]
                    if (f) { setFile(f); setError('') }
                  }}
                />
                {file ? (
                  <div className="space-y-2">
                    <div className="w-12 h-12 mx-auto bg-violet-100 rounded-xl flex items-center justify-center">
                      <svg className="w-6 h-6 text-violet-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m8.25 3v6.75m0 0l-3-3m3 3l3-3M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
                      </svg>
                    </div>
                    <p className="text-sm font-medium text-gray-700">{file.name}</p>
                    <p className="text-xs text-gray-400">{formatFileSize(file.size)}</p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="w-12 h-12 mx-auto bg-gray-100 rounded-xl flex items-center justify-center">
                      <svg className="w-6 h-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
                      </svg>
                    </div>
                    <p className="text-sm text-gray-500">点击选择或拖放 .db 文件</p>
                    <p className="text-xs text-gray-400">最大 100 MB</p>
                  </div>
                )}
              </div>

              {error && (
                <div className="bg-rose-50 border border-rose-200 text-rose-700 rounded-xl px-4 py-3 text-sm flex items-start gap-2">
                  <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                  </svg>
                  {error}
                </div>
              )}

              <button
                onClick={handleUpload}
                disabled={!file || loading}
                className="w-full bg-violet-600 hover:bg-violet-700 disabled:opacity-50 text-white font-semibold rounded-xl py-2.5 text-sm transition-all flex items-center justify-center gap-2"
              >
                {loading ? (
                  <>
                    <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    上传验证中…
                  </>
                ) : '上传并发送验证码'}
              </button>
            </div>
          )}

          {/* Step 2: Verify code */}
          {step === 'verify' && (
            <div className="p-6 space-y-5">
              <div>
                <h2 className="text-lg font-semibold text-gray-800">邮箱验证</h2>
                <p className="text-sm text-gray-500 mt-1">
                  验证码已发送至 <span className="font-medium text-violet-600">{maskedEmail}</span>
                </p>
                <p className="text-xs text-gray-400 mt-1">验证码有效期 10 分钟</p>
              </div>

              {/* Code input */}
              <div>
                <label className="block text-xs font-semibold text-gray-500 mb-2 uppercase tracking-wider">
                  6 位验证码
                </label>
                <input
                  type="text"
                  inputMode="numeric"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  className="w-full border border-gray-200 rounded-xl px-4 py-3 text-center text-2xl font-bold tracking-[0.4em] tabular-nums focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent bg-gray-50 transition-all hover:bg-white"
                  placeholder="000000"
                  autoFocus
                />
              </div>

              {error && (
                <div className="bg-rose-50 border border-rose-200 text-rose-700 rounded-xl px-4 py-3 text-sm flex items-start gap-2">
                  <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
                    <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                  </svg>
                  {error}
                </div>
              )}

              <div className="flex gap-3">
                <button
                  onClick={() => { setStep('upload'); setCode(''); setError(''); setFile(null) }}
                  className="flex-1 bg-gray-100 hover:bg-gray-200 text-gray-600 font-medium rounded-xl py-2.5 text-sm transition-all"
                >
                  重新上传
                </button>
                <button
                  onClick={handleVerify}
                  disabled={code.length !== 6 || loading}
                  className="flex-1 bg-violet-600 hover:bg-violet-700 disabled:opacity-50 text-white font-semibold rounded-xl py-2.5 text-sm transition-all flex items-center justify-center gap-2"
                >
                  {loading ? (
                    <>
                      <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                      恢复中…
                    </>
                  ) : '验证并恢复'}
                </button>
              </div>
            </div>
          )}

          {/* Step 3: Done */}
          {step === 'done' && result && (
            <div className="p-6 text-center space-y-5">
              <div className="w-16 h-16 mx-auto bg-emerald-100 rounded-2xl flex items-center justify-center">
                <svg className="w-8 h-8 text-emerald-600" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              </div>
              <div>
                <h2 className="text-lg font-semibold text-gray-800">恢复成功！</h2>
                <p className="text-sm text-gray-500 mt-1">{result.message}</p>
              </div>
              <div className="bg-gray-50 rounded-xl px-4 py-3 space-y-1.5 text-sm">
                <div className="flex justify-between">
                  <span className="text-gray-500">恢复版本</span>
                  <span className="font-semibold text-gray-700 tabular-nums">v{result.restored_version}</span>
                </div>
                {result.migrated_to > result.restored_version && (
                  <div className="flex justify-between">
                    <span className="text-gray-500">已迁移至</span>
                    <span className="font-semibold text-emerald-600 tabular-nums">v{result.migrated_to}</span>
                  </div>
                )}
              </div>
              <Link
                to="/login"
                className="block w-full bg-violet-600 hover:bg-violet-700 text-white font-semibold rounded-xl py-2.5 text-sm transition-all text-center"
              >
                前往登录
              </Link>
            </div>
          )}
        </div>

        {/* Footer link */}
        <div className="text-center mt-6">
          <Link to="/login" className="text-sm text-gray-400 hover:text-violet-600 transition-colors">
            ← 返回登录页
          </Link>
        </div>
      </div>
    </div>
  )
}
