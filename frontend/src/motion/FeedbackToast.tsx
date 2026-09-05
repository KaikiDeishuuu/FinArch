/**
 * FeedbackToast — 状态反馈动效
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   成功/失败反馈通过 opacity + 4px y-offset 从底部浮入。
 *   配合 sonner toast 使用时可直接用 CSS；此组件为内联反馈提供一致动效。
 *
 * 用户心理：
 *   操作反馈需要"被看到但不打断流程"。轻柔浮入让用户余光可以捕获，
 *   但不会像弹窗一样中断当前操作。
 *
 * 为什么不用炫酷动画：
 *   金融系统中频繁的操作反馈（标记报销、上传状态）如果用夸张动画
 *   会让用户觉得系统"不稳重"。轻柔淡入 → 自然消失最合适。
 *
 * transition 参数：
 *   duration: 0.22s — 与标准过渡一致
 *   ease: [0.4, 0, 0.2, 1]
 */
import { AnimatePresence, motion } from 'framer-motion'
import { T_STANDARD } from './tokens'

type FeedbackType = 'success' | 'error' | 'info'

interface Props {
  /** 是否可见 */
  show: boolean
  /** 反馈类型 */
  type?: FeedbackType
  /** 反馈文案 */
  message: string
  className?: string
}

const typeStyles: Record<FeedbackType, string> = {
  success: 'bg-emerald-50 border-emerald-200 text-emerald-700',
  error: 'bg-rose-50 border-rose-200 text-rose-700',
  info: 'bg-violet-50 border-violet-200 text-violet-700',
}

const typeIcons: Record<FeedbackType, string> = {
  success: '✓',
  error: '✕',
  info: 'i',
}

export default function FeedbackToast({
  show,
  type = 'success',
  message,
  className = '',
}: Props) {
  return (
    <AnimatePresence>
      {show && (
        <motion.div
          className={`flex items-center gap-2.5 px-4 py-2.5 rounded-xl border text-sm font-medium ${typeStyles[type]} ${className}`}
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={T_STANDARD}
        >
          <span className="text-xs font-bold w-5 h-5 rounded-full bg-current/10 flex items-center justify-center shrink-0">
            {typeIcons[type]}
          </span>
          {message}
        </motion.div>
      )}
    </AnimatePresence>
  )
}
