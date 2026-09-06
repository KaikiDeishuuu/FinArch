/**
 * FinArch token-driven select component.
 * Preserves native combobox/listbox semantics, keyboard navigation, and portal
 * positioning while deriving all visual states from the shared theme tokens.
 */

import { useState, useRef, useEffect, useCallback, useMemo, useId } from 'react'
import { createPortal } from 'react-dom'
import { motion, AnimatePresence } from 'framer-motion'
import { Check, ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { EASE_STANDARD, DURATION_NORMAL } from '../motion/tokens'
import { cn } from '../lib/utils'

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
  /** Trigger id used to associate an external label. */
  id?: string
  /** Accessible name when no external label is available. */
  'aria-label'?: string
  /** Id of an external label element. */
  'aria-labelledby'?: string
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
  id,
  'aria-label': ariaLabel,
  'aria-labelledby': ariaLabelledBy,
}: SelectProps) {
  const { t } = useTranslation()
  const generatedId = useId()
  const triggerId = id ?? `select-${generatedId}`
  const listboxId = `${triggerId}-listbox`
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

  const triggerCls = cn(
    'relative w-full cursor-pointer rounded-lg border text-left outline-none transition-colors',
    'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
    SIZE_MAP[size],
    disabled
      ? 'cursor-not-allowed border-border bg-muted text-muted-foreground opacity-50'
      : error
        ? 'border-negative bg-card text-foreground ring-2 ring-negative/20'
        : isActive
          ? 'border-accent/50 bg-accent-soft font-semibold text-accent'
          : open
            ? 'border-accent bg-card text-foreground ring-2 ring-ring/25'
            : 'border-input bg-card text-muted-foreground hover:border-accent/40 hover:bg-muted/60',
    className,
  )

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
        id={triggerId}
        ref={triggerRef}
        type="button"
        role="combobox"
        aria-label={ariaLabel}
        aria-labelledby={ariaLabelledBy}
        aria-controls={listboxId}
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={disabled}
        className={triggerCls}
        onClick={() => open ? closeDropdown() : openDropdown()}
        onKeyDown={handleKeyDown}
      >
        <span className={cn('block truncate', !hasValue && 'text-muted-foreground')}>
          {selected?.label ?? resolvedPlaceholder}
        </span>

        <ChevronDown
          aria-hidden="true"
          className={cn(
            'pointer-events-none absolute top-1/2 -translate-y-1/2 text-muted-foreground transition-transform duration-200',
            CHEVRON_SIZE[size],
            open && 'rotate-180',
          )}
          strokeWidth={2}
        />
      </button>

      {/* Dropdown (Portal) */}
      {createPortal(
        <AnimatePresence>
          {open && dropdownStyle && (
            <motion.div
              id={listboxId}
              ref={dropdownRef}
              role="listbox"
              aria-labelledby={ariaLabelledBy ?? (!ariaLabel ? triggerId : undefined)}
              variants={dropdownVariants}
              initial="hidden"
              animate="visible"
              exit="exit"
              transition={dropdownTransition}
              style={dropdownStyle}
              className="max-h-60 overflow-y-auto overscroll-contain rounded-lg border border-border bg-card py-1 text-card-foreground shadow-[var(--shadow-sm)]"
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
                    className={cn(
                      'mx-1 flex cursor-pointer select-none items-center justify-between gap-2 rounded-md px-3 py-2 text-sm transition-colors duration-150',
                      opt.disabled
                        ? 'cursor-not-allowed text-muted-foreground opacity-40'
                        : isHighlighted
                          ? 'bg-accent-soft text-accent'
                          : 'text-foreground hover:bg-muted',
                    )}
                    onClick={() => selectOption(opt)}
                    onMouseEnter={() => !opt.disabled && setHighlightIndex(idx)}
                  >
                    <span className={`truncate ${isSelected ? 'font-semibold' : 'font-normal'}`}>
                      {opt.label}
                    </span>
                    {isSelected ? <Check className="size-4 shrink-0 text-accent" strokeWidth={2.5} /> : null}
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
