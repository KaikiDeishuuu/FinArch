import type { Transaction } from '../api/client'
import { formatAmount } from './format'
import { transactionAmountToCNY } from './financeAmounts'
import { FALLBACK_RATES } from './exchangeRates'
import i18n from '../i18n'
import { categoryLabel } from './categoryLabel'
import { clampLifecycleTimestamp } from './timestamp'
import {
  transactionWorkflowStage,
  type TransactionWorkflowKind,
} from './transactionWorkflow'

function fmt(t: Transaction) {
  return formatAmount(t.amount_yuan, t.currency)
}

function fmtTotal(n: number) {
  return formatAmount(n, 'CNY')
}

function escapeHtml(value: unknown) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function exportTransactionsPDF(
  filtered: Transaction[],
  filterLabel: string,
  user: { username: string; email: string; role: string } | null,
  rates: Record<string, number> = FALLBACK_RATES,
  workflowKind: TransactionWorkflowKind = 'reimbursement',
  accountMap: Record<string, string> = {},
) {
  const now = new Date()
  const dateStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  const e = escapeHtml

  // ── 按来源分组统计 ──
  const personal = filtered.filter(t => t.source !== 'company')
  const company = filtered.filter(t => t.source === 'company')

  function calcStats(txs: Transaction[]) {
    const inc = txs.filter(t => t.direction === 'income').reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0)
    const exp = txs.filter(t => t.direction === 'expense').reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0)
    const reimb = txs.filter(t => t.direction === 'expense' && t.reimbursed).reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0)
    const net = inc - exp + (workflowKind === 'reimbursement' ? reimb : 0)
    return { count: txs.length, inc, exp, reimb, net }
  }

  const allStats = calcStats(filtered)
  const pStats = calcStats(personal)
  const cStats = calcStats(company)

  const isReimbursementView = workflowKind === 'reimbursement'

  function workflowStatus(t: Transaction) {
    switch (transactionWorkflowStage(t, workflowKind)) {
      case 'income':
        return { text: i18n.t('exportPdf.workflow.incomeNoFlow'), className: 'wf-income' }
      case 'pending-upload':
        return { text: i18n.t('exportPdf.workflow.pendingUpload'), className: 'wf-pending' }
      case 'pending-reimbursement':
        return { text: i18n.t('exportPdf.workflow.pendingReimbursement'), className: 'wf-review' }
      case 'reimbursed':
        return { text: i18n.t('exportPdf.workflow.reimbursed'), className: 'wf-done' }
      case 'pending-settlement':
        return { text: i18n.t('exportPdf.workflow.pendingSettlement'), className: 'wf-review' }
      case 'settled':
        return { text: i18n.t('exportPdf.workflow.settled'), className: 'wf-done' }
      case 'uploaded':
        return { text: i18n.t('exportPdf.workflow.uploaded'), className: 'wf-done' }
    }
  }

  const workflowLegend = isReimbursementView
    ? [
        { className: 'pending', text: i18n.t('exportPdf.workflow.pendingUpload') },
        { className: 'review', text: i18n.t('exportPdf.workflow.pendingReimbursement') },
        { className: 'done', text: i18n.t('exportPdf.workflow.reimbursed') },
      ]
    : workflowKind === 'settlement'
      ? [
          { className: 'pending', text: i18n.t('exportPdf.workflow.pendingUpload') },
          { className: 'review', text: i18n.t('exportPdf.workflow.pendingSettlement') },
          { className: 'done', text: i18n.t('exportPdf.workflow.settled') },
        ]
      : [
          { className: 'pending', text: i18n.t('exportPdf.workflow.pendingUpload') },
          { className: 'done', text: i18n.t('exportPdf.workflow.uploaded') },
        ]

  const ordered = [...filtered].sort((a, b) => {
    const ta = a.transaction_time ?? 0
    const tb = b.transaction_time ?? 0
    if (ta !== tb) return ta - tb
    return a.id.localeCompare(b.id)
  })

  const rows = ordered.map(t => {
    const src = t.source === 'company' ? i18n.t('exportPdf.companyLabel') : i18n.t('exportPdf.personalLabel')
    const acctName = (t.account_id && accountMap[t.account_id]) ? accountMap[t.account_id] : '—'
    const amount = `${t.direction === 'income' ? '+' : '−'}${fmt(t)}`
    const amtColor = t.direction === 'income' ? '#1B7F4C' : '#C93A3F'
    const uploaded = t.uploaded ? i18n.t('exportPdf.uploadedYes') : i18n.t('exportPdf.uploadedNo')
    const uploadedColor = t.uploaded ? '#2F5FD6' : '#8A929F'
    const reimbursed = t.reimbursed ? i18n.t('exportPdf.reimbursedYes') : i18n.t('exportPdf.reimbursedNo')
    const reimbursedColor = t.reimbursed ? '#1B7F4C' : '#8A929F'
    const dotClass = t.direction === 'income' ? 'dot income' : 'dot expense'
    const workflow = workflowStatus(t)
    const reportedAt = clampLifecycleTimestamp(t.reported_at, t.created_at) ?? '—'
    const reimbursedAt = clampLifecycleTimestamp(t.reimbursed_at, t.created_at) ?? '—'
    const lifecycle = [
      `<span>${e(i18n.t('exportPdf.reportedAt'))} ${e(reportedAt)}</span>`,
      ...(isReimbursementView
        ? [`<span>${e(i18n.t('exportPdf.reimbursedAt'))} ${e(reimbursedAt)}</span>`]
        : []),
    ].join('')
    return [
      '<tr class="tx-row">',
      `<td>${e(t.occurred_at)}</td>`,
      `<td><span class="${dotClass}"></span>${e(categoryLabel(t.category))}</td>`,
      `<td>${e(src)}</td>`,
      `<td class="note">${e(acctName)}</td>`,
      `<td>${e(t.project_id ?? '—')}</td>`,
      `<td class="note">${e(t.note || '—')}<div class="lifecycle">${lifecycle}</div></td>`,
      `<td style="color:${amtColor};font-weight:700;text-align:right">${e(amount)}</td>`,
      `<td style="color:${uploadedColor};text-align:center">${e(uploaded)}</td>`,
      ...(isReimbursementView
        ? [`<td style="color:${reimbursedColor};text-align:center">${e(reimbursed)}</td>`]
        : []),
      `<td style="text-align:center"><span class="wf-chip ${workflow.className}">${e(workflow.text)}</span></td>`,
      '</tr>',
    ].join('')
  }).join('')

  const head = [
    '<!DOCTYPE html>',
    `<html lang="${i18n.language === 'en' ? 'en' : 'zh-CN'}">`,
    '<head>',
    '<meta charset="UTF-8">',
    `<title>${e(i18n.t('exportPdf.title'))} ${e(dateStr)}</title>`,
    '<style>',
    '* { box-sizing: border-box; margin: 0; padding: 0; }',
    'body { font-family: "Geist", "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif; font-size: 11px; color: #161A22; background: #fff; padding: 24px 32px; }',
    '.header { border-bottom: 1px solid #E2E6EC; padding-bottom: 14px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-end; }',
    '.header-left { display:flex; align-items:center; gap:12px; }',
    '.header-left h1 { font-size: 20px; font-weight: 700; color: #161A22; letter-spacing: -0.02em; }',
    '.header-left p { font-size: 11px; color: #5D6572; margin-top: 3px; }',
    '.header-right { text-align: right; }',
    '.header-right .user-name { font-size: 14px; font-weight: 700; color: #161A22; }',
    '.header-right .user-detail { font-size: 10px; color: #8A929F; margin-top: 2px; }',
    '.summary { margin-bottom: 16px; display: grid; gap: 10px; }',
    '.summary-table { width: 100%; border-collapse: collapse; margin-bottom: 0; border: 1px solid #E2E6EC; border-radius: 10px; overflow: hidden; }',
    '.summary-table th { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: #8A929F; padding: 6px 10px; text-align: right; border-bottom: 1px solid #E2E6EC; font-weight: 600; }',
    '.summary-table th:first-child { text-align: left; }',
    '.summary-table td { padding: 8px 10px; font-size: 13px; font-weight: 700; font-variant-numeric: tabular-nums; text-align: right; border-bottom: 1px solid #EEF1F5; }',
    '.summary-table td:first-child { text-align: left; font-size: 12px; font-weight: 700; color: #161A22; }',
    '.summary-table tr:last-child td { border-bottom: none; }',
    '.summary-table .row-all td:first-child { color: #2F5FD6; }',
    '.summary-table .row-personal td:first-child { color: #9A5B00; }',
    '.summary-table .row-company td:first-child { color: #2F5FD6; }',
    '.summary-table .income { color: #1B7F4C; }',
    '.summary-table .expense { color: #C93A3F; }',
    '.summary-table .reimb { color: #2F5FD6; }',
    '.summary-table .count { color: #2F5FD6; }',
    '.status-note { display:flex; gap:8px; flex-wrap:wrap; }',
    '.status-pill { font-size:10px; padding:4px 8px; border-radius:999px; font-weight:700; }',
    '.status-pill.pending { color:#9A5B00; background:#FBF0DC; }',
    '.status-pill.review { color:#2F5FD6; background:#E8EEFB; }',
    '.status-pill.done { color:#1B7F4C; background:#E4F4EB; }',
    'table { width: 100%; border-collapse: collapse; font-size: 10.5px; border: 1px solid #E2E6EC; border-radius: 10px; overflow: hidden; }',
    'thead tr { background: #161A22; color: #fff; }',
    'thead th { padding: 8px 10px; text-align: left; font-weight: 600; white-space: nowrap; }',
    'thead th:last-child, thead th:nth-child(7) { text-align: center; }',
    'thead th:nth-child(7) { text-align: right; }',
    'tbody tr { border-bottom: 1px solid #EEF1F5; }',
    'tbody tr, tbody td { page-break-inside: avoid; break-inside: avoid; }',
    'tbody tr:nth-child(even) { background: #F6F7F9; }',
    'tbody td { padding: 6px 10px; vertical-align: middle; }',
    'td.note { max-width: 120px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: #5D6572; }',
    '.tx-row { page-break-inside: avoid; break-inside: avoid; }',
    '.lifecycle { margin-top: 4px; display:flex; flex-direction:column; gap:2px; font-size:9px; color:#5D6572; line-height:1.2; white-space:normal; }',
    '.wf-chip { display:inline-flex; align-items:center; justify-content:center; border-radius:999px; padding:3px 8px; font-size:9.5px; font-weight:700; white-space:nowrap; }',
    '.wf-chip.wf-income { background:#EEF1F5; color:#5D6572; }',
    '.wf-chip.wf-pending { background:#FBF0DC; color:#9A5B00; }',
    '.wf-chip.wf-review { background:#E8EEFB; color:#2F5FD6; }',
    '.wf-chip.wf-done { background:#E4F4EB; color:#1B7F4C; }',
    '.dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 5px; vertical-align: middle; }',
    '.dot.income { background: #1B7F4C; }',
    '.dot.expense { background: #C93A3F; }',
    '.footer { margin-top: 16px; padding-top: 10px; border-top: 1px solid #E2E6EC; display: flex; justify-content: space-between; font-size: 9px; color: #8A929F; }',
    '@media print {',
    '  body { padding: 10px 16px; }',
    '  @page { size: A4 landscape; margin: 10mm; }',
    '}',
    '</style>',
    '</head>',
    '<body>',
  ].join('\n')

  function netStyle(n: number) {
    return n >= 0 ? '#1B7F4C' : '#C93A3F'
  }
  function netFmt(n: number) {
    return (n >= 0 ? '+' : '') + fmtTotal(n)
  }

  function summaryRow(label: string, cls: string, s: { count: number; inc: number; exp: number; reimb: number; net: number }) {
    return [
      `<tr class="${cls}">`,
      `  <td>${e(label)}</td>`,
      `  <td class="count">${e(s.count)} ${e(i18n.t('exportPdf.unit'))}</td>`,
      `  <td class="income">${e(fmtTotal(s.inc))}</td>`,
      `  <td class="expense">${e(fmtTotal(s.exp))}</td>`,
      isReimbursementView ? `  <td class="reimb">+${e(fmtTotal(s.reimb))}</td>` : '',
      `  <td style="color:${netStyle(s.net)}">${e(netFmt(s.net))}</td>`,
      '</tr>',
    ].join('')
  }

  const body = [
    '<div class="header">',
    '  <div class="header-left">',
    '  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="40" height="40" style="border-radius:10px;flex-shrink:0">',
    '',
    '    <rect width="512" height="512" rx="104" ry="104" fill="#161A22"/>',
    '    <rect x="64"  y="272" width="112" height="144" rx="24" fill="#6B7A99"/>',
    '    <rect x="200" y="192" width="112" height="224" rx="24" fill="#3D6BE8"/>',
    '    <rect x="336" y="96"  width="112" height="320" rx="24" fill="#2FB37C"/>',
    '  </svg>',
    '  <div>',
    `    <h1>${e(i18n.t('exportPdf.systemTitle'))}</h1>`,
    `    <p>${e(i18n.t('exportPdf.filter'))}：${e(filterLabel)} &nbsp;|&nbsp; ${e(i18n.t('exportPdf.exportDate'))}：${e(dateStr)}</p>`,
    '  </div>',
    '  </div>',
    '  <div class="header-right">',
    `    <div class="user-name">${e(user?.username ?? '—')}</div>`,
    `    <div class="user-detail">${e(user?.email ?? '')}</div>`,
    '  </div>',
    '</div>',
    '<div class="summary">',
    '  <table class="summary-table">',
    `    <thead><tr><th></th><th>${e(i18n.t('exportPdf.recordCount'))}</th><th>${e(i18n.t('exportPdf.incomeTotal'))}</th><th>${e(i18n.t('exportPdf.expenseTotal'))}</th>${isReimbursementView ? `<th>${e(i18n.t('exportPdf.reimbursedTotal'))}</th>` : ''}<th>${e(i18n.t('exportPdf.netTotal'))}</th></tr></thead>`,
    '    <tbody>',
    summaryRow(i18n.t('exportPdf.allLabel'), 'row-all', allStats),
    summaryRow(i18n.t('exportPdf.personalLabel'), 'row-personal', pStats),
    summaryRow(i18n.t('exportPdf.companyLabel'), 'row-company', cStats),
    '    </tbody>',
    '  </table>',
    '  <div class="status-note">',
    ...workflowLegend.map(({ className, text }) =>
      `    <span class="status-pill ${className}">${e(text)}</span>`),
    '  </div>',
    '</div>',
    '<table>',
    '  <thead>',
    '    <tr>',
    '      <th>' + e(i18n.t('exportPdf.thDate')) + '</th><th>' + e(i18n.t('exportPdf.thCategory')) + '</th><th>' + e(i18n.t('exportPdf.thSource')) + '</th><th>' + e(i18n.t('exportPdf.thAccount')) + '</th><th>' + e(i18n.t('exportPdf.thProject')) + '</th><th>' + e(i18n.t('exportPdf.thNote')) + '</th><th>' + e(i18n.t('exportPdf.thAmount')) + '</th><th>' + e(i18n.t('exportPdf.thUploaded')) + '</th>' + (isReimbursementView ? '<th>' + e(i18n.t('exportPdf.thReimbursed')) + '</th>' : '') + '<th>' + e(i18n.t('exportPdf.thWorkflow')) + '</th>',
    '    </tr>',
    '  </thead>',
    `  <tbody>${rows}</tbody>`,
    '</table>',
    '<div class="footer">',
    `  <span>${e(i18n.t('exportPdf.footer'))} · ${e(now.toLocaleString(i18n.language === 'en' ? 'en-US' : 'zh-CN'))}</span>`,
    `  <span>${e(i18n.t('exportPdf.totalRecords', { count: allStats.count }))}</span>`,
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
