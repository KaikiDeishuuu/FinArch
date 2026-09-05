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
    const amtColor = t.direction === 'income' ? '#28745b' : '#b95642'
    const uploaded = t.uploaded ? i18n.t('exportPdf.uploadedYes') : i18n.t('exportPdf.uploadedNo')
    const uploadedColor = t.uploaded ? '#2d6687' : '#77867e'
    const reimbursed = t.reimbursed ? i18n.t('exportPdf.reimbursedYes') : i18n.t('exportPdf.reimbursedNo')
    const reimbursedColor = t.reimbursed ? '#28745b' : '#77867e'
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
    'body { font-family: "Avenir Next", "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; font-size: 11px; color: #17201d; background: #fff; padding: 24px 32px; }',
    '.header { border-bottom: 2px solid #2d6687; padding-bottom: 14px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-end; }',
    '.header-left { display:flex; align-items:center; gap:12px; }',
    '.header-left h1 { font-family: Georgia, "Songti SC", serif; font-size: 20px; font-weight: 700; color: #17201d; letter-spacing: -0.02em; }',
    '.header-left p { font-size: 11px; color: #5b6b63; margin-top: 3px; }',
    '.header-right { text-align: right; }',
    '.header-right .user-name { font-size: 14px; font-weight: 700; color: #17201d; }',
    '.header-right .user-detail { font-size: 10px; color: #77867e; margin-top: 2px; }',
    '.summary { margin-bottom: 16px; display: grid; gap: 10px; }',
    '.summary-table { width: 100%; border-collapse: collapse; margin-bottom: 0; border: 1px solid #cbd5ce; border-radius: 8px; overflow: hidden; }',
    '.summary-table thead tr { background:#e7ede8; color:#17201d; }',
    '.summary-table th { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: #5b6b63; padding: 6px 10px; text-align: right; border-bottom: 1px solid #cbd5ce; font-weight: 600; }',
    '.summary-table th:first-child { text-align: left; }',
    '.summary-table td { padding: 8px 10px; font-family: "SFMono-Regular", Consolas, monospace; font-size: 13px; font-weight: 700; font-variant-numeric: tabular-nums; text-align: right; border-bottom: 1px solid #e7ede8; }',
    '.summary-table td:first-child { text-align: left; font-family: inherit; font-size: 12px; font-weight: 700; color: #17201d; }',
    '.summary-table tr:last-child td { border-bottom: none; }',
    '.summary-table .row-all td:first-child { color: #17201d; }',
    '.summary-table .row-personal td:first-child { color: #28745b; }',
    '.summary-table .row-company td:first-child { color: #2d6687; }',
    '.summary-table .income { color: #28745b; }',
    '.summary-table .expense { color: #b95642; }',
    '.summary-table .reimb { color: #2d6687; }',
    '.summary-table .count { color: #2d6687; }',
    '.status-note { display:flex; gap:8px; flex-wrap:wrap; }',
    '.status-pill { font-size:10px; padding:4px 8px; border-radius:999px; font-weight:700; }',
    '.status-pill.pending { color:#8a452f; background:#f4e4df; }',
    '.status-pill.review { color:#2d6687; background:#e5eef3; }',
    '.status-pill.done { color:#28745b; background:#e3eee9; }',
    'table { width: 100%; border-collapse: collapse; font-size: 10.5px; border: 1px solid #cbd5ce; border-radius: 8px; overflow: hidden; }',
    'thead tr { background: #17201d; color: #f3f6f2; }',
    'thead th { padding: 8px 10px; text-align: left; font-weight: 600; white-space: nowrap; }',
    'thead th:last-child, thead th:nth-child(7) { text-align: center; }',
    'thead th:nth-child(7) { text-align: right; }',
    'tbody tr { border-bottom: 1px solid #e7ede8; }',
    'tbody tr, tbody td { page-break-inside: avoid; break-inside: avoid; }',
    'tbody tr:nth-child(even) { background: #f3f6f2; }',
    'tbody td { padding: 6px 10px; vertical-align: middle; }',
    'td.note { max-width: 120px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: #5b6b63; }',
    '.tx-row { page-break-inside: avoid; break-inside: avoid; }',
    '.lifecycle { margin-top: 4px; display:flex; flex-direction:column; gap:2px; font-size:9px; color:#5b6b63; line-height:1.2; white-space:normal; }',
    '.wf-chip { display:inline-flex; align-items:center; justify-content:center; border-radius:999px; padding:3px 8px; font-size:9.5px; font-weight:700; white-space:nowrap; }',
    '.wf-chip.wf-income { background:#e7ede8; color:#5b6b63; }',
    '.wf-chip.wf-pending { background:#f4e4df; color:#8a452f; }',
    '.wf-chip.wf-review { background:#e5eef3; color:#2d6687; }',
    '.wf-chip.wf-done { background:#e3eee9; color:#28745b; }',
    '.dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 5px; vertical-align: middle; }',
    '.dot.income { background: #28745b; }',
    '.dot.expense { background: #b95642; }',
    '.footer { margin-top: 16px; padding-top: 10px; border-top: 1px solid #cbd5ce; display: flex; justify-content: space-between; font-size: 9px; color: #77867e; }',
    '@media print {',
    '  body { padding: 10px 16px; }',
    '  @page { size: A4 landscape; margin: 10mm; }',
    '}',
    '</style>',
    '</head>',
    '<body>',
  ].join('\n')

  function netStyle(n: number) {
    return n >= 0 ? '#28745b' : '#b95642'
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
    '  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 200" width="40" height="40" style="border-radius:4px;flex-shrink:0">',
    '    <rect width="200" height="200" rx="18" fill="#17201d"/>',
    '    <path d="M38 34v130h130" fill="none" stroke="#cbd5ce" stroke-width="4" opacity=".72"/>',
    '    <path d="M38 58h12M38 84h8M38 110h12M38 136h8" stroke="#cbd5ce" stroke-width="3" opacity=".8"/>',
    '    <rect x="59" y="112" width="25" height="52" rx="3" fill="#2d6687"/>',
    '    <rect x="99" y="82" width="25" height="82" rx="3" fill="#28745b"/>',
    '    <rect x="139" y="49" width="25" height="115" rx="3" fill="#b95642"/>',
    '    <path d="M59 103h25M99 73h25M139 40h25" stroke="#f3f6f2" stroke-width="3" opacity=".9"/>',
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
