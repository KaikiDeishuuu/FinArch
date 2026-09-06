import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { transformSync } from 'esbuild'

const addTransactionSource = readFileSync(
  new URL('../src/pages/AddTransactionPage.tsx', import.meta.url),
  'utf8',
)
const matchSource = readFileSync(new URL('../src/pages/MatchPage.tsx', import.meta.url), 'utf8')
const transactionsSource = readFileSync(new URL('../src/pages/TransactionsPage.tsx', import.meta.url), 'utf8')
const statsSource = readFileSync(new URL('../src/pages/StatsPage.tsx', import.meta.url), 'utf8')
const dashboardSource = readFileSync(new URL('../src/pages/DashboardPage.tsx', import.meta.url), 'utf8')
const accountBalanceChartSource = readFileSync(new URL('../src/components/AccountBalanceChart.tsx', import.meta.url), 'utf8')
const accountsHookSource = readFileSync(new URL('../src/hooks/useAccounts.ts', import.meta.url), 'utf8')
const financeRefreshSource = readFileSync(new URL('../src/hooks/useRefreshFinanceData.ts', import.meta.url), 'utf8')
const accountScopeSource = readFileSync(new URL('../src/utils/accountScope.ts', import.meta.url), 'utf8')
const transactionWorkflowSource = readFileSync(new URL('../src/utils/transactionWorkflow.ts', import.meta.url), 'utf8')
const enLocaleSource = readFileSync(new URL('../src/i18n/locales/en.ts', import.meta.url), 'utf8')
const zhLocaleSource = readFileSync(new URL('../src/i18n/locales/zh.ts', import.meta.url), 'utf8')

const policySource = matchSource.match(
  /function isEligibleMatchTransaction[\s\S]+?\n}\n\nexport default/,
)?.[0].replace(/^function/, 'export function').replace(/\n\nexport default$/, '')

assert.ok(policySource, 'MatchPage must keep its eligibility policy independently testable')

const tempDir = mkdtempSync(join(tmpdir(), 'finarch-reimbursement-ui-policy-'))
const compiledPath = join(tempDir, 'matchPolicy.mjs')
const compiledAccountScopePath = join(tempDir, 'accountScope.mjs')
const compiledTransactionWorkflowPath = join(tempDir, 'transactionWorkflow.mjs')
const compiledEnLocalePath = join(tempDir, 'enLocale.mjs')
const compiledZhLocalePath = join(tempDir, 'zhLocale.mjs')
writeFileSync(compiledPath, transformSync(policySource, {
  loader: 'ts',
  format: 'esm',
  target: 'es2022',
}).code)
writeFileSync(compiledAccountScopePath, transformSync(accountScopeSource, {
  loader: 'ts',
  format: 'esm',
  target: 'es2022',
}).code)
writeFileSync(compiledTransactionWorkflowPath, transformSync(transactionWorkflowSource, {
  loader: 'ts',
  format: 'esm',
  target: 'es2022',
}).code)
writeFileSync(compiledEnLocalePath, transformSync(enLocaleSource, {
  loader: 'ts',
  format: 'esm',
  target: 'es2022',
}).code)
writeFileSync(compiledZhLocalePath, transformSync(zhLocaleSource, {
  loader: 'ts',
  format: 'esm',
  target: 'es2022',
}).code)
const { isEligibleMatchTransaction } = await import(pathToFileURL(compiledPath).href)
const {
  accountModeForTransactionSource,
  transactionSourceForMode,
} = await import(pathToFileURL(compiledAccountScopePath).href)
const {
  isTransactionInWorkflowTab,
  transactionWorkflowStage,
} = await import(pathToFileURL(compiledTransactionWorkflowPath).href)
const en = (await import(pathToFileURL(compiledEnLocalePath).href)).default.translation
const zh = (await import(pathToFileURL(compiledZhLocalePath).href)).default.translation

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

