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
  const inputClass = 'fin-input w-full px-3.5 py-2.5 font-data text-sm tabular-nums'
  const labelClass = 'page-kicker mb-2 block'

  return (
    <div className="space-y-6 max-w-3xl">
      {/* Header */}
      <div className="ledger-rail pl-5">
        <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-[1.75rem]">{t('match.title')}</h1>
        <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{isLifeMode ? t('match.life.subtitle') : t('match.subtitle')}</p>
      </div>

      {/* Form card — Premium */}
      <div className="ledger-panel overflow-hidden">
        <div className="border-b border-[hsl(var(--border))] px-5 pb-0 pt-4">
          <div className="flex flex-wrap items-center gap-2 mb-4">
            {/* Source filter tabs */}
            <div className="flex w-fit gap-1 rounded-[7px] border border-[hsl(var(--border))] bg-[hsl(var(--muted))] p-1">
              {([enforcedSource] as const).map((key) => (
                <button
                  key={key}
                  type="button"
                  onClick={() => {
                    resetSearch()
                    setFilterAccount('')
                  }}
                  aria-pressed={sourceFilter === key}
                  className={`rounded-[5px] px-3 py-1 text-xs font-medium transition-colors ${sourceFilter === key
                      ? 'bg-[hsl(var(--card))] text-[hsl(var(--mode-accent-strong))] shadow-sm'
                      : 'text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'
                    }`}
                >
                  {t(`match.sourceTabs.${key}`)}
                </button>
              ))}
            </div>

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

            {/* Clear extra filters */}
            {(effectiveFilterCategory || effectiveFilterAccount) && (
              <button
                type="button"
                onClick={() => {
                  resetSearch()
                  setFilterCategory('')
                  setFilterAccount('')
                }}
                className="h-8 px-2.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 text-xs transition-all"
              >
                {t('common.clear')}
              </button>
            )}
          </div>
        </div>

        {/* Info bar */}
        <div className="flex items-start gap-2.5 border-b border-[hsl(var(--border))] bg-[hsl(var(--mode-accent-wash))] px-5 py-3">
          <svg className="mt-0.5 h-4 w-4 shrink-0 text-[hsl(var(--mode-accent-strong))]" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" /></svg>
          <div>
            <p className="text-sm text-[hsl(var(--mode-accent-strong))]">
              <Trans
                i18nKey={isLifeMode ? 'match.life.info.uploadedOnly' : 'match.info.uploadedOnly'}
                values={{ source: t(`match.sourceTabs.${sourceFilter}`) }}
                components={{ strong: <strong /> }}
              />
            </p>
            <p className="mt-1 text-xs text-[hsl(var(--muted-foreground))]">{t('match.info.currencyNote')}</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="p-5">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className={labelClass}>{t('match.form.targetAmount')}</label>
              <input
                type="number"
                required
                min="0.01"
                step="0.01"
                className={inputClass}
                placeholder={t('match.form.targetPlaceholder')}
                value={target}
                onChange={(e) => {
                  resetSearch()
                  setTarget(e.target.value)
                }}
              />
            </div>
            <div>
              <label className={labelClass}>{t('match.form.tolerance')}</label>
              <input
                type="number"
                min="0"
                step="0.01"
                className={inputClass}
                value={tolerance}
                onChange={(e) => {
                  resetSearch()
                  setTolerance(e.target.value)
                }}
              />
            </div>
            <div>
              <label className={labelClass}>{t('match.form.maxCount')}</label>
              <input
                type="number"
                min="1"
                max="50"
                className={inputClass}
                value={maxItems}
                onChange={(e) => {
                  resetSearch()
                  setMaxItems(e.target.value)
                }}
              />
            </div>
          </div>

          {error && (
            <div className="mt-4 bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-800 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
              <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" /></svg>
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="mt-5 flex w-full items-center justify-center gap-2 rounded-[7px] bg-[hsl(var(--primary))] py-2.5 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50"
          >
            {loading ? (
              <>
                <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                {t('match.form.searching')}
              </>
            ) : (
              <>
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
                {t('match.form.searchButton')}
              </>
            )}
          </button>
          <button
            type="button"
            onClick={handleExportPDF}
            disabled={matchedTransactions.length === 0}
            className="fin-control mt-2 w-full py-2.5 text-sm font-semibold text-[hsl(var(--foreground))] disabled:opacity-50"
          >
            {isLifeMode ? t('match.life.exportPdf') : t('transactions.exportPdf')}
          </button>
        </form>
      </div>

      {/* Results */}
      {searched && (
        <div className="space-y-3">
          {/* Time-pruned warning */}
          {timePruned && (
            <div className="flex items-start gap-2.5 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-800 rounded-xl px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" /></svg>
              <span>{t('match.results.timePruned')}</span>
            </div>
          )}

          {searchTruncated && (
            <div
              data-testid="match-truncated-warning"
              className="flex items-start gap-2.5 bg-amber-50 dark:bg-amber-500/10 border border-amber-200 dark:border-amber-800 rounded-xl px-4 py-3 text-sm text-amber-700 dark:text-amber-400"
            >
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" /></svg>
              <span>{t('match.results.truncated')}</span>
            </div>
          )}

          {/* Result header */}
          <div className="flex items-center justify-between px-1">
            {results.length === 0 ? (
              <div className="flex items-center gap-2 text-gray-500">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5 shrink-0"><circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" /></svg>
                <span className="text-sm">{t('match.results.noResultsHint')}</span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5 text-emerald-500 shrink-0"><polyline points="20 6 9 17 4 12" /></svg>
                <span className="text-sm text-gray-600 dark:text-gray-400">
                  {t('match.results.foundBefore')}<span className="font-data text-base font-bold text-[hsl(var(--mode-accent-strong))]">{results.length}</span>{t('match.results.foundAfter')}
                </span>
              </div>
            )}
          </div>

          {results.map((r, i) => {
            const rankBg = i === 0
              ? 'bg-[hsl(var(--mode-accent))] text-[hsl(var(--primary-foreground))]'
              : 'border border-[hsl(var(--border))] bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]'
            return (
              <div key={i} className={`ledger-panel overflow-hidden ${i === 0 ? 'border-l-2 border-l-[hsl(var(--mode-accent))]' : ''}`}>
                {/* Card header (clickable) */}
                <button
                  className="w-full px-5 py-4 flex items-center justify-between hover:bg-gray-50/60 dark:hover:bg-gray-800/40 transition-colors"
                  onClick={() => setExpandedIdx(expandedIdx === i ? null : i)}
                  aria-expanded={expandedIdx === i}
                  aria-controls={`match-result-${i}`}
                >
                  <div className="flex items-center gap-4 text-left">
                    <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-[6px] font-data text-xs font-bold ${rankBg}`}>
                      {i + 1}
                    </span>
                    <div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <p className="font-bold text-gray-800 dark:text-gray-200 text-base">{fmt(cnyTotal(r), 'CNY')}</p>
                        {r.error <= 0.01 && (
                          <span className="text-xs bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-400 px-2 py-0.5 rounded-full font-medium">{t('match.results.exactMatch')}</span>
                        )}
                        {r.score != null && (
                          <span className="rounded bg-[hsl(var(--mode-accent-wash))] px-2 py-0.5 font-data text-xs font-medium tabular-nums text-[hsl(var(--mode-accent-strong))]">
                            Score {r.score.toFixed(3)}
                          </span>
                        )}
                        {hasMixedCurrency(r) && (
                          <span className="text-xs bg-amber-100 dark:bg-amber-500/20 text-amber-700 dark:text-amber-400 px-2 py-0.5 rounded-full font-medium">{t('match.results.mixedCurrency')}</span>
                        )}
                      </div>
                      <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 flex items-center gap-2">
                        <span>{t('match.results.count', { count: r.item_count })}</span>
                        <span className="w-1 h-1 rounded-full bg-gray-300 dark:bg-gray-600" />
                        <span>{t('match.results.projects', { count: r.project_count })}</span>
                      </p>
                    </div>
                  </div>
                  <div className={`flex h-7 w-7 items-center justify-center rounded-[6px] transition-transform ${expandedIdx === i ? 'rotate-180 bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]' : 'bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]'}`}>
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
                  </div>
                </button>

                {/* Expanded detail */}
                {expandedIdx === i && (
                  <div id={`match-result-${i}`} className="border-t border-[hsl(var(--border))]">
                    {r.items && r.items.length > 0 ? (
                      <>
                        {/* Mobile: card list */}
                        <div className="md:hidden divide-y divide-gray-50 dark:divide-gray-800">
                          {r.items.map((item) => {
                            const done = !isLifeMode && reimbursedIds.has(item.id)
                            const confirming = confirmId === item.id
                            const busy = loadingId === item.id
                            return (
                              <div key={item.id} className={`px-4 py-3 space-y-1.5 ${done ? 'opacity-60' : ''}`}>
                                <div className="flex items-center justify-between gap-2">
                                  <span className={`font-semibold text-sm ${done ? 'text-gray-400 dark:text-gray-500 line-through' : 'text-gray-700 dark:text-gray-300'}`}>{categoryLabel(item.category)}</span>
                                  <span className={`font-bold tabular-nums whitespace-nowrap text-sm ${done ? 'text-gray-400 dark:text-gray-500 line-through' : 'text-rose-500'}`}>−{fmt(item.amount_yuan, item.currency)}</span>
                                </div>
                                <div className="flex items-center gap-2 flex-wrap text-xs text-gray-400 dark:text-gray-500">
                                  <span className="tabular-nums">{item.occurred_at}</span>
                                  {item.project_id && (
                                    <span className="font-mono bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400 px-1.5 py-0.5 rounded">{item.project_id}</span>
                                  )}
                                  {item.note && <span className="truncate max-w-[180px]">{item.note}</span>}
                                </div>
                                {!isLifeMode && (done ? (
                                  <span className="inline-flex items-center gap-1 text-xs text-emerald-500 font-medium">
                                    <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
                                    {t('match.reimburse.reimbursed')}
                                  </span>
                                ) : confirming ? (
                                  <div className="flex items-center gap-2 pt-0.5">
                                    <span className="text-xs text-gray-500 dark:text-gray-400">{t('match.reimburse.confirmPrompt')}</span>
                                    <button onClick={() => handleReimburse(item.id)} disabled={busy}
                                      className="text-xs bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white px-2.5 py-1 rounded-lg font-medium">
                                      {busy ? '…' : t('common.confirm')}
                                    </button>
                                    <button onClick={() => setConfirmId(null)}
                                      className="text-xs text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 px-2 py-1">
                                      {t('common.cancel')}
                                    </button>
                                  </div>
                                ) : (
                                  <button onClick={() => setConfirmId(item.id)}
                                    className="text-xs font-medium text-[hsl(var(--mode-accent-strong))] transition-colors hover:text-[hsl(var(--foreground))]">
                                    {t('match.reimburse.markButton')}
                                  </button>
                                ))}
                              </div>
                            )
                          })}
                          <div className="px-4 py-3 bg-gray-50 dark:bg-gray-800/50 flex items-center justify-between">
                            <span className="text-xs font-semibold text-gray-500 dark:text-gray-400">{t('match.results.totalRealtime')}</span>
                            <span className="font-bold text-rose-600 tabular-nums whitespace-nowrap text-sm">−{fmt(cnyTotal(r), 'CNY')}</span>
                          </div>
                        </div>
                        {/* Desktop: table */}
                        <table className="hidden md:table w-full text-xs">
                          <thead>
                            <tr className="bg-gray-50 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 uppercase tracking-wider border-b border-gray-100 dark:border-gray-800">
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
                                <tr key={item.id} className={`border-b border-gray-50 dark:border-gray-800 last:border-0 transition-colors ${done ? 'opacity-50 bg-emerald-50/30 dark:bg-emerald-500/5' : idx % 2 === 0 ? 'hover:bg-gray-50/60 dark:hover:bg-gray-800/40' : 'bg-gray-50/30 dark:bg-gray-800/20 hover:bg-gray-50/60 dark:hover:bg-gray-800/40'}`}>
                                  <td className="px-4 py-2.5 font-mono text-gray-400 dark:text-gray-500 bg-gray-50/50 dark:bg-gray-800/30">{item.id.slice(0, 8)}…</td>
                                  <td className={`px-4 py-2.5 tabular-nums whitespace-nowrap ${done ? 'text-gray-400 dark:text-gray-500 line-through' : 'text-gray-500 dark:text-gray-400'}`}>{item.occurred_at}</td>
                                  <td className={`px-4 py-2.5 font-medium ${done ? 'text-gray-400 dark:text-gray-500 line-through' : 'text-gray-600 dark:text-gray-300'}`}>{categoryLabel(item.category)}</td>
                                  <td className="px-4 py-2.5">
                                    {item.project_id
                                      ? <span className="font-mono bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400 px-1.5 py-0.5 rounded">{item.project_id}</span>
                                      : <span className="text-gray-300 dark:text-gray-600">—</span>
                                    }
                                  </td>
                                  <td className="px-4 py-2.5 text-gray-400 dark:text-gray-500 max-w-[140px] truncate" title={item.note ?? undefined}>{item.note || '—'}</td>
                                  <td className={`px-4 py-2.5 text-right font-bold tabular-nums whitespace-nowrap ${done ? 'text-gray-400 dark:text-gray-500 line-through' : 'text-rose-500'}`}>−{fmt(item.amount_yuan, item.currency)}</td>
                                  {!isLifeMode && <td className="px-4 py-2.5 text-center">
                                    {done ? (
                                      <span className="inline-flex items-center gap-1 text-xs text-emerald-500 font-medium whitespace-nowrap">
                                        <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
                                        {t('match.reimburse.reimbursed')}
                                      </span>
                                    ) : confirming ? (
                                      <div className="flex items-center justify-center gap-1.5">
                                        <button onClick={() => handleReimburse(item.id)} disabled={busy}
                                          className="text-xs bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white px-2 py-1 rounded-lg font-medium">
                                          {busy ? '…' : t('common.confirm')}
                                        </button>
                                        <button onClick={() => setConfirmId(null)}
                                          className="text-xs text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 px-1.5 py-1">
                                          {t('common.cancel')}
                                        </button>
                                      </div>
                                    ) : (
                                      <button onClick={() => setConfirmId(item.id)}
                                        className="whitespace-nowrap text-xs font-medium text-[hsl(var(--mode-accent-strong))] transition-colors hover:text-[hsl(var(--foreground))]">
                                        {t('match.reimburse.markShort')}
                                      </button>
                                    )}
                                  </td>}
                                </tr>
                              )
                            })}
                          </tbody>
                          <tfoot>
                            <tr className="bg-gray-50 dark:bg-gray-800/50 border-t border-gray-200 dark:border-gray-700">
                              <td colSpan={5} className="px-4 py-2.5 text-xs font-semibold text-gray-500 dark:text-gray-400">{t('match.results.totalRealtime')}</td>
                              <td className="px-4 py-2.5 text-right font-bold text-rose-600 tabular-nums whitespace-nowrap">−{fmt(cnyTotal(r), 'CNY')}</td>
                              {!isLifeMode && <td />}
                            </tr>
                          </tfoot>
                        </table>
                      </>
                    ) : (
                      <div className="px-5 py-4">
                        <p className="text-xs text-gray-500 dark:text-gray-400 font-semibold mb-2 uppercase tracking-wider">{t('match.results.idListTitle')}</p>
                        <div className="flex flex-wrap gap-1.5">
                          {r.ids.map((id) => (
                            <span key={id} className="font-mono text-xs bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-400 px-2 py-1 rounded-lg">
                              {id.slice(0, 8)}…
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
