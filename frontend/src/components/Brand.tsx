/** FinArch Ledger brand geometry: an accounting rail crossed with a rising arch. */
const BRAND = {
  ink: '#17201d',
  draft: '#f3f6f2',
  field: '#e7ede8',
  rule: '#cbd5ce',
  work: '#2d6687',
  life: '#28745b',
  oxide: '#b95642',
} as const

// ── LogoMark ────────────────────────────────────────────────────────────────
// Full logo with dark bg + 3 bars. Drop-in replacement for <img src="/logo.svg">

interface LogoMarkProps {
  size?: number
  className?: string
}

export function LogoMark({ size = 36, className = '' }: LogoMarkProps) {
  return (
    <svg
      viewBox="0 0 200 200"
      width={size}
      height={size}
      className={`shrink-0 ${className}`}
      role="img"
      aria-label="FinArch"
    >
      <rect width="200" height="200" rx="18" fill={BRAND.ink} />
      <path d="M38 34v130h130" fill="none" stroke={BRAND.rule} strokeWidth="4" opacity="0.72" />
      <path d="M38 58h12M38 84h8M38 110h12M38 136h8" stroke={BRAND.rule} strokeWidth="3" opacity="0.8" />
      <rect x="59" y="112" width="25" height="52" rx="3" fill={BRAND.work} />
      <rect x="99" y="82" width="25" height="82" rx="3" fill={BRAND.life} />
      <rect x="139" y="49" width="25" height="115" rx="3" fill={BRAND.oxide} />
      <path d="M59 103h25M99 73h25M139 40h25" stroke={BRAND.draft} strokeWidth="3" opacity="0.9" />
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
      <path d="M1 1v38h50" fill="none" stroke={BRAND.rule} strokeWidth="1.5" />
      <path d="M1 10h4M1 20h3M1 30h4" stroke={BRAND.rule} strokeWidth="1.5" />
      <rect x="8" y="24" width="10" height="15" rx="1" fill={BRAND.work} />
      <rect x="24" y="14" width="10" height="25" rx="1" fill={BRAND.life} />
      <rect x="40" y="3" width="10" height="36" rx="1" fill={BRAND.oxide} />
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
        <path d="M7 3v80h109" fill="none" stroke={BRAND.ink} strokeWidth="3" />
        <path d="M7 18h9M7 36h6M7 54h9M7 72h6" stroke={BRAND.ink} strokeWidth="2" />
        <rect x="25" y="51" width="21" height="32" rx="2" fill={BRAND.work} />
        <rect x="57" y="30" width="21" height="53" rx="2" fill={BRAND.life} />
        <rect x="89" y="6" width="21" height="77" rx="2" fill={BRAND.oxide} />
      </svg>
    </div>
  )
}

// ── BrandDivider ────────────────────────────────────────────────────────────
// A ruled divider with evenly measured registration marks.

interface BrandDividerProps {
  className?: string
}

export function BrandDivider({ className = '' }: BrandDividerProps) {
  return (
    <div
      className={`relative h-2 w-full ${className}`}
      style={{ opacity: 0.52 }}
      aria-hidden="true"
    >
      <span className="absolute inset-x-0 top-1/2 h-px" style={{ backgroundColor: BRAND.rule }} />
      <span className="absolute left-1/4 top-0 h-2 w-px" style={{ backgroundColor: BRAND.work }} />
      <span className="absolute left-1/2 top-0 h-2 w-px" style={{ backgroundColor: BRAND.life }} />
      <span className="absolute left-3/4 top-0 h-2 w-px" style={{ backgroundColor: BRAND.oxide }} />
    </div>
  )
}

// ── BrandDot ────────────────────────────────────────────────────────────────
// Small colored dot accent — pick bar1/bar2/bar3

interface BrandDotProps {
  variant?: 'work' | 'life' | 'oxide' | 'indigo' | 'violet' | 'emerald'
  size?: 'sm' | 'md'
  className?: string
}

const DOT_COLORS = {
  work: BRAND.work,
  life: BRAND.life,
  oxide: BRAND.oxide,
  /* Compatibility aliases for pre-redesign call sites. */
  indigo: BRAND.work,
  violet: BRAND.work,
  emerald: BRAND.life,
}

export function BrandDot({ variant = 'work', size = 'sm', className = '' }: BrandDotProps) {
  const s = size === 'sm' ? 'w-1.5 h-1.5' : 'w-2 h-2'
  return (
    <span
      className={`inline-block rounded-full ${s} ${className}`}
      style={{ background: DOT_COLORS[variant] }}
      aria-hidden="true"
    />
  )
}
