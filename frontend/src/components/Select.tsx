/**
 * FinArch — Apple-Style Select Component
 * ─────────────────────────────────────────────────────────────────────────────
 * 企业级财务管理系统统一下拉菜单组件
 *
 * 设计原则：
 * - 圆角统一体系：trigger 10px / dropdown 12px / option 8px
 * - 阴影层级：dropdown 使用 lg 级别 (0 10px 25px -5px)
 * - 动效存在感 < 30%：仅 opacity + 4px y-offset，220ms, cubic-bezier(0.4,0,0.2,1)
 * - 支持：单选 / 占位符 / 键盘导航 / 禁用 / 错误态
 * - Portal 挂载：避免 overflow:hidden 裁切
 * - z-index: 50 (与 tooltip 同级)
 *
 * 状态设计：
 * - default:    bg-gray-50, border-gray-200, 安静不干扰
 * - hover:      border-gray-300, 轻微暗示可交互
 * - focus:      ring-2 ring-violet-500/20, border-violet-400, 明确焦点
 * - open:       同 focus + dropdown 展开
 * - selected:   text-gray-900 (替代 placeholder 灰色)
 * - disabled:   opacity-50, cursor-not-allowed
 * - error:      ring-2 ring-rose-500/20, border-rose-400
 *
 * Token:
 * - 颜色: bg-gray-50(default) → bg-white(hover) → violet-400(focus border)
 * - 圆角: trigger 10px, dropdown 12px, option 8px
 * - 阴影: dropdown shadow-lg
 * - 间距: sm(h-8 px-2.5) / md(h-9 px-3) / lg(h-10 px-3.5)
 * ─────────────────────────────────────────────────────────────────────────────
 */

