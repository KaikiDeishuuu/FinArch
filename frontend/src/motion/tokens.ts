/**
 * FinArch Motion Design Tokens
 * ─────────────────────────────────────────────────────────────────────────────
 * 企业级财务管理系统统一动效令牌
 *
 * 设计原则：
 * - 动效存在感 < 30%，用户应感知"顺滑"而非"动画"
 * - 所有持续时间 180ms – 300ms
 * - 禁止弹跳、夸张缩放、横向大位移
 * - easeInOut 为主 curve，保持空间连续性
 *
 * 参考：Apple Human Interface Guidelines · Financial-grade SaaS
 * ─────────────────────────────────────────────────────────────────────────────
 */

// ── Easing ──────────────────────────────────────────────────────────────────

/** 标准 ease — 适用于绝大多数 UI 元素过渡 */
export const EASE_STANDARD = [0.4, 0, 0.2, 1] as const

/** 进场 ease — 元素从"无"到"有"时偏快起步 */
export const EASE_ENTER = [0.0, 0, 0.2, 1] as const

/** 退场 ease — 元素淡出时保持柔和 */
export const EASE_EXIT = [0.4, 0, 1, 1] as const

// ── Duration (seconds) ─────────────────────────────────────────────────────

/** 极轻量微交互：hover 高亮、toggle 等 */
export const DURATION_INSTANT = 0.15

/** 标准过渡：卡片出现、tab 切换 */
export const DURATION_NORMAL = 0.22

/** 页面级过渡：路由切换、大面积内容替换 */
export const DURATION_PAGE = 0.28

/** 慢速过渡：模态框展开、统计数字滚动 */
export const DURATION_SLOW = 0.35

// ── Transition Presets ─────────────────────────────────────────────────────

/** 标准 transition — 适用于 90% 场景 */
export const T_STANDARD = {
  duration: DURATION_NORMAL,
  ease: EASE_STANDARD,
} as const

/** 页面过渡 */
export const T_PAGE = {
  duration: DURATION_PAGE,
  ease: EASE_STANDARD,
} as const

/** 微交互 transition */
export const T_MICRO = {
  duration: DURATION_INSTANT,
  ease: EASE_STANDARD,
} as const

/** 列表 stagger 过渡（用于子元素延迟出场） */
export const T_STAGGER = {
  duration: DURATION_NORMAL,
  ease: EASE_ENTER,
} as const

// ── Motion Values ──────────────────────────────────────────────────────────
// 最大允许的位移 / 缩放边界

/** 垂直位移上限 (px) — 页面过渡使用 */
export const MAX_Y_OFFSET = 6

/** hover 缩放上限 — 不超过 1.02 */
export const HOVER_SCALE = 1.015

/** press 缩放 — 轻微下压 */
export const TAP_SCALE = 0.985

// ── Variant Factories ──────────────────────────────────────────────────────

/**
 * 页面淡入变体
 * 设计意图：轻量 opacity + 极小 y 位移，营造内容从下方"浮现"的空间连续性。
 * 心理层面：给用户"内容已就绪，稳定呈现"的信任感。
 */
export const pageVariants = {
  initial: { opacity: 0, y: MAX_Y_OFFSET },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
} as const

/**
 * 淡入变体（通用）
 */
export const fadeVariants = {
  initial: { opacity: 0 },
  animate: { opacity: 1 },
  exit: { opacity: 0 },
} as const

/**
 * 列表容器变体（stagger children）
 */
export const staggerContainer = {
  animate: {
    transition: {
      staggerChildren: 0.04,
      delayChildren: 0.02,
    },
  },
} as const

/**
 * 列表子项变体
 */
export const staggerItem = {
  initial: { opacity: 0, y: 4 },
  animate: { opacity: 1, y: 0, transition: T_STAGGER },
  exit: { opacity: 0, y: -4, transition: { duration: DURATION_INSTANT, ease: EASE_EXIT } },
} as const

/**
 * 卡片 hover 变体
 * 设计意图：极微缩放 + shadow 提升，暗示可交互性。
 * 心理层面：物理世界中"轻微悬浮"的隐喻，不打断用户阅读。
 */
export const cardHover = {
  rest: { scale: 1 },
  hover: { scale: HOVER_SCALE },
} as const

/**
 * 展开/收起变体
 */
export const collapseVariants = {
  collapsed: { height: 0, opacity: 0, overflow: 'hidden' as const },
  expanded: { height: 'auto', opacity: 1, overflow: 'hidden' as const },
} as const
