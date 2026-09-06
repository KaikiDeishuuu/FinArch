/** Shared timing tokens for restrained financial-workstation motion. */
export const EASE_STANDARD = [0.4, 0, 0.2, 1] as const

export const DURATION_NORMAL = 0.22
export const DURATION_PAGE = 0.28

export const T_PAGE = {
  duration: DURATION_PAGE,
  ease: EASE_STANDARD,
} as const

export const MAX_Y_OFFSET = 6

export const pageVariants = {
  initial: { opacity: 0, y: MAX_Y_OFFSET },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
} as const
