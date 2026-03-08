import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { executeDisasterRecovery, listDisasterSnapshots, type DisasterSnapshot } from '../api/client'

type Step = 'select' | 'confirm' | 'done'

export default function DisasterRestorePage() {
  const [snapshots, setSnapshots] = useState<DisasterSnapshot[]>([])
  const [selected, setSelected] = useState<DisasterSnapshot | null>(null)
  const [step, setStep] = useState<Step>('select')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [allowMissingMetadata, setAllowMissingMetadata] = useState(false)
  const [result, setResult] = useState<{ recovery_id: string; schema_after: number; duration_ms: number } | null>(null)

  useEffect(() => {
    setLoading(true)
    listDisasterSnapshots()
      .then((items) => {
        setSnapshots(items)
        if (items.length > 0) setSelected(items[0])
      })
      .catch((err: unknown) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setError(msg || 'Failed to load snapshots')
      })
      .finally(() => setLoading(false))
  }, [])

  const selectedLabel = useMemo(() => {
    if (!selected) return ''
    return `${selected.snapshot_id} · schema v${selected.schema_version} · ${(selected.db_size / 1024 / 1024).toFixed(1)}MB`
  }, [selected])

  async function startRestore() {
    if (!selected) return
    setLoading(true)
    setError('')
    try {
      const res = await executeDisasterRecovery(selected.snapshot_id, allowMissingMetadata)
      setResult({ recovery_id: res.recovery_id, schema_after: res.schema_after, duration_ms: res.duration_ms })
      setStep('done')
      toast.success('Disaster recovery completed')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || 'Restore failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-950 flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-2xl bg-white dark:bg-gray-900 rounded-2xl border border-gray-100 dark:border-gray-800 shadow-sm p-6 space-y-5">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100">Disaster Recovery</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400">Restore database snapshot from Litestream/R2 with metadata validation.</p>
        </div>

        {step === 'select' && (
          <>
            <div className="space-y-2">
              <label className="text-xs uppercase tracking-wider text-gray-500">Available snapshots</label>
              <select
                className="w-full rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 px-3 py-2"
                value={selected?.snapshot_id ?? ''}
                onChange={(e) => setSelected(snapshots.find((s) => s.snapshot_id === e.target.value) ?? null)}
                disabled={loading || snapshots.length === 0}
              >
                {snapshots.map((s) => (
                  <option key={s.snapshot_id} value={s.snapshot_id}>
                    {s.snapshot_id} | schema v{s.schema_version} | {(s.db_size / 1024 / 1024).toFixed(1)}MB
                  </option>
                ))}
              </select>
            </div>

            {selected && (
              <div className="rounded-xl bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-700 p-4 text-sm space-y-1">
                <div><b>Snapshot:</b> {selected.snapshot_id}</div>
                <div><b>Created:</b> {selected.created_at}</div>
                <div><b>Schema:</b> v{selected.schema_version}</div>
                <div><b>App:</b> {selected.app_version || '-'}</div>
                <div><b>Environment:</b> {selected.environment || '-'}</div>
                <div><b>Size:</b> {(selected.db_size / 1024 / 1024).toFixed(1)} MB</div>
                {!selected.has_metadata && <div className="text-rose-600">Metadata missing. Manual override required.</div>}
              </div>
            )}

            <button
              disabled={!selected || loading}
              onClick={() => setStep('confirm')}
              className="w-full bg-violet-600 hover:bg-violet-700 disabled:opacity-50 text-white font-semibold rounded-xl py-2.5"
            >
              Continue
            </button>
          </>
        )}

        {step === 'confirm' && selected && (
          <div className="space-y-4">
            <p className="text-sm text-gray-600 dark:text-gray-300">You are about to restore <b>{selectedLabel}</b>. This will replace the live database.</p>
            {!selected.has_metadata && (
              <label className="flex gap-2 items-start text-sm">
                <input type="checkbox" checked={allowMissingMetadata} onChange={(e) => setAllowMissingMetadata(e.target.checked)} />
                I understand metadata is missing and still want to continue.
              </label>
            )}
            <div className="flex gap-2">
              <button className="flex-1 rounded-xl border border-gray-300 py-2" onClick={() => setStep('select')}>Back</button>
              <button
                className="flex-1 bg-rose-600 hover:bg-rose-700 disabled:opacity-50 text-white rounded-xl py-2"
                disabled={loading || (!selected.has_metadata && !allowMissingMetadata)}
                onClick={startRestore}
              >
                {loading ? 'Restoring...' : 'Confirm Restore'}
              </button>
            </div>
          </div>
        )}

        {step === 'done' && result && (
          <div className="rounded-xl bg-emerald-50 dark:bg-emerald-900/20 border border-emerald-200 dark:border-emerald-700 p-4 text-sm space-y-1">
            <div className="font-semibold text-emerald-700 dark:text-emerald-300">Recovery complete</div>
            <div>Recovery ID: {result.recovery_id}</div>
            <div>Schema after: v{result.schema_after}</div>
            <div>Duration: {(result.duration_ms / 1000).toFixed(2)}s</div>
          </div>
        )}

        {error && <div className="text-sm text-rose-600">{error}</div>}

        <div className="text-sm text-center pt-2">
          <Link className="text-violet-600 hover:underline" to="/login">Back to login</Link>
        </div>
      </div>
    </div>
  )
}
