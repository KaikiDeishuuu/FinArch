import { useState, useMemo, useRef, useEffect, useCallback } from 'react'
import type { FormEvent } from 'react'
import { toast } from 'sonner'
import { Trans, useTranslation } from 'react-i18next'
import { toggleReimbursed } from '../api/client'
import type { MatchResult, MatchResultItem, Account, Transaction } from '../api/client'
import { formatAmount } from '../utils/format'
import { transactionAmountToCNY } from '../utils/financeAmounts'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { useTransactions, useInvalidateTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import Select from '../components/Select'
import { isCurrentMatchResponse, MATCH_MAX_INPUT_CANDIDATES } from '../workers/matchProtocol'
import type { MatchWorkerResponse, WorkerTxItem } from '../workers/matchProtocol'
import MatchWorkerConstructor from '../workers/match.worker.ts?worker'
import { categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../hooks/useMode'
import { useAuth } from '../hooks/useAuth'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'
import { accountModeForTransactionSource } from '../utils/accountScope'
import type { TransactionWorkflowKind } from '../utils/transactionWorkflow'
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  Download,
  Info,
  Search,
} from 'lucide-react'
import { Alert } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { Field, Input } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { cn } from '../lib/utils'

function isEligibleMatchTransaction(transaction: Pick<Transaction, 'direction' | 'source' | 'uploaded' | 'reimbursed'>, mode: Transaction['mode']) {
  return transaction.source === 'personal' &&
    transaction.direction === 'expense' &&
    transaction.uploaded &&
    (mode === 'life' || !transaction.reimbursed)
}

export default function MatchPage() {
  const { mode } = useMode()

  return <MatchPageForMode key={mode} mode={mode} />
}

function MatchPageForMode({ mode }: { mode: Transaction['mode'] }) {
  const [target, setTarget] = useState('')
  const [tolerance, setTolerance] = useState('0.01')
  const [maxItems, setMaxItems] = useState('10')
  const isLifeMode = mode === 'life'
  const workflowKind: TransactionWorkflowKind = isLifeMode ? 'upload' : 'reimbursement'
  const enforcedSource = 'personal' as const
  const sourceFilter = enforcedSource
  const [filterCategory, setFilterCategory] = useState('')
  const [filterAccount, setFilterAccount] = useState('')
  const [results, setResults] = useState<MatchResult[]>([])
  const [timePruned, setTimePruned] = useState(false)
  const [searchTruncated, setSearchTruncated] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const [loadingId, setLoadingId] = useState<string | null>(null)
  const [reimbursedIds, setReimbursedIds] = useState<Set<string>>(new Set())
  const { rates } = useExchangeRates()
  const { data: txs = [] } = useTransactions()
  const { data: accounts = [] } = useAccounts(accountModeForTransactionSource(enforcedSource))
  const invalidate = useInvalidateTransactions()
  const { t } = useTranslation()
  const { user } = useAuth()
  const workerRef = useRef<Worker | null>(null)
  const activeRequestIdRef = useRef(0)

  const cancelActiveMatch = useCallback(() => {
    activeRequestIdRef.current += 1
    workerRef.current?.terminate()
    workerRef.current = null
    setLoading(false)
  }, [])

  const resetSearch = useCallback(() => {
    cancelActiveMatch()
    setResults([])
    setTimePruned(false)
    setSearchTruncated(false)
    setSearched(false)
    setExpandedIdx(null)
    setConfirmId(null)
  }, [cancelActiveMatch])

  const eligibleTransactions = useMemo(
    () => txs.filter(tx => isEligibleMatchTransaction(tx, mode)),
    [txs, mode],
  )
  const activeAccounts = useMemo(() =>
    accounts.filter((a: Account) => a.is_active),
    [accounts]
  )

  const eligibleAccountIds = useMemo(
    () => new Set(eligibleTransactions.map(tx => tx.account_id).filter(Boolean)),
    [eligibleTransactions],
  )

  // Only offer personal accounts that currently have matchable transactions.
  const filteredAccounts = useMemo(() => {
    return activeAccounts.filter((a: Account) => a.type === 'personal' && eligibleAccountIds.has(a.id))
  }, [activeAccounts, eligibleAccountIds])

  const allCategories = useMemo(
    () => Array.from(new Set(eligibleTransactions.map(tx => tx.category).filter(Boolean))).sort() as string[],
    [eligibleTransactions]
  )
  const effectiveFilterCategory = allCategories.includes(filterCategory) ? filterCategory : ''
  const effectiveFilterAccount = eligibleAccountIds.has(filterAccount) ? filterAccount : ''

  // Lazily create the worker on first use
  function getWorker(): Worker {
    if (!workerRef.current) {
      workerRef.current = new MatchWorkerConstructor()
    }
    return workerRef.current
  }

  useEffect(() => {
    return () => {
      activeRequestIdRef.current += 1
      workerRef.current?.terminate()
      workerRef.current = null
    }
  }, [])

  // Worker totals are normalized to CNY before matching.
  function cnyTotal(r: MatchResult): number {
    return r.total
  }
  // True if a result contains non-CNY currencies
  function hasMixedCurrency(r: MatchResult): boolean {
    return !!r.items?.some(item => item.currency && item.currency.toUpperCase() !== 'CNY')
  }

  async function handleReimburse(id: string) {
    if (isLifeMode) return
    const transaction = txs.find(tx => tx.id === id)
    if (!transaction || !isEligibleMatchTransaction(transaction, 'work')) return

    setLoadingId(id)
    try {
      await toggleReimbursed(id)
      setReimbursedIds(prev => new Set(prev).add(id))
      toast.success(t('match.reimburse.success'))
      invalidate()
    } finally {
      setLoadingId(null)
      setConfirmId(null)
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    const val = parseFloat(target)
    if (isNaN(val) || val <= 0) { setError(t('match.error.invalidAmount')); return }
    const tol = parseFloat(tolerance) || 0
    const maxD = parseInt(maxItems) || 10

    cancelActiveMatch()
    setLoading(true)
    setResults([])
    setTimePruned(false)
    setSearchTruncated(false)
    setSearched(false)
    setExpandedIdx(null)
    setConfirmId(null)
    setReimbursedIds(new Set())

    // Work mode matches pending personal reimbursements. Life mode uses the
    // same uploaded personal expenses as a read-only combination/export tool.
    const candidates = eligibleTransactions.filter(tx =>
      (!effectiveFilterCategory || tx.category === effectiveFilterCategory) &&
      (!effectiveFilterAccount || tx.account_id === effectiveFilterAccount)
    )
    const inputTruncated = candidates.length > MATCH_MAX_INPUT_CANDIDATES
    const boundedCandidates = candidates.slice(0, MATCH_MAX_INPUT_CANDIDATES)
    const txMap = new Map(boundedCandidates.map(tx => [tx.id, tx]))

    const workerItems: WorkerTxItem[] = boundedCandidates.map(tx => ({
      id: tx.id,
      amountCents: Math.round(transactionAmountToCNY(tx, rates) * 100),
      occurredTs: Math.floor(new Date(tx.occurred_at).getTime() / 1000),
      projectId: tx.project_id ?? undefined,
    }))

    const requestId = activeRequestIdRef.current + 1
    activeRequestIdRef.current = requestId
    const worker = getWorker()
    worker.onmessage = (ev: MessageEvent<MatchWorkerResponse>) => {
      if (!isCurrentMatchResponse(ev.data, activeRequestIdRef.current)) return
      setLoading(false)
      setSearched(true)
      if (!ev.data.ok) {
        setError(ev.data.error || t('match.error.matchFailed'))
        return
      }
      const workerResults = ev.data.results ?? []
      const pruned = ev.data.timePruned === true || workerResults.some(r => r.timePruned)
      setTimePruned(pruned)
      setSearchTruncated(inputTruncated || ev.data.truncated === true)

      // Map WorkerResult → MatchResult, enriching with cached tx data
      const mapped: MatchResult[] = workerResults.map(wr => {
        const items: MatchResultItem[] = wr.ids.flatMap(id => {
          const tx = txMap.get(id)
          if (!tx) return []
          const item: MatchResultItem = {
            id: tx.id,
            occurred_at: tx.occurred_at,
            direction: tx.direction,
            source: tx.source,
            category: tx.category,
            amount_yuan: tx.amount_yuan,
            currency: tx.currency,
            base_amount_cents: tx.base_amount_cents,
            base_currency: tx.base_currency,
            note: tx.note ?? '',
            project_id: tx.project_id ?? '',
            uploaded: tx.uploaded,
          }
          return [item]
        })
        return {
          ids: wr.ids,
          total: wr.totalCents / 100,
          error: wr.errorCents / 100,
          project_count: wr.projectCount,
          item_count: wr.itemCount,
          items,
          total_cents: wr.totalCents,
          error_cents: wr.errorCents,
          score: wr.score,
          time_pruned: wr.timePruned,
        }
      })
      setResults(mapped)
    }
    worker.onerror = (err) => {
      if (activeRequestIdRef.current !== requestId) return
      setLoading(false)
      setSearched(true)
      setError(err.message || t('match.error.matchFailed'))
    }
    worker.postMessage({
      requestId,
      targetCents: Math.round(val * 100),
      toleranceCents: Math.round(tol * 100),
      maxDepth: maxD,
      limit: 20,
      items: workerItems,
    })
  }



  const matchedTransactions = useMemo(() => {
    const index = new Map<string, Transaction>()
    for (const r of results) {
      for (const item of r.items ?? []) {
        index.set(item.id, {
          id: item.id,
          occurred_at: item.occurred_at,
          direction: item.direction as Transaction['direction'],
          source: item.source as Transaction['source'],
          account_id: '',
          category: item.category,
          mode,
          amount_yuan: item.amount_yuan,
          currency: item.currency,
          base_amount_cents: item.base_amount_cents,
          base_currency: item.base_currency,
          note: item.note,
          project_id: item.project_id,
          reimbursed: reimbursedIds.has(item.id),
          // Matching only ever sees personal advances, which never settle.
          settled: false,
          uploaded: item.uploaded,
        })
      }
    }
    return Array.from(index.values())
  }, [results, reimbursedIds, mode])

  function handleExportPDF() {
    if (matchedTransactions.length === 0) return
    const label = isLifeMode
      ? t('match.life.exportLabel', { source: t(`match.sourceTabs.${sourceFilter}`) })
      : t('match.exportLabel', { source: t(`match.sourceTabs.${sourceFilter}`) })
    exportTransactionsPDF(matchedTransactions, label, user, rates, workflowKind, {})
  }

  const fmt = (amount: number, currency: string) => formatAmount(amount, currency)

  return (
    <div className="max-w-4xl space-y-6">
      <PageHeader
        title={t('match.title')}
        description={isLifeMode ? t('match.life.subtitle') : t('match.subtitle')}
        meta={<Badge variant="mode">{isLifeMode ? t('mode.life') : t('mode.work')}</Badge>}
      />

      <Card className="overflow-hidden p-0">
        <div className="border-b border-border px-5 py-4">
          <div className="flex flex-wrap items-center gap-2">
            <Segmented aria-label={t('transactions.sourceFilterLabel')}>
              {([enforcedSource] as const).map((key) => (
                <SegmentedButton
                  key={key}
                  onClick={() => {
                    resetSearch()
                    setFilterAccount('')
                  }}
                  aria-pressed={sourceFilter === key}
                  className="aria-pressed:text-mode"
                >
                  {t(`match.sourceTabs.${key}`)}
                </SegmentedButton>
              ))}
            </Segmented>

            {/* Account filter */}
            {filteredAccounts.length > 1 && (
              <div className="w-fit min-w-[6.5rem]">
                <Select
                  value={effectiveFilterAccount}
                  onChange={(v) => {
                    resetSearch()
                    setFilterAccount(v)
                  }}
                  placeholder={t('match.filters.allAccounts')}
                  size="sm"
                  activeHighlight
                  options={[
                    { value: '', label: t('match.filters.allAccounts') },
                    ...filteredAccounts.map((a: Account) => ({ value: a.id, label: a.name })),
                  ]}
                />
              </div>
            )}

            {/* Category filter */}
            {allCategories.length > 0 && (
              <div className="w-fit min-w-[6.5rem]">
                <Select
                  value={effectiveFilterCategory}
                  onChange={(v) => {
                    resetSearch()
                    setFilterCategory(v)
                  }}
                  placeholder={t('match.filters.allCategories')}
                  size="sm"
                  activeHighlight
                  options={[
                    { value: '', label: t('match.filters.allCategories') },
                    ...allCategories.map(c => ({ value: c, label: categoryLabel(c) })),
                  ]}
                />
              </div>
            )}

            {(effectiveFilterCategory || effectiveFilterAccount) && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  resetSearch()
                  setFilterCategory('')
                  setFilterAccount('')
                }}
              >
                {t('common.clear')}
              </Button>
            )}
          </div>
        </div>

        <Alert variant="info" className="rounded-none border-x-0 border-t-0 px-5 py-3">
          <Info className="mt-0.5 size-4 shrink-0" />
          <div>
            <p>
              <Trans
                i18nKey={isLifeMode ? 'match.life.info.uploadedOnly' : 'match.info.uploadedOnly'}
                values={{ source: t(`match.sourceTabs.${sourceFilter}`) }}
                components={{ strong: <strong /> }}
              />
            </p>
            <p className="mt-1 text-xs opacity-80">{t('match.info.currencyNote')}</p>
          </div>
        </Alert>

        <form onSubmit={handleSubmit} className="p-5">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label={t('match.form.targetAmount')}>
              <Input
                type="number"
                required
                min="0.01"
                step="0.01"
                className="tabular-nums"
                placeholder={t('match.form.targetPlaceholder')}
                value={target}
                onChange={(e) => {
                  resetSearch()
                  setTarget(e.target.value)
                }}
              />
            </Field>
            <Field label={t('match.form.tolerance')}>
              <Input
                type="number"
                min="0"
                step="0.01"
                className="tabular-nums"
                value={tolerance}
                onChange={(e) => {
                  resetSearch()
                  setTolerance(e.target.value)
                }}
              />
            </Field>
            <Field label={t('match.form.maxCount')}>
              <Input
                type="number"
                min="1"
                max="50"
                className="tabular-nums"
                value={maxItems}
                onChange={(e) => {
                  resetSearch()
                  setMaxItems(e.target.value)
                }}
              />
            </Field>
          </div>

          {error && (
            <Alert variant="negative" className="mt-4">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{error}</span>
            </Alert>
          )}

          <div className="mt-5 grid gap-2 sm:grid-cols-2">
            <Button type="submit" loading={loading} loadingText={t('match.form.searching')}>
              <Search className="size-4" />
              {t('match.form.searchButton')}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={handleExportPDF}
              disabled={matchedTransactions.length === 0}
            >
              <Download className="size-4" />
              {isLifeMode ? t('match.life.exportPdf') : t('transactions.exportPdf')}
            </Button>
          </div>
        </form>
      </Card>

      {/* Results */}
      {searched && (
        <div className="space-y-3">
          {timePruned && (
            <Alert variant="warning">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{t('match.results.timePruned')}</span>
            </Alert>
          )}

          {searchTruncated && (
            <Alert data-testid="match-truncated-warning" variant="warning">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{t('match.results.truncated')}</span>
            </Alert>
          )}

          {results.length === 0 ? (
            <EmptyState
              title={t('match.results.noResultsHint')}
              icon={<Search />}
            />
          ) : (
            <div className="flex items-center gap-2 px-1 text-sm text-muted-foreground">
              <CheckCircle2 className="size-5 shrink-0 text-positive" />
              <span>
                {t('match.results.foundBefore')}
                <strong className="mx-1 text-base text-foreground">{results.length}</strong>
                {t('match.results.foundAfter')}
              </span>
            </div>
          )}

          {results.map((r, i) => {
            return (
              <Card key={i} className="overflow-hidden p-0">
                <button
                  type="button"
                  className="flex w-full items-center justify-between gap-3 px-5 py-4 text-left transition-colors hover:bg-muted/65 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  onClick={() => setExpandedIdx(expandedIdx === i ? null : i)}
                  aria-expanded={expandedIdx === i}
                >
                  <div className="flex min-w-0 items-center gap-4">
                    <Badge variant={i === 0 ? 'accent' : 'neutral'} className="grid size-8 place-items-center px-0 font-mono">
                      {i + 1}
                    </Badge>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-base font-semibold text-foreground tabular-nums">{fmt(cnyTotal(r), 'CNY')}</p>
                        {r.error <= 0.01 && <Badge variant="positive">{t('match.results.exactMatch')}</Badge>}
                        {r.score != null && <Badge variant="accent" className="font-mono">Score {r.score.toFixed(3)}</Badge>}
                        {hasMixedCurrency(r) && <Badge variant="warning">{t('match.results.mixedCurrency')}</Badge>}
                      </div>
                      <p className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{t('match.results.count', { count: r.item_count })}</span>
                        <span className="size-1 rounded-full bg-input" />
                        <span>{t('match.results.projects', { count: r.project_count })}</span>
                      </p>
                    </div>
                  </div>
                  <span className={cn('grid size-7 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground transition-transform', expandedIdx === i && 'rotate-180 text-accent')}>
                    <ChevronDown className="size-4" />
                  </span>
                </button>

                {expandedIdx === i && (
                  <div className="border-t border-border">
                    {r.items && r.items.length > 0 ? (
                      <>
                        {/* Mobile: card list */}
                        <div className="md:hidden divide-y divide-border">
                          {r.items.map((item) => {
                            const done = !isLifeMode && reimbursedIds.has(item.id)
                            const confirming = confirmId === item.id
                            const busy = loadingId === item.id
                            return (
                              <div key={item.id} className={`px-4 py-3 space-y-1.5 ${done ? 'opacity-60' : ''}`}>
                                <div className="flex items-center justify-between gap-2">
                                  <span className={`font-semibold text-sm ${done ? 'text-muted-foreground line-through' : 'text-foreground'}`}>{categoryLabel(item.category)}</span>
                                  <span className={`font-bold tabular-nums whitespace-nowrap text-sm ${done ? 'text-muted-foreground line-through' : 'text-negative'}`}>−{fmt(item.amount_yuan, item.currency)}</span>
                                </div>
                                <div className="flex items-center gap-2 flex-wrap text-xs text-muted-foreground">
                                  <span className="tabular-nums">{item.occurred_at}</span>
                                  {item.project_id && (
                                    <span className="font-mono bg-muted text-muted-foreground px-1.5 py-0.5 rounded">{item.project_id}</span>
                                  )}
                                  {item.note && <span className="truncate max-w-[180px]">{item.note}</span>}
                                </div>
                                {!isLifeMode && (done ? (
                                  <span className="inline-flex items-center gap-1 text-xs font-medium text-positive">
                                    <CheckCircle2 className="size-3.5" />
                                    {t('match.reimburse.reimbursed')}
                                  </span>
                                ) : confirming ? (
                                  <div className="flex flex-wrap items-center gap-2 pt-0.5">
                                    <span className="text-xs text-muted-foreground">{t('match.reimburse.confirmPrompt')}</span>
                                    <Button size="sm" variant="positive" onClick={() => handleReimburse(item.id)} disabled={busy} className="h-7 px-2.5">
                                      {busy ? '…' : t('common.confirm')}
                                    </Button>
                                    <Button size="sm" variant="ghost" onClick={() => setConfirmId(null)} className="h-7 px-2">
                                      {t('common.cancel')}
                                    </Button>
                                  </div>
                                ) : (
                                  <Button size="sm" variant="ghost" onClick={() => setConfirmId(item.id)} className="h-7 px-0 text-accent hover:text-accent">
                                    {t('match.reimburse.markButton')}
                                  </Button>
                                ))}
                              </div>
                            )
                          })}
                          <div className="flex items-center justify-between bg-muted/65 px-4 py-3">
                            <span className="text-xs font-semibold text-muted-foreground">{t('match.results.totalRealtime')}</span>
                            <span className="text-sm font-semibold whitespace-nowrap text-negative tabular-nums">−{fmt(cnyTotal(r), 'CNY')}</span>
                          </div>
                        </div>
                        {/* Desktop: table */}
                        <div className="hidden overflow-x-auto md:block">
                        <table className="w-full text-xs">
                          <thead>
                            <tr className="border-b border-border bg-muted/65 text-muted-foreground uppercase tracking-wider">
                              <th className="px-4 py-2.5 text-left font-semibold">{t('match.table.id')}</th>
                              <th className="px-4 py-2.5 text-left font-semibold">{t('match.table.date')}</th>
                              <th className="px-4 py-2.5 text-left font-semibold">{t('match.table.category')}</th>
                              <th className="px-4 py-2.5 text-left font-semibold">{t('match.table.project')}</th>
                              <th className="px-4 py-2.5 text-left font-semibold">{t('match.table.note')}</th>
                              <th className="px-4 py-2.5 text-right font-semibold">{t('match.table.amount')}</th>
                              {!isLifeMode && <th className="px-4 py-2.5 text-center font-semibold">{t('match.table.reimburse')}</th>}
                            </tr>
                          </thead>
                          <tbody>
                            {r.items.map((item, idx) => {
                              const done = !isLifeMode && reimbursedIds.has(item.id)
                              const confirming = confirmId === item.id
                              const busy = loadingId === item.id
                              return (
                                <tr key={item.id} className={cn('border-b border-border last:border-0 transition-colors hover:bg-muted/45', idx % 2 !== 0 && 'bg-muted/20', done && 'bg-positive-soft/30 opacity-55')}>
                                  <td className="bg-muted/35 px-4 py-2.5 font-mono text-muted-foreground">{item.id.slice(0, 8)}…</td>
                                  <td className={cn('px-4 py-2.5 tabular-nums whitespace-nowrap text-muted-foreground', done && 'line-through')}>{item.occurred_at}</td>
                                  <td className={cn('px-4 py-2.5 font-medium text-foreground', done && 'text-muted-foreground line-through')}>{categoryLabel(item.category)}</td>
                                  <td className="px-4 py-2.5">
                                    {item.project_id
                                      ? <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-muted-foreground">{item.project_id}</span>
                                      : <span className="text-subtle">—</span>
                                    }
                                  </td>
                                  <td className="max-w-[140px] truncate px-4 py-2.5 text-muted-foreground" title={item.note ?? undefined}>{item.note || '—'}</td>
                                  <td className={cn('px-4 py-2.5 text-right font-semibold tabular-nums whitespace-nowrap text-negative', done && 'text-muted-foreground line-through')}>−{fmt(item.amount_yuan, item.currency)}</td>
                                  {!isLifeMode && <td className="px-4 py-2.5 text-center">
                                    {done ? (
                                      <span className="inline-flex items-center gap-1 text-xs font-medium whitespace-nowrap text-positive">
                                        <CheckCircle2 className="size-3.5" />
                                        {t('match.reimburse.reimbursed')}
                                      </span>
                                    ) : confirming ? (
                                      <div className="flex items-center justify-center gap-1.5">
                                        <Button size="sm" variant="positive" onClick={() => handleReimburse(item.id)} disabled={busy} className="h-7 px-2">
                                          {busy ? '…' : t('common.confirm')}
                                        </Button>
                                        <Button size="sm" variant="ghost" onClick={() => setConfirmId(null)} className="h-7 px-1.5">
                                          {t('common.cancel')}
                                        </Button>
                                      </div>
                                    ) : (
                                      <Button size="sm" variant="ghost" onClick={() => setConfirmId(item.id)} className="h-7 px-1.5 text-accent hover:text-accent">
                                        {t('match.reimburse.markShort')}
                                      </Button>
                                    )}
                                  </td>}
                                </tr>
                              )
                            })}
                          </tbody>
                          <tfoot>
                            <tr className="border-t border-border bg-muted/65">
                              <td colSpan={5} className="px-4 py-2.5 text-xs font-semibold text-muted-foreground">{t('match.results.totalRealtime')}</td>
                              <td className="px-4 py-2.5 text-right font-semibold whitespace-nowrap text-negative tabular-nums">−{fmt(cnyTotal(r), 'CNY')}</td>
                              {!isLifeMode && <td />}
                            </tr>
                          </tfoot>
                        </table>
                        </div>
                      </>
                    ) : (
                      <div className="px-5 py-4">
                        <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('match.results.idListTitle')}</p>
                        <div className="flex flex-wrap gap-1.5">
                          {r.ids.map((id) => (
                            <span key={id} className="rounded-md bg-muted px-2 py-1 font-mono text-xs text-muted-foreground">
                              {id.slice(0, 8)}…
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}
