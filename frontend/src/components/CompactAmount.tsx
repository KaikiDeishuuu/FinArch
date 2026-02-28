import { useState } from 'react'

interface Props {
  compact: string
  exact: string
  className?: string
  prefix?: string
}

/**
 * Shows a compact/abbreviated amount.
 * - Desktop: native `title` tooltip on hover
 * - Mobile: tap shows a position:fixed popover (escapes overflow:hidden) for 3 s
 * - Dotted underline hints the value is tappable when abbreviated
 */
export default function CompactAmount({ compact, exact, className = '', prefix = '' }: Props) {
  const [show, setShow] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })
  const isAbbreviated = compact !== exact

  function handleClick(e: React.MouseEvent) {
    if (!isAbbreviated) return
    e.stopPropagation()
    if (show) {
      setShow(false)
      return
    }
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    setPos({ x: rect.left + rect.width / 2, y: rect.top - 8 })
    setShow(true)
  }

  return (
    <>
      <span
        title={isAbbreviated ? exact : undefined}
        onClick={handleClick}
        className={[
          className,
          isAbbreviated ? 'cursor-pointer select-none' : '',
        ].join(' ')}
        style={isAbbreviated ? {
          textDecoration: 'underline',
          textDecorationStyle: 'dotted',
          textUnderlineOffset: '3px',
          textDecorationColor: 'currentColor',
        } : {}}
      >
        {prefix}{compact}
      </span>

      {show && (
        <div
          onClick={() => setShow(false)}
          style={{
            position: 'fixed',
            left: pos.x,
            top: pos.y,
            transform: 'translate(-50%, -100%)',
            zIndex: 9999,
            pointerEvents: 'auto',
          }}
          className="bg-gray-900 text-white text-xs font-medium px-3 py-1.5 rounded-lg shadow-lg whitespace-nowrap"
        >
          {prefix}{exact}
          <div
            style={{
              position: 'absolute',
              bottom: -4,
              left: '50%',
              transform: 'translateX(-50%)',
              width: 0,
              height: 0,
              borderLeft: '5px solid transparent',
              borderRight: '5px solid transparent',
              borderTop: '5px solid #111827',
            }}
          />
        </div>
      )}
    </>
  )
}
