/**
 * 公告板的"已读"记录。
 *
 * 按 ANNOUNCEMENT_ID 存：换公告文案时把 ID 一起改掉，已经关过旧公告的人
 * 会重新看到新的那条。localStorage 在隐私窗口、缩略图截取等场景下读写本身
 * 就可能抛异常，所以两端都包了 try/catch —— 读不到就当成"没关过"照常显示，
 * 宁可多显示一次，也不要因为存储不可用把联系方式彻底藏起来。
 */
import { ANNOUNCEMENT_ID } from '../constants/app'

const STORAGE_KEY = 'finarch_announcement_dismissed'

export function isAnnouncementDismissed() {
  try {
    return localStorage.getItem(STORAGE_KEY) === ANNOUNCEMENT_ID
  } catch {
    return false
  }
}

export function dismissAnnouncement() {
  try {
    localStorage.setItem(STORAGE_KEY, ANNOUNCEMENT_ID)
  } catch {
    // A viewer who blocks site data just sees the board again next visit.
  }
}

/** Clears the dismissal so the board shows again on the next dashboard visit. */
export function restoreAnnouncement() {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Nothing was stored in the first place, so the board is already showing.
  }
}
