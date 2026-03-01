/**
 * Web Worker: integer-cent subset-sum matching (mirrors Go dfsCents logic).
 *
 * Input message:
 *   { targetCents: number, toleranceCents: number, maxDepth: number, limit: number, items: WorkerTxItem[] }
 *
 * Output message (array of WorkerResult, sorted by score desc):
 *   WorkerResult[]
 */

export interface WorkerTxItem {
  id: string
  amountCents: number   // positive integer
  occurredTs: number    // Unix timestamp seconds
  projectId?: string
}

export interface WorkerResult {
  ids: string[]
  totalCents: number
  errorCents: number
  projectCount: number
  itemCount: number
  score: number
  timePruned: boolean
}

interface DfsItem extends WorkerTxItem {
  ageDays: number
}

const DP_THRESHOLD = 1_000_000_000 // N × W > 10^9 → time-prune
const TIME_PRUNE_DAYS = 90

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
): void {
  // Prune: even if we take everything remaining, can we reach tolerance?
  if (idx < items.length && suffixSums[idx] + remaining < -toleranceCents) return
  // Prune: even without taking any, are we already within tolerance AND nothing to add?
  if (Math.abs(remaining) <= toleranceCents && chosen.length > 0) {
    // valid result
    const totalCents = targetCents - remaining
    results.push(makeResult(chosen, totalCents, targetCents, timePruned))
    return
  }
  if (idx >= items.length || chosen.length >= maxDepth) return

  const item = items[idx]
  // Take item
  chosen.push(item)
  dfs(items, idx + 1, remaining - item.amountCents, toleranceCents, maxDepth, chosen, results, targetCents, timePruned, suffixSums)
  chosen.pop()
  // Skip item
  dfs(items, idx + 1, remaining, toleranceCents, maxDepth, chosen, results, targetCents, timePruned, suffixSums)
}

function runMatch(
  rawItems: WorkerTxItem[],
  targetCents: number,
  toleranceCents: number,
  maxDepth: number,
  limit: number,
): WorkerResult[] {
  const nowSec = Date.now() / 1000
  let timePruned = false

  function toDfsItems(candidates: WorkerTxItem[]): DfsItem[] {
    // Sort descending by amount, then ascending by occurredTs
    return candidates
      .map(i => ({ ...i, ageDays: (nowSec - i.occurredTs) / 86400 }))
      .sort((a, b) => b.amountCents - a.amountCents || a.occurredTs - b.occurredTs)
  }

  let items = toDfsItems(rawItems)

  // N×W threshold
  if (items.length * targetCents > DP_THRESHOLD) {
    timePruned = true
    const cutoff = nowSec - TIME_PRUNE_DAYS * 86400
    items = toDfsItems(rawItems.filter(i => i.occurredTs >= cutoff))
  }

  // Suffix sums for pruning
  const suffixSums = new Array(items.length + 1).fill(0)
  for (let i = items.length - 1; i >= 0; i--) {
    suffixSums[i] = suffixSums[i + 1] + items[i].amountCents
  }

  const results: WorkerResult[] = []
  dfs(items, 0, targetCents, toleranceCents, maxDepth, [], results, targetCents, timePruned, suffixSums)

  // Sort: error asc, then score desc
  results.sort((a, b) => a.errorCents - b.errorCents || b.score - a.score)

  return results.slice(0, limit)
}

self.onmessage = (e: MessageEvent<{
  targetCents: number
  toleranceCents: number
  maxDepth: number
  limit: number
  items: WorkerTxItem[]
}>) => {
  const { targetCents, toleranceCents, maxDepth, limit, items } = e.data
  try {
    const results = runMatch(items, targetCents, toleranceCents, maxDepth, limit)
    self.postMessage({ ok: true, results })
  } catch (err) {
    self.postMessage({ ok: false, error: String(err) })
  }
}