test('new transaction source is strict, mode-aware, and both work sources remain selectable', () => {
  assert.equal(transactionSourceForMode('work', null), 'company')
  assert.equal(transactionSourceForMode('work', 'personal'), 'personal')
  assert.equal(transactionSourceForMode('work', 'company'), 'company')
  assert.equal(transactionSourceForMode('work', 'invalid'), 'company')
  assert.equal(transactionSourceForMode('life', 'company'), 'personal')
  assert.equal(transactionSourceForMode('life', 'personal'), 'personal')
  assert.match(addTransactionSource, /const initialSource = transactionSourceForMode\(mode, searchParams\.get\('source'\)\)/)
  assert.match(addTransactionSource, /key=\{`\$\{mode\}:\$\{initialSource\}`\}/)
  assert.match(addTransactionSource, /source:\s*initialSource/)
  assert.match(addTransactionSource, /onClick=\{\(\) => set\('source', 'personal'\)}/)
  assert.match(
    addTransactionSource,
    /onClick=\{\(\) => isWorkMode && set\('source', 'company'\)}[\s\S]*?disabled=\{!isWorkMode}/,
  )
  assert.equal(accountModeForTransactionSource('company'), 'work')
  assert.equal(accountModeForTransactionSource('personal'), 'life')
  assert.match(addTransactionSource, /useAccounts\(accountLookupMode\)/)
  assert.match(accountsHookSource, /export function useAccounts\(requestedMode\?: AppMode\)/)
  assert.match(accountsHookSource, /const mode = requestedMode \?\? currentMode/)
  assert.match(financeRefreshSource, /queryKey: \['accounts', user\?\.id\]/)
  assert.match(financeRefreshSource, /queryKey: \['account-balance-history', user\?\.id\]/)
  assert.match(addTransactionSource, /role="group" aria-labelledby="transaction-direction-label"/)
  assert.match(addTransactionSource, /onClick=\{\(\) => set\('direction', 'expense'\)\}\s+aria-pressed=\{isExpense\}/)
  assert.match(addTransactionSource, /onClick=\{\(\) => set\('direction', 'income'\)\}\s+aria-pressed=\{!isExpense\}/)
  assert.match(addTransactionSource, /role="group" aria-labelledby="transaction-source-label"/)
  assert.match(addTransactionSource, /onClick=\{\(\) => set\('source', 'personal'\)\}\s+aria-pressed=\{isPersonal\}/)
  assert.match(addTransactionSource, /disabled=\{!isWorkMode\}\s+aria-pressed=\{!isPersonal\}/)
})

