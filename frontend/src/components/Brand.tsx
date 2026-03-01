/**
 * FinArch — Brand Decorative Components
 * ─────────────────────────────────────────────────────────────────────────────
 * Logo-themed decorative elements for use across the dashboard.
 *
 * Brand Palette:
 * - Bar 1: Indigo-400  #818cf8
 * - Bar 2: Violet-400  #a78bfa
 * - Bar 3: Emerald-400 #34d399
 * - Background: Near-black #0d0b14 → Dark-violet #170f26
 *
 * Components:
 * - LogoMark:       Inline SVG logo (configurable size)
 * - LogoBars:       Standalone 3-bar icon without background
 * - BrandWatermark: Subtle watermark for card/section backgrounds
 * - BrandDivider:   Gradient divider using brand colors
 * - BrandDot:       Small decorative dot accent
 * ─────────────────────────────────────────────────────────────────────────────
 */

/** Brand colors — single source of truth */
export const BRAND = {
  bar1: '#818cf8',  // Indigo-400
  bar2: '#a78bfa',  // Violet-400
  bar3: '#34d399',  // Emerald-400
  bgFrom: '#0d0b14', // Near-black
  bgTo: '#170f26',    // Dark-violet-black
} as const

// ── LogoMark ────────────────────────────────────────────────────────────────
// Full logo with dark bg + 3 bars. Drop-in replacement for <img src="/logo.svg">

interface LogoMarkProps {
  size?: number
  className?: string
}

export function LogoMark({ size = 36, className = '' }: LogoMarkProps) {
  const r = size * 0.2 // corner radius
  return (
    <svg
      viewBox="0 0 200 200"
      width={size}
      height={size}
      className={`shrink-0 ${className}`}
      role="img"
      aria-label="FinArch"
    >
      <defs>
        <linearGradient id="lm-bg" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor={BRAND.bgFrom} />
          <stop offset="100%" stopColor={BRAND.bgTo} />
        </linearGradient>
      </defs>
      <rect width="200" height="200" rx={r / size * 200} fill="url(#lm-bg)" />
      <rect x="28" y="104" width="44" height="58" rx="10" fill={BRAND.bar1} />
      <rect x="78" y="74" width="44" height="88" rx="10" fill={BRAND.bar2} />
      <rect x="128" y="38" width="44" height="124" rx="10" fill={BRAND.bar3} />
    </svg>
  )
}

// ── LogoBars ─────────────────────────────────────────────────────────────────
// Standalone 3 ascending bars without background — for use as subtle inline icon

interface LogoBarsProps {
  size?: number
  className?: string
  opacity?: number
}

export function LogoBars({ size = 20, className = '', opacity = 1 }: LogoBarsProps) {
  return (
    <svg
      viewBox="0 0 52 40"
      width={size}
      height={size * (40/52)}
      className={`shrink-0 ${className}`}
      style={{ opacity }}
      role="presentation"
    >
      <rect x="0" y="22" width="14" height="18" rx="3" fill={BRAND.bar1} />
      <rect x="19" y="12" width="14" height="28" rx="3" fill={BRAND.bar2} />
      <rect x="38" y="0" width="14" height="40" rx="3" fill={BRAND.bar3} />
    </svg>
  )
}

// ── BrandWatermark ──────────────────────────────────────────────────────────
// Large, faint logo bars for card/section backgrounds — purely decorative

interface BrandWatermarkProps {
  className?: string
  opacity?: number
}

export function BrandWatermark({ className = '', opacity = 0.04 }: BrandWatermarkProps) {
  return (
    <div className={`pointer-events-none select-none ${className}`} aria-hidden="true">
      <svg viewBox="0 0 120 90" width="120" height="90" style={{ opacity }}>
        <rect x="0" y="50" width="32" height="40" rx="6" fill={BRAND.bar1} />
        <rect x="42" y="30" width="32" height="60" rx="6" fill={BRAND.bar2} />
        <rect x="84" y="0" width="32" height="90" rx="6" fill={BRAND.bar3} />
      </svg>
    </div>
  )
}

// ── BrandDivider ────────────────────────────────────────────────────────────
// Horizontal gradient line using the 3 brand colors

interface BrandDividerProps {
  className?: string
}

export function BrandDivider({ className = '' }: BrandDividerProps) {
  return (
    <div
      className={`h-px w-full ${className}`}
      style={{
        background: `linear-gradient(90deg, transparent 0%, ${BRAND.bar1} 20%, ${BRAND.bar2} 50%, ${BRAND.bar3} 80%, transparent 100%)`,
        opacity: 0.3,
      }}
      aria-hidden="true"
    />
  )
}

// ── BrandDot ────────────────────────────────────────────────────────────────
// Small colored dot accent — pick bar1/bar2/bar3

interface BrandDotProps {
  variant?: 'indigo' | 'violet' | 'emerald'
  size?: 'sm' | 'md'
  className?: string
}

const DOT_COLORS = {
  indigo: BRAND.bar1,
  violet: BRAND.bar2,
  emerald: BRAND.bar3,
}

export function BrandDot({ variant = 'violet', size = 'sm', className = '' }: BrandDotProps) {
  const s = size === 'sm' ? 'w-1.5 h-1.5' : 'w-2 h-2'
  return (
    <span
      className={`inline-block rounded-full ${s} ${className}`}
      style={{ background: DOT_COLORS[variant] }}
      aria-hidden="true"
    />
  )
}
