/**
 * FinArch Motion System — Barrel Export
 * ─────────────────────────────────────────────────────────────────────────────
 * 统一入口，所有动效组件与 token 从此处导入。
 *
 * Usage:
 *   import { PageTransition, AnimatedNumber } from '@/motion'
 *   import { T_STANDARD, pageVariants } from '@/motion/tokens'
 */

// Tokens & constants
export * from './tokens'

// Components
export { default as PageTransition } from './PageTransition'
export { default as AnimatedNumber } from './AnimatedNumber'
