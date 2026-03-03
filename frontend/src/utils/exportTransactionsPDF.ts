import type { Transaction } from '../api/client'
import { formatAmount, toCNY } from './format'
import { FALLBACK_RATES } from './exchangeRates'
import i18n from '../i18n'
import { categoryLabel } from './categoryLabel'

function fmt(t: Transaction) {
  return formatAmount(t.amount_yuan, t.currency)
}

function fmtTotal(n: number) {
  return formatAmount(n, 'CNY')
}

export function exportTransactionsPDF(
  filtered: Transaction[],
  filterLabel: string,
  user: { username: string; email: string; role: string } | null,
  rates: Record<string, number> = FALLBACK_RATES,
) {
  const now = new Date()
  const dateStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`

  // ── 按来源分组统计 ──
  const personal = filtered.filter(t => t.source !== 'company')
  const company  = filtered.filter(t => t.source === 'company')

  function calcStats(txs: Transaction[]) {
    const inc = txs.filter(t => t.direction === 'income').reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
    const exp = txs.filter(t => t.direction === 'expense').reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
    const reimb = txs.filter(t => t.direction === 'expense' && t.reimbursed).reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
    const net = inc - exp + reimb
    return { count: txs.length, inc, exp, reimb, net }
  }

  const allStats = calcStats(filtered)
  const pStats   = calcStats(personal)
  const cStats   = calcStats(company)

  function workflowStatus(t: Transaction) {
    if (t.direction === 'income') {
      return {
        text: i18n.t('exportPdf.workflow.incomeNoFlow'),
        className: 'wf-income',
      }
    }

    if (t.source === 'company') {
      if (!t.uploaded) return { text: i18n.t('exportPdf.workflow.company.pendingUpload'), className: 'wf-pending' }
      if (!t.reimbursed) return { text: i18n.t('exportPdf.workflow.company.pendingReimburse'), className: 'wf-review' }
      return { text: i18n.t('exportPdf.workflow.company.done'), className: 'wf-done' }
    }

    if (!t.uploaded) return { text: i18n.t('exportPdf.workflow.life.pendingUpload'), className: 'wf-pending' }
    if (!t.reimbursed) return { text: i18n.t('exportPdf.workflow.life.pendingProcess'), className: 'wf-review' }
    return { text: i18n.t('exportPdf.workflow.life.done'), className: 'wf-done' }
  }

  const rows = filtered.map(t => {
    const src = t.source === 'company' ? i18n.t('exportPdf.companyLabel') : i18n.t('exportPdf.personalLabel')
    const amount = `${t.direction === 'income' ? '+' : '−'}${fmt(t)}`
    const amtColor = t.direction === 'income' ? '#16a34a' : '#ef4444'
    const uploaded = t.uploaded ? i18n.t('exportPdf.uploadedYes') : i18n.t('exportPdf.uploadedNo')
    const uploadedColor = t.uploaded ? '#7c3aed' : '#9ca3af'
    const reimbursed = t.reimbursed ? i18n.t('exportPdf.reimbursedYes') : i18n.t('exportPdf.reimbursedNo')
    const reimbursedColor = t.reimbursed ? '#15803d' : '#9ca3af'
    const dotClass = t.direction === 'income' ? 'dot income' : 'dot expense'
    const workflow = workflowStatus(t)
    return [
      '<tr>',
      `<td>${t.occurred_at}</td>`,
      `<td><span class="${dotClass}"></span>${categoryLabel(t.category)}</td>`,
      `<td>${src}</td>`,
      `<td>${t.project_id ?? '—'}</td>`,
      `<td class="note">${t.note || '—'}</td>`,
      `<td style="color:${amtColor};font-weight:700;text-align:right">${amount}</td>`,
      `<td style="color:${uploadedColor};text-align:center">${uploaded}</td>`,
      `<td style="color:${reimbursedColor};text-align:center">${reimbursed}</td>`,
      `<td style="text-align:center"><span class="wf-chip ${workflow.className}">${workflow.text}</span></td>`,
      '</tr>',
    ].join('')
  }).join('')

  const head = [
    '<!DOCTYPE html>',
    `<html lang="${i18n.language === 'en' ? 'en' : 'zh-CN'}">`,
    '<head>',
    '<meta charset="UTF-8">',
    `<title>${i18n.t('exportPdf.title')} ${dateStr}</title>`,
    '<style>',
    '* { box-sizing: border-box; margin: 0; padding: 0; }',
    'body { font-family: "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif; font-size: 11px; color: #1f2937; background: #fff; padding: 24px 32px; }',
    '.header { border-bottom: 2px solid #7c3aed; padding-bottom: 14px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-end; }',
    '.header-left { display:flex; align-items:center; gap:12px; }',
    '.header-left h1 { font-size: 20px; font-weight: 800; color: #5b21b6; letter-spacing: 0.02em; }',
    '.header-left p { font-size: 11px; color: #6b7280; margin-top: 3px; }',
    '.header-right { text-align: right; }',
    '.header-right .user-name { font-size: 14px; font-weight: 700; color: #1f2937; }',
    '.header-right .user-detail { font-size: 10px; color: #9ca3af; margin-top: 2px; }',
    '.summary { margin-bottom: 16px; display: grid; gap: 10px; }',
    '.summary-table { width: 100%; border-collapse: collapse; margin-bottom: 0; border: 1px solid #ede9fe; border-radius: 12px; overflow: hidden; }',
    '.summary-table th { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: #9ca3af; padding: 6px 10px; text-align: right; border-bottom: 1px solid #e5e7eb; font-weight: 600; }',
    '.summary-table th:first-child { text-align: left; }',
    '.summary-table td { padding: 8px 10px; font-size: 13px; font-weight: 800; font-variant-numeric: tabular-nums; text-align: right; border-bottom: 1px solid #f3f4f6; }',
    '.summary-table td:first-child { text-align: left; font-size: 12px; font-weight: 700; color: #1f2937; }',
    '.summary-table tr:last-child td { border-bottom: none; }',
    '.summary-table .row-all td:first-child { color: #5b21b6; }',
    '.summary-table .row-personal td:first-child { color: #d97706; }',
    '.summary-table .row-company td:first-child { color: #0284c7; }',
    '.summary-table .income { color: #16a34a; }',
    '.summary-table .expense { color: #ef4444; }',
    '.summary-table .reimb { color: #7c3aed; }',
    '.summary-table .count { color: #7c3aed; }',
    '.status-note { display:flex; gap:8px; flex-wrap:wrap; }',
    '.status-pill { font-size:10px; padding:4px 8px; border-radius:999px; font-weight:700; }',
    '.status-pill.pending { color:#b45309; background:#fef3c7; }',
    '.status-pill.review { color:#6d28d9; background:#ede9fe; }',
    '.status-pill.done { color:#166534; background:#dcfce7; }',
    'table { width: 100%; border-collapse: collapse; font-size: 10.5px; border: 1px solid #ede9fe; border-radius: 12px; overflow: hidden; }',
    'thead tr { background: #5b21b6; color: #fff; }',
    'thead th { padding: 8px 10px; text-align: left; font-weight: 600; white-space: nowrap; }',
    'thead th:last-child, thead th:nth-child(6) { text-align: center; }',
    'thead th:nth-child(6) { text-align: right; }',
    'tbody tr { border-bottom: 1px solid #f3f4f6; }',
    'tbody tr:nth-child(even) { background: #f9fafb; }',
    'tbody td { padding: 6px 10px; vertical-align: middle; }',
    'td.note { max-width: 140px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: #6b7280; }',
    '.wf-chip { display:inline-flex; align-items:center; justify-content:center; border-radius:999px; padding:3px 8px; font-size:9.5px; font-weight:700; white-space:nowrap; }',
    '.wf-chip.wf-income { background:#e5e7eb; color:#4b5563; }',
    '.wf-chip.wf-pending { background:#fef3c7; color:#92400e; }',
    '.wf-chip.wf-review { background:#ede9fe; color:#6d28d9; }',
    '.wf-chip.wf-done { background:#dcfce7; color:#166534; }',
    '.dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 5px; vertical-align: middle; }',
    '.dot.income { background: #22c55e; }',
    '.dot.expense { background: #ef4444; }',
    '.footer { margin-top: 16px; padding-top: 10px; border-top: 1px solid #e5e7eb; display: flex; justify-content: space-between; font-size: 9px; color: #9ca3af; }',
    '@media print {',
    '  body { padding: 10px 16px; }',
    '  @page { size: A4 landscape; margin: 10mm; }',
    '}',
    '</style>',
    '</head>',
    '<body>',
  ].join('\n')

  function netStyle(n: number) {
    return n >= 0 ? '#16a34a' : '#ef4444'
  }
  function netFmt(n: number) {
    return (n >= 0 ? '+' : '') + fmtTotal(n)
  }

  function summaryRow(label: string, cls: string, s: { count: number; inc: number; exp: number; reimb: number; net: number }) {
    return [
      `<tr class="${cls}">`,
      `  <td>${label}</td>`,
      `  <td class="count">${s.count} ${i18n.t('exportPdf.unit')}</td>`,
      `  <td class="income">${fmtTotal(s.inc)}</td>`,
      `  <td class="expense">${fmtTotal(s.exp)}</td>`,
      `  <td class="reimb">+${fmtTotal(s.reimb)}</td>`,
      `  <td style="color:${netStyle(s.net)}">${netFmt(s.net)}</td>`,
      '</tr>',
    ].join('')
  }

  const body = [
    '<div class="header">',
    '  <div class="header-left">',
    '  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="40" height="40" style="border-radius:10px;flex-shrink:0">',
    '    <defs><linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" stop-color="#0d0b14"/><stop offset="100%" stop-color="#170f26"/></linearGradient></defs>',
    '    <rect width="512" height="512" rx="104" ry="104" fill="url(#bg)"/>',
    '    <rect x="64"  y="272" width="112" height="144" rx="24" fill="#818cf8"/>',
    '    <rect x="200" y="192" width="112" height="224" rx="24" fill="#a78bfa"/>',
    '    <rect x="336" y="96"  width="112" height="320" rx="24" fill="#34d399"/>',
    '  </svg>',
    '  <div>',
    `    <h1>${i18n.t('exportPdf.systemTitle')}</h1>`,
    `    <p>${i18n.t('exportPdf.filter')}：${filterLabel} &nbsp;|&nbsp; ${i18n.t('exportPdf.exportDate')}：${dateStr}</p>`,
    '  </div>',
    '  </div>',
    '  <div class="header-right">',
    `    <div class="user-name">${user?.username ?? '—'}</div>`,
    `    <div class="user-detail">${user?.email ?? ''}</div>`,
    '  </div>',
    '</div>',
    '<div class="summary">',
    '  <table class="summary-table">',
    `    <thead><tr><th></th><th>${i18n.t('exportPdf.recordCount')}</th><th>${i18n.t('exportPdf.incomeTotal')}</th><th>${i18n.t('exportPdf.expenseTotal')}</th><th>${i18n.t('exportPdf.reimbursedTotal')}</th><th>${i18n.t('exportPdf.netTotal')}</th></tr></thead>`,
    '    <tbody>',
    summaryRow(i18n.t('exportPdf.allLabel'), 'row-all', allStats),
    summaryRow(i18n.t('exportPdf.personalLabel'), 'row-personal', pStats),
    summaryRow(i18n.t('exportPdf.companyLabel'), 'row-company', cStats),
    '    </tbody>',
    '  </table>',
    '  <div class="status-note">',
    `    <span class="status-pill pending">${i18n.t('exportPdf.workflowLegend.pending')}</span>`,
    `    <span class="status-pill review">${i18n.t('exportPdf.workflowLegend.review')}</span>`,
    `    <span class="status-pill done">${i18n.t('exportPdf.workflowLegend.done')}</span>`,
    '  </div>',
    '</div>',
    '<table>',
    '  <thead>',
    '    <tr>',
    '      <th>' + i18n.t('exportPdf.thDate') + '</th><th>' + i18n.t('exportPdf.thCategory') + '</th><th>' + i18n.t('exportPdf.thSource') + '</th><th>' + i18n.t('exportPdf.thProject') + '</th><th>' + i18n.t('exportPdf.thNote') + '</th><th>' + i18n.t('exportPdf.thAmount') + '</th><th>' + i18n.t('exportPdf.thUploaded') + '</th><th>' + i18n.t('exportPdf.thReimbursed') + '</th><th>' + i18n.t('exportPdf.thWorkflow') + '</th>',
    '    </tr>',
    '  </thead>',
    `  <tbody>${rows}</tbody>`,
    '</table>',
    '<div class="footer">',
    `  <span>${i18n.t('exportPdf.footer')} · ${now.toLocaleString(i18n.language === 'en' ? 'en-US' : 'zh-CN')}</span>`,
    `  <span>${i18n.t('exportPdf.totalRecords', { count: allStats.count })}</span>`,
    '</div>',
    '</body>',
    '</html>',
  ].join('\n')

  const html = head + '\n' + body

  const win = window.open('', '_blank', 'width=1100,height=800')
  if (!win) {
    alert(i18n.t('exportPdf.popupBlocked'))
    return
  }
  win.document.write(html)
  win.document.close()
  setTimeout(() => win.print(), 400)
}
