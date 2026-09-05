/**
 * Web Worker: integer-cent subset-sum matching (mirrors Go dfsCents logic).
 *
 * Input message:
 *   { requestId: number, targetCents: number, toleranceCents: number, maxDepth: number, limit: number, items: WorkerTxItem[] }
 *
 * Output message (array of WorkerResult, sorted by score desc):
 *   WorkerResult[]
 */

import { MATCH_MAX_INPUT_CANDIDATES } from './matchProtocol'
import type { MatchWorkerRequest, MatchWorkerResponse, WorkerResult, WorkerTxItem } from './matchProtocol'

interface DfsItem extends WorkerTxItem {
  ageDays: number
}

const DP_THRESHOLD = 1_000_000_000 // N × W > 10^9 → time-prune
const TIME_PRUNE_DAYS = 90
const TIME_CHECK_INTERVAL = 256

export interface MatchExecutionLimits {
  maxScannedCandidates: number
  maxCandidates: number
  maxNodes: number
  maxDurationMs: number
  maxResults: number
  clock: () => number
}

export interface MatchRunOutcome {
  results: WorkerResult[]
  truncated: boolean
  timePruned: boolean
}

const monotonicNow = () => (
  typeof performance === 'undefined' ? Date.now() : performance.now()
)

export const DEFAULT_MATCH_EXECUTION_LIMITS: MatchExecutionLimits = {
  maxScannedCandidates: MATCH_MAX_INPUT_CANDIDATES,
  maxCandidates: 500,
  maxNodes: 250_000,
  maxDurationMs: 500,
  maxResults: 20,
  clock: monotonicNow,
}

interface SearchBudget {
  nodes: number
  startedAt: number
  maxNodes: number
  maxDurationMs: number
  clock: () => number
  stopped: boolean
  truncated: boolean
}

function computeScore(itemCount: number, avgAgeDays: number): number {
  return 0.6 * (1 / itemCount) + 0.4 * (avgAgeDays / 365)
}

function makeResult(chosen: DfsItem[], totalCents: number, targetCents: number, timePruned: boolean): WorkerResult {
  const ids = chosen.map(i => i.id)
  const projectIds = new Set(chosen.map(i => i.projectId).filter(Boolean))
  const avgAgeDays = chosen.reduce((s, i) => s + i.ageDays, 0) / chosen.length
  return {
    ids,
    totalCents,
    errorCents: Math.abs(totalCents - targetCents),
    projectCount: projectIds.size,
    itemCount: chosen.length,
    score: computeScore(chosen.length, avgAgeDays),
    timePruned,
  }
}

function compareResults(a: WorkerResult, b: WorkerResult): number {
  return a.errorCents - b.errorCents ||
    b.score - a.score ||
    a.itemCount - b.itemCount ||
    a.ids.join('\0').localeCompare(b.ids.join('\0'))
}

function consumeSearchNode(budget: SearchBudget): boolean {
  budget.nodes += 1
  if (budget.nodes > budget.maxNodes) {
    budget.stopped = true
    budget.truncated = true
    return false
  }
  if (
    (budget.nodes === 1 || budget.nodes % TIME_CHECK_INTERVAL === 0) &&
    budget.clock() - budget.startedAt >= budget.maxDurationMs
  ) {
    budget.stopped = true
    budget.truncated = true
    return false
  }
  return true
}

function recordResult(
  chosen: DfsItem[],
  totalCents: number,
  targetCents: number,
  timePruned: boolean,
  results: WorkerResult[],
  maxResults: number,
  budget: SearchBudget,
) {
  const candidate = makeResult(chosen, totalCents, targetCents, timePruned)
  if (results.length < maxResults) {
    results.push(candidate)
    return
  }

  budget.truncated = true
  let worstIndex = 0
  for (let i = 1; i < results.length; i += 1) {
    if (compareResults(results[worstIndex], results[i]) < 0) worstIndex = i
  }
  if (compareResults(candidate, results[worstIndex]) < 0) {
    results[worstIndex] = candidate
  }
}

function dfs(
  items: DfsItem[],
  idx: number,
  remaining: number,
  toleranceCents: number,
  maxDepth: number,
  chosen: DfsItem[],
  results: WorkerResult[],
  targetCents: number,
  timePruned: boolean,
  suffixSums: number[],
  maxResults: number,
  budget: SearchBudget,
): void {
  if (budget.stopped || !consumeSearchNode(budget)) return
  // All amounts are positive: once over the upper bound, no suffix can help.
  if (remaining < -toleranceCents) return
  if (Math.abs(remaining) <= toleranceCents && chosen.length > 0) {
    const totalCents = targetCents - remaining
    recordResult(chosen, totalCents, targetCents, timePruned, results, maxResults, budget)
    return
  }
  if (idx >= items.length || chosen.length >= maxDepth) return
  // Even taking the complete suffix cannot reach the lower tolerance bound.
  if (remaining - suffixSums[idx] > toleranceCents) return

  const item = items[idx]
  // Take item
  chosen.push(item)
  dfs(
    items, idx + 1, remaining - item.amountCents, toleranceCents, maxDepth,
    chosen, results, targetCents, timePruned, suffixSums, maxResults, budget,
  )
  chosen.pop()
  // Skip item
  dfs(
    items, idx + 1, remaining, toleranceCents, maxDepth,
    chosen, results, targetCents, timePruned, suffixSums, maxResults, budget,
  )
}

