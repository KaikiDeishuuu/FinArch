/**
 * FinArch Motion System — Barrel Export
 * ─────────────────────────────────────────────────────────────────────────────
 * 统一入口，所有动效组件与 token 从此处导入。
 *
 * Usage:
 *   import { PageTransition, AnimatedCard, tokens } from '@/motion'
 *   import { T_STANDARD, pageVariants } from '@/motion/tokens'
 */

// Tokens & constants
export * from './tokens'

// Components
export { default as PageTransition } from './PageTransition'
export { default as AnimatedCard } from './AnimatedCard'
export { default as AnimatedNumber } from './AnimatedNumber'
export { default as AnimatedCollapse } from './AnimatedCollapse'
export { StaggerContainer, StaggerItem } from './StaggerList'
export { default as Skeleton, CardSkeleton, RowSkeleton } from './Skeleton'
export { default as FeedbackToast } from './FeedbackToast'
