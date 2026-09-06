/**
 * AnnouncementBoard — 公告板
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 一条常驻在概览页顶部的欢迎公告，附带支持邮箱。
 *
 * 关掉之后记在本地（见 utils/announcement）。设置页的「重新显示公告」按钮
 * 走 restoreAnnouncement() 把记录清掉；因为概览页每次进入都会重新挂载本组件、
 * 重新读一次本地记录，所以清掉之后用户切回概览就能看到，不需要额外的跨组件通知。
 */
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Megaphone, X } from 'lucide-react'

import { SUPPORT_EMAIL } from '../constants/app'
import { dismissAnnouncement, isAnnouncementDismissed } from '../utils/announcement'
import { Card } from './ui/card'

export default function AnnouncementBoard() {
  const { t } = useTranslation()
  const [dismissed, setDismissed] = useState(isAnnouncementDismissed)

  if (dismissed) return null

  function dismiss() {
    setDismissed(true)
    dismissAnnouncement()
  }

  return (
    <Card aria-labelledby="announcement-title" className="border-accent/35 bg-accent-soft">
      <div className="flex items-start gap-3">
        <span
          aria-hidden="true"
          className="grid size-8 shrink-0 place-items-center rounded-md bg-card text-accent ring-1 ring-accent/25"
        >
          <Megaphone className="size-4" />
        </span>

        <div className="min-w-0 flex-1">
          <h3 id="announcement-title" className="text-sm font-semibold tracking-tight text-foreground">
            {t('announcement.title')}
          </h3>
          <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
            {t('announcement.body')}
          </p>
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            {t('announcement.support')}{' '}
            <a
              href={`mailto:${SUPPORT_EMAIL}`}
              className="font-medium text-accent underline underline-offset-2 hover:no-underline"
            >
              {SUPPORT_EMAIL}
            </a>
          </p>
          <p className="mt-2 text-[11px] leading-relaxed text-muted-foreground/80">
            {t('announcement.dismissHint')}
          </p>
        </div>

        <button
          type="button"
          onClick={dismiss}
          title={t('announcement.dismiss')}
          aria-label={t('announcement.dismiss')}
          className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-card hover:text-foreground"
        >
          <X className="size-4" />
        </button>
      </div>
    </Card>
  )
}