function positiveInteger(value: number, fallback: number): number {
  return Number.isFinite(value) && value > 0 ? Math.max(1, Math.floor(value)) : fallback
}

export function runBoundedMatch(
  rawItems: WorkerTxItem[],
  targetCents: number,
  toleranceCents: number,
  maxDepth: number,
  limit: number,
  overrides: Partial<MatchExecutionLimits> = {},
): MatchRunOutcome {
  if (
    !Number.isSafeInteger(targetCents) || targetCents <= 0 ||
    !Number.isSafeInteger(toleranceCents) || toleranceCents < 0 ||
    toleranceCents > targetCents ||
    !Number.isSafeInteger(targetCents + toleranceCents)
  ) {
    throw new Error('Invalid matching request')
  }

  const limits: MatchExecutionLimits = {
    maxScannedCandidates: positiveInteger(
      overrides.maxScannedCandidates ?? DEFAULT_MATCH_EXECUTION_LIMITS.maxScannedCandidates,
      DEFAULT_MATCH_EXECUTION_LIMITS.maxScannedCandidates,
    ),
    maxCandidates: positiveInteger(
      overrides.maxCandidates ?? DEFAULT_MATCH_EXECUTION_LIMITS.maxCandidates,
      DEFAULT_MATCH_EXECUTION_LIMITS.maxCandidates,
    ),
    maxNodes: positiveInteger(
      overrides.maxNodes ?? DEFAULT_MATCH_EXECUTION_LIMITS.maxNodes,
      DEFAULT_MATCH_EXECUTION_LIMITS.maxNodes,
    ),
    maxDurationMs: positiveInteger(
      overrides.maxDurationMs ?? DEFAULT_MATCH_EXECUTION_LIMITS.maxDurationMs,
      DEFAULT_MATCH_EXECUTION_LIMITS.maxDurationMs,
    ),
    maxResults: positiveInteger(
      overrides.maxResults ?? DEFAULT_MATCH_EXECUTION_LIMITS.maxResults,
      DEFAULT_MATCH_EXECUTION_LIMITS.maxResults,
    ),
    clock: overrides.clock ?? DEFAULT_MATCH_EXECUTION_LIMITS.clock,
  }
  const normalizedDepth = Math.min(50, positiveInteger(maxDepth, 10))
  const requestedLimit = Math.min(20, positiveInteger(limit, 20))
  const resultCapacity = Math.min(requestedLimit, limits.maxResults)
  const upper = targetCents + toleranceCents
  const nowSec = Date.now() / 1000
  let truncated = rawItems.length > limits.maxScannedCandidates
  let timePruned = false

  function toDfsItems(candidates: WorkerTxItem[]): DfsItem[] {
    // Sort descending by amount, then ascending by occurredTs
    return candidates
      .map(i => ({
        ...i,
        ageDays: Math.min(365, Math.max(0, (nowSec - i.occurredTs) / 86400)),
      }))
      .sort((a, b) =>
        b.amountCents - a.amountCents ||
        a.occurredTs - b.occurredTs ||
        a.id.localeCompare(b.id)
      )
  }

  const scannedItems = rawItems.slice(0, limits.maxScannedCandidates)
  let eligibleItems = scannedItems.filter((item) =>
    typeof item.id === 'string' && item.id.length > 0 &&
    Number.isSafeInteger(item.amountCents) &&
    item.amountCents > 0 && item.amountCents <= upper &&
    Number.isFinite(item.occurredTs)
  )
  if (eligibleItems.length > limits.maxCandidates) {
    truncated = true
    eligibleItems = eligibleItems.slice(0, limits.maxCandidates)
  }
  let items = toDfsItems(eligibleItems)

  // N×W threshold
  if (items.length > 0 && targetCents > DP_THRESHOLD / items.length) {
    const cutoff = nowSec - TIME_PRUNE_DAYS * 86400
    const recentItems = items.filter(i => i.occurredTs >= cutoff)
    if (recentItems.length > 0) {
      items = recentItems
      timePruned = true
    }
  }

  // Suffix sums for pruning
  const suffixSums = new Array(items.length + 1).fill(0)
  for (let i = items.length - 1; i >= 0; i--) {
    suffixSums[i] = Math.min(upper, suffixSums[i + 1] + items[i].amountCents)
  }

  const results: WorkerResult[] = []
  const budget: SearchBudget = {
    nodes: 0,
    startedAt: limits.clock(),
    maxNodes: limits.maxNodes,
    maxDurationMs: limits.maxDurationMs,
    clock: limits.clock,
    stopped: false,
    truncated: false,
  }
  dfs(
    items, 0, targetCents, toleranceCents, normalizedDepth, [], results,
    targetCents, timePruned, suffixSums, resultCapacity, budget,
  )

  // Sort: error asc, then score desc
  results.sort(compareResults)
  return {
    results,
    truncated: truncated || budget.truncated,
    timePruned,
  }
}

if (typeof self !== 'undefined') {
  self.onmessage = (e: MessageEvent<MatchWorkerRequest>) => {
    const { requestId, targetCents, toleranceCents, maxDepth, limit, items } = e.data
    try {
      const outcome = runBoundedMatch(items, targetCents, toleranceCents, maxDepth, limit)
      const response: MatchWorkerResponse = { requestId, ok: true, ...outcome }
      self.postMessage(response)
    } catch (err) {
      const response: MatchWorkerResponse = { requestId, ok: false, error: String(err) }
      self.postMessage(response)
    }
  }
}