import { useState, useRef, useEffect, useCallback, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { motion, AnimatePresence } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { EASE_STANDARD, DURATION_NORMAL } from '../motion/tokens'

// ── Types ───────────────────────────────────────────────────────────────────

export interface SelectOption {
  value: string
  label: string
  disabled?: boolean
}

export interface SelectProps {
  /** 选项列表 */
  options: SelectOption[]
  /** 当前选中值 */
  value: string
  /** 值变更回调 */
  onChange: (value: string) => void
  /** 占位符文本 */
  placeholder?: string
  /** 整体禁用 */
  disabled?: boolean
  /** 错误态 */
  error?: boolean
  /** 尺寸：sm / md / lg */
  size?: 'sm' | 'md' | 'lg'
  /** 附加 className */
  className?: string
  /** 是否有"选中 = 高亮"效果（如筛选器被激活时） */
  activeHighlight?: boolean
}

// ── Sizing tokens ───────────────────────────────────────────────────────────

const SIZE_MAP = {
  sm: 'h-8 text-xs px-2.5 pr-7',
  md: 'h-9 text-sm px-3 pr-8',
  lg: 'h-10 text-sm px-3.5 pr-9',
} as const

const CHEVRON_SIZE = { sm: 'w-3 h-3 right-2', md: 'w-3.5 h-3.5 right-2.5', lg: 'w-4 h-4 right-3' } as const

// ── Dropdown animation ─────────────────────────────────────────────────────

const dropdownVariants = {
  hidden: { opacity: 0, y: -4 },
  visible: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
}

const dropdownTransition = {
  duration: DURATION_NORMAL,
  ease: EASE_STANDARD,
}

// ── Component ───────────────────────────────────────────────────────────────

export default function Select({
  options,
  value,
  onChange,
  placeholder = '请选择',
  disabled = false,
  error = false,
  size = 'md',
  className = '',
  activeHighlight = false,
}: SelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder === '请选择' ? t('select.placeholder') : placeholder
  const [open, setOpen] = useState(false)
  const [highlightIndex, setHighlightIndex] = useState(-1)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const [rect, setRect] = useState<DOMRect | null>(null)

  const selected = useMemo(() => options.find(o => o.value === value), [options, value])
  const hasValue = !!selected

  // ── Position calculation ────────────────────────────────────────────────

  const updateRect = useCallback(() => {
    if (triggerRef.current) {
      setRect(triggerRef.current.getBoundingClientRect())
    }
  }, [])

  const openDropdown = useCallback(() => {
    if (disabled) return
    updateRect()
    setOpen(true)
    const idx = options.findIndex(o => o.value === value)
    setHighlightIndex(idx >= 0 ? idx : 0)
  }, [disabled, updateRect, options, value])

  const closeDropdown = useCallback(() => {
    setOpen(false)
    triggerRef.current?.focus()
  }, [])

  const selectOption = useCallback((opt: SelectOption) => {
    if (opt.disabled) return
    onChange(opt.value)
    closeDropdown()
  }, [onChange, closeDropdown])

  // ── Click outside ───────────────────────────────────────────────────────

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (
        triggerRef.current?.contains(e.target as Node) ||
        dropdownRef.current?.contains(e.target as Node)
      ) return
      closeDropdown()
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open, closeDropdown])

  // ── Scroll / resize → reposition ────────────────────────────────────────

  useEffect(() => {
    if (!open) return
    const reposition = () => updateRect()
    window.addEventListener('scroll', reposition, true)
    window.addEventListener('resize', reposition)
    return () => {
      window.removeEventListener('scroll', reposition, true)
      window.removeEventListener('resize', reposition)
    }
  }, [open, updateRect])

  // ── Keyboard navigation ─────────────────────────────────────────────────

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (disabled) return

    if (!open) {
      if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(e.key)) {
        e.preventDefault()
        openDropdown()
      }
      return
    }

    switch (e.key) {
      case 'ArrowDown': {
        e.preventDefault()
        let next = highlightIndex
        do {
          next = (next + 1) % options.length
        } while (options[next]?.disabled && next !== highlightIndex)
        setHighlightIndex(next)
        break
      }
      case 'ArrowUp': {
        e.preventDefault()
        let prev = highlightIndex
        do {
          prev = (prev - 1 + options.length) % options.length
        } while (options[prev]?.disabled && prev !== highlightIndex)
        setHighlightIndex(prev)
        break
      }
      case 'Enter':
      case ' ': {
        e.preventDefault()
        if (highlightIndex >= 0 && !options[highlightIndex]?.disabled) {
          selectOption(options[highlightIndex])
        }
        break
      }
      case 'Escape':
      case 'Tab': {
        closeDropdown()
        break
      }
      case 'Home': {
        e.preventDefault()
        const first = options.findIndex(o => !o.disabled)
        if (first >= 0) setHighlightIndex(first)
        break
      }
      case 'End': {
        e.preventDefault()
        const last = [...options].reverse().findIndex(o => !o.disabled)
        if (last >= 0) setHighlightIndex(options.length - 1 - last)
        break
      }
    }
  }, [disabled, open, highlightIndex, options, openDropdown, closeDropdown, selectOption])

  // ── Scroll highlighted item into view ───────────────────────────────────

  useEffect(() => {
    if (!open || highlightIndex < 0) return
    const el = dropdownRef.current?.querySelector(`[data-index="${highlightIndex}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [open, highlightIndex])

  // ── Trigger classes ─────────────────────────────────────────────────────

  const isActive = activeHighlight && hasValue

  const triggerCls = [
    'relative w-full rounded-[10px] border outline-none transition-all cursor-pointer text-left',
    'focus:ring-2',
    SIZE_MAP[size],
    disabled
      ? 'opacity-50 cursor-not-allowed bg-gray-100 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500'
      : error
        ? 'bg-white dark:bg-gray-900 border-rose-400 ring-2 ring-rose-500/20 text-gray-700 dark:text-gray-300'
        : isActive
          ? 'border-violet-400 bg-violet-50 dark:bg-violet-900/30 text-violet-700 dark:text-violet-400 font-semibold focus:ring-violet-500/20'
          : open
            ? 'bg-white dark:bg-gray-900 border-violet-400 ring-2 ring-violet-500/20 text-gray-700 dark:text-gray-300'
            : 'bg-gray-50 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-500 dark:text-gray-400 hover:bg-white dark:hover:bg-gray-900 hover:border-gray-300 dark:hover:border-gray-600 focus:ring-violet-500/20 focus:border-violet-400',
    className,
  ].join(' ')

  // ── Dropdown position ───────────────────────────────────────────────────

  const dropdownStyle = rect ? {
    position: 'fixed' as const,
    top: rect.bottom + 4,
    left: rect.left,
    width: rect.width,
    zIndex: 50,
  } : undefined

  // ── Render ──────────────────────────────────────────────────────────────

  return (
    <>
      {/* Trigger */}
      <button
        ref={triggerRef}
        type="button"
        role="combobox"
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={disabled}
        className={triggerCls}
        onClick={() => open ? closeDropdown() : openDropdown()}
        onKeyDown={handleKeyDown}
      >
        <span className={`block truncate ${hasValue ? '' : 'text-gray-400 dark:text-gray-500'}`}>
          {selected?.label ?? resolvedPlaceholder}
        </span>

        {/* Chevron */}
        <svg
          className={`absolute top-1/2 -translate-y-1/2 pointer-events-none text-gray-400 dark:text-gray-500 transition-transform duration-200 ${CHEVRON_SIZE[size]} ${open ? 'rotate-180' : ''}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2.5}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Dropdown (Portal) */}
      {createPortal(
        <AnimatePresence>
          {open && dropdownStyle && (
            <motion.div
              ref={dropdownRef}
              role="listbox"
              variants={dropdownVariants}
              initial="hidden"
              animate="visible"
              exit="exit"
              transition={dropdownTransition}
              style={dropdownStyle}
              className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200/80 dark:border-gray-700 shadow-lg py-1 max-h-60 overflow-y-auto overscroll-contain"
            >
              {options.map((opt, idx) => {
                const isSelected = opt.value === value
                const isHighlighted = idx === highlightIndex

                return (
                  <div
                    key={opt.value}
                    role="option"
                    aria-selected={isSelected}
                    data-index={idx}
                    className={[
                      'flex items-center justify-between gap-2 px-3 py-2 mx-1 rounded-lg text-sm cursor-pointer transition-colors duration-150 select-none',
                      opt.disabled
                        ? 'opacity-40 cursor-not-allowed text-gray-400 dark:text-gray-500'
                        : isHighlighted
                          ? 'bg-violet-50 dark:bg-violet-900/30 text-violet-700 dark:text-violet-400'
                          : 'text-gray-700 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800',
                    ].join(' ')}
                    onClick={() => selectOption(opt)}
                    onMouseEnter={() => !opt.disabled && setHighlightIndex(idx)}
                  >
                    <span className={`truncate ${isSelected ? 'font-semibold' : 'font-normal'}`}>
                      {opt.label}
                    </span>
                    {isSelected && (
                      <svg className="w-4 h-4 text-violet-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                    )}
                  </div>
                )
              })}
            </motion.div>
          )}
        </AnimatePresence>,
        document.body,
      )}
    </>
  )
}
