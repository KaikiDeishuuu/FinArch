import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

interface Props {
  compact: string
  exact: string
  className?: string
  prefix?: string
}

export default function CompactAmount({ compact, exact, className = '', prefix = '' }: Props) {
  const [show, setShow] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })
  const triggerRef = useRef<HTMLButtonElement>(null)
  const bubbleRef = useRef<HTMLDivElement>(null)
  const tooltipId = useId()
  const isAbbreviated = compact !== exact

  // Recalculate position from the trigger element
  const updatePos = useCallback(() => {
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    setPos({ x: rect.left + rect.width / 2, y: rect.top - 8 })
  }, [])

  function handleClick(e: React.MouseEvent) {
    e.stopPropagation()
    if (show) { setShow(false); return }
    updatePos()
    setShow(true)
  }

  // Close on outside click; reposition on scroll/resize
  useEffect(() => {
    if (!show) return
    function onDocClick(e: MouseEvent) {
      if (bubbleRef.current && bubbleRef.current.contains(e.target as Node)) return
      if (triggerRef.current && triggerRef.current.contains(e.target as Node)) return
      setShow(false)
    }
    function onScrollOrResize() { updatePos() }
    document.addEventListener('click', onDocClick, true)
    window.addEventListener('scroll', onScrollOrResize, true)
    window.addEventListener('resize', onScrollOrResize)
    return () => {
      document.removeEventListener('click', onDocClick, true)
      window.removeEventListener('scroll', onScrollOrResize, true)
      window.removeEventListener('resize', onScrollOrResize)
    }
  }, [show, updatePos])

  return (
    <>
      {isAbbreviated ? (
        <button
          ref={triggerRef}
          type="button"
          title={exact}
          aria-label={`${prefix}${exact}`}
          aria-describedby={show ? tooltipId : undefined}
          aria-expanded={show}
          onClick={handleClick}
          onKeyDown={(event) => {
            if (event.key !== 'Escape' || !show) return
            event.stopPropagation()
            setShow(false)
          }}
          className={[
            className,
            'cursor-pointer select-none rounded-sm bg-transparent p-0 font-inherit text-inherit underline decoration-dotted underline-offset-[3px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
          ].join(' ')}
        >
          {prefix}{compact}
        </button>
      ) : (
        <span className={className}>{prefix}{compact}</span>
      )}

      {show && createPortal(
        <div
          ref={bubbleRef}
          id={tooltipId}
          role="tooltip"
          style={{
            position: 'fixed',
            left: pos.x,
            top: pos.y,
            transform: 'translate(-50%, -100%)',
            zIndex: 9999,
          }}
          className="pointer-events-auto whitespace-nowrap rounded-lg bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-lg"
        >
          {prefix}{exact}
          <span
            aria-hidden="true"
            className="absolute -bottom-1 left-1/2 size-2 -translate-x-1/2 rotate-45 bg-foreground"
          />
        </div>,
        document.body
      )}
    </>
  )
}
