/**
 * AnimatedCollapse — 展开 / 收起动效
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   使用 height auto → 0 + opacity 同步过渡。
 *   比纯 display:none 切换多了空间连续性：用户能看到内容"被收纳"。
 *
 * 用户心理：
 *   展开/收起是信息层级操作。动画让用户能追踪"信息去了哪里"，
 *   减少"东西突然消失"的认知负担。
 *
 * 为什么不用炫酷动画：
 *   手风琴展开不需要弹簧反弹。在财务场景中，用户可能快速展开
 *   多个分组查看数据，弹跳动画会让操作感觉"拖泥带水"。
 *
 * transition 参数：
 *   duration: 0.22s — 展开需要比 hover 稍慢，保持可追踪性
 *   ease: [0.4, 0, 0.2, 1] — 始终一致的 easeInOut
 */
import { AnimatePresence, motion } from 'framer-motion'
import type { ReactNode } from 'react'
import { T_STANDARD } from './tokens'

interface Props {
  /** 是否展开 */
  open: boolean
  children: ReactNode
  className?: string
}

export default function AnimatedCollapse({ open, children, className }: Props) {
  return (
    <AnimatePresence initial={false}>
      {open && (
        <motion.div
          className={className}
          initial={{ height: 0, opacity: 0, overflow: 'hidden' }}
          animate={{ height: 'auto', opacity: 1, overflow: 'hidden' }}
          exit={{ height: 0, opacity: 0, overflow: 'hidden' }}
          transition={T_STANDARD}
        >
          {children}
        </motion.div>
      )}
    </AnimatePresence>
  )
}
