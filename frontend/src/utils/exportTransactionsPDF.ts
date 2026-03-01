import type { Transaction } from '../api/client'
import { formatAmount, toCNY } from './format'
import { FALLBACK_RATES } from './exchangeRates'

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
  const incSum = filtered.filter(t => t.direction === 'income').reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
  const expSum = filtered.filter(t => t.direction === 'expense').reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
  const reimbursedSum = filtered.filter(t => t.direction === 'expense' && t.reimbursed).reduce((s, t) => s + toCNY(t.amount_yuan, t.currency, rates), 0)
  const net = incSum - expSum + reimbursedSum

  const rows = filtered.map(t => {
    const dir = t.direction === 'income' ? '收入' : '支出'
    void dir
    const src = t.source === 'company' ? '公共' : '个人'
    const amount = `${t.direction === 'income' ? '+' : '−'}${fmt(t)}`
    const amtColor = t.direction === 'income' ? '#16a34a' : '#ef4444'
    const uploaded = t.uploaded ? '✓ 已上传' : '✗ 未上传'
    const uploadedColor = t.uploaded ? '#7c3aed' : '#9ca3af'
    const reimbursed = t.reimbursed ? '✓ 已报销' : '✗ 待报销'
    const reimbursedColor = t.reimbursed ? '#15803d' : '#9ca3af'
    const dotClass = t.direction === 'income' ? 'dot income' : 'dot expense'
    return [
      '<tr>',
      `<td>${t.occurred_at}</td>`,
      `<td><span class="${dotClass}"></span>${t.category}</td>`,
      `<td>${src}</td>`,
      `<td>${t.project_id ?? '—'}</td>`,
      `<td class="note">${t.note || '—'}</td>`,
      `<td style="color:${amtColor};font-weight:700;text-align:right">${amount}</td>`,
      `<td style="color:${uploadedColor};text-align:center">${uploaded}</td>`,
      `<td style="color:${reimbursedColor};text-align:center">${reimbursed}</td>`,
      '</tr>',
    ].join('')
  }).join('')

  const head = [
    '<!DOCTYPE html>',
    '<html lang="zh-CN">',
    '<head>',
    '<meta charset="UTF-8">',
    `<title>FinArch 交易明细 ${dateStr}</title>`,
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
    '.summary { display: flex; gap: 16px; margin-bottom: 16px; }',
    '.summary-card { flex: 1; background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 8px; padding: 10px 14px; }',
    '.summary-card .label { font-size: 9px; text-transform: uppercase; letter-spacing: 0.08em; color: #9ca3af; margin-bottom: 4px; }',
    '.summary-card .value { font-size: 15px; font-weight: 800; font-variant-numeric: tabular-nums; }',
    '.summary-card .value.income { color: #16a34a; }',
    '.summary-card .value.expense { color: #ef4444; }',
    '.summary-card .value.count { color: #7c3aed; }',
    'table { width: 100%; border-collapse: collapse; font-size: 10.5px; }',
    'thead tr { background: #5b21b6; color: #fff; }',
    'thead th { padding: 8px 10px; text-align: left; font-weight: 600; white-space: nowrap; }',
    'thead th:last-child, thead th:nth-child(6) { text-align: center; }',
    'thead th:nth-child(6) { text-align: right; }',
    'tbody tr { border-bottom: 1px solid #f3f4f6; }',
    'tbody tr:nth-child(even) { background: #f9fafb; }',
    'tbody td { padding: 6px 10px; vertical-align: middle; }',
    'td.note { max-width: 140px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: #6b7280; }',
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

  const netColor = net >= 0 ? '#16a34a' : '#ef4444'
  const netStr = (net >= 0 ? '+' : '') + fmtTotal(net)

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
    '    <h1>FinArch 财务系统 · 交易明细</h1>',
    `    <p>筛选：${filterLabel} &nbsp;|&nbsp; 导出日期：${dateStr}</p>`,
    '  </div>',
    '  </div>',
    '  <div class="header-right">',
    `    <div class="user-name">${user?.username ?? '—'}</div>`,
    `    <div class="user-detail">${user?.email ?? ''}</div>`,
    '  </div>',
    '</div>',
    '<div class="summary">',
    '  <div class="summary-card">',
    '    <div class="label">记录数</div>',
    `    <div class="value count">${filtered.length} 笔</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">收入合计</div>',
    `    <div class="value income">${fmtTotal(incSum)}</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">支出合计</div>',
    `    <div class="value expense">${fmtTotal(expSum)}</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">已报销</div>',
    `    <div class="value" style="color:#7c3aed">+${fmtTotal(reimbursedSum)}</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">净结余</div>',
    `    <div class="value" style="color:${netColor}">${netStr}</div>`,
    '  </div>',
    '</div>',
    '<table>',
    '  <thead>',
    '    <tr>',
    '      <th>日期</th><th>类别</th><th>来源</th><th>项目</th><th>备注</th><th>金额</th><th>上传状态</th><th>报销状态</th>',
    '    </tr>',
    '  </thead>',
    `  <tbody>${rows}</tbody>`,
    '</table>',
    '<div class="footer">',
    `  <span>由 FinArch 自动生成 · ${now.toLocaleString('zh-CN')}</span>`,
    `  <span>共 ${filtered.length} 条记录</span>`,
    '</div>',
    '</body>',
    '</html>',
  ].join('\n')

  const html = head + '\n' + body

  const win = window.open('', '_blank', 'width=1100,height=800')
  if (!win) {
    alert('弹出窗口被拦截，请允许本站弹窗后重试')
    return
  }
  win.document.write(html)
  win.document.close()
  setTimeout(() => win.print(), 400)
}
