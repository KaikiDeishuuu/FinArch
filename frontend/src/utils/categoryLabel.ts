import i18n from '../i18n'

/**
 * Category keys — the Chinese strings stored in the database.
 * We keep them as the canonical keys for backward compatibility.
 */
export const CATEGORY_KEYS = [
  '耗材', '材料', '设备', '仪器', 'CNC加工', '加工费',
  '差旅', '劳务', '软件', '培训', '会议', '测试', '其他',
] as const

/** Map from Chinese DB value → i18n key suffix */
const KEY_MAP: Record<string, string> = {
  '耗材': 'consumables',
  '材料': 'materials',
  '设备': 'equipment',
  '仪器': 'instruments',
  'CNC加工': 'cnc',
  '加工费': 'processing',
  '差旅': 'travel',
  '劳务': 'labor',
  '软件': 'software',
  '培训': 'training',
  '会议': 'conference',
  '测试': 'testing',
  '其他': 'other',
}

/**
 * Translate a category value (Chinese DB string) to the current locale display label.
 * If the category is not in the known set (e.g. user-defined custom category), return as-is.
 */
export function categoryLabel(dbValue: string): string {
  const key = KEY_MAP[dbValue]
  if (!key) return dbValue // custom category — return as-is
  return i18n.t(`categories.${key}`)
}
