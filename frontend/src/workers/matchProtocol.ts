export const MATCH_MAX_INPUT_CANDIDATES = 2_000

export interface WorkerTxItem {
  id: string
  amountCents: number
  occurredTs: number
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

export interface MatchWorkerRequest {
  requestId: number
  targetCents: number
  toleranceCents: number
  maxDepth: number
  limit: number
  items: WorkerTxItem[]
}

export interface MatchWorkerResponse {
  requestId: number
  ok: boolean
  results?: WorkerResult[]
  truncated?: boolean
  timePruned?: boolean
  error?: string
}

export function isCurrentMatchResponse(response: MatchWorkerResponse, activeRequestId: number): boolean {
  return response.requestId === activeRequestId
}
