/**
 * DatePicker — Apple-style date picker with calendar popup
 * ─────────────────────────────────────────────────────────────────────────────
 * Custom-styled date input matching the premium UI system.
 * On mobile, delegates to native date picker for best UX.
 * On desktop, shows a custom calendar grid with month navigation.
 *
 * Features:
 * - Custom calendar dropdown (desktop)
 * - Native date input fallback (mobile)
 * - Framer Motion entrance animation
 * - Keyboard accessible
 * - Portal-rendered dropdown
 */
import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { motion, AnimatePresence } from 'framer-motion'
import { useTranslation } from 'react-i18next'

interface DatePickerProps {
  value: string        // 'YYYY-MM-DD'
  onChange: (val: string) => void
  className?: string
  required?: boolean
  placeholder?: string
}

const WEEKDAYS_ZH = ['一', '二', '三', '四', '五', '六', '日']
const MONTH_NAMES_ZH = ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月']

function pad2(n: number) { return String(n).padStart(2, '0') }

function toDateStr(y: number, m: number, d: number) {
  return `${y}-${pad2(m + 1)}-${pad2(d)}`
}

function parseDate(s: string) {
  const [y, m, d] = s.split('-').map(Number)
  return { year: y, month: m - 1, day: d }
}

function daysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate()
}

function firstDayOfWeek(year: number, month: number) {
  // Monday = 0, Sunday = 6
  const d = new Date(year, month, 1).getDay()
  return d === 0 ? 6 : d - 1
}

function isToday(y: number, m: number, d: number) {
  const t = new Date()
  return t.getFullYear() === y && t.getMonth() === m && t.getDate() === d
}

