/**
 * AnimatedNumber — 统计数字平滑过渡
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   当数字变化时（如切换月份、筛选），数字通过 useMotionValue + useSpring
 *   平滑过渡到新值。不使用翻牌、滚轮等花哨效果。
 *
 * 用户心理：
 *   财务数字是用户最关注的元素。直接跳变会让用户"失去"上一个值的记忆，
 *   平滑过渡让大脑能追踪变化方向（增加还是减少），增强数据感知。
 *
 * 为什么不用炫酷动画：
 *   翻牌动画（flip counter）在娱乐产品中很常见，但在财务系统中会显得不专业。
 *   简单的数值 tween 最克制、最安静。
 *
 * transition 参数：
 *   useSpring: stiffness 80, damping 20 — 极轻弹簧，几乎无回弹
 *   等效于约 300ms 的平滑过渡
 */
import { useEffect, useRef } from 'react'
import { useMotionValue, useSpring, useTransform, motion } from 'framer-motion'

interface Props {
  /** 目标数值 */
  value: number
  /** 格式化函数 */
  formatter?: (n: number) => string
  /** 额外 className */
  className?: string
}

const springConfig = { stiffness: 80, damping: 20, mass: 0.8 }

export default function AnimatedNumber({
  value,
  formatter = (n) => n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 }),
  className,
}: Props) {
  const motionVal = useMotionValue(0)
  const spring = useSpring(motionVal, springConfig)
  const display = useTransform(spring, (latest) => formatter(latest))
  const isFirst = useRef(true)

  useEffect(() => {
    if (isFirst.current) {
      // 首次渲染：直接跳到目标值（避免从 0 开始计数）
      motionVal.set(value)
      isFirst.current = false
    } else {
      motionVal.set(value)
    }
  }, [value, motionVal])

  return <motion.span className={className}>{display}</motion.span>
}
