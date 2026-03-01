/**
 * Skeleton — 数据加载占位骨架屏
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   使用极慢的 opacity 脉冲（不是滑动高光），通过 0.4 → 1 → 0.4 呼吸。
 *   比滑动光条更安静，适合金融类严肃界面。
 *
 * 用户心理：
 *   骨架屏告知用户"数据正在准备"，比空白或 spinner 有更好的感知性能。
 *   缓慢的呼吸节奏（2s周期）暗示"系统正在平稳运作"。
 *
 * 为什么不用炫酷动画：
 *   滑动光条（shimmer）在电商、社交产品中常见，但在财务系统中
 *   过于花哨。简朴的呼吸脉动更内敛、更匹配企业审美。
 *
 * transition 参数：
 *   duration: 2s — 一个完整呼吸周期
 *   repeat: Infinity, repeatType: "reverse"
 *   ease: "easeInOut" — 平滑脉动
 */
import { motion } from 'framer-motion'

interface Props {
  className?: string
  /** 宽度类名，如 "w-32" "w-full" */
  width?: string
  /** 高度类名，如 "h-4" "h-10" */
  height?: string
  /** 是否为圆形 */
  rounded?: boolean
}

export default function Skeleton({
  className = '',
  width = 'w-full',
  height = 'h-4',
  rounded = false,
}: Props) {
  return (
    <motion.div
      className={`bg-gray-200/60 ${rounded ? 'rounded-full' : 'rounded-lg'} ${width} ${height} ${className}`}
      animate={{ opacity: [0.4, 1, 0.4] }}
      transition={{
        duration: 2,
        repeat: Infinity,
        ease: 'easeInOut',
      }}
    />
  )
}

// ── Preset: 卡片骨架 ──────────────────────────────────────────────────────────

export function CardSkeleton({ className = '' }: { className?: string }) {
  return (
    <div className={`bg-white rounded-2xl border border-gray-100/80 p-5 space-y-3 ${className}`}>
      <div className="flex items-center gap-2">
        <Skeleton width="w-8" height="h-8" rounded />
        <Skeleton width="w-24" height="h-3" />
      </div>
      <Skeleton width="w-36" height="h-6" />
      <Skeleton width="w-20" height="h-3" />
    </div>
  )
}

// ── Preset: 列表行骨架 ────────────────────────────────────────────────────────

export function RowSkeleton({ className = '' }: { className?: string }) {
  return (
    <div className={`flex items-center gap-4 py-3 px-5 ${className}`}>
      <Skeleton width="w-3" height="h-3" rounded />
      <Skeleton width="w-20" height="h-4" />
      <Skeleton width="w-16" height="h-4" />
      <div className="flex-1" />
      <Skeleton width="w-24" height="h-4" />
      <Skeleton width="w-16" height="h-6" className="rounded-full" />
    </div>
  )
}
