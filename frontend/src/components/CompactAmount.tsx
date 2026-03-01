import { useState, useEffect, useRef, useCallback } from 'react'
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
  const triggerRef = useRef<HTMLSpanElement>(null)
  const bubbleRef = useRef<HTMLDivElement>(null)
  const isAbbreviated = compact !== exact

  // Recalculate position from the trigger element
  const updatePos = useCallback(() => {
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    setPos({ x: rect.left + rect.width / 2, y: rect.top - 8 })
  }, [])

  function handleClick(e: React.MouseEvent) {
    if (!isAbbreviated) return
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
      <span
        ref={triggerRef}
        title={isAbbreviated ? exact : undefined}
        onClick={handleClick}
        className={[className, isAbbreviated ? 'cursor-pointer select-none' : ''].join(' ')}
        style={isAbbreviated ? {
          textDecoration: 'underline',
          textDecorationStyle: 'dotted',
          textUnderlineOffset: '3px',
          textDecorationColor: 'currentColor',
        } : {}}
      >
        {prefix}{compact}
      </span>

      {show && createPortal(
        <div
          ref={bubbleRef}
          style={{
            position: 'fixed',
            left: pos.x,
            top: pos.y,
            transform: 'translate(-50%, -100%)',
            zIndex: 9999,
          }}
          className="bg-gray-900 text-white text-xs font-medium px-3 py-1.5 rounded-lg shadow-lg whitespace-nowrap pointer-events-auto"
        >
          {prefix}{exact}
          <div style={{
            position: 'absolute', bottom: -4, left: '50%',
            transform: 'translateX(-50%)', width: 0, height: 0,
            borderLeft: '5px solid transparent', borderRight: '5px solid transparent',
            borderTop: '5px solid #111827',
          }} />
        </div>,
        document.body
      )}
    </>
  )
}
