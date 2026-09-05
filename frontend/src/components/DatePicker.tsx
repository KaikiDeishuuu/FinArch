/**
 * FinArch Ledger date picker. Mobile keeps the native picker while desktop
 * uses a compact, ruled calendar keyed to the active WORK/LIFE mode.
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
      if (!open && value) {
        const p = parseDate(value)
        setViewYear(p.year)
        setViewMonth(p.month)
      }
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
          fin-input flex w-full items-center gap-3 px-3.5 py-2.5 text-left text-sm outline-none
          ${open ? 'border-[hsl(var(--mode-accent))] bg-[hsl(var(--control-hover))] ring-2 ring-[hsl(var(--ring))]/15' : ''}
        `}
      >
        {/* Calendar icon */}
        <svg className="h-[18px] w-[18px] shrink-0 text-[hsl(var(--mode-accent-strong))]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
        <span className={display ? 'font-medium text-[hsl(var(--foreground))]' : 'text-[hsl(var(--muted-foreground))]/75'}>
          {display || resolvedPlaceholder}
        </span>
        {/* Chevron */}
        <svg className={`ml-auto h-4 w-4 shrink-0 text-[hsl(var(--muted-foreground))] transition-transform ${open ? 'rotate-180 text-[hsl(var(--mode-accent-strong))]' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
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
              className="ledger-panel select-none p-4"
            >
              {/* Month nav header */}
              <div className="mb-3 flex items-center justify-between">
                <button
                  type="button"
                  onClick={prevMonth}
                  className="flex h-8 w-8 items-center justify-center rounded-[4px] text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]"
                >
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" /></svg>
                </button>
                <span className="font-data text-sm font-semibold tabular-nums text-[hsl(var(--foreground))]">
                  {t('datePicker.yearMonth', { year: viewYear, month: MONTH_NAMES[viewMonth] })}
                </span>
                <button
                  type="button"
                  onClick={nextMonth}
                  className="flex h-8 w-8 items-center justify-center rounded-[4px] text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]"
                >
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
                </button>
              </div>

              {/* Weekday headers */}
              <div className="mb-1 grid grid-cols-7 gap-0.5 border-y border-[hsl(var(--border))] py-1">
                {WEEKDAYS.map(w => (
                  <div key={w} className="font-data py-1 text-center text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{w}</div>
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
                      className={`font-data h-8 rounded-[4px] text-xs font-medium tabular-nums transition-colors ${
                        selected
                          ? 'bg-[hsl(var(--mode-accent))] text-[hsl(var(--primary-foreground))]'
                          : today
                            ? 'bg-[hsl(var(--mode-accent-wash))] font-bold text-[hsl(var(--mode-accent-strong))] ring-1 ring-inset ring-[hsl(var(--mode-accent))]/35'
                            : 'text-[hsl(var(--foreground))] hover:bg-[hsl(var(--muted))]'
                      }`}
                    >
                      {day}
                    </button>
                  )
                })}
              </div>

              {/* Footer: today button */}
              <div className="mt-3 flex items-center justify-between border-t border-[hsl(var(--border))] pt-3">
                <button
                  type="button"
                  onClick={goToday}
                  className="rounded-[4px] px-3 py-1.5 text-xs font-semibold text-[hsl(var(--mode-accent-strong))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))]"
                >
                  {t('datePicker.today')}
                </button>
                <span className="font-data text-[10px] tabular-nums text-[hsl(var(--muted-foreground))]">
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