export default function DatePicker({ value, onChange, className = '', required, placeholder }: DatePickerProps) {
  const { t } = useTranslation()
  const WEEKDAYS = (t('datePicker.weekdays', { returnObjects: true }) as string[]) || WEEKDAYS_ZH
  const MONTH_NAMES = (t('datePicker.months', { returnObjects: true }) as string[]) || MONTH_NAMES_ZH
  const weekdaysFull = (t('datePicker.weekdaysFull', { returnObjects: true }) as string[]) || ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

  function formatDisplayI18n(val: string) {
    if (!val) return ''
    const { year, month, day } = parseDate(val)
    const date = new Date(year, month, day)
    const weekday = weekdaysFull[date.getDay()]
    return t('datePicker.displayFormat', { year, month: MONTH_NAMES[month], day, weekday })
  }

  const resolvedPlaceholder = placeholder ?? t('datePicker.placeholder')
  const [open, setOpen] = useState(false)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const nativeRef = useRef<HTMLInputElement>(null)

  // Calendar view state
  const parsed = value ? parseDate(value) : null
  const [viewYear, setViewYear] = useState(parsed?.year ?? new Date().getFullYear())
  const [viewMonth, setViewMonth] = useState(parsed?.month ?? new Date().getMonth())

  // Position state
  const [pos, setPos] = useState({ top: 0, left: 0, width: 0 })

  // Is mobile (use native picker)
  const isMobile = useMemo(() => {
    if (typeof window === 'undefined') return false
    return window.matchMedia('(max-width: 767px)').matches || 'ontouchstart' in window
  }, [])

  const updatePosition = useCallback(() => {
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    const dropH = 340
    const spaceBelow = window.innerHeight - rect.bottom
    const above = spaceBelow < dropH && rect.top > dropH
    setPos({
      top: above ? rect.top - dropH - 4 + window.scrollY : rect.bottom + 4 + window.scrollY,
      left: rect.left + window.scrollX,
      width: Math.max(rect.width, 280),
    })
  }, [])

  // Sync view to value
  useEffect(() => {
    if (value) {
      const p = parseDate(value)
      setViewYear(p.year)
      setViewMonth(p.month)
    }
  }, [value])

  useEffect(() => {
    if (!open) return
    updatePosition()
    const onScroll = () => updatePosition()
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onScroll)
    }
  }, [open, updatePosition])

  // Click outside
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (
        triggerRef.current?.contains(e.target as Node) ||
        dropdownRef.current?.contains(e.target as Node)
      ) return
      setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  // Keyboard
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setOpen(false); triggerRef.current?.focus() }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open])

  function prevMonth() {
    if (viewMonth === 0) { setViewYear(y => y - 1); setViewMonth(11) }
    else setViewMonth(m => m - 1)
  }
  function nextMonth() {
    if (viewMonth === 11) { setViewYear(y => y + 1); setViewMonth(0) }
    else setViewMonth(m => m + 1)
  }

  function selectDate(day: number) {
    onChange(toDateStr(viewYear, viewMonth, day))
    setOpen(false)
    triggerRef.current?.focus()
  }

  function goToday() {
    const t = new Date()
    setViewYear(t.getFullYear())
    setViewMonth(t.getMonth())
    onChange(toDateStr(t.getFullYear(), t.getMonth(), t.getDate()))
    setOpen(false)
  }

  // Build calendar grid
  const totalDays = daysInMonth(viewYear, viewMonth)
  const startDay = firstDayOfWeek(viewYear, viewMonth)
  const cells: (number | null)[] = []
  for (let i = 0; i < startDay; i++) cells.push(null)
  for (let d = 1; d <= totalDays; d++) cells.push(d)
  // Pad to full rows
  while (cells.length % 7 !== 0) cells.push(null)

  const display = formatDisplayI18n(value)

  const handleTriggerClick = () => {
    if (isMobile) {
      nativeRef.current?.showPicker?.()
      nativeRef.current?.click()
    } else {
      setOpen(o => !o)
    }
  }

  return (
    <div className={`relative ${className}`}>
      {/* Mobile native fallback (hidden visually) */}
      <input
        ref={nativeRef}
        type="date"
        value={value}
        required={required}
        onChange={e => onChange(e.target.value)}
        className="sr-only"
        tabIndex={-1}
        aria-hidden="true"
      />

      {/* Custom trigger button */}
      <button
        ref={triggerRef}
        type="button"
        onClick={handleTriggerClick}
        className={`
          w-full flex items-center gap-3 border border-gray-200 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-left
          text-sm bg-gray-50 dark:bg-gray-800 transition-all hover:bg-white dark:hover:bg-gray-900 hover:border-gray-300 dark:hover:border-gray-600
          focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent
          ${open ? 'ring-2 ring-violet-500 border-transparent bg-white dark:bg-gray-900' : ''}
        `}
      >
        {/* Calendar icon */}
        <svg className="w-[18px] h-[18px] text-violet-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
        <span className={display ? 'text-gray-800 dark:text-gray-200 font-medium' : 'text-gray-400 dark:text-gray-500'}>
          {display || resolvedPlaceholder}
        </span>
        {/* Chevron */}
        <svg className={`w-4 h-4 text-gray-400 dark:text-gray-500 ml-auto shrink-0 transition-transform ${open ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Desktop calendar dropdown via portal */}
      {createPortal(
        <AnimatePresence>
          {open && (
            <motion.div
              ref={dropdownRef}
              initial={{ opacity: 0, y: -4 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -4 }}
              transition={{ duration: 0.18, ease: [0.4, 0, 0.2, 1] }}
              style={{ position: 'absolute', top: pos.top, left: pos.left, width: pos.width, zIndex: 50 }}
              className="bg-white dark:bg-gray-900 rounded-2xl border border-gray-200/80 dark:border-gray-700 shadow-xl shadow-gray-200/60 dark:shadow-black/30 p-4 select-none"
            >
              {/* Month nav header */}
              <div className="flex items-center justify-between mb-3">
                <button
                  type="button"
                  onClick={prevMonth}
                  className="w-8 h-8 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center justify-center text-gray-500 dark:text-gray-400 transition-colors"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" /></svg>
                </button>
                <span className="text-sm font-bold text-gray-800 dark:text-gray-200">
                  {t('datePicker.yearMonth', { year: viewYear, month: MONTH_NAMES[viewMonth] })}
                </span>
                <button
                  type="button"
                  onClick={nextMonth}
                  className="w-8 h-8 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 flex items-center justify-center text-gray-500 dark:text-gray-400 transition-colors"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
                </button>
              </div>

              {/* Weekday headers */}
              <div className="grid grid-cols-7 gap-0.5 mb-1">
                {WEEKDAYS.map(w => (
                  <div key={w} className="text-center text-[10px] font-semibold text-gray-400 dark:text-gray-500 uppercase py-1">{w}</div>
                ))}
              </div>

              {/* Day grid */}
              <div className="grid grid-cols-7 gap-0.5">
                {cells.map((day, i) => {
                  if (day === null) return <div key={`e-${i}`} className="h-8" />
                  const selected = value === toDateStr(viewYear, viewMonth, day)
                  const today = isToday(viewYear, viewMonth, day)
                  return (
                    <button
                      key={day}
                      type="button"
                      onClick={() => selectDate(day)}
                      className={`h-8 rounded-lg text-[13px] font-medium transition-all ${
                        selected
                          ? 'bg-violet-600 text-white shadow-sm shadow-violet-300/40'
                          : today
                            ? 'bg-violet-50 dark:bg-violet-900/30 text-violet-700 dark:text-violet-400 font-bold ring-1 ring-violet-200 dark:ring-violet-700'
                            : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800'
                      }`}
                    >
                      {day}
                    </button>
                  )
                })}
              </div>

              {/* Footer: today button */}
              <div className="mt-3 flex items-center justify-between border-t border-gray-100 dark:border-gray-800 pt-3">
                <button
                  type="button"
                  onClick={goToday}
                  className="text-xs font-semibold text-violet-600 dark:text-violet-400 hover:text-violet-700 dark:hover:text-violet-300 hover:bg-violet-50 dark:hover:bg-violet-900/30 px-3 py-1.5 rounded-lg transition-colors"
                >
                  {t('datePicker.today')}
                </button>
                <span className="text-[10px] text-gray-400 dark:text-gray-500 tabular-nums">
                  {value || t('datePicker.notSelected')}
                </span>
              </div>
            </motion.div>
          )}
        </AnimatePresence>,
        document.body,
      )}
    </div>
  )
}
