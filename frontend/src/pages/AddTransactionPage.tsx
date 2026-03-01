import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { createTransaction } from '../api/client'
import { useInvalidateTransactions } from '../hooks/useTransactions'
import { useHaptic } from '../hooks/useHaptic'

const CATEGORIES = [
  '耗材', '材料', '设备', '仪器', 'CNC加工', '加工费',
  '差旅', '劳务', '软件', '培训', '会议', '测试', '其他',
]


export default function AddTransactionPage() {
  const navigate = useNavigate()
  const invalidate = useInvalidateTransactions()
  const haptic = useHaptic()
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  const [customCat, setCustomCat] = useState('')

  const [form, setForm] = useState({
    occurred_at: (() => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` })(),
    direction: 'expense',
    source: 'personal',
    category: CATEGORIES[0],
    amount_yuan: '',
    currency: 'CNY',
    note: '',
    project_id: '',
  })

  function set(key: string, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    const amount = parseFloat(form.amount_yuan)
    if (isNaN(amount) || amount <= 0) {
      haptic.error()
      setError('请输入有效金额')
      return
    }
    setLoading(true)
    try {
      await createTransaction({
        ...form,
        project_id: form.project_id.trim() || undefined,
        amount_yuan: amount,
      })
      haptic.success()
      invalidate()
      toast.success('交易已添加', { description: `¥${amount.toFixed(2)}` })
      setSuccess(true)
      setTimeout(() => navigate('/transactions'), 1200)
    } catch (err: unknown) {
      haptic.error()
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '添加失败，请重试')
      toast.error(msg || '添加失败，请重试')
    } finally {
      setLoading(false)
    }
  }

  const inputClass = 'w-full border border-gray-200 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-teal-500 focus:border-transparent bg-gray-50 transition-all hover:bg-white'
  const labelClass = 'block text-xs font-semibold text-gray-500 mb-2 uppercase tracking-wider'

  const isExpense = form.direction === 'expense'
  const isPersonal = form.source === 'personal'

  return (
    <div className="max-w-3xl pb-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900 tracking-tight">添加交易记录</h1>
        <p className="text-sm text-gray-400 mt-1">记录一笔新的收入或支出</p>
      </div>

      {success && (
        <div className="mb-4 bg-emerald-50 border border-green-200 text-emerald-700 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
          <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
          添加成功，即将跳转…
        </div>
      )}

      <form onSubmit={handleSubmit} className="grid grid-cols-1 md:grid-cols-2 gap-4">

        {/* 收支方向 + 资金来源 ── col 1 */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5 space-y-4">
          <div>
            <label className={labelClass}>收支方向</label>
            <div className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => set('direction', 'expense')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  isExpense
                    ? 'bg-rose-500 border-red-500 text-white shadow-sm'
                    : 'bg-white border-gray-200 text-gray-400 hover:border-red-300 hover:text-red-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M17 13l-5 5m0 0l-5-5m5 5V6" /></svg>
                支出
              </button>
              <button type="button"
                onClick={() => set('direction', 'income')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  !isExpense
                    ? 'bg-emerald-500 border-green-500 text-white shadow-sm'
                    : 'bg-white border-gray-200 text-gray-400 hover:border-green-300 hover:text-green-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M7 11l5-5m0 0l5 5m-5-5v12" /></svg>
                收入
              </button>
            </div>
          </div>

          <div>
            <label className={labelClass}>资金来源</label>
            <div className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => set('source', 'personal')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  isPersonal
                    ? 'bg-amber-500 border-amber-500 text-white shadow-sm'
                    : 'bg-white border-gray-200 text-gray-400 hover:border-amber-300 hover:text-amber-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                个人垫付
              </button>
              <button type="button"
                onClick={() => set('source', 'company')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  !isPersonal
                    ? 'bg-teal-500 border-teal-500 text-white shadow-sm'
                    : 'bg-white border-gray-200 text-gray-400 hover:border-blue-300 hover:text-teal-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
                公司账户
              </button>
            </div>
          </div>
        </div>

        {/* 金额 ── col 2 */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5 flex flex-col justify-between">
          <label className={labelClass}>金额（元）</label>
          <div className={`flex items-center gap-2 rounded-xl border-2 px-3 py-1 transition-all ${isExpense ? 'border-red-200 focus-within:border-red-400' : 'border-green-200 focus-within:border-green-400'}`}>
            <span className={`text-xl font-bold select-none whitespace-nowrap shrink-0 ${isExpense ? 'text-red-400' : 'text-green-400'}`}>
              {isExpense ? '−' : '+'}¥
            </span>
            <input
              type="number"
              required
              min="0.01"
              step="0.01"
              className="flex-1 min-w-0 text-xl font-bold text-gray-800 bg-transparent py-2 focus:outline-none placeholder:text-gray-200"
              placeholder="0.00"
              value={form.amount_yuan}
              onChange={(e) => set('amount_yuan', e.target.value)}
            />
            <div className="relative shrink-0">
              <select
                className="appearance-none text-xs font-medium text-gray-500 bg-gray-100 rounded-lg px-2 py-1.5 pr-6 focus:outline-none cursor-pointer"
                value={form.currency}
                onChange={(e) => set('currency', e.target.value)}
              >
                <option value="CNY">CNY</option>
                <option value="USD">USD</option>
                <option value="EUR">EUR</option>
              </select>
              <svg className="w-3 h-3 text-gray-400 absolute right-1 top-1/2 -translate-y-1/2 pointer-events-none" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
            </div>
          </div>
          <p className="text-xs text-gray-400 mt-2">
            {isExpense ? '本笔支出将从资金池中扣除' : '本笔收入将计入资金池'}
          </p>
        </div>

        {/* 费用类别 ── full width */}
        <div className="md:col-span-2 bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
          <label className={labelClass}>费用类别</label>
          <div className="grid grid-cols-4 sm:grid-cols-7 gap-2">
            {CATEGORIES.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => { set('category', c); setCustomCat('') }}
                className={`flex items-center justify-center py-2.5 px-1 rounded-xl text-xs font-semibold border-2 transition-all ${
                  form.category === c
                    ? 'bg-teal-600 border-blue-600 text-white shadow-sm'
                    : 'bg-white border-gray-200 text-gray-600 hover:border-blue-300 hover:bg-teal-50 hover:text-teal-700'
                }`}
              >
                <span className="leading-tight text-center">{c}</span>
              </button>
            ))}
          </div>
          {/* 自定义类别 */}
          <div className="mt-3 flex items-center gap-2">
            <span className="text-xs text-gray-400 shrink-0">自定义</span>
            <input
              type="text"
              value={customCat}
              onChange={e => {
                const v = e.target.value
                setCustomCat(v)
                set('category', v.trim() !== '' ? v.trim() : CATEGORIES[0])
              }}
              placeholder="输入自定义类别名称…"
              className={`flex-1 text-xs rounded-xl border-2 py-2 px-3 outline-none transition-all placeholder-gray-300 ${
                !CATEGORIES.includes(form.category) && customCat.trim() !== ''
                  ? 'border-teal-500 bg-teal-50 text-teal-700 font-semibold'
                  : 'border-gray-200 bg-white text-gray-600 focus:border-blue-300 focus:bg-teal-50'
              }`}
            />
          </div>
        </div>

        {/* 日期 ── col 1 */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
          <label className={labelClass}>日期</label>
          <input type="date" required className={inputClass} value={form.occurred_at} onChange={(e) => set('occurred_at', e.target.value)} />
        </div>

        {/* 项目编号 + 备注 ── col 2 */}
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5 space-y-4">
          <div>
            <label className={labelClass}>
              项目编号 <span className="text-gray-300 font-normal normal-case tracking-normal">选填</span>
            </label>
            <input
              type="text"
              className={inputClass}
              placeholder="如：PJT-001"
              value={form.project_id}
              onChange={(e) => set('project_id', e.target.value)}
            />
          </div>
          <div>
            <label className={labelClass}>
              备注 <span className="text-gray-300 font-normal normal-case tracking-normal">选填</span>
            </label>
            <input
              type="text"
              className={inputClass}
              placeholder="可输入说明或描述"
              value={form.note}
              onChange={(e) => set('note', e.target.value)}
            />
          </div>
        </div>

        {/* 错误提示 + 操作按钮 ── full width */}
        <div className="md:col-span-2 space-y-3">
          {error && (
            <div className="bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-3 text-sm flex items-start gap-2">
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" /></svg>
              {error}
            </div>
          )}
          <div className="flex gap-3">
            <button
              type="submit"
              disabled={loading || success}
              className={`flex-1 font-semibold rounded-xl py-3 text-sm transition-all disabled:opacity-50 shadow-sm ${
                isExpense
                  ? 'bg-rose-500 hover:bg-red-600 text-white'
                  : 'bg-emerald-500 hover:bg-green-600 text-white'
              }`}
            >
              {loading ? '提交中…' : `保存${isExpense ? '支出' : '收入'}`}
            </button>
            <button
              type="button"
              onClick={() => navigate(-1)}
              className="px-5 py-3 rounded-xl border-2 border-gray-200 text-sm text-gray-500 hover:bg-gray-50 hover:border-gray-300 font-medium transition-all"
            >
              取消
            </button>
          </div>
        </div>

      </form>
    </div>
  )
}
