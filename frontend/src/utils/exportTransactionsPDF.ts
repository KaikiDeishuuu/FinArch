import type { Transaction } from '../api/client'

function fmt(n: number) {
  return `¥${n.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
}

export function exportTransactionsPDF(
  filtered: Transaction[],
  filterLabel: string,
  user: { name: string; email: string; role: string } | null,
) {
  const now = new Date()
  const dateStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
  const incSum = filtered.filter(t => t.direction === 'income').reduce((s, t) => s + t.amount_yuan, 0)
  const expSum = filtered.filter(t => t.direction === 'expense').reduce((s, t) => s + t.amount_yuan, 0)
  const reimbursedSum = filtered.filter(t => t.direction === 'expense' && t.source === 'personal' && t.reimbursed).reduce((s, t) => s + t.amount_yuan, 0)
  const net = incSum - expSum + reimbursedSum

  const rows = filtered.map(t => {
    const dir = t.direction === 'income' ? '收入' : '支出'
    void dir
    const src = t.source === 'company' ? '公司' : '个人'
    const amount = `${t.direction === 'income' ? '+' : '−'}${fmt(t.amount_yuan)}`
    const amtColor = t.direction === 'income' ? '#16a34a' : '#ef4444'
    const uploaded = t.uploaded ? '✓ 已上传' : '✗ 未上传'
    const uploadedColor = t.uploaded ? '#7c3aed' : '#9ca3af'
    const reimbursed = t.source === 'personal' ? (t.reimbursed ? '✓ 已报销' : '✗ 待报销') : '—'
    const reimbursedColor = t.source === 'personal' ? (t.reimbursed ? '#15803d' : '#9ca3af') : '#d1d5db'
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
    '.header { border-bottom: 2px solid #2563eb; padding-bottom: 14px; margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-end; }',
    '.header-left h1 { font-size: 20px; font-weight: 800; color: #1e40af; letter-spacing: 0.02em; }',
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
    '.summary-card .value.count { color: #2563eb; }',
    'table { width: 100%; border-collapse: collapse; font-size: 10.5px; }',
    'thead tr { background: #1e40af; color: #fff; }',
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
  const netStr = (net >= 0 ? '+' : '') + fmt(net)

  const body = [
    '<div class="header">',
    '  <div class="header-left">',
    '    <h1>FinArch 财务系统 · 交易明细</h1>',
    `    <p>筛选：${filterLabel} &nbsp;|&nbsp; 导出日期：${dateStr}</p>`,
    '  </div>',
    '  <div class="header-right">',
    `    <div class="user-name">${user?.name ?? '—'}</div>`,
    `    <div class="user-detail">${user?.email ?? ''} &nbsp;|&nbsp; ${user?.role ?? ''}</div>`,
    '  </div>',
    '</div>',
    '<div class="summary">',
    '  <div class="summary-card">',
    '    <div class="label">记录数</div>',
    `    <div class="value count">${filtered.length} 笔</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">收入合计</div>',
    `    <div class="value income">${fmt(incSum)}</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">支出合计</div>',
    `    <div class="value expense">${fmt(expSum)}</div>`,
    '  </div>',
    '  <div class="summary-card">',
    '    <div class="label">已报销</div>',
    `    <div class="value" style="color:#7c3aed">+${fmt(reimbursedSum)}</div>`,
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