test('work ledgers expose both sources while reimbursement controls stay personal-only', () => {
  assert.match(addTransactionSource, /navigate\(`\/transactions\?source=\$\{form\.source\}`\)/)
  assert.match(transactionsSource, /transactionSourceForMode\(mode, searchParams\.get\('source'\)\)/)
  assert.match(transactionsSource, /key=\{`\$\{mode\}:\$\{effectiveSourceFilter\}`\}/)
  assert.match(transactionsSource, /to=\{`\/add\?source=\$\{effectiveSourceFilter\}`\}/)
  assert.match(transactionsSource, /const isReimbursementView = isWorkMode && effectiveSourceFilter === 'personal'/)
  assert.match(
    transactionsSource,
    /if \(!tx \|\| !isWorkMode \|\| tx\.source !== 'personal' \|\| tx\.direction !== 'expense'\) return/,
  )
  assert.match(transactionsSource, /\{isReimbursementView && <StatusBadge[\s\S]*?onClick=\{\(\) => handleToggle\(tx\.id\)\}/)
  assert.match(statsSource, /\(\['company', 'personal'\] as const\)\.map/)
  assert.match(statsSource, /isWorkMode && sourceFilter === 'personal'[\s\S]*?calculateWorkModeAdjustments/)
  assert.match(statsSource, /useAccounts\(accountLookupMode\)/)
  assert.match(transactionsSource, /useAccounts\(accountLookupMode\)/)
  assert.match(matchSource, /useAccounts\(accountModeForTransactionSource\(enforcedSource\)\)/)
})

test('workflow tabs classify expenses only and leave income in All', () => {
  const income = { direction: 'income', uploaded: true, reimbursed: true }
  assert.equal(isTransactionInWorkflowTab(income, 'all', 'reimbursement'), true)
  assert.equal(isTransactionInWorkflowTab(income, 'unreimbursed', 'reimbursement'), false)
  assert.equal(isTransactionInWorkflowTab(income, 'reimbursed', 'reimbursement'), false)
  assert.equal(isTransactionInWorkflowTab(income, 'unreimbursed', 'upload'), false)
  assert.equal(isTransactionInWorkflowTab(income, 'reimbursed', 'upload'), false)

  const expense = { direction: 'expense', uploaded: false, reimbursed: true }
  assert.equal(isTransactionInWorkflowTab(expense, 'reimbursed', 'reimbursement'), true)
  assert.equal(isTransactionInWorkflowTab(expense, 'unreimbursed', 'upload'), true)

  assert.match(transactionsSource, /role="group" aria-label=\{t\('transactions\.workflowFilterLabel'\)\}/)
  assert.match(transactionsSource, /aria-pressed=\{filter === tb\.key\}/)
  assert.match(transactionsSource, /role="group" aria-label=\{t\('transactions\.sourceFilterLabel'\)\}/)
  assert.match(accountBalanceChartSource, /role="group" aria-label=\{t\('stats\.chart\.rangeLabel'\)\}/)
  assert.match(accountBalanceChartSource, /aria-pressed=\{range === opt\.value\}/)
  assert.equal(en.transactions.workflowFilterLabel, 'Transaction workflow status')
  assert.equal(zh.transactions.workflowFilterLabel, '交易流程状态')
  assert.equal(en.stats.chart.rangeLabel, 'Balance history range')
  assert.equal(zh.stats.chart.rangeLabel, '余额历史时间范围')
})

test('PDF and transaction tabs share explicit reimbursement and upload stages', () => {
  const pendingUpload = { direction: 'expense', uploaded: false, reimbursed: false }
  const uploadedPending = { direction: 'expense', uploaded: true, reimbursed: false }
  const reimbursed = { direction: 'expense', uploaded: true, reimbursed: true }

  assert.equal(transactionWorkflowStage(pendingUpload, 'reimbursement'), 'pending-upload')
  assert.equal(transactionWorkflowStage(uploadedPending, 'reimbursement'), 'pending-reimbursement')
  assert.equal(transactionWorkflowStage(reimbursed, 'reimbursement'), 'reimbursed')
  assert.equal(transactionWorkflowStage(pendingUpload, 'upload'), 'pending-upload')
  assert.equal(transactionWorkflowStage(uploadedPending, 'upload'), 'uploaded')

  assert.match(
    transactionsSource,
    /workflowKind[^\n]*isReimbursementView\s*\n\s*\? 'reimbursement'\s*\n\s*: isSettlementView \? 'settlement' : 'upload'/,
  )
  assert.match(transactionsSource, /exportTransactionsPDF\(filtered,[\s\S]*?workflowKind, accountMap\)/)
  assert.match(matchSource, /workflowKind[^\n]*isLifeMode \? 'upload' : 'reimbursement'/)
  assert.match(matchSource, /exportTransactionsPDF\(matchedTransactions,[\s\S]*?workflowKind, \{\}\)/)
})

test('public-account settlement is a separate lane that never reads reimbursement', () => {
  const pendingUpload = { direction: 'expense', uploaded: false, reimbursed: false, settled: false }
  const uploadedPending = { direction: 'expense', uploaded: true, reimbursed: false, settled: false }
  const settled = { direction: 'expense', uploaded: true, reimbursed: false, settled: true }

  assert.equal(transactionWorkflowStage(pendingUpload, 'settlement'), 'pending-upload')
  assert.equal(transactionWorkflowStage(uploadedPending, 'settlement'), 'pending-settlement')
  assert.equal(transactionWorkflowStage(settled, 'settlement'), 'settled')
  assert.equal(isTransactionInWorkflowTab(settled, 'reimbursed', 'settlement'), true)
  assert.equal(isTransactionInWorkflowTab(uploadedPending, 'unreimbursed', 'settlement'), true)

  // The two lanes must stay independent: reimbursing must not settle, and
  // settling must not reimburse. WORK statistics add reimbursed amounts back
  // into net, so a public-account expense leaking into that lane would inflate
  // the user's net by money that was never theirs.
  const reimbursedOnly = { direction: 'expense', uploaded: true, reimbursed: true, settled: false }
  assert.equal(transactionWorkflowStage(reimbursedOnly, 'settlement'), 'pending-settlement')
  assert.equal(transactionWorkflowStage(settled, 'reimbursement'), 'pending-reimbursement')

  assert.match(transactionsSource, /const isSettlementView = isWorkMode && effectiveSourceFilter === 'company'/)
  assert.match(
    transactionsSource,
    /if \(!tx \|\| !isWorkMode \|\| tx\.source !== 'company' \|\| tx\.direction !== 'expense'\) return/,
  )
  assert.match(transactionsSource, /\{isSettlementView && <StatusBadge[\s\S]*?onClick=\{\(\) => handleToggleSettle\(tx\.id\)\}/)
  assert.equal(en.transactions.badges.settled, 'Settled')
  assert.equal(zh.transactions.badges.settled, '已核销')
  assert.equal(zh.transactions.settlementTabs.done, '已核销')
  assert.equal(zh.exportPdf.workflow.settled, '已核销')
})

test('work dashboard derives reimbursement cards and actions from personal advances', () => {
  assert.match(dashboardSource, /const personalOutstanding = useMemo\(\(\) =>[\s\S]*?t\.source === 'personal'[\s\S]*?!t\.reimbursed/)
  assert.match(dashboardSource, /const pendingTxs = useMemo\([\s\S]*?isWorkMode[\s\S]*?t\.source === 'personal'/)
  assert.match(dashboardSource, /to="\/transactions\?source=personal"/)
  assert.doesNotMatch(dashboardSource, /companyUploadedNotReimbursed|companyOutstanding/)
})

test('work matching accepts only uploaded, pending personal expenses', () => {
  const eligible = {
    source: 'personal',
    direction: 'expense',
    uploaded: true,
    reimbursed: false,
  }

  assert.equal(isEligibleMatchTransaction(eligible, 'work'), true)
  assert.equal(isEligibleMatchTransaction({ ...eligible, source: 'company' }, 'work'), false)
  assert.equal(isEligibleMatchTransaction({ ...eligible, direction: 'income' }, 'work'), false)
  assert.equal(isEligibleMatchTransaction({ ...eligible, uploaded: false }, 'work'), false)
  assert.equal(isEligibleMatchTransaction({ ...eligible, reimbursed: true }, 'work'), false)
  assert.match(matchSource, /<MatchPageForMode key=\{mode\} mode=\{mode\} \/>/)
  assert.match(matchSource, /const eligibleTransactions = useMemo\([\s\S]*?isEligibleMatchTransaction\(tx, mode\)/)
  assert.match(matchSource, /new Set\(eligibleTransactions\.map\(tx => tx\.category\)/)
  assert.match(matchSource, /const effectiveFilterCategory = allCategories\.includes\(filterCategory\) \? filterCategory : ''/)
  assert.match(matchSource, /const effectiveFilterAccount = eligibleAccountIds\.has\(filterAccount\) \? filterAccount : ''/)
  assert.match(matchSource, /const candidates = eligibleTransactions\.filter/)
})

test('life matching is read-only and does not expose reimbursement actions', () => {
  const uploadedPersonalExpense = {
    source: 'personal',
    direction: 'expense',
    uploaded: true,
    reimbursed: true,
  }

  assert.equal(isEligibleMatchTransaction(uploadedPersonalExpense, 'life'), true)
  assert.match(matchSource, /if \(isLifeMode\) return[\s\S]*?isEligibleMatchTransaction\(transaction, 'work'\)/)
  assert.doesNotMatch(matchSource, /toggleUploaded/)
  assert.match(matchSource, /\{!isLifeMode && <th[^>]*>\{t\('match\.table\.reimburse'\)}/)
  assert.match(matchSource, /\{!isLifeMode && <td className="px-4 py-2\.5 text-center">/)
  assert.doesNotMatch(matchSource, /match\.life\.(?:process|table\.process)/)
  assert.match(dashboardSource, /const featureCards = FEATURES\.map/)
  assert.match(dashboardSource, /titleKey: isWorkMode \? feature\.titleKey : feature\.lifeTitleKey/)
  assert.equal(en.dashboard.features.lifeEntry.title, 'Daily Accounting')
  assert.equal(zh.dashboard.features.lifeEntry.title, '日常记账')
  assert.match(en.dashboard.features.lifeMatch.desc, /Read-only/)
  assert.match(zh.dashboard.features.lifeMatch.desc, /只读/)
  assert.match(en.dashboard.workflow.personalLifeDesc4, /without changing transaction status/)
  assert.match(zh.dashboard.workflow.personalLifeDesc4, /不会改变任何交易状态/)
  assert.equal(en.dashboard.balance.lifeExpenseLabel, 'All recorded personal expenses')
  assert.equal(zh.dashboard.balance.lifeExpenseLabel, '累计已记录的个人支出')
  assert.doesNotMatch(`${en.dashboard.workflow.personalLifeStep1} ${en.dashboard.workflow.personalLifeDesc1} ${en.dashboard.workflow.personalLifeStep2} ${en.dashboard.workflow.personalLifeDesc2}`, /advance|source/i)
  assert.doesNotMatch(`${zh.dashboard.workflow.personalLifeStep1} ${zh.dashboard.workflow.personalLifeDesc1} ${zh.dashboard.workflow.personalLifeStep2} ${zh.dashboard.workflow.personalLifeDesc2}`, /垫付|来源/)
  assert.match(dashboardSource, /titleKey: isWorkMode \? 'dashboard\.workflow\.personalStep1' : 'dashboard\.workflow\.personalLifeStep1'/)
  assert.match(dashboardSource, /titleKey: isWorkMode \? 'dashboard\.workflow\.personalStep2' : 'dashboard\.workflow\.personalLifeStep2'/)
  assert.match(dashboardSource, /dashboard\.balance\.lifeExpenseLabel/)
  assert.doesNotMatch(en.dashboard.features.lifeEntry.desc, /reimburs/i)
  assert.doesNotMatch(zh.dashboard.features.lifeEntry.desc, /报销/)
})
