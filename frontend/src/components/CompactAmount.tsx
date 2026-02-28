import { useState, useEffect, useRef } from 'react'

interface Props {
  compact: string        // abbreviated display, e.g. ¥8.5万
  exact: string          // full precise value, e.g. ¥85,000
  className?: string
  prefix?: string        // optional prefix like "+"
}

/**
 * Shows a compact/abbreviated amount.
 * - Desktop: hover shows exact value via native `title` tooltip
 * - Mobile: tap toggles to exact value inline; auto-reverts after 3 s
 * - A dashed underline hints the value is interactive when abbreviated
 */
export default function CompactAmount({ compact, exact, className = '', prefix = '' }: Props) {
  const [expanded, setExpanded] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const isAbbreviated = compact !== exact

  function handleClick() {
    if (!isAbbreviated) return
    if (timerRef.current) clearTimeout(timerRef.current)
    if (expanded) {
      setExpanded(false)
    } else {
      setExpanded(true)
      timerRef.current = setTimeout(() => setExpanded(false), 3000)
    }
  }

  // Cleanup on unmount
  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current) }, [])

  return (
    <span
      title={isAbbreviated ? exact : undefined}
      onClick={handleClick}
      className={[
        className,
        isAbbreviated ? 'cursor-pointer select-none' : '',
      ].join(' ')}
      style={isAbbreviated ? { textDecoration: 'underline', textDecorationStyle: 'dotted', textUnderlineOffset: '3px', textDecorationColor: 'currentColor', opacity: 1 } : {}}
    >
      {expanded ? (
        <span>
          {prefix}{exact}
          <span className="ml-1 text-[0.65em] opacity-50 font-normal align-middle">精确值</span>
        </span>
      ) : (
        <span>{prefix}{compact}</span>
      )}
    </span>
  )
}
