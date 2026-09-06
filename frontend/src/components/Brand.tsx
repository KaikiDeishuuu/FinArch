/** FinArch's ascending bars use one stable palette across product and PWA assets. */
const BRAND_COLORS = {
  bar1: '#6B7A99',
  bar2: '#3D6BE8',
  bar3: '#2FB37C',
} as const

interface LogoMarkProps {
  size?: number
  className?: string
  decorative?: boolean
}

export function LogoMark({ size = 36, className = '', decorative = false }: LogoMarkProps) {
  return (
    <svg
      viewBox="0 0 200 200"
      width={size}
      height={size}
      className={`shrink-0 ${className}`}
      role={decorative ? undefined : 'img'}
      aria-label={decorative ? undefined : 'FinArch'}
      aria-hidden={decorative || undefined}
    >
      <rect width="200" height="200" rx="40" fill="var(--brand-background, #161A22)" />
      <rect x="28" y="104" width="44" height="58" rx="10" fill={BRAND_COLORS.bar1} />
      <rect x="78" y="74" width="44" height="88" rx="10" fill={BRAND_COLORS.bar2} />
      <rect x="128" y="38" width="44" height="124" rx="10" fill={BRAND_COLORS.bar3} />
    </svg>
  )
}

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
      height={size * (40 / 52)}
      className={`shrink-0 ${className}`}
      style={{ opacity }}
      aria-hidden="true"
    >
      <rect x="0" y="22" width="14" height="18" rx="3" fill={BRAND_COLORS.bar1} />
      <rect x="19" y="12" width="14" height="28" rx="3" fill={BRAND_COLORS.bar2} />
      <rect x="38" y="0" width="14" height="40" rx="3" fill={BRAND_COLORS.bar3} />
    </svg>
  )
}
